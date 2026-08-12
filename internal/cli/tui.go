package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Southclaws/opcuacli/internal/cligen"
	"github.com/Southclaws/opcuacli/internal/tui"
)

// explore opens the full-screen address space explorer.
//
// The connection is made before the program starts so a failure is reported as an
// ordinary command error, with the usual diagnostics, rather than inside a
// full-screen view the user then has to leave.
func explore(ctx context.Context, cmd *cobra.Command, commandIO cligen.IO, p cligen.TuiParams) error {
	session, err := connect(ctx, cmd, commandIO)
	if err != nil {
		return err
	}
	defer session.Close(ctx)

	return tui.Run(ctx, session.Client, tui.Options{
		Node:    p.Node,
		Refresh: p.Refresh,
		Theme:   string(p.Theme),
		Watch:   p.Watch,
	})
}
