package fakeapi

import (
	"net/http"
	"strings"
	"testing"
)

// The order is the order of Minka's checks: the body, then whether the project
// is archived for a member, then the team.
func TestAProjectEditRefusesInTheOrderOfMinka(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	path := "/projects/" + f.Project.ID
	tooLong := map[string]any{"description": strings.Repeat("x", 2001)}

	for _, c := range []struct {
		name, token string
		body        any
		archived    bool
		want        string
	}{
		{"nothing named", f.Token, map[string]any{}, false, "422 nothingToChange"},
		{"null names nothing", f.Token, `{"name":null,"description":null}`, false, "422 nothingToChange"},
		{"a name that is no text", f.Token, `{"name":7}`, false, "400 malformedBody"},
		{"an empty name", f.Token, map[string]any{"name": " "}, false, "422 projectNameMissing"},
		{"a name of two lines", f.Token, map[string]any{"name": "A\nB"}, false, "422 projectNameHasControlCharacters"},
		{"a name too long", f.Token, map[string]any{"name": strings.Repeat("ä", 201)}, false, "422 projectNameTooLong"},
		{"a description too long, from outside", f.StrangerToken, tooLong, false, "422 projectDescriptionTooLong"},
		{"a description too long, from a client", "minka_pat_client", tooLong, false, "422 projectDescriptionTooLong"},
		{"a client of an archived project", "minka_pat_client", map[string]any{"name": "X"}, true, "422 projectArchived"},
		{"somebody outside an archived project", f.StrangerToken, map[string]any{"name": "X"}, true, "404"},
		{"a client", "minka_pat_client", map[string]any{"name": "X"}, false, "404"},
		{"somebody outside", f.StrangerToken, map[string]any{"name": "X"}, false, "404"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f.Project.Archived = c.archived
			answer := callSeries(t, s, c.token, http.MethodPatch, path, c.body)
			if answer.got != c.want {
				t.Fatalf("got %s, want %s: %s", answer.got, c.want, answer.raw)
			}
			if answer.got == "404" && answer.raw != NotFoundBody {
				t.Fatalf("a 404 that tells itself apart: %s", answer.raw)
			}
			if f.Project.Name != "Test-Projekt" || f.Project.Description != "For trying things" {
				t.Fatalf("written: %q, %q", f.Project.Name, f.Project.Description)
			}
		})
	}
}

func TestAProjectEditWritesWhatItNamesTrimmed(t *testing.T) {
	s, f := New(t)
	path := "/projects/" + f.Project.ID

	answer := callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"name": "  Summer party ", "description": nil})
	if answer.got != "200" || answer.body["name"] != "Summer party" || answer.body["description"] != "For trying things" {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	answer = callSeries(t, s, f.Token, http.MethodPatch, path, map[string]any{"description": "  "})
	if answer.got != "200" || answer.body["description"] != "" || answer.body["name"] != "Summer party" || answer.body["status"] != "active" {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
}
