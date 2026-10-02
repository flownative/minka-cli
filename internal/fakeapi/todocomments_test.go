package fakeapi

import (
	"net/http"
	"testing"
)

// An archived list takes no word under itself nor under its to-dos, and says
// so for somebody else's comment as well; reading stays open.
func TestAnArchivedListTakesNoCommentsAndSaysSo(t *testing.T) {
	s, f := New(t)
	list, todo := f.ArchivedList, f.ArchivedListTodo
	list.Comments = []*Comment{{ID: newID(), AuthorID: f.Bruno.ID, Body: "<p>As it was</p>", CreatedAt: s.now,
		Reactions: []*Reaction{{Content: "👍", PersonIDs: []string{f.Bruno.ID}}}}}
	todo.Comments = []*Comment{{ID: newID(), AuthorID: f.Anke.ID, Body: "<p>As it was</p>", CreatedAt: s.now,
		Reactions: []*Reaction{{Content: "👍", PersonIDs: []string{f.Anke.ID}}}}}

	for item, comments := range map[string][]*Comment{
		"/api/v1/todo-lists/" + list.ID: list.Comments,
		"/api/v1/todos/" + todo.ID:      todo.Comments,
	} {
		comment := item + "/comments/" + comments[0].ID
		for _, request := range []struct{ method, path, body string }{
			{http.MethodPost, item + "/comments", `{"body":"More"}`},
			{http.MethodPatch, comment, `{"body":"Not after all"}`},
			{http.MethodDelete, comment, ``},
			{http.MethodPost, comment + "/reactions", `{"content":"🎉"}`},
			{http.MethodDelete, comment + "/reactions?content=%F0%9F%91%8D", ``},
		} {
			if got, _ := answerTo(t, s, f.Token, request.method, request.path, request.body); got != "422 todoListArchived" {
				t.Errorf("%s %s: %s", request.method, request.path, got)
			}
		}
		if got, _ := answerTo(t, s, f.Token, http.MethodGet, item+"/comments", ``); got != "200" {
			t.Errorf("%s: reading the comments: %s", item, got)
		}
	}
	if len(list.Comments) != 1 || len(todo.Comments) != 1 || list.Comments[0].Body != "<p>As it was</p>" ||
		len(todo.Comments[0].Reactions) != 1 || todo.Comments[0].EditedAt != nil {
		t.Error("a comment under the archived list changed")
	}
}

// Under a to-do and a list a request is answered in the order of the
// controller: a body that is no JSON before the item, the item and the comment
// under it before what the body says, and the archive of the project before
// the author.
func TestACommentUnderAToDoIsRefusedInTheOrderMinkaAsks(t *testing.T) {
	s, f := New(t)
	todo := f.LooseTodo
	others := &Comment{ID: newID(), AuthorID: f.Anke.ID, Body: "<p>Mine</p>", CreatedAt: s.now}
	todo.Comments = []*Comment{others}
	under := "/api/v1/todos/" + todo.ID + "/comments"
	ofTheList := "/api/v1/todo-lists/" + f.TodoList.ID + "/comments/" + f.TodoList.Comments[0].ID

	for _, c := range []struct{ name, method, path, body, want string }{
		{"a comment that is no JSON on a to-do never written", http.MethodPost, "/api/v1/todos/" + newID() + "/comments", `{`, "400 malformedBody"},
		{"an empty comment on a to-do never written", http.MethodPost, "/api/v1/todos/" + newID() + "/comments", `{"body":""}`, "404"},
		{"an empty comment", http.MethodPost, under, `{"body":""}`, "422 commentMissing"},
		{"a change of a comment under the list", http.MethodPatch, under + "/" + f.TodoList.Comments[0].ID, `{"body":"Mine now"}`, "404"},
		{"an empty change of somebody else's comment", http.MethodPatch, under + "/" + others.ID, `{"body":""}`, "422 commentMissing"},
		{"a change of somebody else's comment", http.MethodPatch, under + "/" + others.ID, `{"body":"Mine now"}`, "404"},
		{"a reaction on a comment under the list", http.MethodPost, under + "/" + f.TodoList.Comments[0].ID + "/reactions", `{"content":"👍"}`, "404"},
		{"an empty reaction", http.MethodPost, under + "/" + others.ID + "/reactions", `{"content":""}`, "422 reactionMissing"},
		{"a reaction on somebody else's comment", http.MethodPost, ofTheList + "/reactions", `{"content":"👍"}`, "201"},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	f.Project.Archived = true
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, under+"/"+others.ID, `{"body":"Mine now"}`); got != "422 projectArchived" {
		t.Errorf("a change of somebody else's comment in an archived project: %s", got)
	}
	if others.Body != "<p>Mine</p>" {
		t.Errorf("the comment changed: %q", others.Body)
	}
}

// Somebody outside the project reaches no comment of a to-do or a list, with
// the 404 of every address that leads nowhere.
func TestSomebodyOutsideTheProjectReachesNoCommentOfAToDo(t *testing.T) {
	s, f := New(t)
	todo := f.LooseTodo
	todo.Comments = []*Comment{{ID: newID(), AuthorID: f.Bruno.ID, Body: "<p>Mine</p>", CreatedAt: s.now}}

	for _, comment := range []string{
		"/api/v1/todos/" + todo.ID + "/comments/" + todo.Comments[0].ID,
		"/api/v1/todo-lists/" + f.TodoList.ID + "/comments/" + f.TodoList.Comments[0].ID,
	} {
		for _, request := range []struct{ method, path, body string }{
			{http.MethodPatch, comment, `{"body":"Mine now"}`},
			{http.MethodDelete, comment, ``},
			{http.MethodPost, comment + "/reactions", `{"content":"👍"}`},
		} {
			got, answer := answerTo(t, s, f.StrangerToken, request.method, request.path, request.body)
			if got != "404" || answer["error"].(map[string]any)["code"] != "notFound" {
				t.Errorf("%s %s: %s", request.method, request.path, got)
			}
		}
	}
}
