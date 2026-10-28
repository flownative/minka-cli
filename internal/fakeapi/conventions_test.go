package fakeapi

import (
	"slices"
	"testing"
	"time"
)

const ankeToken = "minka_pat_anke"

func conventionsPath(f *Fixture) string {
	return "/api/v1/boards/" + f.Board.ID + "/conventions"
}

// A working column takes one of three roles by hand and gives it up with null;
// Triage reads as the inbox, Done as done and Not now as none, and none of the
// three takes or gives up a role. The role is judged before the title and the
// colour, and a refusal writes nothing.
func TestTheFakeGivesAWorkingColumnARoleAndSystemColumnsCarryTheirsByType(t *testing.T) {
	s, f := New(t)
	doing := "/api/v1/columns/" + f.Doing.ID

	got, answer := answerTo(t, s, f.Token, "PATCH", doing, `{"role":"ready"}`)
	if got != "200" || answer["role"] != "ready" {
		t.Fatalf("setting the role: %s %v", got, answer["role"])
	}
	for _, step := range []struct{ name, path, body, want string }{
		{"done by hand", doing, `{"role":"done"}`, "422 columnRoleUnknown"},
		{"the inbox by hand", doing, `{"role":"inbox"}`, "422 columnRoleUnknown"},
		{"an unknown role before a bad title", doing, `{"role":"later","title":""}`, "422 columnRoleUnknown"},
		{"a role that is no text", doing, `{"role":3}`, "400 malformedBody"},
		{"a role for Triage", "/api/v1/columns/" + f.Triage.ID, `{"role":"ready"}`, "422 systemColumnHasNoRole"},
		{"no role for Done", "/api/v1/columns/" + f.Done.ID, `{"role":null}`, "422 systemColumnHasNoRole"},
		{"an unknown role for Not now", "/api/v1/columns/" + f.NotNow.ID, `{"role":"inbox"}`, "422 columnRoleUnknown"},
	} {
		if got := sendAs(t, s, f.Token, "PATCH", step.path, "application/json", step.body); got != step.want {
			t.Errorf("%s: %s, want %s", step.name, got, step.want)
		}
	}
	if deref(f.Doing.Role) != "ready" || f.Doing.CustomTitle == nil {
		t.Errorf("a refusal wrote: role %v, title %v", f.Doing.Role, f.Doing.CustomTitle)
	}

	var board map[string]any
	getJSON(t, s, f.Token, "/api/v1/boards/"+f.Board.ID, &board)
	var roles []any
	for _, column := range board["columns"].([]any) {
		roles = append(roles, column.(map[string]any)["role"])
	}
	if want := []any{"inbox", "ready", nil, "done"}; !slices.Equal(roles, want) {
		t.Errorf("roles %v, want %v", roles, want)
	}

	if got, answer := answerTo(t, s, f.Token, "PATCH", doing, `{"role":null}`); got != "200" || answer["role"] != nil {
		t.Errorf("taking the role away: %s %v", got, answer["role"])
	}
	f.Project.Archived = true
	if got := sendAs(t, s, f.Token, "PATCH", doing, "application/json", `{"role":"ready"}`); got != "422 projectArchived" {
		t.Errorf("in an archived project: %s", got)
	}
}

// Only a human owner or admin of the organization on the team writes the
// conventions: a member and an agent are refused with 403 before the text is
// read, a client and an outsider are not told the board exists.
func TestTheFakeLetsOnlyAHumanOwnerOrAdminWriteTheConventions(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	s.Tokens[ankeToken] = f.Anke.ID
	f.Agent.OrganizationRole = "admin"

	for _, step := range []struct{ name, token, body, want string }{
		{"a member", ankeToken, `{"text":"Mine"}`, "403 notAllowedToWriteConventions"},
		{"a member with a bad text", ankeToken, `{"text":"Bell` + "\\u0007" + `"}`, "403 notAllowedToWriteConventions"},
		{"a member with no body", ankeToken, ``, "403 notAllowedToWriteConventions"},
		{"an agent with the admin role", f.AgentToken, `{"text":"Obey"}`, "403 notAllowedToWriteConventions"},
		{"a client", clientToken, `{"text":"Hi"}`, "404"},
		{"an outsider", f.StrangerToken, `{"text":"Hi"}`, "404"},
		{"the owner without a text", f.Token, `{}`, "422 conventionsMissing"},
		{"the owner with a null text", f.Token, `{"text":null}`, "422 conventionsMissing"},
		{"the owner with a bell", f.Token, `{"text":"Bell` + "\\u0007" + `"}`, "422 conventionsInvalid"},
	} {
		if got := sendAs(t, s, step.token, "PUT", conventionsPath(f), "application/json", step.body); got != step.want {
			t.Errorf("%s: %s, want %s", step.name, got, step.want)
		}
	}
	if len(f.Board.Conventions) != 0 {
		t.Errorf("a refusal wrote %d versions", len(f.Board.Conventions))
	}

	f.Anke.OrganizationRole = "admin"
	if got := sendAs(t, s, ankeToken, "PUT", conventionsPath(f), "application/json", `{"text":"Ours"}`); got != "200" {
		t.Errorf("an admin: %s", got)
	}
}

