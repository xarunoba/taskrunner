// Package version provides the release version embedded in the executable.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var release string

var current = strings.TrimSpace(release)

// Current returns the embedded SemVer number without the tag's v prefix.
func Current() string {
	return current
}
