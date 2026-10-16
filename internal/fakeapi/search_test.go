package fakeapi

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

func searchPath(values url.Values) string {
	return "/api/v1/search?" + values.Encode()
}

// A search is refused in the order Minka refuses it: a scope at all, then
// the scope itself, then the term, then the offset. Every scope that cannot
// be searched is the plain 404, whatever is wrong with it.
func TestASearchIsRefusedInTheOrderMinkaRefuses(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	trashedFolder := &Folder{ID: newID(), Title: "Thrown away", EntryState: EntryState{Trashed: true}}
	inside := &Folder{ID: newID(), Title: "Inside", ParentID: trashedFolder.ID}
	f.Files.Folders = append(f.Files.Folders, trashedFolder, inside)
	otherTool := &FilesTool{ID: newID(), ProjectID: f.Project.ID, Title: "More documents"}
	otherFolder := &Folder{ID: newID(), Title: "Elsewhere"}
	otherTool.Folders = []*Folder{otherFolder}
	s.FilesTools = append(s.FilesTools, otherTool)

	for _, c := range []struct {
		name   string
		token  string
		values url.Values
		want   string
	}{
		{"no scope, no term and no offset", f.Token, url.Values{"offset": {"x"}}, "422 searchScopeMissing"},
		{"a tool that is no ID, before the term", f.Token, url.Values{"toolId": {"files"}}, "404"},
		{"a folder that is no ID", f.Token, url.Values{"folderId": {"plans"}, "q": {"tent"}}, "404"},
		{"a Posts tool", f.Token, url.Values{"toolId": {f.Posts.ID}, "q": {"tent"}}, "404"},
		{"a tool of somebody else", f.Token, url.Values{"toolId": {f.OtherFiles.ID}, "q": {"tent"}}, "404"},
		{"a tool read by somebody outside the project", f.StrangerToken, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}}, "404"},
		{"a document is no folder", f.Token, url.Values{"folderId": {f.DocumentInFolder.ID}, "q": {"tent"}}, "404"},
		{"a folder that is not there", f.Token, url.Values{"folderId": {newID()}, "q": {"tent"}}, "404"},
		{"a folder in the trash", f.Token, url.Values{"folderId": {trashedFolder.ID}, "q": {"tent"}}, "404"},
		{"a folder below one in the trash", f.Token, url.Values{"folderId": {inside.ID}, "q": {"tent"}}, "404"},
		{"a folder outside the tool named", f.Token, url.Values{"toolId": {f.Files.ID}, "folderId": {otherFolder.ID}, "q": {"tent"}}, "404"},
		{"a folder not shown to a client, read by one", "minka_pat_client", url.Values{"folderId": {f.Folder.ID}, "q": {"tent"}}, "404"},
		{"no term, before the offset", f.Token, url.Values{"toolId": {f.Files.ID}, "offset": {"x"}}, "422 searchTermMissing"},
		{"a term of blanks", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {" \t "}}, "422 searchTermMissing"},
		{"an offset that is no number", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}, "offset": {"x"}}, "422 searchOffsetNotANumber"},
		{"an offset of seven digits", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}, "offset": {"1000000"}}, "422 searchOffsetNotANumber"},
		{"a negative offset", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}, "offset": {"-1"}}, "422 searchOffsetNotANumber"},
		{"an empty offset", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}, "offset": {""}}, "200"},
		{"a tool alone", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}}, "200"},
		{"a folder alone", f.Token, url.Values{"folderId": {f.Folder.ID}, "q": {"tent"}}, "200"},
		{"a folder within its tool", f.Token, url.Values{"toolId": {f.Files.ID}, "folderId": {f.Folder.ID}, "q": {"tent"}}, "200"},
		{"IDs in capitals", f.Token, url.Values{"toolId": {strings.ToUpper(f.Files.ID)}, "folderId": {strings.ToUpper(f.Folder.ID)}, "q": {"tent"}}, "200"},
		{"an archived folder", f.Token, url.Values{"folderId": {f.ArchivedFolder.ID}, "q": {"tent"}}, "200"},
		{"an unknown order, taken as the best", f.Token, url.Values{"toolId": {f.Files.ID}, "q": {"tent"}, "sort": {"newest"}}, "200"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _ := answerTo(t, s, c.token, http.MethodGet, searchPath(c.values), "")
			if got != c.want {
				t.Fatalf("answered %s, want %s", got, c.want)
			}
		})
	}
}

