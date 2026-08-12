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

type CallFormat string

const (
	CallFormatPlain CallFormat = "plain"
	CallFormatJson  CallFormat = "json"
	CallFormatYaml  CallFormat = "yaml"
)

type CallParams struct {
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
	ArgType        []string
	Json           bool
	Describe       bool
	Format         CallFormat
	Object         string
	Method         string
	Argument       []string
}
type CallHandler func(ctx context.Context, cmd *cobra.Command, io IO, p CallParams) (CallResult, error)

func NewCallCommand(call CallHandler) CallCommand {
	cmd := &cobra.Command{
		Use:     "call <object-id> <method> [arg]...",
		Short:   "Call a method on an object.",
		Long:    "Calls METHOD on OBJECT with the arguments given.\n\nThe method's InputArguments property is read first, so each argument is\nparsed into the type the method actually declares and a wrong argument\ncount is caught before the call. Pass `--arg-type` to override the\ninferred type of an argument, repeating it in argument order.\n",
		Aliases: []string{"invoke"},
		Example: `  # Call a method with no arguments.
  opcua call i=2253 'ns=2;s=Restart'
  # Call a method by browse name with one argument.
  opcua call 'ns=2;s=Boiler' SetTemperature 21.5
  # Show what arguments the method expects.
  opcua call 'ns=2;s=Boiler' SetTemperature --describe`,
		Args: rangeArgs(2, -1),
	}

	var rawArgType []string
	cmd.Flags().StringArrayVar(&rawArgType, "arg-type", nil, "Force the type of an argument; repeatable, in order.")
	var rawJson bool
	cmd.Flags().BoolVar(&rawJson, "json", false, "Parse each argument as JSON.")
	var rawDescribe bool
	cmd.Flags().BoolVar(&rawDescribe, "describe", false, "Print the method signature and exit without calling it.")
	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "plain", "Output format: plain, json, yaml.")

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
		var format CallFormat
		if rawFormat != "" {
			switch rawFormat {
			case "plain":
				format = CallFormatPlain
			case "json":
				format = CallFormatJson
			case "yaml":
				format = CallFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of plain, json, yaml", rawFormat)
			}
		}
		rawObject := args[0]
		rawMethod := args[1]
		argument := args[2:]

		p := CallParams{
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
			ArgType:        rawArgType,
			Json:           rawJson,
			Describe:       rawDescribe,
			Format:         format,
			Object:         rawObject,
			Method:         rawMethod,
			Argument:       argument,
		}

		result, err := call(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case CallFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case CallFormatYaml:
			enc := yaml.NewEncoder(cmd.OutOrStdout())
			if err := enc.Encode(result); err != nil {
				return err
			}
			return enc.Close()
		}
		return nil
	}

	return CallCommand(cmd)
}

type CallCommand *cobra.Command
