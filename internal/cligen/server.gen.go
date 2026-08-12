package cligen

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/goccy/go-yaml"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type ServerInfoFormat string

const (
	ServerInfoFormatText     ServerInfoFormat = "text"
	ServerInfoFormatMarkdown ServerInfoFormat = "markdown"
	ServerInfoFormatJson     ServerInfoFormat = "json"
	ServerInfoFormatYaml     ServerInfoFormat = "yaml"
)

type ServerInfoParams struct {
	Config         string
	Profile        string
	Endpoint       []string
	Username       string
	Password       string
	PasswordFile   string
	Auth           string
	Policy         string
	Mode           string
	Cert           string
	Key            string
	Insecure       bool
	Timeout        time.Duration
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	NoReconnect    bool
	AppUri         string
	Locale         []string
	Verbose        int
	Trace          bool
	NoColor        bool
	Format         ServerInfoFormat
}
type ServerInfoHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ServerInfoParams) (ServerInfo, error)

func newServerInfoCommand(serverInfo ServerInfoHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show server status, build info and uptime.",
		Long: `Reads Server/ServerStatus: the run state, start time, build info and
the server's own clock, plus the service level that a redundant pair
uses to advertise which member to prefer.
`,
		Aliases: []string{"status"},
		Example: `  # Status of the active profile's server.
  opcua server info
  # Rendered Markdown, for pasting into a report.
  opcua server info --format markdown`,
		Args: rangeArgs(0, 0),
	}

	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "text", "Output format: text, markdown, json, yaml.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("config") {
			if v := os.Getenv("OPCUA_CONFIG"); v != "" {
				_ = cmd.Flags().Set("config", v)
			}
		}
		if !cmd.Flags().Changed("profile") {
			if v := os.Getenv("OPCUA_PROFILE"); v != "" {
				_ = cmd.Flags().Set("profile", v)
			}
		}
		if !cmd.Flags().Changed("endpoint") {
			if v := os.Getenv("OPCUA_ENDPOINT"); v != "" {
				_ = cmd.Flags().Set("endpoint", v)
			}
		}
		if !cmd.Flags().Changed("username") {
			if v := os.Getenv("OPCUA_USERNAME"); v != "" {
				_ = cmd.Flags().Set("username", v)
			}
		}
		if !cmd.Flags().Changed("password") {
			if v := os.Getenv("OPCUA_PASSWORD"); v != "" {
				_ = cmd.Flags().Set("password", v)
			}
		}
		if !cmd.Flags().Changed("cert") {
			if v := os.Getenv("OPCUA_CERT"); v != "" {
				_ = cmd.Flags().Set("cert", v)
			}
		}
		if !cmd.Flags().Changed("key") {
			if v := os.Getenv("OPCUA_KEY"); v != "" {
				_ = cmd.Flags().Set("key", v)
			}
		}
		if !cmd.Flags().Changed("timeout") {
			if v := os.Getenv("OPCUA_TIMEOUT"); v != "" {
				_ = cmd.Flags().Set("timeout", v)
			}
		}
		if !cmd.Flags().Changed("no-color") {
			if v := os.Getenv("NO_COLOR"); v != "" {
				_ = cmd.Flags().Set("no-color", v)
			}
		}
		rawConfig, _ := cmd.Flags().GetString("config")
		rawProfile, _ := cmd.Flags().GetString("profile")
		rawEndpoint, _ := cmd.Flags().GetStringArray("endpoint")
		rawUsername, _ := cmd.Flags().GetString("username")
		rawPassword, _ := cmd.Flags().GetString("password")
		rawPasswordFile, _ := cmd.Flags().GetString("password-file")
		rawAuth, _ := cmd.Flags().GetString("auth")
		rawPolicy, _ := cmd.Flags().GetString("policy")
		rawMode, _ := cmd.Flags().GetString("mode")
		rawCert, _ := cmd.Flags().GetString("cert")
		rawKey, _ := cmd.Flags().GetString("key")
		rawInsecure, _ := cmd.Flags().GetBool("insecure")
		rawTimeout, _ := cmd.Flags().GetDuration("timeout")
		rawDialTimeout, _ := cmd.Flags().GetDuration("dial-timeout")
		rawSessionTimeout, _ := cmd.Flags().GetDuration("session-timeout")
		rawNoReconnect, _ := cmd.Flags().GetBool("no-reconnect")
		rawAppUri, _ := cmd.Flags().GetString("app-uri")
		rawLocale, _ := cmd.Flags().GetStringArray("locale")
		rawVerbose, _ := cmd.Flags().GetCount("verbose")
		rawTrace, _ := cmd.Flags().GetBool("trace")
		rawNoColor, _ := cmd.Flags().GetBool("no-color")
		var format ServerInfoFormat
		if rawFormat != "" {
			switch rawFormat {
			case "text":
				format = ServerInfoFormatText
			case "markdown":
				format = ServerInfoFormatMarkdown
			case "json":
				format = ServerInfoFormatJson
			case "yaml":
				format = ServerInfoFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of text, markdown, json, yaml", rawFormat)
			}
		}

		p := ServerInfoParams{
			Config:         rawConfig,
			Profile:        rawProfile,
			Endpoint:       rawEndpoint,
			Username:       rawUsername,
			Password:       rawPassword,
			PasswordFile:   rawPasswordFile,
			Auth:           rawAuth,
			Policy:         rawPolicy,
			Mode:           rawMode,
			Cert:           rawCert,
			Key:            rawKey,
			Insecure:       rawInsecure,
			Timeout:        rawTimeout,
			DialTimeout:    rawDialTimeout,
			SessionTimeout: rawSessionTimeout,
			NoReconnect:    rawNoReconnect,
			AppUri:         rawAppUri,
			Locale:         rawLocale,
			Verbose:        rawVerbose,
			Trace:          rawTrace,
			NoColor:        rawNoColor,
			Format:         format,
		}

		result, err := serverInfo(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ServerInfoFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ServerInfoFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return cmd
}

