package conn

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gopcua/opcua"
	"github.com/gopcua/opcua/debug"
	"github.com/gopcua/opcua/ua"

	"github.com/Southclaws/opcuacli/internal/config"
	"github.com/Southclaws/opcuacli/internal/pki"
)

// Client is a connected session together with the endpoint and security it
// settled on, so a command can report what it actually did rather than what was
// asked for.
type Client struct {
	*opcua.Client

	// Endpoint is the URL this session is connected to, which with a failover
	// order is not necessarily the first one configured.
	Endpoint string
	// Policy and Mode are the negotiated security, by short name.
	Policy string
	Mode   string
	// TokenType is the user token the session was activated with.
	TokenType string
	// Settings are the settings this client was dialled with, so a
	// re-connection can be made on the same terms.
	Settings *Settings

	// Description is the endpoint description that was selected, kept for the
	// certificate inspection a command may want to do afterwards.
	Description *ua.EndpointDescription
}

// Notice reports something worth telling the user during connection: a
// certificate that had to be generated, a failover to the next endpoint, or the
// security that negotiation settled on. Commands pass a printer-backed function;
// a nil Notice discards them.
type Notice func(format string, args ...any)

// Dial connects to the first endpoint in the failover order that answers.
//
// Each endpoint is probed with GetEndpoints - an unauthenticated call - so that
// security can be negotiated against what the server actually offers before a
// session is attempted. When several endpoints are configured, a failure moves
// on to the next one and the reason is reported through notice.
func Dial(ctx context.Context, settings *Settings, notice Notice) (*Client, error) {
	if notice == nil {
		notice = func(string, ...any) {}
	}
	if settings.Trace {
		debug.Enable = true
	}

	var failures []string
	for index, endpoint := range settings.Endpoints {
		if index > 0 {
			notice("failing over to %s", endpoint)
		}

		client, err := dialOne(ctx, settings, endpoint, notice)
		if err == nil {
			return client, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		failures = append(failures, fmt.Sprintf("%s: %v", endpoint, err))
	}

	return nil, &DialError{Endpoints: settings.Endpoints, Failures: failures}
}

// DialError reports that no endpoint in the failover order could be reached,
// listing what each one said.
type DialError struct {
	Endpoints []string
	Failures  []string
}

func (e *DialError) Error() string {
	if len(e.Failures) == 1 {
		return "cannot connect to " + e.Failures[0]
	}
	return fmt.Sprintf("cannot connect to any of %d endpoints:\n  %s",
		len(e.Endpoints), strings.Join(e.Failures, "\n  "))
}

func dialOne(ctx context.Context, settings *Settings, endpoint string, notice Notice) (*Client, error) {
	discoverCtx, cancel := context.WithTimeout(ctx, settings.DialTimeout)
	defer cancel()

	options := []opcua.Option{opcua.DialTimeout(settings.DialTimeout)}
	descriptions, err := opcua.GetEndpoints(discoverCtx, endpoint, options...)
	if err != nil {
		return nil, fmt.Errorf("get endpoints: %w", err)
	}

	candidates, err := rank(descriptions, settings)
	if err != nil {
		return nil, err
	}

	var attempts []string
	for _, candidate := range candidates {
		client, err := connect(ctx, settings, endpoint, candidate, notice)
		if err == nil {
			return client, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		attempts = append(attempts, fmt.Sprintf("%s/%s: %v",
			policyName(candidate.description.SecurityPolicyURI), modeName(candidate.description.SecurityMode), err))

		// Only negotiation is allowed to keep trying weaker endpoints; an
		// explicit --policy or --mode must fail loudly instead of silently
		// connecting with less security than was asked for.
		if len(candidates) > 1 {
			notice("%s/%s refused us, trying the next endpoint down",
				policyName(candidate.description.SecurityPolicyURI), modeName(candidate.description.SecurityMode))
		}
	}

	return nil, errors.New(strings.Join(attempts, "; "))
}

// candidate is one endpoint description paired with the user token type the
// session would use on it.
type candidate struct {
	description *ua.EndpointDescription
	tokenType   ua.UserTokenType
	policyID    string
}

// rank chooses which endpoint descriptions to attempt, strongest first.
//
// With an explicit policy or mode there is exactly one acceptable answer. With
// negotiation the list is every endpoint that can carry the requested
// authentication, ordered by the server's own security level, so a hardened
// server is met with security and a wide-open one still connects.
func rank(descriptions []*ua.EndpointDescription, settings *Settings) ([]candidate, error) {
	if len(descriptions) == 0 {
		return nil, errors.New("server offers no endpoints")
	}

	wantPolicy := strings.ToLower(settings.Policy)
	wantMode := strings.ToLower(settings.Mode)
	negotiate := (wantPolicy == "auto" || wantPolicy == "") && (wantMode == "auto" || wantMode == "")

	sorted := make([]*ua.EndpointDescription, len(descriptions))
	copy(sorted, descriptions)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].SecurityLevel > sorted[j].SecurityLevel
	})

	var (
		candidates []candidate
		rejected   []string
	)
	for _, description := range sorted {
		policy := strings.ToLower(policyName(description.SecurityPolicyURI))
		mode := strings.ToLower(modeName(description.SecurityMode))

		if !negotiate {
			if wantPolicy != "auto" && wantPolicy != "" && policy != wantPolicy {
				continue
			}
			if wantMode != "auto" && wantMode != "" && mode != wantMode {
				continue
			}
		}

		tokenType, policyID, err := selectToken(description, settings)
		if err != nil {
			rejected = append(rejected, fmt.Sprintf("%s/%s (%v)",
				policyName(description.SecurityPolicyURI), modeName(description.SecurityMode), err))
			continue
		}

		candidates = append(candidates, candidate{description: description, tokenType: tokenType, policyID: policyID})
		if !negotiate {
			break
		}
	}

	if len(candidates) == 0 {
		available := make([]string, 0, len(sorted))
		for _, description := range sorted {
			available = append(available, fmt.Sprintf("%s/%s",
				policyName(description.SecurityPolicyURI), modeName(description.SecurityMode)))
		}
		if len(rejected) > 0 {
			return nil, fmt.Errorf("no endpoint accepts %s authentication: %s",
				settings.AuthMode, strings.Join(rejected, ", "))
		}
		return nil, fmt.Errorf("no endpoint matches policy %s and mode %s; server offers %s",
			settings.Policy, settings.Mode, strings.Join(available, ", "))
	}

	return candidates, nil
}