// A hit names the heading its match stands under, and its address opens the
// document there. A match before the first heading, or in the title alone,
// names no section.
func TestADocumentHitNamesTheSectionOfItsMatch(t *testing.T) {
	s, f := New(t)
	f.Document.Body = "<p>The stage goes up on Friday.</p><h2>Setup</h2><p>Eight people.</p><h2>Setup</h2><p>The tent comes on Saturday.</p>"

	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"TENT"}}), "")
	hit := hitWithID(t, answer, f.Document.ID)
	section, _ := hit["section"].(map[string]any)
	if section["heading"] != "Setup" || section["anchor"] != "h-setup-2" {
		t.Fatalf("section %v, want the second Setup", hit["section"])
	}
	if hit["appUrl"] != s.documentURL(f.Files, f.Document)+"#h-setup-2" {
		t.Errorf("appUrl %v", hit["appUrl"])
	}
	if excerpt := hit["excerpt"].(string); !strings.HasPrefix(excerpt, "The tent comes on Saturday.") || strings.Contains(excerpt, "<") {
		t.Errorf("excerpt %q", hit["excerpt"])
	}
	if hit["kind"] != "document" || hit["project"] != f.Project.Name || hit["toolId"] != f.Files.ID || hit["projectId"] != f.Project.ID {
		t.Errorf("hit %v", hit)
	}

	for _, term := range []string{"stage", "How the"} {
		_, answer = answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {term}}), "")
		hit = hitWithID(t, answer, f.Document.ID)
		if hit["section"] != nil || strings.Contains(hit["appUrl"].(string), "#") {
			t.Errorf("%q: section %v, appUrl %v", term, hit["section"], hit["appUrl"])
		}
	}
}

// A folder is searched with everything beneath it, what is archived
// included, and nothing beside it; a draft and what lies in the trash are found nowhere, an upload by the name of
// its file as well.
func TestASearchFindsWhatStandsInItsScope(t *testing.T) {
	s, f := New(t)
	below := &Folder{ID: newID(), Title: "Below", ParentID: f.Folder.ID}
	f.Files.Folders = append(f.Files.Folders, below)
	deep := &Document{ID: newID(), Title: "Tent pegs", FolderID: below.ID, UpdatedAt: time.Now()}
	draft := &Document{ID: newID(), Title: "Tent draft", Draft: true}
	trashed := &Document{ID: newID(), Title: "Tent gone", FolderID: f.Folder.ID, EntryState: EntryState{Trashed: true}}
	f.Files.Documents = append(f.Files.Documents, deep, draft, trashed)

	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"folderId": {f.Folder.ID}, "q": {"tent"}}), "")
	if ids := hitIDs(answer); !slices.Equal(slices.Sorted(slices.Values(ids)), slices.Sorted(slices.Values([]string{f.DocumentInFolder.ID, f.DocumentBelowTheArchivedFolder.ID, deep.ID}))) {
		t.Fatalf("hits %v, want the document of the folder and the ones below it, archived or not", ids)
	}

	for id, archived := range map[string]bool{f.DocumentInFolder.ID: false, f.DocumentBelowTheArchivedFolder.ID: true} {
		if got := hitWithID(t, answer, id)["archived"]; got != archived {
			t.Errorf("%s: archived %v, want %v", id, got, archived)
		}
	}
	_, answer = answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"done with"}}), "")
	if got := hitWithID(t, answer, f.ArchivedDocument.ID)["archived"]; got != true {
		t.Errorf("a document in the archive: archived %v", got)
	}

	// A client finds only what stands in a folder shown to clients.
	s.Tokens["minka_pat_client"] = f.Client.ID
	shown := &Document{ID: newID(), Title: "Tent for the client", FolderID: f.FolderShownToClients.ID}
	f.Files.Documents = append(f.Files.Documents, shown)
	_, answer = answerTo(t, s, "minka_pat_client", http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"tent"}}), "")
	if ids := hitIDs(answer); !slices.Equal(ids, []string{shown.ID}) {
		t.Errorf("a client finds %v, want only %s", ids, shown.ID)
	}

	f.FilesUpload.Description = ""
	_, answer = answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"SITE-PLAN.PDF"}}), "")
	hit := hitWithID(t, answer, f.FilesUpload.ID)
	if hit["kind"] != "upload" || hit["section"] != nil || hit["excerpt"] != "site-plan.pdf" {
		t.Errorf("hit %v", hit)
	}

	f.Project.Archived = true
	_, answer = answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"tent"}}), "")
	if ids := hitIDs(answer); len(ids) != 0 {
		t.Errorf("an archived project has hits: %v", ids)
	}
}

