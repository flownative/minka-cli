package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// anArchiveOf fills the archive of the fixture's board with as many cards as
// asked for and answers their ids in the order the archive lists them, the card
// put away last first. Every card gets a moment of its own, a minute apart, so
// that the order under test is the one the server sorts by and not the one the
// cards were put away in.
func anArchiveOf(e *testEnv, count int) []string {
	e.t.Helper()
	ids := make([]string, count)
	when := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	for i := range count {
		card := &fakeapi.Card{
			ID:        fmt.Sprintf("0199aaaa-bbbb-4ccc-8ddd-%012d", i),
			Title:     fmt.Sprintf("Put away %d", i),
			CreatedAt: when,
			CreatorID: e.fixture.Bruno.ID,
		}
		e.fixture.Doing.Cards = append(e.fixture.Doing.Cards, card)
		entry := e.server.PutInTheArchive(card)
		if entry == nil {
			e.t.Fatalf("the card %s did not reach the archive", card.ID)
		}
		entry.At = when.Add(time.Duration(i) * time.Minute)
		entry.ByID = e.fixture.Bruno.ID
		ids[count-1-i] = card.ID
	}
	return ids
}

// archiveIDs are the cards of an answer, in the order they stand in it.
func archiveIDs(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var cards []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &cards); err != nil {
		t.Fatalf("the cards of the archive are not a JSON list: %v: %s", err, raw)
	}
	ids := make([]string, len(cards))
	for i, card := range cards {
		ids[i] = card.ID
	}
	return ids
}

// pagesAskedFor are the requests for pages of an archive, in the order they went
// out. Every one of them carries the cursor it asked with.
func pagesAskedFor(e *testEnv) []fakeapi.RecordedRequest {
	var asked []fakeapi.RecordedRequest
	for _, request := range e.server.Requests() {
		if request.Method == http.MethodGet && strings.HasPrefix(request.Path, "/api/v1/boards/") && strings.HasSuffix(request.Path, "/archive") {
			asked = append(asked, request)
		}
	}
	return asked
}

// askForAPage asks for a page without this CLI's client, which bounds the page
// size itself: what is under test with it is the order the server checks its
// refusals in.
func askForAPage(t *testing.T, e *testEnv, boardID, query string) (int, string) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, e.server.URL+"/api/v1/boards/"+boardID+"/archive?"+query, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+e.fixture.Token)
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer answer.Body.Close()
	body, err := io.ReadAll(answer.Body)
	if err != nil {
		t.Fatal(err)
	}
	return answer.StatusCode, string(body)
}

// A page larger than the largest the server serves is this CLI's own usage
// error, refused before anything goes out. The server would cut it down without
// a word, and a page cut down is a full page that reads as a short one — which
// is the only sign the answer carries for the end of the archive.
func TestAPageOfAnArchiveBeyondWhatTheServerServesIsRefusedBeforeItIsAsked(t *testing.T) {
	e := newTestEnv(t)
	anArchiveOf(e, 3)

	for _, limit := range []string{"101", "500", "0"} {
		env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", limit)
		expectCode(t, r, ExitUsage)
		if env.Code != CodeUsage || !strings.Contains(env.Error, "between 1 and 100") {
			t.Errorf("--limit %s: code %q, error %q", limit, env.Code, env.Error)
		}
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Fatalf("%d requests went out for a page the server would have cut down in silence", n)
	}

	// The bound the refusal names is one that works.
	_, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "100")
	expectCode(t, r, ExitOK)
}

// The fake cuts a page down the way the server does, which is what makes the
// bound above a safeguard and not a restriction: nothing in the answer says it
// was cut, and a caller that asked for 500 and got 100 would take a hundred
// cards for the whole archive.
func TestTheServerCutsAPageDownWithoutAWord(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 101)

	status, body := askForAPage(t, e, e.fixture.Board.ID, "limit=500")
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	got := archiveIDs(t, json.RawMessage(body))
	if !slices.Equal(got, ids[:100]) {
		t.Fatalf("%d cards for a page of 500 asked for, want the first 100 of %d", len(got), len(ids))
	}
}

