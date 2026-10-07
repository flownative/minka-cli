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

	status, body := getNotifications(t, server, fixture.Token, "after=ntf0&wait=0")
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
	if status, _ := getNotifications(t, server, fixture.Token, "after=ntf0&wait=0"); status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if !fixture.Board.Coordinator.LastSeenAt.Equal(began) {
		t.Fatal("an ask without a wait renewed the board")
	}

	if status, _ := getNotifications(t, server, fixture.Token, "after=ntf0&wait=1"); status != http.StatusOK {
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
		"after=ntf0&wait=301":  "notificationsWaitInvalid",
		"after=ntf0&wait=-1":   "notificationsWaitInvalid",
		"after=ntf0&limit=0":   "notificationsLimitInvalid",
		"after=ntf0&limit=201": "notificationsLimitInvalid",
		"after=nonsense":       "cursorUnknown",
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
