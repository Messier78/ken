package command

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	Version   = "unknown"
	ChangeLog = "unknown"
	Built = "unknown"
)

// NewVersionCommand ...
func newVersionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "show version info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf(`Ken
    Author: Dwion
    Version: %s
    ChangeLog: %s
    build with %s, at %s
`, Version, ChangeLog, runtime.Version(), Built)
		},
	}

	return cmd
}
