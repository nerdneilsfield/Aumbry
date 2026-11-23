package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func runStoreConformanceTests(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("crud and immediate not found", func(t *testing.T) {
		store := newStore(t)

		if ok, err := store.Exists(ctx, "/xml/a.xml"); err != nil || ok {
			t.Fatalf("Exists before put = (%v, %v), want (false, nil)", ok, err)
		}
		// Open must report ErrNotFound immediately before any Read on the returned stream.
		if _, err := store.Open(ctx, "/xml/a.xml"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Open missing = %v, want ErrNotFound", err)
		}
		if err := store.Delete(ctx, "/xml/a.xml"); err != nil {
			t.Fatalf("Delete missing = %v, want nil", err)
		}

		if err := store.Put(ctx, "/xml/a.xml", strings.NewReader("<a/>")); err != nil {
			t.Fatalf("Put create = %v", err)
		}
		if ok, err := store.Exists(ctx, "/xml/a.xml"); err != nil || !ok {
			t.Fatalf("Exists after put = (%v, %v), want (true, nil)", ok, err)
		}
		if err := store.Put(ctx, "/xml/a.xml", strings.NewReader("<b/>")); err != nil {
			t.Fatalf("Put overwrite = %v", err)
		}
		r, err := store.Open(ctx, "/xml/a.xml")
		if err != nil {
			t.Fatalf("Open existing = %v", err)
		}
		got, err := io.ReadAll(r)
		closeErr := r.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("ReadAll/Close = (%v, %v)", err, closeErr)
		}
		if string(got) != "<b/>" {
			t.Fatalf("content = %q, want <b/>", got)
		}

		if err := store.Delete(ctx, "/xml/a.xml"); err != nil {
			t.Fatalf("Delete existing = %v", err)
		}
		if err := store.Delete(ctx, "/xml/a.xml"); err != nil {
			t.Fatalf("Delete missing second time = %v", err)
		}
		if ok, err := store.Exists(ctx, "/xml/a.xml"); err != nil || ok {
			t.Fatalf("Exists after delete = (%v, %v), want (false, nil)", ok, err)
		}
		if _, err := store.Open(ctx, "/xml/a.xml"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Open after delete = %v, want ErrNotFound", err)
		}
	})

	t.Run("list non-recursive, missing dir empty, and paginated", func(t *testing.T) {
		store := newStore(t)

		missing, err := store.List(ctx, "/xml")
		if err != nil {
			t.Fatalf("List missing /xml = %v, want nil error", err)
		}
		if len(missing) != 0 {
			t.Fatalf("List missing /xml = %v, want empty", missing)
		}

		// Nested /xml/sub/c.xml must be ignored while 5 direct objects exercise multi-page listing.
		if putErr := store.Put(ctx, "/xml/sub/c.xml", strings.NewReader("<sub/>")); putErr != nil {
			t.Fatalf("Put nested = %v", putErr)
		}
		var want []string
		for i := 1; i <= 5; i++ {
			name := fmt.Sprintf("/xml/item-%02d.xml", i)
			want = append(want, name)
			if putErr := store.Put(ctx, name, bytes.NewReader([]byte("<doc/>"))); putErr != nil {
				t.Fatalf("Put %s = %v", name, putErr)
			}
		}

		got, err := store.List(ctx, "/xml")
		if err != nil {
			t.Fatalf("List /xml = %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("List(/xml) = %v, want %v", got, want)
		}

		gotMissingSub, err := store.List(ctx, "/xml/does-not-exist")
		if err != nil {
			t.Fatalf("List missing subdir = %v, want nil error", err)
		}
		if len(gotMissingSub) != 0 {
			t.Fatalf("List missing subdir = %v, want empty", gotMissingSub)
		}
	})

	t.Run("probe healthy", func(t *testing.T) {
		store := newStore(t)
		if err := store.Probe(ctx); err != nil {
			t.Fatalf("Probe() = %v, want nil", err)
		}
	})

	t.Run("open range", func(t *testing.T) {
		store := newStore(t)
		payload := []byte("0123456789abcdef")
		if err := store.Put(ctx, "/assets/range.bin", bytes.NewReader(payload)); err != nil {
			t.Fatalf("Put = %v", err)
		}

		readSlice := func(start, end int64) string {
			t.Helper()
			r, err := store.OpenRange(ctx, "/assets/range.bin", start, end)
			if err != nil {
				t.Fatalf("OpenRange(%d, %d) = %v", start, end, err)
			}
			defer r.Close()
			got, readErr := io.ReadAll(r)
			if readErr != nil {
				t.Fatalf("ReadAll(%d, %d) = %v", start, end, readErr)
			}
			return string(got)
		}

		if got := readSlice(0, 3); got != "0123" {
			t.Fatalf("prefix slice = %q, want 0123", got)
		}
		if got := readSlice(4, 9); got != "456789" {
			t.Fatalf("middle slice = %q, want 456789", got)
		}
		if got := readSlice(5, 5); got != "5" {
			t.Fatalf("single byte = %q, want 5", got)
		}
		if got := readSlice(12, 15); got != "cdef" {
			t.Fatalf("suffix slice = %q, want cdef", got)
		}

		if _, err := store.OpenRange(ctx, "/assets/range.bin", -1, 3); err == nil {
			t.Fatal("expected negative start error")
		}
		if _, err := store.OpenRange(ctx, "/assets/range.bin", 5, 4); err == nil {
			t.Fatal("expected inverted range error")
		}
		if _, err := store.OpenRange(ctx, "/assets/missing.bin", 0, 3); !errors.Is(err, ErrNotFound) {
			t.Fatalf("OpenRange missing = %v, want ErrNotFound", err)
		}
		canceledCtx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := store.OpenRange(canceledCtx, "/assets/range.bin", 0, 3); !errors.Is(err, context.Canceled) {
			t.Fatalf("OpenRange canceled = %v, want context.Canceled", err)
		}
	})
}

