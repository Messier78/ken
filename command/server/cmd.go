package server

import "github.com/spf13/cobra"

type options struct {
}

// NewServiceCommand ...
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage server",
	}

	cmd.AddCommand(
		newStartCommand(),
	)

	return cmd
}
