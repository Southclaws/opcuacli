package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/ua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/pki"
	"github.com/Southclaws/opcuacli/internal/render"
)

// discoverEndpoints lists what a server offers, without opening a session.
//
// GetEndpoints needs no credentials and no security, which makes it the first
// thing to try when a connection is refused: it shows whether the server is
// reachable, what security it demands, and which user tokens it will accept.
func discoverEndpoints(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.DiscoverEndpointsParams) (cligen.EndpointList, error) {
	streams := newOutput(cmd, commandIO)

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return nil, err
	}
	endpointOverride(settings, p.DiscoveryUrl)

	descriptions, err := opcua.GetEndpoints(ctx, settings.Endpoint(), opcua.DialTimeout(settings.DialTimeout))
	if err != nil {
		return nil, fmt.Errorf("%w: get endpoints from %s: %w", ErrConnection, settings.Endpoint(), err)
	}

	endpoints := make(cligen.EndpointList, 0, len(descriptions))
	for _, description := range descriptions {
		if description == nil {
			continue
		}
		if p.Transport != "" && !strings.Contains(description.TransportProfileURI, p.Transport) {
			continue
		}
		endpoints = append(endpoints, describeEndpoint(description, p.ShowCertificate))
	}

	format := string(p.Format)
	if machineReadable(format) {
		return endpoints, nil
	}

	switch format {
	case "plain":
		rows := make([][]string, 0, len(endpoints))
		for _, endpoint := range endpoints {
			rows = append(rows, []string{
				endpoint.EndpointURL,
				endpoint.SecurityPolicy,
				endpoint.SecurityMode,
				strconv.Itoa(endpoint.SecurityLevel),
				strings.Join(endpoint.UserTokenTypes, ","),
			})
		}
		streams.Out.Plain(rows)

	default:
		renderEndpointTable(streams.Out, settings.Endpoint(), endpoints, p.ShowCertificate)
	}

	return endpoints, nil
}

func renderEndpointTable(p *render.Printer, discoveryURL string, endpoints cligen.EndpointList, withCertificates bool) {
	p.Title(discoveryURL, fmt.Sprintf("%d endpoint(s)", len(endpoints)))

	rows := make([][]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		rows = append(rows, []string{
			endpoint.EndpointURL,
			endpoint.SecurityPolicy,
			endpoint.SecurityMode,
			strconv.Itoa(endpoint.SecurityLevel),
			strings.Join(endpoint.UserTokenTypes, ", "),
		})
	}
	p.Table([]string{"ENDPOINT", "POLICY", "MODE", "LEVEL", "TOKENS"}, rows)

	if !withCertificates {
		return
	}
	for _, endpoint := range endpoints {
		if endpoint.Certificate == nil {
			continue
		}
		p.Println()
		p.Title(endpoint.EndpointURL, endpoint.SecurityPolicy)
		renderCertificate(p, *endpoint.Certificate)
	}
}

// describeEndpoint converts an endpoint description into the declared output
// shape, resolving the security policy URI and message security mode to the
// short names --policy and --mode accept.
func describeEndpoint(description *ua.EndpointDescription, withCertificate bool) cligen.Endpoint {
	endpoint := cligen.Endpoint{
		EndpointURL:    description.EndpointURL,
		SecurityPolicy: conn.PolicyName(description.SecurityPolicyURI),
		SecurityMode:   conn.ModeName(description.SecurityMode),
		SecurityLevel:  int(description.SecurityLevel),
	}
	if description.SecurityPolicyURI != "" {
		endpoint.SecurityPolicyURI = new(description.SecurityPolicyURI)
	}
	if description.TransportProfileURI != "" {
		endpoint.TransportProfileURI = new(description.TransportProfileURI)
	}

	for _, token := range description.UserIdentityTokens {
		if token == nil {
			continue
		}
		endpoint.UserTokenTypes = append(endpoint.UserTokenTypes, conn.TokenName(token.TokenType))
	}
	// A server that lists no token policies accepts anonymous sessions, and
	// showing an empty column would read as "none accepted".
	if len(endpoint.UserTokenTypes) == 0 {
		endpoint.UserTokenTypes = []string{"Anonymous"}
	}

	if application := description.Server; application != nil {
		if application.ApplicationName != nil && application.ApplicationName.Text != "" {
			endpoint.ApplicationName = new(application.ApplicationName.Text)
		}
		if application.ApplicationURI != "" {
			endpoint.ApplicationURI = new(application.ApplicationURI)
		}
		if application.ProductURI != "" {
			endpoint.ProductURI = new(application.ProductURI)
		}
		endpoint.ApplicationType = new(applicationTypeName(application.ApplicationType))
		if application.GatewayServerURI != "" {
			endpoint.GatewayServerURI = new(application.GatewayServerURI)
		}
		endpoint.DiscoveryUrls = application.DiscoveryURLs
	}

	if withCertificate && len(description.ServerCertificate) > 0 {
		if info, err := pki.Describe(description.ServerCertificate, ""); err == nil {
			endpoint.Certificate = &info
		}
	}

	return endpoint
}

