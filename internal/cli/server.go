package cli

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// serverInfo reports the server's status, build and uptime.
func serverInfo(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ServerInfoParams) (cligen.ServerInfo, error) {
	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.ServerInfo{}, err
	}
	defer session.Close(ctx)

	info, err := opc.ServerInfo(ctx, session.Client, session.Client.Endpoint)
	if err != nil {
		return cligen.ServerInfo{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return info, nil
	}
	if format == "markdown" {
		return info, renderMarkdown(session.Out, serverMarkdown(info, session.Client))
	}

	session.Out.Title(derefOr(info.ProductName, session.Client.Endpoint), info.Endpoint)

	state := session.Out.T.Styles.Good
	if info.State != "Running" {
		state = session.Out.T.Styles.Uncertain
	}

	fields := []render.Field{
		render.FS("State", info.State, state),
		render.F("Security", session.Client.Security()),
	}
	if info.Uptime != nil {
		fields = append(fields, render.F("Uptime", *info.Uptime))
	}
	if info.StartTime != nil {
		fields = append(fields, render.F("Started", info.StartTime.Local().Format(time.RFC3339)))
	}
	if info.CurrentTime != nil {
		fields = append(fields, render.F("Server clock", info.CurrentTime.Local().Format(time.RFC3339)))
	}
	if info.ClockSkew != nil {
		fields = append(fields, render.F("Clock skew", *info.ClockSkew))
	}
	fields = append(fields, render.Field{})

	for _, field := range []struct {
		key   string
		value *string
	}{
		{"Product", info.ProductName},
		{"Product URI", info.ProductURI},
		{"Manufacturer", info.ManufacturerName},
		{"Version", info.SoftwareVersion},
		{"Build", info.BuildNumber},
	} {
		if field.value != nil && *field.value != "" {
			fields = append(fields, render.F(field.key, *field.value))
		}
	}
	if info.BuildDate != nil {
		fields = append(fields, render.F("Build date", info.BuildDate.Local().Format("2006-01-02")))
	}
	if info.ServiceLevel != nil {
		fields = append(fields, render.F("Service level", strconv.Itoa(*info.ServiceLevel)))
	}
	if info.SecondsTillShutdown != nil && *info.SecondsTillShutdown > 0 {
		fields = append(fields,
			render.FS("Shutting down in", (time.Duration(*info.SecondsTillShutdown)*time.Second).String(),
				session.Out.T.Styles.Bad))
	}
	if info.ShutdownReason != nil && *info.ShutdownReason != "" {
		fields = append(fields, render.F("Shutdown reason", *info.ShutdownReason))
	}

	session.Out.KV(fields)

	if len(info.Namespaces) > 0 {
		session.Out.Println()
		rows := make([][]string, 0, len(info.Namespaces))
		for _, namespace := range info.Namespaces {
			rows = append(rows, []string{strconv.Itoa(namespace.Index), namespace.URI})
		}
		session.Out.Table([]string{"NS", "URI"}, rows)
	}

	return info, nil
}

func serverMarkdown(info cligen.ServerInfo, client *conn.Client) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", derefOr(info.ProductName, info.Endpoint))
	fmt.Fprintf(&b, "- **Endpoint:** `%s`\n", info.Endpoint)
	fmt.Fprintf(&b, "- **Security:** %s\n", client.Security())
	fmt.Fprintf(&b, "- **State:** %s\n", info.State)
	if info.Uptime != nil {
		fmt.Fprintf(&b, "- **Uptime:** %s\n", *info.Uptime)
	}
	if info.SoftwareVersion != nil {
		fmt.Fprintf(&b, "- **Version:** %s (build %s)\n", *info.SoftwareVersion, derefOr(info.BuildNumber, "?"))
	}
	if info.ManufacturerName != nil {
		fmt.Fprintf(&b, "- **Manufacturer:** %s\n", *info.ManufacturerName)
	}

	if len(info.Namespaces) > 0 {
		b.WriteString("\n## Namespaces\n\n| Index | URI |\n|---|---|\n")
		for _, namespace := range info.Namespaces {
			fmt.Fprintf(&b, "| %d | `%s` |\n", namespace.Index, namespace.URI)
		}
	}
	return b.String()
}

