package fakeapi

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// Minka writes an identifier in lowercase before it reads it, wherever it
// stands: in the path, in the query, in a field and in a list of people. One in
// capitals names what it names in lowercase. The name a client gives a draft of
// a chat message is no identifier of Minka's and stays refused in capitals.
func TestAnIdentifierInCapitalsNamesWhatItNamesInLowercase(t *testing.T) {
	up := strings.ToUpper
	type probe struct {
		name, method, path, body, want string
		agent                          bool
		prepare                        func(s *Server, f *Fixture)
		// done says whether the write took place, where the status alone
		// cannot tell: a removal answers 204 for what was not there as well.
		done func(f *Fixture) bool
	}
	withStep := func(_ *Server, f *Fixture) {
		f.CardInTriage.Steps = append(f.CardInTriage.Steps, &Step{ID: newID(), Title: "Check"})
	}
	stepPath := func(f *Fixture) string {
		return "/api/v1/cards/" + f.CardInTriage.ID + "/steps/" + up(f.CardInTriage.Steps[0].ID)
	}
	probes := []probe{
		{name: "path card", method: "GET", want: "200", path: "/api/v1/cards/{card}"},
		{name: "path notes", method: "GET", want: "200", path: "/api/v1/cards/{CARD}/notes"},
		{name: "conversation upload", method: "POST", want: "201", path: "/api/v1/messages/{CONVERSATION}/uploads?filename=plan.txt", body: "plan"},
		{name: "reply to", method: "POST", path: "/api/v1/chats/{chat}/messages", body: `{"body":"<p>hi</p>","replyTo":"{FIRSTMESSAGE}"}`, want: "201"},
		{name: "move next", method: "POST", path: "/api/v1/cards/{card}/moves", body: `{"columnId":"{doing}","nextCardId":"{CARDINDOING}"}`, want: "200"},
		{name: "todo move next", method: "POST", path: "/api/v1/todos/{loose}/moves", body: `{"nextTodoId":"{GROUPED}","groupId":"{stage}"}`, want: "200"},
		{name: "group move next", method: "POST", path: "/api/v1/todo-groups/{stage}/moves", body: `{"nextGroupId":"{KITCHEN}"}`, want: "200"},
		{name: "subtask move next", method: "POST", path: "/api/v1/todos/{grouped}/subtasks/{SECONDSUBTASK}/moves", body: `{"nextSubtaskId":"{SUBTASK}"}`, want: "200"},
		{name: "doc notify everybody", method: "POST", path: "/api/v1/files-tools/{files}/documents", body: `{"title":"t","body":"<p>b</p>","audience":"everybody","notifyIds":["{ANKE}"]}`, want: "422 notifyIdsWithoutChosenAudience"},
		{name: "doc notify chosen", method: "POST", path: "/api/v1/files-tools/{files}/documents", body: `{"title":"t","body":"<p>b</p>","audience":"chosen","notifyIds":["{ANKE}"]}`, want: "201"},
		{name: "doc folder", method: "POST", path: "/api/v1/files-tools/{files}/documents", body: `{"title":"t","body":"<p>b</p>","audience":"nobody","folderId":"{FOLDER}"}`, want: "201"},
		{name: "post notify everybody", method: "POST", path: "/api/v1/posts-tools/{posts}/posts", body: `{"title":"t","body":"<p>b</p>","audience":"everybody","notifyIds":["{ANKE}"]}`, want: "422 notifyIdsWithoutChosenAudience"},
		{name: "post category", method: "POST", path: "/api/v1/posts-tools/{posts}/posts", body: `{"title":"t","body":"<p>b</p>","audience":"nobody","categoryId":"{ANNOUNCEMENT}"}`, want: "201"},
		{name: "post edit category", method: "PATCH", path: "/api/v1/posts/{post}", body: `{"categoryId":"{ANNOUNCEMENT}"}`, want: "200"},
		{name: "card assignees", method: "POST", path: "/api/v1/boards/{board}/cards", body: `{"title":"t","assigneeIds":["{ANKE}"]}`, want: "201"},
		{name: "card column", method: "POST", path: "/api/v1/boards/{board}/cards", body: `{"title":"t","columnId":"{DOING}"}`, want: "201"},
		{name: "move column", method: "POST", path: "/api/v1/cards/{card}/moves", body: `{"columnId":"{DOING}"}`, want: "200"},
		{name: "move prev", method: "POST", path: "/api/v1/cards/{card}/moves", body: `{"columnId":"{doing}","previousCardId":"{CARDINDOING}"}`, want: "200"},
		{name: "assign", method: "POST", path: "/api/v1/cards/{card}/assignees", body: `{"personId":"{ANKE}"}`, want: "201"},
		{name: "unassign", method: "DELETE", path: "/api/v1/cards/{card}/assignees/{ANKE}", want: "204",
			prepare: func(_ *Server, f *Fixture) { f.CardInTriage.AssigneeIDs = []string{f.Anke.ID} },
			done:    func(f *Fixture) bool { return len(f.CardInTriage.AssigneeIDs) == 0 }},
		{name: "subscribe", method: "POST", path: "/api/v1/cards/{card}/subscribers/{ANKE}", want: "200"},
		{name: "unsubscribe", method: "DELETE", path: "/api/v1/cards/{card}/subscribers/{ANKE}", want: "204",
			prepare: func(_ *Server, f *Fixture) { f.CardInTriage.SubscriberIDs = []string{f.Anke.ID} },
			done:    func(f *Fixture) bool { return !slices.Contains(f.CardInTriage.SubscriberIDs, f.Anke.ID) }},
		{name: "step patch", method: "PATCH", body: `{"completed":true}`, want: "200", prepare: withStep,
			done: func(f *Fixture) bool { return f.CardInTriage.Steps[0].Completed }},
		{name: "step delete", method: "DELETE", want: "204", prepare: withStep,
			done: func(f *Fixture) bool { return len(f.CardInTriage.Steps) == 0 }},
		{name: "work", method: "PUT", path: "/api/v1/me/work", body: `{"clientId":"c","cardIds":["{CARD}"]}`, want: "204", agent: true},
		{name: "todo assignees", method: "POST", path: "/api/v1/todo-lists/{list}/todos", body: `{"title":"t","assigneeIds":["{ANKE}"]}`, want: "201"},
		{name: "todo notified", method: "POST", path: "/api/v1/todo-lists/{list}/todos", body: `{"title":"t","notifiedIds":["{ANKE}"]}`, want: "201"},
		{name: "todo add group", method: "POST", path: "/api/v1/todo-lists/{list}/todos", body: `{"title":"t","groupId":"{STAGE}"}`, want: "201"},
		{name: "todo move list", method: "POST", path: "/api/v1/todos/{loose}/moves", body: `{"listId":"{LIST}","position":"top"}`, want: "200"},
		{name: "todo move prev", method: "POST", path: "/api/v1/todos/{loose}/moves", body: `{"previousTodoId":"{GROUPED}","groupId":"{stage}"}`, want: "200"},
		{name: "group move", method: "POST", path: "/api/v1/todo-groups/{stage}/moves", body: `{"previousGroupId":"{KITCHEN}"}`, want: "200"},
		{name: "subtask assignee", method: "POST", path: "/api/v1/todos/{grouped}/subtasks", body: `{"title":"t","assigneeId":"{ANKE}"}`, want: "201"},
		{name: "subtask patch", method: "PATCH", path: "/api/v1/todos/{grouped}/subtasks/{SUBTASK}", body: `{"title":"x"}`, want: "200"},
		{name: "subtask move", method: "POST", path: "/api/v1/todos/{grouped}/subtasks/{SUBTASK}/moves", body: `{"position":"top"}`, want: "200"},
		{name: "subtask move prev", method: "POST", path: "/api/v1/todos/{grouped}/subtasks/{subtask}/moves", body: `{"previousSubtaskId":"{SECONDSUBTASK}"}`, want: "200"},
		{name: "subtask delete", method: "DELETE", path: "/api/v1/todos/{grouped}/subtasks/{SUBTASK}", want: "204",
			done: func(f *Fixture) bool { return len(f.GroupedTodo.Subtasks) == 1 }},
		{name: "question asked", method: "POST", path: "/api/v1/checkins/{checkins}/questions", body: `{"title":"t","rhythm":"daily","timeOfDay":"09:00","weekdays":[1],"askedIds":["{ANKE}"]}`, want: "201"},
		{name: "question edit asked", method: "PATCH", path: "/api/v1/questions/{daily}", body: `{"askedIds":["{ANKE}"]}`, want: "200"},
		{name: "chat client id", method: "POST", path: "/api/v1/chats/{chat}/messages", body: `{"body":"<p>hi</p>","clientMessageId":"{NEW}"}`, want: "422 chatMessageDraftNotAnIdentifier"},
		{name: "conversation start", method: "POST", path: "/api/v1/messages/with/{ANKE}", want: "200"},
		{name: "conversation with oneself", method: "POST", path: "/api/v1/messages/with/{BRUNO}", want: "404"},
		{name: "archive before", method: "GET", path: "/api/v1/boards/{board}/archive?before={CARD}", want: "200",
			prepare: func(s *Server, f *Fixture) { s.PutInTheArchive(f.CardInTriage) }},
		{name: "my todos after", method: "GET", path: "/api/v1/me/todos?after={LOOSE}", want: "200"},
		{name: "answers person", method: "GET", path: "/api/v1/questions/{daily}/answers?personId={ANKE}", want: "200"},
		{name: "answers before", method: "GET", path: "/api/v1/questions/{daily}/answers?before={BRUNOSANSWER}", want: "200"},
		{name: "entries folder", method: "GET", path: "/api/v1/files-tools/{files}/entries?folderId={FOLDER}", want: "200"},
	}
	for _, p := range probes {
		s, f := New(t)
		if p.prepare != nil {
			p.prepare(s, f)
		}
		path := p.path
		if path == "" {
			path = stepPath(f)
		}
		names := map[string]string{
			"card": f.CardInTriage.ID, "cardInDoing": f.CardInDoing.ID, "anke": f.Anke.ID, "bruno": f.Bruno.ID,
			"files": f.Files.ID, "folder": f.Folder.ID, "posts": f.Posts.ID, "post": f.Post.ID,
			"announcement": f.Announcement.ID, "board": f.Board.ID, "doing": f.Doing.ID,
			"list": f.TodoList.ID, "loose": f.LooseTodo.ID, "grouped": f.GroupedTodo.ID,
			"stage": f.StageGroup.ID, "kitchen": f.KitchenGroup.ID,
			"subtask": f.GroupedTodo.Subtasks[0].ID, "secondSubtask": f.GroupedTodo.Subtasks[1].ID,
			"checkins": f.CheckinsTool.ID, "daily": f.DailyQuestion.ID, "chat": f.Chat.ID,
			"brunosAnswer": f.BrunosAnswer.ID, "new": newID(),
			"conversation": f.Conversation.ID, "firstMessage": f.Chat.Messages[0].ID,
		}
		body := p.body
		for name, id := range names {
			for _, text := range []*string{&path, &body} {
				*text = strings.ReplaceAll(*text, "{"+up(name)+"}", up(id))
				*text = strings.ReplaceAll(*text, "{"+name+"}", id)
			}
		}
		token := f.Token
		if p.agent {
			token = f.AgentToken
		}
		if got, _ := answerTo(t, s, token, p.method, path, body); got != p.want {
			t.Errorf("%s: %s, want %s", p.name, got, p.want)
		}
		if p.done != nil && !p.done(f) {
			t.Errorf("%s: answered, and nothing was written", p.name)
		}
	}
}

