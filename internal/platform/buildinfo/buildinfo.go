// Package buildinfo holds version metadata injected at build time via -ldflags.
package buildinfo

var (
	// Version is the semantic version string, set via -ldflags.
	Version = "dev"
	// Commit is the short git commit hash, set via -ldflags.
	Commit = "unknown"
)
