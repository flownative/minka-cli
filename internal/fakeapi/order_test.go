package fakeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// sendAs sends a body of the given media type as the person of the token and
// says how it was answered: the status, and the code of a refusal behind it.
func sendAs(t *testing.T, s *Server, token, method, path, mediaType, body string) string {
	t.Helper()
	request, err := http.NewRequest(method, s.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", mediaType)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	var answer map[string]any
	_ = json.Unmarshal(raw, &answer)
	got := fmt.Sprint(response.StatusCode)
	if refusal, ok := answer["error"].(map[string]any); ok && response.StatusCode != http.StatusNotFound {
		got += " " + fmt.Sprint(refusal["code"])
	}
	return got
}

// Minka looks up what an address names before it reads the body, so whoever
// cannot see it is told 404 whatever the body is, a body that cannot be read
// or one of another media type included. The same body at an address that is
// there is refused for what it is.
func TestAnAddressIsLookedUpBeforeTheBodyIsRead(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Check"}
	f.CardInTriage.Steps = []*Step{step}
	card, comment := f.CardInTriage.ID, f.CardInTriage.Comments[0].ID
	todo, subtask := f.GroupedTodo.ID, f.GroupedTodo.Subtasks[0].ID
	unknown := newID()

	for _, c := range []struct{ method, hidden, seen string }{
		{"PATCH", "/projects/" + f.OtherProject.ID, "/projects/" + f.Project.ID},
		{"POST", "/boards/" + f.OtherBoard.ID + "/cards", "/boards/" + f.Board.ID + "/cards"},
		{"PATCH", "/cards/" + f.OtherCard.ID, "/cards/" + card},
		{"POST", "/cards/" + f.OtherCard.ID + "/moves", "/cards/" + card + "/moves"},
		{"PUT", "/columns/" + unknown + "/on-hold", "/columns/" + f.Doing.ID + "/on-hold"},
		{"POST", "/cards/" + f.OtherCard.ID + "/assignees", "/cards/" + card + "/assignees"},
		{"POST", "/cards/" + f.OtherCard.ID + "/steps", "/cards/" + card + "/steps"},
		{"PUT", "/cards/" + f.OtherCard.ID + "/notes", "/cards/" + card + "/notes"},
		{"PATCH", "/cards/" + card + "/steps/" + unknown, "/cards/" + card + "/steps/" + step.ID},
		{"PATCH", "/cards/" + card + "/comments/" + unknown, "/cards/" + card + "/comments/" + comment},
		{"POST", "/cards/" + card + "/comments/" + unknown + "/reactions", "/cards/" + card + "/comments/" + comment + "/reactions"},
		{"POST", "/chats/" + f.OtherChat.ID + "/messages", "/chats/" + f.Chat.ID + "/messages"},
		{"PATCH", "/chats/" + f.Chat.ID + "/messages/" + f.OtherChat.Messages[0].ID, "/chats/" + f.Chat.ID + "/messages/" + f.Chat.Messages[0].ID},
		{"POST", "/chats/" + f.Chat.ID + "/messages/" + unknown + "/reactions", "/chats/" + f.Chat.ID + "/messages/" + f.Chat.Messages[0].ID + "/reactions"},
		{"POST", "/chats/" + f.OtherChat.ID + "/read", "/chats/" + f.Chat.ID + "/read"},
		{"POST", "/checkins/" + f.OtherCheckinsTool.ID + "/questions", "/checkins/" + f.CheckinsTool.ID + "/questions"},
		{"PATCH", "/questions/" + f.OtherQuestion.ID, "/questions/" + f.DailyQuestion.ID},
		{"POST", "/questions/" + f.OtherQuestion.ID + "/answers", "/questions/" + f.DailyQuestion.ID + "/answers"},
		{"PATCH", "/answers/" + f.AnkesAnswer.ID, "/answers/" + f.BrunosAnswer.ID},
		{"POST", "/todos-tools/" + f.OtherTodos.ID + "/lists", "/todos-tools/" + f.Todos.ID + "/lists"},
		{"PATCH", "/todo-lists/" + f.OtherTodoList.ID, "/todo-lists/" + f.TodoList.ID},
		{"POST", "/todo-lists/" + f.OtherTodoList.ID + "/groups", "/todo-lists/" + f.TodoList.ID + "/groups"},
		{"PATCH", "/todo-groups/" + unknown, "/todo-groups/" + f.StageGroup.ID},
		{"POST", "/todo-groups/" + unknown + "/moves", "/todo-groups/" + f.StageGroup.ID + "/moves"},
		{"POST", "/todo-lists/" + f.OtherTodoList.ID + "/todos", "/todo-lists/" + f.TodoList.ID + "/todos"},
		{"PATCH", "/todos/" + f.OtherTodo.ID, "/todos/" + todo},
		{"POST", "/todos/" + f.OtherTodo.ID + "/moves", "/todos/" + todo + "/moves"},
		{"POST", "/todos/" + f.OtherTodo.ID + "/subtasks", "/todos/" + todo + "/subtasks"},
		{"PATCH", "/todos/" + f.OtherTodo.ID + "/subtasks/" + subtask, "/todos/" + todo + "/subtasks/" + subtask},
		{"POST", "/todos/" + todo + "/subtasks/" + unknown + "/moves", "/todos/" + todo + "/subtasks/" + subtask + "/moves"},
		{"POST", "/todo-lists/" + f.OtherTodoList.ID + "/series", "/todo-lists/" + f.TodoList.ID + "/series"},
		{"PATCH", "/todo-series/" + unknown, "/todo-series/" + f.Series.ID},
	} {
		for _, body := range []struct{ mediaType, text, refusal string }{
			{"application/json", `{`, "400 malformedBody"},
			{"application/json", `[]`, "400 malformedBody"},
			{"text/plain", `title=x`, "415 unsupportedMediaType"},
		} {
			if got := sendAs(t, s, f.Token, c.method, "/api/v1"+c.hidden, body.mediaType, body.text); got != "404" {
				t.Errorf("%s %s with %s: %s, want 404", c.method, c.hidden, body.text, got)
			}
			if got := sendAs(t, s, f.Token, c.method, "/api/v1"+c.seen, body.mediaType, body.text); got != body.refusal {
				t.Errorf("%s %s with %s: %s, want %s", c.method, c.seen, body.text, got, body.refusal)
			}
		}
	}
}

// Where Minka reads the body before it looks anything up, a body that cannot
// be read is refused at any address, but what it says is held only against an
// address that is there.
func TestABodyThatCannotBeReadComesFirstWhereMinkaReadsItFirst(t *testing.T) {
	s, f := New(t)
	unknown := newID()
	post, document, upload := f.Post.ID, f.Document.ID, f.FilesUpload.ID

	for _, c := range []struct{ method, path, valid string }{
		{"POST", "/cards/" + f.OtherCard.ID + "/comments", `{"body":""}`},
		{"POST", "/posts/" + f.OtherPost.ID + "/comments", `{"body":""}`},
		{"PATCH", "/posts/" + post + "/comments/" + unknown, `{"body":""}`},
		{"POST", "/posts/" + post + "/comments/" + unknown + "/reactions", `{"content":""}`},
		{"POST", "/documents/" + f.OtherDocument.ID + "/comments", `{"body":""}`},
		{"PATCH", "/documents/" + document + "/comments/" + unknown, `{"body":""}`},
		{"POST", "/documents/" + document + "/comments/" + unknown + "/reactions", `{"content":""}`},
		{"PATCH", "/files-uploads/" + upload + "/comments/" + unknown, `{"body":""}`},
		{"POST", "/files-uploads/" + upload + "/comments/" + unknown + "/reactions", `{"content":""}`},
		{"PATCH", "/posts/" + f.OtherPost.ID + "/comments/" + unknown, `{"body":""}`},
		{"POST", "/posts/" + f.OtherPost.ID + "/comments/" + unknown + "/reactions", `{"content":""}`},
		{"PATCH", "/documents/" + f.OtherDocument.ID + "/comments/" + unknown, `{"body":""}`},
		{"POST", "/documents/" + f.OtherDocument.ID + "/comments/" + unknown + "/reactions", `{"content":""}`},
		{"POST", "/files-uploads/" + unknown + "/comments", `{"body":""}`},
		{"PATCH", "/files-uploads/" + unknown + "/comments/" + unknown, `{"body":""}`},
		{"POST", "/files-uploads/" + unknown + "/comments/" + unknown + "/reactions", `{"content":""}`},
		{"PATCH", "/todos/" + f.GroupedTodo.ID + "/subtasks/" + unknown, `{"completed":true}`},
	} {
		if got := sendAs(t, s, f.Token, c.method, "/api/v1"+c.path, "application/json", `{`); got != "400 malformedBody" {
			t.Errorf("%s %s with a body that is no JSON: %s, want 400 malformedBody", c.method, c.path, got)
		}
		if got := sendAs(t, s, f.Token, c.method, "/api/v1"+c.path, "text/plain", `x`); got != "415 unsupportedMediaType" {
			t.Errorf("%s %s with a body of another type: %s, want 415", c.method, c.path, got)
		}
		if got := sendAs(t, s, f.Token, c.method, "/api/v1"+c.path, "application/json", c.valid); got != "404" {
			t.Errorf("%s %s with %s: %s, want 404", c.method, c.path, c.valid, got)
		}
	}

	// The text of a comment on a card is held against a card that is there.
	if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/cards/"+f.CardInTriage.ID+"/comments", `{"body":" "}`); got != "422 commentMissing" {
		t.Errorf("an empty comment on a card: %s", got)
	}
}

