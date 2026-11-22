package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ErrDataDirLocked indicates another process (such as a running aumbry-server) holds the data_dir lock.
var ErrDataDirLocked = errors.New("data directory is locked by another process")

const lockFileName = "aumbry.lock"

// DirLock holds an exclusive non-blocking advisory lock on dataDir/aumbry.lock.
type DirLock struct {
	fl *flock.Flock
}

// TryLockDataDir attempts to acquire an exclusive non-blocking lock on dataDir/aumbry.lock.
func TryLockDataDir(dataDir string) (*DirLock, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	fl := flock.New(filepath.Join(dataDir, lockFileName))
	locked, err := fl.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock data directory: %w", err)
	}
	if !locked {
		return nil, ErrDataDirLocked
	}
	return &DirLock{fl: fl}, nil
}

// Close releases the advisory lock and closes the lock file.
func (l *DirLock) Close() error {
	if l == nil || l.fl == nil {
		return nil
	}
	err := l.fl.Close()
	l.fl = nil
	return err
}
