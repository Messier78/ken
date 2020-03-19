package config

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newReloadCommand() *cobra.Command {
	opt := options{}
	cmd := &cobra.Command{
		Use:   "reload",
		Short: "reload config with modules",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reload(&opt, args)
		},
	}
	return cmd
}

func reload(opt *options, args []string) (err error) {
	fmt.Println("reload ", args)
	return nil
}
