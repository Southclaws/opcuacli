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

type ReferencesDirection string

const (
	ReferencesDirectionForward ReferencesDirection = "forward"
	ReferencesDirectionInverse ReferencesDirection = "inverse"
	ReferencesDirectionBoth    ReferencesDirection = "both"
)

type ReferencesFormat string

const (
	ReferencesFormatTable ReferencesFormat = "table"
	ReferencesFormatPlain ReferencesFormat = "plain"
	ReferencesFormatJson  ReferencesFormat = "json"
	ReferencesFormatYaml  ReferencesFormat = "yaml"
)

type ReferencesParams struct {
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
	Direction      ReferencesDirection
	RefType        string
	NoSubtypes     bool
	Class          []string
	Format         ReferencesFormat
	Node           string
}
type ReferencesHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ReferencesParams) (ReferenceList, error)

func NewReferencesCommand(references ReferencesHandler) ReferencesCommand {
	cmd := &cobra.Command{
		Use:     "references <node-id>",
		Short:   "List every reference of a node with its reference type.",
		Long:    "Where `browse` presents the address space as a hierarchy, this presents\none node's raw references: the reference type, its direction, and the\ntarget. Use it to see type definitions, modelling rules, event sources\nand other non-hierarchical links.\n",
		Aliases: []string{"refs"},
		Args:    rangeArgs(1, 1),
	}

	var rawDirection string
	cmd.Flags().StringVar(&rawDirection, "direction", "both", "Reference direction: forward, inverse, both.")
	var rawRefType string
	cmd.Flags().StringVar(&rawRefType, "ref-type", "all", "Reference type to follow, by name, node id, or all.")
	var rawNoSubtypes bool
	cmd.Flags().BoolVar(&rawNoSubtypes, "no-subtypes", false, "Do not follow subtypes of the selected reference type.")
	var rawClass []string
	cmd.Flags().StringSliceVar(&rawClass, "class", nil, "Only report references to these node classes.")
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
		var direction ReferencesDirection
		if rawDirection != "" {
			switch rawDirection {
			case "forward":
				direction = ReferencesDirectionForward
			case "inverse":
				direction = ReferencesDirectionInverse
			case "both":
				direction = ReferencesDirectionBoth
			default:
				return fmt.Errorf("invalid --direction %q: must be one of forward, inverse, both", rawDirection)
			}
		}
		var format ReferencesFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = ReferencesFormatTable
			case "plain":
				format = ReferencesFormatPlain
			case "json":
				format = ReferencesFormatJson
			case "yaml":
				format = ReferencesFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml", rawFormat)
			}
		}
		rawNode := args[0]

		p := ReferencesParams{
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
			Direction:      direction,
			RefType:        rawRefType,
			NoSubtypes:     rawNoSubtypes,
			Class:          rawClass,
			Format:         format,
			Node:           rawNode,
		}

		result, err := references(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ReferencesFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ReferencesFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return ReferencesCommand(cmd)
}

type ReferencesCommand *cobra.Command
