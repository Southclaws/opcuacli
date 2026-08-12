// Package cli implements the handlers behind the generated command tree. Each
// handler parses nothing and validates nothing - the generated constructors have
// already done both - so it is only concerned with talking to a server and
// rendering the result.
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gopcua/opcua/ua"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/config"
	"github.com/Southclaws/opcuacli/internal/conn"
	"github.com/Southclaws/opcuacli/internal/opc"
	"github.com/Southclaws/opcuacli/internal/render"
)

// Sentinel errors let the root command map a failure to the exit code the
// specification documents, without every handler knowing the numbers.
var (
	// ErrConnection reports that no configured endpoint could be reached.
	ErrConnection = errors.New("connection failed")
	// ErrService reports that the server returned a bad status code.
	ErrService = errors.New("service fault")
	// ErrNotFound reports that a node, path or profile does not exist.
	ErrNotFound = errors.New("not found")
)

// output holds the two streams a command writes to: results on stdout, progress
// and warnings on stderr. Keeping them apart is what makes `opcua read --format
// json | jq` work while still reporting a failover on the way.
type output struct {
	Out *render.Printer
	Err *render.Printer
}

func newOutput(cmd *cobra.Command, commandIO cligen.IO) *output {
	noColor, _ := cmd.Flags().GetBool("no-color")
	options := render.Options{NoColor: noColor}

	return &output{
		Out: render.NewPrinter(commandIO.Out, options),
		Err: render.NewPrinter(commandIO.Err, options),
	}
}

// notice returns a conn.Notice that reports connection progress on stderr, at
// the verbosity the user asked for. Without -v only the things a user must know
// about are printed: a generated certificate, and a failover.
func (o *output) notice(verbosity int) conn.Notice {
	return func(format string, args ...any) {
		message := fmt.Sprintf(format, args...)
		if verbosity == 0 && !mustSee(message) {
			return
		}
		o.Err.Warnf("%s", message)
	}
}

// mustSee reports whether a connection notice matters even without -v.
func mustSee(message string) bool {
	for _, significant := range []string{"generated a client certificate", "failing over", "refused us"} {
		if strings.Contains(message, significant) {
			return true
		}
	}
	return false
}

// session is a connected client with the streams and settings that produced it.
type session struct {
	*output

	Client   *conn.Client
	Settings *conn.Settings
	Config   *config.File
}

// connect resolves the connection settings and dials the server. Every command
// that talks to a server starts here, which is what makes the connection flags
// behave identically across all of them.
func connect(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO) (*session, error) {
	streams := newOutput(cmd, commandIO)

	settings, file, err := conn.FromCommand(cmd, commandIO.In)
	if err != nil {
		if errors.Is(err, config.ErrNoProfile) {
			return nil, fmt.Errorf("%w: %w", ErrNotFound, err)
		}
		return nil, err
	}

	if settings.Verbosity > 0 {
		streams.Err.Notef("connecting to %s", strings.Join(settings.Endpoints, ", "))
	}

	client, err := conn.Dial(ctx, settings, streams.notice(settings.Verbosity))
	if err != nil {
		var dialErr *conn.DialError
		if errors.As(err, &dialErr) {
			return nil, fmt.Errorf("%w: %w", ErrConnection, err)
		}
		return nil, err
	}

	if settings.Verbosity > 0 {
		streams.Err.Notef("connected to %s (%s)", client.Endpoint, client.Security())
	}

	return &session{output: streams, Client: client, Settings: settings, Config: file}, nil
}

// Close ends the session.
func (s *session) Close(ctx context.Context) {
	s.Client.CloseQuietly(ctx)
}

// node parses a node id argument, reporting the accepted spellings on failure.
func node(text string) (*ua.NodeID, error) {
	parsed, err := opc.ParseNodeID(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %w (try i=2253, ns=2;s=Name, or a name such as Objects)", ErrNotFound, err)
	}
	return parsed, nil
}

// nodes parses a list of node id arguments, optionally extended with ids read
// from stdin one per line. Reading ids from stdin is what lets find and browse
// pipe into read and monitor.
func nodes(texts []string, fromStdin bool, in io.Reader) ([]*ua.NodeID, error) {
	if fromStdin {
		lines, err := readLines(in)
		if err != nil {
			return nil, err
		}
		texts = append(texts, lines...)
	}
	if len(texts) == 0 {
		return nil, fmt.Errorf("no nodes given")
	}

	parsed := make([]*ua.NodeID, 0, len(texts))
	for _, text := range texts {
		one, err := node(text)
		if err != nil {
			return nil, err
		}
		parsed = append(parsed, one)
	}
	return parsed, nil
}

// readLines reads whitespace-separated identifiers from a stream. Splitting on
// fields rather than lines means a tab-separated column from `--format plain`
// works as well as one id per line.
func readLines(in io.Reader) ([]string, error) {
	if in == nil {
		return nil, nil
	}

	var found []string
	scanner := bufio.NewScanner(in)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// A plain-format row is tab separated with the node id first, so the
		// first field is the identifier and the rest is context.
		found = append(found, strings.Fields(line)[0])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return found, nil
}

// endpointOverride applies a discovery URL given as a positional argument, which
// discovery commands accept so an unconfigured server can be probed in one line.
func endpointOverride(settings *conn.Settings, discoveryURL string) {
	if discoveryURL == "" {
		return
	}
	settings.Endpoints = []string{discoveryURL}
}

// machineReadable reports whether a format is one the generated command encodes
// itself, in which case a handler must print nothing and only return its result.
func machineReadable(format string) bool {
	return format == "json" || format == "yaml"
}
