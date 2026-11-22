package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/config"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func newBackupCmd(configPath *string) *cobra.Command {
	var outPath string
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Create a consistent WAL-safe SQLite snapshot with VACUUM INTO",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := loadAndWarn(cmd, *configPath)
			if err != nil {
				return err
			}
			if err := db.Backup(cmd.Context(), res.Config.DataDir, outPath); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "backup written to %s\n", outPath)
			return err
		},
	}
	cmd.Flags().StringVar(&outPath, "out", "", "destination file path for the SQLite snapshot")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func newVerifyStorageCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "verify-storage",
		Short: "Verify that every XML and asset object referenced in SQLite exists in WebDAV",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := loadAndWarn(cmd, *configPath)
			if err != nil {
				return err
			}
			database, err := db.OpenExisting(res.Config.DataDir)
			if err != nil {
				return err
			}
			defer func() { _ = database.Close() }()

			store, err := storage.NewWebDAV(res.Config.WebDAV.URL, res.Config.WebDAV.User, res.Config.WebDAV.Password)
			if err != nil {
				return fmt.Errorf("configure WebDAV storage: %w", err)
			}
			return verifyStorage(cmd.Context(), database, store, cmd.OutOrStdout())
		},
	}
}

func newGCCmd(configPath *string) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove unreferenced XML and asset objects (destructive runs require the server to be stopped)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := loadAndWarn(cmd, *configPath)
			if err != nil {
				return err
			}
			if !dryRun {
				if _, statErr := os.Stat(filepath.Join(res.Config.DataDir, "aumbry.db")); statErr != nil {
					return fmt.Errorf("database at %s: %w", filepath.Join(res.Config.DataDir, "aumbry.db"), statErr)
				}
				lock, lockErr := db.TryLockDataDir(res.Config.DataDir)
				if lockErr != nil {
					if errors.Is(lockErr, db.ErrDataDirLocked) {
						return errors.New("refusing to run destructive gc while aumbry-server is running; stop the server first or pass --dry-run")
					}
					return lockErr
				}
				defer func() { _ = lock.Close() }()
			}

			database, err := db.OpenExisting(res.Config.DataDir)
			if err != nil {
				return err
			}
			defer func() { _ = database.Close() }()

			store, err := storage.NewWebDAV(res.Config.WebDAV.URL, res.Config.WebDAV.User, res.Config.WebDAV.Password)
			if err != nil {
				return fmt.Errorf("configure WebDAV storage: %w", err)
			}
			return runGC(cmd.Context(), database, store, dryRun, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "list orphan objects without deleting anything (safe while server runs)")
	return cmd
}

func loadAndWarn(cmd *cobra.Command, configPath string) (config.LoadResult, error) {
	res, err := config.Load(configPath)
	if err != nil {
		return config.LoadResult{}, err
	}
	for _, w := range res.Warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w)
	}
	return res, nil
}

func verifyStorage(ctx context.Context, database *sqlx.DB, store storage.Store, out io.Writer) error {
	var items []struct {
		ID      string `db:"id"`
		XMLPath string `db:"xml_storage_path"`
	}
	if err := database.SelectContext(ctx, &items, `SELECT id, xml_storage_path FROM items ORDER BY id`); err != nil {
		return fmt.Errorf("query items: %w", err)
	}
	var assets []struct {
		Hash        string `db:"hash"`
		StoragePath string `db:"storage_path"`
	}
	if err := database.SelectContext(ctx, &assets, `SELECT hash, storage_path FROM assets ORDER BY storage_path`); err != nil {
		return fmt.Errorf("query assets: %w", err)
	}

	var missing []string
	for _, item := range items {
		ok, err := store.Exists(ctx, item.XMLPath)
		if err != nil {
			return fmt.Errorf("check xml %s for item %s: %w", item.XMLPath, item.ID, err)
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("missing xml (item %s): %s", item.ID, item.XMLPath))
		}
	}
	for _, asset := range assets {
		ok, err := store.Exists(ctx, asset.StoragePath)
		if err != nil {
			return fmt.Errorf("check asset %s (%s): %w", asset.StoragePath, asset.Hash, err)
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("missing asset (%s): %s", asset.Hash, asset.StoragePath))
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("storage verification failed: %d missing object(s):\n%s", len(missing), strings.Join(missing, "\n"))
	}
	_, err := fmt.Fprintf(out, "storage OK: verified %d xml and %d asset objects\n", len(items), len(assets))
	return err
}

type orphanAssetRow struct {
	Hash        string `db:"hash"`
	StoragePath string `db:"storage_path"`
}

