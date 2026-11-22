package cmd

import (
	"encoding/json"
	"fmt"
	"io"

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
			if err := writeValidation(command.OutOrStdout(), result, format); err != nil {
				return err
			}
			if !result.Valid {
				return &ValidationFailedError{}
			}
			return nil
		},
	}
	command.Flags().StringVar(&format, "format", "json", "output format: json or text")
	return command
}

// writeValidation prints a validation result in the requested format.
func writeValidation(w io.Writer, result schema.ValidationResult, format string) error {
	if format == "json" {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result)
	}
	if result.Valid {
		_, err := fmt.Fprintln(w, "valid:", result.File)
		return err
	}
	for _, issue := range result.Errors {
		if _, err := fmt.Fprintf(w, "%s: %s: %s\n", issue.Code, issue.Field, issue.Message); err != nil {
			return err
		}
		if issue.Hint != "" {
			if _, err := fmt.Fprintf(w, "  hint: %s\n", issue.Hint); err != nil {
				return err
			}
		}
	}
	return nil
}

// ValidationFailedError reports that the document was invalid. The details have
// already been printed to stdout, so callers should only set the exit status.
type ValidationFailedError struct{}

func (*ValidationFailedError) Error() string { return "document validation failed" }
