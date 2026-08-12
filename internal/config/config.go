// Package config reads and writes the connection profiles that let a command
// name a server instead of describing one. A profile holds everything needed to
// open a session; every field can be overridden by a flag for a one-off check.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// File is the whole configuration document.
type File struct {
	// Default names the profile used when no --profile is given.
	Default string `yaml:"default,omitempty"`
	// Profiles are keyed by the name used with --profile.
	Profiles map[string]*Profile `yaml:"profiles"`

	// path records where this document was read from so Save can write it
	// back without the caller having to thread the location through.
	path string
}

// Profile is one server a command can be pointed at.
//
// Endpoints is a list rather than a single URL because a redundant server set
// expects the client to fail over on its own: every command tries the endpoints
// in order and uses the first that answers.
type Profile struct {
	Description  string       `yaml:"description,omitempty"`
	Endpoints    []string     `yaml:"endpoints"`
	Security     Security     `yaml:"security,omitempty"`
	Auth         Auth         `yaml:"auth,omitempty"`
	Session      Session      `yaml:"session,omitempty"`
	Subscription Subscription `yaml:"subscription,omitempty"`
}

// Security selects how the secure channel is protected.
type Security struct {
	// Policy is a security policy short name such as Basic256Sha256, None, or
	// auto to take the strongest the server offers.
	Policy string `yaml:"policy,omitempty"`
	// Mode is None, Sign, SignAndEncrypt, or auto.
	Mode string `yaml:"mode,omitempty"`
	// Certificate and PrivateKey are the client's own key pair, needed for any
	// policy other than None.
	Certificate string `yaml:"certificate,omitempty"`
	PrivateKey  string `yaml:"privateKey,omitempty"`
	// Insecure accepts a server certificate that fails validity checks.
	Insecure bool `yaml:"insecure,omitempty"`
}

// Auth selects how the session proves who the client is.
type Auth struct {
	// Mode is anonymous, username, certificate, or auto to pick from what the
	// endpoint accepts and what credentials are configured.
	Mode     string `yaml:"mode,omitempty"`
	Username string `yaml:"username,omitempty"`
	// Password is stored in plain text, so PasswordFile or the OPCUA_PASSWORD
	// environment variable is preferable on a shared machine.
	Password     string `yaml:"password,omitempty"`
	PasswordFile string `yaml:"passwordFile,omitempty"`
}

// Session tunes the session itself. Durations are written as strings such as
// "30s" so the file stays readable.
type Session struct {
	Timeout        string   `yaml:"timeout,omitempty"`
	RequestTimeout string   `yaml:"requestTimeout,omitempty"`
	DialTimeout    string   `yaml:"dialTimeout,omitempty"`
	ApplicationURI string   `yaml:"applicationUri,omitempty"`
	Locales        []string `yaml:"locales,omitempty"`
	// NoReconnect disables transparently restoring a dropped session.
	NoReconnect bool `yaml:"noReconnect,omitempty"`
}

// Subscription holds the defaults for monitor and the TUI's watch panel.
type Subscription struct {
	Interval  string `yaml:"interval,omitempty"`
	QueueSize int    `yaml:"queueSize,omitempty"`
}

// ErrNoProfile reports that a named profile is not in the file.
var ErrNoProfile = errors.New("no such profile")

// DefaultEndpoint is used when nothing at all has been configured, so that a
// bare `opcua browse` does something sensible on a developer's machine.
const DefaultEndpoint = "opc.tcp://localhost:4840"

// Dir is the directory holding the configuration file and generated
// certificates: the per-user configuration directory the platform defines, which
// is %AppData% on Windows, Application Support on macOS, and XDG_CONFIG_HOME or
// ~/.config elsewhere.
//
// The fallback is a relative directory, used only when the platform cannot say
// where a user's configuration belongs. --config and OPCUA_CONFIG override the
// file's location outright.
func Dir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "opcua"
	}
	return filepath.Join(dir, "opcua")
}

// Path is the configuration file's location. An explicit override wins, then
// OPCUA_CONFIG, then the default location.
func Path(override string) string {
	if override != "" {
		return override
	}
	if fromEnv := os.Getenv("OPCUA_CONFIG"); fromEnv != "" {
		return fromEnv
	}
	return filepath.Join(Dir(), "config.yaml")
}

// CertDir is where generated client key pairs are kept.
func CertDir() string { return filepath.Join(Dir(), "certs") }

// Load reads the configuration file. A missing file is not an error: it yields
// an empty document, so every command works before anything is configured.
func Load(override string) (*File, error) {
	path := Path(override)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &File{Profiles: map[string]*Profile{}, path: path}, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var file File
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if file.Profiles == nil {
		file.Profiles = map[string]*Profile{}
	}
	file.path = path

	return &file, nil
}

// SourcePath is the file this document was read from.
func (f *File) SourcePath() string { return f.path }

// Save writes the document back, creating the directory if needed. The file is
// written 0600 because it can hold a password.
func (f *File) Save() error {
	if f.path == "" {
		f.path = Path("")
	}
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(f.path), err)
	}

	data, err := yaml.MarshalWithOptions(f, yaml.Indent(2), yaml.IndentSequence(true))
	if err != nil {
		return err
	}

	header := "# opcua connection profiles. Edit by hand or with `opcua config set`.\n"
	if err := os.WriteFile(f.path, append([]byte(header), data...), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", f.path, err)
	}
	return nil
}