type ServerCapabilitiesFormat string

const (
	ServerCapabilitiesFormatText     ServerCapabilitiesFormat = "text"
	ServerCapabilitiesFormatMarkdown ServerCapabilitiesFormat = "markdown"
	ServerCapabilitiesFormatJson     ServerCapabilitiesFormat = "json"
	ServerCapabilitiesFormatYaml     ServerCapabilitiesFormat = "yaml"
)

type ServerCapabilitiesParams struct {
	Config         string
	Profile        string
	Endpoint       []string
	Username       string
	Password       string
	PasswordFile   string
	Auth           string
	Policy         string
	Mode           string
	Cert           string
	Key            string
	Insecure       bool
	Timeout        time.Duration
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	NoReconnect    bool
	AppUri         string
	Locale         []string
	Verbose        int
	Trace          bool
	NoColor        bool
	Format         ServerCapabilitiesFormat
}
type ServerCapabilitiesHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ServerCapabilitiesParams) (Capabilities, error)

func newServerCapabilitiesCommand(serverCapabilities ServerCapabilitiesHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "capabilities",
		Short: "Show operational limits and supported profiles.",
		Long: `Reads Server/ServerCapabilities: the maximum nodes a single Read,
Write, Browse or HistoryRead may carry, the minimum supported
sampling rate, and the conformance profiles the server claims.

Worth checking before a bulk operation: exceeding a limit earns a
BadTooManyOperations instead of a partial result.
`,
		Aliases: []string{"caps"},
		Args:    rangeArgs(0, 0),
	}

	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "text", "Output format: text, markdown, json, yaml.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("config") {
			if v := os.Getenv("OPCUA_CONFIG"); v != "" {
				_ = cmd.Flags().Set("config", v)
			}
		}
		if !cmd.Flags().Changed("profile") {
			if v := os.Getenv("OPCUA_PROFILE"); v != "" {
				_ = cmd.Flags().Set("profile", v)
			}
		}
		if !cmd.Flags().Changed("endpoint") {
			if v := os.Getenv("OPCUA_ENDPOINT"); v != "" {
				_ = cmd.Flags().Set("endpoint", v)
			}
		}
		if !cmd.Flags().Changed("username") {
			if v := os.Getenv("OPCUA_USERNAME"); v != "" {
				_ = cmd.Flags().Set("username", v)
			}
		}
		if !cmd.Flags().Changed("password") {
			if v := os.Getenv("OPCUA_PASSWORD"); v != "" {
				_ = cmd.Flags().Set("password", v)
			}
		}
		if !cmd.Flags().Changed("cert") {
			if v := os.Getenv("OPCUA_CERT"); v != "" {
				_ = cmd.Flags().Set("cert", v)
			}
		}
		if !cmd.Flags().Changed("key") {
			if v := os.Getenv("OPCUA_KEY"); v != "" {
				_ = cmd.Flags().Set("key", v)
			}
		}
		if !cmd.Flags().Changed("timeout") {
			if v := os.Getenv("OPCUA_TIMEOUT"); v != "" {
				_ = cmd.Flags().Set("timeout", v)
			}
		}
		if !cmd.Flags().Changed("no-color") {
			if v := os.Getenv("NO_COLOR"); v != "" {
				_ = cmd.Flags().Set("no-color", v)
			}
		}
		rawConfig, _ := cmd.Flags().GetString("config")
		rawProfile, _ := cmd.Flags().GetString("profile")
		rawEndpoint, _ := cmd.Flags().GetStringArray("endpoint")
		rawUsername, _ := cmd.Flags().GetString("username")
		rawPassword, _ := cmd.Flags().GetString("password")
		rawPasswordFile, _ := cmd.Flags().GetString("password-file")
		rawAuth, _ := cmd.Flags().GetString("auth")
		rawPolicy, _ := cmd.Flags().GetString("policy")
		rawMode, _ := cmd.Flags().GetString("mode")
		rawCert, _ := cmd.Flags().GetString("cert")
		rawKey, _ := cmd.Flags().GetString("key")
		rawInsecure, _ := cmd.Flags().GetBool("insecure")
		rawTimeout, _ := cmd.Flags().GetDuration("timeout")
		rawDialTimeout, _ := cmd.Flags().GetDuration("dial-timeout")
		rawSessionTimeout, _ := cmd.Flags().GetDuration("session-timeout")
		rawNoReconnect, _ := cmd.Flags().GetBool("no-reconnect")
		rawAppUri, _ := cmd.Flags().GetString("app-uri")
		rawLocale, _ := cmd.Flags().GetStringArray("locale")
		rawVerbose, _ := cmd.Flags().GetCount("verbose")
		rawTrace, _ := cmd.Flags().GetBool("trace")
		rawNoColor, _ := cmd.Flags().GetBool("no-color")
		var format ServerCapabilitiesFormat
		if rawFormat != "" {
			switch rawFormat {
			case "text":
				format = ServerCapabilitiesFormatText
			case "markdown":
				format = ServerCapabilitiesFormatMarkdown
			case "json":
				format = ServerCapabilitiesFormatJson
			case "yaml":
				format = ServerCapabilitiesFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of text, markdown, json, yaml", rawFormat)
			}
		}

		p := ServerCapabilitiesParams{
			Config:         rawConfig,
			Profile:        rawProfile,
			Endpoint:       rawEndpoint,
			Username:       rawUsername,
			Password:       rawPassword,
			PasswordFile:   rawPasswordFile,
			Auth:           rawAuth,
			Policy:         rawPolicy,
			Mode:           rawMode,
			Cert:           rawCert,
			Key:            rawKey,
			Insecure:       rawInsecure,
			Timeout:        rawTimeout,
			DialTimeout:    rawDialTimeout,
			SessionTimeout: rawSessionTimeout,
			NoReconnect:    rawNoReconnect,
			AppUri:         rawAppUri,
			Locale:         rawLocale,
			Verbose:        rawVerbose,
			Trace:          rawTrace,
			NoColor:        rawNoColor,
			Format:         format,
		}

		result, err := serverCapabilities(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ServerCapabilitiesFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ServerCapabilitiesFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return cmd
}

