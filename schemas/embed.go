// Package schemas embeds the JSON Schemas that define every aitk artifact.
package schemas

import "embed"

// FS holds the *.schema.json files.
//
//go:embed *.schema.json
var FS embed.FS
