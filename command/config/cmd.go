package config

import "github.com/spf13/cobra"

type options struct {
}

func NewConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage Config",
	}

	cmd.AddCommand(
		newReloadCommand(),
	)
	return cmd
}
