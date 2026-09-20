package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// importEnv runs the CLI against a fake Basecamp account. It knows no Minka
// host and no token: an extraction needs neither.
type importEnv struct {
	t       *testing.T
	account *fakebasecamp.Account
	now     time.Time
	env     *testEnv
}

func newImportEnv(t *testing.T) *importEnv {
	t.Helper()
	return &importEnv{
		t:       t,
		account: fakebasecamp.New(),
		now:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("CET", 3600)),
		env: &testEnv{t: t, vars: map[string]string{
			"HOME":         t.TempDir(),
			"MINKA_CONFIG": filepath.Join(t.TempDir(), "minka", "config.json"),
		}},
	}
}

func (e *importEnv) run(args ...string) result {
	e.t.Helper()
	return e.env.runWith(func(a *app) {
		a.basecampSource = e.account
		a.avatarFetcher = e.account
		a.now = func() time.Time { return e.now }
	}, "", args...)
}

func (e *importEnv) runJSON(args ...string) (envelope, result) {
	e.t.Helper()
	r := e.run(append(args, "--json")...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\nstdout: %s\nstderr: %s", err, r.stdout, r.stderr)
	}
	return env, r
}

// filesUnder reads every file under a directory, by its path in it.
func filesUnder(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		relative, _ := filepath.Rel(dir, path)
		files[filepath.ToSlash(relative)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestImportBasecampWritesABundleAndReportsWhatItHolds(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--project", "7002", "--out", out)
	expectCode(t, r, ExitOK)

	want := map[string]any{
		"source": "basecamp", "sourceInstance": "4242", "dryRun": false, "bundle": out,
		"counts":  map[string]any{"people": float64(5), "projects": float64(2), "tools": float64(5), "columns": float64(3), "attachments": float64(4)},
		"avatars": map[string]any{"count": float64(4), "initialsDistinguishable": false},
		"skippedTools": []any{
			map[string]any{"kind": "kanban_board", "enabled": false, "count": float64(1)},
			map[string]any{"kind": "schedule", "enabled": false, "count": float64(1)},
			map[string]any{"kind": "todoset", "enabled": true, "count": float64(2)},
			map[string]any{"kind": "vault", "enabled": true, "count": float64(1)},
		},
		"missingFiles": []any{}, "invalidAddresses": []any{}, "peopleLeftOut": float64(1),
		"cardsByStatus": map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
		"cardsOnHold":   float64(0), "cardsLeftOut": float64(0), "commentsTrashed": float64(0),
		"mentionsUnresolved": float64(0), "filesInBodies": float64(0), "filesDropped": float64(0),
	}
	if got := decode(t, env.Data); !reflect.DeepEqual(got, want) {
		t.Errorf("data:\n got %v\nwant %v", got, want)
	}
	if !env.OK || !strings.Contains(env.Summary, "5 people, 2 projects, 5 tools, 0 cards") {
		t.Errorf("ok %v, summary %q", env.OK, env.Summary)
	}

	var manifest map[string]any
	if err := json.Unmarshal([]byte(filesUnder(t, out)["manifest.json"]), &manifest); err != nil {
		t.Fatal(err)
	}
	wantManifest := map[string]any{
		"format": float64(1), "source": "basecamp", "sourceInstance": "4242",
		"extractedAt": "2026-01-02T02:04:05Z", "extractor": "minka " + version,
		"counts": want["counts"],
	}
	if !reflect.DeepEqual(manifest, wantManifest) {
		t.Errorf("manifest:\n got %v\nwant %v", manifest, wantManifest)
	}
}

func TestImportBasecampAsTextAndAsBareData(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	r := e.run("import", "basecamp", "--project", "7001", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"Bundle: " + out, "Source: basecamp 4242", "people       5", "attachments  4",
		"Avatars: 4, initials not distinguishable",
		"todoset          not part of Minka  2", "kanban_board     switched off       1",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}

	r = e.run("import", "basecamp", "--project", "7001", "--dry-run", "--quiet")
	expectCode(t, r, ExitOK)
	var data map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &data); err != nil || data["dryRun"] != true {
		t.Errorf("--quiet printed %s (%v)", r.stdout, err)
	}
	if _, has := data["bundle"]; has {
		t.Error("a dry run names a bundle")
	}
}

func TestADryRunWalksBasecampAndWritesNothing(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out, "--dry-run")
	expectCode(t, r, ExitOK)

	data := decode(t, env.Data)
	wantCounts := map[string]any{"people": float64(5), "projects": float64(1), "tools": float64(4), "columns": float64(3), "attachments": float64(4)}
	if !reflect.DeepEqual(data["counts"], wantCounts) {
		t.Errorf("counts %v, want %v", data["counts"], wantCounts)
	}
	if len(e.account.Requests) == 0 {
		t.Error("a dry run did not ask Basecamp for anything")
	}
	if len(e.account.AvatarRequests) != 0 {
		t.Errorf("a dry run downloaded %v", e.account.AvatarRequests)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a dry run wrote %v", entries)
	}
}

