package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

var testIDs = 0

// newTestID is a fresh ID for a step a test puts on a card.
func newTestID() string {
	testIDs++
	return fmt.Sprintf("66666666-6666-4666-8666-%012d", testIDs)
}

// withAShortIdentifier gives the card in Doing the short identifier MNK-1A.
func withAShortIdentifier(e *testEnv) *fakeapi.Card {
	child := e.fixture.CardInDoing
	child.ShortID = "MNK-1A"
	return child
}

// A card is named as people and the skill write it, and goes out the way
// Minka reads a card's name: without the #, out of an address of this host,
// an ID in lowercase. An address of another host is no card here and goes
// out as given.
func TestACardStepGoesOutAsMinkaReadsTheName(t *testing.T) {
	e := newTestEnv(t)
	child := withAShortIdentifier(e)
	parent := e.fixture.CardInTriage.ID
	for given, sent := range map[string]string{
		"#MNK-1A":                         "MNK-1A",
		" mnk-1a ":                        "mnk-1a",
		e.server.URL + "/go/card/MNK-1A":  "MNK-1A",
		e.server.URL + "/go/card/MNK-1A/": "MNK-1A",
		strings.ToUpper(child.ID):         child.ID,
		e.server.URL + "/projects/" + e.fixture.Project.ID + "/boards/" + e.fixture.Board.ID + "/cards/" + strings.ToUpper(child.ID) + "?x=1#comment-1": child.ID,
		"https://elsewhere.example.com/go/card/MNK-1A": "https://elsewhere.example.com/go/card/MNK-1A",
	} {
		_, r := e.runJSON("", "cards", "steps", "create", parent, "--card", given)
		if got := e.lastWriteBody()["cardId"]; got != sent {
			t.Errorf("%q went out as %q, want %q", given, got, sent)
		}
		if sent == "https://elsewhere.example.com/go/card/MNK-1A" {
			expectCode(t, r, ExitValidation)
		} else {
			expectCode(t, r, ExitOK)
		}
	}
	if steps := e.fixture.CardInTriage.Steps; len(steps) != 1 || steps[0].CardID != child.ID {
		t.Fatalf("the card carries %v", steps)
	}
}

// The first time is an addition, every other one says the card was a step
// already, and the data is the parent card as Minka answered it.
func TestACardStepSaysWhetherItWasNew(t *testing.T) {
	e := newTestEnv(t)
	withAShortIdentifier(e)
	parent := e.fixture.CardInTriage

	env, r := e.runJSON("", "cards", "steps", "create", parent.ID, "--card", "MNK-1A")
	expectCode(t, r, ExitOK)
	if env.Summary != "Added card #MNK-1A "+e.fixture.CardInDoing.Title+" as a step of card "+parent.Title || decode(t, env.Data)["id"] != parent.ID {
		t.Fatalf("summary %q, data %s", env.Summary, env.Data)
	}
	env, r = e.runJSON("", "cards", "steps", "create", parent.ID, "--card", "MNK-1A")
	expectCode(t, r, ExitOK)
	if !strings.Contains(env.Summary, "was a step of card "+parent.Title+" already") || len(parent.Steps) != 1 {
		t.Fatalf("summary %q, %d steps", env.Summary, len(parent.Steps))
	}

	_, r = e.runJSON("", "cards", "steps", "create", parent.ID, "A text", "--card", "MNK-1A")
	expectCode(t, r, ExitUsage)
	env, r = e.runJSON("", "cards", "steps", "create", parent.ID, "--card", parent.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepCardIsThisCard" || env.Hint == "" {
		t.Errorf("the card itself: %v, hint %q", env.Meta, env.Hint)
	}
	env, r = e.runJSON("", "cards", "steps", "create", parent.ID, "--card", e.fixture.OtherCard.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepCardUnknown" || !strings.Contains(env.Hint, "#MNK-1A") {
		t.Errorf("a card nobody here can see: %v, hint %q", env.Meta, env.Hint)
	}
}

