package version

import (
	"fmt"
)

// These are targets for compiling in build information.
// See the top level Makefile and cmd/algorand-indexer/main.go

var (
	// Hash Git commit hash. Output of `git log -n 1 --pretty="%H"`
	Hash string

	// CompileTime YYYY-mm-ddTHH:MM:SS+ZZZZ
	CompileTime string

	// ReleaseVersion is set using -ldflags during build.
	ReleaseVersion string
)

// UnknownVersion is used when the version is not known.
const UnknownVersion = "(unknown version)"

// Version the binary version.
func Version() string {
	if ReleaseVersion == "" {
		return UnknownVersion
	}
	return ReleaseVersion
}

// LongVersion the long form of the binary version.
func LongVersion() string {
	tagVersion := Version()
	if tagVersion == UnknownVersion {
		tagVersion = fmt.Sprintf("%s-dev.unknown", ReleaseVersion)
	} else if tagVersion != ReleaseVersion {
		tagVersion = fmt.Sprintf("dev release build .version=%s tag=%s", ReleaseVersion, tagVersion)
	}
	return fmt.Sprintf("%s compiled at %s from git hash %s", tagVersion, CompileTime, Hash)
}
