package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

func newSchemaCommand() *cobra.Command {
	var format string
	command := &cobra.Command{
		Use:   "schema",
		Short: "Print the supported Aumbry XML protocol specification",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			switch format {
			case "md":
				_, err := fmt.Fprint(command.OutOrStdout(), schema.RenderSchemaMarkdown())
				return err
			case "json":
				data, err := schema.RenderSchemaJSON()
				if err != nil {
					return err
				}
				_, err = command.OutOrStdout().Write(append(data, '\n'))
				return err
			default:
				return fmt.Errorf("unsupported format %q: use md or json", format)
			}
		},
	}
	command.Flags().StringVar(&format, "format", "md", "output format: md or json")
	return command
}
