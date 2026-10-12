package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

// shortURL is what one answer says about the short address of a card.
type shortURL struct {
	value   any
	present bool
}

// shortURLsOfACard collects the shortUrl of one card from every answer that
// carries a card resource: the single card, the card in a column of its
// board, one's own cards and a card in the archive of its board.
func shortURLsOfACard(e *testEnv, cardID, archivedID string) map[string]shortURL {
	e.t.Helper()
	found := map[string]shortURL{}
	field := func(where string, card map[string]any) {
		value, present := card["shortUrl"]
		found[where] = shortURL{value, present}
	}

	env, r := e.runJSON("", "cards", "show", cardID)
	expectCode(e.t, r, ExitOK)
	field("cards show", decode(e.t, env.Data))

	env, r = e.runJSON("", "boards", "show", e.fixture.Board.ID)
	expectCode(e.t, r, ExitOK)
	for _, column := range decode(e.t, env.Data)["columns"].([]any) {
		for _, card := range column.(map[string]any)["cards"].([]any) {
			if card.(map[string]any)["id"] == cardID {
				field("boards show", card.(map[string]any))
			}
		}
	}

	env, r = e.runJSON("", "cards", "mine")
	expectCode(e.t, r, ExitOK)
	var mine []map[string]any
	if err := json.Unmarshal(env.Data, &mine); err != nil {
		e.t.Fatal(err)
	}
	for _, card := range mine {
		if card["id"] == cardID {
			field("cards mine", card)
		}
	}

	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID)
	expectCode(e.t, r, ExitOK)
	var archived []map[string]any
	if err := json.Unmarshal(env.Data, &archived); err != nil {
		e.t.Fatal(err)
	}
	for _, card := range archived {
		if card["id"] == archivedID {
			field("boards archive", card)
		}
	}

	for _, where := range []string{"cards show", "boards show", "cards mine", "boards archive"} {
		if _, seen := found[where]; !seen {
			e.t.Fatalf("%s does not list the card", where)
		}
	}
	return found
}

func aCardOfMineAndOneInTheArchive(e *testEnv, prefixed bool) (string, string) {
	e.t.Helper()
	archivedID := anArchiveOf(e, 1)[0]
	if prefixed {
		e.fixture.CardInDoing.ShortID = "MNK-1A"
		e.server.Archive[0].Card.ShortID = "MNK-7G"
	}
	expectCode(e.t, e.run("", "cards", "assign", e.fixture.CardInDoing.ID, "me", "--json"), ExitOK)
	return e.fixture.CardInDoing.ID, archivedID
}

func TestACardCarriesItsShortAddressWhereverItIsListed(t *testing.T) {
	e := newTestEnv(t)
	cardID, archivedID := aCardOfMineAndOneInTheArchive(e, true)

	for where, got := range shortURLsOfACard(e, cardID, archivedID) {
		want := e.server.URL + "/go/card/MNK-1A"
		if where == "boards archive" {
			want = e.server.URL + "/go/card/MNK-7G"
		}
		if got.value != want {
			t.Errorf("%s: shortUrl %v, want %s", where, got.value, want)
		}
	}
}

func TestACardOfABoardWithoutAPrefixHasNoShortAddress(t *testing.T) {
	e := newTestEnv(t)
	cardID, archivedID := aCardOfMineAndOneInTheArchive(e, false)

	for where, got := range shortURLsOfACard(e, cardID, archivedID) {
		if !got.present || got.value != nil {
			t.Errorf("%s: shortUrl %v (present: %v), want null", where, got.value, got.present)
		}
	}
}

// A Minka from before the short address leaves the field out. Its answers are
// read all the same, and passed on as served, without a field it never sent.
func TestAMinkaWithoutShortAddressesIsStillRead(t *testing.T) {
	e := newTestEnv(t)
	e.server.HandsOutNoShortURL = true
	cardID, archivedID := aCardOfMineAndOneInTheArchive(e, true)

	for where, got := range shortURLsOfACard(e, cardID, archivedID) {
		if got.present {
			t.Errorf("%s: shortUrl %v, which the server never sent", where, got.value)
		}
	}
	for _, args := range [][]string{
		{"cards", "show", cardID},
		{"boards", "show", e.fixture.Board.ID},
		{"cards", "mine"},
		{"boards", "archive", e.fixture.Board.ID},
	} {
		expectCode(t, e.run("", append(args, "--styled")...), ExitOK)
	}
}

