package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

type searchData struct {
	Hits []struct {
		Kind, ID, Title, AppURL string
		Section                 *struct{ Heading, Anchor string }
	} `json:"hits"`
	Next *string `json:"next"`
}

// withLamps puts documents that all hold "lamp" into the fixture's tool, the
// newest first in the recent order: "Lamp 00" is the newest.
func withLamps(e *testEnv, count int) []string {
	ids := make([]string, count)
	for i := range count {
		ids[i] = fmt.Sprintf("0192f3a4-5b6c-4d7e-8f90-%012d", i)
		e.fixture.Files.Documents = append(e.fixture.Files.Documents, &fakeapi.Document{
			ID: ids[i], Title: fmt.Sprintf("Lamp %02d", i), CreatorID: e.fixture.Bruno.ID,
			UpdatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(-time.Duration(i) * time.Hour),
		})
	}
	return ids
}

func searchRequests(e *testEnv, before int) []url.Values {
	var queries []url.Values
	for _, request := range e.server.Requests()[before:] {
		if request.Path != "/api/v1/search" {
			e.t.Fatalf("a search sent %s %s", request.Method, request.Path)
		}
		query, err := url.ParseQuery(request.Query)
		if err != nil {
			e.t.Fatal(err)
		}
		queries = append(queries, query)
	}
	return queries
}

// runBeforeTheTerm runs a command with --json in front: behind "--" every
// word is the term, --json included.
func runBeforeTheTerm(e *testEnv, args ...string) (envelope, result) {
	e.t.Helper()
	r := e.run("", append([]string{"--json"}, args...)...)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		e.t.Fatalf("output is not a JSON envelope: %v\n%s", err, r.stdout)
	}
	return env, r
}

