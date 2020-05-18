package server

import (
	"github.com/spf13/cobra"

	"ken/monitor"
	"ken/server"
)

func newStartCommand() *cobra.Command {
	opt := options{}
	cmd := &cobra.Command{
		Use:   "start",
		Short: "server start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return start(&opt, args)
		},
	}

	return cmd
}

func start(opt *options, args []string) (err error) {
	go monitor.Start()
	return server.Start()
}
