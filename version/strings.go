package version

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

// These are targets for compiling in build information.
// See the top level Makefile and cmd/algorand-indexer/main.go

var (
	// Hash Git commit hash. Output of `git log -n 1 --pretty="%H"`
	Hash string

	// Dirty "true" or ""
	// A release build should have no modified files and no unknown files.
	Dirty string

	// CompileTime YYYY-mm-ddTHH:MM:SS+ZZZZ
	CompileTime string

	// GitDecorateBase64 Decorations of latest commit which may include tags. Output of `git log -n 1 --pretty="%D"|base64`
	GitDecorateBase64 string

	// ReleaseVersion What was in /.version when this was compiled.
	ReleaseVersion string
)

// UnknownVersion is used when the version is not known.
const UnknownVersion = "(unknown version)"

// Version the binary version.
func Version() string {
	// parse "tag: 1.2.3" out of the result of `git log -n 1 --pretty="%D"|base64`
	if len(GitDecorateBase64) == 0 {
		return UnknownVersion
	}
	b, err := base64.StdEncoding.DecodeString(GitDecorateBase64)
	if err != nil {
		return fmt.Sprintf("compiled with bad GitDecorateBase64, %s", err.Error())
	}
	tre := regexp.MustCompile("tag:\\s+([^,]+)")
	m := tre.FindAllStringSubmatch(string(b), -1)
	if m == nil {
		return UnknownVersion
	}
	for _, group := range m {
		if len(group[1]) > 0 {
			return strings.TrimSpace(group[1])
		}
	}
	return UnknownVersion
}

// LongVersion the long form of the binary version.
func LongVersion() string {
	dirtyStr := ""
	if (len(Dirty) > 0) && (Dirty != "false") {
		dirtyStr = " (modified)"
	}
	tagVersion := Version()
	if tagVersion == UnknownVersion {
		tagVersion = fmt.Sprintf("%s-dev.unknown", ReleaseVersion)
	} else if tagVersion != ReleaseVersion {
		tagVersion = fmt.Sprintf("dev release build .version=%s tag=%s", ReleaseVersion, tagVersion)
	}
	return fmt.Sprintf("%s compiled at %s from git hash %s%s", tagVersion, CompileTime, Hash, dirtyStr)
}