// A board that is not there, or belongs to a project one is not in, beats both
// refusals of the query. A broken limit against a foreign board therefore says
// "board not found" and nothing about the limit, which is why the bound is held
// here and not left to the server.
func TestABoardThatIsNotThereBeatsTheRefusalsOfAPage(t *testing.T) {
	e := newTestEnv(t)
	anArchiveOf(e, 2)
	foreign := e.fixture.OtherBoard.ID

	if status, body := askForAPage(t, e, foreign, "limit=0&before=nonsense"); status != http.StatusNotFound {
		t.Errorf("a foreign board with a broken query: status %d: %s", status, body)
	}
	for query, code := range map[string]string{
		"limit=0":                            "archivedCardsLimitNotANumber",
		"limit=x":                            "archivedCardsLimitNotANumber",
		"limit=0&before=nonsense":            "archivedCardsLimitNotANumber",
		"before=nonsense":                    "archivedCardsBeforeUnknown",
		"before=" + e.fixture.CardInDoing.ID: "archivedCardsBeforeUnknown",
	} {
		status, body := askForAPage(t, e, e.fixture.Board.ID, query)
		if status != http.StatusUnprocessableEntity || !strings.Contains(body, code) {
			t.Errorf("%s: status %d: %s", query, status, body)
		}
	}
}

// A page lists what the archive holds and no more: the card itself is read at
// its own address.
func TestAPageOfAnArchiveListsTheCardsPutAwayLastFirst(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 4)
	short := "MNK-7G"
	e.server.Archive[0].Card.ShortID = short
	// A card put away before Minka noted the moment, and one nobody's name is
	// known for: both are fields the archive answers with null.
	e.server.Archive[1].At = time.Time{}
	e.server.Archive[1].ByID = ""

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if env.Summary != "4 cards" {
		t.Errorf("summary %q", env.Summary)
	}
	// The card whose moment nobody noted sorts as the oldest, as the server
	// sorts it, so it drops from the third place to the last.
	noMoment := e.server.Archive[1].Card.ID
	if noMoment != ids[2] {
		t.Fatalf("the card without a moment is not the one in the third place: %s", noMoment)
	}
	want := []string{ids[0], ids[1], ids[3], noMoment}
	if got := archiveIDs(t, env.Data); !slices.Equal(got, want) {
		t.Fatalf("cards\n got: %q\nwant: %q", got, want)
	}

	var cards []map[string]any
	if err := json.Unmarshal(env.Data, &cards); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "shortId", "title", "archivedAt", "archivedBy", "appUrl"} {
		if _, says := cards[0][field]; !says {
			t.Errorf("a card of the archive says nothing about %q: %s", field, env.Data)
		}
	}
	if cards[len(cards)-1]["archivedAt"] != nil || cards[len(cards)-1]["archivedBy"] != nil {
		t.Errorf("a card put away before the moment was noted: %v", cards[len(cards)-1])
	}

	r = e.run("", "boards", "archive", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	for _, word := range []string{short, "Put away 3", "by Bruno"} {
		if !strings.Contains(r.stdout, word) {
			t.Errorf("the text view is missing %q:\n%s", word, r.stdout)
		}
	}

	e = newTestEnv(t)
	r = e.run("", "boards", "archive", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Nothing in the archive.") {
		t.Errorf("an empty archive:\n%s", r.stdout)
	}
}

// The next page is named with the cursor it takes, so that an agent does not
// work one out; a page that cannot have more behind it names none.
func TestAPageOfAnArchiveNamesTheOneBehindIt(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 5)

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "3")
	expectCode(t, r, ExitOK)
	want := fmt.Sprintf("minka boards archive %s --before %s --limit 3", e.fixture.Board.ID, ids[2])
	if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Cmd == want }) {
		t.Fatalf("breadcrumbs %+v, want %q", env.Breadcrumbs, want)
	}

	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--before", ids[2], "--limit", "3")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids[3:]) {
		t.Fatalf("the page behind the third card: %q", got)
	}
	if slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Action == "more" }) {
		t.Errorf("a page shorter than asked for names one behind it: %+v", env.Breadcrumbs)
	}
}

// cursorsAskedWith are the cards the pages of a run were asked to start behind,
// in the order the requests went out, with the empty string for a page that was
// asked for without a cursor.
func cursorsAskedWith(e *testEnv) []string {
	var cursors []string
	for _, request := range pagesAskedFor(e) {
		cursor := ""
		for _, pair := range strings.Split(request.Query, "&") {
			if after, found := strings.CutPrefix(pair, "before="); found {
				cursor = after
			}
		}
		cursors = append(cursors, cursor)
	}
	return cursors
}

