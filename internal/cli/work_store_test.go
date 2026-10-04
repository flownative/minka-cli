package cli

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"testing"
	"time"
)

// storedHosts is the store as it lies on disk: what a command wrote, not what a
// command shows of it.
func (e *testEnv) storedHosts() map[string]map[string]any {
	e.t.Helper()
	content, err := os.ReadFile(e.storePath())
	if err != nil {
		e.t.Fatal(err)
	}
	var file struct {
		Hosts map[string]map[string]any `json:"hosts"`
	}
	if err := json.Unmarshal(content, &file); err != nil {
		e.t.Fatalf("the store is not JSON: %v: %s", err, content)
	}
	return file.Hosts
}

// storedList is what the store holds for the host of these tests and the
// session the environment names, the list of no session where it names none.
func (e *testEnv) storedList() map[string]any {
	e.t.Helper()
	entry, known := e.storedHosts()[e.vars["MINKA_HOST"]]
	if !known {
		e.t.Fatalf("the store holds nothing for %s: %v", e.vars["MINKA_HOST"], e.storedHosts())
	}
	sessions, _ := entry["sessions"].(map[string]any)
	list, known := sessions[e.vars[sessionVariable]].(map[string]any)
	if !known {
		e.t.Fatalf("the store holds nothing for the session %q: %v", e.vars[sessionVariable], entry)
	}
	return list
}

// asStored is a moment the way the store writes it down.
func asStored(at time.Time) string {
	return at.UTC().Format(time.RFC3339)
}

// The host of the store is read the way a sign-in writes it, however it stands
// in the file: a default host written by hand with a slash at the end would
// otherwise have one command at another list than the next.
func TestTheHostOfTheStoreIsReadAsASignInWritesIt(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	host := e.server.URL
	e.writeConfig(`{"defaultHost": "` + host + `/", "hosts": {
		"` + host + `": {"token": "` + e.fixture.AgentToken + `"}}}`)
	delete(e.vars, "MINKA_HOST")
	delete(e.vars, "MINKA_TOKEN")

	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	env, r := e.runWithJSON(began, "heartbeat")
	expectCode(t, r, ExitOK)
	if sent := decode(t, env.Data)["sent"]; sent != true {
		t.Fatalf("the beat sent nothing: %s", env.Data)
	}
	if keys := slices.Sorted(maps.Keys(e.storedHosts())); len(keys) != 1 || keys[0] != host {
		t.Errorf("the store is keyed by %v, want the one host %s", keys, host)
	}
}

// A beat of a session and a command somebody typed say that the work is going
// on. Looking at the list says nothing: looking is not working, and a command
// that only reads does not become a writer for this.
func TestWhatSaysThatSomebodyIsStillAtTheStore(t *testing.T) {
	e := newAgentEnv(t)
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	e.vars[sessionVariable] = "a-session-at-work"

	expectCode(t, e.runAt(began, "work", "start", e.fixture.CardInDoing.ID, "--json"), ExitOK)
	if got := e.storedList()["beatAt"]; got != asStored(began) {
		t.Errorf("work start left the mark at %v, want %s", got, asStored(began))
	}

	looked := began.Add(5 * time.Minute)
	expectCode(t, e.runAt(looked, "work", "list", "--json"), ExitOK)
	if got := e.storedList()["beatAt"]; got != asStored(began) {
		t.Errorf("work list moved the mark to %v", got)
	}

	idle := began.Add(7 * time.Minute)
	expectCode(t, e.runAt(idle, "heartbeat", "--idle", "--json"), ExitOK)
	if got := e.storedList()["beatAt"]; got != asStored(began) {
		t.Errorf("an idle beat moved the mark to %v", got)
	}

	beat := began.Add(10 * time.Minute)
	expectCode(t, e.runAt(beat, "heartbeat", "--json"), ExitOK)
	if got := e.storedList()["beatAt"]; got != asStored(beat) {
		t.Errorf("the beat of a session left the mark at %v, want %s", got, asStored(beat))
	}

	stopped := began.Add(20 * time.Minute)
	expectCode(t, e.runAt(stopped, "work", "stop", "--all", "--json"), ExitOK)
	if got := e.storedList()["beatAt"]; got != asStored(stopped) {
		t.Errorf("work stop left the mark at %v, want %s", got, asStored(stopped))
	}
}