// serverCapabilities reports the operational limits and profiles of a server.
func serverCapabilities(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ServerCapabilitiesParams) (cligen.Capabilities, error) {
	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.Capabilities{}, err
	}
	defer session.Close(ctx)

	capabilities, err := opc.Capabilities(ctx, session.Client, session.Client.Endpoint)
	if err != nil {
		return cligen.Capabilities{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return capabilities, nil
	}

	limits := []struct {
		key   string
		value *int
	}{
		{"Nodes per read", capabilities.MaxNodesPerRead},
		{"Nodes per write", capabilities.MaxNodesPerWrite},
		{"Nodes per browse", capabilities.MaxNodesPerBrowse},
		{"Nodes per method call", capabilities.MaxNodesPerMethodCall},
		{"Nodes per translate", capabilities.MaxNodesPerTranslate},
		{"Nodes per register", capabilities.MaxNodesPerRegisterNodes},
		{"Nodes per node management", capabilities.MaxNodesPerNodeManagement},
		{"History read data", capabilities.MaxNodesPerHistoryReadData},
		{"History read events", capabilities.MaxNodesPerHistoryReadEvents},
		{"History update data", capabilities.MaxNodesPerHistoryUpdateData},
		{"Monitored items per call", capabilities.MaxMonitoredItemsPerCall},
		{"Browse continuation points", capabilities.MaxBrowseContinuationPoints},
		{"History continuation points", capabilities.MaxHistoryContinuationPoints},
		{"Query continuation points", capabilities.MaxQueryContinuationPoints},
		{"Max array length", capabilities.MaxArrayLength},
		{"Max string length", capabilities.MaxStringLength},
		{"Max byte string length", capabilities.MaxByteStringLength},
	}

	if format == "markdown" {
		var b strings.Builder
		fmt.Fprintf(&b, "# Capabilities of %s\n\n| Limit | Value |\n|---|---|\n", capabilities.Endpoint)
		for _, limit := range limits {
			if limit.value != nil {
				fmt.Fprintf(&b, "| %s | %d |\n", limit.key, *limit.value)
			}
		}
		if len(capabilities.ServerProfiles) > 0 {
			b.WriteString("\n## Profiles\n\n")
			for _, profile := range capabilities.ServerProfiles {
				fmt.Fprintf(&b, "- `%s`\n", profile)
			}
		}
		return capabilities, renderMarkdown(session.Out, b.String())
	}

	session.Out.Title("Capabilities", capabilities.Endpoint)

	fields := make([]render.Field, 0, len(limits)+2)
	if capabilities.MinSupportedSampleRate != nil {
		fields = append(fields, render.F("Min sample rate",
			(time.Duration(*capabilities.MinSupportedSampleRate)*time.Millisecond).String()))
	}
	for _, limit := range limits {
		if limit.value == nil {
			continue
		}
		// Zero means "no limit declared" in the specification, which is not the
		// same as a limit of zero operations.
		text := strconv.Itoa(*limit.value)
		if *limit.value == 0 {
			text = "unlimited"
		}
		fields = append(fields, render.F(limit.key, text))
	}
	if len(capabilities.LocaleIds) > 0 {
		fields = append(fields, render.F("Locales", strings.Join(capabilities.LocaleIds, ", ")))
	}
	session.Out.KV(fields)

	if len(capabilities.ServerProfiles) > 0 {
		session.Out.Println()
		session.Out.Title("Profiles", fmt.Sprintf("%d claimed", len(capabilities.ServerProfiles)))
		profiles := append([]string(nil), capabilities.ServerProfiles...)
		sort.Strings(profiles)
		for _, profile := range profiles {
			session.Out.Printf("  %s\n", session.Out.T.Styles.Dim.Render(profile))
		}
	}

	return capabilities, nil
}

