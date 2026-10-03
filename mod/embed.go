// Package mod carries the Claude Code mod that reports from inside the running
// Claude Code process, as a marketplace of one plugin. minka writes it out
// beside its configuration, and Claude Code loads it from there in place, so
// the mod a machine runs is the one of the minka that wrote it.
package mod

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

// Marketplace is the name the marketplace goes by in Claude Code.
const Marketplace = "minka-cli"

// Plugin is how Claude Code names the mod when it installs and lists it.
const Plugin = "minka-heartbeat@" + Marketplace

// The tests of the mod stay out of what is written: Claude Code needs none of
// them to run it.
//
//go:embed marketplace/.claude-plugin/marketplace.json
//go:embed marketplace/minka-heartbeat/.claude-plugin/plugin.json
//go:embed marketplace/minka-heartbeat/hooks/hooks.json
//go:embed marketplace/minka-heartbeat/hooks/register.js
var marketplace embed.FS

// programLine is where the module names the minka it runs. As it stands in
// the repository it names the minka on the PATH.
var programLine = []byte("const minka = 'minka'\n")

// Files are the files of the marketplace by their path below its directory,
// with the module made to run the minka at program.
func Files(program string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(marketplace, "marketplace", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := marketplace.ReadFile(path)
		if err != nil {
			return err
		}
		files[path[len("marketplace/"):]] = content
		return nil
	})
	if err != nil {
		return nil, err
	}

	const module = "minka-heartbeat/hooks/register.js"
	if bytes.Count(files[module], programLine) != 1 {
		return nil, fmt.Errorf("%s does not name the minka it runs in one line of its own", module)
	}
	named, err := json.Marshal(program)
	if err != nil {
		return nil, err
	}
	files[module] = bytes.Replace(files[module], programLine, []byte("const minka = "+string(named)+"\n"), 1)
	return files, nil
}