// Names lists the profile names in alphabetical order.
func (f *File) Names() []string {
	names := make([]string, 0, len(f.Profiles))
	for name := range f.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultName is the profile a command uses when none is named: the declared
// default, or the only profile when there is exactly one, so a single-server
// setup needs no `default:` key at all.
func (f *File) DefaultName() string {
	if f.Default != "" {
		return f.Default
	}
	if len(f.Profiles) == 1 {
		return f.Names()[0]
	}
	return ""
}

// Resolve returns the named profile, or the default one when name is empty. A
// name that is not present is an error, but an empty file is not: it resolves
// to an unnamed, empty profile that flags can fill in.
func (f *File) Resolve(name string) (string, *Profile, error) {
	if name == "" {
		name = f.DefaultName()
	}
	if name == "" {
		return "", &Profile{}, nil
	}

	profile, ok := f.Profiles[name]
	if !ok {
		return "", nil, fmt.Errorf("%w: %q (configured: %s)", ErrNoProfile, name, strings.Join(f.Names(), ", "))
	}
	if profile == nil {
		profile = &Profile{}
	}
	return name, profile, nil
}

// Upsert returns the named profile, creating it when absent.
func (f *File) Upsert(name string) *Profile {
	if f.Profiles == nil {
		f.Profiles = map[string]*Profile{}
	}
	if profile, ok := f.Profiles[name]; ok && profile != nil {
		return profile
	}
	profile := &Profile{}
	f.Profiles[name] = profile
	if f.Default == "" {
		f.Default = name
	}
	return profile
}

// Remove deletes a profile, clearing the default when it pointed at it.
func (f *File) Remove(name string) error {
	if _, ok := f.Profiles[name]; !ok {
		return fmt.Errorf("%w: %q", ErrNoProfile, name)
	}
	delete(f.Profiles, name)
	if f.Default == name {
		f.Default = ""
	}
	return nil
}

// Endpoint is the profile's primary endpoint, or the built-in default.
func (p *Profile) Endpoint() string {
	if len(p.Endpoints) > 0 {
		return p.Endpoints[0]
	}
	return DefaultEndpoint
}

// Redacted returns a copy with secrets replaced, for printing.
func (p *Profile) Redacted() *Profile {
	clone := *p
	if clone.Auth.Password != "" {
		clone.Auth.Password = "••••••••"
	}
	return &clone
}

// SettableKeys are the dotted keys `config set` understands, in the order they
// are listed in help.
var SettableKeys = []string{
	"description",
	"endpoint",
	"endpoints",
	"security.policy",
	"security.mode",
	"security.certificate",
	"security.privateKey",
	"security.insecure",
	"auth.mode",
	"auth.username",
	"auth.password",
	"auth.passwordFile",
	"session.timeout",
	"session.requestTimeout",
	"session.dialTimeout",
	"session.applicationUri",
	"session.locales",
	"session.noReconnect",
	"subscription.interval",
	"subscription.queueSize",
}

// Set assigns one dotted key on a profile. List-valued keys take a
// comma-separated value; boolean keys take anything strconv.ParseBool accepts.
func (p *Profile) Set(key, value string) error {
	list := func() []string {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		parts := strings.Split(value, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}

	boolean := func() (bool, error) {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("%s must be true or false, got %q", key, value)
		}
		return parsed, nil
	}

	duration := func() (string, error) {
		if value == "" {
			return "", nil
		}
		if _, err := time.ParseDuration(value); err != nil {
			return "", fmt.Errorf("%s must be a duration such as 30s, got %q", key, value)
		}
		return value, nil
	}

	var err error
	switch key {
	case "description":
		p.Description = value
	case "endpoint", "endpoints":
		p.Endpoints = list()
	case "security.policy":
		p.Security.Policy = value
	case "security.mode":
		p.Security.Mode = value
	case "security.certificate":
		p.Security.Certificate = value
	case "security.privateKey":
		p.Security.PrivateKey = value
	case "security.insecure":
		p.Security.Insecure, err = boolean()
	case "auth.mode":
		p.Auth.Mode = value
	case "auth.username":
		p.Auth.Username = value
	case "auth.password":
		p.Auth.Password = value
	case "auth.passwordFile":
		p.Auth.PasswordFile = value
	case "session.timeout":
		p.Session.Timeout, err = duration()
	case "session.requestTimeout":
		p.Session.RequestTimeout, err = duration()
	case "session.dialTimeout":
		p.Session.DialTimeout, err = duration()
	case "session.applicationUri":
		p.Session.ApplicationURI = value
	case "session.locales":
		p.Session.Locales = list()
	case "session.noReconnect":
		p.Session.NoReconnect, err = boolean()
	case "subscription.interval":
		p.Subscription.Interval, err = duration()
	case "subscription.queueSize":
		size, parseErr := strconv.Atoi(value)
		if parseErr != nil {
			return fmt.Errorf("subscription.queueSize must be a number, got %q", value)
		}
		p.Subscription.QueueSize = size
	default:
		return fmt.Errorf("unknown setting %q (known: %s)", key, strings.Join(SettableKeys, ", "))
	}

	return err
}
