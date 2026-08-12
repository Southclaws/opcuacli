// Command opcua is a terminal client for OPC UA servers.
//
// The command tree, its flags, its help text and the shape of its output are
// generated from opencli.yaml by OpenCLI; this package only wires the generated
// tree to its handlers and hands the exit code back to the shell.
package main

import (
	"context"
	"os"

	"github.com/Southclaws/opcuacli/internal/cli"
)

func main() {
	os.Exit(cli.Execute(context.Background()))
}
