package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/skills"
)

func (e *testEnv) installedSkillPath() string {
	return filepath.Join(e.vars["HOME"], ".claude", "skills", "minka", "SKILL.md")
}

func (e *testEnv) writeInstalledSkill(content string) {
	e.t.Helper()
	path := e.installedSkillPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func TestSkillInstallWritesTheSkillTheBinaryCarries(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "skill", "install")
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["path"]; got != e.installedSkillPath() {
		t.Fatalf("path %v, want %s", got, e.installedSkillPath())
	}

	installed, err := os.ReadFile(e.installedSkillPath())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(installed, skills.Minka) {
		t.Fatal("the installed skill is not the one the binary carries")
	}

	env, r = e.runJSON("", "skill", "status")
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["state"]; got != skillCurrent {
		t.Fatalf("state %v, want %s", got, skillCurrent)
	}
}

func TestSkillInstallReplacesAnOlderSkill(t *testing.T) {
	e := newTestEnv(t)
	e.writeInstalledSkill("an older skill")

	_, r := e.runJSON("", "skill", "install")
	expectCode(t, r, ExitOK)

	installed, _ := os.ReadFile(e.installedSkillPath())
	if !bytes.Equal(installed, skills.Minka) {
		t.Fatal("the older skill was not replaced")
	}
}

func TestSkillInstallDoesNotWriteThroughASymbolicLink(t *testing.T) {
	e := newTestEnv(t)
	elsewhere := t.TempDir()
	theirs := filepath.Join(elsewhere, "SKILL.md")
	if err := os.WriteFile(theirs, []byte("somebody else's file"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(e.vars["HOME"], ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(skillsDir, "minka")); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "skill", "install")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig {
		t.Fatalf("code %q, want %q", env.Code, CodeConfig)
	}
	if content, _ := os.ReadFile(theirs); string(content) != "somebody else's file" {
		t.Fatal("the file behind the link was changed")
	}
}

func TestSkillInstallDoesNotWriteThroughALinkedDirectoryOfSkills(t *testing.T) {
	e := newTestEnv(t)
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(e.vars["HOME"], ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(e.vars["HOME"], ".claude", "skills")); err != nil {
		t.Fatal(err)
	}

	_, r := e.runJSON("", "skill", "install")
	expectCode(t, r, ExitUsage)
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("something was written behind the link: %v", entries)
	}
}

func TestSkillInstallReplacesALinkedFileInsteadOfWritingThroughIt(t *testing.T) {
	e := newTestEnv(t)
	theirs := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(theirs, []byte("somebody else's file"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(e.installedSkillPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(theirs, e.installedSkillPath()); err != nil {
		t.Fatal(err)
	}

	_, r := e.runJSON("", "skill", "install")
	expectCode(t, r, ExitOK)
	if content, _ := os.ReadFile(theirs); string(content) != "somebody else's file" {
		t.Fatal("the file behind the link was changed")
	}
	if info, err := os.Lstat(e.installedSkillPath()); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("the installed skill is not a regular file: %v %v", info, err)
	}
}

func TestNoNoticeAboutASkillBehindALink(t *testing.T) {
	e := newTestEnv(t)
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "SKILL.md"), []byte("kept by somebody else"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillsDir := filepath.Join(e.vars["HOME"], ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(skillsDir, "minka")); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "version")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Fatalf("notice %q advises an install that would be refused", env.Notice)
	}
}

func TestACommandWorksWithoutAHomeDirectory(t *testing.T) {
	e := newTestEnv(t)
	delete(e.vars, "HOME")
	t.Setenv("HOME", "")

	_, r := e.runJSON("", "version")
	expectCode(t, r, ExitOK)
}

func TestSkillInstallReportsAnAbsolutePath(t *testing.T) {
	e := newTestEnv(t)
	t.Chdir(t.TempDir())

	env, r := e.runJSON("", "skill", "install", "--dir", "skills")
	expectCode(t, r, ExitOK)
	if path, _ := decode(t, env.Data)["path"].(string); !filepath.IsAbs(path) {
		t.Fatalf("path %q is relative", path)
	}
}

func TestSkillStatusOfANamedDirectoryStillMentionsTheHomeCopy(t *testing.T) {
	e := newTestEnv(t)
	e.writeInstalledSkill("an older skill")

	env, r := e.runJSON("", "skill", "status", "--dir", t.TempDir())
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "minka skill install") {
		t.Fatalf("notice %q does not mention the copy in the home directory", env.Notice)
	}
}

