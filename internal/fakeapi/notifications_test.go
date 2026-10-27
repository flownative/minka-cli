package fakeapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

func getNotifications(t *testing.T, server *Server, token, query string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, server.URL+"/api/v1/me/notifications?"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(response.Body).Decode(&body)
	return response.StatusCode, body
}

// The list answers in Minka's shape, with no envelope: the lines under
// notifications and the position beside them.
func TestTheListAnswersNotificationsAndACursor(t *testing.T) {
	server, fixture := New(t)
	line := server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "card.moved", Summary: "moved"})

	status, body := getNotifications(t, server, fixture.Token, "after=0&wait=0")
	lines, _ := body["notifications"].([]any)
	if status != http.StatusOK || len(lines) != 1 || body["cursor"] != CursorOf(line) || body["data"] != nil || body["meta"] != nil {
		t.Fatalf("%d %v", status, body)
	}
}

// Only a request that waits is a sign of life of a board its person
// coordinates; one that asks with a wait of 0 is not.
func TestOnlyAWaitOfMoreThanNothingKeepsTheBoard(t *testing.T) {
	server, fixture := New(t)
	began := server.Now()
	fixture.Board.Coordinators = map[string]*Coordinator{fixture.Bruno.ID: {Session: "planning", Since: began, LastSeenAt: began}}
	server.WaitsOut = func(time.Duration) <-chan time.Time {
		passed := make(chan time.Time, 1)
		passed <- began
		return passed
	}

	server.SetNow(began.Add(time.Hour))
	if status, _ := getNotifications(t, server, fixture.Token, "after=0&wait=0"); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !fixture.Board.Coordinators[fixture.Bruno.ID].LastSeenAt.Equal(began) {
		t.Fatal("an ask without a wait renewed the board")
	}

	if status, _ := getNotifications(t, server, fixture.Token, "after=0&wait=1"); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !fixture.Board.Coordinators[fixture.Bruno.ID].LastSeenAt.Equal(began.Add(time.Hour)) {
		t.Fatal("a wait did not renew the board")
	}
}

// A wait or a limit out of bounds is refused by its own name.
func TestAWaitOrALimitOutOfBoundsIsRefused(t *testing.T) {
	server, fixture := New(t)
	for query, code := range map[string]string{
		"after=0&wait=301":  "notificationsWaitInvalid",
		"after=0&wait=-1":   "notificationsWaitInvalid",
		"after=0&limit=0":   "notificationsLimitInvalid",
		"after=0&limit=201": "notificationsLimitInvalid",
		"after=nonsense":    "cursorUnknown",
	} {
		status, body := getNotifications(t, server, fixture.Token, query)
		if refusal, _ := body["error"].(map[string]any); status != http.StatusUnprocessableEntity || refusal["code"] != code {
			t.Errorf("%s: %d %v, want %s", query, status, body, code)
		}
	}
}

func claim(t *testing.T, server *Server, token, boardID, session string) (int, map[string]any) {
	t.Helper()
	request, err := http.NewRequest(http.MethodPut, server.URL+"/api/v1/boards/"+boardID+"/coordinator", strings.NewReader(`{"session":"`+session+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(response.Body).Decode(&body)
	return response.StatusCode, body
}

// A claim answers the coordinator bare. A renewal keeps since; another session
// of the same person takes over with since anew.
func TestATakeoverStartsSinceAnewAndARenewalKeepsIt(t *testing.T) {
	server, fixture := New(t)
	began := server.Now()
	if status, body := claim(t, server, fixture.Token, fixture.Board.ID, "planning"); status != http.StatusOK || body["session"] != "planning" || body["coordinator"] != nil {
		t.Fatalf("%d %v", status, body)
	}

	server.SetNow(began.Add(time.Hour))
	_, renewed := claim(t, server, fixture.Token, fixture.Board.ID, "planning")
	if renewed["since"] != timestamp(began) || renewed["lastSeenAt"] != timestamp(began.Add(time.Hour)) {
		t.Fatalf("renewed %v", renewed)
	}

	server.SetNow(began.Add(2 * time.Hour))
	_, taken := claim(t, server, fixture.Token, fixture.Board.ID, "release")
	if taken["session"] != "release" || taken["since"] != timestamp(began.Add(2*time.Hour)) {
		t.Fatalf("taken over %v", taken)
	}
}

// A client of the project is told there is no such board, and a session the
// board cannot keep is refused by its own name.
func TestAClaimRefusesAClientAndASessionTheBoardCannotKeep(t *testing.T) {
	server, fixture := New(t)
	server.Tokens["minka_pat_client"] = fixture.Client.ID
	if status, _ := claim(t, server, "minka_pat_client", fixture.Board.ID, "planning"); status != http.StatusNotFound {
		t.Fatalf("a client got %d", status)
	}
	status, body := claim(t, server, fixture.Token, fixture.Board.ID, strings.Repeat("a", 65))
	if refusal, _ := body["error"].(map[string]any); status != http.StatusUnprocessableEntity || refusal["code"] != "coordinatorSessionInvalid" {
		t.Fatalf("%d %v", status, body)
	}
}

// A position is any whole number up to where the bell stands, written without
// a sign or a leading zero; an empty one is none. An empty wait is refused.
func TestAPositionIsAWholeNumberUpToWhereTheBellStands(t *testing.T) {
	server, fixture := New(t)
	server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "card.moved"})
	server.Notify(&Notification{PersonID: fixture.Bruno.ID, Occasion: "card.moved"})

	for _, after := range []string{"0", "1", "2"} {
		if status, body := getNotifications(t, server, fixture.Token, "after="+after); status != http.StatusOK {
			t.Errorf("%s: %d %v", after, status, body)
		}
	}
	for _, after := range []string{"abc", "-1", "01", "3"} {
		if status, _ := getNotifications(t, server, fixture.Token, "after="+after); status != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d, want 422", after, status)
		}
	}
	if status, body := getNotifications(t, server, fixture.Token, "after="); status != http.StatusOK || body["cursor"] != "2" {
		t.Errorf("an empty position: %d %v", status, body)
	}
	if status, _ := getNotifications(t, server, fixture.Token, "after=0&wait="); status != http.StatusUnprocessableEntity {
		t.Errorf("an empty wait: %d, want 422", status)
	}
}

