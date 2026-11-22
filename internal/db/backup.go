package db

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Backup writes a consistent, WAL-safe SQLite snapshot of dataDir/aumbry.db to outPath using VACUUM INTO.
func Backup(ctx context.Context, dataDir, outPath string) error {
	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		return errors.New("backup output path is required")
	}
	absSrc, err := filepath.Abs(filepath.Join(dataDir, "aumbry.db"))
	if err != nil {
		return err
	}
	absOut, err := filepath.Abs(outPath)
	if err != nil {
		return err
	}
	if absOut == absSrc {
		return errors.New("backup output path must differ from live database")
	}
	// Create the destination file upfront with 0600 (O_CREATE|O_EXCL) so VACUUM INTO writes into
	// an already-restricted file instead of briefly exposing a 0644 snapshot under default umask.
	f, err := os.OpenFile(absOut, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("backup output path %s already exists", outPath)
		}
		return fmt.Errorf("create backup output path %s: %w", outPath, err)
	}
	if closeErr := f.Close(); closeErr != nil {
		_ = os.Remove(absOut)
		return fmt.Errorf("close backup output path %s: %w", outPath, closeErr)
	}
	removeOnErr := true
	defer func() {
		if removeOnErr {
			_ = os.Remove(absOut)
		}
	}()

	database, err := OpenExisting(dataDir)
	if err != nil {
		return err
	}
	defer func() { _ = database.Close() }()

	if _, err := database.ExecContext(ctx, `VACUUM INTO ?`, absOut); err != nil {
		return fmt.Errorf("vacuum into %s: %w", outPath, err)
	}
	if err := os.Chmod(absOut, 0o600); err != nil {
		return fmt.Errorf("chmod backup %s: %w", outPath, err)
	}
	removeOnErr = false
	return nil
}
