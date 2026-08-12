package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/gopcua/opcua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/config"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/render"
)

// configInit creates a profile, asking for each setting unless told not to.
//
// The form offers the security policies and user token types the server actually
// advertises, because those are the only answers that will work and guessing is
// how a first connection fails.
func configInit(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigInitParams) error {
	streams := newOutput(cmd, commandIO)

	path, _ := cmd.Flags().GetString("config")
	file, err := config.Load(path)
	if err != nil {
		return err
	}

	if _, exists := file.Profiles[p.Name]; exists && !p.Force && !p.NonInteractive {
		replace, err := promptConfirm(fmt.Sprintf("Profile %q already exists. Replace it?", p.Name))
		if err != nil || !replace {
			return fmt.Errorf("profile %q already exists; pass --force to replace it", p.Name)
		}
	} else if exists && !p.Force {
		return fmt.Errorf("profile %q already exists; pass --force to replace it", p.Name)
	}

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return err
	}

	form := profileForm{
		Name:        p.Name,
		Endpoint:    settings.Endpoint(),
		Policy:      settings.Policy,
		Mode:        settings.Mode,
		AuthMode:    settings.AuthMode,
		Username:    settings.Username,
		Password:    settings.Password,
		MakeDefault: p.Default,
	}

	if !p.NonInteractive {
		policies, modes, tokens := offered(ctx, settings, streams)
		if err := runProfileForm(&form, policies, modes, tokens); err != nil {
			return err
		}
	}

	profile := file.Upsert(form.Name)
	profile.Endpoints = []string{form.Endpoint}
	profile.Security.Policy = form.Policy
	profile.Security.Mode = form.Mode
	profile.Auth.Mode = form.AuthMode
	profile.Auth.Username = form.Username
	profile.Auth.Password = form.Password
	if settings.CertFile != "" {
		profile.Security.Certificate = settings.CertFile
		profile.Security.PrivateKey = settings.KeyFile
	}
	if form.MakeDefault {
		file.Default = form.Name
	}

	if err := file.Save(); err != nil {
		return err
	}

	streams.Out.Okf("wrote profile %q to %s", form.Name, file.SourcePath())
	if form.Password != "" {
		streams.Err.Warnf("the password is stored in plain text; consider auth.passwordFile or OPCUA_PASSWORD instead")
	}
	streams.Out.Notef("try it with: opcua ping --profile %s", form.Name)
	return nil
}

