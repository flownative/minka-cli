package fakeapi

import (
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func bell(t *testing.T, server *Server, token, method, path string) (int, string, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+"/api/v1/"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	return response.StatusCode, string(raw), body
}

func occasionsOf(body map[string]any) []string {
	var occasions []string
	lines, _ := body["notifications"].([]any)
	for _, line := range lines {
		occasions = append(occasions, line.(map[string]any)["occasion"].(string))
	}
	return occasions
}

// A row of an occasion Minka does not know makes no line, and still counts
// toward the limit: the page comes back without it, and the cursor moves past
// it, so the next page goes on behind it.
func TestARowThatMakesNoLineUsesUpThePageAndMovesTheCursorOn(t *testing.T) {
	server, fixture := New(t)
	retired := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "retired.kind"})
	server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "comment.written", Summary: "Anke wrote"})

	_, first := getNotifications(t, server, fixture.Token, "after=0&limit=1")
	if len(occasionsOf(first)) != 0 || first["cursor"] != CursorOf(retired) {
		t.Fatalf("first page %v, want no line and the cursor %s", first, CursorOf(retired))
	}
	_, rest := getNotifications(t, server, fixture.Token, "after="+first["cursor"].(string)+"&limit=1")
	if occasions := occasionsOf(rest); len(occasions) != 1 || occasions[0] != "comment.written" {
		t.Fatalf("next page %v", rest)
	}
}

// A held wait that only rows without a line arrived in answers empty when its
// time is up, its cursor behind them.
func TestAWaitThatRanOutReadsPastRowsThatMakeNoLine(t *testing.T) {
	server, fixture := New(t)
	ranOut := make(chan time.Time, 1)
	server.WaitsOut = func(time.Duration) <-chan time.Time { return ranOut }
	server.WhileWaiting = func(*Person) {
		server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "retired.kind"})
		ranOut <- server.Now()
	}

	_, body := getNotifications(t, server, fixture.Token, "after=0&wait=5")
	if len(occasionsOf(body)) != 0 || body["cursor"] != "1" {
		t.Fatalf("answer %v", body)
	}
}

// Rows the person may no longer see are left out before the limit counts and
// do not move the cursor: no member of the project any more, a project in the
// archive, a card in the trash or in the archive of its board.
func TestRowsThePersonMayNoLongerSeeDoNotUseUpAPage(t *testing.T) {
	server, fixture := New(t)
	trashed, archived := &Card{ID: newID(), Title: "Thrown out"}, &Card{ID: newID(), Title: "Put away"}
	fixture.Doing.Cards = append(fixture.Doing.Cards, trashed, archived)
	for _, hidden := range []*Notification{
		{PersonID: fixture.Bruno.ID, Occasion: "comment.written", ProjectID: fixture.OtherProject.ID},
		{PersonID: fixture.Bruno.ID, Occasion: "card.assigned", CardID: trashed.ID},
		{PersonID: fixture.Bruno.ID, Occasion: "card.assigned", CardID: archived.ID},
	} {
		server.Notify(hidden)
	}
	fixture.Doing.Cards = slices.DeleteFunc(fixture.Doing.Cards, func(c *Card) bool { return c == trashed })
	server.Trash = append(server.Trash, &TrashedCard{Card: trashed, Column: fixture.Doing})
	server.PutInTheArchive(archived)
	shown := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "card.assigned", CardID: fixture.CardInTriage.ID})

	_, page := getNotifications(t, server, fixture.Token, "after=0&limit=1")
	if occasions := occasionsOf(page); len(occasions) != 1 || page["cursor"] != CursorOf(shown) {
		t.Fatalf("page %v, want the one line that is shown", page)
	}

	fixture.Project.Archived = true
	_, page = getNotifications(t, server, fixture.Token, "after=0")
	if len(occasionsOf(page)) != 0 || page["cursor"] != "0" {
		t.Fatalf("a line of an archived project: %v", page)
	}
}

// A sign of life renews only a board the person still sees as one of the team:
// one of a project they are a member of, archived or not, as somebody who is
// no client.
func TestASignOfLifeRenewsOnlyABoardThePersonStillSeesOnTheTeam(t *testing.T) {
	server, fixture := New(t)
	began := server.Now()
	server.WaitsOut = func(time.Duration) <-chan time.Time {
		passed := make(chan time.Time, 1)
		passed <- began
		return passed
	}
	members := slices.Clone(fixture.Project.MemberIDs)
	renews := func(change, undo func()) bool {
		t.Helper()
		fixture.Board.Coordinator = &Coordinator{PersonID: fixture.Bruno.ID, Session: "planning", Since: began, LastSeenAt: began}
		change()
		defer undo()
		server.SetNow(began.Add(time.Hour))
		defer server.SetNow(began)
		getNotifications(t, server, fixture.Token, "after=0&wait=1")
		return fixture.Board.Coordinator.LastSeenAt.After(began)
	}

	if !renews(func() { fixture.Project.Archived = true }, func() { fixture.Project.Archived = false }) {
		t.Error("a board of an archived project was not renewed")
	}
	if renews(func() {
		fixture.Project.MemberIDs = slices.DeleteFunc(slices.Clone(members), func(id string) bool { return id == fixture.Bruno.ID })
	}, func() { fixture.Project.MemberIDs = members }) {
		t.Error("a board of a project the person left was renewed")
	}
	if renews(func() { fixture.Bruno.Client = true }, func() { fixture.Bruno.Client = false }) {
		t.Error("a board was renewed for a client")
	}
}

