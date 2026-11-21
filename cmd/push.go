package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/nerdneilsfield/aumbry/internal/cliconfig"
	"github.com/nerdneilsfield/aumbry/internal/client"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

func newPushCommand() *cobra.Command {
	var profileName, itemID, externalKey string
	var dirs []string
	command := &cobra.Command{
		Use:   "push <file.xml>",
		Short: "Validate and upload a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			file := args[0]
			input, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("read XML file %q: %w", file, err)
			}
			dir := filepath.Dir(file)
			doc, issues := schema.Parse(input, schema.Options{HasAsset: schema.DirAssetChecker(dir)})
			if len(issues) > 0 {
				result := schema.ValidationResult{File: file, Errors: issues}
				if err := writeValidation(command.OutOrStdout(), result, "json"); err != nil {
					return err
				}
				return &ValidationFailedError{}
			}

			cfg, err := cliconfig.Load()
			if err != nil {
				return err
			}
			_, profile, err := cfg.Resolve(profileName)
			if err != nil {
				return err
			}
			if profile.Server == "" {
				return fmt.Errorf("no server configured: run `aumbry config set --server URL` or set %s", cliconfig.EnvServer)
			}
			if profile.Token == "" {
				return fmt.Errorf("no token configured: run `aumbry config set --token TOKEN` or set %s", cliconfig.EnvToken)
			}
			if len(dirs) == 0 {
				dirs = profile.DefaultDirs
			}

			assets := make([]client.Asset, len(doc.Assets))
			for i, src := range doc.Assets {
				assets[i] = client.Asset{Src: src, Path: filepath.Join(dir, filepath.FromSlash(src))}
			}
			loggerFrom(command).Debug("pushing document", zap.String("server", profile.Server), zap.Int("assets", len(assets)), zap.Strings("dirs", dirs))
			body, err := client.Push(command.Context(), client.PushRequest{
				Server:      profile.Server,
				Token:       profile.Token,
				XML:         input,
				Assets:      assets,
				Dirs:        dirs,
				ItemID:      itemID,
				ExternalKey: externalKey,
			})
			var rejected *client.ValidationRejected
			if errors.As(err, &rejected) {
				if _, werr := command.OutOrStdout().Write(client.Indent(rejected.Body)); werr != nil {
					return werr
				}
				return &ValidationFailedError{}
			}
			if err != nil {
				return err
			}
			_, err = command.OutOrStdout().Write(client.Indent(body))
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	flags.StringArrayVar(&dirs, "dir", nil, "directory ID to mount the item in (repeatable; overrides default_dirs)")
	flags.StringVar(&itemID, "id", "", "update this existing item ID")
	flags.StringVar(&externalKey, "key", "", "idempotency key: upsert the item with this external_key")
	return command
}
