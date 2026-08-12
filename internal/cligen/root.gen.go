package cligen

import (
	"github.com/spf13/cobra"
)

func NewRootCommand(
	discover DiscoverCommand,
	browse BrowseCommand,
	find FindCommand,
	resolve ResolveCommand,
	references ReferencesCommand,
	read ReadCommand,
	write WriteCommand,
	call CallCommand,
	history HistoryCommand,
	attributes AttributesCommand,
	types TypesCommand,
	namespaces NamespacesCommand,
	server ServerCommand,
	ping PingCommand,
	monitor MonitorCommand,
	events EventsCommand,
	tui TuiCommand,
	config ConfigCommand,
	cert CertCommand,
) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "opcua",
		Short: "Explore, query and monitor OPC UA servers from the terminal.",
		Long:  "**opcua** is a terminal client for OPC UA servers.\n\nIt discovers endpoints, walks the address space, reads and writes values,\ncalls methods, reads history, inspects types and metadata, and streams live\nsubscriptions. Connection details live in a config file of named profiles,\nand every profile setting can be overridden with a flag for one-off checks.\n\nRun `opcua tui` for a full-screen address space explorer.\n",
	}
	cmd.PersistentFlags().String("config", "", "Path to the configuration file.")
	cmd.PersistentFlags().StringP("profile", "p", "", "Configuration profile to connect with.")
	cmd.PersistentFlags().StringArrayP("endpoint", "e", nil, "Endpoint URL; repeat to declare a failover order.")
	cmd.PersistentFlags().StringP("username", "u", "", "User name for username/password authentication.")
	cmd.PersistentFlags().String("password", "", "Password for username/password authentication.")
	cmd.PersistentFlags().String("password-file", "", "Read the password from a file, or - for stdin.")
	cmd.PersistentFlags().String("auth", "auto", "Authentication mode: auto, anonymous, username, certificate.")
	cmd.PersistentFlags().String("policy", "auto", "Security policy: auto, None, Basic256Sha256, Aes256Sha256RsaPss, ...")
	cmd.PersistentFlags().String("mode", "auto", "Message security mode: auto, None, Sign, SignAndEncrypt.")
	cmd.PersistentFlags().String("cert", "", "Client certificate in PEM or DER form.")
	cmd.PersistentFlags().String("key", "", "Client private key in PEM or DER form.")
	cmd.PersistentFlags().Bool("insecure", false, "Accept the server certificate without verifying it.")
	cmd.PersistentFlags().Duration("timeout", mustParseDuration("10s"), "Deadline for a single service request.")
	cmd.PersistentFlags().Duration("dial-timeout", mustParseDuration("10s"), "Deadline for establishing the secure channel.")
	cmd.PersistentFlags().Duration("session-timeout", mustParseDuration("20m"), "Requested session lifetime.")
	cmd.PersistentFlags().Bool("no-reconnect", false, "Fail instead of transparently restoring a dropped session.")
	cmd.PersistentFlags().String("app-uri", "", "Application URI announced to the server.")
	cmd.PersistentFlags().StringArray("locale", nil, "Preferred locale for localized text; repeatable, most wanted first.")
	cmd.PersistentFlags().CountP("verbose", "v", "Increase log verbosity (-v, -vv).")
	cmd.PersistentFlags().Bool("trace", false, "Log every OPC UA service call on stderr.")
	cmd.PersistentFlags().Bool("no-color", false, "Disable colour and styling in output.")

	cmd.AddCommand(
		(*cobra.Command)(discover),
		(*cobra.Command)(browse),
		(*cobra.Command)(find),
		(*cobra.Command)(resolve),
		(*cobra.Command)(references),
		(*cobra.Command)(read),
		(*cobra.Command)(write),
		(*cobra.Command)(call),
		(*cobra.Command)(history),
		(*cobra.Command)(attributes),
		(*cobra.Command)(types),
		(*cobra.Command)(namespaces),
		(*cobra.Command)(server),
		(*cobra.Command)(ping),
		(*cobra.Command)(monitor),
		(*cobra.Command)(events),
		(*cobra.Command)(tui),
		(*cobra.Command)(config),
		(*cobra.Command)(cert),
	)

	return cmd
}
