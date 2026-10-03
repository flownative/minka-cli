package cli

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

// workingColumns names the working columns of the fixture board in their
// order, as the board is read.
func workingColumns(e *testEnv) []string {
	e.t.Helper()
	env, r := e.runJSON("", "boards", "show", e.fixture.Board.ID)
	expectCode(e.t, r, ExitOK)
	var board api.Board
	if err := json.Unmarshal(env.Data, &board); err != nil {
		e.t.Fatal(err)
	}
	var titles []string
	for _, column := range board.Columns {
		if column.Type == api.ColumnWork {
			titles = append(titles, column.Title)
		}
	}
	return titles
}

// expectRefusal holds a refusal of Minka against its code and the hint the
// command gives for it.
func expectRefusal(t *testing.T, env envelope, r result, apiCode, hint string) {
	t.Helper()
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCode {
		t.Errorf("apiCode %v, want %s", env.Meta["apiCode"], apiCode)
	}
	if !strings.Contains(env.Hint, hint) {
		t.Errorf("hint %q does not say %q", env.Hint, hint)
	}
}

func asClient(e *testEnv) {
	e.server.Tokens["minka_pat_client"] = e.fixture.Client.ID
	e.vars["MINKA_TOKEN"] = "minka_pat_client"
}

func TestColumnsCreateAddsAWorkingColumnBehindTheOthers(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "columns", "create", "--board", e.fixture.Board.ID, "--title", "Review")
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if len(writes) != 1 || writes[0].Method != http.MethodPost || writes[0].Path != "/api/v1/boards/"+e.fixture.Board.ID+"/columns" {
		t.Fatalf("requests %+v", writes)
	}
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "Review" {
		t.Errorf("body sent %v", body)
	}
	column := decode(t, env.Data)
	if column["type"] != "column" || column["title"] != "Review" || column["color"] != "red" {
		t.Errorf("column %s", env.Data)
	}
	if env.Summary != "Added column Review in red" {
		t.Errorf("summary %q", env.Summary)
	}
	if got := workingColumns(e); !slices.Equal(got, []string{"Doing", "Review"}) {
		t.Errorf("working columns %v", got)
	}
}

// A title is required by the command, so that a forgotten flag does not reach
// Minka as an empty title.
func TestColumnsCreateNeedsATitleAndABoard(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"columns", "create", "--board", e.fixture.Board.ID},
		{"columns", "create", "--title", "Review"},
		{"columns", "create", "--board", "Doing", "--title", "Review"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

func TestColumnsCreateSaysWhatIsWrongWithTheTitle(t *testing.T) {
	e := newTestEnv(t)
	for title, refusal := range map[string][2]string{
		"":                       {"columnTitleMissing", "needs a title"},
		strings.Repeat("x", 201): {"columnTitleTooLong", "200 characters"},
		"Two\nlines":             {"columnTitleHasControlCharacters", "one line"},
	} {
		env, r := e.runJSON("", "columns", "create", "--board", e.fixture.Board.ID, "--title", title)
		expectRefusal(t, env, r, refusal[0], refusal[1])
	}
	if got := workingColumns(e); !slices.Equal(got, []string{"Doing"}) {
		t.Errorf("working columns %v", got)
	}
}

// A board nobody may see, and a client's write on one they do see, are the
// same 404, and the hint says so.
func TestColumnsCreateOnABoardOfNoTeamOfOneselfIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "columns", "create", "--board", e.fixture.OtherBoard.ID, "--title", "Review")
	expectCode(t, r, ExitNotFound)
	if env.Hint != columnBoardNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}

	asClient(e)
	env, r = e.runJSON("", "columns", "create", "--board", e.fixture.Board.ID, "--title", "Review")
	expectCode(t, r, ExitNotFound)
	if env.Hint != columnBoardNotFoundHint {
		t.Errorf("hint for a client %q", env.Hint)
	}
}

func TestColumnsEditRenamesAndRecoloursWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--title", "In progress")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "In progress" {
		t.Errorf("body sent %v", body)
	}
	if path := e.server.WritingRequests()[0].Path; path != "/api/v1/columns/"+e.fixture.Doing.ID {
		t.Errorf("path %s", path)
	}

	env, r = e.runJSON("", "columns", "edit", "In progress", "--board", e.fixture.Board.ID, "--color", "purple")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); len(body) != 1 || body["color"] != "purple" {
		t.Errorf("body sent %v", body)
	}
	if column := decode(t, env.Data); column["title"] != "In progress" || column["color"] != "purple" {
		t.Errorf("column %s", env.Data)
	}
	if env.Summary != "Changed column "+e.fixture.Doing.ID+": In progress, colour purple" {
		t.Errorf("summary %q", env.Summary)
	}
}

// An empty title is sent as it is: Triage takes it as the name of its type
// again, a working column refuses it.
func TestColumnsEditGivesTriageItsNameBackAndAWorkingColumnNone(t *testing.T) {
	e := newTestEnv(t)

	_, r := e.runJSON("", "columns", "edit", "triage", "--board", e.fixture.Board.ID, "--title", "Inbox")
	expectCode(t, r, ExitOK)
	env, r := e.runJSON("", "columns", "edit", "Inbox", "--board", e.fixture.Board.ID, "--title", "")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["title"] != "" {
		t.Errorf("body sent %v", body)
	}
	if column := decode(t, env.Data); column["title"] != "Triage" || column["customTitle"] != nil {
		t.Errorf("column %s", env.Data)
	}

	env, r = e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--title", "")
	expectRefusal(t, env, r, "columnTitleMissing", "Only Triage, Not now and Done take an empty one")
}

func TestColumnsEditNamesTheColoursMinkaTakes(t *testing.T) {
	e := newTestEnv(t)
	for _, color := range []string{"teal", "", "Red"} {
		env, r := e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--color", color)
		expectRefusal(t, env, r, "columnColorUnknown", strings.Join(minkaColors, ", "))
	}
	if e.fixture.Doing.Color != nil {
		t.Errorf("Doing got the colour %s", *e.fixture.Doing.Color)
	}
}

// Done refuses a colour, and the title sent with it is not written either.
func TestColumnsEditOfDoneWithAColourChangesNothing(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "columns", "edit", "done", "--board", e.fixture.Board.ID, "--title", "Finished", "--color", "red")
	expectRefusal(t, env, r, "systemColumnHasNoColor", "Nothing was changed, the title included")
	if e.fixture.Done.CustomTitle != nil {
		t.Errorf("Done was renamed to %s", *e.fixture.Done.CustomTitle)
	}
}

func TestColumnsEditNeedsSomethingToChangeAndAnIDWithoutABoard(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"columns", "edit", e.fixture.Doing.ID},
		{"columns", "edit", "Doing", "--title", "In progress"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

// A column nobody may see, one in the trash and a client's write all answer
// as an address that names nothing, and the hint names each of them.
func TestColumnsEditOfAColumnOneMayNotChangeIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	for _, columnID := range []string{e.fixture.OtherBoard.Columns[0].ID, unknownID} {
		env, r := e.runJSON("", "columns", "edit", columnID, "--title", "Mine")
		expectCode(t, r, ExitNotFound)
		if env.Hint != columnNotFoundHint {
			t.Errorf("hint %q", env.Hint)
		}
	}

	asClient(e)
	env, r := e.runJSON("", "columns", "edit", e.fixture.Doing.ID, "--title", "Mine")
	expectCode(t, r, ExitNotFound)
	if env.Hint != columnNotFoundHint {
		t.Errorf("hint for a client %q", env.Hint)
	}
	if *e.fixture.Doing.CustomTitle != "Doing" {
		t.Error("the client renamed the column")
	}
}

