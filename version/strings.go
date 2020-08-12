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
	Hash              string
	Dirty             string
	CompileTime       string
	GitDecorateBase64 string
)

const UnknownVersion = "(unknown version)"

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