// Fewer cards than were asked for is the only sign the answer has for the end of
// the archive, so a page that was full is never the end: one more request is
// owed, and only nothing coming back closes the list.
func TestAFullLastPageIsNotTheEndOfAnArchive(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 20)

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("%d cards of %d: %q", len(got), len(ids), got)
	}
	if !strings.Contains(env.Summary, "20 cards in the archive, read in 2 pages") {
		t.Errorf("summary %q", env.Summary)
	}
	// Two full pages, the empty answer behind them, and the page behind the
	// card before the last one, which settles that the empty answer was the end
	// of the list: it brings that last card and nothing beyond it.
	want := []string{"", ids[9], ids[19], ids[18]}
	if got := cursorsAskedWith(e); !slices.Equal(got, want) {
		t.Fatalf("the pages were asked for behind\n got: %q\nwant: %q", got, want)
	}
	// A page asked for to settle the end is no page of the archive, and the card
	// it brought back was read once.
	if strings.Contains(env.Notice, "came a second time") {
		t.Errorf("nothing came twice here: %q", env.Notice)
	}
}

// The race the server carries as a debt: the card of the cursor goes between the
// check that it lies in the archive and the read of the page, so the page comes
// back empty and reads like the end. Asking the same cursor again would only
// meet the same emptiness; the page behind the card before it says either way
// whether the list has ended, and the walk carries on.
func TestAnEmptyPageBehindAFullOneIsSettledBehindTheCardBeforeIt(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 25)
	// The same window met twice, which is what a cursor asked for a second time
	// runs into: the answer reads like the end of the list, and five cards would
	// be missing from a run that reported success.
	e.server.RacesOnTheArchiveBoundariesOf = []string{ids[19], ids[19]}

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("the walk stopped at the empty page: %d cards of %d: %q", len(got), len(ids), got)
	}
	// The cursor of the page that came back empty is asked with once and never
	// again; what follows it is read behind the card before it.
	want := []string{"", ids[9], ids[19], ids[18], ids[24]}
	if got := cursorsAskedWith(e); !slices.Equal(got, want) {
		t.Fatalf("the pages were asked for behind\n got: %q\nwant: %q", got, want)
	}
	if !strings.Contains(env.Notice, "1 card came a second time") {
		t.Errorf("notice %q does not count what was swallowed", env.Notice)
	}
}

// A page shorter than asked for is the end of the list as the address tells it,
// so nothing behind it is doubted: the empty answer that follows closes the run,
// and no further request is spent on it.
func TestAnEmptyPageBehindAShortOneIsTheEndOfTheArchive(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 12)

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("%d cards of %d: %q", len(got), len(ids), got)
	}
	want := []string{"", ids[9], ids[11]}
	if got := cursorsAskedWith(e); !slices.Equal(got, want) {
		t.Fatalf("the pages were asked for behind\n got: %q\nwant: %q", got, want)
	}
}

// A walk of the whole archive starts where it was told to start. A cursor the
// command was given and passed no further would read the archive from the top
// and look right doing it.
func TestAWalkOfTheWholeArchiveStartsBehindTheCardItWasGiven(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 25)

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--before", ids[4], "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids[5:]) {
		t.Fatalf("%d cards, want the 20 behind the fifth: %q", len(got), got)
	}
	if got := cursorsAskedWith(e); len(got) == 0 || got[0] != ids[4] {
		t.Fatalf("the first page was asked for behind %q", got)
	}

	// Where the card the run was told to start behind is the one that has left
	// the archive, there is nothing of an earlier page to fall back through and
	// nowhere to begin again: the run ends on the refusal, once.
	e = newTestEnv(t)
	ids = anArchiveOf(e, 25)
	e.server.ForgetsTheArchiveBoundariesOf = []string{ids[4]}

	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--before", ids[4], "--limit", "10", "--all")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "archivedCardsBeforeUnknown" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	if strings.Contains(env.Notice, "began again") {
		t.Errorf("the run began again where it had nowhere to begin: %q", env.Notice)
	}
	if want := []string{ids[4]}; !slices.Equal(cursorsAskedWith(e), want) {
		t.Errorf("the pages were asked for behind %q, want %q", cursorsAskedWith(e), want)
	}
}

// One walk can lose one boundary after another, and every one of them sends it
// one card further back through the page it last read, never to the top.
func TestAWalkFallsBackThroughOneLostBoundaryAfterAnother(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 25)
	e.server.ForgetsTheArchiveBoundariesOf = []string{ids[19], ids[18]}

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("%d cards of %d, each once: %q", len(got), len(ids), got)
	}
	want := []string{"", ids[9], ids[19], ids[18], ids[17], ids[24]}
	if got := cursorsAskedWith(e); !slices.Equal(got, want) {
		t.Fatalf("the pages were asked for behind\n got: %q\nwant: %q", got, want)
	}
	if !strings.Contains(env.Notice, "2 cards came a second time") {
		t.Errorf("notice %q does not count what was swallowed", env.Notice)
	}
	if strings.Contains(env.Notice, "began again") {
		t.Errorf("the walk started over although cards of the page were left: %q", env.Notice)
	}
}

