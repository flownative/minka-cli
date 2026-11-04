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

// zeppelins puts the word "zeppelin" into one of each kind the search finds,
// and into what it never finds.
type zeppelins struct {
	cardComment, documentComment, chatLine, conversationLine, otherChatLine string
	folder                                                                  *Folder
}

func withZeppelins(s *Server, f *Fixture) zeppelins {
	f.CardInDoing.Description = "<p>The zeppelin lands at noon.</p>"
	cardComment := &Comment{ID: newID(), AuthorID: f.Anke.ID, Body: "<p>A zeppelin, really?</p>", CreatedAt: s.now}
	f.CardInTriage.Comments = append(f.CardInTriage.Comments, cardComment)
	f.CardInTriage.ShortID = "TEST-7"
	f.OtherCard.Title = "Zeppelin of somebody else"

	f.Post.Body = "<p>On the zeppelin.</p>"
	f.DraftPost.Title = "Zeppelin draft"
	f.TrashedPost.Title = "Zeppelin gone"
	f.ArchivedPost.Title = "Old zeppelin"

	f.TodoList.Title = "Zeppelin list"
	f.StageGroup.Title = "Zeppelin stage"
	f.LooseTodo.Notes = "<p>Book the zeppelin.</p>"
	f.ArchivedListTodo.Title = "Zeppelin in the archive"
	f.TemplateTodo.Title = "Zeppelin template"

	folder := &Folder{ID: newID(), Title: "Zeppelin plans"}
	f.Files.Folders = append(f.Files.Folders, folder)
	documentComment := &Comment{ID: newID(), AuthorID: f.Anke.ID, Body: "zeppelin noted", CreatedAt: s.now}
	f.Document.Comments = append(f.Document.Comments, documentComment)
	f.DraftDocument.Title = "Zeppelin draft document"

	f.OnceEvent.Title = "Zeppelin day"

	chatLine := f.Chat.post(f.Anke.ID, "the zeppelin is late", "", s.now)
	conversationLine := f.Conversation.post(f.Anke.ID, "zeppelin, privately", "", s.now)
	gone := f.Chat.post(f.Anke.ID, "zeppelin taken back", "", s.now)
	gone.Removed = true
	otherChatLine := f.OtherChat.post(f.Stranger.ID, "zeppelin elsewhere", "", s.now)
	return zeppelins{cardComment: cardComment.ID, documentComment: documentComment.ID, chatLine: chatLine.ID,
		conversationLine: conversationLine.ID, otherChatLine: otherChatLine.ID, folder: folder}
}

func hitsByKind(answer map[string]any) []string {
	hits, _ := answer["hits"].([]any)
	found := []string{}
	for _, hit := range hits {
		hit := hit.(map[string]any)
		found = append(found, fmt.Sprint(hit["kind"], ":", hit["id"]))
	}
	slices.Sort(found)
	return found
}

func sortedOf(found ...string) []string {
	slices.Sort(found)
	return found
}

// Without a scope a search finds every kind the person may read, a comment
// under any item and a line of a chat or of their own conversation, and
// never a draft, the trash, a line taken back, another project or a
// template.
func TestASearchFindsEveryKindAcrossMinka(t *testing.T) {
	s, f := New(t)
	z := withZeppelins(s, f)

	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"ZEPPELIN"}}), "")
	want := sortedOf(
		"card:"+f.CardInDoing.ID, "comment:"+z.cardComment, "post:"+f.Post.ID, "post:"+f.ArchivedPost.ID,
		"todoList:"+f.TodoList.ID, "todoGroup:"+f.StageGroup.ID, "todo:"+f.LooseTodo.ID, "todo:"+f.ArchivedListTodo.ID,
		"folder:"+z.folder.ID, "comment:"+z.documentComment, "event:"+f.OnceEvent.ID,
		"chat:"+z.chatLine, "conversation:"+z.conversationLine)
	if got := hitsByKind(answer); !slices.Equal(got, want) {
		t.Fatalf("hits\n%v\nwant\n%v", got, want)
	}
	if got := hitWithID(t, answer, f.ArchivedListTodo.ID)["archived"]; got != true {
		t.Errorf("a to-do of a list in the archive: archived %v", got)
	}
	if got := hitWithID(t, answer, f.ArchivedPost.ID)["archived"]; got != true {
		t.Errorf("a post in the archive: archived %v", got)
	}

	// The template library and an archived project are searched for nobody.
	f.Project.Archived = true
	_, answer = answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}}), "")
	if got := hitsByKind(answer); !slices.Equal(got, []string{"conversation:" + z.conversationLine}) {
		t.Errorf("in an archived project: %v", got)
	}
}