type ServerDiagnosticsFormat string

const (
	ServerDiagnosticsFormatText     ServerDiagnosticsFormat = "text"
	ServerDiagnosticsFormatMarkdown ServerDiagnosticsFormat = "markdown"
	ServerDiagnosticsFormatJson     ServerDiagnosticsFormat = "json"
	ServerDiagnosticsFormatYaml     ServerDiagnosticsFormat = "yaml"
)

type ServerDiagnosticsParams struct {
	Config         string
	Profile        string
	Endpoint       []string
	Username       string
	Password       string
	PasswordFile   string
	Auth           string
	Policy         string
	Mode           string
	Cert           string
	Key            string
	Insecure       bool
	Timeout        time.Duration
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	NoReconnect    bool
	AppUri         string
	Locale         []string
	Verbose        int
	Trace          bool
	NoColor        bool
	Sessions       bool
	Format         ServerDiagnosticsFormat
}
type ServerDiagnosticsHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ServerDiagnosticsParams) (Diagnostics, error)

func newServerDiagnosticsCommand(serverDiagnostics ServerDiagnosticsHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diagnostics",
		Short: "Show diagnostic counters for the server and its sessions.",
		Long: `Reads Server/ServerDiagnostics. Diagnostics are optional and are
often disabled by default; if the counters come back as
BadNodeIdUnknown, set EnabledFlag on the server first.
`,
		Aliases: []string{"diag"},
		Args:    rangeArgs(0, 0),
	}

	var rawSessions bool
	cmd.Flags().BoolVar(&rawSessions, "sessions", false, "Include the per-session diagnostics array.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "text", "Output format: text, markdown, json, yaml.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("config") {
			if v := os.Getenv("OPCUA_CONFIG"); v != "" {
				_ = cmd.Flags().Set("config", v)
			}
		}
		if !cmd.Flags().Changed("profile") {
			if v := os.Getenv("OPCUA_PROFILE"); v != "" {
				_ = cmd.Flags().Set("profile", v)
			}
		}
		if !cmd.Flags().Changed("endpoint") {
			if v := os.Getenv("OPCUA_ENDPOINT"); v != "" {
				_ = cmd.Flags().Set("endpoint", v)
			}
		}
		if !cmd.Flags().Changed("username") {
			if v := os.Getenv("OPCUA_USERNAME"); v != "" {
				_ = cmd.Flags().Set("username", v)
			}
		}
		if !cmd.Flags().Changed("password") {
			if v := os.Getenv("OPCUA_PASSWORD"); v != "" {
				_ = cmd.Flags().Set("password", v)
			}
		}
		if !cmd.Flags().Changed("cert") {
			if v := os.Getenv("OPCUA_CERT"); v != "" {
				_ = cmd.Flags().Set("cert", v)
			}
		}
		if !cmd.Flags().Changed("key") {
			if v := os.Getenv("OPCUA_KEY"); v != "" {
				_ = cmd.Flags().Set("key", v)
			}
		}
		if !cmd.Flags().Changed("timeout") {
			if v := os.Getenv("OPCUA_TIMEOUT"); v != "" {
				_ = cmd.Flags().Set("timeout", v)
			}
		}
		if !cmd.Flags().Changed("no-color") {
			if v := os.Getenv("NO_COLOR"); v != "" {
				_ = cmd.Flags().Set("no-color", v)
			}
		}
		rawConfig, _ := cmd.Flags().GetString("config")
		rawProfile, _ := cmd.Flags().GetString("profile")
		rawEndpoint, _ := cmd.Flags().GetStringArray("endpoint")
		rawUsername, _ := cmd.Flags().GetString("username")
		rawPassword, _ := cmd.Flags().GetString("password")
		rawPasswordFile, _ := cmd.Flags().GetString("password-file")
		rawAuth, _ := cmd.Flags().GetString("auth")
		rawPolicy, _ := cmd.Flags().GetString("policy")
		rawMode, _ := cmd.Flags().GetString("mode")
		rawCert, _ := cmd.Flags().GetString("cert")
		rawKey, _ := cmd.Flags().GetString("key")
		rawInsecure, _ := cmd.Flags().GetBool("insecure")
		rawTimeout, _ := cmd.Flags().GetDuration("timeout")
		rawDialTimeout, _ := cmd.Flags().GetDuration("dial-timeout")
		rawSessionTimeout, _ := cmd.Flags().GetDuration("session-timeout")
		rawNoReconnect, _ := cmd.Flags().GetBool("no-reconnect")
		rawAppUri, _ := cmd.Flags().GetString("app-uri")
		rawLocale, _ := cmd.Flags().GetStringArray("locale")
		rawVerbose, _ := cmd.Flags().GetCount("verbose")
		rawTrace, _ := cmd.Flags().GetBool("trace")
		rawNoColor, _ := cmd.Flags().GetBool("no-color")
		var format ServerDiagnosticsFormat
		if rawFormat != "" {
			switch rawFormat {
			case "text":
				format = ServerDiagnosticsFormatText
			case "markdown":
				format = ServerDiagnosticsFormatMarkdown
			case "json":
				format = ServerDiagnosticsFormatJson
			case "yaml":
				format = ServerDiagnosticsFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of text, markdown, json, yaml", rawFormat)
			}
		}

		p := ServerDiagnosticsParams{
			Config:         rawConfig,
			Profile:        rawProfile,
			Endpoint:       rawEndpoint,
			Username:       rawUsername,
			Password:       rawPassword,
			PasswordFile:   rawPasswordFile,
			Auth:           rawAuth,
			Policy:         rawPolicy,
			Mode:           rawMode,
			Cert:           rawCert,
			Key:            rawKey,
			Insecure:       rawInsecure,
			Timeout:        rawTimeout,
			DialTimeout:    rawDialTimeout,
			SessionTimeout: rawSessionTimeout,
			NoReconnect:    rawNoReconnect,
			AppUri:         rawAppUri,
			Locale:         rawLocale,
			Verbose:        rawVerbose,
			Trace:          rawTrace,
			NoColor:        rawNoColor,
			Sessions:       rawSessions,
			Format:         format,
		}

		result, err := serverDiagnostics(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ServerDiagnosticsFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ServerDiagnosticsFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return cmd
}

type ServerRedundancyFormat string

const (
	ServerRedundancyFormatText     ServerRedundancyFormat = "text"
	ServerRedundancyFormatMarkdown ServerRedundancyFormat = "markdown"
	ServerRedundancyFormatJson     ServerRedundancyFormat = "json"
	ServerRedundancyFormatYaml     ServerRedundancyFormat = "yaml"
)

type ServerRedundancyParams struct {
	Config         string
	Profile        string
	Endpoint       []string
	Username       string
	Password       string
	PasswordFile   string
	Auth           string
	Policy         string
	Mode           string
	Cert           string
	Key            string
	Insecure       bool
	Timeout        time.Duration
	DialTimeout    time.Duration
	SessionTimeout time.Duration
	NoReconnect    bool
	AppUri         string
	Locale         []string
	Verbose        int
	Trace          bool
	NoColor        bool
	Format         ServerRedundancyFormat
}
type ServerRedundancyHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ServerRedundancyParams) (Redundancy, error)