func runGC(ctx context.Context, database *sqlx.DB, store storage.Store, dryRun bool, out io.Writer) error {
	liveXML, err := selectPathSet(ctx, database, `SELECT xml_storage_path FROM items`)
	if err != nil {
		return fmt.Errorf("query live xml paths: %w", err)
	}
	liveAssets, err := selectPathSet(ctx, database, `SELECT DISTINCT a.storage_path FROM assets a JOIN item_assets ia ON ia.asset_hash = a.hash`)
	if err != nil {
		return fmt.Errorf("query live asset paths: %w", err)
	}
	knownAssets, err := selectPathSet(ctx, database, `SELECT storage_path FROM assets`)
	if err != nil {
		return fmt.Errorf("query known asset paths: %w", err)
	}

	var orphanDBAssets []orphanAssetRow
	if err := database.SelectContext(ctx, &orphanDBAssets,
		`SELECT a.hash, a.storage_path FROM assets a WHERE NOT EXISTS (SELECT 1 FROM item_assets ia WHERE ia.asset_hash = a.hash) ORDER BY a.hash`); err != nil {
		return fmt.Errorf("query orphan asset rows: %w", err)
	}

	remoteXML, err := listPrefix(ctx, store, "/xml")
	if err != nil {
		return err
	}
	remoteAssets, err := listPrefix(ctx, store, "/assets")
	if err != nil {
		return err
	}

	orphanRemoteXML := diffPaths(remoteXML, liveXML)
	orphanRemoteAssets := diffPaths(remoteAssets, liveAssets)

	if dryRun {
		remoteSet := make(map[string]bool, len(orphanRemoteAssets))
		for _, p := range orphanRemoteXML {
			fmt.Fprintf(out, "orphan xml: %s\n", p)
		}
		for _, p := range orphanRemoteAssets {
			remoteSet[p] = true
			kind := "untracked"
			if knownAssets[p] {
				kind = "unreferenced"
			}
			fmt.Fprintf(out, "orphan asset (%s): %s\n", kind, p)
		}
		danglingRows := 0
		for _, row := range orphanDBAssets {
			if !remoteSet[row.StoragePath] {
				fmt.Fprintf(out, "orphan db asset row (missing in storage): %s (%s)\n", row.Hash, row.StoragePath)
				danglingRows++
			}
		}
		_, err = fmt.Fprintf(out, "dry-run: %d orphan xml, %d orphan remote assets, %d orphan db asset rows (%d db-only)\n",
			len(orphanRemoteXML), len(orphanRemoteAssets), len(orphanDBAssets), danglingRows)
		return err
	}

	// Delete remote objects first, then remove DB rows in a transaction (PRD 10.6).
	failed := map[string]error{}
	var deleteErrs []error
	deletedXML := 0
	for _, p := range orphanRemoteXML {
		if delErr := store.Delete(ctx, p); delErr != nil {
			failed[p] = delErr
			deleteErrs = append(deleteErrs, fmt.Errorf("delete %s: %w", p, delErr))
			continue
		}
		deletedXML++
	}
	deletedAssets := 0
	for _, p := range orphanRemoteAssets {
		if delErr := store.Delete(ctx, p); delErr != nil {
			failed[p] = delErr
			deleteErrs = append(deleteErrs, fmt.Errorf("delete %s: %w", p, delErr))
			continue
		}
		deletedAssets++
	}

	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Join(append(deleteErrs, fmt.Errorf("begin gc transaction: %w", err))...)
	}
	defer func() { _ = tx.Rollback() }()

	deletedRows := 0
	for _, row := range orphanDBAssets {
		if failed[row.StoragePath] != nil {
			continue
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM assets WHERE hash = ? AND NOT EXISTS (SELECT 1 FROM item_assets WHERE asset_hash = ?)`, row.Hash, row.Hash)
		if err != nil {
			return errors.Join(append(deleteErrs, fmt.Errorf("delete asset row %s: %w", row.Hash, err))...)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			deletedRows++
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(append(deleteErrs, fmt.Errorf("commit gc transaction: %w", err))...)
	}

	if _, err := fmt.Fprintf(out, "gc complete: deleted %d xml, %d remote assets, %d db asset rows\n",
		deletedXML, deletedAssets, deletedRows); err != nil {
		return err
	}
	return errors.Join(deleteErrs...)
}

func selectPathSet(ctx context.Context, database *sqlx.DB, query string) (map[string]bool, error) {
	var paths []string
	if err := database.SelectContext(ctx, &paths, query); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[p] = true
	}
	return set, nil
}

func listPrefix(ctx context.Context, store storage.Store, dir string) ([]string, error) {
	paths, err := store.List(ctx, dir)
	if errors.Is(err, storage.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	return paths, nil
}

func diffPaths(remote []string, live map[string]bool) []string {
	var out []string
	for _, p := range remote {
		if !live[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}
