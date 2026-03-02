package buildinfo

// These are set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"
)