func TestSkillInstallIntoANamedDirectory(t *testing.T) {
	e := newTestEnv(t)
	dir := filepath.Join(t.TempDir(), "project", ".claude", "skills")

	_, r := e.runJSON("", "skill", "install", "--dir", dir)
	expectCode(t, r, ExitOK)

	if _, err := os.Stat(filepath.Join(dir, "minka", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(e.installedSkillPath()); err == nil {
		t.Fatal("the skill was written to the home directory as well")
	}
}

func TestADifferingSkillIsReportedAndNotReplaced(t *testing.T) {
	e := newTestEnv(t)
	e.writeInstalledSkill("an older skill")

	env, r := e.runJSON("", "version")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "minka skill install") {
		t.Fatalf("notice %q does not tell how to install the skill", env.Notice)
	}
	if content, _ := os.ReadFile(e.installedSkillPath()); string(content) != "an older skill" {
		t.Fatal("the installed skill was changed without being asked")
	}

	quiet := e.run("", "version", "--quiet")
	expectCode(t, quiet, ExitOK)
	if strings.Contains(quiet.stdout, "skill") {
		t.Fatalf("the notice reached stdout of --quiet: %s", quiet.stdout)
	}
	if !strings.Contains(quiet.stderr, "minka skill install") {
		t.Fatalf("stderr %q does not carry the notice", quiet.stderr)
	}
}

func TestNoNoticeWithoutASkillOrWithTheCurrentOne(t *testing.T) {
	e := newTestEnv(t)

	env, _ := e.runJSON("", "version")
	if env.Notice != "" {
		t.Fatalf("notice %q although no skill is installed", env.Notice)
	}

	e.writeInstalledSkill(string(skills.Minka))
	env, _ = e.runJSON("", "version")
	if env.Notice != "" {
		t.Fatalf("notice %q although the skill is current", env.Notice)
	}
}

func TestSkillStatusSaysItOnce(t *testing.T) {
	e := newTestEnv(t)
	e.writeInstalledSkill("an older skill")

	env, r := e.runJSON("", "skill", "status")
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["state"]; got != skillDiffers {
		t.Fatalf("state %v, want %s", got, skillDiffers)
	}
	if env.Notice != "" {
		t.Fatalf("notice %q repeats what the command reports", env.Notice)
	}
}

func TestSkillShowPrintsTheSkill(t *testing.T) {
	e := newTestEnv(t)

	r := e.run("", "skill", "show", "--styled")
	expectCode(t, r, ExitOK)
	if r.stdout != string(skills.Minka) {
		t.Fatal("the text output is not the skill")
	}
}

// minkaCall matches a call of minka wherever a command line can start: at the
// start of a line, in inline code, behind a pipe or inside $( ).
var minkaCall = regexp.MustCompile("(?m)(?:^|[`|(] ?)minka ([a-z][a-z-]*(?: [a-z][a-z-]*)?)")

func TestTheSkillNamesEveryCommandAndNoOther(t *testing.T) {
	root := newApp(t.Context(), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, func(string) string { return "" }).newRootCmd()
	skill := string(skills.Minka)

	existing := map[string]bool{}
	for _, group := range root.Commands() {
		if group.Hidden || group.Name() == "help" {
			continue
		}
		if !group.HasSubCommands() {
			existing[group.Name()] = true
			continue
		}
		for _, cmd := range group.Commands() {
			existing[group.Name()+" "+cmd.Name()] = true
		}
	}

	for path := range existing {
		// A single word in inline code may be a placeholder, such as `me` for
		// a person, so it only counts behind "minka".
		prefix := "(?:minka |`)"
		if !strings.Contains(path, " ") {
			prefix = "minka "
		}
		named := regexp.MustCompile(prefix + regexp.QuoteMeta(path) + "(?:[^a-z-]|$)")
		if !named.MatchString(skill) {
			t.Errorf("the skill does not name %q", path)
		}
	}

	for _, match := range minkaCall.FindAllStringSubmatch(skill, -1) {
		path := match[1]
		if existing[path] {
			continue
		}
		if first, _, _ := strings.Cut(path, " "); existing[first] {
			continue
		}
		t.Errorf("the skill names %q, which is not a command", path)
	}
}
