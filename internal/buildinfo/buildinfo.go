// Package buildinfo carries release metadata injected with -ldflags.
package buildinfo

var (
	// Version is the application version.
	Version = "dev"
	// Commit is the source revision.
	Commit = "none"
	// Date is the build timestamp.
	Date = "unknown"
)
