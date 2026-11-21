// Package auth defines the session identity consumed by protected API handlers.
package auth

// Principal represents administrator or visitor authorization state.
type Principal struct {
	Admin                bool
	UnlockedDirectoryIDs []string
}
