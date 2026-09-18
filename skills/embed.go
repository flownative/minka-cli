// Package skills carries the description of the CLI for agents inside the
// binary, so that what an agent reads always fits the commands it can run.
package skills

import _ "embed"

// Minka is the SKILL.md of the minka skill.
//
//go:embed minka/SKILL.md
var Minka []byte