// A card step reads as its card, without a tick box of its own; one the
// reader cannot see says so; a card names the cards it is a step of.
func TestTheTextViewsOfACardStep(t *testing.T) {
	e := newTestEnv(t)
	child := withAShortIdentifier(e)
	child.AssigneeIDs = []string{e.fixture.Anke.ID}
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), Title: "Measure"}, {ID: newTestID(), CardID: child.ID}, {ID: newTestID(), CardID: e.fixture.OtherCard.ID}}

	r := e.run("", "cards", "steps", "list", parent.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, line := range []string{
		"1. [ ] Measure",
		"2. ↳ #MNK-1A " + child.Title + "  [Doing] (Anke) " + e.server.URL + "/go/card/MNK-1A",
		"3. ↳ (a card you cannot open)",
	} {
		if !strings.Contains(r.stdout, line) {
			t.Errorf("the list lacks %q:\n%s", line, r.stdout)
		}
	}

	r = e.run("", "cards", "show", child.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "# "+child.Title+"\nStep of: "+parent.Title+"\n") {
		t.Errorf("the child does not name its parent:\n%s", r.stdout)
	}
}

// A step without a title, such as a card step whose card the reader cannot
// see, is never named by an empty text.
func TestAnEmptyTextNamesNoStep(t *testing.T) {
	e := newTestEnv(t)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), CardID: e.fixture.OtherCard.ID}}
	writes := len(e.server.WritingRequests())

	_, r := e.runJSON("", "cards", "steps", "delete", parent.ID, "")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes || len(parent.Steps) != 1 {
		t.Fatal("an empty text named the step")
	}

	env, r := e.runJSON("", "cards", "steps", "delete", parent.ID, "1")
	expectCode(t, r, ExitOK)
	if env.Summary != "Took a card you cannot open off the steps of card "+parent.Title+"; the card itself stays" || decode(t, env.Data)["wasOnCard"] != true {
		t.Errorf("summary %q, data %s", env.Summary, env.Data)
	}
}

// A summary names a card step by its card, and one whose card the reader
// cannot see by its position.
func TestASummaryNamesACardStepByItsCard(t *testing.T) {
	e := newTestEnv(t)
	child := withAShortIdentifier(e)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), CardID: child.ID}, {ID: newTestID(), CardID: e.fixture.OtherCard.ID}}

	env, r := e.runJSON("", "cards", "steps", "move", parent.ID, "1", "--bottom")
	expectCode(t, r, ExitOK)
	if want := "Moved card step #MNK-1A " + child.Title + " on card " + parent.Title; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
	env, r = e.runJSON("", "cards", "steps", "move", parent.ID, "1", "--bottom")
	expectCode(t, r, ExitOK)
	if want := "Moved step 1, a card you cannot open on card " + parent.Title; env.Summary != want {
		t.Errorf("summary %q, want %q", env.Summary, want)
	}
}

// --card given empty names no card, and nothing goes out.
func TestAnEmptyCardFlagIsAUsageError(t *testing.T) {
	e := newTestEnv(t)
	for _, given := range []string{"", " "} {
		env, r := e.runJSON("", "cards", "steps", "create", e.fixture.CardInTriage.ID, "--card", given)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "--card names no card") || len(e.server.WritingRequests()) != 0 {
			t.Errorf("%q: error %q, %d writes", given, env.Error, len(e.server.WritingRequests()))
		}
	}
}

// A card step whose card the reader cannot see cannot be changed through its
// card either, so the hint offers only to remove the link.
func TestTheHintOfAnUnseenCardStepOffersOnlyTheLink(t *testing.T) {
	e := newTestEnv(t)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), CardID: e.fixture.OtherCard.ID}}

	env, r := e.runJSON("", "cards", "steps", "complete", parent.ID, "1")
	expectCode(t, r, ExitValidation)
	if !strings.Contains(env.Hint, "minka cards steps delete "+parent.ID+" 1") || strings.Contains(env.Hint, "cards move") {
		t.Errorf("hint %q", env.Hint)
	}
}