func TestASecondRunOverTheSameAccountWritesTheSameBytes(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	expectCode(t, e.run("import", "basecamp", "--project", "7001", "--project", "7002", "--out", out, "--json"), ExitOK)
	first := filesUnder(t, out)

	e.now = e.now.Add(48 * time.Hour)
	expectCode(t, e.run("import", "basecamp", "--project", "7002", "--project", "7001", "--out", out, "--json"), ExitOK)
	second := filesUnder(t, out)

	if first["manifest.json"] == second["manifest.json"] {
		t.Error("the manifest of the second run does not say when it ran")
	}
	firstManifest := strings.Replace(first["manifest.json"], "2026-01-02T02:04:05Z", "2026-01-04T02:04:05Z", 1)
	if firstManifest != second["manifest.json"] {
		t.Errorf("the manifests differ in more than extractedAt:\n%s\n%s", first["manifest.json"], second["manifest.json"])
	}
	delete(first, "manifest.json")
	delete(second, "manifest.json")
	if len(first) != 9 || !reflect.DeepEqual(first, second) {
		t.Errorf("the files of the two runs differ, or are not the 9 expected:\n%v\n%v", first, second)
	}
}

// The tool that fails is the last of its project: the project and the tools
// before it are written by then.
func TestARunThatFailsLeavesNoHalfBundle(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "made", "bundle")
	lastTool := "buckets/7001/chats/9004.json"
	e.account.Fail[lastTool] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitNetwork)
	if env.Code != CodeNetwork || !env.Retryable {
		t.Errorf("code %q, retryable %v", env.Code, env.Retryable)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a failed run left %v", entries)
	}

	delete(e.account.Fail, lastTool)
	expectCode(t, e.run("import", "basecamp", "--project", "7001", "--out", out, "--json"), ExitOK)
	before := filesUnder(t, out)

	e.account.Fail[lastTool] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
	e.now = e.now.Add(time.Hour)
	expectCode(t, e.run("import", "basecamp", "--project", "7001", "--project", "7002", "--out", out, "--json"), ExitNetwork)

	if after := filesUnder(t, out); !reflect.DeepEqual(before, after) {
		t.Errorf("a failed run changed the bundle of the run before:\n%v\n%v", before, after)
	}
	if entries, _ := os.ReadDir(filepath.Dir(out)); len(entries) != 1 {
		t.Errorf("a failed run left %v next to the bundle", entries)
	}
}