// Minka answers an address whose identifier is none with the 404 of an
// address that names nothing, before it asks whether the project is archived:
// only an identifier that could name something learns that the project is,
// and of a comment, a message or a step only one that is there.
func TestAnAddressThatNamesNoIdentifierIsNotFoundBeforeTheArchive(t *testing.T) {
	s, f := New(t)
	f.CardInTriage.Steps = []*Step{{ID: newID(), Title: "Check"}}
	f.Project.Archived = true
	card, todo := f.CardInTriage.ID, f.GroupedTodo.ID
	document, upload, chat := f.Document.ID, f.FilesUpload.ID, f.Chat.ID
	for _, p := range []struct{ method, path, body string }{
		{"DELETE", "/api/v1/cards/" + card + "/comments/%s", ""},
		{"DELETE", "/api/v1/documents/" + document + "/comments/%s", ""},
		{"DELETE", "/api/v1/files-uploads/" + upload + "/comments/%s", ""},
		{"DELETE", "/api/v1/chats/" + chat + "/messages/%s", ""},
		{"DELETE", "/api/v1/cards/" + card + "/assignees/%s", ""},
		{"POST", "/api/v1/cards/" + card + "/subscribers/%s", ""},
		{"DELETE", "/api/v1/cards/" + card + "/subscribers/%s", ""},
		{"PATCH", "/api/v1/cards/" + card + "/steps/%s", `{"completed":true}`},
		{"DELETE", "/api/v1/cards/" + card + "/steps/%s", ""},
		{"PATCH", "/api/v1/todos/" + todo + "/subtasks/%s", `{"title":"x"}`},
		{"POST", "/api/v1/todos/" + todo + "/subtasks/%s/moves", `{"position":"top"}`},
		{"DELETE", "/api/v1/todos/" + todo + "/subtasks/%s", ""},
	} {
		if got, _ := answerTo(t, s, f.Token, p.method, fmt.Sprintf(p.path, "nonsense"), p.body); got != "404" {
			t.Errorf("%s %s with no identifier: %s, want 404", p.method, p.path, got)
		}
	}

	// A comment, a message or a step that is not there is looked for before
	// the archive, and is not found.
	for _, p := range []struct{ method, path, body string }{
		{"DELETE", "/api/v1/cards/" + card + "/comments/" + newID(), ""},
		{"DELETE", "/api/v1/documents/" + document + "/comments/" + newID(), ""},
		{"DELETE", "/api/v1/files-uploads/" + upload + "/comments/" + newID(), ""},
		{"DELETE", "/api/v1/chats/" + chat + "/messages/" + newID(), ""},
		{"PATCH", "/api/v1/cards/" + card + "/steps/" + newID(), `{"completed":true}`},
	} {
		if got, _ := answerTo(t, s, f.Token, p.method, p.path, p.body); got != "404" {
			t.Errorf("%s %s: %s, want 404", p.method, p.path, got)
		}
	}

	// Any other identifier that names nothing is asked about the archive.
	for _, p := range []struct{ method, path, body string }{
		{"DELETE", "/api/v1/cards/" + card + "/assignees/" + newID(), ""},
		{"POST", "/api/v1/cards/" + card + "/subscribers/" + newID(), ""},
		{"DELETE", "/api/v1/cards/" + card + "/subscribers/" + newID(), ""},
		{"DELETE", "/api/v1/cards/" + card + "/steps/" + newID(), ""},
		{"DELETE", "/api/v1/todos/" + todo + "/subtasks/" + newID(), ""},
	} {
		if got, _ := answerTo(t, s, f.Token, p.method, p.path, p.body); got != "422 projectArchived" {
			t.Errorf("%s %s: %s, want 422 projectArchived", p.method, p.path, got)
		}
	}
}

