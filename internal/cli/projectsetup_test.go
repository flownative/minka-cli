package cli

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestProjectsCreateSendsWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	e.server.ProjectCategories = []string{"Customers"}

	env := runOK(t, e, "projects", "create", "--name", "Summer party")
	if body := e.lastWriteBody(); len(body) != 1 || body["name"] != "Summer party" {
		t.Fatalf("sent %v", body)
	}
	data := decode(t, env.Data)
	if data["name"] != "Summer party" || data["category"] != nil || env.Summary != "Started project Summer party" {
		t.Fatalf("data %v, summary %q", data, env.Summary)
	}
	if !strings.HasPrefix(crumbFor(t, env.Breadcrumbs, "tools"), "minka tools add "+data["id"].(string)) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	env, r := e.runJSON("For the website.\n", "projects", "create", "--name", "Website", "--description", "-", "--category", "customers")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 3 || body["description"] != "For the website.\n" || body["category"] != "customers" {
		t.Fatalf("sent %v", body)
	}
	if data := decode(t, env.Data); data["category"] != "Customers" || data["description"] != "For the website." {
		t.Fatalf("data %v", data)
	}
}

func TestProjectsListShowsTheCategoryWhereOneIsFiled(t *testing.T) {
	e := newTestEnv(t)

	r := e.run("", "projects", "list", "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "CATEGORY") {
		t.Fatalf("a column of no category:\n%s", r.stdout)
	}
	e.fixture.Project.Category = "Customers"
	r = e.run("", "projects", "list", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "CATEGORY") || !strings.Contains(r.stdout, "Customers") {
		t.Fatalf("the category is not shown:\n%s", r.stdout)
	}
}

func TestProjectsCreateNeedsAName(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "projects", "create", "--description", "Nameless")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "--name") || len(e.server.WritingRequests()) != 0 {
		t.Fatalf("error %q, %d writes", env.Error, len(e.server.WritingRequests()))
	}
	env, r = e.runJSON("", "projects", "create", "--name", " ")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "projectNameMissing" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

func TestProjectsCreateNamesTheRefusals(t *testing.T) {
	e := newTestEnv(t)
	e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID

	env, r := e.runJSON("", "projects", "create", "--name", "X", "--category", "Nowhere")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "projectCategoryUnknown" || env.Hint != setupHints["projectCategoryUnknown"] {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	e.vars["MINKA_TOKEN"] = "minka_pat_client"
	env, r = e.runJSON("", "projects", "create", "--name", "Own")
	expectCode(t, r, ExitForbidden)
	if env.Meta["apiCode"] != "organizationTeamOnly" || env.Hint != setupHints["organizationTeamOnly"] {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// A Minka without the addresses answers with a page of its web, and then no
// project, person or tool is at fault.
func TestSettingUpAProjectOnAMinkaWithoutTheAddressesSaysSo(t *testing.T) {
	e := newTestEnv(t)
	older := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("<!DOCTYPE html><html><body>Not found</body></html>"))
	}))
	t.Cleanup(older.Close)
	e.vars["MINKA_HOST"] = older.URL
	f := e.fixture

	for _, args := range [][]string{
		{"projects", "create", "--name", "X"},
		{"people", "add", f.Project.ID, f.Theo.ID},
		{"people", "remove", f.Project.ID, f.Anke.ID},
		{"tools", "list", f.Project.ID},
		{"tools", "add", f.Project.ID, "--type", "board", "--title", "X"},
		{"tools", "rename", f.Project.ID, f.Board.ID, "--title", "X"},
		{"tools", "move", f.Project.ID, f.Board.ID},
		{"tools", "trash", f.Project.ID, f.Board.ID},
	} {
		cannot := settingUp
		if args[0] == "tools" && args[1] == "list" {
			cannot = listingTools
		}
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if env.Hint != olderMinkaHint(cannot) || env.Error != older.URL+" cannot "+cannot+" yet" {
			t.Fatalf("%v: error %q, hint %q", args, env.Error, env.Hint)
		}
	}
}