func TestARunWithFilesMissingWritesTheBundleAndSaysSoInEveryMode(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	e.account.FailAvatarOf[103] = &basecamp.Error{Kind: basecamp.API, Message: "downloading an avatar: HTTP 404"}

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitAPI)
	if env.OK || env.Code != CodeAPI || env.Retryable || !strings.Contains(env.Error, "1 file could not be fetched and is left out") || !strings.Contains(env.Hint, "missingFiles") {
		t.Errorf("ok %v, code %q, retryable %v, error %q, hint %q", env.OK, env.Code, env.Retryable, env.Error, env.Hint)
	}
	data := decode(t, env.Data)
	wantMissing := []any{map[string]any{
		"attachmentSourceId": "avatar-103", "ownerKind": "person", "ownerSourceId": "103",
		"reason": "downloading an avatar: HTTP 404", "retryable": false,
	}}
	if !reflect.DeepEqual(data["missingFiles"], wantMissing) {
		t.Errorf("missingFiles %v, want %v", data["missingFiles"], wantMissing)
	}
	wantCounts := map[string]any{"people": float64(5), "projects": float64(1), "tools": float64(4), "columns": float64(3), "attachments": float64(3)}
	if !reflect.DeepEqual(data["counts"], wantCounts) || data["bundle"] != out {
		t.Errorf("counts %v, bundle %v", data["counts"], data["bundle"])
	}

	files := filesUnder(t, out)
	if !strings.Contains(files["manifest.json"], `"attachments": 3`) || strings.Contains(files["attachments.jsonl"], "avatar-103") {
		t.Errorf("the bundle:\n%s\n%s", files["manifest.json"], files["attachments.jsonl"])
	}
	if !strings.Contains(files["people.jsonl"], `"sourceId":"103","createdAt":"2020-03-01T08:00:00.000Z","updatedAt":"2024-03-01T08:00:00.000Z","createdBy":null,"name":"Linus Placeholder","email":null,"avatar":null`) {
		t.Errorf("the person keeps a reference to a file that is not there:\n%s", files["people.jsonl"])
	}

	// One file that may come next time makes the whole run worth repeating.
	e.account.FailAvatarOf[101] = &basecamp.Error{Kind: basecamp.Network, Message: "downloading an avatar: connection reset"}
	env, r = e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitNetwork)
	if env.Code != CodeNetwork || !env.Retryable || !strings.Contains(env.Error, "2 files could not be fetched and are left out") {
		t.Errorf("code %q, retryable %v, error %q", env.Code, env.Retryable, env.Error)
	}

	r = e.run("import", "basecamp", "--project", "7001", "--out", out, "--quiet")
	expectCode(t, r, ExitNetwork)
	var quiet envelope
	if err := json.Unmarshal([]byte(r.stdout), &quiet); err != nil || quiet.Code != CodeNetwork || len(decode(t, quiet.Data)["missingFiles"].([]any)) != 2 {
		t.Errorf("--quiet printed %s (%v)", r.stdout, err)
	}

	r = e.run("import", "basecamp", "--project", "7001", "--out", out, "--styled")
	expectCode(t, r, ExitNetwork)
	for _, want := range []string{"Bundle: " + out, "attachments  2", "Missing files:", "avatar-103 of person 103: downloading an avatar: HTTP 404"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
	if !strings.Contains(r.stderr, "Error: the bundle was written, but 2 files") {
		t.Errorf("stderr: %s", r.stderr)
	}

	// The next run brings what was missing.
	e.account.FailAvatarOf = map[int64]error{}
	env, r = e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitOK)
	if !strings.Contains(filesUnder(t, out)["attachments.jsonl"], "avatar-103") {
		t.Error("the file that was missing did not come with the next run")
	}
}

func TestAnOldBundleThatCannotBeRemovedIsANoticeNotAFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("nothing is closed to root")
	}
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	expectCode(t, e.run("import", "basecamp", "--project", "7001", "--out", out, "--json"), ExitOK)

	closed := filepath.Join(out, "files", "avatar-101")
	if err := os.Chmod(closed, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(out), "*", "files", "avatar-101"))
		hidden, _ := filepath.Glob(filepath.Join(filepath.Dir(out), ".*", "files", "avatar-101"))
		for _, dir := range append(leftovers, hidden...) {
			_ = os.Chmod(dir, 0o700)
		}
	})

	e.now = e.now.Add(time.Hour)
	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, filepath.Dir(out)) || !strings.Contains(env.Notice, ".previous") {
		t.Errorf("notice %q does not name where the old bundle lies", env.Notice)
	}
	if !strings.Contains(filesUnder(t, out)["manifest.json"], "2026-01-02T03:04:05Z") {
		t.Error("the new bundle is not in place")
	}
}

