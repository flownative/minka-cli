package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/flownative/minka-cli/skills"
	"github.com/spf13/cobra"
)

const (
	skillName = "minka"
	skillFile = "SKILL.md"

	skillCurrent = "current"
	skillDiffers = "differs"
	skillMissing = "not installed"
)

func (a *app) newSkillCmd() *cobra.Command {
	return group("skill", "Show or install the description of minka for agents",
		a.newSkillShowCmd(),
		a.newSkillInstallCmd(),
		a.newSkillStatusCmd(),
	)
}

func (a *app) newSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print the skill this minka carries",
		Args:  requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			return a.ok(success{
				data:    map[string]any{"name": skillName, "version": version, "content": string(skills.Minka), "references": skillReferences()},
				summary: "The skill of minka " + version,
				text:    func(w io.Writer) { _, _ = w.Write(skills.Minka) },
			})
		},
	}
}

func (a *app) newSkillInstallCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Write the skill to where agents look for skills",
		Long: `Write the skill to where agents look for skills.

Without --dir that is ~/.claude/skills, where Claude Code finds it in every
project. Name another directory of skills with --dir, such as .claude/skills
for one project or ~/.agents/skills.

Run it again after an upgrade: minka says so whenever the skill in
~/.claude/skills differs from the one it carries.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			skillsDir, err := a.skillsDir(dir)
			if err != nil {
				return err
			}
			path, err := installSkill(skillsDir)
			if err != nil {
				return err
			}
			return a.ok(success{
				data:    map[string]string{"name": skillName, "version": version, "path": path, "state": skillCurrent},
				summary: "Installed the skill at " + path,
				text:    func(w io.Writer) { fmt.Fprintf(w, "Installed the skill at %s\n", path) },
			})
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory of skills to install into (default: ~/.claude/skills)")
	return cmd
}

func (a *app) newSkillStatusCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Tell whether the installed skill is the one this minka carries",
		Args:  requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			skillsDir, err := a.skillsDir(dir)
			if err != nil {
				return err
			}
			path := filepath.Join(skillsDir, skillName, skillFile)
			state := skillState(path)
			a.skillIsTheTopic = dir == ""

			s := success{
				data:    map[string]string{"name": skillName, "version": version, "path": path, "state": state},
				summary: "The skill at " + path + " is " + state,
				text:    func(w io.Writer) { fmt.Fprintf(w, "%s: %s\n", path, state) },
			}
			if state != skillCurrent {
				s.breadcrumbs = []Breadcrumb{{Action: "install", Cmd: "minka skill install", Description: "Write the skill this minka carries"}}
			}
			return a.ok(s)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Directory of skills to look into (default: ~/.claude/skills)")
	return cmd
}

// skillsDir is the directory of skills a command works on: the one named, or
// the one Claude Code reads for every project.
func (a *app) skillsDir(named string) (string, error) {
	if named != "" {
		absolute, err := filepath.Abs(named)
		if err != nil {
			return "", &Error{Code: CodeConfig, Message: err.Error()}
		}
		return absolute, nil
	}
	home := a.getenv("HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return "", &Error{Code: CodeConfig, Message: "cannot find the home directory: " + err.Error(), Hint: "Name the directory of skills with --dir."}
		}
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

// installSkill writes the skill into a directory of skills and returns the
// path of its SKILL.md. Every file the skill carries is written, the
// references beside SKILL.md included, each of them replaced whole.
//
// A directory of skills, the directory of this skill in it, or a directory of
// the skill below that, which is a symbolic link belongs to somebody else: a
// checkout somebody works in, or the store of another tool. Writing through it
// would change a file there, so it is refused before anything is written.
// Links further up, such as a home directory kept elsewhere, are where
// somebody chose to keep their own files.
func installSkill(skillsDir string) (string, error) {
	dir := filepath.Join(skillsDir, skillName)
	files := skills.MinkaFiles()
	if link := firstSymlink(append([]string{skillsDir}, skillDirectories(dir, files)...)...); link != "" {
		return "", &Error{
			Code:    CodeConfig,
			Message: link + " is a symbolic link; the skill is not written through it",
			Hint:    "Remove the link, or name another directory of skills with --dir.",
		}
	}

	names := slices.Sorted(maps.Keys(files))
	for _, name := range names {
		if err := writeSkillFile(filepath.Join(dir, filepath.FromSlash(name)), files[name]); err != nil {
			return "", &Error{Code: CodeConfig, Message: err.Error()}
		}
	}
	return filepath.Join(dir, skillFile), nil
}

// skillReferences are the files of the skill beside SKILL.md, by their path
// below the directory of the skill.
func skillReferences() map[string]string {
	references := map[string]string{}
	for name, content := range skills.MinkaFiles() {
		if name != skillFile {
			references[name] = string(content)
		}
	}
	return references
}

// skillDirectories are the directory of the skill and every directory below
// it that a file of the skill lies in.
func skillDirectories(dir string, files map[string][]byte) []string {
	dirs := []string{dir}
	for name := range files {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if full := filepath.Join(dir, filepath.FromSlash(parent)); !slices.Contains(dirs, full) {
				dirs = append(dirs, full)
			}
		}
	}
	slices.Sort(dirs)
	return dirs
}

// writeSkillFile writes one file through a temporary file renamed into place,
// so that a file that is a symbolic link is replaced, not written through.
func writeSkillFile(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".skill-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())

	_, err = temp.Write(content)
	if err == nil {
		err = temp.Chmod(0o644)
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), path)
	}
	return err
}

func firstSymlink(paths ...string) string {
	for _, path := range paths {
		if info, err := os.Lstat(path); err == nil && info.Mode()&fs.ModeSymlink != 0 {
			return path
		}
	}
	return ""
}

// skillState compares the skill whose SKILL.md lies at path with the one this
// binary carries, file by file. A reference missing beside SKILL.md makes it
// differ: a pointer to it would lead nowhere. A file the binary does not carry
// is left out of the comparison; it harms nobody.
func skillState(path string) string {
	installed, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return skillMissing
	case err != nil || !bytes.Equal(installed, skills.Minka):
		return skillDiffers
	}
	dir := filepath.Dir(path)
	for name, content := range skills.MinkaFiles() {
		if installed, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name))); err != nil || !bytes.Equal(installed, content) {
			return skillDiffers
		}
	}
	return skillCurrent
}

// staleSkillNotice says when the skill Claude Code reads is not the one this
// binary carries, which is what happens after an upgrade. It only says so:
// nothing is written into a home directory unasked. A skill behind a symbolic
// link is somebody else's to keep current, and installing would be refused, so
// there is nothing to advise.
func (a *app) staleSkillNotice() string {
	skillsDir, err := a.skillsDir("")
	if err != nil {
		return ""
	}
	if firstSymlink(append([]string{skillsDir}, skillDirectories(filepath.Join(skillsDir, skillName), skills.MinkaFiles())...)...) != "" {
		return ""
	}
	path := filepath.Join(skillsDir, skillName, skillFile)
	if skillState(path) != skillDiffers {
		return ""
	}
	return "the skill at " + path + " differs from the one this minka carries; run: minka skill install"
}
