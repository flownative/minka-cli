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
