// Package commands embeds aitk's built-in slash commands and personas (aitk-*.md).
package commands

import "embed"

// FS holds the built-in command files.
//
//go:embed aitk-*.md
var FS embed.FS
