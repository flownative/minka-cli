package fakeapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func answeredList(t *testing.T, answer seriesAnswer) []map[string]any {
	t.Helper()
	var list []map[string]any
	if err := json.Unmarshal([]byte(answer.raw), &list); err != nil {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	return list
}

func fieldOf(list []map[string]any, name string) []any {
	var values []any
	for _, entry := range list {
		values = append(values, entry[name])
	}
	return values
}

// The order is Minka's: the name, the description, the category, and only
// then whether the creator is on the team of the organization.
func TestAProjectIsStartedInTheOrderOfMinka(t *testing.T) {
	s, f := New(t)
	s.ProjectCategories = []string{"Beach"}
	s.Tokens["minka_pat_client"] = f.Client.ID
	before := len(s.Projects)

	for _, c := range []struct {
		name, token string
		body        any
		want        string
	}{
		{"no name", f.Token, map[string]any{}, "422 projectNameMissing"},
		{"a blank name", f.Token, map[string]any{"name": " "}, "422 projectNameMissing"},
		{"a long name", f.Token, map[string]any{"name": strings.Repeat("ä", 201)}, "422 projectNameTooLong"},
		{"a line break", f.Token, map[string]any{"name": "a\nb"}, "422 projectNameHasControlCharacters"},
		{"a name that is no text", f.Token, `{"name":3}`, "400 malformedBody"},
		{"a blank name before a description that is no text", f.Token, `{"name":" ","description":3}`, "422 projectNameMissing"},
		{"a description that is no text", f.Token, `{"name":"X","description":3}`, "400 malformedBody"},
		{"a long description before a category that is no text", f.Token, `{"name":"X","description":"` + strings.Repeat("x", 2001) + `","category":3}`, "422 projectDescriptionTooLong"},
		{"a long description", f.Token, map[string]any{"name": "X", "description": strings.Repeat("x", 2001)}, "422 projectDescriptionTooLong"},
		{"an unknown category", f.Token, map[string]any{"name": "X", "category": "Nirgends"}, "422 projectCategoryUnknown"},
		{"a client with an unknown category", "minka_pat_client", map[string]any{"name": "X", "category": "Nirgends"}, "422 projectCategoryUnknown"},
		{"a client", "minka_pat_client", map[string]any{"name": "X"}, "403 organizationTeamOnly"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if answer := callSeries(t, s, c.token, http.MethodPost, "/projects", c.body); answer.got != c.want {
				t.Fatalf("got %s, want %s: %s", answer.got, c.want, answer.raw)
			}
			if len(s.Projects) != before {
				t.Fatal("a refused project was started")
			}
		})
	}
	if got := sendAs(t, s, f.Token, http.MethodPost, "/api/v1/projects", "text/plain", `{"name":"X"}`); got != "415 unsupportedMediaType" {
		t.Fatalf("got %s", got)
	}
}