// A move names the column it went into as that column is called now, from
// the trash as well; once the column is gone for good, the line says the
// bell's own sentence about the card.
func TestAMoveNamesItsColumnUntilTheColumnIsGone(t *testing.T) {
	server, fixture := New(t)
	server.Notify(&Notification{
		PersonID: fixture.Bruno.ID, Occasion: "card.moved", ActorID: fixture.Anke.ID, CardID: fixture.CardInTriage.ID,
		ProjectID: fixture.Project.ID, MovedToColumnID: fixture.Doing.ID, Summary: "as it was written",
	})
	summary := func() string {
		t.Helper()
		_, page := getNotifications(t, server, fixture.Token, "after=0")
		lines, _ := page["notifications"].([]any)
		if len(lines) != 1 {
			t.Fatalf("page %v", page)
		}
		return lines[0].(map[string]any)["summary"].(string)
	}

	renamed := "In progress"
	fixture.Doing.CustomTitle = &renamed
	if got, want := summary(), "Anke moved “"+fixture.CardInTriage.Title+"” to “In progress”"; got != want {
		t.Fatalf("summary %q, want %q", got, want)
	}

	fixture.Board.Columns = slices.DeleteFunc(fixture.Board.Columns, func(c *Column) bool { return c == fixture.Doing })
	server.TrashedColumns = append(server.TrashedColumns, &TrashedColumn{Column: fixture.Doing, Board: fixture.Board})
	if got := summary(); !strings.Contains(got, "to “In progress”") {
		t.Fatalf("a column in the trash is no longer named: %q", got)
	}

	server.TrashedColumns = nil
	if got, want := summary(), "Moved to another column: "+fixture.CardInTriage.Title; got != want {
		t.Fatalf("summary %q, want %q", got, want)
	}
}

// One line reads and settles only for its person: somebody else's, a malformed
// and an unknown ID, one that makes no line and a reaction all answer the same
// 404, and so does a line of an archived project. Marking read twice settles
// it once, and the count is the bell's, with what makes no line and reactions
// in it.
func TestOneLineReadsAndSettlesOnlyForItsPerson(t *testing.T) {
	server, fixture := New(t)
	line := server.Notify(&Notification{
		PersonID: fixture.Bruno.ID, Occasion: "person.mentioned", ActorID: fixture.Anke.ID, CardID: fixture.CardInTriage.ID,
		ProjectID: fixture.Project.ID, Summary: "Anke mentioned you",
	})
	retired := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "retired.kind"})
	reaction := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: reactionKind})
	agents := server.Notify(&Notification{PersonID: fixture.Agent.ID, Occasion: "comment.written"})
	// Unread, and about a project Bruno is no member of: the bell does not count it.
	server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "comment.written", ProjectID: fixture.OtherProject.ID})

	status, _, shown := bell(t, server, fixture.Token, http.MethodGet, "notifications/"+strings.ToUpper(line.ID))
	if status != http.StatusOK || shown["id"] != line.ID || shown["title"] != "Anke mentioned you" || shown["body"] != fixture.Project.Name ||
		shown["read"] != false || shown["threadId"] != fixture.CardInTriage.ID || shown["timeSensitive"] != true ||
		shown["unreadCount"] != float64(3) || !strings.HasSuffix(shown["appUrl"].(string), "/cards/"+fixture.CardInTriage.ID) {
		t.Fatalf("%d %v", status, shown)
	}

	for _, path := range []string{"notifications/" + agents.ID, "notifications/no-identifier", "notifications/" + newID(),
		"notifications/" + retired.ID, "notifications/" + reaction.ID} {
		for _, address := range []string{path, path + "/read"} {
			method := http.MethodGet
			if strings.HasSuffix(address, "/read") {
				method = http.MethodPost
			}
			if status, raw, _ := bell(t, server, fixture.Token, method, address); status != http.StatusNotFound || raw != NotFoundBody {
				t.Errorf("%s %s: %d %s", method, address, status, raw)
			}
		}
	}
	if agents.Read || retired.Read || reaction.Read {
		t.Fatal("a line that was not found was marked read")
	}

	for range 2 {
		status, _, count := bell(t, server, fixture.Token, http.MethodPost, "notifications/"+line.ID+"/read")
		if status != http.StatusOK || len(count) != 1 || count["unreadCount"] != float64(2) {
			t.Fatalf("mark read: %d %v", status, count)
		}
	}
	if _, _, count := bell(t, server, fixture.Token, http.MethodGet, "me/notifications/count"); count["unreadCount"] != float64(2) {
		t.Fatalf("count %v", count)
	}
	if _, _, again := bell(t, server, fixture.Token, http.MethodGet, "notifications/"+line.ID); again["read"] != true {
		t.Fatalf("a line marked read reads %v", again)
	}

	fixture.Project.Archived = true
	if status, raw, _ := bell(t, server, fixture.Token, http.MethodGet, "notifications/"+line.ID); status != http.StatusNotFound || raw != NotFoundBody {
		t.Fatalf("a line of an archived project: %d %s", status, raw)
	}
}

