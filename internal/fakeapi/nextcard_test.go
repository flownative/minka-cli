package fakeapi

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func nextCardPath(f *Fixture) string {
	return "/api/v1/boards/" + f.Board.ID + "/next-card"
}

// nextCardID is the id of the card the server names next for Bruno, "null"
// where it names none.
func nextCardID(t *testing.T, s *Server, f *Fixture) string {
	t.Helper()
	body, _ := getBytes(t, s, f.Token, nextCardPath(f))
	if body == "null" {
		return body
	}
	var card map[string]any
	if err := json.Unmarshal([]byte(body), &card); err != nil {
		t.Fatalf("no card: %s", body)
	}
	id, _ := card["id"].(string)
	return id
}

// Of the columns with the role ready from left to right, each read from the
// bottom, the first card that is Bruno's and neither on hold nor completed;
// a column of another role does not count, however far left it stands.
func TestTheFakeNamesTheBottomCardOfTheLeftmostReadyColumnThatHasOne(t *testing.T) {
	s, f := New(t)
	ready, inProgress := "ready", "in_progress"
	left := f.Doing
	left.Role = &ready
	left.OnHoldEnabled = true
	doingTitle, rightTitle := "In progress", "Also ready"
	doing := &Column{ID: newID(), Type: "column", CustomTitle: &doingTitle, Role: &inProgress}
	right := &Column{ID: newID(), Type: "column", CustomTitle: &rightTitle, Role: &ready}
	f.Board.Columns = slices.Insert(f.Board.Columns, 1, doing)
	f.Board.Columns = slices.Insert(f.Board.Columns, 3, right)

	card := func(column *Column, title string, assignees ...string) *Card {
		c := &Card{ID: newID(), Title: title, CreatedAt: s.now, CreatorID: f.Bruno.ID, AssigneeIDs: assignees}
		column.Cards = append(column.Cards, c)
		return c
	}
	bruno := f.Bruno.ID
	left.Cards = nil
	card(doing, "Already at it", bruno)
	upper := card(right, "Upper", bruno)
	lower := card(right, "Lower", bruno)
	card(right, "Anke's", f.Anke.ID)
	if got := nextCardID(t, s, f); got != lower.ID {
		t.Fatalf("next %s, want the bottom card of the right column %s (upper is %s)", got, lower.ID, upper.ID)
	}

	top := card(left, "Top of the left", bruno)
	onHold := card(left, "On hold", bruno)
	onHold.OnHold = true
	completed := card(left, "Completed", bruno)
	now := time.Now()
	completed.CompletedAt = &now
	card(left, "Nobody's")
	if got := nextCardID(t, s, f); got != top.ID {
		t.Errorf("next %s, want the only card of Bruno's released on the left %s", got, top.ID)
	}

	left.Role = nil
	right.Cards = []*Card{upper}
	upper.AssigneeIDs = []string{f.Anke.ID}
	if got := nextCardID(t, s, f); got != "null" {
		t.Errorf("nothing released: %s", got)
	}
}

// The card comes back as the list of one's cards gives it.
func TestTheFakeNamesTheNextCardAsTheListOfMyCardsGivesIt(t *testing.T) {
	s, f := New(t)
	ready := "ready"
	f.Doing.Role = &ready
	f.Doing.Cards[0].AssigneeIDs = []string{f.Bruno.ID}

	next, _ := getBytes(t, s, f.Token, nextCardPath(f))
	var mine []json.RawMessage
	getJSON(t, s, f.Token, "/api/v1/me/cards?boardId="+f.Board.ID, &mine)
	if len(mine) != 1 || string(mine[0]) != next {
		t.Errorf("next %s, mine %s", next, mine)
	}
}

// A board somebody cannot see answers exactly as the board does, before a
// board without a ready column is refused; a Minka without the address
// answers with a page of its web.
func TestTheFakeRefusesTheNextCardAsMinkaDoes(t *testing.T) {
	s, f := New(t)

	if got := sendAs(t, s, f.Token, "GET", nextCardPath(f), "application/json", ""); got != "409 boardHasNoReadyColumn" {
		t.Errorf("no ready column: %s", got)
	}

	board, _ := getBytes(t, s, f.StrangerToken, "/api/v1/boards/"+f.Board.ID)
	next, contentType := getBytes(t, s, f.StrangerToken, nextCardPath(f))
	if next != board || next != NotFoundBody || contentType != "application/json; charset=utf-8" {
		t.Errorf("a stranger: %s (%s), the board answers %s", next, contentType, board)
	}
	if got := sendAs(t, s, "minka_pat_nobody", "GET", nextCardPath(f), "application/json", ""); got != "401 unauthenticated" {
		t.Errorf("no token: %s", got)
	}

	ready := "ready"
	f.Doing.Role = &ready
	s.KnowsNoNextCard = true
	if body, contentType := getBytes(t, s, f.Token, nextCardPath(f)); contentType != "text/html" {
		t.Errorf("an older Minka: %s (%s)", body, contentType)
	}
}
