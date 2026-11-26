// Package standards embeds aitk's built-in development standards (one markdown file per topic).
package standards

import "embed"

// FS holds the built-in standard files.
//
//go:embed *.md
var FS embed.FS
