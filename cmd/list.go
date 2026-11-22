package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/client"
)

func newListCommand() *cobra.Command {
	var profileName, key, query, dir string
	var limit, offset int
	command := &cobra.Command{
		Use:   "list",
		Short: "List items visible to the agent API",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			profile, err := resolveProfile(profileName)
			if err != nil {
				return err
			}
			raw, err := client.ListItems(command.Context(), client.ListItemsRequest{
				Server: profile.Server,
				Token:  profile.Token,
				Key:    key,
				Query:  query,
				Dir:    dir,
				Limit:  limit,
				Offset: offset,
			})
			if err != nil {
				return err
			}
			_, err = command.OutOrStdout().Write(client.Indent(raw))
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	flags.StringVar(&key, "key", "", "exact external_key filter")
	flags.StringVar(&query, "q", "", "filter by title or external_key")
	flags.StringVar(&dir, "dir", "", "filter by mounted directory ID")
	flags.IntVar(&limit, "limit", 0, "maximum items to return")
	flags.IntVar(&offset, "offset", 0, "pagination offset")
	return command
}
