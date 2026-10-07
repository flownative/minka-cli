package fakeapi

import (
	"encoding/json"
	"net/http"
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
	fixture.Board.Coordinator = &Coordinator{PersonID: fixture.Bruno.ID, Session: "planning", Since: began, LastSeenAt: began}
	server.WaitsOut = func(time.Duration) <-chan time.Time {
		passed := make(chan time.Time, 1)
		passed <- began
		return passed
	}

	server.SetNow(began.Add(time.Hour))
	if status, _ := getNotifications(t, server, fixture.Token, "after=0&wait=0"); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !fixture.Board.Coordinator.LastSeenAt.Equal(began) {
		t.Fatal("an ask without a wait renewed the board")
	}

	if status, _ := getNotifications(t, server, fixture.Token, "after=0&wait=1"); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !fixture.Board.Coordinator.LastSeenAt.Equal(began.Add(time.Hour)) {
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
	fixture.Board.Coordinator = &Coordinator{PersonID: fixture.Bruno.ID, Session: "planning", Since: began, LastSeenAt: began}
	for i, query := range []string{"wait=5", "after=99&wait=5"} {
		server.SetNow(began.Add(time.Duration(i+1) * time.Hour))
		getNotifications(t, server, fixture.Token, query)
		if !fixture.Board.Coordinator.LastSeenAt.Equal(server.Now()) {
			t.Errorf("%s did not renew the board", query)
		}
	}
}

// In an archived project a claim is refused as a write into it, after the
// conflict with another holder; a release still goes through. A session may
// hold blanks, and no control or format character.
func TestAClaimInAnArchivedProjectAndTheNamesOfASession(t *testing.T) {
	server, fixture := New(t)
	fixture.Project.Archived = true
	status, body := claim(t, server, fixture.Token, fixture.Board.ID, "planning")
	if refusal, _ := body["error"].(map[string]any); status != http.StatusUnprocessableEntity || refusal["code"] != "projectArchived" {
		t.Fatalf("%d %v", status, body)
	}
	fixture.Board.Coordinator = &Coordinator{PersonID: fixture.Anke.ID, Session: "hers", Since: server.Now(), LastSeenAt: server.Now()}
	if status, _ := claim(t, server, fixture.Token, fixture.Board.ID, "planning"); status != http.StatusConflict {
		t.Fatalf("status %d, want the conflict first", status)
	}
	request, _ := http.NewRequest(http.MethodDelete, server.URL+"/api/v1/boards/"+fixture.Board.ID+"/coordinator", nil)
	request.Header.Set("Authorization", "Bearer "+fixture.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent || fixture.Board.Coordinator != nil {
		t.Fatalf("release in an archived project: %d", response.StatusCode)
	}

	fixture.Project.Archived = false
	if status, _ := claim(t, server, fixture.Token, fixture.Board.ID, "Planung Robert"); status != http.StatusOK {
		t.Fatalf("a session with a blank: %d", status)
	}
	if status, _ := claim(t, server, fixture.Token, fixture.Board.ID, "a‮b"); status != http.StatusUnprocessableEntity {
		t.Fatalf("a session with a format character: %d", status)
	}
}