// Removing a step or a subtask that is not there answers 204, and one of
// another card or to-do is not there; an address that names no step or
// subtask at all is 404, and so is a subscriber that is no identifier.
func TestARemovalNamingNoIdentifierIsNotFound(t *testing.T) {
	s, f := New(t)
	foreignStep := &Step{ID: newID(), Title: "Theirs"}
	f.CardInDoing.Steps = []*Step{foreignStep}
	card, todo := f.CardInTriage.ID, f.GroupedTodo.ID

	for _, p := range []struct{ method, path, want string }{
		{"DELETE", "/api/v1/cards/" + card + "/steps/nonsense", "404"},
		{"DELETE", "/api/v1/todos/" + todo + "/subtasks/nonsense", "404"},
		{"POST", "/api/v1/cards/" + card + "/subscribers/nonsense", "404"},
		{"DELETE", "/api/v1/cards/" + card + "/subscribers/nonsense", "404"},
		{"DELETE", "/api/v1/cards/" + card + "/steps/" + foreignStep.ID, "204"},
		{"PATCH", "/api/v1/cards/" + card + "/steps/" + foreignStep.ID, "404"},
		{"DELETE", "/api/v1/todos/" + todo + "/subtasks/" + f.LooseTodo.ID, "204"},
	} {
		body := ""
		if p.method == "PATCH" {
			body = `{"completed":true}`
		}
		if got, _ := answerTo(t, s, f.Token, p.method, p.path, body); got != p.want {
			t.Errorf("%s %s: %s, want %s", p.method, p.path, got, p.want)
		}
	}
	if len(f.CardInDoing.Steps) != 1 || f.CardInDoing.Steps[0].Completed {
		t.Errorf("the step of another card was touched: %+v", f.CardInDoing.Steps)
	}
}

