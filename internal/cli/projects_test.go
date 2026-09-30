package cli

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAProjectEditSendsOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "projects", "edit", f.Project.ID, "--name", "Summer party")
	if body := e.lastWriteBody(); len(body) != 1 || body["name"] != "Summer party" {
		t.Fatalf("sent %v", body)
	}
	if f.Project.Description != "For trying things" {
		t.Fatalf("the description changed with the name: %q", f.Project.Description)
	}
	data := decode(t, env.Data)
	if data["name"] != "Summer party" || data["status"] != "active" || data["id"] != f.Project.ID {
		t.Fatalf("data %v", data)
	}
	if !strings.Contains(env.Summary, "Summer party") {
		t.Fatalf("summary %q", env.Summary)
	}
	if crumbFor(t, env.Breadcrumbs, "boards") != "minka boards list "+f.Project.ID || crumbFor(t, env.Breadcrumbs, "projects") != "minka projects list" {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	env, r := e.runJSON("Everything for the party.\nAnd the day after.\n", "projects", "edit", f.Project.ID, "--description", "-")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["description"] != "Everything for the party.\nAnd the day after.\n" {
		t.Fatalf("sent %v", body)
	}
	if decode(t, env.Data)["description"] != "Everything for the party.\nAnd the day after." || f.Project.Name != "Summer party" {
		t.Fatalf("data %s, name %q", env.Data, f.Project.Name)
	}

	runOK(t, e, "projects", "edit", f.Project.ID, "--description", "")
	if body := e.lastWriteBody(); len(body) != 1 || body["description"] != "" {
		t.Fatalf("sent %v", body)
	}
	if f.Project.Description != "" {
		t.Fatalf("an empty description did not empty it: %q", f.Project.Description)
	}
}

func TestAProjectDescriptionIsSentAsPlainText(t *testing.T) {
	e := newTestEnv(t)

	runOK(t, e, "projects", "edit", e.fixture.Project.ID, "--description", "The **apps** for <iPhone>")
	if body := e.lastWriteBody(); body["description"] != "The **apps** for <iPhone>" {
		t.Fatalf("sent %v", body)
	}
}

func TestAProjectEditWithoutAChangeIsNotSent(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "projects", "edit", e.fixture.Project.ID)
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage || !strings.Contains(env.Error, "--name") {
		t.Fatalf("code %q, error %q", env.Code, env.Error)
	}
	if len(e.server.WritingRequests()) != 0 {
		t.Fatal("an edit without a change was sent")
	}
}

func TestAProjectKeepsAName(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "projects", "edit", e.fixture.Project.ID, "--name", " ")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "projectNameMissing" || e.fixture.Project.Name != "Test-Projekt" {
		t.Fatalf("apiCode %v, name %q", env.Meta["apiCode"], e.fixture.Project.Name)
	}
}

func TestOnlyTheTeamOfAProjectFindsItToEdit(t *testing.T) {
	for name, token := range map[string]string{"a client": "minka_pat_client", "somebody outside": "minka_pat_stranger"} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
			e.vars["MINKA_TOKEN"] = token

			env, r := e.runJSON("", "projects", "edit", e.fixture.Project.ID, "--name", "Taken over")
			expectCode(t, r, ExitNotFound)
			if env.Hint != projectNotFoundHint {
				t.Fatalf("hint %q", env.Hint)
			}
			if e.fixture.Project.Name != "Test-Projekt" {
				t.Fatalf("renamed to %q", e.fixture.Project.Name)
			}
		})
	}
}

func TestAProjectEditPrintsTheDescriptionAsText(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	r := e.run("", "projects", "edit", f.Project.ID, "--description", "Everything for the party.", "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"Changed project Test-Projekt.", "/projects/" + f.Project.ID, "Everything for the party."} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("%q is not in:\n%s", want, r.stdout)
		}
	}
}

// A Minka without the address answers with a page of its web, not with the
// JSON of its API, and the project is then not the one at fault.
func TestAProjectEditToAMinkaWithoutTheAddressSaysSo(t *testing.T) {
	e := newTestEnv(t)
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Not found</body></html>"))
	}))
	t.Cleanup(older.Close)
	e.vars["MINKA_HOST"] = older.URL

	env, r := e.runJSON("", "projects", "edit", e.fixture.Project.ID, "--name", "Renamed")
	expectCode(t, r, ExitNotFound)
	if env.Hint != projectEditUnknownHint || !strings.Contains(env.Error, "cannot rename") {
		t.Fatalf("error %q, hint %q", env.Error, env.Hint)
	}
}