// selectToken picks the user token type for an endpoint, honouring an explicit
// --auth and otherwise preferring the strongest identity the settings can prove.
func selectToken(description *ua.EndpointDescription, settings *Settings) (ua.UserTokenType, string, error) {
	offered := map[ua.UserTokenType]string{}
	for _, policy := range description.UserIdentityTokens {
		if policy == nil {
			continue
		}
		if _, seen := offered[policy.TokenType]; !seen {
			offered[policy.TokenType] = policy.PolicyID
		}
	}
	// A server that lists no token policies accepts anonymous sessions.
	if len(offered) == 0 {
		offered[ua.UserTokenTypeAnonymous] = ""
	}

	want := strings.ToLower(settings.AuthMode)
	if want == "auto" || want == "" {
		switch {
		case settings.Username != "":
			want = "username"
		case settings.CertFile != "" && hasToken(offered, ua.UserTokenTypeCertificate) && !hasToken(offered, ua.UserTokenTypeAnonymous):
			// A client certificate is only used as an identity when the server
			// will not take an anonymous session; otherwise the certificate is
			// just the channel's, and anonymous is the lighter choice.
			want = "certificate"
		default:
			want = "anonymous"
		}
	}

	switch want {
	case "anonymous":
		if policyID, ok := offered[ua.UserTokenTypeAnonymous]; ok {
			return ua.UserTokenTypeAnonymous, policyID, nil
		}
		return 0, "", errors.New("endpoint requires credentials")

	case "username":
		policyID, ok := offered[ua.UserTokenTypeUserName]
		if !ok {
			return 0, "", errors.New("endpoint does not accept a user name")
		}
		if settings.Username == "" {
			return 0, "", errors.New("--username is required for username authentication")
		}
		return ua.UserTokenTypeUserName, policyID, nil

	case "certificate":
		policyID, ok := offered[ua.UserTokenTypeCertificate]
		if !ok {
			return 0, "", errors.New("endpoint does not accept a certificate identity")
		}
		return ua.UserTokenTypeCertificate, policyID, nil

	default:
		return 0, "", fmt.Errorf("unknown authentication mode %q (known: auto, anonymous, username, certificate)", settings.AuthMode)
	}
}

func hasToken(offered map[ua.UserTokenType]string, want ua.UserTokenType) bool {
	_, ok := offered[want]
	return ok
}

