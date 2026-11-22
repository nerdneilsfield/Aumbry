package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/client"
)

func newDirsCommand() *cobra.Command {
	var profileName, format string
	command := &cobra.Command{
		Use:   "dirs",
		Short: "List server directories available to the API token",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if format != "json" && format != "tree" {
				return fmt.Errorf("unsupported format %q: use json or tree", format)
			}
			profile, err := resolveProfile(profileName)
			if err != nil {
				return err
			}
			dirs, raw, err := client.ListDirs(command.Context(), profile.Server, profile.Token, nil)
			if err != nil {
				return err
			}
			if format == "json" {
				_, err = command.OutOrStdout().Write(client.Indent(raw))
				return err
			}
			w := command.OutOrStdout()
			for _, d := range dirs {
				depth := strings.Count(d.Path, " / ")
				indent := strings.Repeat("  ", depth)
				status := "public"
				if d.HasPassword {
					status = "password"
				} else if d.Protected {
					status = "protected"
				}
				if _, err := fmt.Fprintf(w, "%s- %s (id=%s, items=%d, %s)\n", indent, d.Name, d.ID, d.ItemCount, status); err != nil {
					return err
				}
			}
			return nil
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	flags.StringVar(&format, "format", "json", "output format: json or tree")
	return command
}
