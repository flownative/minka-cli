package cli

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// A search without a tool or a folder goes out with its kinds, each once and
// joined by commas, and its project, and asks for neither a tool nor a
// folder.
func TestASearchAcrossMinkaSendsItsKindsAndProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.CardInDoing.Title = "Zeppelin"

	before := len(e.server.Requests())
	env, r := e.runJSON("", "search", "zeppelin", "--kind", "board, posts", "--kind", "board", "--project", strings.ToUpper(f.Project.ID))
	expectCode(t, r, ExitOK)
	queries := searchRequests(e, before)
	if len(queries) != 1 {
		t.Fatalf("%d requests", len(queries))
	}
	q := queries[0]
	if q.Get("kinds") != "board,posts" || q.Get("projectId") != f.Project.ID || q.Has("toolId") || q.Has("folderId") {
		t.Fatalf("query %v", q)
	}
	if data := decodeSearch(t, env.Data); len(data.Hits) != 1 || data.Hits[0].ID != f.CardInDoing.ID {
		t.Fatalf("data %+v", data)
	}
	if env.Breadcrumbs[0].Cmd != "minka cards show "+f.CardInDoing.ID {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}

	// Files among the kinds go beside a tool, and the command that reads on
	// keeps kinds and project.
	_, r = e.runJSON("", "search", "tent", "--tool", f.Files.ID, "--kind", "files")
	expectCode(t, r, ExitOK)
	query := searchCommand(api.SearchQuery{Term: "zeppelin", Kinds: []string{"board", "posts"}, ProjectID: f.Project.ID, Sort: "best"}, 25, "25")
	if query != "minka search zeppelin --kind board,posts --project "+f.Project.ID+" --offset 25" {
		t.Errorf("command %q", query)
	}
}

// A project that is not there or not to be read exits 2 with the way to one
// that is.
func TestASearchInAProjectThatIsNotThereIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "search", "tent", "--project", e.fixture.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "minka projects list") {
		t.Errorf("hint %q", env.Hint)
	}
}