func connect(ctx context.Context, settings *Settings, endpoint string, chosen candidate, notice Notice) (*Client, error) {
	description := chosen.description
	secure := description.SecurityMode != ua.MessageSecurityModeNone ||
		policyName(description.SecurityPolicyURI) != "None" ||
		chosen.tokenType == ua.UserTokenTypeCertificate

	if !settings.Insecure && len(description.ServerCertificate) > 0 {
		if err := pki.Validate(description.ServerCertificate); err != nil {
			return nil, fmt.Errorf("server certificate rejected: %w (pass --insecure to connect anyway)", err)
		}
	}

	options := []opcua.Option{
		opcua.SecurityFromEndpoint(description, chosen.tokenType),
		opcua.DialTimeout(settings.DialTimeout),
		opcua.RequestTimeout(settings.RequestTimeout),
		opcua.SessionTimeout(settings.SessionTimeout),
		opcua.ApplicationURI(settings.ApplicationURI),
		opcua.ApplicationName("opcuacli"),
		opcua.ProductURI("urn:opcuacli"),
		opcua.AutoReconnect(settings.Reconnect),
		opcua.SessionName(fmt.Sprintf("opcuacli@%s", shortHost(endpoint))),
	}
	if len(settings.Locales) > 0 {
		options = append(options, opcua.Locales(settings.Locales...))
	}

	if secure {
		certFile, keyFile := settings.CertFile, settings.KeyFile
		if certFile == "" || keyFile == "" {
			// Security needs a key pair and there is no useful way to ask for
			// one mid-connection, so provision it in the config directory. It
			// is stable across runs, which matters because the server operator
			// has to approve it once.
			generatedCert, generatedKey, generated, err := pki.Ensure(config.CertDir(), settings.ApplicationURI)
			if err != nil {
				return nil, fmt.Errorf("client certificate: %w", err)
			}
			if generated {
				notice("generated a client certificate in %s; the server may need to trust it", config.CertDir())
			}
			certFile, keyFile = generatedCert, generatedKey
		}
		options = append(options, opcua.CertificateFile(certFile), opcua.PrivateKeyFile(keyFile))
	}

	switch chosen.tokenType {
	case ua.UserTokenTypeUserName:
		options = append(options, opcua.AuthUsername(settings.Username, settings.Password))
	case ua.UserTokenTypeCertificate:
		certificate, err := pki.Load(settings.CertFile)
		if err != nil {
			return nil, fmt.Errorf("read certificate identity: %w", err)
		}
		options = append(options, opcua.AuthCertificate(certificate))
	default:
		options = append(options, opcua.AuthAnonymous())
	}
	if chosen.policyID != "" {
		options = append(options, opcua.AuthPolicyID(chosen.policyID))
	}

	if settings.Verbosity > 1 {
		options = append(options, opcua.StateChangedFunc(func(state opcua.ConnState) {
			notice("connection state: %s", state)
		}))
	}

	// The endpoint URL a server advertises is often the address it knows itself
	// by, which is not necessarily reachable from here - a container's hostname,
	// or an address behind a port forward. Dialling the URL the user gave and
	// carrying the advertised endpoint's security is what makes a forwarded or
	// NAT'd server work at all.
	client, err := opcua.NewClient(endpoint, options...)
	if err != nil {
		return nil, err
	}
	if err := client.Connect(ctx); err != nil {
		return nil, err
	}

	return &Client{
		Client:      client,
		Endpoint:    endpoint,
		Policy:      policyName(description.SecurityPolicyURI),
		Mode:        modeName(description.SecurityMode),
		TokenType:   tokenName(chosen.tokenType),
		Settings:    settings,
		Description: description,
	}, nil
}

// Security is a one-line summary of the negotiated channel, for status lines.
func (c *Client) Security() string {
	return fmt.Sprintf("%s/%s as %s", c.Policy, c.Mode, c.TokenType)
}

// CloseQuietly closes the session, ignoring the error. A command that has
// already produced its output has nothing useful to do with a failure to close,
// and a session the server has already dropped reports one routinely.
func (c *Client) CloseQuietly(ctx context.Context) {
	if c == nil || c.Client == nil {
		return
	}
	// The parent context is usually already cancelled by the time a streaming
	// command unwinds, so closing needs a deadline of its own.
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	_ = c.Client.Close(closeCtx)
}

// policyName is the short name of a security policy URI, e.g. Basic256Sha256.
func policyName(uri string) string {
	if uri == "" {
		return "None"
	}
	if index := strings.LastIndex(uri, "#"); index >= 0 {
		return uri[index+1:]
	}
	if index := strings.LastIndex(uri, "/"); index >= 0 {
		return uri[index+1:]
	}
	return uri
}

// PolicyName is the short name of a security policy URI.
func PolicyName(uri string) string { return policyName(uri) }

// modeName is the readable name of a message security mode.
func modeName(mode ua.MessageSecurityMode) string {
	switch mode {
	case ua.MessageSecurityModeNone:
		return "None"
	case ua.MessageSecurityModeSign:
		return "Sign"
	case ua.MessageSecurityModeSignAndEncrypt:
		return "SignAndEncrypt"
	default:
		return "Invalid"
	}
}

// ModeName is the readable name of a message security mode.
func ModeName(mode ua.MessageSecurityMode) string { return modeName(mode) }

// TokenName is the readable name of a user token type.
func TokenName(token ua.UserTokenType) string { return tokenName(token) }

func tokenName(token ua.UserTokenType) string {
	switch token {
	case ua.UserTokenTypeAnonymous:
		return "Anonymous"
	case ua.UserTokenTypeUserName:
		return "UserName"
	case ua.UserTokenTypeCertificate:
		return "Certificate"
	case ua.UserTokenTypeIssuedToken:
		return "IssuedToken"
	default:
		return fmt.Sprintf("Unknown(%d)", token)
	}
}

// shortHost is the host part of an endpoint URL, for naming the session
// something a server operator can recognise in a session list.
func shortHost(endpoint string) string {
	trimmed := strings.TrimPrefix(endpoint, "opc.tcp://")
	if index := strings.IndexAny(trimmed, ":/"); index > 0 {
		return trimmed[:index]
	}
	return trimmed
}

func defaultApplicationURI() string { return pki.DefaultApplicationURI() }
