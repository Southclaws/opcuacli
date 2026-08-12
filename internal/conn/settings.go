// Package conn turns configuration and flags into a connected OPC UA client.
// It owns security policy negotiation, credential resolution, client
// certificate provisioning, and the failover order that lets one command try
// several endpoints of a redundant server set.
package conn

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/config"
)

// Settings is everything needed to open a session, after the configuration file
// and the command line have both had their say.
type Settings struct {
	// Profile names the configuration profile these settings came from, or ""
	// when everything was given on the command line.
	Profile string

	// Endpoints is the failover order: each is tried in turn until one answers.
	Endpoints []string

	// Policy and Mode are a security policy short name and message security
	// mode, or "auto" to negotiate from what the server offers.
	Policy string
	Mode   string

	// AuthMode is anonymous, username, certificate, or auto.
	AuthMode string
	Username string
	Password string

	CertFile string
	KeyFile  string

	// Insecure skips validity checking of the server's certificate.
	Insecure bool

	RequestTimeout time.Duration
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	Reconnect      bool

	ApplicationURI string
	Locales        []string

	// SubscriptionInterval and QueueSize are the profile's subscription
	// defaults, used when a command does not override them.
	SubscriptionInterval time.Duration
	QueueSize            uint32

	// Verbosity is the -v count; Trace enables gopcua's own wire logging.
	Verbosity int
	Trace     bool
}

// Endpoint is the first endpoint in the failover order, which is the one a
// single-server setup uses.
func (s *Settings) Endpoint() string {
	if len(s.Endpoints) == 0 {
		return config.DefaultEndpoint
	}
	return s.Endpoints[0]
}

// Secure reports whether the settings ask for anything beyond an unsecured
// channel, which is what decides whether a client certificate is needed.
func (s *Settings) Secure() bool {
	policy := strings.ToLower(s.Policy)
	mode := strings.ToLower(s.Mode)
	if policy == "auto" || mode == "auto" {
		// Negotiation prefers the strongest endpoint on offer, so a key pair
		// may well be needed.
		return true
	}
	return policy != "none" && policy != "" || mode != "none" && mode != ""
}

