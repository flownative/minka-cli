package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A card in the archive of its board is read like any other, and says where it
// is: in no column, and in the archive it was put into.
func TestACardInTheArchiveClaimsNoColumnItStandsIn(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing
	e.server.PutInTheArchive(card)

	env, r := e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)
	if shown := decode(t, env.Data); shown["archived"] != true {
		t.Errorf("the card does not say it is archived: %s", env.Data)
	}
	if want := "Card Review the API in the archive of " + e.fixture.Board.Title; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}

	r = e.run("", "cards", "show", card.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "Column:") {
		t.Errorf("an archived card names a column it stands in:\n%s", r.stdout)
	}
	if want := "Archived: yes, was in Doing (column, " + e.fixture.Doing.ID + ")"; !strings.Contains(r.stdout, want) {
		t.Errorf("missing %q in:\n%s", want, r.stdout)
	}

	// The board it was taken off lists it no longer.
	r = e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, card.Title) {
		t.Errorf("the board still lists the archived card:\n%s", r.stdout)
	}

	// A card on its board says nothing of an archive and names its column.
	r = e.run("", "cards", "show", e.fixture.CardInTriage.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "Archived") || !strings.Contains(r.stdout, "Column: Triage (triage, ") {
		t.Errorf("a card on its board:\n%s", r.stdout)
	}
}

// The cards somebody is responsible for are the ones still on a board: a card
// put into the archive drops out of the list, which is why the list saying
// nothing about a card does not mean the card is done.
func TestACardInTheArchiveIsNotAmongTheCardsOfSomebody(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing
	card.AssigneeIDs = []string{e.fixture.Bruno.ID}

	env, r := e.runJSON("", "cards", "mine")
	expectCode(t, r, ExitOK)
	if !strings.Contains(string(env.Data), card.ID) {
		t.Fatalf("the card is not in the list to begin with: %s", env.Data)
	}

	e.server.PutInTheArchive(card)
	env, r = e.runJSON("", "cards", "mine")
	expectCode(t, r, ExitOK)
	if strings.Contains(string(env.Data), card.ID) {
		t.Errorf("the archived card is still listed: %s", env.Data)
	}
}

// A card that was in the archive before it went into the trash comes back onto
// its board, not into the archive.
func TestACardFromTheArchiveComesBackOntoItsBoard(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing
	e.server.PutInTheArchive(card)

	_, r := e.runJSON("", "cards", "delete", card.ID)
	expectCode(t, r, ExitOK)
	env, r := e.runJSON("", "cards", "restore", card.ID)
	expectCode(t, r, ExitOK)
	restored := decode(t, env.Data)
	if restored["archived"] != false {
		t.Errorf("the restored card is still in the archive: %s", env.Data)
	}
	if column, _ := restored["column"].(map[string]any); column["title"] != "Doing" {
		t.Errorf("restored card %s", env.Data)
	}
	if !strings.Contains(env.Summary, "Restored card Review the API to Doing") {
		t.Errorf("summary %q", env.Summary)
	}

	r = e.run("", "cards", "show", card.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, "Archived") || !strings.Contains(r.stdout, "Column: Doing (column, ") {
		t.Errorf("the card did not come back onto the board:\n%s", r.stdout)
	}
}

// Every card of the API carries the field, so the resource passed through says
// of every card whether it lies in an archive.
func TestEveryCardSaysWhetherItIsArchived(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "cards", "show", e.fixture.CardInTriage.ID)
	expectCode(t, r, ExitOK)
	if shown := decode(t, env.Data); shown["archived"] != false {
		t.Errorf("a card on its board: %s", env.Data)
	}

	e.fixture.CardInDoing.AssigneeIDs = []string{e.fixture.Bruno.ID}
	env, r = e.runJSON("", "cards", "mine")
	expectCode(t, r, ExitOK)
	var mine []map[string]any
	if err := json.Unmarshal(env.Data, &mine); err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 {
		t.Fatalf("cards %s", env.Data)
	}
	if _, says := mine[0]["archived"]; !says {
		t.Errorf("a card of the list says nothing about the archive: %s", env.Data)
	}
}

// A card put away stays in the list of the machine until the board leaves it out
// of the set: one mechanism takes a card out of that list, and the command is not
// it. Putting a card away that nobody says they are working on therefore says
// nothing about a list either.
func TestPuttingACardAwayLeavesTheListOfTheMachineToTheBoard(t *testing.T) {
	e := newAgentEnv(t)
	now := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	card := e.fixture.CardInDoing

	expectCode(t, e.runAt(now, "work", "start", card.ID, "--json"), ExitOK)
	expectCode(t, e.runAt(now, "heartbeat", "--json"), ExitOK)

	env, r := e.runWithJSON(now, "cards", "archive", card.ID)
	expectCode(t, r, ExitOK)
	if strings.Contains(env.Notice, "no longer among the cards") {
		t.Errorf("the command took the card out of the list itself: %q", env.Notice)
	}
	listing, _ := e.runWithJSON(now, "work", "list")
	if got := cardIDsOfTheList(t, listing); len(got) != 1 || got[0] != card.ID {
		t.Fatalf("the list holds %v, want the card until the board says otherwise", got)
	}

	// The next signal is where the card goes, with the word of the board for why.
	env, r = e.runWithJSON(now.Add(61*time.Second), "heartbeat")
	expectCode(t, r, ExitOK)
	if reasons := letGoOfTheList(t, env); reasons[card.ID] != "cardArchived" {
		t.Errorf("the beat says %v about what was left out", reasons)
	}
}