// The text names each hit by its kind: a card with its short address, a
// comment by what it stands under, a line of a chat by the chat, a line of a
// conversation by the other person.
func TestASearchAcrossMinkaAsText(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.CardInTriage.ShortID = "TEST-7"
	f.CardInTriage.Title = "Zeppelin card"
	comment := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-000000000c01", AuthorID: f.Anke.ID, Body: "<p>A zeppelin, really?</p>", CreatedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	f.CardInDoing.Comments = append(f.CardInDoing.Comments, comment)
	line := f.Chat.Import(f.Anke.ID, "the zeppelin is late", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	private := f.Conversation.Import(f.Anke.ID, "zeppelin, privately", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))

	r := e.run("", "search", "zeppelin", "--sort", "recent", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{
		"- " + f.CardInTriage.ID + "  card #TEST-7 Zeppelin card (in Test-Projekt)\n",
		"- " + comment.ID + "  comment on card " + f.CardInDoing.Title + " (in Test-Projekt)\n  A zeppelin, really?\n  " + e.server.URL + "/projects/" + f.Project.ID + "/boards/" + f.Board.ID + "/cards/" + f.CardInDoing.ID + "#comment-" + comment.ID + "\n",
		"- " + line.ID + "  chat line in " + f.Chat.Name + " (in Test-Projekt)\n  the zeppelin is late\n",
		"- " + private.ID + "  message with Anke\n  zeppelin, privately\n",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("no %q in\n%s", want, r.stdout)
		}
	}

	// A comment found first leads to its card; a line of a chat to nothing a
	// command reads at the line.
	env, r := e.runJSON("", "search", "really")
	expectCode(t, r, ExitOK)
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Cmd != "minka cards show "+f.CardInDoing.ID {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}
	env, r = e.runJSON("", "search", "late", "--kind", "chat")
	expectCode(t, r, ExitOK)
	if len(env.Breadcrumbs) != 0 {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}

	// Behind a line of a chat found first, the crumb reads the card of the
	// comment found after it, and says so without calling it the first hit.
	newest := f.Chat.Import(f.Anke.ID, "airship ahoy", time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC))
	f.CardInDoing.Comments = append(f.CardInDoing.Comments, &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-000000000c02", AuthorID: f.Anke.ID,
		Body: "airship below", CreatedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)})
	env, r = e.runJSON("", "search", "airship", "--sort", "recent")
	expectCode(t, r, ExitOK)
	if data := decodeSearch(t, env.Data); len(data.Hits) != 2 || data.Hits[0].ID != newest.ID {
		t.Fatalf("hits %+v", data.Hits)
	}
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Cmd != "minka cards show "+f.CardInDoing.ID ||
		env.Breadcrumbs[0].Description != "Read the card a comment found stands under" {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

// No offset beyond 500 is asked for or handed out, whether Minka names one
// there or not: the run stops at the deepest page, says so, and the hits it
// read are all there.
func TestASearchStopsAtTheDeepestOffset(t *testing.T) {
	for _, beyond := range []bool{false, true} {
		t.Run(fmt.Sprintf("Minka names pages beyond: %v", beyond), func(t *testing.T) {
			e := newTestEnv(t)
			e.server.NamesPagesBeyondTheDeepestOffset = beyond
			withLamps(e, 560)

			for _, c := range []struct {
				offset, limit string
				hits          int
				next          string
			}{
				{"450", "100", 75, ""},
				{"490", "20", 20, ""},
				{"450", "50", 50, "500"},
			} {
				before := len(e.server.Requests())
				env, r := e.runJSON("", "search", "lamp", "--sort", "recent", "--offset", c.offset, "--limit", c.limit)
				expectCode(t, r, ExitOK)
				for _, q := range searchRequests(e, before) {
					if n, _ := strconv.Atoi(q.Get("offset")); n > 500 {
						t.Fatalf("asked for offset %d", n)
					}
				}
				data := decodeSearch(t, env.Data)
				if len(data.Hits) != c.hits {
					t.Fatalf("offset %s, limit %s: %d hits, want %d", c.offset, c.limit, len(data.Hits), c.hits)
				}
				if c.next == "" {
					if data.Next != nil || !strings.Contains(env.Notice, "no further than offset 500") || strings.Contains(env.Notice, "--offset") {
						t.Errorf("offset %s, limit %s: next %v, notice %q", c.offset, c.limit, data.Next, env.Notice)
					}
					for _, crumb := range env.Breadcrumbs {
						if crumb.Action == "next" {
							t.Errorf("breadcrumb %+v", crumb)
						}
					}
				} else if data.Next == nil || *data.Next != c.next || strings.Contains(env.Notice, "no further than offset 500") {
					t.Errorf("offset %s, limit %s: next %v, notice %q", c.offset, c.limit, data.Next, env.Notice)
				}
			}
		})
	}
}

// A Minka that searches only within files exits 2 on a search without a tool
// or a folder, saying so; beside a tool it searches as before.
func TestAnOlderMinkaSearchesOnlyWithinATool(t *testing.T) {
	e := newTestEnv(t)
	e.server.SearchesOnlyWithinFiles = true

	env, r := e.runJSON("", "search", "tent", "--kind", "board")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Error, "searches only within a Docs & Files tool or folder; give --tool or --folder") || !strings.Contains(env.Hint, "minka docs tools") {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}

	env, r = e.runJSON("", "search", "tent", "--tool", e.fixture.Files.ID, "--kind", "files")
	expectCode(t, r, ExitOK)
	if data := decodeSearch(t, env.Data); len(data.Hits) == 0 || env.Breadcrumbs[0].Cmd != "minka docs show "+data.Hits[0].ID {
		t.Errorf("data %+v, breadcrumbs %+v", data, env.Breadcrumbs)
	}
}