// A body that names a field the web keeps for its forms is refused before the
// address is looked at, as Minka refuses it before any action.
func TestArgumentsOfWebFormsAreRefusedBeforeTheAddress(t *testing.T) {
	s, f := New(t)
	for _, path := range []string{"/api/v1/cards/" + newID(), "/api/v1/cards/" + f.CardInTriage.ID} {
		for _, c := range []struct{ method, query, mediaType, body string }{
			{"PATCH", "", "application/json", `{"__trustedProperties":"x","title":"Y"}`},
			{"PATCH", "?__trustedProperties=x", "application/json", `{"title":"Y"}`},
			{"GET", "?__referrer[@action]=x", "", ``},
			{"PATCH", "", "application/x-www-form-urlencoded", `__trustedProperties=x&title=Y`},
			{"PATCH", "", "text/plain", `__trustedProperties=x`},
		} {
			if got := sendAs(t, s, f.Token, c.method, path+c.query, c.mediaType, c.body); got != "400 malformedBody" {
				t.Errorf("%s %s%s with %q: %s, want 400 malformedBody", c.method, path, c.query, c.body, got)
			}
		}
	}
	if f.CardInTriage.Title != "Write the CLI" {
		t.Errorf("the card was renamed: %q", f.CardInTriage.Title)
	}

	// A body without a media type is not read as arguments, and neither are
	// the bytes of an upload.
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/cards/"+newID(), "", `__trustedProperties=x`); got != "404" {
		t.Errorf("a body without a media type: %s, want 404", got)
	}
	if got := sendAs(t, s, f.Token, "POST", "/api/v1/projects/"+f.Project.ID+"/uploads?filename=a.txt", "text/plain", `__a=1`); got != "201" {
		t.Errorf("an upload whose bytes read like a form: %s, want 201", got)
	}
}

