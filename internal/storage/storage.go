// Package storage is the private object store; all reads go through the server.
package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/url"
	"path"

	"github.com/studio-b12/gowebdav"
)

// ErrNotFound is returned when an object does not exist.
var ErrNotFound = errors.New("storage: object not found")

// Store abstracts private object storage. Paths are absolute, slash separated, e.g. /xml/{id}.xml.
type Store interface {
	Put(ctx context.Context, name string, body io.Reader) error
	Open(ctx context.Context, name string) (io.ReadCloser, error)
	Exists(ctx context.Context, name string) (bool, error)
	Delete(ctx context.Context, name string) error
}

// WebDAV stores objects on a WebDAV server.
type WebDAV struct {
	client *gowebdav.Client
}

// NewWebDAV connects to the WebDAV root at rawURL.
func NewWebDAV(rawURL, user, password string) (*WebDAV, error) {
	if _, err := url.Parse(rawURL); err != nil {
		return nil, err
	}
	return &WebDAV{client: gowebdav.NewClient(rawURL, user, password)}, nil
}

// Put writes body to name, creating parent collections.
func (w *WebDAV) Put(_ context.Context, name string, body io.Reader) error {
	if err := w.client.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	return w.client.WriteStream(name, body, 0o644)
}

// Open streams name.
func (w *WebDAV) Open(_ context.Context, name string) (io.ReadCloser, error) {
	r, err := w.client.ReadStream(name)
	return r, mapErr(err)
}

// Exists reports whether name exists.
func (w *WebDAV) Exists(_ context.Context, name string) (bool, error) {
	_, err := w.client.Stat(name)
	if err = mapErr(err); errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// Delete removes name; a missing object is not an error.
func (w *WebDAV) Delete(_ context.Context, name string) error {
	if err := mapErr(w.client.Remove(name)); !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

func mapErr(err error) error {
	if err != nil && (gowebdav.IsErrNotFound(err) || errors.Is(err, fs.ErrNotExist)) {
		return ErrNotFound
	}
	return err
}
