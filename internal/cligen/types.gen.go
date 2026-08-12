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

type TypeGetFormat string

const (
	TypeGetFormatText     TypeGetFormat = "text"
	TypeGetFormatMarkdown TypeGetFormat = "markdown"
	TypeGetFormatJson     TypeGetFormat = "json"
	TypeGetFormatYaml     TypeGetFormat = "yaml"
)

type TypeGetParams struct {
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
	NoFields       bool
	Format         TypeGetFormat
	Node           string
}
type TypeGetHandler func(ctx context.Context, cmd *cobra.Command, io IO, p TypeGetParams) (TypeInfo, error)

func newTypeGetCommand(typeGet TypeGetHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "get <node-id>",
		Short: "Describe a data type, including its fields.",
		Long: `Reports a type node's inheritance chain and, for a structure or
enumeration, the fields the server publishes in its
DataTypeDefinition attribute. That is what lets a client decode a
custom structure, so this is the command to reach for when a value
comes back as an opaque extension object.
`,
		Args: rangeArgs(1, 1),
	}

	var rawNoFields bool
	cmd.Flags().BoolVar(&rawNoFields, "no-fields", false, "Skip reading the DataTypeDefinition attribute.")
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
		var format TypeGetFormat
		if rawFormat != "" {
			switch rawFormat {
			case "text":
				format = TypeGetFormatText
			case "markdown":
				format = TypeGetFormatMarkdown
			case "json":
				format = TypeGetFormatJson
			case "yaml":
				format = TypeGetFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of text, markdown, json, yaml", rawFormat)
			}
		}
		rawNode := args[0]

		p := TypeGetParams{
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
			NoFields:       rawNoFields,
			Format:         format,
			Node:           rawNode,
		}

		result, err := typeGet(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case TypeGetFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case TypeGetFormatYaml:
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

type TypeOfFormat string

const (
	TypeOfFormatPlain TypeOfFormat = "plain"
	TypeOfFormatTable TypeOfFormat = "table"
	TypeOfFormatJson  TypeOfFormat = "json"
	TypeOfFormatYaml  TypeOfFormat = "yaml"
)

type TypeOfParams struct {
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
	Format         TypeOfFormat
	Node           string
}
type TypeOfHandler func(ctx context.Context, cmd *cobra.Command, io IO, p TypeOfParams) (TypeOf, error)

func newTypeOfCommand(typeOf TypeOfHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "of <node-id>",
		Short: "Show the data type of a variable.",
		Long:  "Reads a variable's DataType, ValueRank and ArrayDimensions and\nresolves the data type node to its browse name, so an answer of\n`i=11` is reported as `Double`.\n",
		Args:  rangeArgs(1, 1),
	}

	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "plain", "Output format: plain, table, json, yaml.")

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
		var format TypeOfFormat
		if rawFormat != "" {
			switch rawFormat {
			case "plain":
				format = TypeOfFormatPlain
			case "table":
				format = TypeOfFormatTable
			case "json":
				format = TypeOfFormatJson
			case "yaml":
				format = TypeOfFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of plain, table, json, yaml", rawFormat)
			}
		}
		rawNode := args[0]

		p := TypeOfParams{
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
			Node:           rawNode,
		}

		result, err := typeOf(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case TypeOfFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case TypeOfFormatYaml:
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

type TypeListKind string

const (
	TypeListKindData      TypeListKind = "data"
	TypeListKindObject    TypeListKind = "object"
	TypeListKindVariable  TypeListKind = "variable"
	TypeListKindReference TypeListKind = "reference"
	TypeListKindEvent     TypeListKind = "event"
)

type TypeListFormat string

const (
	TypeListFormatTree  TypeListFormat = "tree"
	TypeListFormatTable TypeListFormat = "table"
	TypeListFormatPlain TypeListFormat = "plain"
	TypeListFormatJson  TypeListFormat = "json"
	TypeListFormatYaml  TypeListFormat = "yaml"
)

type TypeListParams struct {
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
	Kind           TypeListKind
	Depth          int
	Namespace      []string
	Limit          int
	Format         TypeListFormat
}
type TypeListHandler func(ctx context.Context, cmd *cobra.Command, io IO, p TypeListParams) (NodeList, error)

func newTypeListCommand(typeList TypeListHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the type hierarchy of a server.",
		Long:  "Walks one of the type folders and reports the tree of types the\nserver defines. Combine with `--namespace` to see only the types a\ncompanion specification or a vendor added.\n",
		Example: `  # Show the event type hierarchy.
  opcua types list --kind event
  # Show only the data types a vendor namespace adds.
  opcua types list --namespace 2`,
		Args: rangeArgs(0, 0),
	}

	var rawKind string
	cmd.Flags().StringVar(&rawKind, "kind", "data", "Which hierarchy: data, object, variable, reference, event.")
	var rawDepth int
	cmd.Flags().IntVarP(&rawDepth, "depth", "d", 0, "Levels to descend (0 = no limit).")
	var rawNamespace []string
	cmd.Flags().StringSliceVar(&rawNamespace, "namespace", nil, "Only report types in this namespace index; repeatable.")
	var rawLimit int
	cmd.Flags().IntVar(&rawLimit, "limit", 0, "Stop after this many types (0 = no limit).")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "tree", "Output format: tree, table, plain, json, yaml.")

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
		var kind TypeListKind
		if rawKind != "" {
			switch rawKind {
			case "data":
				kind = TypeListKindData
			case "object":
				kind = TypeListKindObject
			case "variable":
				kind = TypeListKindVariable
			case "reference":
				kind = TypeListKindReference
			case "event":
				kind = TypeListKindEvent
			default:
				return fmt.Errorf("invalid --kind %q: must be one of data, object, variable, reference, event", rawKind)
			}
		}
		var format TypeListFormat
		if rawFormat != "" {
			switch rawFormat {
			case "tree":
				format = TypeListFormatTree
			case "table":
				format = TypeListFormatTable
			case "plain":
				format = TypeListFormatPlain
			case "json":
				format = TypeListFormatJson
			case "yaml":
				format = TypeListFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of tree, table, plain, json, yaml", rawFormat)
			}
		}

		p := TypeListParams{
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
			Kind:           kind,
			Depth:          rawDepth,
			Namespace:      rawNamespace,
			Limit:          rawLimit,
			Format:         format,
		}

		result, err := typeList(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case TypeListFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case TypeListFormatYaml:
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
func NewTypesCommand(
	typeGet TypeGetHandler,
	typeOf TypeOfHandler,
	typeList TypeListHandler,
) TypesCommand {
	cmd := &cobra.Command{
		Use:     "types",
		Short:   "Inspect data types and type definitions.",
		Aliases: []string{"type"},
	}
	cmd.AddCommand(
		newTypeGetCommand(typeGet),
		newTypeOfCommand(typeOf),
		newTypeListCommand(typeList),
	)
	return TypesCommand(cmd)
}

type TypesCommand *cobra.Command
