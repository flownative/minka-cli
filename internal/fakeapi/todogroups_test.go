package fakeapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// groupAnswerTo sends a body as the CLI would and hands back the status, the error
// code and the colour the answer names, if any.
func groupAnswerTo(t *testing.T, url, token, method, path, body string) (int, string, any) {
	t.Helper()
	request, err := http.NewRequest(method, url+"/api/v1"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	var answer map[string]any
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		t.Fatalf("%s %s %s: the answer is no JSON object: %v", method, path, body, err)
	}
	code, _ := answer["code"].(string)
	if errorBody, isError := answer["error"].(map[string]any); isError {
		code, _ = errorBody["code"].(string)
	}
	return response.StatusCode, code, answer["color"]
}

// Minka looks the list up first and then reads the body of a new group in a
// fixed order: the type of the title, the title, the type of the colour, the
// colour. So whatever is aimed at a list that does not exist is not found.
func TestANewGroupIsRefusedInTheOrderMinkaChecks(t *testing.T) {
	server, fixture := New(t)
	unknown := "/todo-lists/01990000-0000-4000-8000-000000000000/groups"
	known := "/todo-lists/" + fixture.TodoList.ID + "/groups"

	for _, c := range []struct {
		path, body string
		status     int
		code       string
	}{
		{known, `{"title": 5, "color": 5}`, http.StatusBadRequest, "malformedBody"},
		{known, `{"title": "", "color": 5}`, http.StatusUnprocessableEntity, "todoGroupTitleMissing"},
		{known, `{"title": "Light", "color": 5}`, http.StatusBadRequest, "malformedBody"},
		{known, `{"title": "Light", "color": ""}`, http.StatusUnprocessableEntity, "todoGroupColorUnknown"},
		{known, `{"title": "Light", "color": "teal"}`, http.StatusUnprocessableEntity, "todoGroupColorUnknown"},
		{known, `{"title": "Light", "color": "Red"}`, http.StatusUnprocessableEntity, "todoGroupColorUnknown"},
		{unknown, `{"title": 5, "color": 5}`, http.StatusNotFound, ""},
		{unknown, `{"title": "Light", "color": "teal"}`, http.StatusNotFound, ""},
		{unknown, `{`, http.StatusNotFound, ""},
		{unknown, `{"title": "Light", "color": "red"}`, http.StatusNotFound, ""},
	} {
		status, code, _ := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPost, c.path, c.body)
		if status != c.status || (c.code != "" && code != c.code) {
			t.Errorf("%s: %d %s, want %d %s", c.body, status, code, c.status, c.code)
		}
	}
	if len(fixture.TodoList.Groups) != 2 {
		t.Fatalf("a refused group was added: %d groups", len(fixture.TodoList.Groups))
	}
}

// A new group takes a colour or none; white and null both are none, and the
// answer always names the colour, as null where there is none.
func TestANewGroupCarriesItsColourAndWhiteIsNone(t *testing.T) {
	server, fixture := New(t)
	path := "/todo-lists/" + fixture.TodoList.ID + "/groups"

	for body, want := range map[string]any{
		`{"title": "Blue"}`:                   nil,
		`{"title": "Blue", "color": null}`:    nil,
		`{"title": "Blue", "color": "white"}`: nil,
		`{"title": "Blue", "color": "blue"}`:  "blue",
	} {
		status, _, color := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPost, path, body)
		if status != http.StatusCreated || color != want {
			t.Errorf("%s: %d with the colour %v, want 201 with %v", body, status, color, want)
		}
	}
}

// An edit looks the group up first, and checks the title before whether
// anything is to change at all; a colour of null is something to change.
// Title and colour are written together or not at all.
func TestAnEditOfAGroupIsRefusedInTheOrderMinkaChecksAndWritesAllOrNothing(t *testing.T) {
	server, fixture := New(t)
	stage := "/todo-groups/" + fixture.StageGroup.ID
	unknown := "/todo-groups/01990000-0000-4000-8000-000000000000"

	for _, c := range []struct {
		path, body string
		status     int
		code       string
	}{
		{stage, `{"title": 5}`, http.StatusBadRequest, "malformedBody"},
		{stage, `{}`, http.StatusUnprocessableEntity, "nothingToChange"},
		{stage, `{"title": null}`, http.StatusUnprocessableEntity, "nothingToChange"},
		{stage, `{"title": "", "color": "blue"}`, http.StatusUnprocessableEntity, "todoGroupTitleMissing"},
		{stage, `{"title": "Renamed", "color": "teal"}`, http.StatusUnprocessableEntity, "todoGroupColorUnknown"},
		{stage, `{"title": "Renamed", "color": 5}`, http.StatusBadRequest, "malformedBody"},
		{stage, `{"color": ""}`, http.StatusUnprocessableEntity, "todoGroupColorUnknown"},
		{unknown, `{"title": 5}`, http.StatusNotFound, ""},
		{unknown, `{}`, http.StatusNotFound, ""},
		{unknown, `{"color": ""}`, http.StatusNotFound, ""},
		{unknown, `{"color": "green"}`, http.StatusNotFound, ""},
		{unknown, `{"color": null}`, http.StatusNotFound, ""},
	} {
		status, code, _ := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPatch, c.path, c.body)
		if status != c.status || (c.code != "" && code != c.code) {
			t.Errorf("%s %s: %d %s, want %d %s", c.path, c.body, status, code, c.status, c.code)
		}
	}
	if fixture.StageGroup.Title != "Stage" || fixture.StageGroup.Color != "red" {
		t.Fatalf("a refused edit changed the group: %q in %q", fixture.StageGroup.Title, fixture.StageGroup.Color)
	}

	for _, c := range []struct {
		body         string
		title, color string
		answered     any
	}{
		{`{"title": "Renamed"}`, "Renamed", "red", "red"},
		{`{"color": "green"}`, "Renamed", "green", "green"},
		{`{"title": "Stage", "color": "white"}`, "Stage", "", nil},
		{`{"color": "blue"}`, "Stage", "blue", "blue"},
		{`{"color": null}`, "Stage", "", nil},
	} {
		status, _, color := groupAnswerTo(t, server.URL, fixture.Token, http.MethodPatch, stage, c.body)
		if status != http.StatusOK || color != c.answered || fixture.StageGroup.Title != c.title || fixture.StageGroup.Color != c.color {
			t.Errorf("%s: %d answered %v, stored %q in %q, want %q in %q", c.body, status, color, fixture.StageGroup.Title, fixture.StageGroup.Color, c.title, c.color)
		}
	}
}