func TestLocalConformance(t *testing.T) {
	runStoreConformanceTests(t, func(t *testing.T) Store {
		store, err := NewLocalWithOptions(filepath.Join(t.TempDir(), "objects"), LocalOptions{CreateRoot: true})
		if err != nil {
			t.Fatal(err)
		}
		return store
	})
}

func TestLocalCreateRootOptionAndPermissions(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "missing-root")
	if _, err := NewLocal(absent); err == nil {
		t.Fatalf("NewLocal(%s) succeeded for absent dir, want error", absent)
	}
	if _, err := os.Stat(absent); !os.IsNotExist(err) {
		t.Fatalf("NewLocal(%s) created absent directory", absent)
	}

	root := filepath.Join(t.TempDir(), "objects")
	store, err := NewLocalWithOptions(root, LocalOptions{CreateRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := store.Put(ctx, "/xml/doc.xml", strings.NewReader("<document/>")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, "/assets/"+strings.Repeat("a", 64)+".png", strings.NewReader("pngbytes")); err != nil {
		t.Fatal(err)
	}

	for _, dir := range []string{root, filepath.Join(root, "xml"), filepath.Join(root, "assets")} {
		info, statErr := os.Stat(dir)
		if statErr != nil {
			t.Fatalf("stat %s: %v", dir, statErr)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("dir %s mode = %04o, want 0700", dir, perm)
		}
	}
	for _, file := range []string{
		filepath.Join(root, "xml", "doc.xml"),
		filepath.Join(root, "assets", strings.Repeat("a", 64)+".png"),
	} {
		info, statErr := os.Stat(file)
		if statErr != nil {
			t.Fatalf("stat %s: %v", file, statErr)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("file %s mode = %04o, want 0600", file, perm)
		}
	}
}

func TestLocalSecurityPathRejection(t *testing.T) {
	store, err := NewLocalWithOptions(filepath.Join(t.TempDir(), "objects"), LocalOptions{CreateRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	badObjects := []string{
		"",
		"xml/a.xml",
		"/xml/",
		"/xml/../../x",
		"/xml/../xml/a.xml",
		"/xml/./a.xml",
		"/xml//a.xml",
		"/xml/bad\x00.xml",
		`/xml/bad\a.xml`,
		"/assets/../../x",
		"/other/a.xml",
	}
	for _, name := range badObjects {
		if putErr := store.Put(ctx, name, strings.NewReader("x")); putErr == nil {
			t.Errorf("Put(%q) succeeded, want rejection", name)
		}
		if _, openErr := store.Open(ctx, name); openErr == nil {
			t.Errorf("Open(%q) succeeded, want rejection", name)
		}
		if _, exErr := store.Exists(ctx, name); exErr == nil {
			t.Errorf("Exists(%q) succeeded, want rejection", name)
		}
		if delErr := store.Delete(ctx, name); delErr == nil {
			t.Errorf("Delete(%q) succeeded, want rejection", name)
		}
	}

	for _, badDir := range []string{"", "xml", "/etc", "/xml/..", "/xml//sub", "/xml/\x00", `/xml\sub`} {
		if _, listErr := store.List(ctx, badDir); listErr == nil {
			t.Errorf("List(%q) succeeded, want rejection", badDir)
		}
	}
}

func TestLocalSymlinkEscapeBlocked(t *testing.T) {
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("top-secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "objects")
	store, err := NewLocalWithOptions(root, LocalOptions{CreateRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if putErr := store.Put(ctx, "/xml/legit.xml", strings.NewReader("<ok/>")); putErr != nil {
		t.Fatal(putErr)
	}
	if linkErr := os.Symlink(outsideFile, filepath.Join(root, "xml", "escape.xml")); linkErr != nil {
		t.Fatal(linkErr)
	}
	if linkErr := os.Symlink(outside, filepath.Join(root, "xml", "escapedir")); linkErr != nil {
		t.Fatal(linkErr)
	}

	if _, openErr := store.Open(ctx, "/xml/escape.xml"); openErr == nil {
		t.Fatal("Open followed file symlink outside root")
	}
	if _, openErr := store.Open(ctx, "/xml/escapedir/secret.txt"); openErr == nil {
		t.Fatal("Open followed directory symlink outside root")
	}
	if ok, exErr := store.Exists(ctx, "/xml/escape.xml"); exErr == nil || ok {
		t.Fatalf("Exists(file symlink) = (%v, %v), want (false, err)", ok, exErr)
	}
	if delErr := store.Delete(ctx, "/xml/escapedir/secret.txt"); delErr == nil {
		t.Fatal("Delete traversed escaping directory symlink")
	}
	if putErr := store.Put(ctx, "/xml/escapedir/evil.xml", strings.NewReader("<evil/>")); putErr == nil {
		t.Fatal("Put wrote through a directory symlink outside root")
	}
	if _, statErr := os.Stat(filepath.Join(outside, "evil.xml")); statErr == nil {
		t.Fatal("Put created a file outside root")
	}
	if _, statErr := os.Stat(outsideFile); statErr != nil {
		t.Fatalf("outside file damaged: %v", statErr)
	}

	listed, err := store.List(ctx, "/xml")
	if err != nil {
		t.Fatalf("List(/xml) = %v", err)
	}
	if !reflect.DeepEqual(listed, []string{"/xml/legit.xml"}) {
		t.Fatalf("List(/xml) = %v, want only [/xml/legit.xml]", listed)
	}
}

type failingReadSeeker struct {
	data    []byte
	offset  int
	failAt  int
	err     error
	onChunk func()
}

func (r *failingReadSeeker) Read(p []byte) (int, error) {
	if r.onChunk != nil {
		r.onChunk()
	}
	if r.offset >= r.failAt {
		return 0, r.err
	}
	limit := min(len(p), r.failAt-r.offset, len(r.data)-r.offset)
	n := copy(p[:limit], r.data[r.offset:r.offset+limit])
	r.offset += n
	if n == 0 && r.offset >= len(r.data) {
		return 0, io.EOF
	}
	return n, nil
}

func (r *failingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var next int64
	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = int64(r.offset) + offset
	case io.SeekEnd:
		next = int64(len(r.data)) + offset
	default:
		return 0, errors.New("invalid whence")
	}
	if next < 0 {
		return 0, errors.New("negative offset")
	}
	r.offset = int(next)
	return next, nil
}

func TestLocalAtomicWriteAndTempExclusion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "objects")
	store, err := NewLocalWithOptions(root, LocalOptions{CreateRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	if putErr := store.Put(ctx, "/xml/existing.xml", strings.NewReader("good-content")); putErr != nil {
		t.Fatal(putErr)
	}

	failErr := errors.New("mid-stream failure")
	if putErr := store.Put(ctx, "/xml/existing.xml", &failingReadSeeker{
		data:   []byte("corrupt-data"),
		failAt: 4,
		err:    failErr,
	}); !errors.Is(putErr, failErr) {
		t.Fatalf("overwrite failure = %v, want %v", putErr, failErr)
	}
	r, err := store.Open(ctx, "/xml/existing.xml")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	_ = r.Close()
	if string(got) != "good-content" {
		t.Fatalf("existing file corrupted after failed overwrite: %q", got)
	}

	if putErr := store.Put(ctx, "/xml/partial.xml", &failingReadSeeker{
		data:   []byte("partial-data"),
		failAt: 5,
		err:    failErr,
	}); !errors.Is(putErr, failErr) {
		t.Fatalf("new file failure = %v, want %v", putErr, failErr)
	}
	if ok, exErr := store.Exists(ctx, "/xml/partial.xml"); exErr != nil || ok {
		t.Fatalf("partial file exists = (%v, %v), want (false, nil)", ok, exErr)
	}

	cctx, cancel := context.WithCancel(ctx)
	if putErr := store.Put(cctx, "/xml/canceled.xml", &failingReadSeeker{
		data:   bytes.Repeat([]byte("a"), 1024),
		failAt: 1024,
		err:    io.EOF,
		onChunk: func() {
			cancel()
		},
	}); !errors.Is(putErr, context.Canceled) {
		t.Fatalf("canceled Put = %v, want context.Canceled", putErr)
	}
	if ok, exErr := store.Exists(ctx, "/xml/canceled.xml"); exErr != nil || ok {
		t.Fatalf("canceled file exists = (%v, %v), want (false, nil)", ok, exErr)
	}

	// Failed writes must remove their staging files, not just hide them from List.
	if entries, dirErr := os.ReadDir(filepath.Join(root, "xml")); dirErr != nil {
		t.Fatal(dirErr)
	} else if len(entries) != 1 || entries[0].Name() != "existing.xml" {
		t.Fatalf("xml dir after failed writes = %v, want only existing.xml", entries)
	}

	// Simulate an orphaned temp file left behind by a killed process; List must ignore it.
	if writeErr := os.WriteFile(filepath.Join(root, "xml", ".tmp-orphan"), []byte("tmp"), 0o600); writeErr != nil {
		t.Fatal(writeErr)
	}
	listed, err := store.List(ctx, "/xml")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(listed, []string{"/xml/existing.xml"}) {
		t.Fatalf("List(/xml) = %v, want [/xml/existing.xml]", listed)
	}
}

func TestLocalProbe(t *testing.T) {
	root := filepath.Join(t.TempDir(), "objects")
	store, err := NewLocalWithOptions(root, LocalOptions{CreateRoot: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if probeErr := store.Probe(ctx); probeErr != nil {
		t.Fatalf("Probe() healthy = %v, want nil", probeErr)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Probe left entries in root: %v", entries)
	}

	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := store.Probe(ctx); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("Probe missing root = %v, want ErrUnreachable", err)
	}
}