func newServerRedundancyCommand(serverRedundancy ServerRedundancyHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "redundancy",
		Short:   "Show redundancy support and the failover server set.",
		Long:    "Reads Server/ServerRedundancy: which redundancy mode the server\nsupports, the URIs of the other members of the set, and the current\nservice level.\n\nA cold or warm pair expects a client to fail over on its own, which\nis what a profile's endpoint list is for: `--endpoint` may be\nrepeated, and every command tries them in order.\n",
		Aliases: []string{"ha"},
		Args:    rangeArgs(0, 0),
	}

	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "text", "Output format: text, markdown, json, yaml.")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("config") {
			if v := os.Getenv("OPCUA_CONFIG"); v != "" {
				_ = cmd.Flags().Set("config", v)
			}
		}
		if !cmd.Flags().Changed("profile") {
			if v := os.Getenv("OPCUA_PROFILE"); v != "" {
				_ = cmd.Flags().Set("profile", v)
			}
		}
		if !cmd.Flags().Changed("endpoint") {
			if v := os.Getenv("OPCUA_ENDPOINT"); v != "" {
				_ = cmd.Flags().Set("endpoint", v)
			}
		}
		if !cmd.Flags().Changed("username") {
			if v := os.Getenv("OPCUA_USERNAME"); v != "" {
				_ = cmd.Flags().Set("username", v)
			}
		}
		if !cmd.Flags().Changed("password") {
			if v := os.Getenv("OPCUA_PASSWORD"); v != "" {
				_ = cmd.Flags().Set("password", v)
			}
		}
		if !cmd.Flags().Changed("cert") {
			if v := os.Getenv("OPCUA_CERT"); v != "" {
				_ = cmd.Flags().Set("cert", v)
			}
		}
		if !cmd.Flags().Changed("key") {
			if v := os.Getenv("OPCUA_KEY"); v != "" {
				_ = cmd.Flags().Set("key", v)
			}
		}
		if !cmd.Flags().Changed("timeout") {
			if v := os.Getenv("OPCUA_TIMEOUT"); v != "" {
				_ = cmd.Flags().Set("timeout", v)
			}
		}
		if !cmd.Flags().Changed("no-color") {
			if v := os.Getenv("NO_COLOR"); v != "" {
				_ = cmd.Flags().Set("no-color", v)
			}
		}
		rawConfig, _ := cmd.Flags().GetString("config")
		rawProfile, _ := cmd.Flags().GetString("profile")
		rawEndpoint, _ := cmd.Flags().GetStringArray("endpoint")
		rawUsername, _ := cmd.Flags().GetString("username")
		rawPassword, _ := cmd.Flags().GetString("password")
		rawPasswordFile, _ := cmd.Flags().GetString("password-file")
		rawAuth, _ := cmd.Flags().GetString("auth")
		rawPolicy, _ := cmd.Flags().GetString("policy")
		rawMode, _ := cmd.Flags().GetString("mode")
		rawCert, _ := cmd.Flags().GetString("cert")
		rawKey, _ := cmd.Flags().GetString("key")
		rawInsecure, _ := cmd.Flags().GetBool("insecure")
		rawTimeout, _ := cmd.Flags().GetDuration("timeout")
		rawDialTimeout, _ := cmd.Flags().GetDuration("dial-timeout")
		rawSessionTimeout, _ := cmd.Flags().GetDuration("session-timeout")
		rawNoReconnect, _ := cmd.Flags().GetBool("no-reconnect")
		rawAppUri, _ := cmd.Flags().GetString("app-uri")
		rawLocale, _ := cmd.Flags().GetStringArray("locale")
		rawVerbose, _ := cmd.Flags().GetCount("verbose")
		rawTrace, _ := cmd.Flags().GetBool("trace")
		rawNoColor, _ := cmd.Flags().GetBool("no-color")
		var format ServerRedundancyFormat
		if rawFormat != "" {
			switch rawFormat {
			case "text":
				format = ServerRedundancyFormatText
			case "markdown":
				format = ServerRedundancyFormatMarkdown
			case "json":
				format = ServerRedundancyFormatJson
			case "yaml":
				format = ServerRedundancyFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of text, markdown, json, yaml", rawFormat)
			}
		}

		p := ServerRedundancyParams{
			Config:         rawConfig,
			Profile:        rawProfile,
			Endpoint:       rawEndpoint,
			Username:       rawUsername,
			Password:       rawPassword,
			PasswordFile:   rawPasswordFile,
			Auth:           rawAuth,
			Policy:         rawPolicy,
			Mode:           rawMode,
			Cert:           rawCert,
			Key:            rawKey,
			Insecure:       rawInsecure,
			Timeout:        rawTimeout,
			DialTimeout:    rawDialTimeout,
			SessionTimeout: rawSessionTimeout,
			NoReconnect:    rawNoReconnect,
			AppUri:         rawAppUri,
			Locale:         rawLocale,
			Verbose:        rawVerbose,
			Trace:          rawTrace,
			NoColor:        rawNoColor,
			Format:         format,
		}

		result, err := serverRedundancy(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ServerRedundancyFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ServerRedundancyFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return cmd
}
func NewServerCommand(
	serverInfo ServerInfoHandler,
	serverCapabilities ServerCapabilitiesHandler,
	serverDiagnostics ServerDiagnosticsHandler,
	serverRedundancy ServerRedundancyHandler,
) ServerCommand {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Inspect a server's status, capabilities and diagnostics.",
	}
	cmd.AddCommand(
		newServerInfoCommand(serverInfo),
		newServerCapabilitiesCommand(serverCapabilities),
		newServerDiagnosticsCommand(serverDiagnostics),
		newServerRedundancyCommand(serverRedundancy),
	)
	return ServerCommand(cmd)
}

type ServerCommand *cobra.Command
