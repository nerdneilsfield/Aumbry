package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loggerFrom(command).Debug("printing version")
			_, err := fmt.Fprintf(command.OutOrStdout(),
				"aumbry %s\ncommit: %s\nbuilt: %s\ngo: %s %s/%s\n",
				buildinfo.Version, buildinfo.Commit, buildinfo.Date,
				runtime.Version(), runtime.GOOS, runtime.GOARCH)
			return err
		},
	}
}