// A page holds 25 hits, and next is the offset of the page after it, as a
// string, and null on the last.
func TestASearchIsPagedByTwentyFive(t *testing.T) {
	s, f := New(t)
	for i := range 30 {
		f.Files.Documents = append(f.Files.Documents, &Document{ID: newID(), Title: fmt.Sprintf("Lamp %02d", i), UpdatedAt: time.Unix(int64(i), 0)})
	}

	_, first := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"lamp"}, "sort": {"recent"}}), "")
	if ids := hitIDs(first); len(ids) != 25 || first["next"] != "25" {
		t.Fatalf("%d hits, next %#v", len(ids), first["next"])
	}
	_, second := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"lamp"}, "sort": {"recent"}, "offset": {"25"}}), "")
	if ids := hitIDs(second); len(ids) != 5 || second["next"] != nil {
		t.Fatalf("%d hits, next %#v", len(ids), second["next"])
	}
	hits, _ := second["hits"].([]any)
	if last, _ := hits[4].(map[string]any); last["title"] != "Lamp 00" {
		t.Errorf("the last hit of the recent ones is %v", last["title"])
	}
}

// Hits of the same moment stand in the order of their identifiers, as Minka
// settles a tie.
func TestATieIsSettledByTheIdentifier(t *testing.T) {
	s, f := New(t)
	moment := time.Unix(100, 0)
	f.Files.Documents = append(f.Files.Documents,
		&Document{ID: "0192f3a4-5b6c-4d7e-8f90-00000000000b", Title: "Lantern", UpdatedAt: moment},
		&Document{ID: "0192f3a4-5b6c-4d7e-8f90-00000000000a", Title: "Lantern", UpdatedAt: moment})
	for _, sort := range []string{"best", "recent"} {
		_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"lantern"}, "sort": {sort}}), "")
		if ids := hitIDs(answer); !slices.Equal(ids, []string{"0192f3a4-5b6c-4d7e-8f90-00000000000a", "0192f3a4-5b6c-4d7e-8f90-00000000000b"}) {
			t.Errorf("%s: %v", sort, ids)
		}
	}
}

// The anchors of the headings are Minka's: "h-" and a slug, umlauts spelled
// out, accents dropped, other characters a dash, a repeat counted on.
func TestTheAnchorsOfHeadingsAreMinkas(t *testing.T) {
	headings := []string{"Setup", "Setup", "Über die Größe", "Café crème", " ¿? ", "Привет, мир", "A -- b", "h-setup-2", "Setup"}
	want := []string{"h-setup", "h-setup-2", "h-ueber-die-groesse", "h-cafe-creme", "h-section", "h-привет-мир", "h-a-b", "h-h-setup-2", "h-setup-3"}
	if got := headingAnchors(headings); !slices.Equal(got, want) {
		t.Fatalf("anchors %q, want %q", got, want)
	}
}

func hitIDs(answer map[string]any) []string {
	hits, _ := answer["hits"].([]any)
	ids := []string{}
	for _, hit := range hits {
		ids = append(ids, hit.(map[string]any)["id"].(string))
	}
	return ids
}

func hitWithID(t *testing.T, answer map[string]any, id string) map[string]any {
	t.Helper()
	hits, _ := answer["hits"].([]any)
	for _, hit := range hits {
		if hit := hit.(map[string]any); hit["id"] == id {
			return hit
		}
	}
	t.Fatalf("no hit %s among %v", id, answer)
	return nil
}
