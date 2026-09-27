package cli

import (
	"context"
	"encoding/json"
	"errors"
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
	// source stands between the CLI and the account, where a test needs more
	// than the account does by itself.
	source basecamp.Source
	// pauses are the pauses a run asked for before it asked a request again.
	// No test waits for one.
	pauses []time.Duration
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
		if e.source != nil {
			a.basecampSource = e.source
		}
		a.avatarFetcher = e.account
		a.basecampFiles = e.account
		a.now = func() time.Time { return e.now }
		a.retryPause = func(ctx context.Context, pause time.Duration) error {
			e.pauses = append(e.pauses, pause)
			return ctx.Err()
		}
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
		"counts":  map[string]any{"people": float64(5), "projects": float64(2), "tools": float64(6), "columns": float64(3), "attachments": float64(4)},
		"avatars": map[string]any{"count": float64(4), "initialsDistinguishable": false},
		"skippedTools": []any{
			map[string]any{"kind": "kanban_board", "enabled": false, "count": float64(1)},
			map[string]any{"kind": "schedule", "enabled": false, "count": float64(1)},
			map[string]any{"kind": "todoset", "enabled": false, "count": float64(1)},
			map[string]any{"kind": "vault", "enabled": false, "count": float64(1)},
		},
		"missingFiles": []any{}, "invalidAddresses": []any{}, "addressesUnquoted": float64(0), "peopleLeftOut": float64(8),
		"creatorProbesRefused": float64(0),
		"peopleAsClients":      float64(0), "clientMemberships": float64(0),
		"visibilityLeftToTheContainer": float64(0),
		"cardsByStatus":                map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
		"cardsOnHold":                  float64(0), "cardsLeftOut": float64(0), "commentsTrashed": float64(0),
		"todoListsByStatus": map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
		"todosByStatus":     map[string]any{"active": float64(0), "trashed": float64(0)},
		"todosNotDone":      float64(0), "todosWithPeople": float64(0),
		"todosWithSubtasks": float64(0), "subtasksLeftOut": float64(0), "todosThatRepeat": float64(0),
		"todosLeftOut":      map[string]any{"lists": float64(0), "groups": float64(0), "todos": float64(0)},
		"todoPlacesLeftOut": []any{}, "todosWithoutAList": []any{},
		"mentionsUnresolved": float64(0), "filesInBodies": float64(0), "filesDropped": float64(0), "linkPreviews": float64(0), "picturesLinked": float64(0), "embeds": float64(0), "embedsLeftOut": float64(0),
		"filesExpected": float64(0), "filesBytesExpected": float64(0), "filesBytes": float64(0), "filesFetched": float64(0), "filesReused": float64(0),
		"filesGone": float64(0), "fileNamesChanged": float64(0), "captionsDropped": float64(0), "filesOfSeveralOwners": float64(0),
		"postsByStatus": map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
		"postsLeftOut":  float64(0), "postsWithEmptyBody": float64(0),
		"messagesByType":     map[string]any{"text": float64(0), "richText": float64(0), "upload": float64(0), "integration": float64(0)},
		"uploadLinesLeftOut": float64(0), "messagesWithEmptyBody": float64(0), "recordsLeftOut": []any{},
		"questionsLeftOut": []any{}, "anchorsInferred": []any{}, "nobodyAsked": []any{},
		"askedLeftOut": map[string]any{"notInTheProject": float64(0), "clients": float64(0),
			"retired": float64(0), "withoutAnAddress": float64(0)},
		"answersWithoutABody": float64(0), "answersWithoutADay": float64(0), "answersTrashed": float64(0),
		"forwards": map[string]any{
			"byStatus": map[string]any{"active": float64(0), "trashed": float64(0)}, "fromTheShelf": float64(0),
			"withoutASubject": float64(0), "subjectsChanged": float64(0), "givenALineOfTheRun": float64(0),
			"withoutAForwarder": float64(0), "withoutAnAddress": []any{},
			"repliesFromOutside": float64(0), "repliesWithoutASender": []any{}, "reactionsOnRepliesLeftOut": float64(0),
			"longerThanMinkaTakes": float64(0)},
		"docs": map[string]any{
			"foldersByStatus":       map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
			"documentsByStatus":     map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
			"uploadsByStatus":       map[string]any{"active": float64(0), "archived": float64(0), "trashed": float64(0)},
			"documentsWithoutWords": float64(0), "titlesFromTheFileName": float64(0),
			"namesMadeIntoATitle": float64(0), "withoutAName": float64(0),
			"withoutAStoredFile": []any{}, "commentsWithoutTheirUpload": float64(0), "boostsWithoutTheirUpload": float64(0),
			"onlyAFileNoRunWillBring": []any{}, "foldersThatSaidNothingOfWhatIsInThem": float64(0),
			"silentAboutClientsInsideAFolder": []any{},
			"longerThanMinkaTakes":            float64(0),
			"leftOut":                         map[string]any{"folders": float64(0), "documents": float64(0), "uploads": float64(0)},
			"placesLeftOut":                   []any{}},
		"longestBody": map[string]any{"cards": float64(0), "comments": float64(0), "posts": float64(0), "messages": float64(0), "answers": float64(0),
			"todolists": float64(0), "todos": float64(0), "forwards": float64(0), "forwardreplies": float64(0),
			"documents": float64(0), "uploads": float64(0)},
		"longestBodyRecords": map[string]any{},
		"references":         map[string]any{"cards": float64(0), "posts": float64(0), "tools": float64(0)},
		"requestsRetried":    float64(0),
	}
	if got := decode(t, env.Data); !reflect.DeepEqual(got, want) {
		t.Errorf("data:\n got %v\nwant %v", got, want)
	}
	if !env.OK || !strings.Contains(env.Summary, "5 people, 2 projects, 6 tools, 0 cards, 0 posts, 0 messages") {
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
		"todoset          switched off       1", "kanban_board     switched off       1",
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
	wantCounts := map[string]any{"people": float64(5), "projects": float64(1), "tools": float64(5), "columns": float64(3), "attachments": float64(4)}
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
	wantCounts := map[string]any{"people": float64(5), "projects": float64(1), "tools": float64(5), "columns": float64(3), "attachments": float64(3)}
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
		// asked is how often the request went out: once, or as often as a
		// request is tried when its failure may pass.
		asked int
	}{
		{"the Basecamp CLI is missing", basecamp.NotInstalled, CodeConfig, ExitUsage, false, "basecamp auth login", 1},
		{"not found", basecamp.NotFound, CodeNotFound, ExitNotFound, false, "", 1},
		{"not signed in", basecamp.AuthRequired, CodeAuth, ExitAuth, false, "basecamp auth login", 1},
		{"forbidden", basecamp.Forbidden, CodeForbidden, ExitForbidden, false, "", 1},
		{"rate limited", basecamp.RateLimited, CodeAPI, ExitAPI, true, "", 4},
		{"network", basecamp.Network, CodeNetwork, ExitNetwork, true, "", 4},
		{"Basecamp or a gateway failed", basecamp.Unavailable, CodeAPI, ExitAPI, true, "try again", 4},
		{"interrupted", basecamp.Interrupted, CodeNetwork, ExitNetwork, false, "", 1},
		{"API", basecamp.API, CodeAPI, ExitAPI, false, "", 1},
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
			if asked := timesAsked(e.account.Requests, "projects/7001/people.json?page=1"); asked != c.asked {
				t.Errorf("asked %d times, want %d", asked, c.asked)
			}
			if want := basecamp.RetryPauses[:c.asked-1]; !slices.Equal(e.pauses, want) {
				t.Errorf("waited %v, want %v", e.pauses, want)
			}
		})
	}
}

// timesAsked is how often one request went out.
func timesAsked(requests []string, wanted string) int {
	count := 0
	for _, request := range requests {
		if request == wanted {
			count++
		}
	}
	return count
}

