package cmd

import (
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/client"
)

func newUnpublishCommand() *cobra.Command {
	var profileName string
	command := &cobra.Command{
		Use:   "unpublish <id>",
		Short: "Clear all directory mounts for an owned item (turns it back into a draft)",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			profile, err := resolveProfile(profileName)
			if err != nil {
				return err
			}
			if err := client.Unpublish(command.Context(), profile.Server, profile.Token, args[0], nil); err != nil {
				return err
			}
			out, err := json.MarshalIndent(map[string]any{"id": args[0], "published": false}, "", "  ")
			if err != nil {
				return err
			}
			_, err = command.OutOrStdout().Write(append(out, '\n'))
			return err
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	return command
}
