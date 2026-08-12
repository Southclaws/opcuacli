package cligen

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type MonitorTrigger string

const (
	MonitorTriggerStatus               MonitorTrigger = "status"
	MonitorTriggerStatusValue          MonitorTrigger = "status-value"
	MonitorTriggerStatusValueTimestamp MonitorTrigger = "status-value-timestamp"
)

type MonitorDeadband string

const (
	MonitorDeadbandNone     MonitorDeadband = "none"
	MonitorDeadbandAbsolute MonitorDeadband = "absolute"
	MonitorDeadbandPercent  MonitorDeadband = "percent"
)

type MonitorMonitoringMode string

const (
	MonitorMonitoringModeReporting MonitorMonitoringMode = "reporting"
	MonitorMonitoringModeSampling  MonitorMonitoringMode = "sampling"
	MonitorMonitoringModeDisabled  MonitorMonitoringMode = "disabled"
)

type MonitorTimestamps string

const (
	MonitorTimestampsSource  MonitorTimestamps = "source"
	MonitorTimestampsServer  MonitorTimestamps = "server"
	MonitorTimestampsBoth    MonitorTimestamps = "both"
	MonitorTimestampsNeither MonitorTimestamps = "neither"
)

type MonitorFormat string

const (
	MonitorFormatLive  MonitorFormat = "live"
	MonitorFormatPlain MonitorFormat = "plain"
	MonitorFormatCsv   MonitorFormat = "csv"
	MonitorFormatJsonl MonitorFormat = "jsonl"
)

type MonitorParams struct {
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
	Interval       time.Duration
	Sampling       time.Duration
	QueueSize      int
	DiscardOldest  bool
	Trigger        MonitorTrigger
	Deadband       MonitorDeadband
	DeadbandValue  float64
	MonitoringMode MonitorMonitoringMode
	Priority       int
	LifetimeCount  int
	KeepaliveCount int
	Timestamps     MonitorTimestamps
	Duration       time.Duration
	Count          int
	Stats          bool
	Stdin          bool
	Format         MonitorFormat
	Node           []string
}
type MonitorHandler func(ctx context.Context, cmd *cobra.Command, io IO, p MonitorParams) error