// Each hit names itself and what its addresses lead to: a comment its item, a
// line its chat or conversation; a card and a comment under it the short
// address of the card.
func TestEachHitNamesWhatItLeadsTo(t *testing.T) {
	s, f := New(t)
	z := withZeppelins(s, f)
	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}}), "")

	comment := hitWithID(t, answer, z.cardComment)
	for field, want := range map[string]any{
		"targetKind": "card", "targetId": f.CardInTriage.ID, "title": f.CardInTriage.Title, "excerpt": "A zeppelin, really?",
		"shortId": "TEST-7", "shortUrl": s.URL + "/go/card/TEST-7", "project": f.Project.Name, "projectId": f.Project.ID,
		"toolId": f.Board.ID, "appUrl": s.cardURL(f.Board, f.CardInTriage) + "#comment-" + z.cardComment,
		"apiUrl": s.URL + "/api/v1/cards/" + f.CardInTriage.ID, "section": nil, "archived": false,
	} {
		if comment[field] != want {
			t.Errorf("comment %s: %#v, want %#v", field, comment[field], want)
		}
	}

	card := hitWithID(t, answer, f.CardInDoing.ID)
	if card["targetKind"] != "card" || card["targetId"] != f.CardInDoing.ID || card["shortId"] != nil || card["shortUrl"] != nil {
		t.Errorf("card %v", card)
	}

	chat := hitWithID(t, answer, z.chatLine)
	for field, want := range map[string]any{
		"targetKind": "chat", "targetId": f.Chat.ID, "title": f.Chat.Name, "projectId": f.Project.ID,
		"apiUrl": s.URL + "/api/v1/chats/" + f.Chat.ID + "/messages", "shortId": nil,
	} {
		if chat[field] != want {
			t.Errorf("chat %s: %#v, want %#v", field, chat[field], want)
		}
	}
	if !strings.HasSuffix(chat["appUrl"].(string), "#chat-message-"+z.chatLine) {
		t.Errorf("chat appUrl %v", chat["appUrl"])
	}

	conversation := hitWithID(t, answer, z.conversationLine)
	for field, want := range map[string]any{
		"targetKind": "conversation", "targetId": f.Conversation.ID, "title": f.Anke.Name,
		"projectId": nil, "toolId": nil, "project": nil, "apiUrl": s.URL + "/api/v1/chats/" + f.Conversation.ID + "/messages",
	} {
		if conversation[field] != want {
			t.Errorf("conversation %s: %#v, want %#v", field, conversation[field], want)
		}
	}

	if group := hitWithID(t, answer, f.StageGroup.ID); group["apiUrl"] != nil {
		t.Errorf("a group has an apiUrl: %v", group["apiUrl"])
	}
	if list := hitWithID(t, answer, f.TodoList.ID); list["apiUrl"] != s.URL+"/api/v1/todo-lists/"+f.TodoList.ID {
		t.Errorf("list apiUrl %v", list["apiUrl"])
	}
	if documentComment := hitWithID(t, answer, z.documentComment); documentComment["targetKind"] != "document" || documentComment["apiUrl"] != s.URL+"/api/v1/documents/"+f.Document.ID {
		t.Errorf("comment under a document %v", documentComment)
	}

	// Anke, the other one of the conversation, finds it under Bruno's name.
	_, answer = answerTo(t, s, "minka_pat_anke", http.MethodGet, searchPath(url.Values{"q": {"privately"}}), "")
	if len(hitIDs(answer)) != 0 {
		t.Fatalf("a token nobody has found %v", hitIDs(answer))
	}
	s.Tokens["minka_pat_anke"] = f.Anke.ID
	_, answer = answerTo(t, s, "minka_pat_anke", http.MethodGet, searchPath(url.Values{"q": {"privately"}}), "")
	if hit := hitWithID(t, answer, z.conversationLine); hit["title"] != f.Bruno.Name {
		t.Errorf("Anke's conversation is named %v", hit["title"])
	}
}

