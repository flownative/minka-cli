package cli

import (
	"strings"
	"testing"
)

func TestTheShortIdentifierOfACardIsShownWhereACardIs(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"
	expectCode(t, e.run("", "cards", "assign", e.fixture.CardInDoing.ID, "me", "--json"), ExitOK)

	for _, args := range [][]string{
		{"boards", "show", e.fixture.Board.ID},
		{"cards", "mine"},
	} {
		r := e.run("", append(args, "--styled")...)
		expectCode(t, r, ExitOK)
		if !strings.Contains(r.stdout, "MNK-1A  "+e.fixture.CardInDoing.Title) {
			t.Errorf("%v does not show the short identifier in front of the title:\n%s", args, r.stdout)
		}
		if !strings.Contains(r.stdout, e.fixture.CardInDoing.ID) {
			t.Errorf("%v no longer shows the ID, which is what commands take", args)
		}
	}
}

func TestACardWithoutAShortIdentifierLooksAsBefore(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "cards", "assign", e.fixture.CardInDoing.ID, "me", "--json"), ExitOK)

	for _, args := range [][]string{
		{"cards", "show", e.fixture.CardInDoing.ID},
		{"boards", "show", e.fixture.Board.ID},
		{"cards", "mine"},
	} {
		r := e.run("", append(args, "--styled")...)
		expectCode(t, r, ExitOK)
		if strings.Contains(r.stdout, "Short ID") || strings.Contains(r.stdout, e.fixture.CardInDoing.ID+"    ") {
			t.Errorf("%v leaves a trace of a short identifier that is not there:\n%s", args, r.stdout)
		}
	}

	env, _ := e.runJSON("", "cards", "show", e.fixture.CardInDoing.ID)
	if value, present := decode(t, env.Data)["shortId"]; !present || value != nil {
		t.Fatalf("shortId is %v (present: %v), want null", value, present)
	}
}

func TestTheHeadingOfACardStaysItsBareTitle(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"

	r := e.run("", "cards", "show", e.fixture.CardInDoing.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.HasPrefix(r.stdout, "# "+e.fixture.CardInDoing.Title+"\nShort ID: MNK-1A\n") {
		t.Fatalf("the heading is not the bare title followed by the short identifier:\n%s", r.stdout)
	}
}

func TestACardIsFoundByItsShortIdentifierInAnyCase(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"

	for _, name := range []string{"MNK-1A", "mnk-1a"} {
		env, r := e.runJSON("", "cards", "show", name)
		expectCode(t, r, ExitOK)
		if got := decode(t, env.Data)["id"]; got != e.fixture.CardInDoing.ID {
			t.Fatalf("%s: card %v, want %s", name, got, e.fixture.CardInDoing.ID)
		}
	}
}

func TestACommandThatWritesGoesOnWithTheIDOfTheCard(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"

	env, r := e.runJSON("", "comments", "create", "MNK-1A", "A comment")
	expectCode(t, r, ExitOK)

	write := e.server.WritingRequests()[0]
	if write.Path != "/api/v1/cards/"+e.fixture.CardInDoing.ID+"/comments" {
		t.Fatalf("the comment went to %s", write.Path)
	}
	for _, crumb := range env.Breadcrumbs {
		if strings.Contains(crumb.Cmd, "MNK-1A") {
			t.Fatalf("a breadcrumb names the card by its short identifier, which may stop naming it: %s", crumb.Cmd)
		}
	}
}

func TestANeighbourIsNamedByItsShortIdentifierToo(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInTriage.ShortID = "MNK-1A"
	e.fixture.CardInDoing.ShortID = "MNK-2B"

	_, r := e.runJSON("", "cards", "move", "MNK-1A", "--column", "Doing", "--after", "mnk-2b")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if body["previousCardId"] != e.fixture.CardInDoing.ID {
		t.Fatalf("the neighbour did not travel as an ID: %v", body)
	}

	_, itself := e.runJSON("", "cards", "move", "MNK-1A", "--column", "Doing", "--after", "mnk-1a")
	expectCode(t, itself, ExitUsage)
}

func TestAShortIdentifierThatNamesNothingSaysWhatThatCanMean(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.OtherCard.ShortID = "OTH-1"

	for _, name := range []string{"MNK-9Z", "OTH-1"} {
		env, r := e.runJSON("", "comments", "create", name, "A comment")
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Error, strings.ToUpper(name)) || !strings.Contains(env.Hint, "short identifiers") {
			t.Fatalf("%s: error %q, hint %q", name, env.Error, env.Hint)
		}
		if len(e.server.WritingRequests()) != 0 {
			t.Fatal("something was written")
		}
	}
}

func TestWhatIsNeitherAnIDNorAShortIdentifierSendsNothing(t *testing.T) {
	e := newTestEnv(t)

	for _, name := range []string{"MNK", "MNK-", "-1A", "MNK-1A/../../me", "M-1", "MNK-1234567890123", "MNK 1A"} {
		_, r := e.runJSON("", "cards", "show", name)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
}

// Every command that takes a card has to look a short identifier up and go on
// with the ID: nothing but the lookup may carry the short identifier.
func TestEveryCommandThatTakesACardTakesItsShortIdentifier(t *testing.T) {
	reading := map[string][]string{
		"cards show": {"cards", "show", "CARD"},
		"steps list": {"steps", "list", "CARD"},
	}

	names := map[string]bool{}
	for name := range everyWrite(newTestEnv(t)) {
		names[name] = true
	}
	for name := range reading {
		names[name] = true
	}

	for name := range names {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			args, isWrite := everyWrite(e)[name]
			if !isWrite {
				args = reading[name]
			}
			e.fixture.CardInDoing.ShortID = "MNK-1A"
			e.fixture.CardInTriage.ShortID = "MNK-2B"

			named := false
			for i, arg := range args {
				switch arg {
				case e.fixture.CardInDoing.ID, "CARD":
					args[i], named = "mnk-1a", true
				case e.fixture.CardInTriage.ID:
					args[i], named = "mnk-2b", true
				}
			}
			if !named {
				t.Skip("takes no card")
			}
			before := len(e.server.Requests())

			_, r := e.runJSON("", args...)
			if name == "cards restore" {
				// A card in the trash goes by no short identifier.
				expectCode(t, r, ExitNotFound)
			} else {
				expectCode(t, r, ExitOK)
			}

			lookups := 0
			for _, request := range e.server.Requests()[before:] {
				if strings.HasPrefix(request.Path, "/api/v1/cards/by-short-id/") {
					lookups++
					continue
				}
				if strings.Contains(strings.ToUpper(request.Path+request.Query+string(request.Body)), "MNK-") {
					t.Errorf("%s %s carries the short identifier", request.Method, request.Path)
				}
			}
			if lookups != 1 {
				t.Errorf("%d lookups, want 1", lookups)
			}
		})
	}
}

func TestAnAnswerWithoutAnIDStopsTheCommand(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.CardInDoing.ShortID = "MNK-1A"
	e.server.AnswersCardsWithoutID = true

	env, r := e.runJSON("", "comments", "create", "MNK-1A", "A comment")
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI {
		t.Fatalf("code %q", env.Code)
	}
	if len(e.server.WritingRequests()) != 0 {
		t.Fatal("something was written to a card without an ID")
	}
}