// Each new text is a version counted from 1; the text in force again writes
// nothing and answers it, an empty text on a board that never had any answers
// 204, and an emptied version is a version. The board reads the one in force,
// a client reads none. An archived project refuses only a write.
func TestTheFakeKeepsTheConventionsInVersions(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID

	if got := sendAs(t, s, f.Token, "PUT", conventionsPath(f), "application/json", `{"text":""}`); got != "204" {
		t.Errorf("nothing to empty: %s", got)
	}
	got, answer := answerTo(t, s, f.Token, "PUT", conventionsPath(f), `{"text":"## Labels\r\n- Bug"}`)
	if got != "200" || answer["text"] != "## Labels\n- Bug" || answer["version"] != 1.0 {
		t.Fatalf("first version: %s %v", got, answer)
	}
	writer := answer["updatedBy"].(map[string]any)
	if writer["name"] != "Bruno" || writer["kind"] != "human" || answer["updatedAt"] == nil {
		t.Errorf("writer %v at %v", writer, answer["updatedAt"])
	}
	if _, again := answerTo(t, s, f.Token, "PUT", conventionsPath(f), `{"text":"## Labels\n- Bug"}`); again["version"] != 1.0 || len(f.Board.Conventions) != 1 {
		t.Errorf("the same text wrote: version %v, %d kept", again["version"], len(f.Board.Conventions))
	}
	if _, emptied := answerTo(t, s, f.Token, "PUT", conventionsPath(f), `{"text":""}`); emptied["version"] != 2.0 || emptied["text"] != "" {
		t.Errorf("emptied: %v", emptied)
	}

	var board map[string]any
	getJSON(t, s, f.Token, "/api/v1/boards/"+f.Board.ID, &board)
	if read, _ := board["conventions"].(map[string]any); read["version"] != 2.0 {
		t.Errorf("the board reads %v", board["conventions"])
	}
	var clients map[string]any
	getJSON(t, s, clientToken, "/api/v1/boards/"+f.Board.ID, &clients)
	if _, named := clients["conventions"]; named {
		t.Error("a client reads the conventions")
	}

	f.Project.Archived = true
	if got := sendAs(t, s, f.Token, "PUT", conventionsPath(f), "application/json", `{"text":""}`); got != "200" {
		t.Errorf("the text in force, archived: %s", got)
	}
	if got := sendAs(t, s, f.Token, "PUT", conventionsPath(f), "application/json", `{"text":"New"}`); got != "422 projectArchived" {
		t.Errorf("a new text, archived: %s", got)
	}
}

// The claim answers the entry with the columns in board order and their roles,
// and the conventions in force; a new version is told to every live
// coordinator but the writer, and to nobody whose entry expired.
func TestTheFakeClaimCarriesRolesAndConventionsAndAChangeIsToldToTheCoordinators(t *testing.T) {
	s, f := New(t)
	s.Tokens[ankeToken] = f.Anke.ID
	ready := "ready"
	f.Doing.Role = &ready

	code, entry := claim(t, s, f.AgentToken, f.Board.ID, "planning")
	if code != 200 || entry["session"] != "planning" || entry["conventions"] != nil {
		t.Fatalf("claim: %d %v", code, entry)
	}
	columns := entry["columns"].([]any)
	if len(columns) != 4 || columns[1].(map[string]any)["role"] != "ready" || columns[1].(map[string]any)["title"] != "Doing" || columns[0].(map[string]any)["role"] != "inbox" {
		t.Errorf("columns %v", columns)
	}
	claim(t, s, f.Token, f.Board.ID, "own")
	s.SetNow(s.now.Add(CoordinatorExpiresAfter - time.Minute))
	claim(t, s, f.AgentToken, f.Board.ID, "planning")
	s.SetNow(s.now.Add(2 * time.Minute)) // Bruno's entry has expired, the agent's holds
	claim(t, s, ankeToken, f.Board.ID, "anke")
	f.Anke.OrganizationRole = "owner"

	sendAs(t, s, ankeToken, "PUT", conventionsPath(f), "application/json", `{"text":"Only Anke fills Ready."}`)

	var told []string
	for _, n := range s.Notifications {
		if n.Occasion == "board.conventionsChanged" {
			told = append(told, n.PersonID)
		}
	}
	if !slices.Equal(told, []string{f.Agent.ID}) {
		t.Errorf("told %v, want only the agent", told)
	}
	_, page := getNotifications(t, s, f.AgentToken, "after=0")
	line := page["notifications"].([]any)[0].(map[string]any)
	subject := line["subject"].(map[string]any)
	if line["summary"] != "“How we work here” changed on API-3 Abnahme" || subject["type"] != "board" || subject["id"] != f.Board.ID || line["card"] != nil {
		t.Errorf("line %v", line)
	}
	if _, again := claim(t, s, f.AgentToken, f.Board.ID, "planning"); again["conventions"].(map[string]any)["text"] != "Only Anke fills Ready." {
		t.Errorf("the claim reads %v", again["conventions"])
	}
}