// A page of one card leaves no other card of it to ask behind, so the same
// cursor is all there is to ask with again — and a walk that meets the race
// twice must doubt the second empty answer as it doubted the first.
func TestAWalkWithAPageOfOneAsksTheSameCursorAgain(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 3)
	e.server.RacesOnTheArchiveBoundariesOf = []string{ids[0], ids[1]}

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "1", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("%d cards of %d: %q", len(got), len(ids), got)
	}
	if len(e.server.RacesOnTheArchiveBoundariesOf) != 0 {
		t.Errorf("a page was never asked behind %q", e.server.RacesOnTheArchiveBoundariesOf)
	}
	want := []string{"", ids[0], ids[0], ids[1], ids[1], ids[2], ids[2]}
	if got := cursorsAskedWith(e); !slices.Equal(got, want) {
		t.Fatalf("the pages were asked for behind\n got: %q\nwant: %q", got, want)
	}
}

// The weakness the server carries: the cursor holds only while the card it names
// lies in the archive. Once somebody takes that card out, the page behind it is
// refused — and another card of the same page carries on, instead of the walk
// starting over at the first page.
func TestAWalkCarriesOnBehindAnotherCardOfTheSamePage(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 25)
	e.server.ForgetsTheArchiveBoundariesOf = []string{ids[19]}

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	// The precondition: a page was asked behind the card the archive will not
	// be read behind, or the fallback was never reached.
	if !slices.ContainsFunc(pagesAskedFor(e), func(r fakeapi.RecordedRequest) bool {
		return strings.Contains(r.Query, "before="+ids[19])
	}) {
		t.Fatal("no page was asked behind the card the boundary was lost on")
	}
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("%d cards of %d, each once: %q", len(got), len(ids), got)
	}
	if !strings.Contains(env.Notice, "1 card came a second time") {
		t.Errorf("notice %q does not count what was swallowed", env.Notice)
	}
	if strings.Contains(env.Notice, "began again") {
		t.Errorf("the walk started over although a card of the page was left: %q", env.Notice)
	}
}

// Where no card of a page is left to carry on behind, the walk begins again
// where it started, and says so rather than passing it over. A card it will
// never get behind ends it, with what it read handed over all the same.
func TestAWalkWithNoCardLeftToCarryOnBehindBeginsAgainAndSaysSo(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 3)
	// A page of one leaves one candidate, and it is the card the archive will
	// not be read behind: there is nothing of that page to fall back through.
	e.server.ForgetsTheArchiveBoundariesOf = []string{ids[0]}

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "1", "--all")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "archivedCardsBeforeUnknown" {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
	if !strings.Contains(env.Notice, "began again") || !strings.Contains(env.Notice, "the run broke off after 1 card") {
		t.Errorf("notice %q", env.Notice)
	}
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids[:1]) {
		t.Errorf("a walk that broke off hands over what it read: %s", env.Data)
	}
	if !strings.Contains(env.Hint, "again from the beginning") {
		t.Errorf("hint %q", env.Hint)
	}
}

// What a walk of several pages promises, and what it does not, is said as soon
// as there was more than one page — not left in small print.
func TestAWalkOfSeveralPagesSaysWhatItDoesNotPromise(t *testing.T) {
	e := newTestEnv(t)
	anArchiveOf(e, 12)

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	for _, word := range []string{"read in 2 pages", "listed once", "no picture of one moment", "lands on the first page", "without a trace"} {
		if !strings.Contains(env.Notice, word) {
			t.Errorf("the notice is missing %q: %q", word, env.Notice)
		}
	}

	// One page has nothing it could have missed, so it says nothing.
	e = newTestEnv(t)
	anArchiveOf(e, 3)
	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	if env.Notice != "" {
		t.Errorf("notice %q for a walk of one page", env.Notice)
	}
}

func TestTheArchiveOfABoardThatIsNotThereSaysWhatThatCanMean(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "boards", "archive", e.fixture.OtherBoard.ID)
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "minka boards list") {
		t.Errorf("hint %q", env.Hint)
	}
}