// make-card tells a step that is not on the card from a Minka without the
// address by the API's own 404, and names the people of a refused step.
func TestMakeCardRefusals(t *testing.T) {
	e := newTestEnv(t)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), Title: "Lights", AssigneeIDs: []string{e.fixture.Stranger.ID}}}

	env, r := e.runJSON("", "cards", "steps", "make-card", parent.ID, newTestID())
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Error, "does not know") {
		t.Errorf("a step not on the card reads as an old Minka: %q", env.Error)
	}

	env, r = e.runJSON("", "cards", "steps", "make-card", parent.ID, "1")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "personNotAProjectMember" || env.Hint != stepPeopleHint {
		t.Errorf("a stranger on the step: %v, hint %q", env.Meta, env.Hint)
	}
}

// A card step carries nothing of its own, and the refusal says where its
// title, people, day and column are changed.
func TestACardStepIsChangedOnItsCard(t *testing.T) {
	e := newTestEnv(t)
	child := withAShortIdentifier(e)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), CardID: child.ID}}

	for _, args := range [][]string{
		{"cards", "steps", "complete", parent.ID, "1"},
		{"cards", "steps", "reopen", parent.ID, "1"},
		{"cards", "steps", "edit", parent.ID, "1", "--due", "2026-12-01"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != "stepIsACard" || !strings.Contains(env.Hint, "minka cards move "+child.ID) {
			t.Errorf("%s: %v, hint %q", args[2], env.Meta, env.Hint)
		}
	}
}

// A step made into a card hands over the new card and says where it is and
// that the step now is that card.
func TestAStepIsMadeIntoACard(t *testing.T) {
	e := newTestEnv(t)
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), Title: "Other"}, {ID: newTestID(), Title: "Measure the place"}}

	env, r := e.runJSON("", "cards", "steps", "make-card", parent.ID, "Measure the place")
	expectCode(t, r, ExitOK)
	made := decode(t, env.Data)
	if made["title"] != "Measure the place" || made["id"] != parent.Steps[1].CardID || env.Summary != `Made step "Measure the place" into card Measure the place in Triage` {
		t.Fatalf("summary %q, data %v", env.Summary, made)
	}

	env, r = e.runJSON("", "cards", "steps", "make-card", parent.ID, "2")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "stepIsACard" || !strings.Contains(env.Hint, "a card already") {
		t.Errorf("a card step: %v, hint %q", env.Meta, env.Hint)
	}

	parent.Steps = append(parent.Steps, &fakeapi.Step{ID: newTestID(), Title: "Lights"})
	r = e.run("", "cards", "steps", "make-card", parent.ID, "3", "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "\nStep 3 of card "+parent.Title+" now is that card.\n") {
		t.Errorf("the text:\n%s", r.stdout)
	}
}

// A Minka without card steps answers --card with the missing text and has no
// address for make-card: both say the host does not know card steps, as a
// feature that is not there, and its card steps' absence breaks no view.
func TestAMinkaWithoutCardSteps(t *testing.T) {
	e := newTestEnv(t)
	e.server.KnowsNoCardSteps = true
	parent := e.fixture.CardInTriage
	parent.Steps = []*fakeapi.Step{{ID: newTestID(), Title: "Measure"}}

	for _, args := range [][]string{
		{"cards", "steps", "create", parent.ID, "--card", e.fixture.CardInDoing.ID},
		{"cards", "steps", "make-card", parent.ID, "1"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Error, "does not know a card as the step of another") || !strings.Contains(env.Hint, "older") {
			t.Errorf("%s: %q, hint %q", args[2], env.Error, env.Hint)
		}
	}

	r := e.run("", "cards", "show", parent.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "1. [ ] Measure") || strings.Contains(r.stdout, "Step of") {
		t.Errorf("the card reads:\n%s", r.stdout)
	}
}