// A wait longer than the server holds a request is held for as long as it
// holds one, and answered empty then.
func TestALongWaitIsHeldAsLongAsTheServerHoldsOne(t *testing.T) {
	server, fixture := New(t)
	var held []time.Duration
	server.WaitsOut = func(d time.Duration) <-chan time.Time {
		held = append(held, d)
		passed := make(chan time.Time, 1)
		passed <- server.now
		return passed
	}
	for _, wait := range []string{"300", "20"} {
		if status, _ := getNotifications(t, server, fixture.Token, "after=0&wait="+wait); status != http.StatusOK {
			t.Fatalf("status %d", status)
		}
	}
	if len(held) != 2 || held[0] != WaitAtMost || held[1] != 20*time.Second {
		t.Fatalf("held %v, want %s and 20s", held, WaitAtMost)
	}
}

// A wait is a sign of life before its position is read: without one, and with
// one the list refuses, as well.
func TestAWaitIsASignOfLifeWhateverItsPosition(t *testing.T) {
	server, fixture := New(t)
	began := server.Now()
	fixture.Board.Coordinators = map[string]*Coordinator{fixture.Bruno.ID: {Session: "planning", Since: began, LastSeenAt: began}}
	for i, query := range []string{"wait=5", "after=99&wait=5"} {
		server.SetNow(began.Add(time.Duration(i+1) * time.Hour))
		getNotifications(t, server, fixture.Token, query)
		if !fixture.Board.Coordinators[fixture.Bruno.ID].LastSeenAt.Equal(server.Now()) {
			t.Errorf("%s did not renew the board", query)
		}
	}
}

// In an archived project a claim is refused as a write into it; a release
// still goes through. A session may hold blanks, and no control or format
// character.
func TestAClaimInAnArchivedProjectAndTheNamesOfASession(t *testing.T) {
	server, fixture := New(t)
	fixture.Project.Archived = true
	status, body := claim(t, server, fixture.Token, fixture.Board.ID, "planning")
	if refusal, _ := body["error"].(map[string]any); status != http.StatusUnprocessableEntity || refusal["code"] != "projectArchived" {
		t.Fatalf("%d %v", status, body)
	}
	fixture.Board.Coordinators = map[string]*Coordinator{fixture.Bruno.ID: {Session: "planning", Since: server.Now(), LastSeenAt: server.Now()}}
	if status, _, _ := bell(t, server, fixture.Token, http.MethodDelete, "boards/"+fixture.Board.ID+"/coordinator"); status != http.StatusNoContent || fixture.Board.Coordinators[fixture.Bruno.ID] != nil {
		t.Fatalf("release in an archived project: %d", status)
	}

	fixture.Project.Archived = false
	if status, _ := claim(t, server, fixture.Token, fixture.Board.ID, "Planung Robert"); status != http.StatusOK {
		t.Fatalf("a session with a blank: %d", status)
	}
	if status, _ := claim(t, server, fixture.Token, fixture.Board.ID, "a‮b"); status != http.StatusUnprocessableEntity {
		t.Fatalf("a session with a format character: %d", status)
	}
}

