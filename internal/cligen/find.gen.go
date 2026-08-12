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

type FindMatch string

const (
	FindMatchAny         FindMatch = "any"
	FindMatchBrowseName  FindMatch = "browse-name"
	FindMatchDisplayName FindMatch = "display-name"
	FindMatchNodeId      FindMatch = "node-id"
)

type FindFormat string

const (
	FindFormatTable FindFormat = "table"
	FindFormatPlain FindFormat = "plain"
	FindFormatJson  FindFormat = "json"
	FindFormatYaml  FindFormat = "yaml"
	FindFormatJsonl FindFormat = "jsonl"
)

type FindParams struct {
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
	Root           string
	MaxDepth       int
	Match          FindMatch
	Regex          bool
	Class          []string
	Namespace      []string
	Values         bool
	Limit          int
	Format         FindFormat
	Pattern        string
}
type FindHandler func(ctx context.Context, cmd *cobra.Command, io IO, p FindParams) (NodeList, error)

func NewFindCommand(find FindHandler) FindCommand {
	cmd := &cobra.Command{
		Use:     "find <pattern>",
		Short:   "Search the address space for nodes by name.",
		Long:    "Walks the address space from `--root` and reports nodes whose name\nmatches PATTERN. Matching is a case-insensitive substring test unless\n`--regex` is given.\n\nThis is a client-side crawl, not a server-side query, so bound it with\n`--max-depth` and `--limit` on a large address space.\n",
		Aliases: []string{"search"},
		Example: `  # Find nodes with "temperature" in the name.
  opcua find temperature
  # Find sensor variables by pattern and read them.
  opcua find '^Sensor\d+$' --regex --class variable --values`,
		Args: rangeArgs(1, 1),
	}

	var rawRoot string
	cmd.Flags().StringVar(&rawRoot, "root", "i=84", "Node to search from.")
	var rawMaxDepth int
	cmd.Flags().IntVar(&rawMaxDepth, "max-depth", 8, "Levels to descend (0 = no limit).")
	var rawMatch string
	cmd.Flags().StringVar(&rawMatch, "match", "any", "Which name to match: any, browse-name, display-name, node-id.")
	var rawRegex bool
	cmd.Flags().BoolVar(&rawRegex, "regex", false, "Treat PATTERN as a regular expression.")
	var rawClass []string
	cmd.Flags().StringSliceVar(&rawClass, "class", nil, "Only report these node classes.")
	var rawNamespace []string
	cmd.Flags().StringSliceVar(&rawNamespace, "namespace", nil, "Only report nodes in this namespace index; repeatable.")
	var rawValues bool
	cmd.Flags().BoolVar(&rawValues, "values", false, "Read the value of every variable that matches.")
	var rawLimit int
	cmd.Flags().IntVar(&rawLimit, "limit", 100, "Stop after this many matches (0 = no limit).")
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
		var match FindMatch
		if rawMatch != "" {
			switch rawMatch {
			case "any":
				match = FindMatchAny
			case "browse-name":
				match = FindMatchBrowseName
			case "display-name":
				match = FindMatchDisplayName
			case "node-id":
				match = FindMatchNodeId
			default:
				return fmt.Errorf("invalid --match %q: must be one of any, browse-name, display-name, node-id", rawMatch)
			}
		}
		var format FindFormat
		if rawFormat != "" {
			switch rawFormat {
			case "table":
				format = FindFormatTable
			case "plain":
				format = FindFormatPlain
			case "json":
				format = FindFormatJson
			case "yaml":
				format = FindFormatYaml
			case "jsonl":
				format = FindFormatJsonl
			default:
				return fmt.Errorf("invalid --format %q: must be one of table, plain, json, yaml, jsonl", rawFormat)
			}
		}
		rawPattern := args[0]

		p := FindParams{
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
			Root:           rawRoot,
			MaxDepth:       rawMaxDepth,
			Match:          match,
			Regex:          rawRegex,
			Class:          rawClass,
			Namespace:      rawNamespace,
			Values:         rawValues,
			Limit:          rawLimit,
			Format:         format,
			Pattern:        rawPattern,
		}

		result, err := find(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case FindFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case FindFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return FindCommand(cmd)
}

type FindCommand *cobra.Command