// subjectsOfAWait are the subjects of the lines a wait prints, by the card
// each is about: one with a short identifier and one without.
func subjectsOfAWait(e *testEnv) (prefixed, plain map[string]any) {
	e.t.Helper()
	e.fixture.CardInDoing.ShortID = "MNK-B4"
	e.aLineArrivesInTheWait(e.aMove(e.fixture.CardInDoing), e.aMove(e.fixture.CardInTriage))

	env, r := e.runWaitJSON(nil)
	expectCode(e.t, r, ExitOK)
	for _, line := range linesOf(e.t, env) {
		subject := line["subject"].(map[string]any)
		switch subject["id"] {
		case e.fixture.CardInDoing.ID:
			prefixed = subject
		case e.fixture.CardInTriage.ID:
			plain = subject
		}
	}
	if prefixed == nil || plain == nil {
		e.t.Fatalf("the wait does not print both lines: %s", env.Data)
	}
	return prefixed, plain
}

func TestTheSubjectOfANotificationCarriesTheShortAddressOfItsCard(t *testing.T) {
	e := newTestEnv(t)
	prefixed, plain := subjectsOfAWait(e)

	if want := e.server.URL + "/go/card/MNK-B4"; prefixed["shortUrl"] != want {
		t.Errorf("shortUrl %v, want %s", prefixed["shortUrl"], want)
	}
	if value, present := plain["shortUrl"]; !present || value != nil {
		t.Errorf("a card without a prefix: shortUrl %v (present: %v), want null", value, present)
	}
}

func TestTheSubjectOfANotificationFromAnOlderMinkaIsStillRead(t *testing.T) {
	e := newTestEnv(t)
	e.server.HandsOutNoShortURL = true
	prefixed, plain := subjectsOfAWait(e)

	for _, subject := range []map[string]any{prefixed, plain} {
		if value, present := subject["shortUrl"]; present {
			t.Errorf("shortUrl %v, which the server never sent", value)
		}
	}
}

// textAddressesOfACard is the address the text views give for a card: the one
// it shows, the one an assignment answers with and the one an edit answers
// with.
func textAddressesOfACard(e *testEnv, cardID string) map[string]string {
	e.t.Helper()
	addresses := map[string]string{}
	for where, args := range map[string][]string{
		"cards show":   {"cards", "show", cardID},
		"cards assign": {"cards", "assign", cardID, "me"},
		"cards edit":   {"cards", "edit", cardID, "--title", "Renamed"},
	} {
		r := e.run("", append(args, "--styled")...)
		expectCode(e.t, r, ExitOK)
		address := ""
		for _, line := range strings.Split(r.stdout, "\n") {
			line = strings.TrimPrefix(line, "URL: ")
			if strings.HasPrefix(line, e.server.URL+"/") {
				address = line
			}
		}
		if address == "" {
			e.t.Fatalf("%s gives no address:\n%s", where, r.stdout)
		}
		addresses[where] = address
	}
	return addresses
}

func TestTheTextViewsGiveTheShortAddressOfACard(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"

	for where, got := range textAddressesOfACard(e, e.fixture.CardInDoing.ID) {
		if want := e.server.URL + "/go/card/MNK-1A"; got != want {
			t.Errorf("%s: %s, want %s", where, got, want)
		}
	}
}

func TestTheTextViewsGiveTheLongAddressWhereThereIsNoShortOne(t *testing.T) {
	for name, olderMinka := range map[string]bool{"a board without a prefix": false, "an older Minka": true} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			if olderMinka {
				e.server.HandsOutNoShortURL = true
				e.fixture.CardInDoing.ShortID = "MNK-1A"
			}
			board := e.fixture.Board
			want := e.server.URL + "/projects/" + board.ProjectID + "/boards/" + board.ID + "/cards/" + e.fixture.CardInDoing.ID
			for where, got := range textAddressesOfACard(e, e.fixture.CardInDoing.ID) {
				if got != want {
					t.Errorf("%s: %s, want %s", where, got, want)
				}
			}
		})
	}
}