func TestARequestThatFailsAndThenAnswersDoesNotCostTheRun(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	e.account.Fail["projects/7001/people.json"] = &basecamp.Error{Kind: basecamp.Unavailable, Message: "Basecamp: Request failed (HTTP 525)"}
	e.account.FailFirst["projects/7001/people.json"] = 2

	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", out)
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["requestsRetried"]; got != float64(2) {
		t.Errorf("requestsRetried %v, want 2", got)
	}
	if asked := timesAsked(e.account.Requests, "projects/7001/people.json?page=1"); asked != 3 {
		t.Errorf("asked %d times, want 3", asked)
	}
	if want := []time.Duration{2 * time.Second, 10 * time.Second}; !slices.Equal(e.pauses, want) {
		t.Errorf("waited %v, want %v", e.pauses, want)
	}
	if strings.Contains(r.stdout, "525") || strings.Contains(r.stdout, "people.json") {
		t.Errorf("a run that ended well names the failure or the request: %s", r.stdout)
	}

	e.account.FailFirst["projects/7001/people.json"] = 1
	r = e.run("import", "basecamp", "--project", "7001", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	if want := "Requests asked again after a failure that passed: 1\n"; !strings.Contains(r.stdout, want) {
		t.Errorf("the text lacks %q:\n%s", want, r.stdout)
	}

	r = e.run("import", "basecamp", "--project", "7001", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "asked again") {
		t.Errorf("a run without a failure talks of one:\n%s", r.stdout)
	}
}

// stumbling fails every request the first times it is asked, of whatever kind
// it is, and passes it on after that: once, when times is 0.
type stumbling struct {
	account *fakebasecamp.Account
	times   int
	asked   map[string]int
	failed  int
}

func (s *stumbling) Get(ctx context.Context, path string) ([]byte, error) {
	if s.asked == nil {
		s.asked = map[string]int{}
	}
	if s.asked[path] < max(s.times, 1) {
		s.asked[path]++
		s.failed++
		return nil, &basecamp.Error{Kind: basecamp.Unavailable, Message: "Basecamp: Gateway error (502)"}
	}
	return s.account.Get(ctx, path)
}

// A project per run: every request of a run stumbles, and a run asks no more
// requests again than MaxRetriesOfARun.
func TestARunWhoseEveryRequestIsAskedAgainWritesTheSameBytes(t *testing.T) {
	files, stumbled := 0, 0
	for _, project := range []string{"7001", "7003", "7004"} {
		calm := newImportEnv(t)
		calmOut := filepath.Join(t.TempDir(), "bundle")
		calmEnv, r := calm.runJSON("import", "basecamp", "--out", calmOut, "--project", project)
		expectCode(t, r, ExitOK)

		e := newImportEnv(t)
		source := &stumbling{account: e.account}
		e.source = source
		out := filepath.Join(t.TempDir(), "bundle")
		env, r := e.runJSON("import", "basecamp", "--out", out, "--project", project)
		expectCode(t, r, ExitOK)

		want, got := filesUnder(t, calmOut), filesUnder(t, out)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("project %s: the bundles differ:\n got %v\nwant %v", project, got, want)
		}
		if !slices.Equal(e.account.Requests, calm.account.Requests) {
			t.Errorf("project %s: the account was asked for other things, or in another order", project)
		}

		// Every request of every kind stumbled once: none goes round the retries.
		if source.failed > basecamp.MaxRetriesOfARun {
			t.Fatalf("project %s takes %d requests, more than a run asks again: the test needs a smaller walk", project, source.failed)
		}
		if source.failed != len(calm.account.Requests) {
			t.Fatalf("project %s: %d requests failed once, of %d the run makes", project, source.failed, len(calm.account.Requests))
		}
		data, calmData := decode(t, env.Data), decode(t, calmEnv.Data)
		if data["requestsRetried"] != float64(source.failed) || calmData["requestsRetried"] != float64(0) {
			t.Errorf("project %s: requestsRetried %v and %v, want %d and 0", project, data["requestsRetried"], calmData["requestsRetried"], source.failed)
		}
		for _, pause := range e.pauses {
			if pause != basecamp.RetryPauses[0] {
				t.Fatalf("project %s: waited %v before a second attempt", project, pause)
			}
		}
		delete(data, "requestsRetried")
		delete(calmData, "requestsRetried")
		data["bundle"], calmData["bundle"] = "", ""
		if !reflect.DeepEqual(data, calmData) {
			t.Errorf("project %s: the reports differ in more than the retries:\n got %v\nwant %v", project, data, calmData)
		}
		files, stumbled = files+len(want), stumbled+source.failed
	}
	if files < 20 || stumbled < 50 {
		t.Errorf("%d files and %d requests are too little to tell", files, stumbled)
	}
}

// Every request passes on its third attempt: no request uses up its attempts,
// and the run would take the pauses of all of them.
func TestARunWhoseSourceKeepsFailingEndsInsteadOfWaitingForEveryRequest(t *testing.T) {
	for name, args := range map[string][]string{
		"a run":     {"--out", "bundle"},
		"a dry run": {"--dry-run"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newImportEnv(t)
			source := &stumbling{account: e.account, times: 2}
			e.source = source
			parent := t.TempDir()
			if args[0] == "--out" {
				args = []string{"--out", filepath.Join(parent, args[1])}
			}

			env, r := e.runJSON(append([]string{"import", "basecamp", "--project", "7003", "--project", "7004"}, args...)...)
			expectCode(t, r, ExitAPI)
			if env.Code != CodeAPI || !env.Retryable {
				t.Errorf("code %q, retryable %v", env.Code, env.Retryable)
			}
			if env.Error != "Basecamp: Gateway error (502); Basecamp kept failing: 100 requests of this run were asked again already, and no further one is" {
				t.Errorf("the message does not say that Basecamp kept failing: %q", env.Error)
			}
			if len(e.pauses) != 100 || source.failed != 101 {
				t.Errorf("waited %d times and met %d failures, want 100 and one more", len(e.pauses), source.failed)
			}
			if entries, _ := os.ReadDir(parent); len(entries) != 0 {
				t.Errorf("left %v", entries)
			}
		})
	}
}