// Within the body Minka holds its fields in the order it reads them, and a
// card in the archive learns why it takes no write only after its body.
func TestTheFieldsOfABodyAreHeldInMinkasOrder(t *testing.T) {
	s, f := New(t)
	for _, c := range []struct{ method, path, body, want string }{
		{"POST", "/api/v1/chats/" + f.Chat.ID + "/messages", `{"clientMessageId":"nonsense","body":""}`, "422 chatMessageDraftNotAnIdentifier"},
		{"POST", "/api/v1/boards/" + f.Board.ID + "/cards", `{"title":"A","dueOn":"tomorrow","description":"` + strings.Repeat("x", 100001) + `"}`, "422 cardDueDateNotADate"},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.method, c.path, got, c.want)
		}
	}

	s.PutInTheArchive(f.CardInTriage)
	card := "/api/v1/cards/" + f.CardInTriage.ID
	for _, c := range []struct{ method, path, body, want string }{
		{"PATCH", card, `{}`, "422 nothingToChange"},
		{"PATCH", card, `{"description":"` + strings.Repeat("x", 100001) + `"}`, "422 cardDescriptionTooLong"},
		{"PATCH", card, `{"title":"Y"}`, "422 cardArchived"},
		{"POST", card + "/moves", `{}`, "422 columnIdMissing"},
		{"POST", card + "/moves", `{"columnId":"` + f.Done.ID + `"}`, "422 cardArchived"},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != c.want {
			t.Errorf("%s %s with %s: %s, want %s", c.method, c.path, c.body, got, c.want)
		}
	}
}

// A removed message is found in its chat, so an archived project is told
// first; otherwise a reaction on it is refused like a message that is not
// there.
func TestAReactionOnARemovedMessageIsNotFound(t *testing.T) {
	s, f := New(t)
	removed := f.Chat.Messages[1]
	removed.Removed = true
	path := "/api/v1/chats/" + f.Chat.ID + "/messages/" + removed.ID + "/reactions"
	for _, archived := range []bool{false, true} {
		f.Project.Archived = archived
		want := "404"
		if archived {
			want = "422 projectArchived"
		}
		if got, _ := answerTo(t, s, f.Token, "POST", path, `{"content":"🎉"}`); got != want {
			t.Errorf("a reaction, archived %v: %s, want %s", archived, got, want)
		}
		if got, _ := answerTo(t, s, f.Token, "DELETE", path+"?content=%F0%9F%91%8D", ``); got != want {
			t.Errorf("a reaction taken back, archived %v: %s, want %s", archived, got, want)
		}
	}
	if len(removed.Reactions) != 1 || len(removed.Reactions[0].PersonIDs) != 1 {
		t.Errorf("the reactions of a removed message changed: %+v", removed.Reactions)
	}
}

