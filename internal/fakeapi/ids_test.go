package fakeapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Minka reads an identifier in lowercase only, and an identifier in capitals
// names nothing wherever it stands: in the path, in the query, in a field and in
// a list of people. How that is answered differs from address to address, and
// the fake answers the same, so that a request carrying capitals is noticed.
func TestAnIdentifierInCapitalsNamesNothing(t *testing.T) {
	up := strings.ToUpper
	type probe struct{ name, method, path, body, want string }
	s, f := New(t)
	card := f.CardInTriage
	card.Steps = append(card.Steps, &Step{ID: newID(), Title: "Check"})
	subtask := f.GroupedTodo.Subtasks[0]
	probes := []probe{
		{"path card", "GET", "/api/v1/cards/" + up(card.ID), "", "404"},
		{"doc notify everybody", "POST", "/api/v1/files-tools/" + f.Files.ID + "/documents", `{"title":"t","body":"<p>b</p>","audience":"everybody","notifyIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"doc notify chosen", "POST", "/api/v1/files-tools/" + f.Files.ID + "/documents", `{"title":"t","body":"<p>b</p>","audience":"chosen","notifyIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"doc folder", "POST", "/api/v1/files-tools/" + f.Files.ID + "/documents", `{"title":"t","body":"<p>b</p>","audience":"nobody","folderId":"` + up(f.Folder.ID) + `"}`, "422 folderUnknown"},
		{"post notify everybody", "POST", "/api/v1/posts-tools/" + f.Posts.ID + "/posts", `{"title":"t","body":"<p>b</p>","audience":"everybody","notifyIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"post category", "POST", "/api/v1/posts-tools/" + f.Posts.ID + "/posts", `{"title":"t","body":"<p>b</p>","audience":"nobody","categoryId":"` + up(f.Announcement.ID) + `"}`, "422 postCategoryUnknown"},
		{"post edit category", "PATCH", "/api/v1/posts/" + f.Post.ID, `{"categoryId":"` + up(f.Announcement.ID) + `"}`, "422 postCategoryUnknown"},
		{"card assignees", "POST", "/api/v1/boards/" + f.Board.ID + "/cards", `{"title":"t","assigneeIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"card column", "POST", "/api/v1/boards/" + f.Board.ID + "/cards", `{"title":"t","columnId":"` + up(f.Doing.ID) + `"}`, "404"},
		{"move column", "POST", "/api/v1/cards/" + card.ID + "/moves", `{"columnId":"` + up(f.Doing.ID) + `"}`, "404"},
		{"move prev", "POST", "/api/v1/cards/" + card.ID + "/moves", `{"columnId":"` + f.Doing.ID + `","previousCardId":"` + up(f.CardInDoing.ID) + `"}`, "404"},
		{"assign", "POST", "/api/v1/cards/" + card.ID + "/assignees", `{"personId":"` + up(f.Anke.ID) + `"}`, "422 personNotAProjectMember"},
		{"unassign", "DELETE", "/api/v1/cards/" + card.ID + "/assignees/" + up(f.Anke.ID), "", "404"},
		{"subscribe", "POST", "/api/v1/cards/" + card.ID + "/subscribers/" + up(f.Anke.ID), "", "422 personNotAProjectMember"},
		{"unsubscribe", "DELETE", "/api/v1/cards/" + card.ID + "/subscribers/" + up(f.Anke.ID), "", "422 personNotAProjectMember"},
		{"step patch", "PATCH", "/api/v1/cards/" + card.ID + "/steps/" + up(card.Steps[0].ID), `{"completed":true}`, "404"},
		{"step delete", "DELETE", "/api/v1/cards/" + card.ID + "/steps/" + up(card.Steps[0].ID), "", "204"},
		// The one list Minka writes in lowercase itself before it reads it.
		{"work", "PUT", "/api/v1/me/work", `{"clientId":"c","cardIds":["` + up(card.ID) + `"]}`, "204"},
		{"todo assignees", "POST", "/api/v1/todo-lists/" + f.TodoList.ID + "/todos", `{"title":"t","assigneeIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"todo notified", "POST", "/api/v1/todo-lists/" + f.TodoList.ID + "/todos", `{"title":"t","notifiedIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"todo add group", "POST", "/api/v1/todo-lists/" + f.TodoList.ID + "/todos", `{"title":"t","groupId":"` + up(f.StageGroup.ID) + `"}`, "422 todoPlaceUnknown"},
		{"todo move list", "POST", "/api/v1/todos/" + f.LooseTodo.ID + "/moves", `{"listId":"` + up(f.TodoList.ID) + `","position":"top"}`, "422 todoPlaceUnknown"},
		{"todo move prev", "POST", "/api/v1/todos/" + f.LooseTodo.ID + "/moves", `{"previousTodoId":"` + up(f.GroupedTodo.ID) + `","groupId":"` + f.StageGroup.ID + `"}`, "422 todoPlaceUnknown"},
		{"group move", "POST", "/api/v1/todo-groups/" + f.StageGroup.ID + "/moves", `{"previousGroupId":"` + up(f.KitchenGroup.ID) + `"}`, "422 todoPlaceUnknown"},
		{"subtask assignee", "POST", "/api/v1/todos/" + f.GroupedTodo.ID + "/subtasks", `{"title":"t","assigneeId":"` + up(f.Anke.ID) + `"}`, "422 personNotAProjectMember"},
		{"subtask patch", "PATCH", "/api/v1/todos/" + f.GroupedTodo.ID + "/subtasks/" + up(subtask.ID), `{"title":"x"}`, "404"},
		{"subtask move", "POST", "/api/v1/todos/" + f.GroupedTodo.ID + "/subtasks/" + up(subtask.ID) + "/moves", `{"position":"top"}`, "404"},
		{"subtask move prev", "POST", "/api/v1/todos/" + f.GroupedTodo.ID + "/subtasks/" + subtask.ID + "/moves", `{"previousSubtaskId":"` + up(f.GroupedTodo.Subtasks[1].ID) + `"}`, "422 todoPlaceUnknown"},
		{"subtask delete", "DELETE", "/api/v1/todos/" + f.GroupedTodo.ID + "/subtasks/" + up(subtask.ID), "", "204"},
		{"question asked", "POST", "/api/v1/checkins/" + f.CheckinsTool.ID + "/questions", `{"title":"t","schedule":{"rhythm":"daily","time":"09:00","weekdays":[1]},"askedIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"question edit asked", "PATCH", "/api/v1/questions/" + f.DailyQuestion.ID, `{"askedIds":["` + up(f.Anke.ID) + `"]}`, "422 personNotAProjectMember"},
		{"chat client id", "POST", "/api/v1/chats/" + f.Chat.ID + "/messages", `{"body":"<p>hi</p>","clientMessageId":"` + up(newID()) + `"}`, "422 chatMessageDraftNotAnIdentifier"},
		{"conversation start", "POST", "/api/v1/messages/with/" + up(f.Anke.ID), "", "404"},
		{"archive before", "GET", "/api/v1/boards/" + f.Board.ID + "/archive?before=" + up(card.ID), "", "422 archivedCardsBeforeUnknown"},
		{"my todos after", "GET", "/api/v1/me/todos?after=" + up(f.LooseTodo.ID), "", "422 todosAfterUnknown"},
		{"answers person", "GET", "/api/v1/questions/" + f.DailyQuestion.ID + "/answers?personId=" + up(f.Anke.ID), "", "404"},
		{"answers before", "GET", "/api/v1/questions/" + f.DailyQuestion.ID + "/answers?before=" + up(f.BrunosAnswer.ID), "", "422 answersBeforeUnknown"},
		{"entries folder", "GET", "/api/v1/files-tools/" + f.Files.ID + "/entries?folderId=" + up(f.Folder.ID), "", "404"},
	}
	for _, p := range probes {
		req, _ := http.NewRequest(p.method, s.URL+p.path, strings.NewReader(p.body))
		req.Header.Set("Authorization", "Bearer "+f.Token)
		if p.name == "work" {
			req.Header.Set("Authorization", "Bearer "+f.AgentToken)
		}
		if p.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var e struct{ Error struct{ Code string } }
		_ = json.Unmarshal(raw, &e)
		got := fmt.Sprint(resp.StatusCode)
		if e.Error.Code != "" && resp.StatusCode != 404 {
			got += " " + e.Error.Code
		}
		if got != p.want {
			t.Errorf("%s: %s, want %s", p.name, got, p.want)
		}
	}
}
