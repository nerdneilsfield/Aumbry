package cmd

import (
	"encoding/json"
	"errors"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/client"
)

func newDeleteCommand() *cobra.Command {
	var profileName string
	var yes bool
	command := &cobra.Command{
		Use:   "delete <id> --yes",
		Short: "Permanently delete an owned item and its search index entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !yes {
				return errors.New("refusing to delete without --yes confirmation")
			}
			profile, err := resolveProfile(profileName)
			if err != nil {
				return err
			}
			if err := client.DeleteItem(command.Context(), profile.Server, profile.Token, args[0], nil); err != nil {
				return err
			}
			out, err := json.MarshalIndent(map[string]any{"id": args[0], "deleted": true}, "", "  ")
			if err != nil {
				return err
			}
			_, err = command.OutOrStdout().Write(append(out, '\n'))
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	flags.BoolVar(&yes, "yes", false, "confirm permanent deletion")
	return command
}
