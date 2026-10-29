package cli

import (
	"strings"
	"testing"
)

// The next card is the one Minka names, passed on as Minka sent it; the CLI
// reads nothing else for it and writes nothing.
func TestCardsNextShowsTheCardMinkaNames(t *testing.T) {
	e := newAgentEnv(t)
	ready(e)
	upper, lower := e.fixture.Doing.Cards[0], e.fixture.Doing.Cards[1]
	upper.AssigneeIDs = []string{e.fixture.Agent.ID}
	lower.AssigneeIDs = []string{e.fixture.Agent.ID}

	env, r := e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["id"] != lower.ID || env.Notice != "" || env.Summary != "Next for you: "+lower.Title+" in Doing" {
		t.Errorf("next %v, summary %q, notice %q", data["id"], env.Summary, env.Notice)
	}
	if _, comments := data["comments"]; !comments {
		t.Errorf("the card is not passed on as Minka sent it: %s", env.Data)
	}
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Cmd != "minka cards show "+lower.ID {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}
	var paths []string
	for _, request := range e.server.Requests() {
		paths = append(paths, request.Method+" "+request.Path)
	}
	if want := "GET /api/v1/boards/" + e.fixture.Board.ID + "/next-card"; len(paths) != 1 || paths[0] != want {
		t.Errorf("requests %v, want only %s", paths, want)
	}
}

// Nothing released is no failure: exit 0 with data null, and a notice only
// where the board has no column with the role ready at all.
func TestCardsNextWithNothingReleasedExitsZero(t *testing.T) {
	e := newAgentEnv(t)

	env, r := e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || !strings.Contains(env.Notice, "no column with the role ready") || !strings.Contains(env.Notice, "columns edit --role ready") {
		t.Errorf("no ready column: %s, notice %q", env.Data, env.Notice)
	}

	ready(e)
	e.fixture.Doing.Cards[0].AssigneeIDs = []string{e.fixture.Bruno.ID}
	env, r = e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if string(env.Data) != "null" || env.Notice != "" || !strings.HasPrefix(env.Summary, "Nothing on board "+e.fixture.Board.ID+" is released for you") {
		t.Errorf("nothing released: %s, summary %q, notice %q", env.Data, env.Summary, env.Notice)
	}
	if len(env.Breadcrumbs) != 1 || env.Breadcrumbs[0].Action != "wait" {
		t.Errorf("breadcrumbs %v", env.Breadcrumbs)
	}
}

// A board that is not there, or not the caller's to see, is Minka's own 404
// and reads as one, not as a Minka too old for the command.
func TestCardsNextOnABoardNotYoursIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	ready(e)
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken

	env, r := e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Error+env.Hint, "older") || !strings.Contains(env.Hint, "not a member of") {
		t.Errorf("error %q, hint %q", env.Error, env.Hint)
	}
}

// The safeguard: a Minka without the address is said to be too old, and the
// CLI does not work the order out in its place.
func TestCardsNextAgainstAMinkaTooOldSaysSo(t *testing.T) {
	for name, older := range map[string]func(e *testEnv){
		"without the address": func(e *testEnv) { e.server.KnowsNoNextCard = true },
		"without roles":       func(e *testEnv) { e.server.IgnoresColumnRoles = true },
	} {
		t.Run(name, func(t *testing.T) {
			e := newAgentEnv(t)
			ready(e)
			e.fixture.Doing.Cards[0].AssigneeIDs = []string{e.fixture.Agent.ID}
			older(e)

			env, r := e.runJSON("", "cards", "next", "--board", e.fixture.Board.ID)
			expectCode(t, r, ExitNotFound)
			if !strings.Contains(env.Error, "does not name the next card") || !strings.Contains(env.Hint, "older than this minka expects") || string(env.Data) != "" {
				t.Errorf("error %q, hint %q, data %s", env.Error, env.Hint, env.Data)
			}
			if requests := e.server.Requests(); len(requests) != 1 {
				t.Errorf("%d requests went out, the next card was looked for elsewhere", len(requests))
			}
		})
	}
}
