package command

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"ken/command/config"
	"ken/command/server"
)

var rootCmd = &cobra.Command{
	Use:          "ken",
	Short:        "Ultraman Ken",
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(
		server.NewCommand(),
		config.NewCommand(),
		newVersionCommand(),
	)
}