// Under an address that is there, a comment, a message, a step or a subtask
// that is not is 404 before the body and before the archive. One that is
// there has its body held before the archive is told.
func TestAThingUnderAnAddressIsLookedForBeforeTheBodyAndTheArchive(t *testing.T) {
	s, f := New(t)
	step := &Step{ID: newID(), Title: "Check"}
	f.CardInTriage.Steps = []*Step{step}
	f.Project.Archived = true
	card, comment := "/api/v1/cards/"+f.CardInTriage.ID, f.CardInTriage.Comments[0].ID
	chat, message := "/api/v1/chats/"+f.Chat.ID, f.Chat.Messages[0].ID
	post, postComment := "/api/v1/posts/"+f.Post.ID, f.Post.Comments[0].ID
	todo, subtask := "/api/v1/todos/"+f.GroupedTodo.ID, f.GroupedTodo.Subtasks[0].ID

	for _, c := range []struct{ method, missing, there, invalid, refusal, valid string }{
		{"PATCH", card + "/comments/" + newID(), card + "/comments/" + comment, `{"body":" "}`, "422 commentMissing", `{"body":"Yes"}`},
		{"DELETE", card + "/comments/" + newID(), card + "/comments/" + comment, ``, "", ``},
		{"POST", card + "/comments/" + newID() + "/reactions", card + "/comments/" + comment + "/reactions", `{"content":" "}`, "422 reactionMissing", `{"content":"🎉"}`},
		{"DELETE", card + "/comments/" + newID() + "/reactions?content=", card + "/comments/" + comment + "/reactions?content=", ``, "422 reactionMissing", ``},
		{"PATCH", card + "/steps/" + newID(), card + "/steps/" + step.ID, `{}`, "422 stepCompletedMissing", `{"completed":true}`},
		{"PATCH", chat + "/messages/" + newID(), chat + "/messages/" + message, `{"body":" "}`, "422 chatMessageMissing", `{"body":"Hi"}`},
		{"DELETE", chat + "/messages/" + newID(), chat + "/messages/" + message, ``, "", ``},
		{"POST", chat + "/messages/" + newID() + "/reactions", chat + "/messages/" + message + "/reactions", `{"content":" "}`, "422 reactionMissing", `{"content":"🎉"}`},
		{"DELETE", chat + "/messages/" + newID() + "/reactions?content=", chat + "/messages/" + message + "/reactions?content=", ``, "422 reactionMissing", ``},
		{"PATCH", post + "/comments/" + newID(), post + "/comments/" + postComment, `{"body":" "}`, "422 commentMissing", `{"body":"Yes"}`},
		{"DELETE", post + "/comments/" + newID(), post + "/comments/" + postComment, ``, "", ``},
		{"POST", post + "/comments/" + newID() + "/reactions", post + "/comments/" + postComment + "/reactions", `{"content":" "}`, "422 reactionMissing", `{"content":"🎉"}`},
		{"DELETE", post + "/comments/" + newID() + "/reactions?content=", post + "/comments/" + postComment + "/reactions?content=", ``, "422 reactionMissing", ``},
		{"POST", todo + "/subtasks/" + newID() + "/moves", todo + "/subtasks/" + subtask + "/moves", `{"previousSubtaskId":5}`, "400 malformedBody", `{}`},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.missing, c.invalid); got != "404" {
			t.Errorf("%s %s: %s, want 404", c.method, c.missing, got)
		}
		if c.refusal != "" {
			if got, _ := answerTo(t, s, f.Token, c.method, c.there, c.invalid); got != c.refusal {
				t.Errorf("%s %s with %s: %s, want %s", c.method, c.there, c.invalid, got, c.refusal)
			}
		}
		// A reaction taken back names itself in the query.
		there := strings.Replace(c.there, "?content=", "?content=x", 1)
		if got, _ := answerTo(t, s, f.Token, c.method, there, c.valid); got != "422 projectArchived" {
			t.Errorf("%s %s: %s, want 422 projectArchived", c.method, there, got)
		}
	}
}