// serverDiagnostics reports the diagnostic counters of a server.
func serverDiagnostics(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ServerDiagnosticsParams) (cligen.Diagnostics, error) {
	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.Diagnostics{}, err
	}
	defer session.Close(ctx)

	diagnostics, err := opc.Diagnostics(ctx, session.Client, session.Client.Endpoint, p.Sessions)
	if err != nil {
		return cligen.Diagnostics{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return diagnostics, nil
	}

	keys := make([]string, 0, len(diagnostics.Summary))
	for key := range diagnostics.Summary {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	if format == "markdown" {
		var b strings.Builder
		fmt.Fprintf(&b, "# Diagnostics of %s\n\n", diagnostics.Endpoint)
		fmt.Fprintf(&b, "Collection is %s.\n\n", enabledText(diagnostics.Enabled))
		if len(keys) > 0 {
			b.WriteString("| Counter | Value |\n|---|---|\n")
			for _, key := range keys {
				fmt.Fprintf(&b, "| %s | %v |\n", key, diagnostics.Summary[key])
			}
		}
		return diagnostics, renderMarkdown(session.Out, b.String())
	}

	session.Out.Title("Diagnostics", diagnostics.Endpoint)

	enabled := session.Out.T.Styles.Good
	if !diagnostics.Enabled {
		enabled = session.Out.T.Styles.Uncertain
	}
	session.Out.KV([]render.Field{render.FS("Collection", enabledText(diagnostics.Enabled), enabled)})

	if len(keys) == 0 {
		session.Err.Warnf("the server published no diagnostics summary; set Server/ServerDiagnostics/EnabledFlag to collect them")
		return diagnostics, nil
	}

	session.Out.Println()
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, fmt.Sprint(diagnostics.Summary[key])})
	}
	session.Out.Table([]string{"COUNTER", "VALUE"}, rows)

	if len(diagnostics.Sessions) > 0 {
		session.Out.Println()
		session.Out.Title("Sessions", fmt.Sprintf("%d", len(diagnostics.Sessions)))
		sessionRows := make([][]string, 0, len(diagnostics.Sessions))
		for _, entry := range diagnostics.Sessions {
			sessionRows = append(sessionRows, []string{
				fmt.Sprint(entry["sessionName"]),
				fmt.Sprint(entry["clientDescription"]),
				fmt.Sprint(entry["currentSubscriptionsCount"]),
				fmt.Sprint(entry["currentMonitoredItemsCount"]),
			})
		}
		session.Out.Table([]string{"SESSION", "CLIENT", "SUBS", "ITEMS"}, sessionRows)
	}

	return diagnostics, nil
}

func enabledText(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// serverRedundancy reports how a server expects failover to be handled.
func serverRedundancy(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.ServerRedundancyParams) (cligen.Redundancy, error) {
	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return cligen.Redundancy{}, err
	}
	defer session.Close(ctx)

	redundancy, err := opc.Redundancy(ctx, session.Client, session.Client.Endpoint, session.Settings.Endpoints)
	if err != nil {
		return cligen.Redundancy{}, fmt.Errorf("%w: %w", ErrService, err)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return redundancy, nil
	}

	if format == "markdown" {
		var b strings.Builder
		fmt.Fprintf(&b, "# Redundancy of %s\n\n", redundancy.Endpoint)
		fmt.Fprintf(&b, "- **Support:** %s\n", redundancy.Support)
		if redundancy.ServiceLevel != nil {
			fmt.Fprintf(&b, "- **Service level:** %d\n", *redundancy.ServiceLevel)
		}
		for _, uri := range redundancy.ServerUris {
			fmt.Fprintf(&b, "- **Member:** `%s`\n", uri)
		}
		return redundancy, renderMarkdown(session.Out, b.String())
	}

	session.Out.Title("Redundancy", redundancy.Endpoint)

	fields := []render.Field{render.F("Support", redundancy.Support)}
	if redundancy.Transparent != nil {
		failover := "this client fails over between --endpoint values"
		if *redundancy.Transparent {
			failover = "the server handles failover; one address is enough"
		}
		fields = append(fields, render.F("Failover", failover))
	}
	if redundancy.ServiceLevel != nil {
		fields = append(fields, render.F("Service level", strconv.Itoa(*redundancy.ServiceLevel)))
	}
	if len(redundancy.ServerUris) > 0 {
		fields = append(fields, render.F("Set members", strings.Join(redundancy.ServerUris, "\n")))
	}
	fields = append(fields, render.F("Configured", strings.Join(redundancy.ConfiguredEndpoints, "\n")))
	session.Out.KV(fields)

	if redundancy.Support == "None" {
		session.Err.Warnf("this server declares no redundancy; a second --endpoint would be a different server")
	}
	return redundancy, nil
}

