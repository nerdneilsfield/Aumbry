package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/nerdneilsfield/aumbry/internal/client"
	"github.com/nerdneilsfield/aumbry/pkg/schema"
)

func newPushCommand() *cobra.Command {
	var profileName, itemID, externalKey string
	var dirs []string
	var draft, createOnly, dryRun bool
	command := &cobra.Command{
		Use:   "push <file.xml>",
		Short: "Validate and upload a document",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			flags := command.Flags()
			if draft && flags.Changed("dir") {
				return errors.New("--draft cannot be used with --dir")
			}
			if createOnly && (flags.Changed("id") || itemID != "") {
				return errors.New("--create-only cannot be used with --id")
			}
			if createOnly && strings.TrimSpace(externalKey) == "" {
				return errors.New("--create-only requires --key")
			}

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

			profile, err := resolveProfile(profileName)
			if err != nil {
				return err
			}
			if draft {
				dirs = nil
			} else if len(dirs) == 0 {
				dirs = profile.DefaultDirs
			}

			if dryRun {
				return runPushDryRun(command, profile.Server, profile.Token, file, doc.Meta.Type, len(doc.Assets), dirs, draft, createOnly)
			}

			if draft || createOnly {
				if _, _, err := client.ListDirs(command.Context(), profile.Server, profile.Token, nil); err != nil {
					var apiErr *client.APIError
					if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
						return fmt.Errorf("server does not support --draft/--create-only (/api/v1/agent/dirs returned 404): upgrade server to v1.1")
					}
					return err
				}
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
				AssetDir:    dir,
				Assets:      assets,
				Dirs:        dirs,
				ItemID:      itemID,
				ExternalKey: externalKey,
				Draft:       draft,
				CreateOnly:  createOnly,
			})
			var rejected *client.ValidationRejected
			if errors.As(err, &rejected) {
				if _, werr := command.OutOrStdout().Write(client.Indent(rejected.Body)); werr != nil {
					return werr
				}
				if strings.Contains(string(rejected.Body), "UNSUPPORTED_VERSION") {
					_, _ = fmt.Fprintln(command.ErrOrStderr(), "hint: server rejected document version; upgrade the Aumbry server to v1.1")
				}
				return &ValidationFailedError{}
			}
			var apiErr *client.APIError
			if errors.As(err, &apiErr) && apiErr.Code == "already_published" {
				return fmt.Errorf("cannot push --draft onto an already published item: run `aumbry unpublish <id>` first or push without --draft (%w)", apiErr)
			}
			if err != nil {
				return err
			}
			_, err = command.OutOrStdout().Write(augmentPushResponse(body, profile.Server, draft))
			return err
		},
	}
	flags := command.Flags()
	flags.StringVar(&profileName, "profile", "", "configuration profile (default: the active profile)")
	flags.StringArrayVar(&dirs, "dir", nil, "directory ID to mount the item in (repeatable; overrides default_dirs)")
	flags.StringVar(&itemID, "id", "", "update this existing item ID")
	flags.StringVar(&externalKey, "key", "", "idempotency key: upsert the item with this external_key")
	flags.BoolVar(&draft, "draft", false, "upload as unmounted draft (ignores default_dirs/AUMBRY_DIRS)")
	flags.BoolVar(&createOnly, "create-only", false, "fail with 409 if external_key already exists")
	flags.BoolVar(&dryRun, "dry-run", false, "validate locally and verify server, token, and directories without uploading")
	return command
}

func runPushDryRun(command *cobra.Command, server, token, file, docType string, assetCount int, dirs []string, draft, createOnly bool) error {
	agentDirs, _, err := client.ListDirs(command.Context(), server, token, nil)
	if err != nil {
		return err
	}
	if !draft && len(dirs) > 0 {
		known := make(map[string]bool, len(agentDirs))
		for _, d := range agentDirs {
			known[d.ID] = true
		}
		var missing []string
		for _, d := range dirs {
			if !known[d] {
				missing = append(missing, d)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("unknown directories: %s", strings.Join(missing, ", "))
		}
	}
	if dirs == nil {
		dirs = []string{}
	}
	out, err := json.MarshalIndent(map[string]any{
		"dry_run":     true,
		"valid":       true,
		"file":        file,
		"type":        docType,
		"assets":      assetCount,
		"dirs":        dirs,
		"draft":       draft,
		"create_only": createOnly,
	}, "", "  ")
	if err != nil {
		return err
	}
	_, err = command.OutOrStdout().Write(append(out, '\n'))
	return err
}

func augmentPushResponse(body []byte, server string, draft bool) []byte {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return client.Indent(body)
	}
	if rel, _ := payload["url"].(string); rel != "" {
		payload["full_url"] = client.FullURL(server, rel)
	}
	if _, ok := payload["draft"]; !ok {
		payload["draft"] = draft
	}
	if _, ok := payload["published"]; !ok {
		dirs, _ := payload["dirs"].([]any)
		payload["published"] = len(dirs) > 0
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return client.Indent(body)
	}
	return append(encoded, '\n')
}