// offered asks the server what it supports, so the form can present real
// choices. A server that cannot be reached is not an error here - the profile is
// being created, possibly before the server exists - so the built-in lists are
// used instead.
func offered(ctx context.Context, settings *conn.Settings, streams *output) (policies, modes, tokens []string) {
	descriptions, err := opcua.GetEndpoints(ctx, settings.Endpoint(), opcua.DialTimeout(settings.DialTimeout))
	if err != nil {
		streams.Err.Warnf("cannot reach %s yet (%v); offering the standard choices", settings.Endpoint(), err)
		return nil, nil, nil
	}

	policies = []string{"auto"}
	modes = []string{"auto"}
	tokens = []string{"auto"}
	for _, description := range descriptions {
		if description == nil {
			continue
		}
		policies = appendUnique(policies, conn.PolicyName(description.SecurityPolicyURI))
		modes = appendUnique(modes, conn.ModeName(description.SecurityMode))
		for _, token := range description.UserIdentityTokens {
			if token == nil {
				continue
			}
			tokens = appendUnique(tokens, strings.ToLower(conn.TokenName(token.TokenType)))
		}
	}
	if len(tokens) == 1 {
		tokens = append(tokens, "anonymous")
	}
	return policies, modes, tokens
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// configShow prints the settings a command would connect with.
func configShow(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigShowParams) error {
	streams := newOutput(cmd, commandIO)

	path, _ := cmd.Flags().GetString("config")
	file, err := config.Load(path)
	if err != nil {
		return err
	}

	if p.All {
		document := *file
		if !p.Reveal {
			document.Profiles = map[string]*config.Profile{}
			for name, profile := range file.Profiles {
				document.Profiles[name] = profile.Redacted()
			}
		}
		return encodeConfig(streams, string(p.Format), document)
	}

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return err
	}

	resolved := map[string]any{
		"profile":        settings.Profile,
		"endpoints":      settings.Endpoints,
		"securityPolicy": settings.Policy,
		"securityMode":   settings.Mode,
		"authMode":       settings.AuthMode,
		"username":       settings.Username,
		"certificate":    settings.CertFile,
		"privateKey":     settings.KeyFile,
		"insecure":       settings.Insecure,
		"requestTimeout": settings.RequestTimeout.String(),
		"dialTimeout":    settings.DialTimeout.String(),
		"sessionTimeout": settings.SessionTimeout.String(),
		"reconnect":      settings.Reconnect,
		"applicationUri": settings.ApplicationURI,
		"locales":        settings.Locales,
		"configFile":     file.SourcePath(),
	}
	if settings.Password != "" {
		if p.Reveal {
			resolved["password"] = settings.Password
		} else {
			resolved["password"] = "••••••••"
		}
	}

	if string(p.Format) == "text" {
		streams.Out.Title("Resolved connection", file.SourcePath())
		fields := []render.Field{
			render.F("Profile", orNone(settings.Profile)),
			render.F("Endpoints", strings.Join(settings.Endpoints, "\n")),
			render.F("Policy", settings.Policy),
			render.F("Mode", settings.Mode),
			render.F("Auth", settings.AuthMode),
		}
		if settings.Username != "" {
			fields = append(fields, render.F("User name", settings.Username))
		}
		if settings.CertFile != "" {
			fields = append(fields, render.F("Certificate", settings.CertFile), render.F("Private key", settings.KeyFile))
		}
		fields = append(fields,
			render.F("Request timeout", settings.RequestTimeout.String()),
			render.F("Session timeout", settings.SessionTimeout.String()),
			render.F("Reconnect", strconv.FormatBool(settings.Reconnect)),
			render.F("Application URI", settings.ApplicationURI),
		)
		streams.Out.KV(fields)
		return nil
	}

	return encodeConfig(streams, string(p.Format), resolved)
}

func encodeConfig(streams *output, format string, document any) error {
	if format == "json" {
		encoder := json.NewEncoder(streams.Out.Raw())
		encoder.SetIndent("", "  ")
		return encoder.Encode(document)
	}

	encoded, err := yaml.MarshalWithOptions(document, yaml.Indent(2), yaml.IndentSequence(true))
	if err != nil {
		return err
	}
	_, err = streams.Out.Raw().Write(encoded)
	return err
}

func orNone(value string) string {
	if value == "" {
		return "(none configured)"
	}
	return value
}

