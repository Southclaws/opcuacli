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

type AttributesFormat string

const (
	AttributesFormatTable AttributesFormat = "table"
	AttributesFormatPlain AttributesFormat = "plain"
	AttributesFormatJson  AttributesFormat = "json"
	AttributesFormatYaml  AttributesFormat = "yaml"
)

type AttributesParams struct {
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
	All            bool
	Format         AttributesFormat
	Node           string
}
type AttributesHandler func(ctx context.Context, cmd *cobra.Command, io IO, p AttributesParams) (AttributeSet, error)

func NewAttributesCommand(attributes AttributesHandler) AttributesCommand {
	cmd := &cobra.Command{
		Use:     "attributes <node-id>",
		Short:   "Show every attribute of a node.",
		Long:    "Reads all attributes defined for the node's class in one Read call and\nreports the ones the server answers. Attributes that do not apply come\nback as BadAttributeIdInvalid and are hidden unless `--all` is given.\n\nThis is the fastest way to understand an unfamiliar node: its class,\ndata type, value rank, access level, historizing flag and more.\n",
		Aliases: []string{"attrs", "describe"},
		Example: `  # Describe a variable node.
  opcua attributes 'ns=2;s=Temperature'`,
		Args: rangeArgs(1, 1),
	}

	var rawAll bool
	cmd.Flags().BoolVar(&rawAll, "all", false, "Include attributes the node does not support.")
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
		var format AttributesFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = AttributesFormatTable
			case "plain":
				format = AttributesFormatPlain
			case "json":
				format = AttributesFormatJson
			case "yaml":
				format = AttributesFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml", rawFormat)
			}
		}
		rawNode := args[0]

		p := AttributesParams{
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
			All:            rawAll,
			Format:         format,
			Node:           rawNode,
		}

		result, err := attributes(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case AttributesFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case AttributesFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return AttributesCommand(cmd)
}

type AttributesCommand *cobra.Command
