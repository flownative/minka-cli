package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

// --board and --column narrow the cards on the server, each by itself and as
// an ID in lowercase; a column named by its title is looked up on the board
// given with --board first.
func TestMyCardsNarrowToABoardAndAColumn(t *testing.T) {
	e := newTestEnv(t)
	fixture := e.fixture
	fixture.CardInTriage.AssigneeIDs = []string{fixture.Bruno.ID}
	fixture.CardInDoing.AssigneeIDs = []string{fixture.Bruno.ID}
	titles := func(args ...string) string {
		t.Helper()
		env, r := e.runJSON("", append([]string{"cards", "mine"}, args...)...)
		expectCode(t, r, ExitOK)
		var cards []api.Card
		if err := json.Unmarshal(env.Data, &cards); err != nil {
			t.Fatalf("data %s", env.Data)
		}
		var found []string
		for _, card := range cards {
			found = append(found, card.Title)
		}
		return strings.Join(found, ", ")
	}
	lastQuery := func() string {
		requests := e.server.Requests()
		return requests[len(requests)-1].Query
	}

	if got := titles("--column", strings.ToUpper(fixture.Doing.ID)); got != fixture.CardInDoing.Title || lastQuery() != "columnId="+fixture.Doing.ID {
		t.Errorf("--column: %q, asked %q", got, lastQuery())
	}
	if got := titles("--board", strings.ToUpper(fixture.Board.ID)); got != "Review the API, Write the CLI" || lastQuery() != "boardId="+fixture.Board.ID {
		t.Errorf("--board: %q, asked %q", got, lastQuery())
	}
	if got := titles("--board", fixture.Board.ID, "--column", "doing"); got != fixture.CardInDoing.Title || lastQuery() != "boardId="+fixture.Board.ID+"&columnId="+fixture.Doing.ID {
		t.Errorf("--column by title: %q, asked %q", got, lastQuery())
	}
	if got := titles("--board", fixture.OtherBoard.ID); got != "" {
		t.Errorf("another board: %q", got)
	}
}

// The safeguard: what is no ID never reaches the query, and a column named by
// its title without the board to look it up on is refused, before any request.
func TestMyCardsRefuseWhatIsNoIDBeforeARequest(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"--board", "nonsense"},
		{"--board", ""},
		{"--column", "Doing"},
		{"--column", ""},
		{"--board", e.fixture.Board.ID, "--column", " "},
	} {
		_, r := e.runJSON("", append([]string{"cards", "mine"}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if requests := e.server.Requests(); len(requests) != 0 {
		t.Fatalf("%d requests went out, want none", len(requests))
	}
}

const (
	boardOfMine    = "0192f3a4-5b6c-4d7e-8f90-00000000000b"
	readyColumn    = "0192f3a4-5b6c-4d7e-8f90-0000000000c1"
	anotherColumn  = "0192f3a4-5b6c-4d7e-8f90-0000000000c2"
	anotherBoardID = "0192f3a4-5b6c-4d7e-8f90-00000000000a"
)

func cardAt(id, boardID, columnID string) string {
	return `{"id":"` + id + `","title":"` + id + `","boardId":"` + boardID + `","column":{"id":"` + columnID + `","type":"column","title":"C"}}`
}

// The safeguard: a Minka that ignores the filters sends every card, and the
// answer is narrowed here all the same, so no card from elsewhere is handed
// over; the IDs are compared in any case.
func TestMyCardsAreNarrowedAgainstAMinkaThatIgnoresTheFilters(t *testing.T) {
	e := newTestEnv(t)
	all := "200 [" + cardAt("released", strings.ToUpper(boardOfMine), readyColumn) + "," + cardAt("intake", boardOfMine, anotherColumn) + "," + cardAt("elsewhere", anotherBoardID, readyColumn) + "]"
	e.answeredBy(&bell{answers: []string{all, all, all}})
	for _, run := range []struct {
		args []string
		want string
	}{
		{[]string{"--board", boardOfMine, "--column", readyColumn}, "released"},
		{[]string{"--board", boardOfMine}, "released, intake"},
		{[]string{"--column", strings.ToUpper(readyColumn)}, "released, elsewhere"},
	} {
		env, r := e.runJSON("", append([]string{"cards", "mine"}, run.args...)...)
		expectCode(t, r, ExitOK)
		var cards []map[string]any
		if err := json.Unmarshal(env.Data, &cards); err != nil {
			t.Fatalf("data %s", env.Data)
		}
		var ids []string
		for _, card := range cards {
			ids = append(ids, card["id"].(string))
		}
		if strings.Join(ids, ", ") != run.want || env.Summary != plural(len(ids), "card", "cards") {
			t.Errorf("%v: %v, %q, want %s", run.args, ids, env.Summary, run.want)
		}
	}
}

// The safeguard: where a card does not say on which board or in which column
// it stands, the filters cannot be held to and are refused, never answered with
// every card. Without a filter the list is handed over as it came.
func TestMyCardsRefuseTheFiltersWhereACardDoesNotSayWhereItStands(t *testing.T) {
	e := newTestEnv(t)
	noBoard := `200 [{"id":"a","title":"a","column":{"id":"` + readyColumn + `"}}]`
	noColumn := `200 [{"id":"b","title":"b","boardId":"` + boardOfMine + `"}]`
	e.answeredBy(&bell{answers: []string{noBoard, noColumn, noBoard}})

	env, r := e.runJSON("", "cards", "mine", "--board", boardOfMine)
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI || !strings.Contains(env.Hint, "without --board and --column") || (len(env.Data) > 0 && string(env.Data) != "null") {
		t.Fatalf("envelope %+v", env)
	}
	_, r = e.runJSON("", "cards", "mine", "--column", readyColumn)
	expectCode(t, r, ExitAPI)

	env, r = e.runJSON("", "cards", "mine")
	expectCode(t, r, ExitOK)
	if env.Summary != "1 card" {
		t.Fatalf("without a filter: %q", env.Summary)
	}
}