func TestImportRefusesADirectoryThatHoldsSomethingElse(t *testing.T) {
	e := newImportEnv(t)
	out := t.TempDir()
	if err := os.WriteFile(filepath.Join(out, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || !strings.Contains(env.Hint, "--out") {
		t.Errorf("code %q, hint %q", env.Code, env.Hint)
	}
	if len(e.account.Requests) != 0 {
		t.Errorf("Basecamp was asked for %v before the directory was looked at", e.account.Requests)
	}
	if got := filesUnder(t, out); !reflect.DeepEqual(got, map[string]string{"notes.txt": "mine"}) {
		t.Errorf("the directory now holds %v", got)
	}

	// A dry run says so beforehand.
	env, r = e.runJSON("import", "basecamp", "--project", "7001", "--out", out, "--dry-run")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || len(e.account.Requests) != 0 {
		t.Errorf("a dry run: code %q, asked Basecamp for %v", env.Code, e.account.Requests)
	}
}

func TestImportRefusesADirectoryWithTheManifestOfSomethingElse(t *testing.T) {
	for name, manifest := range map[string]string{
		"a web application": `{"name":"An app","start_url":"/"}`,
		"another source":    `{"format":1,"source":"jira"}`,
	} {
		t.Run(name, func(t *testing.T) {
			e := newImportEnv(t)
			out := t.TempDir()
			held := map[string]string{"manifest.json": manifest, "index.html": "<html>"}
			for file, content := range held {
				if err := os.WriteFile(filepath.Join(out, file), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
			expectCode(t, r, ExitUsage)
			if env.Code != CodeUsage {
				t.Errorf("code %q", env.Code)
			}
			if got := filesUnder(t, out); !reflect.DeepEqual(got, held) {
				t.Errorf("the directory now holds %v", got)
			}
		})
	}
}

func TestImportFailuresHaveTheCodeOfTheirClass(t *testing.T) {
	cases := []struct {
		name      string
		kind      basecamp.Kind
		code      string
		exit      int
		retryable bool
		hint      string
	}{
		{"the Basecamp CLI is missing", basecamp.NotInstalled, CodeConfig, ExitUsage, false, "basecamp auth login"},
		{"not signed in", basecamp.AuthRequired, CodeAuth, ExitAuth, false, "basecamp auth login"},
		{"forbidden", basecamp.Forbidden, CodeForbidden, ExitForbidden, false, ""},
		{"rate limited", basecamp.RateLimited, CodeAPI, ExitAPI, true, ""},
		{"network", basecamp.Network, CodeNetwork, ExitNetwork, true, ""},
		{"interrupted", basecamp.Interrupted, CodeNetwork, ExitNetwork, false, ""},
		{"API", basecamp.API, CodeAPI, ExitAPI, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newImportEnv(t)
			parent := t.TempDir()
			e.account.Fail["projects/7001/people.json"] = &basecamp.Error{Kind: c.kind, Message: "what went wrong"}

			env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", filepath.Join(parent, "made", "deeper", "bundle"))
			expectCode(t, r, c.exit)
			if env.OK || env.Code != c.code || env.Retryable != c.retryable || env.Error != "what went wrong" {
				t.Errorf("ok %v, code %q, retryable %v, error %q", env.OK, env.Code, env.Retryable, env.Error)
			}
			if !strings.Contains(env.Hint, c.hint) {
				t.Errorf("hint %q, want it to name %q", env.Hint, c.hint)
			}
			if entries, _ := os.ReadDir(parent); len(entries) != 0 {
				t.Errorf("left %v", entries)
			}
		})
	}
}

func TestImportOfAProjectThatIsNotThereIsNotFound(t *testing.T) {
	e := newImportEnv(t)
	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--project", "8888", "--dry-run")
	expectCode(t, r, ExitNotFound)
	if env.Code != CodeNotFound || !strings.Contains(env.Error, "8888") {
		t.Errorf("code %q, error %q", env.Code, env.Error)
	}
}

func TestImportChecksTheCommandLineBeforeAskingBasecamp(t *testing.T) {
	parent := t.TempDir()
	out := filepath.Join(parent, "made", "bundle")
	for name, args := range map[string][]string{
		"no project":         {"import", "basecamp", "--out", out},
		"no directory":       {"import", "basecamp", "--project", "7001"},
		"a name as project":  {"import", "basecamp", "--project", "Example Project", "--out", out},
		"a path as project":  {"import", "basecamp", "--project", "7001/../../people", "--out", out},
		"a flag as project":  {"import", "basecamp", "--project=--help", "--out", out},
		"a number too long":  {"import", "basecamp", "--project", "99999999999999999999999", "--out", out},
		"one of two is none": {"import", "basecamp", "--project", "7001", "--project", "1234567890123456789", "--out", out},
		"an argument":        {"import", "basecamp", "7001", "--out", out},
		"an unknown command": {"import", "jira"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newImportEnv(t)
			env, r := e.runJSON(args...)
			expectCode(t, r, ExitUsage)
			if env.Code != CodeUsage {
				t.Errorf("code %q", env.Code)
			}
			if len(e.account.Requests) != 0 {
				t.Errorf("Basecamp was asked for %v", e.account.Requests)
			}
			if entries, _ := os.ReadDir(parent); len(entries) != 0 {
				t.Errorf("directories were made on the way to the bundle: %v", entries)
			}
		})
	}
}

func TestImportNeedsNoMinkaHostOrToken(t *testing.T) {
	e := newImportEnv(t)
	for _, name := range []string{"MINKA_HOST", "MINKA_TOKEN"} {
		if e.env.vars[name] != "" {
			t.Fatalf("$%s is set", name)
		}
	}
	if _, err := os.Stat(e.env.vars["MINKA_CONFIG"]); err == nil {
		t.Fatal("there is a config file")
	}

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--dry-run")
	expectCode(t, r, ExitOK)
	if !env.OK {
		t.Errorf("stdout: %s", r.stdout)
	}
}

func TestACommandIsEndedByCtrlCAndByASupervisor(t *testing.T) {
	for _, want := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		if !slices.Contains(Signals, want) {
			t.Errorf("%v does not cancel a command, so it would leave what it was writing", want)
		}
	}
}

func TestImportRefusesALinkAsTheDirectoryOfTheBundle(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	behind := filepath.Join(parent, "behind")
	expectCode(t, e.run("import", "basecamp", "--project", "7001", "--out", behind, "--json"), ExitOK)
	before := filesUnder(t, behind)
	link := filepath.Join(parent, "bundle")
	if err := os.Symlink(behind, link); err != nil {
		t.Fatal(err)
	}
	e.account.Requests = nil
	e.now = e.now.Add(time.Hour)

	for _, args := range [][]string{
		{"import", "basecamp", "--project", "7001", "--out", link},
		{"import", "basecamp", "--project", "7001", "--out", link, "--dry-run"},
	} {
		env, r := e.runJSON(args...)
		expectCode(t, r, ExitUsage)
		if env.Code != CodeConfig || !strings.Contains(env.Error, "symbolic link") || !strings.Contains(env.Hint, "--out") {
			t.Errorf("code %q, error %q, hint %q", env.Code, env.Error, env.Hint)
		}
	}
	if len(e.account.Requests) != 0 {
		t.Errorf("Basecamp was asked for %v", e.account.Requests)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced")
	}
	if after := filesUnder(t, behind); !reflect.DeepEqual(before, after) {
		t.Error("the bundle behind the link changed")
	}
}

func TestImportRefusesADirectoryAnotherRunIsWriting(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	other, err := bundle.Create(out, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Abort()

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig || env.Retryable || !strings.Contains(env.Error, filepath.Join(parent, ".bundle.lock")) {
		t.Errorf("code %q, retryable %v, error %q", env.Code, env.Retryable, env.Error)
	}
	if len(e.account.Requests) != 0 {
		t.Errorf("Basecamp was asked for %v", e.account.Requests)
	}

	// The other run goes on as if nothing had happened.
	if err := other.Add(bundle.People, bundle.Person{Name: "Ada Example"}); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Commit(bundle.Manifest{}); err != nil {
		t.Fatalf("the run that held the lock failed: %v", err)
	}
}

func TestMaskedAddressesAreForbiddenAndNeverShown(t *testing.T) {
	const local = "grace"
	masked := "g\u2022\u2022\u2022\u2022@\u2022\u2022\u2022\u2022\u2022\u2022\u2022.\u2022\u2022\u2022\u2022"

	for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
		e := newImportEnv(t)
		e.account.People[0].Email = masked
		e.account.People[3].Email = masked
		parent := t.TempDir()

		r := e.run(append([]string{"import", "basecamp", "--project", "7001", "--out", filepath.Join(parent, "made", "bundle")}, mode...)...)
		expectCode(t, r, ExitForbidden)
		output := r.stdout + r.stderr
		for _, want := range []string{"2 of 5", "may not see the e-mail addresses"} {
			if !strings.Contains(output, want) {
				t.Errorf("%v: the output lacks %q:\n%s", mode, want, output)
			}
		}
		for _, secret := range []string{masked, "\u2022", local, "Grace", "Linus", "102", "103", "@"} {
			if strings.Contains(output, secret) {
				t.Errorf("%v: the output names %q:\n%s", mode, secret, output)
			}
		}
		if mode[0] != "--styled" {
			var env envelope
			if err := json.Unmarshal([]byte(r.stdout), &env); err != nil || env.Code != CodeForbidden || env.Retryable {
				t.Errorf("%v: %s (%v)", mode, r.stdout, err)
			}
		}
		if entries, _ := os.ReadDir(parent); len(entries) != 0 {
			t.Errorf("%v: left %v", mode, entries)
		}
	}
}

func TestSomebodyWithAnAddressThatIsNoneComesWithoutALoginAndTheRunSucceeds(t *testing.T) {
	const quoted = `"linus@example.test"`
	for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
		e := newImportEnv(t)
		e.account.People[3].Email = quoted                                   // 103, a member
		e.account.People[0].Email = "grace at example"                       // 102, a member
		e.account.Projects[0].Dock[2].Creator.Email = "former dot colleague" // 99, a creator who left
		out := filepath.Join(t.TempDir(), "bundle")

		r := e.run(append([]string{"import", "basecamp", "--project", "7001", "--out", out}, mode...)...)
		expectCode(t, r, ExitOK)
		output := r.stdout + r.stderr
		for _, secret := range []string{"linus@", "example.test\"", "grace at", "former dot", "Linus", "Grace", "Former", "Niklaus"} {
			if strings.Contains(output, secret) {
				t.Errorf("%v: the output names %q:\n%s", mode, secret, output)
			}
		}
		if !strings.Contains(output, "3 people come without an e-mail address") {
			t.Errorf("%v: no notice says how many came without an address:\n%s", mode, output)
		}

		switch mode[0] {
		case "--styled":
			if !strings.Contains(r.stdout, "Invalid addresses: 3 (person 99, 102, 103)") || !strings.Contains(r.stdout, "left out: 1") {
				t.Errorf("the text:\n%s", r.stdout)
			}
		case "--quiet":
			var data map[string]any
			if err := json.Unmarshal([]byte(r.stdout), &data); err != nil || !reflect.DeepEqual(data["invalidAddresses"], []any{"99", "102", "103"}) {
				t.Errorf("--quiet printed %s (%v)", r.stdout, err)
			}
		default:
			var env envelope
			if err := json.Unmarshal([]byte(r.stdout), &env); err != nil || !env.OK || !strings.Contains(env.Notice, "3 people") {
				t.Fatalf("%v: %s (%v)", mode, r.stdout, err)
			}
			if got := decode(t, env.Data)["invalidAddresses"]; !reflect.DeepEqual(got, []any{"99", "102", "103"}) {
				t.Errorf("%v: invalidAddresses %v, want the IDs in the order of the numbers", mode, got)
			}
		}

		if len(mode) == 1 {
			people := filesUnder(t, out)["people.jsonl"]
			if strings.Contains(people, "linus@") || strings.Count(people, `"email":null`) != 3 {
				t.Errorf("%v: people.jsonl:\n%s", mode, people)
			}
		}
	}
}