// ping measures how long it takes to open a channel, activate a session, and
// read the server clock.
//
// Splitting the timings apart is what makes it diagnostic: the channel covers
// the network and the security handshake, the session covers the credentials.
func ping(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.PingParams) (cligen.PingResultList, error) {
	streams := newOutput(cmd, commandIO)

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return nil, err
	}

	format := string(p.Format)
	human := !machineReadable(format)

	var results cligen.PingResultList
	for sequence := 1; p.Count == 0 || sequence <= p.Count; sequence++ {
		if ctx.Err() != nil {
			break
		}
		if sequence > 1 {
			select {
			case <-ctx.Done():
				return results, nil
			case <-time.After(p.Interval):
			}
		}

		result := probe(ctx, settings, sequence, streams)
		results = append(results, result)

		if human && format != "table" {
			renderProbe(streams.Out, result)
		}
	}

	if human && format == "table" {
		rows := make([][]string, 0, len(results))
		for _, result := range results {
			rows = append(rows, []string{
				strconv.Itoa(result.Sequence),
				result.Endpoint,
				statusWord(streams.Out, result.Ok),
				derefOr(result.Channel, "-"),
				derefOr(result.Session, "-"),
				derefOr(result.Read, "-"),
				derefOr(result.Total, "-"),
			})
		}
		streams.Out.Table([]string{"#", "ENDPOINT", "OK", "CHANNEL", "SESSION", "READ", "TOTAL"}, rows)
	}

	for _, result := range results {
		if result.Ok {
			return results, nil
		}
	}
	if len(results) > 0 {
		return results, fmt.Errorf("%w: %s", ErrConnection, derefOr(results[0].Error, "no endpoint answered"))
	}
	return results, nil
}

// probe performs one connectivity measurement.
func probe(ctx context.Context, settings *conn.Settings, sequence int, streams *output) cligen.PingResult {
	result := cligen.PingResult{Sequence: sequence, Endpoint: settings.Endpoint()}

	start := time.Now()
	client, err := conn.Dial(ctx, settings, streams.notice(settings.Verbosity))
	if err != nil {
		result.Error = new(err.Error())
		result.Total = new(time.Since(start).Round(time.Millisecond).String())
		return result
	}
	defer client.CloseQuietly(ctx)

	// gopcua opens the channel and activates the session inside Connect, so the
	// two cannot be timed apart from the outside; the combined figure is
	// reported as the session timing and the channel timing is left to the
	// endpoint probe that preceded it.
	connected := time.Now()
	result.Endpoint = client.Endpoint
	result.SecurityPolicy = new(client.Policy)
	result.SecurityMode = new(client.Mode)
	result.Session = new(connected.Sub(start).Round(time.Millisecond).String())

	info, err := opc.ServerInfo(ctx, client, client.Endpoint)
	readDone := time.Now()
	if err != nil {
		result.Error = new(err.Error())
		result.Total = new(readDone.Sub(start).Round(time.Millisecond).String())
		return result
	}

	result.Read = new(readDone.Sub(connected).Round(time.Millisecond).String())
	result.Total = new(readDone.Sub(start).Round(time.Millisecond).String())
	result.ServerTime = info.CurrentTime
	result.Ok = true
	return result
}

func renderProbe(p *render.Printer, result cligen.PingResult) {
	if !result.Ok {
		p.Printf("%s %s %s\n",
			p.T.Styles.Bad.Render(p.T.Symbols.Bad),
			result.Endpoint,
			p.T.Styles.Dim.Render(derefOr(result.Error, "failed")))
		return
	}

	p.Printf("%s %s  session %s  read %s  total %s  %s\n",
		p.T.Styles.Good.Render(p.T.Symbols.Good),
		result.Endpoint,
		p.T.Styles.Value.Render(derefOr(result.Session, "")),
		p.T.Styles.Value.Render(derefOr(result.Read, "")),
		p.T.Styles.Accent.Render(derefOr(result.Total, "")),
		p.T.Styles.Faint.Render(fmt.Sprintf("%s/%s",
			derefOr(result.SecurityPolicy, ""), derefOr(result.SecurityMode, ""))))
}

func statusWord(p *render.Printer, ok bool) string {
	if ok {
		return p.T.Styles.Good.Render("yes")
	}
	return p.T.Styles.Bad.Render("no")
}

// renderMarkdown renders a Markdown document for the terminal, falling back to
// the source text when the renderer cannot be built.
func renderMarkdown(p *render.Printer, document string) error {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(markdownStyle(p)),
		glamour.WithWordWrap(min(p.Width, 100)),
	)
	if err != nil {
		p.Print(document)
		return nil
	}

	rendered, err := renderer.Render(document)
	if err != nil {
		p.Print(document)
		return nil
	}
	p.Print(rendered)
	return nil
}

func markdownStyle(p *render.Printer) string {
	if p.T.Dark {
		return "dark"
	}
	return "light"
}