// The kinds narrow to their tools, a comment counting toward the tool of its
// item; a project narrows to itself and leaves the conversations out.
func TestKindsAndAProjectNarrowASearch(t *testing.T) {
	s, f := New(t)
	z := withZeppelins(s, f)

	for _, c := range []struct {
		kinds string
		want  []string
	}{
		{"board", []string{"card:" + f.CardInDoing.ID, "comment:" + z.cardComment}},
		{"chat", []string{"chat:" + z.chatLine}},
		{"conversation", []string{"conversation:" + z.conversationLine}},
		{"files", []string{"folder:" + z.folder.ID, "comment:" + z.documentComment}},
		{"calendar,posts", []string{"event:" + f.OnceEvent.ID, "post:" + f.Post.ID, "post:" + f.ArchivedPost.ID}},
	} {
		_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}, "kinds": {c.kinds}}), "")
		if got := hitsByKind(answer); !slices.Equal(got, sortedOf(c.want...)) {
			t.Errorf("%s: %v, want %v", c.kinds, got, sortedOf(c.want...))
		}
	}

	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}, "projectId": {f.Project.ID}, "kinds": {"chat,conversation"}}), "")
	if got := hitsByKind(answer); !slices.Equal(got, []string{"chat:" + z.chatLine}) {
		t.Errorf("within the project: %v", got)
	}

	// The stranger finds the line of their own project's chat, and narrowed to
	// it nothing of Bruno's.
	_, answer = answerTo(t, s, f.StrangerToken, http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}, "projectId": {f.OtherProject.ID}}), "")
	if got := hitsByKind(answer); !slices.Equal(got, sortedOf("card:"+f.OtherCard.ID, "chat:"+z.otherChatLine)) {
		t.Errorf("the stranger finds %v", got)
	}
}

// Beside a tool or a folder only documents and uploads are found: kinds
// without files and a project the tool does not stand in leave nothing.
func TestAScopeWithinFilesKeepsToItsEntries(t *testing.T) {
	s, f := New(t)
	withZeppelins(s, f)
	f.Document.Title = "Zeppelin document"

	for _, c := range []struct {
		name   string
		values url.Values
		want   []string
	}{
		{"the tool", url.Values{"toolId": {f.Files.ID}}, []string{"document:" + f.Document.ID}},
		{"with files among the kinds", url.Values{"toolId": {f.Files.ID}, "kinds": {"chat,files"}}, []string{"document:" + f.Document.ID}},
		{"with its own project", url.Values{"toolId": {f.Files.ID}, "projectId": {f.Project.ID}}, []string{"document:" + f.Document.ID}},
		{"kinds without files", url.Values{"toolId": {f.Files.ID}, "kinds": {"board,chat"}}, []string{}},
	} {
		values := c.values
		values.Set("q", "zeppelin")
		_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(values), "")
		if got := hitsByKind(answer); !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}

	// Another project of Bruno's, which the tool does not stand in.
	other := &Project{ID: newID(), Name: "Bruno's other", MemberIDs: []string{f.Bruno.ID}}
	s.Projects = append(s.Projects, other)
	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}, "toolId": {f.Files.ID}, "projectId": {other.ID}}), "")
	if got := hitsByKind(answer); len(got) != 0 {
		t.Errorf("a tool of another project finds %v", got)
	}
}

// A client finds no list hidden from the clients, no post they are not shown
// and nothing of a tool they are not let into.
func TestAClientFindsWhatTheClientsAreShown(t *testing.T) {
	s, f := New(t)
	withZeppelins(s, f)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.TodoList.HiddenFromClients = true
	f.Post.ShownToClients = false

	_, answer := answerTo(t, s, "minka_pat_client", http.MethodGet, searchPath(url.Values{"q": {"zeppelin"}, "kinds": {"todos,posts"}}), "")
	got := hitsByKind(answer)
	for _, hidden := range []string{"todoList:" + f.TodoList.ID, "todo:" + f.LooseTodo.ID, "post:" + f.Post.ID} {
		if slices.Contains(got, hidden) {
			t.Errorf("the client finds %s", hidden)
		}
	}
}