func TestACursorOfAnArchiveThatIsNoIDSendsNothing(t *testing.T) {
	e := newTestEnv(t)

	for _, before := range []string{"MNK-1A", "nonsense", "../me"} {
		_, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--before", before)
		expectCode(t, r, ExitUsage)
	}
	if n := len(e.server.Requests()); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
}

func TestACardIsPutAwayIntoTheArchiveAndBroughtBack(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing

	env, r := e.runJSON("", "cards", "archive", card.ID)
	expectCode(t, r, ExitOK)
	if put := decode(t, env.Data); put["archived"] != true {
		t.Fatalf("the card does not say it is archived: %s", env.Data)
	}
	if !strings.Contains(env.Summary, "Put card Review the API into the archive of API-3 Abnahme") {
		t.Errorf("summary %q", env.Summary)
	}

	// It is in the archive of its board, and in no column of it.
	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, []string{card.ID}) {
		t.Fatalf("the archive holds %q", got)
	}
	r = e.run("", "boards", "show", e.fixture.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	if strings.Contains(r.stdout, card.Title) {
		t.Errorf("the board still lists the card:\n%s", r.stdout)
	}

	env, r = e.runJSON("", "cards", "unarchive", card.ID)
	expectCode(t, r, ExitOK)
	back := decode(t, env.Data)
	if back["archived"] != false {
		t.Fatalf("the card is still archived: %s", env.Data)
	}
	if column, _ := back["column"].(map[string]any); column["title"] != "Doing" {
		t.Errorf("the card came back to %v", back["column"])
	}
	if !strings.Contains(env.Summary, "Brought card Review the API out of the archive to Doing") {
		t.Errorf("summary %q", env.Summary)
	}

	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); len(got) != 0 {
		t.Errorf("the archive still holds %q", got)
	}
}

// Nobody asks for the hold to go, and the answer does not say that it did, so
// the command says it.
func TestACardOutOfTheArchiveComesBackOffHold(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing
	e.fixture.Doing.OnHoldEnabled = true
	card.OnHold = true
	e.server.PutInTheArchive(card)

	env, r := e.runJSON("", "cards", "unarchive", card.ID)
	expectCode(t, r, ExitOK)
	if back := decode(t, env.Data); back["onHold"] != false {
		t.Fatalf("the card came back on hold: %s", env.Data)
	}
	if !strings.Contains(env.Notice, "off hold") {
		t.Errorf("notice %q does not say that the hold is gone", env.Notice)
	}
}

func TestPuttingAwayACardOfAnArchiveAndBringingBackOneOfABoardAreRefused(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "cards", "unarchive", e.fixture.CardInDoing.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "cardNotArchived" || !strings.Contains(env.Hint, "minka cards archive") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}

	e.server.PutInTheArchive(e.fixture.CardInDoing)
	env, r = e.runJSON("", "cards", "archive", e.fixture.CardInDoing.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "cardArchived" || !strings.Contains(env.Hint, "minka cards unarchive") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// The trash is not the archive, and a card in it answers here like one that
// never existed. The hint names the trash and the way back out of it, instead of
// borrowing the wording of a post, which is refused with a code of its own.
func TestACardInTheTrashIsNoCardOfAnArchive(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing
	expectCode(t, e.run("", "cards", "delete", card.ID, "--json"), ExitOK)

	for _, action := range []string{"archive", "unarchive"} {
		env, r := e.runJSON("", "cards", action, card.ID)
		expectCode(t, r, ExitNotFound)
		if env.Meta["apiCode"] == "cardArchived" || env.Meta["apiCode"] == "cardNotArchived" {
			t.Errorf("%s: a card in the trash is refused as one of an archive: %v", action, env.Meta)
		}
		if !strings.Contains(env.Hint, "trash") || !strings.Contains(env.Hint, "minka cards restore") {
			t.Errorf("%s: hint %q does not name the trash and the way back", action, env.Hint)
		}
	}
}

// A card that went into the trash out of an archive keeps nothing of it: it
// comes back onto its board, and the archive of that board has forgotten it.
func TestACardTrashedOutOfTheArchiveKeepsNothingOfIt(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInDoing
	e.server.PutInTheArchive(card)

	expectCode(t, e.run("", "cards", "delete", card.ID, "--json"), ExitOK)
	env, r := e.runJSON("", "cards", "restore", card.ID)
	expectCode(t, r, ExitOK)
	if back := decode(t, env.Data); back["archived"] != false {
		t.Fatalf("the restored card is still in the archive: %s", env.Data)
	}

	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); len(got) != 0 {
		t.Fatalf("the archive of the board holds %q", got)
	}
}

