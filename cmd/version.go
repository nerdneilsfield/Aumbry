package cmd

import (
	"encoding/json"
	"io"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/buildinfo"
)

func newVersionCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			loggerFrom(command).Debug("printing version")
			if asJSON {
				enc := json.NewEncoder(command.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(buildinfo.Current())
			}
			_, err := io.WriteString(command.OutOrStdout(), buildinfo.Format("aumbry"))
			return err
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output version metadata as JSON")
	return cmd
}