func decodeSearch(t *testing.T, raw json.RawMessage) searchData {
	t.Helper()
	var data searchData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

// The words of the term are one term, and the scope, the order and the offset
// go out as they were given.
func TestASearchSendsItsTermAndScope(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	before := len(e.server.Requests())
	env, r := e.runJSON("", "search", "tent", "costs", "--tool", f.Files.ID, "--folder", f.Folder.ID, "--sort", "recent")
	expectCode(t, r, ExitOK)
	queries := searchRequests(e, before)
	if len(queries) != 1 {
		t.Fatalf("%d requests", len(queries))
	}
	q := queries[0]
	if q.Get("q") != "tent costs" || q.Get("toolId") != f.Files.ID || q.Get("folderId") != f.Folder.ID || q.Get("sort") != "recent" || q.Has("offset") {
		t.Fatalf("query %v", q)
	}
	data := decodeSearch(t, env.Data)
	if len(data.Hits) != 1 || data.Hits[0].ID != f.DocumentInFolder.ID || data.Next != nil {
		t.Fatalf("data %+v", data)
	}
	if next, ok := decode(t, env.Data)["next"]; !ok || next != nil {
		t.Errorf("data says nothing of a next page: %s", env.Data)
	}
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Cmd != "minka docs show "+f.DocumentInFolder.ID {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

// What Minka would refuse, or answer with nothing or the best match without a
// word, is refused before any request: an unknown kind, kinds without files
// beside a tool or a folder, a project beside them, an unknown order and an
// offset beyond the deepest one.
func TestASearchIsCheckedBeforeItGoesOut(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, c := range []struct {
		name string
		args []string
	}{
		{"an unknown kind", []string{"search", "tent", "--kind", "cards"}},
		{"an empty kind between commas", []string{"search", "tent", "--kind", "chat,"}},
		{"a kind in capitals", []string{"search", "tent", "--kind", "Board"}},
		{"all is no kind", []string{"search", "tent", "--kind", "all"}},
		{"kinds without files beside a tool", []string{"search", "tent", "--tool", f.Files.ID, "--kind", "board", "--kind", "chat"}},
		{"kinds without files beside a folder", []string{"search", "tent", "--folder", f.Folder.ID, "--kind", "posts"}},
		{"a project beside a tool", []string{"search", "tent", "--tool", f.Files.ID, "--project", f.Project.ID}},
		{"a project beside a folder", []string{"search", "tent", "--folder", f.Folder.ID, "--project", f.Project.ID}},
		{"a project that is no ID", []string{"search", "tent", "--project", "Test-Projekt"}},
		{"an offset beyond 500", []string{"search", "tent", "--offset", "501"}},
		{"an offset of six digits", []string{"search", "tent", "--offset", "999999"}},
		{"an order Minka does not know", []string{"search", "tent", "--tool", f.Files.ID, "--sort", "newest"}},
		{"no term", []string{"search", "--tool", f.Files.ID}},
		{"a term of blanks", []string{"search", "  ", "--tool", f.Files.ID}},
		{"a tool that is no ID", []string{"search", "tent", "--tool", "files"}},
		{"a folder that is no ID", []string{"search", "tent", "--folder", "plans"}},
		{"no hit at all asked for", []string{"search", "tent", "--tool", f.Files.ID, "--limit", "0"}},
		{"an offset that is no number", []string{"search", "tent", "--tool", f.Files.ID, "--offset", "MjU="}},
		{"an empty offset", []string{"search", "tent", "--tool", f.Files.ID, "--offset", ""}},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := len(e.server.Requests())
			env, r := e.runJSON("", c.args...)
			expectCode(t, r, ExitUsage)
			if env.Code != CodeUsage {
				t.Errorf("code %q", env.Code)
			}
			if sent := e.server.Requests()[before:]; len(sent) != 0 {
				t.Errorf("%d requests went out", len(sent))
			}
		})
	}
}

// --limit is reached with as few pages as it takes, and the run stops inside
// a page where the limit falls there: next then points at the first hit left
// behind, and reading on from it misses none and repeats none.
func TestASearchReadsOnlyThePagesItsLimitNeeds(t *testing.T) {
	e := newTestEnv(t)
	ids := withLamps(e, 60)
	f := e.fixture

	for _, c := range []struct {
		limit    string
		requests int
		hits     int
		next     string
	}{
		{"10", 1, 10, "10"},
		{"25", 1, 25, "25"},
		{"30", 2, 30, "30"},
		{"50", 2, 50, "50"},
		{"100", 3, 60, ""},
	} {
		t.Run(c.limit, func(t *testing.T) {
			before := len(e.server.Requests())
			env, r := e.runJSON("", "search", "lamp", "--tool", f.Files.ID, "--sort", "recent", "--limit", c.limit)
			expectCode(t, r, ExitOK)
			if queries := searchRequests(e, before); len(queries) != c.requests {
				t.Fatalf("%d requests, want %d", len(queries), c.requests)
			}
			data := decodeSearch(t, env.Data)
			if len(data.Hits) != c.hits {
				t.Fatalf("%d hits, want %d", len(data.Hits), c.hits)
			}
			for i, hit := range data.Hits {
				if hit.ID != ids[i] {
					t.Fatalf("hit %d is %s, want %s", i, hit.ID, ids[i])
				}
			}
			if c.next == "" {
				if data.Next != nil {
					t.Fatalf("next %q at the end", *data.Next)
				}
				return
			}
			if data.Next == nil || *data.Next != c.next {
				t.Fatalf("next %v, want %s", data.Next, c.next)
			}
			crumb := env.Breadcrumbs[len(env.Breadcrumbs)-1]
			want := "minka search lamp --tool " + f.Files.ID + " --sort recent --limit " + c.limit + " --offset " + c.next
			if c.limit == "25" {
				want = "minka search lamp --tool " + f.Files.ID + " --sort recent --offset 25"
			}
			if crumb.Action != "next" || crumb.Cmd != want {
				t.Fatalf("breadcrumb %+v, want %s", crumb, want)
			}

			// Flags appended to the command, --json among them, stay flags.
			args := append(strings.Fields(strings.TrimPrefix(crumb.Cmd, "minka ")), "--limit", "5")
			onward, r := e.runJSON("", args...)
			expectCode(t, r, ExitOK)
			if rest := decodeSearch(t, onward.Data); len(rest.Hits) == 0 || rest.Hits[0].ID != ids[c.hits] {
				t.Fatalf("reading on starts at %+v, want %s", rest.Hits, ids[c.hits])
			}
		})
	}
}

// The text names each hit by its title and section, its excerpt on one line
// and its address with the anchor, and the command that reads on.
func TestASearchAsText(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Document.Body = "<h2>Setup</h2><p>Eight people.</p><h2>The tent</h2><p>It comes\n\non   Saturday.</p>"

	r := e.run("", "search", "saturday", "--tool", f.Files.ID, "--styled")
	expectCode(t, r, ExitOK)
	want := "- " + f.Document.ID + "  document How the stage goes up › The tent (in Test-Projekt)\n" +
		"  It comes on Saturday.\n" +
		"  " + e.server.URL + "/projects/" + f.Project.ID + "/files/" + f.Files.ID + "/" + f.Document.ID + "#h-the-tent\n"
	if r.stdout != want {
		t.Fatalf("text\n%s\nwant\n%s", r.stdout, want)
	}

	withLamps(e, 30)
	r = e.run("", "search", "lamp", "--tool", f.Files.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stderr, "there are more hits: minka search lamp --tool "+f.Files.ID+" --offset 25") || strings.Contains(r.stdout, "minka search") {
		t.Fatalf("the command that reads on is not a notice:\nstdout %s\nstderr %s", r.stdout, r.stderr)
	}

	r = e.run("", "search", "nothing like it", "--tool", f.Files.ID, "--styled")
	expectCode(t, r, ExitOK)
	if r.stdout != "No hits.\n" {
		t.Fatalf("text %q", r.stdout)
	}
}

// A term starting with a dash, Minka's NOT, reads on behind "--", where it is
// taken as the term and not as a flag.
func TestATermStartingWithADashReadsOn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	for i := range 30 {
		f.Files.Documents = append(f.Files.Documents, &fakeapi.Document{
			ID: fmt.Sprintf("0192f3a4-5b6c-4d7e-8f90-%012d", i), Title: fmt.Sprintf("Lamp -dark %02d", i), CreatorID: f.Bruno.ID,
		})
	}

	env, r := runBeforeTheTerm(e, "search", "--tool", f.Files.ID, "--", "-dark")
	expectCode(t, r, ExitOK)
	if len(env.Breadcrumbs) == 0 {
		t.Fatalf("no breadcrumbs: %+v", env)
	}
	crumb := env.Breadcrumbs[len(env.Breadcrumbs)-1]
	if crumb.Cmd != "minka search --tool "+f.Files.ID+" --offset 25 -- -dark" {
		t.Fatalf("breadcrumb %q", crumb.Cmd)
	}
	before := len(e.server.Requests())
	onward, r := runBeforeTheTerm(e, strings.Fields(strings.TrimPrefix(crumb.Cmd, "minka "))...)
	expectCode(t, r, ExitOK)
	if data := decodeSearch(t, onward.Data); len(data.Hits) != 5 {
		t.Fatalf("reading on found %d hits, want the last 5", len(data.Hits))
	}
	if queries := searchRequests(e, before); len(queries) != 1 || queries[0].Get("q") != "-dark" || queries[0].Get("offset") != "25" {
		t.Errorf("queries %v", queries)
	}
}