func TestPeopleAddSaysWhetherSomebodyIsNew(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "people", "add", f.Project.ID, strings.ToUpper(f.Theo.ID))
	if body := e.lastWriteBody(); len(body) != 1 || body["personId"] != f.Theo.ID {
		t.Fatalf("sent %v", body)
	}
	if env.Summary != "Added Theo to the project" || decode(t, env.Data)["id"] != f.Theo.ID {
		t.Fatalf("summary %q, data %s", env.Summary, env.Data)
	}
	env = runOK(t, e, "people", "add", f.Project.ID, f.Theo.ID)
	if env.Summary != "Theo was in the project already" {
		t.Fatalf("summary %q", env.Summary)
	}
}

func TestPeopleAddNamesTheRefusals(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.Tokens["minka_pat_client"] = f.Client.ID

	env, r := e.runJSON("", "people", "add", f.Project.ID, f.RetiredAgent.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "agentRetired" || env.Hint != setupHints["agentRetired"] {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	e.vars["MINKA_TOKEN"] = "minka_pat_client"
	env, r = e.runJSON("", "people", "add", f.Project.ID, f.Theo.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != peopleAddNotFoundHint || slices.Contains(f.Project.MemberIDs, f.Theo.ID) {
		t.Fatalf("hint %q, members %v", env.Hint, f.Project.MemberIDs)
	}
}

func TestPeopleRemoveTakesTheAgentsAlong(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "people", "remove", f.Project.ID, "me")
	if slices.Contains(f.Project.MemberIDs, f.Bruno.ID) || slices.Contains(f.Project.MemberIDs, f.Agent.ID) {
		t.Fatalf("members %v", f.Project.MemberIDs)
	}
	if env.Summary != "Left project "+f.Project.ID {
		t.Fatalf("summary %q", env.Summary)
	}
}

func TestPeopleRemoveFindsThePersonAmongThePeopleOfTheProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "people", "remove", f.Project.ID, strings.ToUpper(f.Anke.Email))
	if slices.Contains(f.Project.MemberIDs, f.Anke.ID) || slices.Contains(f.Project.MemberIDs, f.RetiredAgent.ID) {
		t.Fatalf("members %v", f.Project.MemberIDs)
	}
	if env.Summary != "Took Anke out of project "+f.Project.ID {
		t.Fatalf("summary %q", env.Summary)
	}
}