func TestADryRunAsksAgainTheWayARealRunDoes(t *testing.T) {
	projects := []string{"--project", "7004"}

	e := newImportEnv(t)
	source := &stumbling{account: e.account}
	e.source = source
	parent := t.TempDir()
	env, r := e.runJSON(append([]string{"import", "basecamp", "--out", filepath.Join(parent, "bundle"), "--dry-run"}, projects...)...)
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["requestsRetried"]; source.failed < 20 || got != float64(source.failed) {
		t.Errorf("requestsRetried %v, want %d", got, source.failed)
	}
	if len(e.pauses) != source.failed {
		t.Errorf("waited %d times, want %d", len(e.pauses), source.failed)
	}

	failing := newImportEnv(t)
	failing.account.Fail["projects/7004/people.json"] = &basecamp.Error{Kind: basecamp.Unavailable, Message: "Basecamp: Request failed (HTTP 525)"}
	env, r = failing.runJSON(append([]string{"import", "basecamp", "--dry-run"}, projects...)...)
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || !env.Retryable || env.Error != "Basecamp: Request failed (HTTP 525)" {
		t.Errorf("code %q, retryable %v, error %q", env.Code, env.Retryable, env.Error)
	}
	if asked := timesAsked(failing.account.Requests, "projects/7004/people.json?page=1"); asked != 4 {
		t.Errorf("asked %d times, want 4", asked)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a dry run wrote %v", entries)
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
		for _, want := range []string{"2 of 10", "may not see the e-mail addresses"} {
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
	for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
		e := newImportEnv(t)
		e.account.People[3].Email = "linus@exämple.test"                     // 103, a member
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
			if !strings.Contains(r.stdout, "Invalid addresses: 3 (person 99, 102, 103)") || !strings.Contains(r.stdout, "left out: 8") {
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

// A pair of quotation marks around the whole of an address comes off, which is
// the one thing a run repairs rather than passes on. The run says how often it
// did, and names nobody it did it for.
func TestAnAddressInQuotationMarksComesOutWithoutThemAndIsCounted(t *testing.T) {
	for _, mode := range [][]string{{"--json"}, {"--styled"}} {
		e := newImportEnv(t)
		e.account.People[3].Email = `"linus@example.test"` // 103, a member
		out := filepath.Join(t.TempDir(), "bundle")

		r := e.run(append([]string{"import", "basecamp", "--project", "7001", "--out", out}, mode...)...)
		expectCode(t, r, ExitOK)
		output := r.stdout + r.stderr
		for _, secret := range []string{"linus@", "example.test", "Linus"} {
			if strings.Contains(output, secret) {
				t.Errorf("%v: the output names %q:\n%s", mode, secret, output)
			}
		}
		if !strings.Contains(output, "1 record came with its e-mail address inside a pair of quotation marks") {
			t.Errorf("%v: no notice says how many came out of quotation marks:\n%s", mode, output)
		}

		if mode[0] == "--styled" {
			if !strings.Contains(r.stdout, "Records whose address a pair of quotation marks came off: 1") {
				t.Errorf("the text:\n%s", r.stdout)
			}
			continue
		}
		var env envelope
		if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
			t.Fatalf("%v: %s (%v)", mode, r.stdout, err)
		}
		data := decode(t, env.Data)
		if data["addressesUnquoted"] != float64(1) || !reflect.DeepEqual(data["invalidAddresses"], []any{}) {
			t.Errorf("addressesUnquoted %v, invalidAddresses %v", data["addressesUnquoted"], data["invalidAddresses"])
		}
		people := filesUnder(t, out)["people.jsonl"]
		if !strings.Contains(people, `"email":"linus@example.test"`) {
			t.Errorf("people.jsonl:\n%s", people)
		}
	}
}

func TestMostAddressesBeingNoneEndsTheRunWithNothingWritten(t *testing.T) {
	e := newImportEnv(t)
	for _, i := range []int{0, 3, 4, 6, 7, 8} {
		e.account.People[i].Email = "x***@*****.***"
	}
	parent := t.TempDir()
	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", filepath.Join(parent, "made", "bundle"))
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || !strings.Contains(env.Error, "6 of 10") || !strings.Contains(env.Hint, "identity") || strings.Contains(r.stdout, "***") {
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
	if data["peopleLeftOut"] != float64(11) || decode(t, env.Data)["counts"].(map[string]any)["people"] != float64(1) {
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
			"cards": float64(10), "steps": float64(3), "comments": float64(3), "reactions": float64(3), "attachments": float64(7)},
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

func TestADryRunCountsTheBoardsAndTheTalksTheWayARealRunDoes(t *testing.T) {
	for _, project := range []string{"7003", "7004"} {
		e := newImportEnv(t)
		real, r := e.runJSON("import", "basecamp", "--project", project, "--out", filepath.Join(t.TempDir(), "bundle"))
		expectCode(t, r, ExitOK)

		parent := t.TempDir()
		dry, r := e.runJSON("import", "basecamp", "--project", project, "--out", filepath.Join(parent, "bundle"), "--dry-run")
		expectCode(t, r, ExitOK)

		want, got := decode(t, real.Data), decode(t, dry.Data)
		delete(want, "bundle")
		want["dryRun"] = true
		// Only a download shows what became of a file.
		if want["filesFetched"] == float64(0) || want["filesBytes"] != want["filesBytesExpected"] {
			t.Errorf("project %s: the real run fetched %v files of %v bytes, expected %v", project, want["filesFetched"], want["filesBytes"], want["filesBytesExpected"])
		}
		want["filesBytes"], want["filesFetched"] = float64(0), float64(0)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("project %s: a dry run reports\n%v\na real one\n%v", project, got, want)
		}
		if real.Notice != dry.Notice {
			t.Errorf("project %s: a dry run warns %q, a real one %q", project, dry.Notice, real.Notice)
		}
		if entries, _ := os.ReadDir(parent); len(entries) != 0 {
			t.Errorf("project %s: a dry run wrote %v", project, entries)
		}
	}
}

func TestImportBasecampReportsThePostsAndTheChatsOfAProject(t *testing.T) {
	e := newImportEnv(t)
	e.account.Projects[3].Dock[5].Lines[0].Content = " "
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7004", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	parts := []any{"messages-0001.jsonl", "messages-0002.jsonl"}
	want := map[string]any{
		"counts": map[string]any{"people": float64(7), "projects": float64(1), "tools": float64(5), "categories": float64(4),
			"posts": float64(6), "messages": float64(8), "comments": float64(3), "reactions": float64(4), "attachments": float64(10)},
		"parts":         map[string]any{"messages": parts},
		"postsByStatus": map[string]any{"active": float64(4), "archived": float64(1), "trashed": float64(1)},
		"postsLeftOut":  float64(1), "postsWithEmptyBody": float64(1),
		"messagesByType":     map[string]any{"text": float64(3), "richText": float64(2), "upload": float64(2), "integration": float64(1)},
		"uploadLinesLeftOut": float64(1), "messagesWithEmptyBody": float64(1), "picturesLinked": float64(1),
		"recordsLeftOut": []any{map[string]any{"record": "line", "field": "type", "value": fakebasecamp.UnknownLine,
			"in": "chat", "inSourceId": "9033", "leftOut": float64(2)}},
		"longestBody": map[string]any{"cards": float64(0), "comments": float64(94), "posts": float64(212), "messages": float64(213), "answers": float64(0),
			"todolists": float64(0), "todos": float64(0), "forwards": float64(0), "forwardreplies": float64(0),
			"documents": float64(0), "uploads": float64(0)},
		"longestBodyRecords": map[string]any{
			"comments": map[string]any{"record": "comment", "sourceId": "9752", "in": "project", "inSourceId": "7004", "characters": float64(94)},
			"posts":    map[string]any{"record": "post", "sourceId": "9701", "in": "project", "inSourceId": "7004", "characters": float64(212)},
			"messages": map[string]any{"record": "line", "sourceId": "9802", "in": "chat", "inSourceId": "9033", "characters": float64(213)},
		},
		"filesInBodies": float64(4), "filesDropped": float64(2), "linkPreviews": float64(3), "mentionsUnresolved": float64(0),
		"embeds": float64(1), "embedsLeftOut": float64(0),
	}
	for key, value := range want {
		if !reflect.DeepEqual(data[key], value) {
			t.Errorf("%s: got %v, want %v", key, data[key], value)
		}
	}
	if !strings.Contains(env.Summary, "7 people, 1 project, 5 tools, 0 cards, 6 posts, 8 messages") {
		t.Errorf("summary %q", env.Summary)
	}
	for _, notice := range []string{"2 files embedded in texts or dropped into chats",
		"1 line of a chat is left out, because it was files alone",
		"2 records are left out, because the source describes them with words a bundle has none for"} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("notice %q does not say %q", env.Notice, notice)
		}
	}

	// The manifest names the parts the way the report does, and the bundle
	// holds them and not the one file beside them.
	files := filesUnder(t, out)
	var manifest map[string]any
	if err := json.Unmarshal([]byte(files["manifest.json"]), &manifest); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(manifest["parts"], want["parts"]) || !reflect.DeepEqual(manifest["counts"], want["counts"]) {
		t.Errorf("the manifest names the parts %v and counts %v", manifest["parts"], manifest["counts"])
	}
	for name, lines := range map[string]int{"categories.jsonl": 4, "posts.jsonl": 6, "messages-0001.jsonl": 7, "messages-0002.jsonl": 1, "messages.jsonl": 0, "messages-0003.jsonl": 0} {
		if got := strings.Count(files[name], "\n"); got != lines {
			t.Errorf("%s holds %d lines, want %d", name, got, lines)
		}
	}
	if _, has := files["messages.jsonl"]; has {
		t.Error("the bundle carries the one file beside its parts")
	}

	r = e.run("import", "basecamp", "--project", "7004", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"categories   4", "posts        6", "messages     8",
		"Posts: 4 active, 1 archived, 1 trashed; 1 of them without a text",
		"posts of a message board that is not extracted, left out: 1",
		"Messages: 3 text, 2 rich text, 2 upload, 1 integration",
		"Lines that say nothing, left out: 1",
		"Messages are written in 2 parts, messages-0001.jsonl to messages-0002.jsonl",
		"Lines that were files Basecamp names none of, left out: 1",
		"Records the source describes with a word a bundle has none for, left out: 2",
		"line     type   " + fakebasecamp.UnknownLine + "     in chat 9033: 2",
		"In the texts: 4 files referred to, 2 files Basecamp lists nothing for dropped, 0 mentions left as names, 3 previews of links turned into links, 1 picture at a foreign address turned into a link",
		"Longest text in characters: cards 0, comments 94, posts 212, messages 213",
		"  comment  9752     in project 7004: 94",
		"  post     9701     in project 7004: 212",
		"  line     9802     in chat 9033: 213",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
	if !strings.Contains(r.stderr, "Warning: 1 line of a chat is left out") {
		t.Errorf("stderr: %s", r.stderr)
	}
}

// The report counts the frames that became an embed and those that are left
// out, and no output says where a frame led: an address may carry a token.
func TestTheReportCountsEmbedsAndNoOutputSaysAnAddress(t *testing.T) {
	const taken, refused = "https://player.example.test/embed/s3cret-one?t=1", "//player.example.test/embed/s3cret-two"
	lines := func(a *fakebasecamp.Account) []fakebasecamp.Line { return a.Projects[3].Dock[3].Lines }
	framed := func(a *fakebasecamp.Account) {
		lines(a)[0].Type = fakebasecamp.RichTextLine
		lines(a)[0].Content = `<figure><iframe src="` + taken + `"></iframe><figcaption>A film</figcaption></figure>`
		lines(a)[4].Type = fakebasecamp.RichTextLine
		lines(a)[4].Content = `<figure><iframe src="` + refused + `"></iframe></figure>`
	}

	e := newImportEnv(t)
	framed(e.account)
	out := filepath.Join(t.TempDir(), "bundle")
	env, r := e.runJSON("import", "basecamp", "--project", "7004", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["embeds"] != float64(2) || data["embedsLeftOut"] != float64(1) || data["messagesWithEmptyBody"] != float64(1) {
		t.Errorf("embeds %v, embedsLeftOut %v, messagesWithEmptyBody %v", data["embeds"], data["embedsLeftOut"], data["messagesWithEmptyBody"])
	}
	if !strings.Contains(env.Notice, "1 embedded frame is left out of its text") {
		t.Errorf("notice %q", env.Notice)
	}
	var written string
	for name, content := range filesUnder(t, out) {
		if strings.HasPrefix(name, "messages") {
			written += content
		}
	}
	if !strings.Contains(written, `"body":"<import-embed url=\"`+taken+`\"></import-embed><p>A film</p>"`) {
		t.Errorf("the bundle lacks the embed:\n%s", written)
	}
	if strings.Contains(written, "s3cret-two") || strings.Contains(written, "iframe") {
		t.Errorf("the bundle holds a frame, or an address no embed may carry:\n%s", written)
	}
	// A second run writes the same bytes.
	e.now = e.now.Add(48 * time.Hour)
	expectCode(t, e.run("import", "basecamp", "--project", "7004", "--out", out, "--json"), ExitOK)
	for name, content := range filesUnder(t, out) {
		if strings.HasPrefix(name, "messages") && !strings.Contains(written, content) {
			t.Errorf("a second run wrote %s anew", name)
		}
	}

	r = e.run("import", "basecamp", "--project", "7004", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Embedded frames turned into embeds: 2", "Embedded frames with an address no embed may carry, left out: 1"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
	// A run without a frame has no line for them.
	r = e.run("import", "basecamp", "--project", "7001", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "Embedded frames") {
		t.Errorf("a run without a frame shows a line for them:\n%s", r.stdout)
	}

	// A line in the trash is left out, not a failure, and its text goes with it.
	changes := map[string]func(*fakebasecamp.Account){
		"nothing fails":       func(*fakebasecamp.Account) {},
		"a line at no time":   func(a *fakebasecamp.Account) { lines(a)[4].CreatedAt = "" },
		"a line in the trash": func(a *fakebasecamp.Account) { lines(a)[0].Status = "trashed" },
	}
	goesOn := map[string]bool{"nothing fails": true, "a line in the trash": true}
	for name, change := range changes {
		for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
			e := newImportEnv(t)
			framed(e.account)
			change(e.account)
			r := e.run(append([]string{"import", "basecamp", "--project", "7004", "--out", filepath.Join(t.TempDir(), "bundle")}, mode...)...)
			if (r.code == ExitOK) != goesOn[name] {
				t.Errorf("%s %v: exit %d", name, mode, r.code)
			}
			for _, text := range []string{"player.example", "s3cret", "embed/", "A film"} {
				if strings.Contains(r.stdout+r.stderr, text) {
					t.Errorf("%s %v: the output says %q:\n%s%s", name, mode, text, r.stdout, r.stderr)
				}
			}
		}
	}
}

func TestABundleWithOneSmallChatCarriesTheOneFileAndNamesNoParts(t *testing.T) {
	e := newImportEnv(t)
	e.account.Projects[3].Dock[5].Lines = nil
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7004", "--out", out)
	expectCode(t, r, ExitOK)
	if _, has := decode(t, env.Data)["parts"]; has {
		t.Errorf("the report names parts: %s", env.Data)
	}
	files := filesUnder(t, out)
	if strings.Count(files["messages.jsonl"], "\n") != 7 || strings.Contains(files["manifest.json"], "parts") {
		t.Errorf("messages.jsonl holds %d lines; the manifest:\n%s", strings.Count(files["messages.jsonl"], "\n"), files["manifest.json"])
	}
	if _, has := files["messages-0001.jsonl"]; has {
		t.Error("the bundle carries a part beside the one file")
	}

	r = e.run("import", "basecamp", "--project", "7004", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "parts") {
		t.Errorf("the text speaks of parts:\n%s", r.stdout)
	}
}

// What people wrote is in the bundle and nowhere else: no output says what a
// post is called, what somebody said in a chat or who did.
// writtenInAPostOrAChat is what people wrote in the posts and the chats of the
// fake: no output of a run over them may say any of it.
var writtenInAPostOrAChat = []string{
	"News", "Pitches", "Attic", "Campfire", "Side room", "Office move", "Reminder", "house rules", "tea club", "book shelf",
	"Announcement", "faq", "Heartbeat", "Pitch", "📢", "💡", "Count me in", "plants", "fern", "Good morning", "Build 42", "Goodbye", "Ping",
	"minutes.pdf", "whiteboard.png", "route.png", "nowhere.png", "0a1b2c3d", "not a key", "Shelves", "weather", "Click", "alert",
	"Grace", "Ada", "Former", "Robot", "Deleted Somebody", "example.test", "👏", "🎲", "Whatever",
}

func TestNoOutputSaysWhatStandsInAPostOrAChat(t *testing.T) {
	written := writtenInAPostOrAChat
	lines := func(a *fakebasecamp.Account) []fakebasecamp.Line { return a.Projects[3].Dock[3].Lines }
	posts := func(a *fakebasecamp.Account) []fakebasecamp.Post { return a.Projects[3].Dock[0].Posts }
	changes := map[string]func(*fakebasecamp.Account){
		"nothing fails":               func(*fakebasecamp.Account) {},
		"a kind of line nobody knows": func(a *fakebasecamp.Account) { lines(a)[1].Type = "Chat::Lines::Poll" },
		"a line in the trash":         func(a *fakebasecamp.Account) { lines(a)[1].Status = "trashed" },
		"a line at no time":           func(a *fakebasecamp.Account) { lines(a)[1].CreatedAt = "when Grace said it" },
		"a draft":                     func(a *fakebasecamp.Account) { posts(a)[0].ShownStatus = "draft" },
		"a post without a time":       func(a *fakebasecamp.Account) { posts(a)[0].UpdatedAt = "" },
		"a boost of nobody":           func(a *fakebasecamp.Account) { posts(a)[0].Boosts[0].Booster = nil },
		"lines that fail": func(a *fakebasecamp.Account) {
			a.Fail["buckets/7004/chats/9033/lines.json"] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
		},
	}
	// A kind of line nobody knows costs the line and not the run, so such a run
	// goes through and has to hold its tongue about the chat all the same.
	goesOn := map[string]bool{"nothing fails": true, "a kind of line nobody knows": true,
		"a line in the trash": true, "a draft": true}
	for name, change := range changes {
		for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
			e := newImportEnv(t)
			change(e.account)
			r := e.run(append([]string{"import", "basecamp", "--project", "7004", "--out", filepath.Join(t.TempDir(), "bundle")}, mode...)...)
			if (r.code == ExitOK) != goesOn[name] {
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

// The record that holds the longest text of its kind is named in every mode,
// by its identifier and the place it stands in, and by nothing that was written
// in it.
func TestTheLongestTextNamesItsRecordAndNothingOfWhatItSays(t *testing.T) {
	for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
		e := newImportEnv(t)
		r := e.run(append([]string{"import", "basecamp", "--project", "7004", "--out", filepath.Join(t.TempDir(), "bundle")}, mode...)...)
		expectCode(t, r, ExitOK)
		for _, id := range []string{"9752", "9701", "9802"} {
			if !strings.Contains(r.stdout, id) {
				t.Errorf("%v: the output does not name the record %s of a longest text:\n%s", mode, id, r.stdout)
			}
		}
		for _, text := range writtenInAPostOrAChat {
			if strings.Contains(r.stdout+r.stderr, text) {
				t.Errorf("%v: the output says %q:\n%s%s", mode, text, r.stdout, r.stderr)
			}
		}
	}
}

func TestALineOrAPostABundleCannotCarryIsAnErrorOfTheAPIThatNamesItByID(t *testing.T) {
	for name, c := range map[string]struct {
		change  func(*fakebasecamp.Account)
		message string
	}{
		"a line at no time":     {func(a *fakebasecamp.Account) { a.Projects[3].Dock[3].Lines[1].CreatedAt = "at dawn" }, "line 9802 was created at something that is no time"},
		"a post without a time": {func(a *fakebasecamp.Account) { a.Projects[3].Dock[0].Posts[0].UpdatedAt = "" }, "when post 9701 was updated"},
	} {
		e := newImportEnv(t)
		c.change(e.account)
		parent := t.TempDir()
		env, r := e.runJSON("import", "basecamp", "--project", "7004", "--out", filepath.Join(parent, "bundle"))
		expectCode(t, r, ExitAPI)
		if env.Code != CodeAPI || !strings.Contains(env.Error, c.message) {
			t.Errorf("%s: code %q, error %q", name, env.Code, env.Error)
		}
		if entries, _ := os.ReadDir(parent); len(entries) != 0 {
			t.Errorf("%s: the run left %v", name, entries)
		}
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

func TestImportBasecampBringsTheFilesOfTextsAndChats(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := map[string]any{
		"filesInBodies": float64(16), "filesDropped": float64(0), "filesExpected": float64(13), "filesBytesExpected": float64(15950),
		"filesBytes": float64(15114), "filesFetched": float64(12), "filesReused": float64(0), "filesGone": float64(1),
		"fileNamesChanged": float64(5), "captionsDropped": float64(1), "filesOfSeveralOwners": float64(2), "missingFiles": []any{},
	}
	for field, value := range want {
		if !reflect.DeepEqual(data[field], value) {
			t.Errorf("%s: got %v, want %v", field, data[field], value)
		}
	}
	// The 13 files, 2 avatars, and a record for each of the two further owners
	// of one file.
	if counts := data["counts"].(map[string]any); counts["attachments"] != float64(17) {
		t.Errorf("counts %v", counts)
	}
	for _, notice := range []string{"1 file of a text or a chat is gone", "1 caption of a file is left off its reference"} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("the notice lacks %q: %s", notice, env.Notice)
		}
	}
	files := filesUnder(t, out)
	if len(files["files/attachment-5101/holiday photo.png"]) != 3000 || !strings.Contains(files["attachments.jsonl"], `"sourceId":"attachment-5108"`) {
		t.Error("the bundle lacks a file or the record of the one that is gone")
	}

	// A second run takes over what the first brought.
	downloaded := len(e.account.Downloads)
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitOK)
	data = decode(t, env.Data)
	if data["filesReused"] != float64(12) || data["filesFetched"] != float64(0) || data["filesBytes"] != float64(15114) || len(e.account.Downloads) != downloaded+1 {
		t.Errorf("reused %v, fetched %v, %v bytes, %d downloads", data["filesReused"], data["filesFetched"], data["filesBytes"], len(e.account.Downloads)-downloaded)
	}
	delete(files, "manifest.json")
	again := filesUnder(t, out)
	delete(again, "manifest.json")
	if !reflect.DeepEqual(again, files) {
		t.Error("the second run wrote another bundle than the first")
	}

	r = e.run("import", "basecamp", "--project", "7005", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, line := range []string{
		"Files: 12 of 13 brought, 15114 bytes of 15950 expected; 0 fetched, 12 taken over from the bundle before, 1 gone at Basecamp",
		"Names of files changed: 5; captions too long for a reference, left off: 1; records of a file under a further owner: 2",
	} {
		if !strings.Contains(r.stdout, line) {
			t.Errorf("the text lacks %q:\n%s", line, r.stdout)
		}
	}
}

func TestADryRunDownloadsNoFileAndSaysWhatARunWouldFetch(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", filepath.Join(parent, "bundle"), "--dry-run")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	for field, value := range map[string]float64{
		"filesExpected": 13, "filesBytesExpected": 15950, "filesBytes": 0, "filesFetched": 0, "filesReused": 0, "filesGone": 0,
		"fileNamesChanged": 5, "captionsDropped": 1,
	} {
		if data[field] != value {
			t.Errorf("%s: got %v, want %v", field, data[field], value)
		}
	}
	if len(e.account.Downloads) != 0 || len(e.account.AvatarRequests) != 0 {
		t.Errorf("a dry run downloaded %v and %v", e.account.Downloads, e.account.AvatarRequests)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("a dry run wrote %v", entries)
	}

	r = e.run("import", "basecamp", "--project", "7005", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	if want := "Files a run would fetch: 13, 15950 bytes as Basecamp gives their sizes"; !strings.Contains(r.stdout, want) {
		t.Errorf("the text lacks %q:\n%s", want, r.stdout)
	}
}

func TestAFileOfATextThatCannotBeFetchedFailsTheRunAndTheNextOneBringsIt(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	waiting := fakebasecamp.FileKey(4711)
	e.account.FailFileOf[waiting] = &basecamp.Error{Kind: basecamp.Network, Message: "the network failed during the download"}

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitNetwork)
	if env.Code != CodeNetwork || !env.Retryable || !strings.Contains(env.Error, "1 file could not be fetched and is left out") {
		t.Errorf("code %q, retryable %v, error %q", env.Code, env.Retryable, env.Error)
	}
	data := decode(t, env.Data)
	wantMissing := []any{map[string]any{
		"attachmentSourceId": "attachment-4711", "ownerKind": "card", "ownerSourceId": "9902",
		"reason": "the network failed during the download", "retryable": true,
	}}
	if !reflect.DeepEqual(data["missingFiles"], wantMissing) || data["filesFetched"] != float64(11) {
		t.Errorf("missingFiles %v, fetched %v", data["missingFiles"], data["filesFetched"])
	}
	files := filesUnder(t, out)
	if strings.Contains(files["attachments.jsonl"], "attachment-4711") || !strings.Contains(files["cards.jsonl"], `ref=\"attachment-4711\"`) {
		t.Error("the file that is missing has a record, or the text lost its reference")
	}
	for path := range files {
		if strings.HasPrefix(path, "files/attachment-4711/") {
			t.Errorf("half a file stayed at %s", path)
		}
	}

	// A failure that does not pass is no reason to try again.
	e.account.FailFileOf[waiting] = &basecamp.Error{Kind: basecamp.Forbidden, Message: "Basecamp forbids the download"}
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitAPI)
	if env.Retryable {
		t.Error("a file Basecamp forbids makes the run worth repeating")
	}

	delete(e.account.FailFileOf, waiting)
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitOK)
	data = decode(t, env.Data)
	if data["filesFetched"] != float64(1) || data["filesReused"] != float64(11) || len(filesUnder(t, out)["files/attachment-4711/waiting.png"]) != 600 {
		t.Errorf("the next run fetched %v and reused %v", data["filesFetched"], data["filesReused"])
	}
}

func TestADownloadThatIsStoppedEndsTheRunWithNothingWritten(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	e.account.FailFileOf[fakebasecamp.FileKey(5102)] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", filepath.Join(parent, "bundle"))
	expectCode(t, r, ExitNetwork)
	if env.OK || env.Data != nil || !strings.Contains(env.Error, "interrupted") {
		t.Errorf("ok %v, error %q, data %s", env.OK, env.Error, env.Data)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("the run left %v", entries)
	}
}

func TestADownloadWithoutASignInEndsTheRunTheWayARequestDoes(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	e.account.FailFileOf[fakebasecamp.FileKey(5102)] = &basecamp.Error{Kind: basecamp.AuthRequired, Message: "the Basecamp CLI is not signed in"}

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", filepath.Join(parent, "bundle"))
	expectCode(t, r, ExitAuth)
	if env.OK || env.Code != CodeAuth || env.Data != nil || !strings.Contains(env.Hint, "basecamp auth login") {
		t.Errorf("ok %v, code %q, hint %q, data %s", env.OK, env.Code, env.Hint, env.Data)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("the run left %v", entries)
	}
}

func TestNoOutputSaysWhatAFileIsCalledOrWhereItLies(t *testing.T) {
	written := []string{
		"holiday", "photo.png", "beach", "reports", "q1.pdf", "break", "shredded", "waiting", "wordy", "unsized", "minutes", "scan",
		"long name", "jpeg", "Sunset", "sea", "The beach", "goes on", "storage.", "basecampapi", "blobs", "download/", "%20",
	}
	failures := map[string]func(*fakebasecamp.Account){
		"nothing fails": func(*fakebasecamp.Account) {},
		"the network": func(a *fakebasecamp.Account) {
			a.FailFileOf[fakebasecamp.FileKey(5101)] = &basecamp.Error{Kind: basecamp.Network, Message: "the network failed during the download"}
		},
		"another size":       func(a *fakebasecamp.Account) { a.ServeBytes[fakebasecamp.FileKey(6001)] = 17 },
		"a key that is none": func(a *fakebasecamp.Account) { a.Projects[4].Dock[0].Lists[0].Cards[0].Files[1].Key = "k-one" },
		"a bundle that is full": func(a *fakebasecamp.Account) {
			a.FailFileOf[fakebasecamp.FileKey(5103)] = errors.New("no space left on device")
		},
	}
	// A dry run ends well for what only a download can show, and badly for what
	// the source itself already says: a file whose address cannot be asked for is
	// one no run will ever bring, and a dry run is what somebody reads before
	// spending an hour on the files.
	seenWithoutADownload := map[string]bool{"a key that is none": true}
	for name, change := range failures {
		for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
			e := newImportEnv(t)
			change(e.account)
			r := e.run(append([]string{"import", "basecamp", "--project", "7005", "--out", filepath.Join(t.TempDir(), "bundle")}, mode...)...)
			wantOK := name == "nothing fails" || (slices.Contains(mode, "--dry-run") && !seenWithoutADownload[name])
			if (r.code == ExitOK) != wantOK {
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

// Docs & Files is made of file names, so nothing a run says of it may carry
// one, nor a title of a document, nor a folder's name: the report names the
// numeric identifier and what is wrong with it.
func TestNoOutputOfDocsAndFilesSaysWhatAnythingIsCalled(t *testing.T) {
	written := []string{
		// The names of files, and of the one in a cloud that has none.
		"logo.png", "sketch.png", "minutes.pdf", "old.pdf", "draft.pdf", "last-year.pdf", "checked.pdf", "held.pdf",
		"lost.pdf", "table.png", "chart.png", "shredded.png", "long name", "jpeg", "Plan in the cloud",
		// The names of folders and the titles of documents that come.
		"Handbook", "Drafts", "Press", "Finished", "Thrown out", "Chapter one", "Appendix", "Not for outside",
		"Read me first", "The picture", "Last quarter", "Wrong place", "Last year", "For everybody",
		// And of the records that are left out, which a careless message about
		// what fell would be the first to quote.
		"Held back", "Below the one held back", "Below as well", "Under review", "Behind a switch",
		"Behind a switch as well", "Nobody reads this", "In use behind a switch", "Under a folder nothing sees",
		// Nothing of an address of the source either.
		"storage.", "basecampapi", "blobs", "download/",
	}
	for _, mode := range [][]string{{"--json"}, {"--quiet"}, {"--styled"}, {"--json", "--dry-run"}} {
		e := newImportEnv(t)
		r := e.run(append([]string{"import", "basecamp", "--project", "7011", "--out", filepath.Join(t.TempDir(), "bundle")}, mode...)...)
		expectCode(t, r, ExitOK)
		for _, text := range written {
			if strings.Contains(r.stdout+r.stderr, text) {
				t.Errorf("%v: the output says %q:\n%s%s", mode, text, r.stdout, r.stderr)
			}
		}
	}
}

func TestImportBasecampReportsTheLinksThatBecameReferences(t *testing.T) {
	e := newImportEnv(t)

	env, r := e.runJSON("import", "basecamp", "--project", "7006", "--dry-run")
	expectCode(t, r, ExitOK)
	want := map[string]any{"cards": float64(4), "posts": float64(2), "tools": float64(3)}
	if got := decode(t, env.Data)["references"]; !reflect.DeepEqual(got, want) {
		t.Errorf("references: got %v, want %v", got, want)
	}

	r = e.run("import", "basecamp", "--project", "7006", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	if want := "Links turned into references: 4 to cards, 2 to posts, 3 to chats"; !strings.Contains(r.stdout, want) {
		t.Errorf("the text lacks %q:\n%s", want, r.stdout)
	}
	// A run without such a link has no line for them.
	r = e.run("import", "basecamp", "--project", "7001", "--dry-run", "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "references") {
		t.Errorf("a run without a reference shows a line for them:\n%s", r.stdout)
	}
}

func TestAToolTheIdentityMayNotSeeIsSaidToBePassedOver(t *testing.T) {
	for _, mode := range [][]string{{"--json"}, {"--styled"}} {
		e := newImportEnv(t)
		for _, path := range []string{"buckets/7001/todosets/9005.json", "buckets/7001/schedules/9006.json"} {
			e.account.Fail[path] = &basecamp.Error{Kind: basecamp.Forbidden, Message: "Basecamp forbids it"}
		}
		out := filepath.Join(t.TempDir(), "bundle")

		r := e.run(append([]string{"import", "basecamp", "--project", "7001", "--out", out}, mode...)...)
		expectCode(t, r, ExitOK)
		output := r.stdout + r.stderr
		if !strings.Contains(output, "2 tools of projects could not be asked who created their project") {
			t.Errorf("%v: no notice says that tools were refused:\n%s", mode, output)
		}
		if mode[0] == "--styled" && !strings.Contains(r.stdout, "asked for the creator of a project and passed over: 2") {
			t.Errorf("the text:\n%s", r.stdout)
		}
	}

	e := newImportEnv(t)
	env, r := e.runJSON("import", "basecamp", "--project", "7001", "--out", filepath.Join(t.TempDir(), "bundle"))
	expectCode(t, r, ExitOK)
	if strings.Contains(env.Notice, "could not be asked who created") {
		t.Errorf("a run that met no refusal says one: %q", env.Notice)
	}
}

func TestImportBasecampReportsTheCheckinsOfAProject(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7007", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := map[string]any{
		"counts": map[string]any{"people": float64(6), "projects": float64(1), "tools": float64(1),
			"questions": float64(6), "answers": float64(6), "comments": float64(1), "reactions": float64(2), "attachments": float64(6)},
		"questionsLeftOut": []any{map[string]any{"questionSourceId": "9314", "field": "group_on"}},
		"anchorsInferred":  []any{map[string]any{"questionSourceId": "9312", "from": "2024-02-28"}},
		"nobodyAsked":      []any{},
		"askedLeftOut": map[string]any{
			"notInTheProject": float64(1), "clients": float64(0), "retired": float64(1), "withoutAnAddress": float64(1)},
		"answersWithoutABody": float64(1), "answersWithoutADay": float64(0), "answersTrashed": float64(0),
		"longestBody": map[string]any{"cards": float64(0), "comments": float64(17), "posts": float64(0), "messages": float64(0), "answers": float64(150),
			"todolists": float64(0), "todos": float64(0), "forwards": float64(0), "forwardreplies": float64(0),
			"documents": float64(0), "uploads": float64(0)},
		"longestBodyRecords": map[string]any{
			"comments": map[string]any{"record": "comment", "sourceId": "9350", "in": "project", "inSourceId": "7007", "characters": float64(17)},
			"answers":  map[string]any{"record": "answer", "sourceId": "9320", "in": "project", "inSourceId": "7007", "characters": float64(150)},
		},
	}
	for key, value := range want {
		if !reflect.DeepEqual(data[key], value) {
			t.Errorf("%s: got %v, want %v", key, data[key], value)
		}
	}
	for _, notice := range []string{
		"1 question of a check-in is left out",
		"1 question asked every other week is anchored on the day it was last answered",
		"3 people questions ask cannot be said to be asked",
		"1 answer of a check-in is left out, because it says nothing",
	} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("notice %q does not say %q", env.Notice, notice)
		}
	}

	r = e.run("import", "basecamp", "--project", "7007", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"questions    6", "answers      6",
		"Questions whose schedule Minka has no words for, left out: 1 — 9314 (group_on)",
		"Questions asked every other week anchored on the day they were last answered: 1 — 9312 from 2024-02-28",
		"People a question asks that the bundle cannot say are asked: 1 no member of the project, 0 a client, 1 no longer there, 1 without an address",
		"Answers that say nothing, left out: 1; answers in the trash, left out: 0",
		"Answers Basecamp names no day for, dated by when they were written: 0",
		"answer   9320     in project 7007: 150",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
}

// A run that met no questionnaire says nothing about check-ins, and no output
// ever says what somebody answered.
func TestARunWithoutACheckinSaysNothingAboutThem(t *testing.T) {
	e := newImportEnv(t)
	r := e.run("import", "basecamp", "--project", "7001", "--out", filepath.Join(t.TempDir(), "bundle"), "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "Questions") || strings.Contains(r.stdout, "Answers") {
		t.Errorf("a run without a questionnaire talks about check-ins:\n%s", r.stdout)
	}
}

func TestNoOutputSaysWhatSomebodyAnsweredACheckin(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	answered := []string{"Reviewed the plan", "Packed the boxes", "Joined from the outside", "On course", "The deploys", "hours.png", "What did you work on"}

	env, r := e.runJSON("import", "basecamp", "--project", "7007", "--out", out)
	expectCode(t, r, ExitOK)
	styled := e.run("import", "basecamp", "--project", "7007", "--out", out, "--styled")
	expectCode(t, styled, ExitOK)

	for _, text := range answered {
		for what, output := range map[string]string{
			"the envelope": r.stdout + r.stderr, "the text": styled.stdout + styled.stderr, "the notices": env.Notice,
		} {
			if strings.Contains(output, text) {
				t.Errorf("%s says %q", what, text)
			}
		}
	}
}

// A question whose every subscriber is somebody the bundle cannot name as asked
// comes asking nobody, and asks nobody again: a later run never touches asked.
// So it is named on its own and not only counted.
func TestAQuestionThatComesAskingNobodyIsNamed(t *testing.T) {
	e := newImportEnv(t)
	// Of the weekend question's one subscriber the bundle knows no address.
	questions := e.account.Projects[6].Dock[0].Questions
	for i := range questions {
		if questions[i].ID == 9311 {
			questions[i].Subscribers = []fakebasecamp.Person{{ID: 103}}
		}
	}
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7007", "--out", out)
	expectCode(t, r, ExitOK)
	want := []any{map[string]any{"questionSourceId": "9311", "leftOut": float64(1)}}
	if got := decode(t, env.Data)["nobodyAsked"]; !reflect.DeepEqual(got, want) {
		t.Errorf("nobodyAsked: got %v, want %v", got, want)
	}
	if !strings.Contains(env.Notice, "1 question of a check-in comes asking nobody at all") {
		t.Errorf("notice %q", env.Notice)
	}

	styled := e.run("import", "basecamp", "--project", "7007", "--out", out, "--styled")
	expectCode(t, styled, ExitOK)
	if !strings.Contains(styled.stdout, "Questions that come asking nobody at all: 1 — 9311 (1 left out)") {
		t.Errorf("the text lacks the question:\n%s", styled.stdout)
	}
}

func TestImportBasecampReportsTheToDosOfAProject(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7008", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := map[string]any{
		"todoListsByStatus": map[string]any{"active": float64(3), "archived": float64(1), "trashed": float64(1)},
		"todosByStatus":     map[string]any{"active": float64(7), "trashed": float64(1)},
		"todosLeftOut":      map[string]any{"lists": float64(1), "groups": float64(2), "todos": float64(4)},
		"todoPlacesLeftOut": []any{
			map[string]any{"record": "to-do list", "sourceId": "9406", "groups": float64(1), "todos": float64(1)},
			map[string]any{"record": "group", "sourceId": "9414", "groups": float64(0), "todos": float64(1)},
		},
		"todosWithoutAList": []any{
			map[string]any{"sourceId": "9431", "projectSourceId": "7008"},
			map[string]any{"sourceId": "9432", "projectSourceId": "7008"},
		},
		"todosNotDone": float64(6), "todosWithPeople": float64(2),
		"todosWithSubtasks": float64(1), "subtasksLeftOut": float64(2), "todosThatRepeat": float64(2),
		"recordsLeftOut": []any{map[string]any{"record": "group", "field": "status", "value": "archived",
			"in": "project", "inSourceId": "7008", "leftOut": float64(1)}},
	}
	for key, value := range want {
		if !reflect.DeepEqual(data[key], value) {
			t.Errorf("%s: got %v, want %v", key, data[key], value)
		}
	}
	for _, notice := range []string{
		"2 subtasks of to-dos are left out",
		"2 to-dos name somebody as an assignee or as somebody to tell when they are done",
		"6 to-dos are open at the source, and a later run reopens nothing",
		"2 lists or groups of To-dos tools are left out with what stood in them",
		"2 to-dos hang on To-dos tools themselves and stand in no list of them",
		"2 to-dos come again at the source and are carried as ordinary ones",
		"1 record is left out, because the source describes it with a word a bundle has none for",
	} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("notice %q does not say %q", env.Notice, notice)
		}
	}

	// A run that met no To-dos tool says nothing of them at all, as one that met
	// no questionnaire says nothing of check-ins.
	r = e.run("import", "basecamp", "--project", "7003", "--out", filepath.Join(t.TempDir(), "boards"), "--styled")
	expectCode(t, r, ExitOK)
	for _, quiet := range []string{"To-do lists:", "To-dos:", "To-dos that had subtasks", "Left out of the To-dos tools",
		"To-dos that come again", "in no list of it"} {
		if strings.Contains(r.stdout, quiet) {
			t.Errorf("a run over a project without a To-dos tool says %q:\n%s", quiet, r.stdout)
		}
	}

	r = e.run("import", "basecamp", "--project", "7008", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"todolists    5", "todogroups   4", "todos        8",
		"To-do lists: 3 active, 1 archived, 1 trashed",
		"To-dos: 7 active, 1 trashed; 6 of them still open, 2 carrying people",
		"To-dos that had subtasks: 1, with 2 subtasks a bundle has no word for",
		"To-dos that come again at the source, carried as ordinary ones: 2",
		"Left out of the To-dos tools: 1 lists, 2 groups, 4 to-dos",
		"  with to-do list 9406 fell: 1 groups, 1 to-dos",
		"  with group 9414 fell: 0 groups, 1 to-dos",
		"To-dos hanging on a To-dos tool itself and in no list of it, left out: 2 — 9431 of project 7008, 9432 of project 7008",
		"to-do list 9401     in project 7008: 159",
		"to-do    9421     in project 7008: 151",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the text lacks %q:\n%s", want, r.stdout)
		}
	}
}

func TestImportBasecampReportsTheForwardedMailOfAProject(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7010", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := map[string]any{
		"byStatus": map[string]any{"active": float64(8), "trashed": float64(2)}, "fromTheShelf": float64(1),
		"withoutASubject": float64(1), "subjectsChanged": float64(1), "givenALineOfTheRun": float64(4),
		"withoutAForwarder": float64(1), "withoutAnAddress": []any{"9307"},
		"repliesFromOutside": float64(2), "repliesWithoutASender": []any{"9404"},
		"reactionsOnRepliesLeftOut": float64(2), "longerThanMinkaTakes": float64(0),
	}
	if !reflect.DeepEqual(data["forwards"], want) {
		t.Errorf("forwards:\n got %v\nwant %v", data["forwards"], want)
	}
	for _, notice := range []string{
		"1 forwarded mail is left out, because what the source calls its sender is no e-mail address Minka takes",
		"1 answer by mail is left out, because the source names neither who wrote it nor an address",
		"1 forwarded mail is on the shelf at the source and comes into the trash here",
		"4 forwarded mails or answers by mail carry a line of the run's own",
		"2 forwarded mails carry titles the run made of their subjects",
		"1 forwarded mail names nobody who forwarded it",
		"2 boosts on answers by mail are left out",
	} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("notice %q does not say %q", env.Notice, notice)
		}
	}
	// Never a subject, never an address, never a name of a file: a report ends up
	// in a log.
	for _, secret := range []string{"Invoice", "supplier.test", "partner.test", "invoice.pdf", "localhost", "anna@"} {
		if strings.Contains(r.stdout+r.stderr, secret) {
			t.Errorf("the output says %q:\n%s", secret, r.stdout)
		}
	}

	r = e.run("import", "basecamp", "--project", "7010", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, line := range []string{
		"forwards     10", "forwardreplies 4",
		"Forwarded mail: 8 active, 2 trashed; 1 of them on the shelf at the source",
		"Mails without a subject: 1; subjects folded or cut short to make a title: 1",
		"Mails and answers carrying a line of the run's own, because what they say would arrive as nothing: 4",
		"Mails nobody is named for, which Minka gives to the owner of the organization: 1",
		"Mails whose sender is no address Minka takes, left out: 1 (forward 9307)",
		"Answers by mail: 4, 2 of them from outside the account",
		"Answers naming neither a writer nor an address, left out: 1 (reply 9404)",
		"Boosts on an answer by mail, which Minka takes none of, left out: 2",
		"forward  9301     in project 7010:",
	} {
		if !strings.Contains(r.stdout, line) {
			t.Errorf("the text lacks %q:\n%s", line, r.stdout)
		}
	}

	// A run that met no inbox says nothing of forwarded mail at all.
	r = e.run("import", "basecamp", "--project", "7003", "--out", filepath.Join(t.TempDir(), "boards"), "--styled")
	expectCode(t, r, ExitOK)
	for _, quiet := range []string{"Forwarded mail:", "Answers by mail:", "Mails without a subject"} {
		if strings.Contains(r.stdout, quiet) {
			t.Errorf("a run over a project without an inbox says %q:\n%s", quiet, r.stdout)
		}
	}
}

