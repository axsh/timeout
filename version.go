package timeout

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionRaw string

// Version is the library/product version from the VERSION file (trimmed).
// Released Go module tags are "v" + Version (for example v0.3.0).
var Version = strings.TrimSpace(versionRaw)
