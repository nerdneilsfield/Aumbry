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
	if _, statErr := os.Stat(absOut); statErr == nil {
		return fmt.Errorf("backup output path %s already exists", outPath)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat backup output path %s: %w", outPath, statErr)
	}

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
	return nil
}
