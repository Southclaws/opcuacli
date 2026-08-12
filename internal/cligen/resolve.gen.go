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

type ResolveFormat string

const (
	ResolveFormatPlain ResolveFormat = "plain"
	ResolveFormatTable ResolveFormat = "table"
	ResolveFormatJson  ResolveFormat = "json"
	ResolveFormatYaml  ResolveFormat = "yaml"
)

type ResolveParams struct {
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
	Namespace      int
	Format         ResolveFormat
	Path           []string
}
type ResolveHandler func(ctx context.Context, cmd *cobra.Command, io IO, p ResolveParams) (ResolvedPathList, error)

func NewResolveCommand(resolve ResolveHandler) ResolveCommand {
	cmd := &cobra.Command{
		Use:     "resolve <path>...",
		Short:   "Translate a browse path into a node id.",
		Long:    "Calls TranslateBrowsePathsToNodeIds, which resolves a path of browse\nnames against the server rather than crawling it.\n\nPath elements are separated by `/`. An element may carry an explicit\nnamespace as `2:Name`; without one the namespace of `--namespace` is\nused. Paths are relative to `--root`, which defaults to the Objects\nfolder, so `Server/ServerStatus/CurrentTime` resolves from there.\n",
		Aliases: []string{"path"},
		Example: `  # Resolve a well-known path to its node id.
  opcua resolve Server/ServerStatus/CurrentTime
  # Resolve from the root folder instead.
  opcua resolve --root i=84 Objects/Server`,
		Args: rangeArgs(1, -1),
	}

	var rawRoot string
	cmd.Flags().StringVar(&rawRoot, "root", "i=85", "Node the path is relative to.")
	var rawNamespace int
	cmd.Flags().IntVar(&rawNamespace, "namespace", 0, "Namespace index for path elements that do not name one.")
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
		var format ResolveFormat
		if rawFormat != "" {
			switch rawFormat {
			case "plain":
				format = ResolveFormatPlain
			case "table":
				format = ResolveFormatTable
			case "json":
				format = ResolveFormatJson
			case "yaml":
				format = ResolveFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of plain, table, json, yaml", rawFormat)
			}
		}
		path := args[0:]

		p := ResolveParams{
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
			Namespace:      rawNamespace,
			Format:         format,
			Path:           path,
		}

		result, err := resolve(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case ResolveFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case ResolveFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return ResolveCommand(cmd)
}

type ResolveCommand *cobra.Command
