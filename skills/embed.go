// Package skills carries the description of the CLI for agents inside the
// binary, so that what an agent reads always fits the commands it can run.
package skills

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// Minka is the SKILL.md of the minka skill.
//
//go:embed minka/SKILL.md
var Minka []byte

//go:embed minka
var minka embed.FS

// MinkaFiles is every file of the minka skill by its path below the directory
// of the skill, SKILL.md and the references it points to alike. They travel
// and are installed together, so that a pointer never leads nowhere.
func MinkaFiles() map[string][]byte {
	files := map[string][]byte{}
	err := fs.WalkDir(minka, "minka", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := minka.ReadFile(name)
		if err != nil {
			return err
		}
		relative, _ := strings.CutPrefix(name, "minka/")
		files[path.Clean(relative)] = content
		return nil
	})
	if err != nil {
		// What is embedded is read from memory; a failure is a broken build.
		panic(err)
	}
	return files
}
