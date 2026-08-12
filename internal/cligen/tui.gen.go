package cligen

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"time"
)

type TuiTheme string

const (
	TuiThemeCharm   TuiTheme = "charm"
	TuiThemeNord    TuiTheme = "nord"
	TuiThemeDracula TuiTheme = "dracula"
	TuiThemeMono    TuiTheme = "mono"
)

type TuiParams struct {
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
	Node           string
	Refresh        time.Duration
	Theme          TuiTheme
	Watch          []string
}
type TuiHandler func(ctx context.Context, cmd *cobra.Command, io IO, p TuiParams) error

func NewTuiCommand(tui TuiHandler) TuiCommand {
	cmd := &cobra.Command{
		Use:     "tui",
		Short:   "Explore a server in a full-screen interactive browser.",
		Long:    "Opens the address space explorer: an expandable tree of the address space\non the left, and the selected node's attributes, type, references and live\nvalues on the right.\n\nMove with the arrow keys or hjkl. Right expands a branch in place and left\ncollapses it, so the shape of the server builds up as you walk it; each\nlevel is fetched once and kept. Enter re-roots the tree at the selected\nnode, which is how to get past a branch too deep to read, and escape goes\nback up.\n\n`/` filters the tree, keeping any branch whose descendant matches. `:`\nopens the command bar for a node id, a standard name, or a browse path.\n`tab` moves the keyboard to the detail pane, where left and right change\ntab and up and down scroll. `w` adds the selected variable to the watch\npanel, which subscribes to it and plots it. `?` lists every key.\n",
		Aliases: []string{"explore", "ui"},
		Example: `  # Explore the active profile's server.
  opcua tui
  # Open at a node with a different theme.
  opcua tui --node 'ns=2;s=Machine' --theme nord`,
		Args: rangeArgs(0, 0),
	}

	var rawNode string
	cmd.Flags().StringVar(&rawNode, "node", "i=85", "Node to open at.")
	var rawRefresh time.Duration
	cmd.Flags().DurationVar(&rawRefresh, "refresh", mustParseDuration("2s"), "How often to re-read values of the visible level.")
	var rawTheme string
	cmd.Flags().StringVar(&rawTheme, "theme", "charm", "Colour theme: charm, nord, dracula, mono.")
	var rawWatch []string
	cmd.Flags().StringSliceVar(&rawWatch, "watch", nil, "Add these nodes to the watch panel on startup; repeatable.")

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
		var theme TuiTheme
		if rawTheme != "" {
			switch rawTheme {
			case "charm":
				theme = TuiThemeCharm
			case "nord":
				theme = TuiThemeNord
			case "dracula":
				theme = TuiThemeDracula
			case "mono":
				theme = TuiThemeMono
			default:
				return fmt.Errorf("invalid --theme %q: must be one of charm, nord, dracula, mono", rawTheme)
			}
		}

		p := TuiParams{
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
			Node:           rawNode,
			Refresh:        rawRefresh,
			Theme:          theme,
			Watch:          rawWatch,
		}

		return tui(cmd.Context(), cmd, newIO(cmd), p)
	}

	return TuiCommand(cmd)
}

type TuiCommand *cobra.Command
