package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

func newValidateCommand() *cobra.Command {
	var format string
	command := &cobra.Command{
		Use:   "validate <file.xml>",
		Short: "Validate an Aumbry XML document",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if format != "json" && format != "text" {
				return fmt.Errorf("unsupported format %q: use json or text", format)
			}
			result, err := schema.ValidateFile(args[0])
			if err != nil {
				return err
			}
			if format == "json" {
				encoder := json.NewEncoder(command.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(result); err != nil {
					return err
				}
			} else if result.Valid {
				_, err = fmt.Fprintln(command.OutOrStdout(), "valid:", result.File)
			} else {
				for _, issue := range result.Errors {
					_, err = fmt.Fprintf(command.OutOrStdout(), "%s: %s: %s\n", issue.Code, issue.Field, issue.Message)
					if err != nil {
						return err
					}
				}
			}
			if err != nil {
				return err
			}
			if !result.Valid {
				return &validationFailedError{}
			}
			return nil
		},
	}
	command.Flags().StringVar(&format, "format", "json", "output format: json or text")
	return command
}

type validationFailedError struct{}

func (*validationFailedError) Error() string { return "document validation failed" }
