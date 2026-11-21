// Package webembed exposes the compiled frontend embedded in the server.
package webembed

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var files embed.FS

// Files returns the embedded frontend filesystem.
func Files() (fs.FS, error) {
	return fs.Sub(files, "dist")
}