// An upload found first is shown as an upload, and its line says what it is.
func TestAnUploadFoundFirstIsShownAsAnUpload(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "search", "everything", "--folder", f.Folder.ID)
	expectCode(t, r, ExitOK)
	if data := decodeSearch(t, env.Data); len(data.Hits) != 1 || data.Hits[0].Kind != "upload" {
		t.Fatalf("data %+v", data)
	}
	if env.Breadcrumbs[0].Cmd != "minka docs uploads show "+f.FilesUpload.ID {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}
	r = e.run("", "search", "everything", "--folder", f.Folder.ID, "--styled")
	if !strings.HasPrefix(r.stdout, "- "+f.FilesUpload.ID+"  upload site-plan.pdf (in Test-Projekt)\n") {
		t.Errorf("text %q", r.stdout)
	}
}

// A hit in the archive, or below an archived folder, is marked as one.
func TestAnArchivedHitIsMarked(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	r := e.run("", "search", "tent cost", "--folder", f.Folder.ID, "--sort", "recent", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "- "+f.DocumentBelowTheArchivedFolder.ID+"  document What the tent cost last year (in Test-Projekt, archived)\n") {
		t.Errorf("the hit below the archived folder is not marked:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "- "+f.DocumentInFolder.ID+"  document What the tent costs (in Test-Projekt)\n") {
		t.Errorf("a hit in use is marked:\n%s", r.stdout)
	}
}

// A scope Minka cannot search is not found, with the way to a tool and a
// folder that can be.
func TestASearchInAScopeThatIsNotThereIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, args := range [][]string{
		{"--tool", f.OtherFiles.ID},
		{"--folder", f.Folder.ID, "--tool", f.Posts.ID},
		{"--folder", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"},
	} {
		env, r := e.runJSON("", append([]string{"search", "tent"}, args...)...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Hint, "minka docs tools <projectId>") || !strings.Contains(env.Hint, "minka docs list <toolId>") {
			t.Errorf("hint %q", env.Hint)
		}
	}
}

// A term longer than Minka searches for goes out as it is, and a notice says
// that only its start counts.
func TestATermLongerThanMinkaSearchesIsSaidSo(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "search", strings.Repeat("tent ", 41), "--tool", e.fixture.Files.ID)
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Notice, "first 200") {
		t.Errorf("notice %q", env.Notice)
	}
	env, _ = e.runJSON("", "search", "tent", "--tool", e.fixture.Files.ID)
	if strings.Contains(env.Notice, "200") {
		t.Errorf("notice %q for a short term", env.Notice)
	}
}

// --quiet prints the data alone: the hits and where to read on.
func TestASearchQuietPrintsTheHitsAndNext(t *testing.T) {
	e := newTestEnv(t)
	r := e.run("", "search", "tent", "--tool", e.fixture.Files.ID, "--quiet")
	expectCode(t, r, ExitOK)
	data := decodeSearch(t, json.RawMessage(r.stdout))
	if len(data.Hits) == 0 || !strings.Contains(r.stdout, `"next"`) {
		t.Fatalf("stdout %s", r.stdout)
	}
}