func TestPeopleRemoveNamesTheRefusals(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Project.MemberIDs = []string{f.Bruno.ID, f.Agent.ID}

	env, r := e.runJSON("", "people", "remove", f.Project.ID, "me")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "lastProjectMember" || env.Hint != setupHints["lastProjectMember"] {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	env, r = e.runJSON("", "people", "remove", f.Project.ID, f.Theo.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != peopleRemoveNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
	writes := len(e.server.WritingRequests())
	_, r = e.runJSON("", "people", "remove", f.Project.ID, "Anke")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a name that is no ID was sent")
	}
}

func TestToolsListShowsEveryTypeInOrder(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "tools", "list", f.Project.ID)
	tools := listOfObjects(t, env.Data)
	if len(tools) < 5 || tools[0]["id"] != f.Board.ID || tools[0]["type"] != "board" || tools[0]["position"] != 1.0 {
		t.Fatalf("tools %v", tools)
	}
	r := e.run("", "tools", "list", f.Project.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"POSITION", "TYPE", "1  ", "board", f.Board.ID, f.Board.Title} {
		if !strings.Contains(r.stdout, want) {
			t.Fatalf("%q is not in:\n%s", want, r.stdout)
		}
	}
	env, r = e.runJSON("", "tools", "list", f.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != toolsListNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestToolsAddSendsTheTypeAsTyped(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	count := len(listOfObjects(t, runOK(t, e, "tools", "list", f.Project.ID).Data))

	env := runOK(t, e, "tools", "add", f.Project.ID, "--type", "chat", "--title", "Talk")
	if body := e.lastWriteBody(); len(body) != 2 || body["type"] != "chat" || body["title"] != "Talk" {
		t.Fatalf("sent %v", body)
	}
	if data := decode(t, env.Data); data["type"] != "chat" || data["position"] != float64(count+1) {
		t.Fatalf("data %v", data)
	}

	for _, toolType := range []string{"Chat", "forwards"} {
		env, r := e.runJSON("", "tools", "add", f.Project.ID, "--type", toolType, "--title", "Talk")
		expectCode(t, r, ExitValidation)
		if body := e.lastWriteBody(); body["type"] != toolType || env.Meta["apiCode"] != "toolTypeUnknown" || !strings.Contains(env.Error, `"board"`) {
			t.Fatalf("%s: sent %v, apiCode %v, hint %q", toolType, body, env.Meta["apiCode"], env.Hint)
		}
	}

	writes := len(e.server.WritingRequests())
	for _, args := range [][]string{{"--type", "chat"}, {"--title", "Talk"}} {
		_, r := e.runJSON("", append([]string{"tools", "add", f.Project.ID}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a tool without a type or a title was sent")
	}
}

func TestToolsRenameNeedsATitle(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "tools", "rename", f.Project.ID, strings.ToUpper(f.Board.ID), "--title", "Release")
	if f.Board.Title != "Release" || decode(t, env.Data)["title"] != "Release" {
		t.Fatalf("title %q, data %s", f.Board.Title, env.Data)
	}
	writes := len(e.server.WritingRequests())
	_, r := e.runJSON("", "tools", "rename", f.Project.ID, f.Board.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a rename without a title was sent")
	}
	env, r = e.runJSON("", "tools", "rename", f.Project.ID, f.OtherBoard.ID, "--title", "X")
	expectCode(t, r, ExitNotFound)
	if env.Hint != toolNotFoundHint {
		t.Fatalf("hint %q", env.Hint)
	}
}

func TestToolsMoveSendsOneNeighbourOrNone(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "tools", "move", f.Project.ID, f.Files.ID)
	if body := e.lastWriteBody(); len(body) != 0 {
		t.Fatalf("sent %v", body)
	}
	if data := decode(t, env.Data); data["position"] != 1.0 || data["id"] != f.Files.ID {
		t.Fatalf("data %v", data)
	}
	env = runOK(t, e, "tools", "move", f.Project.ID, f.Files.ID, "--after", f.Board.ID)
	if body := e.lastWriteBody(); len(body) != 1 || body["previousToolId"] != f.Board.ID || decode(t, env.Data)["position"] != 2.0 {
		t.Fatalf("sent %v, data %s", body, env.Data)
	}
	runOK(t, e, "tools", "move", f.Project.ID, f.Files.ID, "--before", f.Board.ID)
	if body := e.lastWriteBody(); len(body) != 1 || body["nextToolId"] != f.Board.ID {
		t.Fatalf("sent %v", body)
	}

	env, r := e.runJSON("", "tools", "move", f.Project.ID, f.Files.ID, "--after", f.OtherBoard.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "toolPlaceUnknown" || env.Hint != setupHints["toolPlaceUnknown"] {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	writes := len(e.server.WritingRequests())
	for _, args := range [][]string{
		{"--after", f.Board.ID, "--before", f.Chat.ID},
		{"--after", strings.ToUpper(f.Files.ID)},
		{"--before", "Board"},
	} {
		_, r := e.runJSON("", append([]string{"tools", "move", f.Project.ID, f.Files.ID}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a move the CLI refuses was sent")
	}
}

func TestToolsTrashPointsToTheTrash(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "tools", "trash", f.Project.ID, f.Board.ID)
	if crumbFor(t, env.Breadcrumbs, "trash") != "minka trash list "+f.Project.ID {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
	trash := listOfObjects(t, runOK(t, e, "trash", "list", f.Project.ID).Data)
	if !slices.ContainsFunc(trash, func(entry map[string]any) bool {
		return entry["kind"] == "tool" && entry["id"] == f.Board.ID && entry["toolType"] == "board"
	}) {
		t.Fatalf("trash %v", trash)
	}
	_, r := e.runJSON("", "tools", "trash", f.Project.ID, f.Board.ID)
	expectCode(t, r, ExitNotFound)
}