// The safeguard: every person keeps an entry of their own on a board. A claim
// is never refused for somebody else's, a board names the reader's own entry
// alone, and a release removes nothing but the own one.
func TestEveryPersonCoordinatesABoardWithAnEntryOfTheirOwn(t *testing.T) {
	server, fixture := New(t)
	if status, body := claim(t, server, fixture.AgentToken, fixture.Board.ID, "agent"); status != http.StatusOK || body["session"] != "agent" {
		t.Fatalf("the agent: %d %v", status, body)
	}
	if status, body := claim(t, server, fixture.Token, fixture.Board.ID, "planning"); status != http.StatusOK || body["session"] != "planning" {
		t.Fatalf("Bruno beside the agent: %d %v", status, body)
	}

	ownEntry := func(token string) any {
		t.Helper()
		_, _, board := bell(t, server, token, http.MethodGet, "boards/"+fixture.Board.ID)
		if entry, ok := board["coordinator"].(map[string]any); ok {
			return entry["session"]
		}
		return board["coordinator"]
	}
	if ownEntry(fixture.AgentToken) != "agent" || ownEntry(fixture.Token) != "planning" {
		t.Fatalf("the board names %v to the agent and %v to Bruno", ownEntry(fixture.AgentToken), ownEntry(fixture.Token))
	}

	if status, _, _ := bell(t, server, fixture.Token, http.MethodDelete, "boards/"+fixture.Board.ID+"/coordinator"); status != http.StatusNoContent {
		t.Fatalf("release: %d", status)
	}
	if ownEntry(fixture.Token) != nil || ownEntry(fixture.AgentToken) != "agent" {
		t.Fatal("a release touched somebody else's entry, or left the own one")
	}
	if status, _, _ := bell(t, server, fixture.Token, http.MethodDelete, "boards/"+fixture.Board.ID+"/coordinator"); status != http.StatusNoContent {
		t.Fatalf("a release with nothing to remove: %d", status)
	}

	server.Tokens["minka_pat_client"] = fixture.Client.ID
	if _, _, board := bell(t, server, "minka_pat_client", http.MethodGet, "boards/"+fixture.Board.ID); board == nil {
		t.Fatal("a client was not shown the board")
	} else if _, named := board["coordinator"]; named {
		t.Fatal("a client was given the field coordinator")
	}
	if status, _, _ := bell(t, server, "minka_pat_client", http.MethodDelete, "boards/"+fixture.Board.ID+"/coordinator"); status != http.StatusNotFound {
		t.Fatalf("a client's release: %d", status)
	}
}

// A wait renews the person's own entries and nobody else's.
func TestAWaitRenewsOnlyTheOwnEntries(t *testing.T) {
	server, fixture := New(t)
	began := server.Now()
	fixture.Board.Coordinators = map[string]*Coordinator{
		fixture.Bruno.ID: {Session: "planning", Since: began, LastSeenAt: began},
		fixture.Anke.ID:  {Session: "hers", Since: began, LastSeenAt: began},
	}
	server.SetNow(began.Add(time.Hour))
	getNotifications(t, server, fixture.Token, "wait=5")
	if !fixture.Board.Coordinators[fixture.Bruno.ID].LastSeenAt.Equal(server.Now()) || !fixture.Board.Coordinators[fixture.Anke.ID].LastSeenAt.Equal(began) {
		t.Fatal("a wait did not renew the own entry alone")
	}
}

// The cards of a person narrow to a board and a column, each by itself, the
// ID taken in any case; an ID that matches nothing, malformed or not, leaves
// none and is no refusal.
func TestMyCardsNarrowToABoardAndAColumn(t *testing.T) {
	server, fixture := New(t)
	count := func(query string) int {
		t.Helper()
		status, raw, _ := bell(t, server, fixture.Token, http.MethodGet, "me/cards"+query)
		var cards []any
		if status != http.StatusOK || json.Unmarshal([]byte(raw), &cards) != nil {
			t.Fatalf("%s: %d %s", query, status, raw)
		}
		return len(cards)
	}
	var column *Column
	for _, c := range fixture.Board.Columns {
		for _, card := range c.Cards {
			if openCard(c, card) && !slices.Contains(card.AssigneeIDs, fixture.Bruno.ID) {
				card.AssigneeIDs = append(card.AssigneeIDs, fixture.Bruno.ID)
			}
			if openCard(c, card) && column == nil {
				column = c
			}
		}
	}
	all := count("")
	inColumn := 0
	for _, card := range column.Cards {
		if openCard(column, card) {
			inColumn++
		}
	}
	if inColumn == 0 || inColumn == all {
		t.Fatalf("the fixture gives %d of %d cards in the column; the test needs some inside and some outside", inColumn, all)
	}
	if got := count("?columnId=" + strings.ToUpper(column.ID)); got != inColumn {
		t.Errorf("columnId: %d cards, want %d", got, inColumn)
	}
	if got := count("?boardId=" + strings.ToUpper(fixture.Board.ID)); got != all {
		t.Errorf("boardId: %d cards, want all %d", got, all)
	}
	if got := count("?boardId=" + fixture.OtherBoard.ID); got != 0 {
		t.Errorf("another board: %d cards, want none", got)
	}
	if got := count("?boardId=" + fixture.Board.ID + "&columnId=" + column.ID); got != inColumn {
		t.Errorf("boardId and columnId: %d cards, want %d", got, inColumn)
	}
	if got := count("?boardId=" + fixture.OtherBoard.ID + "&columnId=" + column.ID); got != 0 {
		t.Errorf("a column of another board: %d cards, want none", got)
	}
	for _, query := range []string{"?boardId=nonsense", "?columnId=nonsense"} {
		if got := count(query); got != 0 {
			t.Errorf("%s: %d cards, want none", query, got)
		}
	}
}