func TestColumnsMovePutsAColumnBetweenItsNeighbours(t *testing.T) {
	e := newTestEnv(t)
	withAReviewColumn(e)
	expectCode(t, e.run("", "columns", "create", "--board", e.fixture.Board.ID, "--title", "Shipped", "--json"), ExitOK)
	board := e.fixture.Board.ID
	shipped := e.fixture.Board.Columns[3].ID

	for _, c := range []struct {
		args []string
		body map[string]any
		want []string
	}{
		{[]string{"Doing", "--after", "Shipped"}, map[string]any{"previousColumnId": shipped}, []string{"Review", "Shipped", "Doing"}},
		{[]string{"Doing", "--before", "Shipped"}, map[string]any{"nextColumnId": shipped}, []string{"Review", "Doing", "Shipped"}},
		{[]string{"Shipped", "--first"}, map[string]any{}, []string{"Shipped", "Review", "Doing"}},
	} {
		env, r := e.runJSON("", append([]string{"columns", "move", "--board", board}, c.args...)...)
		expectCode(t, r, ExitOK)
		writes := e.server.WritingRequests()
		if last := writes[len(writes)-1]; last.Method != http.MethodPost || !strings.HasSuffix(last.Path, "/moves") {
			t.Errorf("%v: request %s %s", c.args, last.Method, last.Path)
		}
		if body := e.lastWriteBody(); len(body) != len(c.body) || body["previousColumnId"] != c.body["previousColumnId"] || body["nextColumnId"] != c.body["nextColumnId"] {
			t.Errorf("%v: body sent %v, want %v", c.args, body, c.body)
		}
		if got := workingColumns(e); !slices.Equal(got, c.want) {
			t.Errorf("%v: working columns %v, want %v", c.args, got, c.want)
		}
		if !strings.HasPrefix(env.Summary, "Moved column ") {
			t.Errorf("summary %q", env.Summary)
		}
	}
}

// Minka places a column next to anything that is no other working column of
// the board as though it had been put first, so such a neighbour is refused
// before anything moves.
func TestColumnsMoveRefusesANeighbourMinkaWouldPassOver(t *testing.T) {
	e := newTestEnv(t)
	withAReviewColumn(e)
	writes := len(e.server.WritingRequests())

	for _, neighbour := range []string{"triage", "not_now", "Done", "Review", e.fixture.Board.Columns[2].ID} {
		env, r := e.runJSON("", "columns", "move", "Review", "--board", e.fixture.Board.ID, "--before", neighbour)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "no other working column") {
			t.Errorf("%s: error %q", neighbour, env.Error)
		}
		// The hint leads to the two places a misplaced neighbour usually
		// means: the front, and the end behind the last other column.
		if !strings.Contains(env.Hint, "--first") || !strings.Contains(env.Hint, `--after "Doing" (`+e.fixture.Doing.ID+`)`) {
			t.Errorf("%s: hint %q", neighbour, env.Hint)
		}
	}
	_, r := e.runJSON("", "columns", "move", "Review", "--board", e.fixture.Board.ID, "--after", "Nowhere")
	expectCode(t, r, ExitUsage)
	if len(e.server.WritingRequests()) != writes {
		t.Error("a move was sent")
	}
	if got := workingColumns(e); !slices.Equal(got, []string{"Doing", "Review"}) {
		t.Errorf("working columns %v", got)
	}
}

