package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
)

// LocalOptions controls initialization of a local directory object store.
type LocalOptions struct {
	// CreateRoot creates the storage root directory (0700) when missing.
	// Only the HTTP server enables this; maintenance commands leave it false so
	// misconfigured paths fail instead of appearing empty.
	CreateRoot bool
}

// Local stores objects inside a host directory confined with os.Root.
type Local struct {
	rootPath string
}

// NewLocal opens an existing local storage root directory without creating it.
func NewLocal(rootPath string) (*Local, error) {
	return NewLocalWithOptions(rootPath, LocalOptions{})
}

// NewLocalWithOptions opens a local storage root directory according to opts.
func NewLocalWithOptions(rootPath string, opts LocalOptions) (*Local, error) {
	trimmed := strings.TrimSpace(rootPath)
	if trimmed == "" {
		return nil, errors.New("local storage path is required")
	}
	if opts.CreateRoot {
		if err := os.MkdirAll(trimmed, 0o700); err != nil {
			return nil, fmt.Errorf("create local storage root %s: %w", trimmed, err)
		}
	}
	root, err := os.OpenRoot(trimmed)
	if err != nil {
		return nil, fmt.Errorf("open local storage root %s: %w", trimmed, err)
	}
	if err := root.Close(); err != nil {
		return nil, fmt.Errorf("close local storage root %s: %w", trimmed, err)
	}
	return &Local{rootPath: trimmed}, nil
}

func (l *Local) openRoot() (*os.Root, error) {
	return os.OpenRoot(l.rootPath)
}

// Put writes body atomically via a sibling .tmp-* file, fsync, close, and rename.
func (l *Local) Put(ctx context.Context, name string, body io.ReadSeeker) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if body == nil {
		return errors.New("storage: nil body")
	}
	rel, err := normalizeObjectPath(name)
	if err != nil {
		return err
	}
	root, err := l.openRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	parent := path.Dir(rel)
	if parent != "." {
		if mkErr := root.MkdirAll(parent, 0o700); mkErr != nil {
			return mkErr
		}
	}
	tmpName, err := randomTempName(parent, ".tmp-")
	if err != nil {
		return err
	}
	f, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}

	cleanup := func(cause error) error {
		_ = f.Close()
		_ = root.Remove(tmpName)
		return cause
	}

	if err := copyWithContext(ctx, f, body); err != nil {
		return cleanup(err)
	}
	if err := f.Sync(); err != nil {
		return cleanup(err)
	}
	if err := f.Close(); err != nil {
		_ = root.Remove(tmpName)
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = root.Remove(tmpName)
		return err
	}
	if err := root.Rename(tmpName, rel); err != nil {
		_ = root.Remove(tmpName)
		return err
	}
	return nil
}

// Open opens name for reading, returning ErrNotFound before returning a stream if missing.
func (l *Local) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rel, err := normalizeObjectPath(name)
	if err != nil {
		return nil, err
	}
	root, err := l.openRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	info, statErr := f.Stat()
	if statErr != nil {
		_ = f.Close()
		return nil, statErr
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, ErrNotFound
	}
	return f, nil
}

// OpenRange reads the inclusive byte range [start, end] from name.
func (l *Local) OpenRange(ctx context.Context, name string, start, end int64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateRange(start, end); err != nil {
		return nil, err
	}
	rel, err := normalizeObjectPath(name)
	if err != nil {
		return nil, err
	}
	root, err := l.openRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	info, statErr := f.Stat()
	if statErr != nil {
		_ = f.Close()
		return nil, statErr
	}
	if info.IsDir() {
		_ = f.Close()
		return nil, ErrNotFound
	}
	if _, seekErr := f.Seek(start, io.SeekStart); seekErr != nil {
		_ = f.Close()
		return nil, seekErr
	}
	return &limitedReadCloser{
		r: io.LimitReader(f, end-start+1),
		c: f,
	}, nil
}

// Exists reports whether a regular object at name exists.
func (l *Local) Exists(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	rel, err := normalizeObjectPath(name)
	if err != nil {
		return false, err
	}
	root, err := l.openRoot()
	if err != nil {
		return false, err
	}
	defer func() { _ = root.Close() }()

	info, err := root.Stat(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

// Delete removes name; a missing object is not an error.
func (l *Local) Delete(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	rel, err := normalizeObjectPath(name)
	if err != nil {
		return err
	}
	root, err := l.openRoot()
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()

	err = root.Remove(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// List returns sorted full object paths directly inside dir, skipping subdirectories,
// escaping/broken symlinks, and .tmp-* staging files. Missing directories return nil, nil.
func (l *Local) List(ctx context.Context, dir string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rel, err := normalizeDirPath(dir)
	if err != nil {
		return nil, err
	}
	root, err := l.openRoot()
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	entries, err := fs.ReadDir(root.FS(), rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".tmp-") || entry.IsDir() {
			continue
		}
		childRel := path.Join(rel, name)
		if entry.Type()&fs.ModeSymlink != 0 {
			info, statErr := root.Stat(childRel)
			if statErr != nil || info.IsDir() {
				continue
			}
		}
		paths = append(paths, "/"+childRel)
	}
	sort.Strings(paths)
	return paths, nil
}

// Probe verifies that the storage root directory exists and is writable by creating and
// immediately deleting a temporary file inside it.
func (l *Local) Probe(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return mapLocalProbeErr(err)
	}
	root, err := l.openRoot()
	if err != nil {
		return mapLocalProbeErr(err)
	}
	defer func() { _ = root.Close() }()

	tmpName, err := randomTempName(".", ".tmp-probe-")
	if err != nil {
		return mapLocalProbeErr(err)
	}
	f, err := root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return mapLocalProbeErr(err)
	}
	closeErr := f.Close()
	removeErr := root.Remove(tmpName)
	if err := errors.Join(closeErr, removeErr); err != nil {
		return mapLocalProbeErr(err)
	}
	if err := ctx.Err(); err != nil {
		return mapLocalProbeErr(err)
	}
	return nil
}

func mapLocalProbeErr(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%w: %w", ErrTimeout, err)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: %w", ErrAuth, err)
	default:
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
}

func randomTempName(dir, prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	name := prefix + hex.EncodeToString(raw[:])
	if dir == "" || dir == "." {
		return name, nil
	}
	return path.Join(dir, name), nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
			written, writeErr := dst.Write(buf[:n])
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}