func TestMostAddressesBeingNoneEndsTheRunWithNothingWritten(t *testing.T) {
	e := newImportEnv(t)
	for _, i := range []int{0, 3, 4} {
		e.account.People[i].Email = "x***@*****.***"
	}
	parent := t.TempDir()
	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", filepath.Join(parent, "made", "bundle"))
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || !strings.Contains(env.Error, "3 of 5") || !strings.Contains(env.Hint, "identity") || strings.Contains(r.stdout, "***") {
		t.Errorf("code %q, error %q, hint %q", env.Code, env.Error, env.Hint)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("left %v", entries)
	}
}

func TestPeopleNothingRefersToAreLeftOutAndCounted(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	env, r := e.runJSON("import", "basecamp", "--project", "7002", "--out", out)
	expectCode(t, r, ExitOK)

	data := decode(t, env.Data)
	if data["peopleLeftOut"] != float64(4) || decode(t, env.Data)["counts"].(map[string]any)["people"] != float64(1) {
		t.Errorf("left out %v, counts %v", data["peopleLeftOut"], data["counts"])
	}
	files := filesUnder(t, out)
	if strings.Count(files["people.jsonl"], "\n") != 1 || !strings.Contains(files["people.jsonl"], "Ada Example") {
		t.Errorf("people.jsonl:\n%s", files["people.jsonl"])
	}
	if len(e.account.AvatarRequests) != 1 {
		t.Errorf("downloaded %v", e.account.AvatarRequests)
	}
}

