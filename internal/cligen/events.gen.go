package cligen

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type EventsFormat string

const (
	EventsFormatLive  EventsFormat = "live"
	EventsFormatPlain EventsFormat = "plain"
	EventsFormatCsv   EventsFormat = "csv"
	EventsFormatJsonl EventsFormat = "jsonl"
)

type EventsParams struct {
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
	Field          []string
	EventType      string
	SeverityMin    int
	Interval       time.Duration
	QueueSize      int
	Duration       time.Duration
	Count          int
	Format         EventsFormat
	Node           string
}
type EventsHandler func(ctx context.Context, cmd *cobra.Command, io IO, p EventsParams) error

func NewEventsCommand(events EventsHandler) EventsCommand {
	cmd := &cobra.Command{
		Use:     "events [node-id]",
		Short:   "Stream events and alarms from a notifier node.",
		Long:    "Subscribes to the EventNotifier attribute of a node and prints each\nevent as it arrives.\n\n`--field` chooses the event fields to select, as browse paths relative\nto the event type; the defaults cover the base event type.\n`--severity-min` installs a server-side where clause so the server only\nsends events at or above that severity.\n",
		Aliases: []string{"alarms"},
		Example: `  # Watch the server's own event stream.
  opcua events
  # Stream significant events as JSONL.
  opcua events --severity-min 500 --format jsonl`,
		Args: rangeArgs(0, 1),
	}

	var rawField []string
	cmd.Flags().StringSliceVar(&rawField, "field", nil, "Event field to select; repeatable.")
	var rawEventType string
	cmd.Flags().StringVar(&rawEventType, "event-type", "i=2041", "Only report events of this event type.")
	var rawSeverityMin int
	cmd.Flags().IntVar(&rawSeverityMin, "severity-min", 0, "Only report events at or above this severity, 1 to 1000.")
	var rawInterval time.Duration
	cmd.Flags().DurationVarP(&rawInterval, "interval", "i", mustParseDuration("1s"), "Publishing interval of the subscription.")
	var rawQueueSize int
	cmd.Flags().IntVar(&rawQueueSize, "queue-size", 100, "Server-side queue depth for the notifier.")
	var rawDuration time.Duration
	cmd.Flags().DurationVar(&rawDuration, "duration", mustParseDuration("0s"), "Stop after this long (0 = until interrupted).")
	var rawCount int
	cmd.Flags().IntVarP(&rawCount, "count", "c", 0, "Stop after this many events (0 = no limit).")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "live", "Output format: live, plain, csv, jsonl.")

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
		var format EventsFormat
		if rawFormat != "" {
			switch rawFormat {
			case "live":
				format = EventsFormatLive
			case "plain":
				format = EventsFormatPlain
			case "csv":
				format = EventsFormatCsv
			case "jsonl":
				format = EventsFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of live, plain, csv, jsonl", rawFormat)
			}
		}
		rawNode := "i=2253"
		if len(args) > 0 {
			rawNode = args[0]
		}

		p := EventsParams{
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
			Field:          rawField,
			EventType:      rawEventType,
			SeverityMin:    rawSeverityMin,
			Interval:       rawInterval,
			QueueSize:      rawQueueSize,
			Duration:       rawDuration,
			Count:          rawCount,
			Format:         format,
			Node:           rawNode,
		}

		return events(cmd.Context(), cmd, newIO(cmd), p)
	}

	return EventsCommand(cmd)
}

type EventsCommand *cobra.Command