// Somebody else's answer is not there for the person asking, before anything
// about the body: an edit that names nothing to change is refused only on
// their own.
func TestSomebodyElsesAnswerIsNotFoundBeforeTheBody(t *testing.T) {
	s, f := New(t)
	if got, _ := answerTo(t, s, f.Token, "PATCH", "/api/v1/answers/"+f.AnkesAnswer.ID, `{}`); got != "404" {
		t.Errorf("somebody else's answer: %s, want 404", got)
	}
	if got, _ := answerTo(t, s, f.Token, "PATCH", "/api/v1/answers/"+f.BrunosAnswer.ID, `{}`); got != "422 nothingToChange" {
		t.Errorf("one's own answer: %s, want 422 nothingToChange", got)
	}
}

// Every write on a card in the archive is refused with one code, the ways of
// Minka's own test, and nothing is written.
func TestEveryWriteToAnArchivedCardIsRefusedWithOneCode(t *testing.T) {
	s, f := New(t)
	archived := f.CardInTriage
	step := &Step{ID: newID(), Title: "Get an offer"}
	archived.Steps = []*Step{step}
	archived.AssigneeIDs = []string{f.Anke.ID}
	archived.SubscriberIDs = []string{f.Bruno.ID, f.Anke.ID}
	s.PutInTheArchive(archived)
	card := "/api/v1/cards/" + archived.ID
	comment := card + "/comments/" + archived.Comments[0].ID

	for way, c := range map[string]struct{ method, path, body string }{
		"renaming it":                         {"PATCH", card, `{"title":"Cancel the tent"}`},
		"moving it":                           {"POST", card + "/moves", `{"columnId":"` + f.Done.ID + `"}`},
		"writing a comment":                   {"POST", card + "/comments", `{"body":"Still open"}`},
		"changing a comment":                  {"PATCH", comment, `{"body":"Not after all"}`},
		"taking a comment back":               {"DELETE", comment, ``},
		"reacting to a comment":               {"POST", comment + "/reactions", `{"content":"🎉"}`},
		"taking a reaction off a comment":     {"DELETE", comment + "/reactions?content=%F0%9F%91%8D", ``},
		"assigning somebody":                  {"POST", card + "/assignees", `{"personId":"` + f.Anke.ID + `"}`},
		"taking somebody off":                 {"DELETE", card + "/assignees/" + f.Anke.ID, ``},
		"putting somebody on its list":        {"POST", card + "/subscribers/" + f.Agent.ID, ``},
		"taking somebody off its list":        {"DELETE", card + "/subscribers/" + f.Anke.ID, ``},
		"adding a step":                       {"POST", card + "/steps", `{"title":"Compare prices"}`},
		"ticking a step off":                  {"PATCH", card + "/steps/" + step.ID, `{"completed":true}`},
		"taking a step off":                   {"DELETE", card + "/steps/" + step.ID, ``},
		"taking off a step that is not there": {"DELETE", card + "/steps/" + newID(), ``},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != "422 cardArchived" {
			t.Errorf("%s: %s, want 422 cardArchived", way, got)
		}
	}
	if archived.Title != "Write the CLI" || len(archived.Comments) != 1 || archived.Comments[0].Body != "First comment" ||
		len(archived.Comments[0].Reactions) != 2 || len(archived.Steps) != 1 || archived.Steps[0].Completed ||
		len(archived.AssigneeIDs) != 1 || len(archived.SubscriberIDs) != 2 || s.inTheArchive(archived.ID) == nil {
		t.Errorf("a refused write changed the card: %+v", archived)
	}

	// Its notes are read, and a write on them is held for its body first, then
	// refused, the same text as stored included.
	archived.Notes = "Call the rental"
	if got, _ := answerTo(t, s, f.Token, "GET", card+"/notes", ``); got != "200" {
		t.Errorf("the notes of an archived card: %s", got)
	}
	for body, want := range map[string]string{
		`{}`:                         "422 cardNotesMissing",
		`{"body":"Call them"}`:       "422 cardArchived",
		`{"body":"Call the rental"}`: "422 cardArchived",
	} {
		if got, _ := answerTo(t, s, f.Token, "PUT", card+"/notes", body); got != want {
			t.Errorf("notes %s: %s, want %s", body, got, want)
		}
	}
	if archived.Notes != "Call the rental" {
		t.Errorf("the notes were written: %q", archived.Notes)
	}

	// In an archived project the project is told first.
	f.Project.Archived = true
	if got, _ := answerTo(t, s, f.Token, "POST", card+"/steps", `{"title":"Compare prices"}`); got != "422 projectArchived" {
		t.Errorf("a step on an archived card of an archived project: %s", got)
	}
}
