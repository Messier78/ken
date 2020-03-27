package config

import "github.com/spf13/cobra"

type options struct {
}

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Config",
	}

	cmd.AddCommand(
		newReloadCommand(),
	)
	return cmd
}