// discoverServers lists the servers registered with a discovery endpoint.
func discoverServers(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.DiscoverServersParams) (cligen.ServerList, error) {
	streams := newOutput(cmd, commandIO)

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return nil, err
	}
	endpointOverride(settings, p.DiscoveryUrl)

	applications, err := opcua.FindServers(ctx, settings.Endpoint(), opcua.DialTimeout(settings.DialTimeout))
	if err != nil {
		return nil, fmt.Errorf("%w: find servers on %s: %w", ErrConnection, settings.Endpoint(), err)
	}

	servers := make(cligen.ServerList, 0, len(applications))
	for _, application := range applications {
		if application == nil {
			continue
		}
		if len(p.Uri) > 0 && !matchesAny(application.ApplicationURI, p.Uri) {
			continue
		}

		server := cligen.Server{
			ApplicationURI:  application.ApplicationURI,
			ApplicationType: applicationTypeName(application.ApplicationType),
			DiscoveryUrls:   application.DiscoveryURLs,
		}
		if application.ApplicationName != nil && application.ApplicationName.Text != "" {
			server.ApplicationName = new(application.ApplicationName.Text)
		}
		if application.ProductURI != "" {
			server.ProductURI = new(application.ProductURI)
		}
		if application.GatewayServerURI != "" {
			server.GatewayServerURI = new(application.GatewayServerURI)
		}
		if application.DiscoveryProfileURI != "" {
			server.DiscoveryProfileURI = new(application.DiscoveryProfileURI)
		}
		servers = append(servers, server)
	}

	format := string(p.Format)
	if machineReadable(format) {
		return servers, nil
	}

	rows := make([][]string, 0, len(servers))
	for _, server := range servers {
		rows = append(rows, []string{
			derefOr(server.ApplicationName, "-"),
			server.ApplicationType,
			server.ApplicationURI,
			strings.Join(server.DiscoveryUrls, " "),
		})
	}

	if format == "plain" {
		streams.Out.Plain(rows)
		return servers, nil
	}

	streams.Out.Title(settings.Endpoint(), fmt.Sprintf("%d server(s)", len(servers)))
	streams.Out.Table([]string{"NAME", "TYPE", "APPLICATION URI", "DISCOVERY URLS"}, rows)
	return servers, nil
}

// discoverNetwork lists servers a discovery server has learnt about from
// multicast announcements.
func discoverNetwork(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.DiscoverNetworkParams) (cligen.NetworkServerList, error) {
	streams := newOutput(cmd, commandIO)

	settings, _, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		return nil, err
	}
	endpointOverride(settings, p.DiscoveryUrl)

	found, err := opcua.FindServersOnNetwork(ctx, settings.Endpoint(), opcua.DialTimeout(settings.DialTimeout))
	if err != nil {
		return nil, fmt.Errorf("%w: find servers on network via %s: %w (only a discovery server answers this)",
			ErrConnection, settings.Endpoint(), err)
	}

	servers := make(cligen.NetworkServerList, 0, len(found))
	for _, entry := range found {
		if entry == nil {
			continue
		}
		if p.StartRecord > 0 && int(entry.RecordID) < p.StartRecord {
			continue
		}
		if len(p.Capability) > 0 && !containsAll(entry.ServerCapabilities, p.Capability) {
			continue
		}
		servers = append(servers, cligen.NetworkServer{
			RecordID:           int(entry.RecordID),
			ServerName:         entry.ServerName,
			DiscoveryURL:       entry.DiscoveryURL,
			ServerCapabilities: entry.ServerCapabilities,
		})
		if p.MaxRecords > 0 && len(servers) >= p.MaxRecords {
			break
		}
	}

	format := string(p.Format)
	if machineReadable(format) {
		return servers, nil
	}

	rows := make([][]string, 0, len(servers))
	for _, server := range servers {
		rows = append(rows, []string{
			strconv.Itoa(server.RecordID),
			server.ServerName,
			server.DiscoveryURL,
			strings.Join(server.ServerCapabilities, ","),
		})
	}

	if format == "plain" {
		streams.Out.Plain(rows)
		return servers, nil
	}

	streams.Out.Title(settings.Endpoint(), fmt.Sprintf("%d server(s) on the network", len(servers)))
	streams.Out.Table([]string{"RECORD", "NAME", "DISCOVERY URL", "CAPABILITIES"}, rows)
	return servers, nil
}

func applicationTypeName(applicationType ua.ApplicationType) string {
	switch applicationType {
	case ua.ApplicationTypeServer:
		return "Server"
	case ua.ApplicationTypeClient:
		return "Client"
	case ua.ApplicationTypeClientAndServer:
		return "ClientAndServer"
	case ua.ApplicationTypeDiscoveryServer:
		return "DiscoveryServer"
	default:
		return "Unknown"
	}
}

func matchesAny(value string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(strings.ToLower(value), strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

func containsAll(values, wanted []string) bool {
	for _, want := range wanted {
		found := false
		for _, value := range values {
			if strings.EqualFold(value, want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func derefOr(value *string, fallback string) string {
	if value == nil || *value == "" {
		return fallback
	}
	return *value
}