func TestImportBasecampReportsTheBoardsOfAProject(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7003", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := map[string]any{
		"counts": map[string]any{"people": float64(6), "projects": float64(1), "tools": float64(2), "columns": float64(8),
			"cards": float64(10), "steps": float64(3), "comments": float64(3), "reactions": float64(3), "attachments": float64(5)},
		"cardsByStatus": map[string]any{"active": float64(7), "archived": float64(2), "trashed": float64(1)},
		"cardsOnHold":   float64(2), "cardsLeftOut": float64(1),
		"mentionsUnresolved": float64(1), "filesInBodies": float64(2), "filesDropped": float64(1),
	}
	for key, value := range want {
		if !reflect.DeepEqual(data[key], value) {
			t.Errorf("%s: got %v, want %v", key, data[key], value)
		}
	}
	if !strings.Contains(env.Summary, "6 people, 1 project, 2 tools, 10 cards") {
		t.Errorf("summary %q", env.Summary)
	}
	if !strings.Contains(env.Notice, "1 file embedded in a text") {
		t.Errorf("notice %q does not say that a file was dropped from a text", env.Notice)
	}

	files := filesUnder(t, out)
	for name, lines := range map[string]int{"columns.jsonl": 8, "cards.jsonl": 10, "steps.jsonl": 3, "comments.jsonl": 3, "reactions.jsonl": 3} {
		if got := strings.Count(files[name], "\n"); got != lines {
			t.Errorf("%s holds %d lines, want %d", name, got, lines)
		}
	}
	if !strings.Contains(files["manifest.json"], `"cards": 10`) {
		t.Errorf("the manifest:\n%s", files["manifest.json"])
	}

	r = e.run("import", "basecamp", "--project", "7003", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"cards        10", "reactions    3",
		"Cards: 7 active, 2 archived, 1 trashed; 2 of them on hold",
		"cards whose column is in no extracted board, left out: 1",
		"Comments in the trash, left out: 0",
		"In the texts: 2 files referred to, 1 file Basecamp lists nothing for dropped, 1 mention left as a name",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
	if !strings.Contains(r.stderr, "Warning: 1 file embedded in a text") {
		t.Errorf("stderr: %s", r.stderr)
	}
}

