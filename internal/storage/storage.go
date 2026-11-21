// Package storage defines the private object store interface.
package storage

import (
	"context"
	"io"
)

// Store abstracts private object storage; the WebDAV adapter is not wired yet.
type Store interface {
	Put(context.Context, string, io.Reader, string) error
	Open(context.Context, string) (io.ReadCloser, error)
	Exists(context.Context, string) (bool, error)
}
