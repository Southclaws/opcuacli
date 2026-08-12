package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/carapace-sh/carapace"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
)

// Version is the version reported by --version. It is overridden at build time
// with -ldflags "-X github.com/Southclaws/opcuacli/internal/cli.Version=…".
var Version = "0.1.0"

//go:generate opencli generate go-cobra ../../opencli.yaml --output ../cligen --package cligen

// exitCodes maps the sentinel errors to the exit codes the specification
// documents, so a script can tell a connection problem from a bad node id.
var exitCodes = []struct {
	err  error
	code int
}{
	{ErrConnection, 2},
	{ErrService, 3},
	{ErrNotFound, 4},
}

// NewRootCommand assembles the command tree from the generated constructors.
//
// Every command's flags, arguments, help text and output contract come from
// opencli.yaml; the handlers below only implement behaviour. Adding a command
// therefore starts in the specification, not here.
func NewRootCommand() *cobra.Command {
	root := cligen.NewRootCommand(
		cligen.NewDiscoverCommand(discoverEndpoints, discoverServers, discoverNetwork),
		cligen.NewBrowseCommand(browse),
		cligen.NewFindCommand(find),
		cligen.NewResolveCommand(resolve),
		cligen.NewReferencesCommand(references),
		cligen.NewReadCommand(read),
		cligen.NewWriteCommand(write),
		cligen.NewCallCommand(call),
		cligen.NewHistoryCommand(historyRead, historyAt, historyEvents),
		cligen.NewAttributesCommand(attributes),
		cligen.NewTypesCommand(typeGet, typeOf, typeList),
		cligen.NewNamespacesCommand(namespaces),
		cligen.NewServerCommand(serverInfo, serverCapabilities, serverDiagnostics, serverRedundancy),
		cligen.NewPingCommand(ping),
		cligen.NewMonitorCommand(monitor),
		cligen.NewEventsCommand(events),
		cligen.NewTuiCommand(explore),
		cligen.NewConfigCommand(configInit, configShow, configList, configUse, configSet, configRemove, configPath),
		cligen.NewCertCommand(certGenerate, certShow),
	)

	// Handlers report their own errors through fang's error handler, so cobra
	// must not also print usage for a runtime failure.
	root.SilenceUsage = true
	root.SilenceErrors = true

	registerCompletions(root)

	return root
}

// Execute runs the command line.
//
// fang provides the styled help, error and version output, and the manpage and
// completion subcommands; carapace provides the completion values themselves.
// Both are wrappers around the generated cobra tree rather than replacements for
// it.
func Execute(ctx context.Context) int {
	// A streaming command needs an interruptible context: ctrl-c must unwind the
	// subscription and close the session rather than killing the process and
	// leaving the server holding it open.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	root := NewRootCommand()

	err := fang.Execute(ctx, root,
		fang.WithVersion(Version),
		fang.WithColorSchemeFunc(colorScheme),
	)
	if err == nil {
		return 0
	}

	// fang has already reported the error; only the exit code is left to decide.
	for _, mapping := range exitCodes {
		if errors.Is(err, mapping.err) {
			return mapping.code
		}
	}
	return 1
}

// registerCompletions teaches the shell what each flag and argument accepts.
//
// The values come from the same tables the parsers use, so a completion can
// never drift from what a command will actually accept; the profile completion
// reads the configuration file, and the node completion browses the server.
func registerCompletions(root *cobra.Command) {
	carapace.Gen(root).FlagCompletion(carapace.ActionMap{
		"profile":       actionProfiles(),
		"config":        carapace.ActionFiles(".yaml", ".yml"),
		"endpoint":      actionEndpoints(),
		"cert":          carapace.ActionFiles(".pem", ".der", ".crt"),
		"key":           carapace.ActionFiles(".pem", ".der", ".key"),
		"password-file": carapace.ActionFiles(),
		"policy":        actionPolicies(),
		"mode":          carapace.ActionValues("auto", "None", "Sign", "SignAndEncrypt"),
		"auth":          carapace.ActionValues("auto", "anonymous", "username", "certificate"),
		"locale":        carapace.ActionValues("en", "en-GB", "en-US", "de", "fr", "es", "it", "ja", "zh-CN"),
	})

	for _, command := range allCommands(root) {
		registerCommandCompletions(command)
	}
}

// allCommands flattens the command tree.
func allCommands(root *cobra.Command) []*cobra.Command {
	commands := []*cobra.Command{root}
	for _, child := range root.Commands() {
		commands = append(commands, allCommands(child)...)
	}
	return commands
}

// registerCommandCompletions attaches completions for one command, keyed by the
// path a user types. The path rather than the operation id is used because that
// is what identifies a command to a shell.
func registerCommandCompletions(command *cobra.Command) {
	path := command.CommandPath()

	flags := carapace.ActionMap{}
	if command.Flags().Lookup("attribute") != nil {
		flags["attribute"] = carapace.ActionValues(attributeNames()...)
	}
	if command.Flags().Lookup("ref-type") != nil {
		flags["ref-type"] = carapace.ActionValues(referenceTypeNames()...)
	}
	if command.Flags().Lookup("class") != nil {
		flags["class"] = carapace.ActionValues(nodeClassNames()...)
	}
	if command.Flags().Lookup("type") != nil {
		flags["type"] = carapace.ActionValues(writableTypeNames()...)
	}
	if command.Flags().Lookup("root") != nil {
		flags["root"] = actionNodes()
	}
	if command.Flags().Lookup("node") != nil {
		flags["node"] = actionNodes()
	}
	if command.Flags().Lookup("out-dir") != nil {
		flags["out-dir"] = carapace.ActionDirectories()
	}
	if command.Flags().Lookup("event-type") != nil {
		flags["event-type"] = actionNodes()
	}
	if len(flags) > 0 {
		carapace.Gen(command).FlagCompletion(flags)
	}

	switch path {
	case "opcua browse", "opcua ls", "opcua read", "opcua get", "opcua write", "opcua set",
		"opcua monitor", "opcua watch", "opcua attributes", "opcua attrs", "opcua references",
		"opcua refs", "opcua events", "opcua types get", "opcua types of",
		"opcua history read", "opcua history at", "opcua history events", "opcua call":
		carapace.Gen(command).PositionalAnyCompletion(actionNodes())

	case "opcua config use", "opcua config remove", "opcua config rm":
		carapace.Gen(command).PositionalCompletion(actionProfiles())

	case "opcua config set":
		carapace.Gen(command).PositionalCompletion(actionSettings())

	case "opcua cert show":
		carapace.Gen(command).PositionalCompletion(carapace.ActionFiles(".pem", ".der", ".crt"))

	case "opcua discover endpoints", "opcua discover ep", "opcua discover servers", "opcua discover network":
		carapace.Gen(command).PositionalCompletion(actionEndpoints())
	}
}
