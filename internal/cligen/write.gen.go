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

type WriteFormat string

const (
	WriteFormatPlain WriteFormat = "plain"
	WriteFormatJson  WriteFormat = "json"
	WriteFormatYaml  WriteFormat = "yaml"
)

type WriteParams struct {
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
	Type           string
	Attribute      string
	IndexRange     string
	Array          bool
	Json           bool
	Yes            bool
	DryRun         bool
	Format         WriteFormat
	Node           string
	Value          string
}
type WriteHandler func(ctx context.Context, cmd *cobra.Command, io IO, p WriteParams) (WriteResult, error)

func NewWriteCommand(write WriteHandler) WriteCommand {
	cmd := &cobra.Command{
		Use:     "write <node-id> <value>",
		Short:   "Write a value to a node.",
		Long:    "Writes VALUE to the Value attribute of NODE.\n\nThe OPC UA type of the value matters: a server rejects a write whose\nvariant type does not match the node's DataType. By default the node's\nDataType attribute is read first and VALUE is parsed into that type, so\n`opcua write 'ns=2;s=Setpoint' 21.5` does the right thing. Pass `--type`\nto force a type, `--array` for a comma-separated list, or `--json` to\nsupply the value as JSON.\n",
		Aliases: []string{"set"},
		Example: `  # Write a value, inferring its type from the node.
  opcua write 'ns=2;s=Setpoint' 21.5
  # Force the written type.
  opcua write 'ns=2;s=Enabled' true --type boolean
  # Write an array of Int32.
  opcua write 'ns=2;s=Window' 1,2,3 --array --type int32
  # Check a write without performing it.
  opcua write 'ns=2;s=Setpoint' 30 --dry-run`,
		Args: rangeArgs(2, 2),
	}

	var rawType string
	cmd.Flags().StringVarP(&rawType, "type", "t", "auto", "OPC UA type to write, or auto to use the node's DataType.")
	var rawAttribute string
	cmd.Flags().StringVarP(&rawAttribute, "attribute", "a", "Value", "Attribute to write.")
	var rawIndexRange string
	cmd.Flags().StringVar(&rawIndexRange, "index-range", "", "Range of the array to overwrite, e.g. 0:9.")
	var rawArray bool
	cmd.Flags().BoolVar(&rawArray, "array", false, "Parse VALUE as a comma-separated array.")
	var rawJson bool
	cmd.Flags().BoolVar(&rawJson, "json", false, "Parse VALUE as JSON.")
	var rawYes bool
	cmd.Flags().BoolVarP(&rawYes, "yes", "y", false, "Skip the confirmation prompt.")
	var rawDryRun bool
	cmd.Flags().BoolVarP(&rawDryRun, "dry-run", "n", false, "Show what would be written without writing it.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "plain", "Output format: plain, json, yaml.")
	cmd.MarkFlagsMutuallyExclusive("array", "json")

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
		var format WriteFormat
		if rawFormat != "" {
			switch rawFormat {
			case "plain":
				format = WriteFormatPlain
			case "json":
				format = WriteFormatJson
			case "yaml":
				format = WriteFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of plain, json, yaml", rawFormat)
			}
		}
		rawNode := args[0]
		rawValue := args[1]

		p := WriteParams{
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
			Type:           rawType,
			Attribute:      rawAttribute,
			IndexRange:     rawIndexRange,
			Array:          rawArray,
			Json:           rawJson,
			Yes:            rawYes,
			DryRun:         rawDryRun,
			Format:         format,
			Node:           rawNode,
			Value:          rawValue,
		}

		result, err := write(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case WriteFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case WriteFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return WriteCommand(cmd)
}

type WriteCommand *cobra.Command