// A reply answers a message of the same chat that still stands; a value that
// is no identifier, and a message of another chat, are refused alike, but only
// the first before an archived project.
func TestAReplyAnswersAMessageOfTheSameChat(t *testing.T) {
	s, f := New(t)
	removed := f.Chat.Messages[1]
	removed.Removed = true
	for replyTo, want := range map[string]string{
		removed.ID:                 "422 chatMessageReplyToUnknown",
		"nonsense":                 "422 chatMessageReplyToUnknown",
		f.OtherChat.Messages[0].ID: "422 chatMessageReplyToUnknown",
		f.Chat.Messages[0].ID:      "201",
		"":                         "201",
	} {
		body := `{"body":"<p>hi</p>","replyTo":"` + replyTo + `"}`
		if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/chats/"+f.Chat.ID+"/messages", body); got != want {
			t.Errorf("replyTo %q: %s, want %s", replyTo, got, want)
		}
	}

	// A value that is no identifier is refused before an archived project is
	// told; a message that is not there, after.
	f.Project.Archived = true
	for replyTo, want := range map[string]string{
		"nonsense": "422 chatMessageReplyToUnknown",
		newID():    "422 projectArchived",
	} {
		body := `{"body":"<p>hi</p>","replyTo":"` + replyTo + `"}`
		if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/chats/"+f.Chat.ID+"/messages", body); got != want {
			t.Errorf("replyTo %q in an archived project: %s, want %s", replyTo, got, want)
		}
	}
}

