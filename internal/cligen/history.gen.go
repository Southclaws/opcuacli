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

type HistoryReadFormat string

const (
	HistoryReadFormatTable HistoryReadFormat = "table"
	HistoryReadFormatPlain HistoryReadFormat = "plain"
	HistoryReadFormatJson  HistoryReadFormat = "json"
	HistoryReadFormatYaml  HistoryReadFormat = "yaml"
	HistoryReadFormatJsonl HistoryReadFormat = "jsonl"
)

type HistoryReadParams struct {
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
	Last           time.Duration
	Start          string
	End            string
	Limit          int
	Bounds         bool
	Modified       bool
	Format         HistoryReadFormat
	Node           string
}
type HistoryReadHandler func(ctx context.Context, cmd *cobra.Command, io IO, p HistoryReadParams) (HistoryResult, error)

func newHistoryReadCommand(historyRead HistoryReadHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "read <node-id>",
		Short:   "Read archived values of a node over a time range.",
		Long:    "Calls HistoryRead with ReadRawModifiedDetails and follows\ncontinuation points until the range is covered or `--limit` is hit.\n\nTimes accept RFC 3339 (`2024-01-01T00:00:00Z`), a date\n(`2024-01-01`), or an offset from now (`-2h`, `-30m`). `--last` is\nshorthand for a range ending now.\n",
		Aliases: []string{"values"},
		Example: `  # A day of archived values.
  opcua history read 'ns=2;s=Temperature' --last 24h
  # Stream a week of values as JSONL.
  opcua history read 'ns=2;s=Temperature' --start -7d --format jsonl`,
		Args: rangeArgs(1, 1),
	}

	var rawLast time.Duration
	cmd.Flags().DurationVar(&rawLast, "last", mustParseDuration("1h"), "Read the values of the last interval, ending now.")
	var rawStart string
	cmd.Flags().StringVar(&rawStart, "start", "", "Start of the range; overrides --last.")
	var rawEnd string
	cmd.Flags().StringVar(&rawEnd, "end", "", "End of the range; defaults to now.")
	var rawLimit int
	cmd.Flags().IntVar(&rawLimit, "limit", 1000, "Stop after this many values (0 = no limit).")
	var rawBounds bool
	cmd.Flags().BoolVar(&rawBounds, "bounds", false, "Include the bounding values either side of the range.")
	var rawModified bool
	cmd.Flags().BoolVar(&rawModified, "modified", false, "Read modified values instead of raw ones.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "table", "Output format: table, plain, json, yaml, jsonl.")

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
		var format HistoryReadFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = HistoryReadFormatTable
			case "plain":
				format = HistoryReadFormatPlain
			case "json":
				format = HistoryReadFormatJson
			case "yaml":
				format = HistoryReadFormatYaml
			case "jsonl":
				format = HistoryReadFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml, jsonl", rawFormat)
			}
		}
		rawNode := args[0]

		p := HistoryReadParams{
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
			Last:           rawLast,
			Start:          rawStart,
			End:            rawEnd,
			Limit:          rawLimit,
			Bounds:         rawBounds,
			Modified:       rawModified,
			Format:         format,
			Node:           rawNode,
		}

		result, err := historyRead(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case HistoryReadFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case HistoryReadFormatYaml:
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

type HistoryAtFormat string

const (
	HistoryAtFormatTable HistoryAtFormat = "table"
	HistoryAtFormatPlain HistoryAtFormat = "plain"
	HistoryAtFormatJson  HistoryAtFormat = "json"
	HistoryAtFormatYaml  HistoryAtFormat = "yaml"
	HistoryAtFormatJsonl HistoryAtFormat = "jsonl"
)

type HistoryAtParams struct {
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
	Time            []string
	UseSimpleBounds bool
	Format          HistoryAtFormat
	Node            string
}
type HistoryAtHandler func(ctx context.Context, cmd *cobra.Command, io IO, p HistoryAtParams) (HistoryResult, error)

func newHistoryAtCommand(historyAt HistoryAtHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "at <node-id>",
		Short: "Read the value of a node at specific points in time.",
		Long: `Calls HistoryRead with ReadAtTimeDetails, which returns one value
per requested timestamp, interpolated or stepped by the server.
`,
		Args: rangeArgs(1, 1),
	}

	var rawTime []string
	cmd.Flags().StringArrayVar(&rawTime, "time", nil, "A timestamp to read at; repeatable.")
	var rawUseSimpleBounds bool
	cmd.Flags().BoolVar(&rawUseSimpleBounds, "use-simple-bounds", false, "Use simple bounding values instead of interpolating.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "table", "Output format: table, plain, json, yaml, jsonl.")
	_ = cmd.MarkFlagRequired("time")

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
		var format HistoryAtFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = HistoryAtFormatTable
			case "plain":
				format = HistoryAtFormatPlain
			case "json":
				format = HistoryAtFormatJson
			case "yaml":
				format = HistoryAtFormatYaml
			case "jsonl":
				format = HistoryAtFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml, jsonl", rawFormat)
			}
		}
		rawNode := args[0]

		p := HistoryAtParams{
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
			Time:            rawTime,
			UseSimpleBounds: rawUseSimpleBounds,
			Format:          format,
			Node:            rawNode,
		}

		result, err := historyAt(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case HistoryAtFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case HistoryAtFormatYaml:
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

type HistoryEventsFormat string

const (
	HistoryEventsFormatTable HistoryEventsFormat = "table"
	HistoryEventsFormatPlain HistoryEventsFormat = "plain"
	HistoryEventsFormatJson  HistoryEventsFormat = "json"
	HistoryEventsFormatYaml  HistoryEventsFormat = "yaml"
	HistoryEventsFormatJsonl HistoryEventsFormat = "jsonl"
)

type HistoryEventsParams struct {
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
	Last           time.Duration
	Start          string
	End            string
	Field          []string
	EventType      string
	Limit          int
	Format         HistoryEventsFormat
	Node           string
}
type HistoryEventsHandler func(ctx context.Context, cmd *cobra.Command, io IO, p HistoryEventsParams) (EventList, error)

func newHistoryEventsCommand(historyEvents HistoryEventsHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "events [node-id]",
		Short: "Read archived events of a notifier node.",
		Long:  "Calls HistoryRead with ReadEventDetails against a node that is an\nevent notifier, such as the Server object.\n\n`--field` selects which event fields to retrieve, as browse paths\nrelative to the event type.\n",
		Args:  rangeArgs(0, 1),
	}

	var rawLast time.Duration
	cmd.Flags().DurationVar(&rawLast, "last", mustParseDuration("1h"), "Read events from the last interval, ending now.")
	var rawStart string
	cmd.Flags().StringVar(&rawStart, "start", "", "Start of the range; overrides --last.")
	var rawEnd string
	cmd.Flags().StringVar(&rawEnd, "end", "", "End of the range; defaults to now.")
	var rawField []string
	cmd.Flags().StringSliceVar(&rawField, "field", nil, "Event field to retrieve; repeatable.")
	var rawEventType string
	cmd.Flags().StringVar(&rawEventType, "event-type", "i=2041", "Only retrieve events of this event type.")
	var rawLimit int
	cmd.Flags().IntVar(&rawLimit, "limit", 100, "Stop after this many events (0 = no limit).")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "table", "Output format: table, plain, json, yaml, jsonl.")

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
		var format HistoryEventsFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = HistoryEventsFormatTable
			case "plain":
				format = HistoryEventsFormatPlain
			case "json":
				format = HistoryEventsFormatJson
			case "yaml":
				format = HistoryEventsFormatYaml
			case "jsonl":
				format = HistoryEventsFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml, jsonl", rawFormat)
			}
		}
		rawNode := "i=2253"
		if len(args) > 0 {
			rawNode = args[0]
		}

		p := HistoryEventsParams{
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
			Last:           rawLast,
			Start:          rawStart,
			End:            rawEnd,
			Field:          rawField,
			EventType:      rawEventType,
			Limit:          rawLimit,
			Format:         format,
			Node:           rawNode,
		}

		result, err := historyEvents(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case HistoryEventsFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case HistoryEventsFormatYaml:
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
func NewHistoryCommand(
	historyRead HistoryReadHandler,
	historyAt HistoryAtHandler,
	historyEvents HistoryEventsHandler,
) HistoryCommand {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Read historical values and events.",
	}
	cmd.AddCommand(
		newHistoryReadCommand(historyRead),
		newHistoryAtCommand(historyAt),
		newHistoryEventsCommand(historyEvents),
	)
	return HistoryCommand(cmd)
}

type HistoryCommand *cobra.Command