// configList lists the configured profiles.
func configList(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigListParams) (cligen.ProfileList, error) {
	streams := newOutput(cmd, commandIO)

	path, _ := cmd.Flags().GetString("config")
	file, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	defaultName := file.DefaultName()
	profiles := make(cligen.ProfileList, 0, len(file.Profiles))
	for _, name := range file.Names() {
		profile := file.Profiles[name]
		entry := cligen.Profile{
			Name:      name,
			IsDefault: name == defaultName,
			Endpoints: profile.Endpoints,
		}
		if profile.Description != "" {
			entry.Description = new(profile.Description)
		}
		if profile.Security.Policy != "" {
			entry.SecurityPolicy = new(profile.Security.Policy)
		}
		if profile.Security.Mode != "" {
			entry.SecurityMode = new(profile.Security.Mode)
		}
		if profile.Auth.Mode != "" {
			entry.AuthMode = new(profile.Auth.Mode)
		}
		if profile.Auth.Username != "" {
			entry.Username = new(profile.Auth.Username)
		}
		if profile.Security.Certificate != "" {
			entry.Certificate = new(profile.Security.Certificate)
		}
		if profile.Security.PrivateKey != "" {
			entry.PrivateKey = new(profile.Security.PrivateKey)
		}
		if profile.Security.Insecure {
			entry.Insecure = new(true)
		}
		if profile.Session.RequestTimeout != "" {
			entry.RequestTimeout = new(profile.Session.RequestTimeout)
		}
		if profile.Session.Timeout != "" {
			entry.SessionTimeout = new(profile.Session.Timeout)
		}
		profiles = append(profiles, entry)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return profiles, nil
	}

	if len(profiles) == 0 {
		streams.Out.Notef("no profiles in %s; create one with: opcua config init", file.SourcePath())
		return profiles, nil
	}

	rows := make([][]string, 0, len(profiles))
	for _, profile := range profiles {
		marker := ""
		if profile.IsDefault {
			marker = streams.Out.T.Symbols.Good
		}
		rows = append(rows, []string{
			marker,
			profile.Name,
			strings.Join(profile.Endpoints, " "),
			derefOr(profile.SecurityPolicy, "auto"),
			derefOr(profile.SecurityMode, "auto"),
			derefOr(profile.AuthMode, "auto"),
		})
	}

	if format == "plain" {
		streams.Out.Plain(rows)
		return profiles, nil
	}

	streams.Out.Title("Profiles", file.SourcePath())
	streams.Out.Table([]string{"", "NAME", "ENDPOINTS", "POLICY", "MODE", "AUTH"}, rows)
	return profiles, nil
}

// configUse makes a profile the default.
func configUse(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigUseParams) error {
	streams := newOutput(cmd, commandIO)

	path, _ := cmd.Flags().GetString("config")
	file, err := config.Load(path)
	if err != nil {
		return err
	}
	if _, _, err := file.Resolve(p.Name); err != nil {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}

	file.Default = p.Name
	if err := file.Save(); err != nil {
		return err
	}

	streams.Out.Okf("default profile is now %q", p.Name)
	return nil
}

// configSet changes one setting of a profile.
func configSet(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigSetParams) error {
	streams := newOutput(cmd, commandIO)

	path, _ := cmd.Flags().GetString("config")
	file, err := config.Load(path)
	if err != nil {
		return err
	}

	name := p.Name
	if name == "" {
		if fromFlag, _ := cmd.Flags().GetString("profile"); fromFlag != "" {
			name = fromFlag
		} else {
			name = file.DefaultName()
		}
	}
	if name == "" {
		name = "default"
	}

	profile := file.Upsert(name)
	if err := profile.Set(p.Setting, p.Value); err != nil {
		return err
	}
	if err := file.Save(); err != nil {
		return err
	}

	streams.Out.Okf("%s.%s = %s", name, p.Setting, p.Value)
	return nil
}

// configRemove deletes a profile.
func configRemove(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigRemoveParams) error {
	streams := newOutput(cmd, commandIO)

	path, _ := cmd.Flags().GetString("config")
	file, err := config.Load(path)
	if err != nil {
		return err
	}

	if !p.Yes {
		confirmed, err := promptConfirm(fmt.Sprintf("Delete profile %q?", p.Name))
		if err != nil {
			return fmt.Errorf("%s (pass --yes to skip the prompt)", err)
		}
		if !confirmed {
			return fmt.Errorf("cancelled")
		}
	}

	if err := file.Remove(p.Name); err != nil {
		return fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if err := file.Save(); err != nil {
		return err
	}

	streams.Out.Okf("deleted profile %q", p.Name)
	return nil
}

// configPath prints where the configuration file lives.
func configPath(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ConfigPathParams) error {
	path, _ := cmd.Flags().GetString("config")
	fmt.Fprintln(commandIO.Out, config.Path(path))
	return nil
}