func TestADryRunCountsTheBoardsTheWayARealRunDoes(t *testing.T) {
	e := newImportEnv(t)
	real, r := e.runJSON("import", "basecamp", "--project", "7003", "--out", filepath.Join(t.TempDir(), "bundle"))
	expectCode(t, r, ExitOK)

	parent := t.TempDir()
	dry, r := e.runJSON("import", "basecamp", "--project", "7003", "--out", filepath.Join(parent, "bundle"), "--dry-run")
	expectCode(t, r, ExitOK)

	want, got := decode(t, real.Data), decode(t, dry.Data)
	delete(want, "bundle")
	want["dryRun"] = true
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a dry run reports\n%v\na real one\n%v", got, want)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a dry run wrote %v", entries)
	}
}

// What people wrote is in the bundle and nowhere else: no output says what a
// card is called, who is on it or what stands in it.
func TestNoOutputSaysWhatStandsOnABoard(t *testing.T) {
	written := []string{
		"Roadmap", "Doing", "Inbox", "Draw the map", "Buy paper", "Outline the coast", "Fish", "Settled", "Well done", "🎉",
		"Gone Person", "Grace", "Niklaus", "Former", "sketch.png", "lost.png", "plan.png", "example.test", "tea",
	}
	failures := map[string]func(*fakebasecamp.Account){
		"nothing fails":         func(*fakebasecamp.Account) {},
		"a second Done":         func(a *fakebasecamp.Account) { a.Projects[2].Dock[0].Lists[1].Type = fakebasecamp.Done },
		"a card without a time": func(a *fakebasecamp.Account) { a.Projects[2].Dock[0].Lists[2].Cards[0].UpdatedAt = "" },
		"a boost of nobody":     func(a *fakebasecamp.Account) { a.Projects[2].Dock[0].Lists[2].Cards[0].Boosts[0].Booster = nil },
		"a comment at no time": func(a *fakebasecamp.Account) {
			a.Projects[2].Dock[0].Lists[2].Cards[0].Comments[0].CreatedAt = "when Grace wrote it"
		},
		"comments that fail": func(a *fakebasecamp.Account) {
			a.Fail["buckets/7003/recordings/9201/comments.json"] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
		},
	}
	for name, change := range failures {
		for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
			e := newImportEnv(t)
			change(e.account)
			r := e.run(append([]string{"import", "basecamp", "--project", "7003", "--out", filepath.Join(t.TempDir(), "bundle")}, mode...)...)
			if (r.code == ExitOK) != (name == "nothing fails") {
				t.Errorf("%s %v: exit %d", name, mode, r.code)
			}
			for _, text := range written {
				if strings.Contains(r.stdout+r.stderr, text) {
					t.Errorf("%s %v: the output says %q:\n%s%s", name, mode, text, r.stdout, r.stderr)
				}
			}
		}
	}
}

func TestTheReportCountsWhatATextLostAndWhatWasInTheTrash(t *testing.T) {
	e := newImportEnv(t)
	comments := e.account.Projects[2].Dock[0].Lists[2].Cards[0].Comments
	comments[0].Status = "trashed"
	comments[2].Content += fakebasecamp.EmbeddedFile(fakebasecamp.SGID("a file nobody lists"), "gone.png")

	env, r := e.runJSON("import", "basecamp", "--project", "7003", "--dry-run")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["commentsTrashed"] != float64(1) || data["filesDropped"] != float64(2) || data["counts"].(map[string]any)["comments"] != float64(2) {
		t.Errorf("commentsTrashed %v, filesDropped %v, counts %v", data["commentsTrashed"], data["filesDropped"], data["counts"])
	}
	if !strings.Contains(env.Notice, "2 files embedded in texts") {
		t.Errorf("notice %q", env.Notice)
	}

	r = e.run("import", "basecamp", "--project", "7003", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Comments in the trash, left out: 1", "2 files Basecamp lists nothing for dropped, 1 mention left as a name"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
}
