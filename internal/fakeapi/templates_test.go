package fakeapi

import (
	"net/http"
	"strings"
	"testing"
)

// Minka reads the type of the template before the type of the title, and
// both before it asks for the library; a null template is no empty list.
// What a template is not comes before the title, and a template in the archive
// is told before the title only where no title is given.
func TestAListFromATemplateIsRefusedInTheOrderMinkaChecks(t *testing.T) {
	server, fixture := New(t)
	lists := "/todos-tools/" + fixture.Todos.ID + "/lists"
	archived := `"template": "` + fixture.ArchivedTemplate.ID + `"`

	for _, c := range []struct {
		body   string
		status int
		code   string
	}{
		{`{"template": null, "title": "Empty"}`, http.StatusBadRequest, "malformedBody"},
		{`{"template": 5, "title": 5}`, http.StatusBadRequest, "malformedBody"},
		{`{"template": "` + fixture.Template.ID + `", "title": 5}`, http.StatusBadRequest, "malformedBody"},
		{`{"template": "", "title": ""}`, http.StatusUnprocessableEntity, "templateUnknown"},
		{`{"template": "` + strings.ToUpper(fixture.ArchivedTemplate.ID) + `"}`, http.StatusUnprocessableEntity, "templateArchived"},
		{`{` + archived + `, "title": ""}`, http.StatusUnprocessableEntity, "todoListTitleMissing"},
		{`{` + archived + `, "title": "Titled"}`, http.StatusUnprocessableEntity, "templateArchived"},
		{`{"template": "` + fixture.Template.ID + `", "description": 5}`, http.StatusCreated, ""},
	} {
		status, code, _ := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPost, lists, c.body)
		if status != c.status || code != c.code {
			t.Errorf("%s: %d %q, want %d %q", c.body, status, code, c.status, c.code)
		}
	}

	status, code, _ := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPost, "/todos-tools/"+fixture.Templates.ID+"/lists", `{"template": "nothing"}`)
	if status != http.StatusUnprocessableEntity || code != "templateIntoLibrary" {
		t.Errorf("into the library: %d %q, want the library before the template", status, code)
	}
}

// A day taken off a template is no day: "" and null are both allowed there,
// on a to-do and on a subtask, and only a day itself is refused.
func TestADayTakenOffInTheLibraryIsNoRefusal(t *testing.T) {
	server, fixture := New(t)
	todo := "/todos/" + fixture.TemplateTodo.ID
	subtask := todo + "/subtasks/" + fixture.TemplateTodo.Subtasks[0].ID

	for _, c := range []struct {
		path, body string
		status     int
		code       string
	}{
		{todo, `{"startsOn": "", "dueOn": ""}`, http.StatusOK, ""},
		{todo, `{"dueOn": null}`, http.StatusOK, ""},
		{todo, `{"dueOn": "2026-10-01"}`, http.StatusUnprocessableEntity, "templateTakesNoDates"},
		{subtask, `{"dueOn": ""}`, http.StatusOK, ""},
		{subtask, `{"dueOn": "2026-10-01"}`, http.StatusUnprocessableEntity, "templateTakesNoDates"},
	} {
		status, code, _ := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPatch, c.path, c.body)
		if status != c.status || code != c.code {
			t.Errorf("%s %s: %d %q, want %d %q", c.path, c.body, status, code, c.status, c.code)
		}
	}
}