// The deepest page Minka serves starts at 500 and names no next page,
// whatever stands behind it; the older rule named one it then refused.
func TestASearchNamesNoPageBeyondTheDeepestOffset(t *testing.T) {
	s, f := New(t)
	for i := range 560 {
		f.Files.Documents = append(f.Files.Documents, &Document{ID: newID(), Title: fmt.Sprintf("Lamp %03d", i), UpdatedAt: time.Unix(int64(i), 0)})
	}
	for _, c := range []struct {
		offset   string
		beyond   bool
		wantNext any
	}{
		{"450", false, "475"},
		{"475", false, "500"},
		{"500", false, nil},
		{"500", true, "525"},
	} {
		s.NamesPagesBeyondTheDeepestOffset = c.beyond
		_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"lamp"}, "offset": {c.offset}}), "")
		if len(hitIDs(answer)) != 25 || answer["next"] != c.wantNext {
			t.Errorf("offset %s, beyond %v: %d hits, next %#v", c.offset, c.beyond, len(hitIDs(answer)), answer["next"])
		}
	}
}

// An older Minka asks for a tool or a folder before anything else, passes
// over kinds and a project, takes any offset of six digits, and its hits
// name no target.
func TestAnOlderMinkaSearchesOnlyWithinFiles(t *testing.T) {
	s, f := New(t)
	s.SearchesOnlyWithinFiles = true
	for i := range 30 {
		f.Files.Documents = append(f.Files.Documents, &Document{ID: newID(), Title: fmt.Sprintf("Lamp %02d", i)})
	}

	for _, c := range []struct {
		name   string
		values url.Values
		want   string
	}{
		{"no scope, before the kinds and the term", url.Values{"kinds": {"nonsense"}}, "422 searchScopeMissing"},
		{"kinds it does not know", url.Values{"toolId": {f.Files.ID}, "kinds": {"nonsense"}, "q": {"lamp"}}, "200"},
		{"a project that is none", url.Values{"toolId": {f.Files.ID}, "projectId": {"nope"}, "q": {"lamp"}}, "200"},
		{"an offset beyond 500", url.Values{"toolId": {f.Files.ID}, "offset": {"600"}, "q": {"lamp"}}, "200"},
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodGet, searchPath(c.values), ""); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"toolId": {f.Files.ID}, "q": {"lamp"}}), "")
	hits, _ := answer["hits"].([]any)
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	for _, field := range []string{"targetKind", "targetId", "shortId", "shortUrl", "apiUrl"} {
		if _, ok := hits[0].(map[string]any)[field]; ok {
			t.Errorf("an older Minka's hit carries %s", field)
		}
	}
}

// A comment is marked archived by its own item alone: not by the list a to-do
// stands in, nor by an archived folder above a document or an upload, which
// marks the item itself.
func TestACommentIsMarkedByItsOwnItemsArchive(t *testing.T) {
	s, f := New(t)
	upload := &FilesUpload{ID: newID(), Title: "Old plan", FolderID: f.ArchivedFolder.ID}
	f.Files.Uploads = append(f.Files.Uploads, upload)
	for _, c := range []struct {
		name     string
		comments *[]*Comment
		archived bool
	}{
		{"a to-do in an archived list", &f.ArchivedListTodo.Comments, false},
		{"a document below an archived folder", &f.DocumentBelowTheArchivedFolder.Comments, false},
		{"an upload below an archived folder", &upload.Comments, false},
		{"a list in the archive", &f.ArchivedList.Comments, true},
	} {
		comment := &Comment{ID: newID(), AuthorID: f.Bruno.ID, Body: "airship", CreatedAt: s.now}
		*c.comments = append(*c.comments, comment)
		_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"airship"}}), "")
		if got := hitWithID(t, answer, comment.ID)["archived"]; got != c.archived {
			t.Errorf("a comment under %s: archived %v, want %v", c.name, got, c.archived)
		}
		*c.comments = (*c.comments)[:len(*c.comments)-1]
	}

	_, answer := answerTo(t, s, f.Token, http.MethodGet, searchPath(url.Values{"q": {"old plan"}}), "")
	if got := hitWithID(t, answer, upload.ID)["archived"]; got != true {
		t.Errorf("the upload below the archived folder: archived %v", got)
	}
}
