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

type BrowseDirection string

const (
	BrowseDirectionForward BrowseDirection = "forward"
	BrowseDirectionInverse BrowseDirection = "inverse"
	BrowseDirectionBoth    BrowseDirection = "both"
)

type BrowseFormat string

const (
	BrowseFormatTree  BrowseFormat = "tree"
	BrowseFormatTable BrowseFormat = "table"
	BrowseFormatPlain BrowseFormat = "plain"
	BrowseFormatJson  BrowseFormat = "json"
	BrowseFormatYaml  BrowseFormat = "yaml"
	BrowseFormatJsonl BrowseFormat = "jsonl"
)

type BrowseParams struct {
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
	Depth          int
	Direction      BrowseDirection
	RefType        string
	NoSubtypes     bool
	Class          []string
	Filter         string
	Regex          bool
	Values         bool
	Limit          int
	Format         BrowseFormat
	Node           string
}
type BrowseHandler func(ctx context.Context, cmd *cobra.Command, io IO, p BrowseParams) (NodeList, error)

func NewBrowseCommand(browse BrowseHandler) BrowseCommand {
	cmd := &cobra.Command{
		Use:     "browse [node-id]",
		Short:   "List the references of a node.",
		Long:    "Walks the address space from a starting node. With the default depth of\n1 this lists the immediate children, like `ls`; raise `--depth` to\nrecurse, and the tree renderer draws the hierarchy.\n\nOnly hierarchical references are followed by default, which is what\nmakes the result look like a filesystem. Pass `--ref-type` to follow a\ndifferent reference, or `--ref-type all` for every reference including\ntype definitions and modelling rules.\n\nAdd `--values` to read the Value attribute of every variable found, so\none call gives both the shape of a subtree and its current data.\n",
		Aliases: []string{"ls"},
		Example: `  # List the Objects folder of the active profile's server.
  opcua browse
  # Walk three levels of the Server object.
  opcua browse i=2253 --depth 3
  # Read every variable in the whole address space.
  opcua browse --depth 0 --values --class variable
  # Every reference of a node, as JSON.
  opcua browse 'ns=2;s=Machine' --ref-type all --format json`,
		Args: rangeArgs(0, 1),
	}

	var rawDepth int
	cmd.Flags().IntVarP(&rawDepth, "depth", "d", 1, "Levels to descend (1 = immediate children).")
	var rawDirection string
	cmd.Flags().StringVar(&rawDirection, "direction", "forward", "Reference direction: forward, inverse, both.")
	var rawRefType string
	cmd.Flags().StringVar(&rawRefType, "ref-type", "HierarchicalReferences", "Reference type to follow, by name, node id, or all.")
	var rawNoSubtypes bool
	cmd.Flags().BoolVar(&rawNoSubtypes, "no-subtypes", false, "Do not follow subtypes of the selected reference type.")
	var rawClass []string
	cmd.Flags().StringSliceVar(&rawClass, "class", nil, "Only include these node classes: object, variable, method, ...")
	var rawFilter string
	cmd.Flags().StringVar(&rawFilter, "filter", "", "Only include nodes whose name matches this pattern.")
	var rawRegex bool
	cmd.Flags().BoolVar(&rawRegex, "regex", false, "Treat --filter as a regular expression.")
	var rawValues bool
	cmd.Flags().BoolVar(&rawValues, "values", false, "Read the value of every variable that is found.")
	var rawLimit int
	cmd.Flags().IntVar(&rawLimit, "limit", 0, "Stop after this many nodes (0 = no limit).")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "tree", "Output format: tree, table, plain, json, yaml, jsonl.")

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
		var direction BrowseDirection
		if rawDirection != "" {
			switch rawDirection {
			case "forward":
				direction = BrowseDirectionForward
			case "inverse":
				direction = BrowseDirectionInverse
			case "both":
				direction = BrowseDirectionBoth
			default:
				return fmt.Errorf("invalid --direction %q: must be one of forward, inverse, both", rawDirection)
			}
		}
		var format BrowseFormat
		if rawFormat != "" {
			switch rawFormat {
			case "tree":
				format = BrowseFormatTree
			case "table":
				format = BrowseFormatTable
			case "plain":
				format = BrowseFormatPlain
			case "json":
				format = BrowseFormatJson
			case "yaml":
				format = BrowseFormatYaml
			case "jsonl":
				format = BrowseFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of tree, table, plain, json, yaml, jsonl", rawFormat)
			}
		}
		rawNode := "i=85"
		if len(args) > 0 {
			rawNode = args[0]
		}

		p := BrowseParams{
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
			Depth:          rawDepth,
			Direction:      direction,
			RefType:        rawRefType,
			NoSubtypes:     rawNoSubtypes,
			Class:          rawClass,
			Filter:         rawFilter,
			Regex:          rawRegex,
			Values:         rawValues,
			Limit:          rawLimit,
			Format:         format,
			Node:           rawNode,
		}

		result, err := browse(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case BrowseFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case BrowseFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return BrowseCommand(cmd)
}

type BrowseCommand *cobra.Command