func TestColumnsMoveNeedsExactlyOnePlaceAndTheBoard(t *testing.T) {
	e := newTestEnv(t)
	board := e.fixture.Board.ID
	for _, args := range [][]string{
		{"Doing", "--board", board},
		{"Doing", "--board", board, "--first", "--after", "Doing"},
		{e.fixture.Doing.ID, "--first"},
		{"Doing", "--board", "Board", "--first"},
	} {
		_, r := e.runJSON("", append([]string{"columns", "move"}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

func TestColumnsMoveOfASystemColumnIsRefusedByMinka(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "columns", "move", "not_now", "--board", e.fixture.Board.ID, "--first")
	expectRefusal(t, env, r, "systemColumnCannotMove", columnSystemColumnsKeepTheirPlaces)
}

func TestColumnsDeleteTakesTheColumnAndItsCardsIntoTheTrash(t *testing.T) {
	e := newTestEnv(t)
	trashed := e.fixture.CardInDoing.ID
	card := e.fixture.Doing.Cards[1].ID
	expectCode(t, e.run("", "cards", "delete", trashed, "--json"), ExitOK)

	env, r := e.runJSON("", "columns", "delete", "Doing", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodDelete || last.Path != "/api/v1/columns/"+e.fixture.Doing.ID {
		t.Errorf("request %s %s", last.Method, last.Path)
	}
	if data := decode(t, env.Data); data["id"] != e.fixture.Doing.ID || data["deleted"] != true {
		t.Errorf("data %s", env.Data)
	}
	if env.Summary != "Moved column Doing to the trash, with its 1 card" {
		t.Errorf("summary %q", env.Summary)
	}
	if got := workingColumns(e); len(got) != 0 {
		t.Errorf("working columns %v", got)
	}
	_, r = e.runJSON("", "cards", "show", card)
	expectCode(t, r, ExitNotFound)

	// The card that lay in the trash before can come back only after its
	// column, which the API does not bring back.
	env, r = e.runJSON("", "trash", "list", e.fixture.Project.ID)
	expectCode(t, r, ExitOK)
	var entries []api.TrashEntry
	if err := json.Unmarshal(env.Data, &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Kind != api.TrashColumn || entries[0].ID != e.fixture.Doing.ID || entries[0].Title != "Doing" || entries[0].Restorable {
		t.Fatalf("trash %s", env.Data)
	}
	if entries[1].ID != trashed || entries[1].Restorable || deref(entries[1].BlockedBy) != "column" {
		t.Errorf("the card in the trash %s", env.Data)
	}
	_, r = e.runJSON("", "cards", "restore", trashed)
	expectCode(t, r, ExitNotFound)

	env, r = e.runJSON("", "columns", "delete", e.fixture.Doing.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != columnNotFoundHint {
		t.Errorf("hint %q", env.Hint)
	}
}

// The cards on hold go into the trash with the column as well, and the summary
// counts them.
func TestColumnsDeleteCountsTheCardsOnHold(t *testing.T) {
	e := newTestEnv(t)
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", e.fixture.CardInDoing.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)

	env, r := e.runJSON("", "columns", "delete", "Doing", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)
	if env.Summary != "Moved column Doing to the trash, with its 2 cards" {
		t.Errorf("summary %q", env.Summary)
	}
}

func TestColumnsDeleteOfASystemColumnIsRefusedByMinka(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "columns", "delete", e.fixture.Triage.ID)
	expectRefusal(t, env, r, "systemColumnCannotBeRemoved", columnSystemColumnsBelongToTheBoard)
	if len(e.fixture.Board.Columns) != 4 {
		t.Error("a column left the board")
	}
}

// A column named by its title needs the board, and the column has to be on
// it; neither costs a write.
func TestColumnsDeleteFindsTheColumnOnlyOnTheBoardNamed(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		{"columns", "delete", "Doing"},
		{"columns", "delete", "Nowhere", "--board", e.fixture.Board.ID},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	env, r := e.runJSON("", "columns", "delete", "Doing", "--board", e.fixture.OtherBoard.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != columnBoardNotReadHint {
		t.Errorf("hint %q", env.Hint)
	}
	if len(e.server.WritingRequests()) != 0 {
		t.Error("a write was sent")
	}

	// A client reads the board; it is the write that is refused, and the
	// hint says so of the column.
	asClient(e)
	env, r = e.runJSON("", "columns", "delete", "Doing", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitNotFound)
	if env.Hint != columnNotFoundHint {
		t.Errorf("hint for a client %q", env.Hint)
	}
}

// The colour of a column reaches the board as Minka draws it: Done green,
// Triage and Not now without one.
func TestTheBoardCarriesTheColourOfEveryColumn(t *testing.T) {
	e := newTestEnv(t)
	expectCode(t, e.run("", "columns", "edit", e.fixture.Doing.ID, "--color", "aqua", "--json"), ExitOK)
	colors := map[string]*string{}
	for _, column := range []*fakeapi.Column{e.fixture.Triage, e.fixture.Doing, e.fixture.NotNow, e.fixture.Done} {
		colors[column.Type] = columnOf(e, column.ID).Color
	}
	if colors["triage"] != nil || colors["not_now"] != nil || deref(colors["column"]) != "aqua" || deref(colors["done"]) != "green" {
		t.Errorf("colours %v", colors)
	}
}
