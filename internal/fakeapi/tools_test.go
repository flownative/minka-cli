package fakeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// A tool read by its identifier alone names the project it stands in, and is
// at no address for whoever may not open it: somebody outside the project, a
// client the tool is not shown to, an identifier of another kind of tool or
// one that is none.
func TestAToolNamesItsProjectToWhoeverMayOpenIt(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.Posts.ShownToClients = false
	internal := &FilesTool{ID: newID(), ProjectID: f.Project.ID, Title: "Internal"}
	s.FilesTools = append(s.FilesTools, internal)

	for _, c := range []struct {
		name, token, path, want string
	}{
		{"a Docs & Files tool", f.Token, "/api/v1/files-tools/" + f.Files.ID, "200"},
		{"a Posts tool", f.Token, "/api/v1/posts-tools/" + f.Posts.ID, "200"},
		{"a Posts tool no longer in use", f.Token, "/api/v1/posts-tools/" + f.PostsOutOfUse.ID, "200"},
		{"a To-dos tool", f.Token, "/api/v1/todos-tools/" + f.Todos.ID, "200"},
		{"a Check-ins tool", f.Token, "/api/v1/checkins/" + f.CheckinsTool.ID, "200"},
		{"a chat", f.Token, "/api/v1/chats/" + f.Chat.ID, "200"},
		{"a tool shown to clients, read by one", "minka_pat_client", "/api/v1/files-tools/" + f.Files.ID, "200"},
		{"a tool of a project of somebody else", f.Token, "/api/v1/files-tools/" + f.OtherFiles.ID, "404"},
		{"a Posts tool of somebody else", f.Token, "/api/v1/posts-tools/" + f.OtherPosts.ID, "404"},
		{"a To-dos tool of somebody else", f.Token, "/api/v1/todos-tools/" + f.OtherTodos.ID, "404"},
		{"a Check-ins tool of somebody else", f.Token, "/api/v1/checkins/" + f.OtherCheckinsTool.ID, "404"},
		{"a chat of somebody else", f.Token, "/api/v1/chats/" + f.OtherChat.ID, "404"},
		{"a private conversation", f.Token, "/api/v1/chats/" + f.Conversation.ID, "404"},
		{"a Posts tool hidden from clients, read by one", "minka_pat_client", "/api/v1/posts-tools/" + f.Posts.ID, "404"},
		{"a Docs & Files tool hidden from clients, read by one", "minka_pat_client", "/api/v1/files-tools/" + internal.ID, "404"},
		{"what stands in it, read by one", "minka_pat_client", "/api/v1/files-tools/" + internal.ID + "/entries", "404"},
		{"a tool read by somebody outside the project", f.StrangerToken, "/api/v1/files-tools/" + f.Files.ID, "404"},
		{"a Posts tool at the address of Docs & Files", f.Token, "/api/v1/files-tools/" + f.Posts.ID, "404"},
		{"a Docs & Files tool at the address of To-dos", f.Token, "/api/v1/todos-tools/" + f.Files.ID, "404"},
		{"an identifier that is none", f.Token, "/api/v1/files-tools/files", "404"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, answer := answerTo(t, s, c.token, http.MethodGet, c.path, "")
			if got != c.want {
				t.Fatalf("answered %s, want %s", got, c.want)
			}
			if got != "200" {
				return
			}
			project, _ := answer["project"].(map[string]any)
			if answer["projectId"] != f.Project.ID || project["id"] != f.Project.ID || project["name"] != f.Project.Name {
				t.Errorf("projectId %v, project %v, want %s", answer["projectId"], answer["project"], f.Project.ID)
			}
		})
	}

	// The tools of a project leave out what a client is not shown.
	for token, want := range map[string]int{f.Token: 2, "minka_pat_client": 1} {
		if tools := readJSON(t, s, token, "/api/v1/projects/"+f.Project.ID+"/files").([]any); len(tools) != want {
			t.Errorf("%s is listed %d Docs & Files tools, want %d", token, len(tools), want)
		}
		if tools := readJSON(t, s, token, "/api/v1/projects/"+f.Project.ID+"/posts").([]any); len(tools) != want-1 {
			t.Errorf("%s is listed %d Posts tools, want %d", token, len(tools), want-1)
		}
	}
}

// Every line of the lists a tool or a board hands over names its project, so
// that a line read alone says where it stands.
func TestEveryLineOfAListNamesItsProject(t *testing.T) {
	s, f := New(t)
	s.PutInTheArchive(f.CardInDoing)
	for _, c := range []struct {
		path string
		of   func(answer any) []any
	}{
		{"/api/v1/files-tools/" + f.Files.ID + "/entries", func(answer any) []any {
			entries := answer.(map[string]any)
			return append(append(entries["folders"].([]any), entries["documents"].([]any)...), entries["uploads"].([]any)...)
		}},
		{"/api/v1/posts-tools/" + f.Posts.ID + "/posts", func(answer any) []any { return answer.(map[string]any)["posts"].([]any) }},
		{"/api/v1/posts-tools/" + f.Posts.ID + "/archive", func(answer any) []any { return answer.([]any) }},
		{"/api/v1/todos-tools/" + f.Todos.ID + "/lists", func(answer any) []any { return answer.([]any) }},
		{"/api/v1/todos-tools/" + f.Todos.ID + "/archive", func(answer any) []any { return answer.([]any) }},
		{"/api/v1/boards/" + f.Board.ID + "/archive", func(answer any) []any { return answer.([]any) }},
	} {
		t.Run(c.path, func(t *testing.T) {
			lines := c.of(readJSON(t, s, f.Token, c.path))
			if len(lines) == 0 {
				t.Fatal("the fixture lists nothing here, so nothing is checked")
			}
			for _, line := range lines {
				if got := line.(map[string]any)["projectId"]; got != f.Project.ID {
					t.Errorf("projectId %v, want %s: %v", got, f.Project.ID, line)
				}
			}
		})
	}
}

// readJSON reads what one address answers, whatever its shape.
func readJSON(t *testing.T, s *Server, token, path string) any {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, s.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	raw, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("answered %d: %s", response.StatusCode, raw)
	}
	var answer any
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatal(err)
	}
	return answer
}