func bellWith(t *testing.T, server *Server, token, method, path, body string) int {
	t.Helper()
	request, err := http.NewRequest(method, server.URL+"/api/v1/"+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	return response.StatusCode
}

// A card moved into a column that is put into the trash afterwards stays on
// that column, so its line is still listed, names the column, and is shown
// and marked read: the bell does not ask whether a column is in the trash.
func TestALineAboutACardInATrashedColumnStaysAndNamesTheColumn(t *testing.T) {
	server, fixture := New(t)
	server.Tokens["minka_pat_anke"] = fixture.Anke.ID
	fixture.CardInTriage.AssigneeIDs = []string{fixture.Bruno.ID}
	if status := bellWith(t, server, "minka_pat_anke", http.MethodPost, "cards/"+fixture.CardInTriage.ID+"/moves", `{"columnId":"`+fixture.Doing.ID+`"}`); status != http.StatusOK {
		t.Fatalf("move: %d", status)
	}
	if status := bellWith(t, server, "minka_pat_anke", http.MethodDelete, "columns/"+fixture.Doing.ID, ""); status != http.StatusNoContent {
		t.Fatalf("delete column: %d", status)
	}

	_, page := getNotifications(t, server, fixture.Token, "after=0")
	lines, _ := page["notifications"].([]any)
	if len(lines) != 1 {
		t.Fatalf("page %v", page)
	}
	line := lines[0].(map[string]any)
	card, _ := line["card"].(map[string]any)
	column, _ := card["column"].(map[string]any)
	if line["summary"] != "Anke moved “"+fixture.CardInTriage.Title+"” to “Doing”" || column["id"] != fixture.Doing.ID || column["title"] != "Doing" {
		t.Fatalf("line %v", line)
	}
	id := line["id"].(string)
	if status, _, _ := bell(t, server, fixture.Token, http.MethodGet, "notifications/"+id); status != http.StatusOK {
		t.Fatalf("show: %d", status)
	}
	if status, _, _ := bell(t, server, fixture.Token, http.MethodPost, "notifications/"+id+"/read"); status != http.StatusOK {
		t.Fatalf("mark read: %d", status)
	}
}

// A reaction is left out before the limit counts, as a line the person may
// not see is: it neither uses up a page nor moves the cursor.
func TestAReactionNeitherUsesUpAPageNorMovesTheCursor(t *testing.T) {
	server, fixture := New(t)
	server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: reactionKind})
	if _, page := getNotifications(t, server, fixture.Token, "after=0&limit=1"); len(occasionsOf(page)) != 0 || page["cursor"] != "0" {
		t.Fatalf("page %v", page)
	}
	comment := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "comment.written"})
	if _, page := getNotifications(t, server, fixture.Token, "after=0&limit=1"); len(occasionsOf(page)) != 1 || page["cursor"] != CursorOf(comment) {
		t.Fatalf("page %v", page)
	}
}

// What a push of a line says: a comment arrives quietly, a completion is the
// bell's own sentence about the card as it is called now, and a line that
// names no project is about the project of its card. A move that names no
// column is summed up in the list with the same sentence.
func TestALineSaysWhatItsPushSaid(t *testing.T) {
	server, fixture := New(t)
	quiet := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "comment.written", CardID: fixture.CardInTriage.ID, Summary: "Anke wrote"})
	completed := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "card.completed", CardID: fixture.CardInTriage.ID, Summary: "Completed: as it was"})
	moved := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "card.moved", CardID: fixture.CardInTriage.ID, Summary: "as it was written"})
	fixture.CardInTriage.Title = "Renamed"

	_, page := getNotifications(t, server, fixture.Token, "after="+CursorOf(completed))
	if lines, _ := page["notifications"].([]any); len(lines) != 1 || lines[0].(map[string]any)["id"] != moved.ID ||
		lines[0].(map[string]any)["summary"] != "Moved to another column: Renamed" {
		t.Fatalf("a move without its column: %v", page)
	}

	_, _, shown := bell(t, server, fixture.Token, http.MethodGet, "notifications/"+quiet.ID)
	if shown["timeSensitive"] != false || shown["body"] != fixture.Project.Name {
		t.Fatalf("a comment: %v", shown)
	}
	if _, _, shown = bell(t, server, fixture.Token, http.MethodGet, "notifications/"+completed.ID); shown["title"] != "Completed: Renamed" {
		t.Fatalf("a completion: %v", shown)
	}
}
