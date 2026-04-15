package buildinfo

// Version and Commit are set at build time via -ldflags.
var (
	Version = "dev"
	Commit  = "unknown"
)