func NewMonitorCommand(monitor MonitorHandler) MonitorCommand {
	cmd := &cobra.Command{
		Use:     "monitor <node-id>...",
		Short:   "Stream value changes of nodes as they happen.",
		Long:    "Creates a subscription, monitors every node given, and prints each data\nchange as it arrives until interrupted.\n\n`--interval` is the publishing interval the server sends batches at;\n`--sampling` is how often it samples the node, and defaults to the\npublishing interval. A deadband suppresses small changes: `--deadband\nabsolute --deadband-value 0.5` reports only moves of half a unit or\nmore.\n\nThe default `live` renderer keeps one row per node, updating in place,\nwith a sparkline of recent numeric values. Use `--format jsonl` to pipe\nthe stream somewhere else.\n",
		Aliases: []string{"watch", "sub"},
		Example: `  # Watch two nodes in a live table.
  opcua monitor 'ns=2;s=Temperature' 'ns=2;s=Pressure'
  # Fast sampling, ignoring changes under 0.1.
  opcua monitor 'ns=2;s=Flow' --interval 100ms --deadband absolute --deadband-value 0.1
  # Stream changes as JSONL into a file.
  opcua monitor 'ns=2;s=Temperature' --format jsonl | tee log.jsonl
  # Monitor everything a search turned up.
  opcua find --class variable temperature --format plain | opcua monitor --stdin`,
		Args: rangeArgs(1, -1),
	}

	var rawInterval time.Duration
	cmd.Flags().DurationVarP(&rawInterval, "interval", "i", mustParseDuration("500ms"), "Publishing interval of the subscription.")
	var rawSampling time.Duration
	cmd.Flags().DurationVar(&rawSampling, "sampling", mustParseDuration("0s"), "Sampling interval per item; defaults to --interval.")
	var rawQueueSize int
	cmd.Flags().IntVar(&rawQueueSize, "queue-size", 10, "Server-side queue depth per monitored item.")
	var rawDiscardOldest bool
	cmd.Flags().BoolVar(&rawDiscardOldest, "discard-oldest", true, "Drop the oldest queued value when the queue is full.")
	var rawTrigger string
	cmd.Flags().StringVar(&rawTrigger, "trigger", "status-value", "What counts as a change: status, status-value, status-value-timestamp.")
	var rawDeadband string
	cmd.Flags().StringVar(&rawDeadband, "deadband", "none", "Deadband kind: none, absolute, percent.")
	var rawDeadbandValue float64
	cmd.Flags().Float64Var(&rawDeadbandValue, "deadband-value", 0, "Deadband size, in units or percent of range.")
	var rawMonitoringMode string
	cmd.Flags().StringVar(&rawMonitoringMode, "monitoring-mode", "reporting", "Monitoring mode: reporting, sampling, disabled.")
	var rawPriority int
	cmd.Flags().IntVar(&rawPriority, "priority", 0, "Subscription priority, 0 to 255.")
	var rawLifetimeCount int
	cmd.Flags().IntVar(&rawLifetimeCount, "lifetime-count", 0, "Publishing intervals before the subscription expires.")
	var rawKeepaliveCount int
	cmd.Flags().IntVar(&rawKeepaliveCount, "keepalive-count", 0, "Publishing intervals between keep-alive messages.")
	var rawTimestamps string
	cmd.Flags().StringVar(&rawTimestamps, "timestamps", "both", "Timestamps to return: source, server, both, neither.")
	var rawDuration time.Duration
	cmd.Flags().DurationVar(&rawDuration, "duration", mustParseDuration("0s"), "Stop after this long (0 = until interrupted).")
	var rawCount int
	cmd.Flags().IntVarP(&rawCount, "count", "c", 0, "Stop after this many notifications (0 = no limit).")
	var rawStats bool
	cmd.Flags().BoolVar(&rawStats, "stats", false, "Show delivered and dropped counters while streaming.")
	var rawStdin bool
	cmd.Flags().BoolVar(&rawStdin, "stdin", false, "Read additional node ids from stdin, one per line.")
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
		var trigger MonitorTrigger
		if rawTrigger != "" {
			switch rawTrigger {
			case "status":
				trigger = MonitorTriggerStatus
			case "status-value":
				trigger = MonitorTriggerStatusValue
			case "status-value-timestamp":
				trigger = MonitorTriggerStatusValueTimestamp
			default:
				return fmt.Errorf("invalid --trigger %q: must be one of status, status-value, status-value-timestamp", rawTrigger)
			}
		}
		var deadband MonitorDeadband
		if rawDeadband != "" {
			switch rawDeadband {
			case "none":
				deadband = MonitorDeadbandNone
			case "absolute":
				deadband = MonitorDeadbandAbsolute
			case "percent":
				deadband = MonitorDeadbandPercent
			default:
				return fmt.Errorf("invalid --deadband %q: must be one of none, absolute, percent", rawDeadband)
			}
		}
		var monitoringMode MonitorMonitoringMode
		if rawMonitoringMode != "" {
			switch rawMonitoringMode {
			case "reporting":
				monitoringMode = MonitorMonitoringModeReporting
			case "sampling":
				monitoringMode = MonitorMonitoringModeSampling
			case "disabled":
				monitoringMode = MonitorMonitoringModeDisabled
			default:
				return fmt.Errorf("invalid --monitoring-mode %q: must be one of reporting, sampling, disabled", rawMonitoringMode)
			}
		}
		var timestamps MonitorTimestamps
		if rawTimestamps != "" {
			switch rawTimestamps {
			case "source":
				timestamps = MonitorTimestampsSource
			case "server":
				timestamps = MonitorTimestampsServer
			case "both":
				timestamps = MonitorTimestampsBoth
			case "neither":
				timestamps = MonitorTimestampsNeither
			default:
				return fmt.Errorf("invalid --timestamps %q: must be one of source, server, both, neither", rawTimestamps)
			}
		}
		var format MonitorFormat
		if rawFormat != "" {
			switch rawFormat {
			case "live":
				format = MonitorFormatLive
			case "plain":
				format = MonitorFormatPlain
			case "csv":
				format = MonitorFormatCsv
			case "jsonl":
				format = MonitorFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of live, plain, csv, jsonl", rawFormat)
			}
		}
		node := args[0:]

		p := MonitorParams{
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
			Interval:       rawInterval,
			Sampling:       rawSampling,
			QueueSize:      rawQueueSize,
			DiscardOldest:  rawDiscardOldest,
			Trigger:        trigger,
			Deadband:       deadband,
			DeadbandValue:  rawDeadbandValue,
			MonitoringMode: monitoringMode,
			Priority:       rawPriority,
			LifetimeCount:  rawLifetimeCount,
			KeepaliveCount: rawKeepaliveCount,
			Timestamps:     timestamps,
			Duration:       rawDuration,
			Count:          rawCount,
			Stats:          rawStats,
			Stdin:          rawStdin,
			Format:         format,
			Node:           node,
		}

		return monitor(cmd.Context(), cmd, newIO(cmd), p)
	}

	return MonitorCommand(cmd)
}

type MonitorCommand *cobra.Command
