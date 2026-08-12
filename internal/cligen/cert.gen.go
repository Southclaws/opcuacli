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

type CertGenerateParams struct {
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
	OutDir         string
	CommonName     string
	Dns            []string
	Ip             []string
	Bits           int
	Days           int
	Force          bool
	UpdateProfile  bool
}
type CertGenerateHandler func(ctx context.Context, cmd *cobra.Command, io IO, p CertGenerateParams) error

func newCertGenerateCommand(certGenerate CertGenerateHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Create a self-signed client certificate and key.",
		Long: `Writes a certificate and private key suitable for OPC UA client
authentication, with the application URI in a subject alternative
name as the specification requires.

The server will reject the certificate until it is trusted; most
servers move a rejected certificate into a folder for an operator to
approve, so expect to connect once, approve, then connect again.
`,
		Aliases: []string{"gen"},
		Example: `  # Create a certificate and register it with the active profile.
  opcua cert generate`,
		Args: rangeArgs(0, 0),
	}

	var rawOutDir string
	cmd.Flags().StringVarP(&rawOutDir, "out-dir", "o", "", "Directory to write cert.pem and key.pem into.")
	var rawCommonName string
	cmd.Flags().StringVar(&rawCommonName, "common-name", "opcuacli", "Certificate common name.")
	var rawDns []string
	cmd.Flags().StringSliceVar(&rawDns, "dns", nil, "DNS name to add as a subject alternative name; repeatable.")
	var rawIp []string
	cmd.Flags().StringSliceVar(&rawIp, "ip", nil, "IP address to add as a subject alternative name; repeatable.")
	var rawBits int
	cmd.Flags().IntVar(&rawBits, "bits", 2048, "RSA key size in bits.")
	var rawDays int
	cmd.Flags().IntVar(&rawDays, "days", 3650, "Validity period in days.")
	var rawForce bool
	cmd.Flags().BoolVar(&rawForce, "force", false, "Overwrite existing files.")
	var rawUpdateProfile bool
	cmd.Flags().BoolVar(&rawUpdateProfile, "update-profile", true, "Point the active profile at the new certificate.")

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

		p := CertGenerateParams{
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
			OutDir:         rawOutDir,
			CommonName:     rawCommonName,
			Dns:            rawDns,
			Ip:             rawIp,
			Bits:           rawBits,
			Days:           rawDays,
			Force:          rawForce,
			UpdateProfile:  rawUpdateProfile,
		}

		return certGenerate(cmd.Context(), cmd, newIO(cmd), p)
	}

	return cmd
}

type CertShowFormat string

const (
	CertShowFormatText CertShowFormat = "text"
	CertShowFormatJson CertShowFormat = "json"
	CertShowFormatYaml CertShowFormat = "yaml"
)

type CertShowParams struct {
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
	Format         CertShowFormat
	File           string
}
type CertShowHandler func(ctx context.Context, cmd *cobra.Command, io IO, p CertShowParams) (CertificateInfo, error)

func newCertShowCommand(certShow CertShowHandler) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show [file]",
		Short: "Show certificate details, locally or from a server.",
		Long: `With no argument this fetches the certificate the server presents on
its endpoints, which is how to check an expiry or a thumbprint
without a session. Give a file to inspect a local certificate
instead.
`,
		Args: rangeArgs(0, 1),
	}

	var rawFormat string
	cmd.Flags().StringVarP(&rawFormat, "format", "f", "text", "Output format: text, json, yaml.")

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
		var format CertShowFormat
		if rawFormat != "" {
			switch rawFormat {
			case "text":
				format = CertShowFormatText
			case "json":
				format = CertShowFormatJson
			case "yaml":
				format = CertShowFormatYaml
			default:
				return fmt.Errorf("invalid --format %q: must be one of text, json, yaml", rawFormat)
			}
		}
		rawFile := ""
		if len(args) > 0 {
			rawFile = args[0]
		}

		p := CertShowParams{
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
			File:           rawFile,
		}

		result, err := certShow(cmd.Context(), cmd, newIO(cmd), p)
		if err != nil {
			return err
		}
		switch format {
		case CertShowFormatJson:
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		case CertShowFormatYaml:
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
func NewCertCommand(
	certGenerate CertGenerateHandler,
	certShow CertShowHandler,
) CertCommand {
	cmd := &cobra.Command{
		Use:   "cert",
		Short: "Manage the client certificate used for secure channels.",
	}
	cmd.AddCommand(
		newCertGenerateCommand(certGenerate),
		newCertShowCommand(certShow),
	)
	return CertCommand(cmd)
}

type CertCommand *cobra.Command