// Where no card of the last page is left to ask behind, nothing can check the end
// of the list: an answer that carried nothing has to stand for it, and a card
// that left the archive in that moment reads exactly the same. A page of one card
// is in that place from the start, and such a run says so rather than reporting
// an end it could not check.
func TestARunThatCouldNotCheckTheEndOfTheListSaysSo(t *testing.T) {
	e := newTestEnv(t)
	ids := anArchiveOf(e, 3)
	e.server.RacesOnTheArchiveBoundariesOf = []string{ids[1]}

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "1", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids) {
		t.Fatalf("%d cards of %d: %q", len(got), len(ids), got)
	}
	if !strings.Contains(env.Notice, "the end of the list could not be checked") {
		t.Errorf("notice %q passes an end off as checked", env.Notice)
	}

	// A page of two or more is in the same place once the fallback has used up
	// the cards of the last page: a refusal takes the one candidate, and the race
	// on the card it fell back to leaves nothing to ask behind. Two cards are
	// missing from the run, and that is what the notice is for.
	e = newTestEnv(t)
	ids = anArchiveOf(e, 4)
	e.server.ForgetsTheArchiveBoundariesOf = []string{ids[1]}
	e.server.RacesOnTheArchiveBoundariesOf = []string{ids[0], ids[0]}

	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "2", "--all")
	expectCode(t, r, ExitOK)
	if got := archiveIDs(t, env.Data); !slices.Equal(got, ids[:2]) {
		t.Fatalf("%d cards: %q", len(got), got)
	}
	want := []string{"", ids[1], ids[0], ids[0]}
	if got := cursorsAskedWith(e); !slices.Equal(got, want) {
		t.Fatalf("the pages were asked for behind\n got: %q\nwant: %q", got, want)
	}
	if !strings.Contains(env.Notice, "the end of the list could not be checked") {
		t.Errorf("notice %q passes an end off as checked", env.Notice)
	}

	// A run that could check its end says nothing of the kind.
	e = newTestEnv(t)
	anArchiveOf(e, 4)
	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "2", "--all")
	expectCode(t, r, ExitOK)
	if strings.Contains(env.Notice, "could not be checked") {
		t.Errorf("a run that settled its end doubts it: %q", env.Notice)
	}
}

// A page nobody sized is the one the server gives without a limit, and this CLI
// names it in every request: a default that drifted from the server's would be
// read as the server's own.
func TestAPageOfAnArchiveAsksForFiftyCardsWhereNobodySaysOtherwise(t *testing.T) {
	e := newTestEnv(t)
	anArchiveOf(e, 1)

	_, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	asked := pagesAskedFor(e)
	if len(asked) != 1 || !strings.Contains(asked[0].Query, "limit=50") {
		t.Fatalf("the page was asked for with %q", asked)
	}
}

// What a whole run hands over is a list of cards, a run that read nothing
// included, and it names no page behind it: there is none left to read.
func TestAWholeRunHandsOverAListAndNamesNoPageBehindIt(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--all")
	expectCode(t, r, ExitOK)
	if string(env.Data) != "[]" {
		t.Errorf("a run that read nothing hands over %s", env.Data)
	}

	e = newTestEnv(t)
	anArchiveOf(e, 10)
	env, r = e.runJSON("", "boards", "archive", e.fixture.Board.ID, "--limit", "10", "--all")
	expectCode(t, r, ExitOK)
	for _, crumb := range env.Breadcrumbs {
		if crumb.Action == "more" || crumb.Action == "all" {
			t.Errorf("a whole run sends a reader after %q: %s", crumb.Action, crumb.Cmd)
		}
	}
	if !slices.ContainsFunc(env.Breadcrumbs, func(b Breadcrumb) bool { return b.Action == "show" }) {
		t.Errorf("breadcrumbs %+v", env.Breadcrumbs)
	}
}

// A page this client will not ask for is nobody's answer but its own refusal of
// the caller, and no command reaches it: it is reported as the usage error it is
// and not as something the server said.
func TestAPageBeyondTheBoundIsReportedAsAUsageError(t *testing.T) {
	s := &session{host: "https://minka.example"}

	e := archiveRefusal(s, api.ErrArchivePageOutOfBounds, "0199")
	if e.Code != CodeUsage || e.ExitCode() != ExitUsage {
		t.Errorf("code %q, exit %d", e.Code, e.ExitCode())
	}
}
