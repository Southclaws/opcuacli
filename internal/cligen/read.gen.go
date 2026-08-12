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

type ReadTimestamps string

const (
	ReadTimestampsSource  ReadTimestamps = "source"
	ReadTimestampsServer  ReadTimestamps = "server"
	ReadTimestampsBoth    ReadTimestamps = "both"
	ReadTimestampsNeither ReadTimestamps = "neither"
)

type ReadFormat string

const (
	ReadFormatTable ReadFormat = "table"
	ReadFormatPlain ReadFormat = "plain"
	ReadFormatJson  ReadFormat = "json"
	ReadFormatYaml  ReadFormat = "yaml"
	ReadFormatJsonl ReadFormat = "jsonl"
)

type ReadParams struct {
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
	Attribute      []string
	IndexRange     string
	MaxAge         time.Duration
	Timestamps     ReadTimestamps
	Stdin          bool
	Format         ReadFormat
	Node           []string
}
type ReadHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ReadParams) (ReadResultList, error)

func NewReadCommand(read ReadHandler) ReadCommand {
	cmd := &cobra.Command{
		Use:     "read <node-id>...",
		Short:   "Read attributes of one or more nodes.",
		Long:    "Reads the Value attribute of every node given, in a single Read service\ncall. Pass `--attribute` to read something else, repeating it for\nseveral attributes.\n\nEvery result carries its own status code, so a partial failure reports\nper node rather than failing the command. `--index-range` reads a slice\nof an array value, e.g. `0:9` or `2` or `1:3,0:2` for a matrix.\n",
		Aliases: []string{"get"},
		Example: `  # Read one value.
  opcua read 'ns=2;s=Temperature'
  # Read the server time and state as JSON.
  opcua read i=2258 i=2259 --format json
  # Read several attributes of one node.
  opcua read 'ns=3;i=1001' --attribute Value,DataType,AccessLevel
  # Read every node that a search turned up.
  opcua find sensor --format plain | opcua read --stdin`,
		Args: rangeArgs(1, -1),
	}

	var rawAttribute []string
	cmd.Flags().StringSliceVarP(&rawAttribute, "attribute", "a", nil, "Attribute to read; repeatable. Defaults to Value.")
	var rawIndexRange string
	cmd.Flags().StringVar(&rawIndexRange, "index-range", "", "Range of an array value to read, e.g. 0:9.")
	var rawMaxAge time.Duration
	cmd.Flags().DurationVar(&rawMaxAge, "max-age", mustParseDuration("0s"), "Accept a cached value up to this old.")
	var rawTimestamps string
	cmd.Flags().StringVar(&rawTimestamps, "timestamps", "both", "Timestamps to return: source, server, both, neither.")
	var rawStdin bool
	cmd.Flags().BoolVar(&rawStdin, "stdin", false, "Read additional node ids from stdin, one per line.")
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
		var timestamps ReadTimestamps
		if rawTimestamps != "" {
			switch rawTimestamps {
			case "source":
				timestamps = ReadTimestampsSource
			case "server":
				timestamps = ReadTimestampsServer
			case "both":
				timestamps = ReadTimestampsBoth
			case "neither":
				timestamps = ReadTimestampsNeither
			default:
				return fmt.Errorf("invalid --timestamps %q: must be one of source, server, both, neither", rawTimestamps)
			}
		}
		var format ReadFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = ReadFormatTable
			case "plain":
				format = ReadFormatPlain
			case "json":
				format = ReadFormatJson
			case "yaml":
				format = ReadFormatYaml
			case "jsonl":
				format = ReadFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml, jsonl", rawFormat)
			}
		}
		node := args[0:]

		p := ReadParams{
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
			Attribute:      rawAttribute,
			IndexRange:     rawIndexRange,
			MaxAge:         rawMaxAge,
			Timestamps:     timestamps,
			Stdin:          rawStdin,
			Format:         format,
			Node:           node,
		}

		result, err := read(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ReadFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ReadFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return ReadCommand(cmd)
}

type ReadCommand *cobra.Command