// FromCommand resolves the settings for one invocation: the configuration file
// is loaded, the profile is selected, and any flag the user actually set
// overrides it. Reading the flags rather than the generated parameter struct
// keeps this in one place, since the connection flags are global and therefore
// present on every command.
func FromCommand(cmd *cobra.Command, stdin io.Reader) (*Settings, *config.File, error) {
	flags := cmd.Flags()
	changed := func(name string) bool { return flags.Changed(name) }
	text := func(name string) string {
		value, _ := flags.GetString(name)
		return value
	}
	duration := func(name string) time.Duration {
		value, _ := flags.GetDuration(name)
		return value
	}
	boolean := func(name string) bool {
		value, _ := flags.GetBool(name)
		return value
	}
	list := func(name string) []string {
		value, _ := flags.GetStringArray(name)
		return value
	}

	file, err := config.Load(text("config"))
	if err != nil {
		return nil, nil, err
	}

	name, profile, err := file.Resolve(text("profile"))
	if err != nil {
		return nil, nil, err
	}

	settings := &Settings{
		Profile:        name,
		Endpoints:      profile.Endpoints,
		Policy:         orDefault(profile.Security.Policy, "auto"),
		Mode:           orDefault(profile.Security.Mode, "auto"),
		AuthMode:       orDefault(profile.Auth.Mode, "auto"),
		Username:       profile.Auth.Username,
		CertFile:       profile.Security.Certificate,
		KeyFile:        profile.Security.PrivateKey,
		Insecure:       profile.Security.Insecure,
		RequestTimeout: duration("timeout"),
		DialTimeout:    duration("dial-timeout"),
		SessionTimeout: duration("session-timeout"),
		Reconnect:      !profile.Session.NoReconnect,
		ApplicationURI: profile.Session.ApplicationURI,
		Locales:        profile.Session.Locales,
		Verbosity:      count(cmd, "verbose"),
		Trace:          boolean("trace"),
	}

	// Durations from the profile only apply when the flag still holds its
	// default, so an explicit --timeout always wins.
	for _, item := range []struct {
		flag  string
		value string
		field *time.Duration
	}{
		{"timeout", profile.Session.RequestTimeout, &settings.RequestTimeout},
		{"dial-timeout", profile.Session.DialTimeout, &settings.DialTimeout},
		{"session-timeout", profile.Session.Timeout, &settings.SessionTimeout},
	} {
		if changed(item.flag) || item.value == "" {
			continue
		}
		parsed, err := time.ParseDuration(item.value)
		if err != nil {
			return nil, nil, fmt.Errorf("profile %q: %s: %w", name, item.flag, err)
		}
		*item.field = parsed
	}

	if profile.Subscription.Interval != "" {
		parsed, err := time.ParseDuration(profile.Subscription.Interval)
		if err != nil {
			return nil, nil, fmt.Errorf("profile %q: subscription.interval: %w", name, err)
		}
		settings.SubscriptionInterval = parsed
	}
	if profile.Subscription.QueueSize > 0 {
		settings.QueueSize = uint32(profile.Subscription.QueueSize)
	}

	if endpoints := list("endpoint"); len(endpoints) > 0 {
		settings.Endpoints = endpoints
	}
	if len(settings.Endpoints) == 0 {
		settings.Endpoints = []string{config.DefaultEndpoint}
	}

	for _, override := range []struct {
		flag  string
		field *string
	}{
		{"policy", &settings.Policy},
		{"mode", &settings.Mode},
		{"auth", &settings.AuthMode},
		{"username", &settings.Username},
		{"cert", &settings.CertFile},
		{"key", &settings.KeyFile},
		{"app-uri", &settings.ApplicationURI},
	} {
		if changed(override.flag) {
			*override.field = text(override.flag)
		}
	}
	if changed("insecure") {
		settings.Insecure = boolean("insecure")
	}
	if changed("no-reconnect") {
		settings.Reconnect = !boolean("no-reconnect")
	}
	if locales := list("locale"); len(locales) > 0 {
		settings.Locales = locales
	}

	password, err := resolvePassword(text("password"), text("password-file"), profile, stdin)
	if err != nil {
		return nil, nil, err
	}
	settings.Password = password

	// A user name with no explicit mode means username authentication; that is
	// what supplying one is for.
	if settings.AuthMode == "auto" && settings.Username != "" {
		settings.AuthMode = "username"
	}

	if settings.ApplicationURI == "" {
		settings.ApplicationURI = defaultApplicationURI()
	}

	return settings, file, nil
}

// resolvePassword takes the password from the highest-precedence source that
// has one: the flag, a file named by the flag, the profile's password, or the
// profile's password file. A file of "-" is read from stdin, which is how a
// password reaches a command without appearing in the process list.
func resolvePassword(flagValue, flagFile string, profile *config.Profile, stdin io.Reader) (string, error) {
	switch {
	case flagValue != "":
		return flagValue, nil
	case flagFile != "":
		return readPasswordFile(flagFile, stdin)
	case profile.Auth.Password != "":
		return profile.Auth.Password, nil
	case profile.Auth.PasswordFile != "":
		return readPasswordFile(profile.Auth.PasswordFile, stdin)
	default:
		return "", nil
	}
}

func readPasswordFile(path string, stdin io.Reader) (string, error) {
	if path == "-" {
		raw, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("read password from stdin: %w", err)
		}
		return strings.TrimRight(string(raw), "\r\n"), nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read password file: %w", err)
	}
	return strings.TrimRight(string(raw), "\r\n"), nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func count(cmd *cobra.Command, name string) int {
	value, err := cmd.Flags().GetCount(name)
	if err != nil {
		return 0
	}
	return value
}
