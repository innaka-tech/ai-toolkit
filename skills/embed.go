// Package skills embeds the Agent Skills that ship with aitk.
package skills

import _ "embed"

// AitkSkill is skills/aitk/SKILL.md.
//
//go:embed aitk/SKILL.md
var AitkSkill []byte