// The reply is read only after the chat is found, so a chat the person may
// not see answers 404 whatever the reply carries.
func TestAReplyIsReadAfterTheChat(t *testing.T) {
	s, f := New(t)
	body := `{"body":"<p>hi</p>","replyTo":1}`
	if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/chats/"+f.OtherChat.ID+"/messages", body); got != "404" {
		t.Errorf("a reply that is no string to a chat not seen: %s, want 404", got)
	}
	if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/chats/"+f.Chat.ID+"/messages", body); got != "400 malformedBody" {
		t.Errorf("a reply that is no string: %s, want 400 malformedBody", got)
	}
}

// A person on a new card who is named by no identifier answers like one who is
// no member, and before the description, the column and the archive are asked
// about: Minka reads the people right after the title and the day, and asks
// about membership of the rest only once the card is being written.
func TestAPersonOnANewCardNamedByNoIdentifierIsRefusedFirst(t *testing.T) {
	for _, c := range []struct {
		name, body string
		archived   bool
	}{
		{"beside a column that is not there", `{"title":"t","assigneeIds":["nobody"],"columnId":"nonsense"}`, false},
		{"beside a description too long", `{"title":"t","assigneeIds":["nobody"],"description":"` + strings.Repeat("x", maxDescriptionLength+1) + `"}`, false},
		{"in an archived project", `{"title":"t","assigneeIds":["nobody"]}`, true},
	} {
		s, f := New(t)
		f.Project.Archived = c.archived
		before := len(f.Triage.Cards)
		if got, _ := answerTo(t, s, f.Token, "POST", "/api/v1/boards/"+f.Board.ID+"/cards", c.body); got != "422 personNotAProjectMember" {
			t.Errorf("%s: %s, want 422 personNotAProjectMember", c.name, got)
		}
		if len(f.Triage.Cards) != before {
			t.Errorf("%s: a refused card was written", c.name)
		}
	}
}