func TestAProjectIsStartedFiledUnderItsCategoryWithItsCreator(t *testing.T) {
	s, f := New(t)
	s.ProjectCategories = []string{"Beach"}

	answer := callSeries(t, s, f.Token, http.MethodPost, "/projects", map[string]any{"name": " Sommerfest ", "description": "Planung", "category": " beach "})
	if answer.got != "201" || answer.body["name"] != "Sommerfest" || answer.body["description"] != "Planung" || answer.body["status"] != "active" || answer.body["category"] != "Beach" {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	people := answeredList(t, callSeries(t, s, f.Token, http.MethodGet, "/projects/"+answer.body["id"].(string)+"/people", nil))
	if !slices.Equal(fieldOf(people, "name"), []any{"Bruno"}) {
		t.Fatalf("members %v", fieldOf(people, "name"))
	}

	answer = callSeries(t, s, f.AgentToken, http.MethodPost, "/projects", map[string]any{"name": "Recherche", "category": ""})
	if answer.got != "201" || answer.body["category"] != nil {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	people = answeredList(t, callSeries(t, s, f.Token, http.MethodGet, "/projects/"+answer.body["id"].(string)+"/people", nil))
	if !slices.Equal(fieldOf(people, "name"), []any{f.Agent.Name, "Bruno"}) {
		t.Fatalf("an agent starts a project without its human: %v", fieldOf(people, "name"))
	}
}

func TestAMemberIsAddedInTheOrderOfMinka(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	path := "/projects/" + f.Project.ID + "/people"
	agentOfTheo := &Person{ID: newID(), Name: "Robo", PrincipalID: f.Theo.ID}
	s.People[agentOfTheo.ID] = agentOfTheo
	without := &Project{ID: newID(), Name: "Without clients", MemberIDs: []string{f.Bruno.ID}, WithoutClientAccess: true}
	s.Projects = append(s.Projects, without)

	for _, c := range []struct {
		name, token, path string
		body              any
		archived          bool
		want              string
	}{
		{"no person", f.Token, path, map[string]any{}, false, "422 personIdMissing"},
		{"a person that is no text", f.Token, path, `{"personId":1}`, false, "400 malformedBody"},
		{"no identifier", f.Token, path, map[string]any{"personId": "nobody"}, false, "422 personNotAnOrganizationMember"},
		{"nobody of the organization", f.Token, path, map[string]any{"personId": newID()}, false, "422 personNotAnOrganizationMember"},
		{"an agent without its human", f.Token, path, map[string]any{"personId": agentOfTheo.ID}, false, "422 principalNotAProjectMember"},
		{"a retired agent", f.Token, path, map[string]any{"personId": f.RetiredAgent.ID}, false, "422 agentRetired"},
		{"a client into a project without clients", f.Token, "/projects/" + without.ID + "/people", map[string]any{"personId": f.ForeignClient.ID}, false, "422 projectWithoutClientAccess"},
		{"somebody into an archived project", f.Token, path, map[string]any{"personId": f.Theo.ID}, true, "422 projectArchived"},
		{"nobody of the organization into an archived project", f.Token, path, map[string]any{"personId": "nobody"}, true, "422 personNotAnOrganizationMember"},
		{"somebody in already, archived", f.Token, path, map[string]any{"personId": strings.ToUpper(f.Anke.ID)}, true, "200"},
		{"from outside", f.StrangerToken, path, map[string]any{"personId": f.Theo.ID}, false, "404"},
		{"from a client", "minka_pat_client", path, map[string]any{"personId": f.Theo.ID}, false, "404"},
		{"from a client, with no body", "minka_pat_client", path, `{`, false, "404"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f.Project.Archived = c.archived
			members := len(f.Project.MemberIDs)
			answer := callSeries(t, s, c.token, http.MethodPost, c.path, c.body)
			if answer.got != c.want {
				t.Fatalf("got %s, want %s: %s", answer.got, c.want, answer.raw)
			}
			if len(f.Project.MemberIDs) != members || len(without.MemberIDs) != 1 {
				t.Fatal("somebody was added")
			}
		})
	}
	f.Project.Archived = false

	answer := callSeries(t, s, f.Token, http.MethodPost, path, map[string]any{"personId": f.Theo.ID})
	if answer.got != "201" || answer.body["name"] != "Theo" || answer.body["kind"] != "human" || answer.body["mention"] == nil {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPost, path, map[string]any{"personId": f.Theo.ID}); answer.got != "200" || answer.body["id"] != f.Theo.ID {
		t.Fatalf("a second time: %s %s", answer.got, answer.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPost, path, map[string]any{"personId": agentOfTheo.ID}); answer.got != "201" || answer.body["kind"] != "agent" {
		t.Fatalf("an agent whose human is in: %s %s", answer.got, answer.raw)
	}
}

func TestAMemberIsTakenOutInTheOrderOfMinka(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	path := "/projects/" + f.Project.ID + "/people/"

	for _, c := range []struct {
		name, token, personID string
		archived              bool
		want                  string
	}{
		{"somebody not in it", f.Token, f.Theo.ID, false, "404"},
		{"no identifier", f.Token, "nobody", false, "404"},
		{"from outside", f.StrangerToken, f.Anke.ID, false, "404"},
		{"somebody else, from a client", "minka_pat_client", f.Anke.ID, false, "404"},
		{"somebody else in an archived project", f.Token, f.Anke.ID, true, "422 projectArchived"},
		{"somebody not in an archived project", f.Token, f.Theo.ID, true, "404"},
		{"somebody else, from a client of an archived project", "minka_pat_client", f.Anke.ID, true, "422 projectArchived"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f.Project.Archived = c.archived
			if answer := callSeries(t, s, c.token, http.MethodDelete, path+c.personID, nil); answer.got != c.want {
				t.Fatalf("got %s, want %s: %s", answer.got, c.want, answer.raw)
			}
			if len(f.Project.MemberIDs) != 5 {
				t.Fatal("somebody was taken out")
			}
		})
	}

	// One leaves an archived project, a client included; a human takes their
	// agents along.
	f.Project.Archived = true
	if answer := callSeries(t, s, "minka_pat_client", http.MethodDelete, path+f.Client.ID, nil); answer.got != "204" {
		t.Fatalf("a client leaving: %s %s", answer.got, answer.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodDelete, path+strings.ToUpper(f.Bruno.ID), nil); answer.got != "204" {
		t.Fatalf("leaving an archived project: %s %s", answer.got, answer.raw)
	}
	if slices.Contains(f.Project.MemberIDs, f.Agent.ID) || !slices.Equal(f.Project.MemberIDs, []string{f.Anke.ID, f.RetiredAgent.ID}) {
		t.Fatalf("members %v", f.Project.MemberIDs)
	}
	s.Tokens["minka_pat_anke"] = f.Anke.ID
	if answer := callSeries(t, s, "minka_pat_anke", http.MethodDelete, path+f.Anke.ID, nil); answer.got != "422 lastProjectMember" {
		t.Fatalf("the last human: %s %s", answer.got, answer.raw)
	}
}

func toolsOf(t *testing.T, s *Server, token, projectID string) []map[string]any {
	t.Helper()
	return answeredList(t, callSeries(t, s, token, http.MethodGet, "/projects/"+projectID+"/tools", nil))
}

func TestToolsOfEveryTypeAreAddedRenamedMovedAndTrashed(t *testing.T) {
	s, f := New(t)
	project := &Project{ID: newID(), Name: "Website", MemberIDs: []string{f.Bruno.ID}}
	s.Projects = append(s.Projects, project)
	forwards := &ForwardsTool{ID: newID(), ProjectID: project.ID, Title: "Weitergeleitet"}
	s.ForwardsTools = append(s.ForwardsTools, forwards)
	tools := "/projects/" + project.ID + "/tools"

	ids := map[string]string{}
	for _, toolType := range addableToolTypes {
		answer := callSeries(t, s, f.Token, http.MethodPost, tools, map[string]any{"type": toolType, "title": " Ein " + toolType + " "})
		if answer.got != "201" || answer.body["type"] != toolType || answer.body["title"] != "Ein "+toolType || answer.body["projectId"] != project.ID {
			t.Fatalf("%s: %s %s", toolType, answer.got, answer.raw)
		}
		ids[toolType] = answer.body["id"].(string)
	}
	listed := toolsOf(t, s, f.Token, project.ID)
	if !slices.Equal(fieldOf(listed, "type"), []any{"forwards", "board", "chat", "posts", "todos", "checkins", "calendar", "decisions", "files"}) {
		t.Fatalf("types %v", fieldOf(listed, "type"))
	}
	if !slices.Equal(fieldOf(listed, "position"), []any{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0}) {
		t.Fatalf("positions %v", fieldOf(listed, "position"))
	}

	for _, c := range []struct {
		name string
		body any
		want string
	}{
		{"forwards", map[string]any{"type": "forwards", "title": "Mails"}, "422 toolTypeUnknown"},
		{"an unknown type", map[string]any{"type": "wiki", "title": "Wiki"}, "422 toolTypeUnknown"},
		{"a type in capitals", map[string]any{"type": "Board", "title": "Wiki"}, "422 toolTypeUnknown"},
		{"no type", map[string]any{"title": "Wiki"}, "422 toolTypeUnknown"},
		{"no title", map[string]any{"type": "board"}, "422 toolTitleMissing"},
		{"a long title", map[string]any{"type": "board", "title": strings.Repeat("x", 201)}, "422 toolTitleTooLong"},
		{"a tab", map[string]any{"type": "board", "title": "a\tb"}, "422 toolTitleHasControlCharacters"},
		{"a type that is no text", `{"type":1,"title":"X"}`, "400 malformedBody"},
		{"an unknown type before a title that is no text", `{"type":"wiki","title":3}`, "422 toolTypeUnknown"},
		{"a title that is no text", `{"type":"board","title":3}`, "400 malformedBody"},
	} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, tools, c.body); answer.got != c.want {
			t.Fatalf("%s: got %s, want %s", c.name, answer.got, c.want)
		}
	}

	if answer := callSeries(t, s, f.Token, http.MethodPatch, tools+"/"+forwards.ID, map[string]any{"title": "Mails"}); answer.got != "200" || answer.body["title"] != "Mails" || forwards.Title != "Mails" {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, tools+"/"+ids["board"], map[string]any{}); answer.got != "422 nothingToChange" {
		t.Fatalf("%s", answer.got)
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, tools+"/"+ids["board"], map[string]any{"title": " "}); answer.got != "422 toolTitleMissing" {
		t.Fatalf("%s", answer.got)
	}

	for _, c := range []struct {
		name, toolID string
		body         any
		position     float64
	}{
		{"before the first", ids["files"], map[string]any{"nextToolId": forwards.ID}, 1},
		{"after the last, named in capitals", ids["chat"], map[string]any{"previousToolId": strings.ToUpper(ids["decisions"])}, 9},
		{"with both, the previous decides", ids["todos"], map[string]any{"previousToolId": ids["board"], "nextToolId": ids["files"]}, 4},
		{"with neither, in front", ids["checkins"], map[string]any{}, 1},
		{"empty neighbours are none", ids["calendar"], map[string]any{"previousToolId": "", "nextToolId": nil}, 1},
	} {
		answer := callSeries(t, s, f.Token, http.MethodPost, tools+"/"+c.toolID+"/moves", c.body)
		if answer.got != "200" || answer.body["position"] != c.position || answer.body["id"] != c.toolID {
			t.Fatalf("%s: %s %s", c.name, answer.got, answer.raw)
		}
	}
	for _, body := range []map[string]any{{"previousToolId": ids["chat"]}, {"nextToolId": "nobody"}, {"nextToolId": f.Board.ID}, {"previousToolId": ids["board"], "nextToolId": newID()}} {
		if answer := callSeries(t, s, f.Token, http.MethodPost, tools+"/"+ids["chat"]+"/moves", body); answer.got != "422 toolPlaceUnknown" {
			t.Fatalf("%v: %s", body, answer.got)
		}
	}
	if got := sendAs(t, s, f.Token, http.MethodPost, "/api/v1"+tools+"/"+ids["chat"]+"/moves", "text/plain", `{}`); got != "415 unsupportedMediaType" {
		t.Fatalf("a move without JSON: %s", got)
	}

	if answer := callSeries(t, s, f.Token, http.MethodDelete, tools+"/"+ids["posts"], nil); answer.got != "204" {
		t.Fatalf("%s %s", answer.got, answer.raw)
	}
	if answer := callSeries(t, s, f.Token, http.MethodDelete, tools+"/"+ids["posts"], nil); answer.got != "404" {
		t.Fatalf("a second trash: %s", answer.got)
	}
	want := []any{"calendar", "checkins", "files", "forwards", "board", "todos", "decisions", "chat"}
	if got := fieldOf(toolsOf(t, s, f.Token, project.ID), "type"); !slices.Equal(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	trash := answeredList(t, callSeries(t, s, f.Token, http.MethodGet, "/projects/"+project.ID+"/trash", nil))
	if len(trash) != 1 || trash[0]["kind"] != "tool" || trash[0]["toolType"] != "posts" || trash[0]["id"] != ids["posts"] || trash[0]["restorable"] != false || trash[0]["board"] != nil {
		t.Fatalf("trash %v", trash)
	}
	if answer := callSeries(t, s, f.Token, http.MethodGet, "/posts-tools/"+ids["posts"], nil); answer.got != "404" {
		t.Fatalf("the trashed tool is still read: %s", answer.got)
	}
}

// A tool out of use, and one hidden from a client, take no place: the
// positions count what the reader sees.
func TestThePositionsCountTheToolsTheReaderSees(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID

	team := toolsOf(t, s, f.Token, f.Project.ID)
	for i, tool := range team {
		if tool["position"] != float64(i+1) {
			t.Fatalf("positions %v", fieldOf(team, "position"))
		}
		if tool["id"] == f.TodosOutOfUse.ID {
			t.Fatal("a tool out of use is listed")
		}
	}
	client := toolsOf(t, s, "minka_pat_client", f.Project.ID)
	if len(client) == 0 || len(client) >= len(team) || slices.Contains(fieldOf(client, "id"), any(f.Board.ID)) {
		t.Fatalf("a client sees %v", fieldOf(client, "type"))
	}
	for i, tool := range client {
		if tool["position"] != float64(i+1) {
			t.Fatalf("a client's positions %v", fieldOf(client, "position"))
		}
	}
	if answer := callSeries(t, s, f.Token, http.MethodPatch, "/projects/"+f.Project.ID+"/tools/"+f.TodosOutOfUse.ID, map[string]any{"title": "X"}); answer.got != "404" {
		t.Fatalf("a tool out of use renamed: %s", answer.got)
	}
}

func TestToolsAreWrittenByTheTeamAndNotInAnArchivedProject(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	tools := "/projects/" + f.Project.ID + "/tools"
	writes := []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, tools, map[string]any{"type": "board", "title": "Neu"}},
		{http.MethodPatch, tools + "/" + f.Board.ID, map[string]any{"title": "Neu"}},
		{http.MethodPost, tools + "/" + f.Board.ID + "/moves", map[string]any{}},
		{http.MethodDelete, tools + "/" + f.Board.ID, nil},
	}
	if answer := callSeries(t, s, f.StrangerToken, http.MethodGet, tools, nil); answer.got != "404" {
		t.Fatalf("an outsider lists: %s", answer.got)
	}
	before := toolsOf(t, s, f.Token, f.Project.ID)
	for _, token := range []string{f.StrangerToken, "minka_pat_client"} {
		for _, write := range writes {
			if answer := callSeries(t, s, token, write.method, write.path, write.body); answer.got != "404" || answer.raw != NotFoundBody {
				t.Fatalf("%s %s %s: %s", token, write.method, write.path, answer.got)
			}
		}
	}
	// A client is told the 404 before anything about the body, also for a
	// tool shown to clients.
	for _, write := range []struct{ method, path string }{
		{http.MethodPost, tools},
		{http.MethodPatch, tools + "/" + f.Files.ID},
		{http.MethodPost, tools + "/" + f.Files.ID + "/moves"},
	} {
		for _, body := range []struct{ mediaType, content string }{{"text/plain", `{}`}, {"application/json", `{`}, {"application/json", `{"title":3}`}} {
			if got := sendAs(t, s, "minka_pat_client", write.method, "/api/v1"+write.path, body.mediaType, body.content); got != "404" {
				t.Fatalf("a client %s %s with %s %s: %s", write.method, write.path, body.mediaType, body.content, got)
			}
		}
	}
	if after := toolsOf(t, s, f.Token, f.Project.ID); len(after) != len(before) {
		t.Fatalf("a refused write added a tool: %v", fieldOf(after, "title"))
	}
	f.Project.Archived = true
	for _, write := range writes {
		if answer := callSeries(t, s, f.Token, write.method, write.path, write.body); answer.got != "422 projectArchived" {
			t.Fatalf("%s %s: %s", write.method, write.path, answer.got)
		}
	}
	for _, c := range []struct {
		method, path string
		body         any
		want         string
	}{
		{http.MethodPost, tools, map[string]any{"type": "wiki", "title": "X"}, "422 toolTypeUnknown"},
		{http.MethodPost, tools, map[string]any{"type": "board", "title": " "}, "422 toolTitleMissing"},
		{http.MethodPatch, tools + "/" + f.Board.ID, map[string]any{"title": " "}, "422 toolTitleMissing"},
		{http.MethodPost, tools + "/" + f.Board.ID + "/moves", map[string]any{"nextToolId": f.Board.ID}, "422 toolPlaceUnknown"},
	} {
		if answer := callSeries(t, s, f.Token, c.method, c.path, c.body); answer.got != c.want {
			t.Fatalf("the body is told before the archive: %s %s: %s", c.method, c.path, answer.got)
		}
	}
	if len(toolsOf(t, s, f.Token, f.Project.ID)) == 0 || f.Board.Trashed {
		t.Fatal("an archived project lost its tools")
	}
}

// A client is told no category, whatever the project is filed under.
func TestTheCategoryIsNamedToTheTeam(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.Project.Category = "Beach"

	for token, want := range map[string]any{f.Token: "Beach", "minka_pat_client": nil} {
		list := answeredList(t, callSeries(t, s, token, http.MethodGet, "/projects", nil))
		index := slices.IndexFunc(list, func(p map[string]any) bool { return p["id"] == f.Project.ID })
		if index < 0 || list[index]["category"] != want {
			t.Fatalf("%s: %v", token, list)
		}
	}
}
