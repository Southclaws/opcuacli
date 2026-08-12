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

type DiscoverEndpointsFormat string

const (
	DiscoverEndpointsFormatTable DiscoverEndpointsFormat = "table"
	DiscoverEndpointsFormatPlain DiscoverEndpointsFormat = "plain"
	DiscoverEndpointsFormatJson  DiscoverEndpointsFormat = "json"
	DiscoverEndpointsFormatYaml  DiscoverEndpointsFormat = "yaml"
)

type DiscoverEndpointsParams struct {
	Config          string
	Profile         string
	Endpoint        []string
	Username        string
	Password        string
	PasswordFile    string
	Auth            string
	Policy          string
	Mode            string
	Cert            string
	Key             string
	Insecure        bool
	Timeout         time.Duration
	DialTimeout     time.Duration
	SessionTimeout  time.Duration
	NoReconnect     bool
	AppUri          string
	Locale          []string
	Verbose         int
	Trace           bool
	NoColor         bool
	ShowCertificate bool
	Transport       string
	Format          DiscoverEndpointsFormat
	DiscoveryUrl    string
}
type DiscoverEndpointsHandler func(ctx context.Context, cmd *cobra.Command, io IO, p DiscoverEndpointsParams) (EndpointList, error)

func newDiscoverEndpointsCommand(discoverEndpoints DiscoverEndpointsHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "endpoints [endpoint]",
		Short:   "List the endpoints an OPC UA server offers.",
		Long:    "Calls GetEndpoints on the discovery URL and reports every endpoint\nwith its security policy, message security mode and accepted user\ntoken types.\n\nThis is an unauthenticated call: it needs no session, so it is the\nfirst thing to try when a connection is refused. The `SEC` column\nshows the policy short name and the mode, which is exactly what\n`--policy` and `--mode` expect.\n",
		Aliases: []string{"ep"},
		Example: `  # List every endpoint of a local server.
  opcua discover endpoints opc.tcp://localhost:4840
  # Machine-readable endpoints for the active profile.
  opcua discover endpoints --format json`,
		Args: rangeArgs(0, 1),
	}

	var rawShowCertificate bool
	cmd.Flags().BoolVar(&rawShowCertificate, "show-certificate", false, "Include the server certificate of each endpoint.")
	var rawTransport string
	cmd.Flags().StringVar(&rawTransport, "transport", "", "Only report endpoints using this transport profile.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "table", "Output format: table, plain, json, yaml.")

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
		var format DiscoverEndpointsFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = DiscoverEndpointsFormatTable
			case "plain":
				format = DiscoverEndpointsFormatPlain
			case "json":
				format = DiscoverEndpointsFormatJson
			case "yaml":
				format = DiscoverEndpointsFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml", rawFormat)
			}
		}
		rawDiscoveryUrl := ""
		if len(args) > 0 {
			rawDiscoveryUrl = args[0]
		}

		p := DiscoverEndpointsParams{
			Config:          rawConfig,
			Profile:         rawProfile,
			Endpoint:        rawEndpoint,
			Username:        rawUsername,
			Password:        rawPassword,
			PasswordFile:    rawPasswordFile,
			Auth:            rawAuth,
			Policy:          rawPolicy,
			Mode:            rawMode,
			Cert:            rawCert,
			Key:             rawKey,
			Insecure:        rawInsecure,
			Timeout:         rawTimeout,
			DialTimeout:     rawDialTimeout,
			SessionTimeout:  rawSessionTimeout,
			NoReconnect:     rawNoReconnect,
			AppUri:          rawAppUri,
			Locale:          rawLocale,
			Verbose:         rawVerbose,
			Trace:           rawTrace,
			NoColor:         rawNoColor,
			ShowCertificate: rawShowCertificate,
			Transport:       rawTransport,
			Format:          format,
			DiscoveryUrl:    rawDiscoveryUrl,
		}

		result, err := discoverEndpoints(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case DiscoverEndpointsFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case DiscoverEndpointsFormatYaml:
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

type DiscoverServersFormat string

const (
	DiscoverServersFormatTable DiscoverServersFormat = "table"
	DiscoverServersFormatPlain DiscoverServersFormat = "plain"
	DiscoverServersFormatJson  DiscoverServersFormat = "json"
	DiscoverServersFormatYaml  DiscoverServersFormat = "yaml"
)

type DiscoverServersParams struct {
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
	Uri            []string
	Format         DiscoverServersFormat
	DiscoveryUrl   string
}
type DiscoverServersHandler func(ctx context.Context, cmd *cobra.Command, io IO, p DiscoverServersParams) (ServerList, error)

func newDiscoverServersCommand(discoverServers DiscoverServersHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "servers [endpoint]",
		Short: "List the servers registered with a discovery endpoint.",
		Long: `Calls FindServers, which a local discovery server answers with every
server that has registered with it.
`,
		Args: rangeArgs(0, 1),
	}

	var rawUri []string
	cmd.Flags().StringArrayVar(&rawUri, "uri", nil, "Only report servers whose application URI matches; repeatable.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "table", "Output format: table, plain, json, yaml.")

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
		var format DiscoverServersFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = DiscoverServersFormatTable
			case "plain":
				format = DiscoverServersFormatPlain
			case "json":
				format = DiscoverServersFormatJson
			case "yaml":
				format = DiscoverServersFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml", rawFormat)
			}
		}
		rawDiscoveryUrl := ""
		if len(args) > 0 {
			rawDiscoveryUrl = args[0]
		}

		p := DiscoverServersParams{
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
			Uri:            rawUri,
			Format:         format,
			DiscoveryUrl:   rawDiscoveryUrl,
		}

		result, err := discoverServers(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case DiscoverServersFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case DiscoverServersFormatYaml:
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

type DiscoverNetworkFormat string

const (
	DiscoverNetworkFormatTable DiscoverNetworkFormat = "table"
	DiscoverNetworkFormatPlain DiscoverNetworkFormat = "plain"
	DiscoverNetworkFormatJson  DiscoverNetworkFormat = "json"
	DiscoverNetworkFormatYaml  DiscoverNetworkFormat = "yaml"
)

type DiscoverNetworkParams struct {
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
	StartRecord    int
	MaxRecords     int
	Capability     []string
	Format         DiscoverNetworkFormat
	DiscoveryUrl   string
}
type DiscoverNetworkHandler func(ctx context.Context, cmd *cobra.Command, io IO, p DiscoverNetworkParams) (NetworkServerList, error)

func newDiscoverNetworkCommand(discoverNetwork DiscoverNetworkHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network [endpoint]",
		Short: "List servers known to a discovery server via mDNS.",
		Long: `Calls FindServersOnNetwork. Only a discovery server that participates
in multicast announcement answers this; a plain server returns
BadServiceUnsupported.
`,
		Args: rangeArgs(0, 1),
	}

	var rawStartRecord int
	cmd.Flags().IntVar(&rawStartRecord, "start-record", 0, "Record counter to resume from.")
	var rawMaxRecords int
	cmd.Flags().IntVar(&rawMaxRecords, "max-records", 0, "Maximum records to return (0 = server's choice).")
	var rawCapability []string
	cmd.Flags().StringArrayVar(&rawCapability, "capability", nil, "Only report servers with this server capability; repeatable.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "table", "Output format: table, plain, json, yaml.")

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
		var format DiscoverNetworkFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = DiscoverNetworkFormatTable
			case "plain":
				format = DiscoverNetworkFormatPlain
			case "json":
				format = DiscoverNetworkFormatJson
			case "yaml":
				format = DiscoverNetworkFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml", rawFormat)
			}
		}
		rawDiscoveryUrl := ""
		if len(args) > 0 {
			rawDiscoveryUrl = args[0]
		}

		p := DiscoverNetworkParams{
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
			StartRecord:    rawStartRecord,
			MaxRecords:     rawMaxRecords,
			Capability:     rawCapability,
			Format:         format,
			DiscoveryUrl:   rawDiscoveryUrl,
		}

		result, err := discoverNetwork(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case DiscoverNetworkFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case DiscoverNetworkFormatYaml:
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
func NewDiscoverCommand(
	discoverEndpoints DiscoverEndpointsHandler,
	discoverServers DiscoverServersHandler,
	discoverNetwork DiscoverNetworkHandler,
) DiscoverCommand {
	cmd := &cobra.Command{
		Use:     "discover",
		Short:   "Find servers and inspect the endpoints they offer.",
		Aliases: []string{"disco"},
	}
	cmd.AddCommand(
		newDiscoverEndpointsCommand(discoverEndpoints),
		newDiscoverServersCommand(discoverServers),
		newDiscoverNetworkCommand(discoverNetwork),
	)
	return DiscoverCommand(cmd)
}

type DiscoverCommand *cobra.Command