func TestImportBasecampReportsTheDocsAndFilesOfAProject(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7011", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	want := map[string]any{
		"foldersByStatus":       map[string]any{"active": float64(3), "archived": float64(1), "trashed": float64(1)},
		"documentsByStatus":     map[string]any{"active": float64(6), "archived": float64(2), "trashed": float64(1)},
		"uploadsByStatus":       map[string]any{"active": float64(7), "archived": float64(1), "trashed": float64(1)},
		"documentsWithoutWords": float64(1), "titlesFromTheFileName": float64(2),
		"namesMadeIntoATitle": float64(2), "withoutAName": float64(1),
		"withoutAStoredFile": []any{"9262"}, "commentsWithoutTheirUpload": float64(1), "boostsWithoutTheirUpload": float64(1),
		"onlyAFileNoRunWillBring": []any{"9245", "9276"}, "foldersThatSaidNothingOfWhatIsInThem": float64(2),
		"silentAboutClientsInsideAFolder": []any{
			map[string]any{"record": "document", "sourceId": "9255", "projectSourceId": "7011"},
			map[string]any{"record": "document", "sourceId": "9252", "projectSourceId": "7011"},
			map[string]any{"record": "upload", "sourceId": "9270", "projectSourceId": "7011"},
		},
		"longerThanMinkaTakes": float64(0),
		"leftOut":              map[string]any{"folders": float64(3), "documents": float64(5), "uploads": float64(2)},
		"placesLeftOut": []any{
			map[string]any{"record": "folder", "sourceId": "9224", "folders": float64(1), "documents": float64(1), "uploads": float64(1)},
			map[string]any{"record": "Docs & Files tool", "sourceId": "9212", "folders": float64(1), "documents": float64(1), "uploads": float64(0)},
			map[string]any{"record": "folder", "sourceId": "9227", "folders": float64(0), "documents": float64(1), "uploads": float64(0)},
		},
	}
	if !reflect.DeepEqual(data["docs"], want) {
		t.Errorf("docs:\n got %v\nwant %v", data["docs"], want)
	}
	for _, notice := range []string{
		"1 upload of a Docs & Files tool is left out, because Basecamp names no stored file for it",
		"2 records of Docs & Files tools lost a file no run will bring",
		"2 uploads are called nothing at the source, so the names of their files are their titles here",
		"2 uploads carry titles the run had to fold to fit",
		"3 folders or Docs & Files tools are left out with what stood in them",
	} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("notice %q does not say %q", env.Notice, notice)
		}
	}

	r = e.run("import", "basecamp", "--project", "7011", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, line := range []string{
		"folders      5", "documents    9", "uploads      9",
		"Folders: 3 active, 1 archived, 1 trashed",
		"Documents: 6 active, 2 archived, 1 trashed; 1 of them without words",
		"Uploads: 7 active, 1 archived, 1 trashed; 2 titled by the name of their file, 1 of them with neither",
		"Uploads Basecamp names no stored file for, left out: 1 (upload 9262), with 1 comments and 1 boosts",
		"Records that lost a file no run will bring, the documents among them wordless: 2 (record 9245, 9276)",
		"Left out of the Docs & Files tools: 3 folders, 5 documents, 2 uploads",
		"  with folder 9224 fell: 1 folders, 1 documents, 1 uploads",
		"  with Docs & Files tool 9212 fell: 1 folders, 1 documents, 0 uploads",
		"Entries inside a folder that say nothing about whether clients see them, which cannot hide the folder above them: 3",
		"  document 9255     in project 7011",
		"  upload   9270     in project 7011",
	} {
		if !strings.Contains(r.stdout, line) {
			t.Errorf("the text lacks %q:\n%s", line, r.stdout)
		}
	}

	// A run that met no Docs & Files tool says nothing about them at all.
	r = e.run("import", "basecamp", "--project", "7003", "--out", filepath.Join(t.TempDir(), "boards"), "--styled")
	expectCode(t, r, ExitOK)
	for _, quiet := range []string{"Folders:", "Documents:", "Uploads:", "Left out of the Docs & Files tools"} {
		if strings.Contains(r.stdout, quiet) {
			t.Errorf("a run over a project without a Docs & Files tool says %q:\n%s", quiet, r.stdout)
		}
	}

	// The address the source gives an upload is asked for by nothing, so no
	// address of one makes a run say anything: whatever shape it has, the file is
	// fetched by the identifier of the upload, and a dry run ends well.
	e = newImportEnv(t)
	theUploadOfTheDocsProject(t, e.account, 9267).DownloadURL = "not an address at all"
	env, r = e.runJSON("import", "basecamp", "--project", "7011", "--dry-run")
	expectCode(t, r, ExitOK)
	if got := decode(t, env.Data)["missingFiles"]; len(got.([]any)) != 0 {
		t.Errorf("a dry run says %v of an upload whose address nothing asks for, want nothing", got)
	}
	if docs := decode(t, env.Data)["docs"].(map[string]any); len(docs["withoutAStoredFile"].([]any)) != 1 {
		t.Errorf("a dry run says %v name no stored file, want the one in the cloud alone", docs["withoutAStoredFile"])
	}

	// One that met a tool of which the bundle carries nothing at all still says
	// what became of it: a report that fell silent there would be the one place
	// a whole tool goes missing without a word.
	e = newImportEnv(t)
	onlyInTheCloud(t, e.account)
	r = e.run("import", "basecamp", "--project", "7011", "--out", filepath.Join(t.TempDir(), "cloud"), "--styled")
	expectCode(t, r, ExitOK)
	if want := "Uploads Basecamp names no stored file for, left out:"; !strings.Contains(r.stdout, want) {
		t.Errorf("a run whose Docs & Files come to nothing says nothing of them:\n%s", r.stdout)
	}
}

// theUploadOfTheDocsProject finds an upload of the fake's docs project by its ID.
func theUploadOfTheDocsProject(t *testing.T, a *fakebasecamp.Account, id int64) *fakebasecamp.UploadedFile {
	t.Helper()
	var found *fakebasecamp.UploadedFile
	var down func([]fakebasecamp.Folder)
	look := func(uploads []fakebasecamp.UploadedFile) {
		for i := range uploads {
			if uploads[i].ID == id {
				found = &uploads[i]
			}
		}
	}
	down = func(folders []fakebasecamp.Folder) {
		for i := range folders {
			look(folders[i].Uploads)
			down(folders[i].Folders)
		}
	}
	for d := range a.Projects[10].Dock {
		look(a.Projects[10].Dock[d].Uploads)
		down(a.Projects[10].Dock[d].Folders)
	}
	if found == nil {
		t.Fatalf("the docs project holds no upload %d", id)
	}
	return found
}

// onlyInTheCloud leaves the docs project with nothing a bundle can carry: every
// folder and document gone from the account, and every upload in a cloud.
func onlyInTheCloud(t *testing.T, a *fakebasecamp.Account) {
	t.Helper()
	var strip func([]fakebasecamp.Folder)
	strip = func(folders []fakebasecamp.Folder) {
		for i := range folders {
			strip(folders[i].Folders)
			for u := range folders[i].Uploads {
				folders[i].Uploads[u].InTheCloud, folders[i].Uploads[u].ShownStatus = true, ""
			}
			folders[i].Documents = nil
		}
	}
	for d := range a.Projects[10].Dock {
		tool := &a.Projects[10].Dock[d]
		strip(tool.Folders)
		tool.Folders, tool.Documents = nil, nil
		for u := range tool.Uploads {
			tool.Uploads[u].InTheCloud, tool.Uploads[u].ShownStatus = true, ""
		}
	}
}

func TestImportBasecampSaysWhatABundleHoldsAboutClients(t *testing.T) {
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7009", "--out", out)
	expectCode(t, r, ExitOK)

	data := decode(t, env.Data)
	want := map[string]any{"peopleAsClients": float64(5), "clientMemberships": float64(5),
		"visibilityLeftToTheContainer": float64(4)}
	for field, number := range want {
		if data[field] != number {
			t.Errorf("%s is %v, want %v", field, data[field], number)
		}
	}
	for _, notice := range []string{
		"5 memberships of projects belong to clients",
		"4 people questions ask cannot be said to be asked",
	} {
		if !strings.Contains(env.Notice, notice) {
			t.Errorf("no notice says %q:\n%s", notice, env.Notice)
		}
	}
	// Never a name and never a company: a report is read by whoever ran the
	// import and ends up in a log.
	for _, secret := range []string{"Northwind", "Iris", "Otto", "Nora", "Cora", "Tessa", "Tombstone"} {
		if strings.Contains(r.stdout+r.stderr, secret) {
			t.Errorf("the output names %q:\n%s", secret, r.stdout)
		}
	}

	r = e.run("import", "basecamp", "--project", "7009", "--out", out, "--styled")
	expectCode(t, r, ExitOK)
	for _, line := range []string{
		"Clients: 5, with 5 memberships Minka leaves out of their projects",
		"4 a client,",
		"Words about clients on a record that follows what it hangs on, left out: 4",
	} {
		if !strings.Contains(r.stdout, line) {
			t.Errorf("the text lacks %q:\n%s", line, r.stdout)
		}
	}
	// Every kind of tool is extracted, so no report names a gap any more.
	if strings.Contains(r.stdout, "a run does not extract") {
		t.Errorf("the report still names a kind of tool no run extracts:\n%s", r.stdout)
	}
}

// A run that met no client says nothing about them: a line that counts nothing
// on every run is a line nobody reads any more when it counts something.
func TestARunWithoutAClientSaysNothingAboutThem(t *testing.T) {
	e := newImportEnv(t)
	r := e.run("import", "basecamp", "--project", "7001", "--out", filepath.Join(t.TempDir(), "bundle"), "--styled")
	expectCode(t, r, ExitOK)

	for _, line := range []string{"Clients:", "Words about clients"} {
		if strings.Contains(r.stdout, line) {
			t.Errorf("a run without a client says %q:\n%s", line, r.stdout)
		}
	}
}
