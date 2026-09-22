package cli

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/api"
)

// switchOnHold gives Doing its area for cards on hold, the way a person would.
func switchOnHold(e *testEnv) {
	e.t.Helper()
	expectCode(e.t, e.run("", "columns", "on-hold", e.fixture.Doing.ID, "--json"), ExitOK)
}

// columnOf reads one column of the fixture board as the CLI reads it.
func columnOf(e *testEnv, columnID string) api.Column {
	e.t.Helper()
	env, r := e.runJSON("", "boards", "show", e.fixture.Board.ID)
	expectCode(e.t, r, ExitOK)
	var board api.Board
	if err := json.Unmarshal(env.Data, &board); err != nil {
		e.t.Fatal(err)
	}
	for _, column := range board.Columns {
		if column.ID == columnID {
			return column
		}
	}
	e.t.Fatalf("the board has no column %s", columnID)
	return api.Column{}
}

func cardIDs(cards []api.BoardCard) string {
	ids := make([]string, len(cards))
	for i, card := range cards {
		ids[i] = card.ID
	}
	return strings.Join(ids, ",")
}

func TestColumnsOnHoldSwitchesTheAreaOnByTitle(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "columns", "on-hold", "Doing", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitOK)

	writes := e.server.WritingRequests()
	if len(writes) != 1 || writes[0].Method != http.MethodPut || writes[0].Path != "/api/v1/columns/"+e.fixture.Doing.ID+"/on-hold" {
		t.Fatalf("requests %+v", writes)
	}
	if body := e.lastWriteBody(); len(body) != 1 || body["onHoldEnabled"] != true {
		t.Errorf("body sent %v", body)
	}
	column := decode(t, env.Data)
	if column["id"] != e.fixture.Doing.ID || column["onHoldEnabled"] != true {
		t.Errorf("column %s", env.Data)
	}
	if !strings.Contains(env.Summary, "on") || len(env.Breadcrumbs) == 0 || !strings.Contains(env.Breadcrumbs[0].Cmd, "--on-hold") {
		t.Errorf("summary %q, breadcrumbs %+v", env.Summary, env.Breadcrumbs)
	}
	if !columnOf(e, e.fixture.Doing.ID).OnHoldEnabled {
		t.Error("the board does not show the area")
	}
}

func TestColumnsOnHoldOffHandsTheCardsBackToTheEndOfTheColumn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", f.CardInDoing.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)
	second := columnOf(e, f.Doing.ID).Cards[0].ID

	env, r := e.runJSON("", "columns", "on-hold", f.Doing.ID, "--off")
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["onHoldEnabled"] != false {
		t.Errorf("body sent %v", body)
	}
	var column api.Column
	if err := json.Unmarshal(env.Data, &column); err != nil {
		t.Fatal(err)
	}
	if column.OnHoldEnabled || column.OnHoldCardCount != 0 || len(column.OnHoldCards) != 0 {
		t.Errorf("the area is still there: %s", env.Data)
	}
	if got, want := cardIDs(column.Cards), second+","+f.CardInDoing.ID; got != want || column.CardCount != 2 {
		t.Errorf("cards %s (%d), want %s", got, column.CardCount, want)
	}
	card, _ := e.runJSON("", "cards", "show", f.CardInDoing.ID)
	if decode(t, card.Data)["onHold"] != false {
		t.Errorf("the card is still on hold: %s", card.Data)
	}
}

func TestMoveOnHoldPutsTheCardApartFromTheColumn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)

	env, r := e.runJSON("", "cards", "move", f.CardInTriage.ID, "--column", "Doing", "--on-hold", "--bottom")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if body["onHold"] != true || body["columnId"] != f.Doing.ID || body["position"] != "bottom" {
		t.Errorf("body sent %v", body)
	}
	if decode(t, env.Data)["onHold"] != true || !strings.HasSuffix(env.Summary, "Doing, on hold") {
		t.Errorf("summary %q, card %s", env.Summary, env.Data)
	}

	doing := columnOf(e, f.Doing.ID)
	if doing.CardCount != 2 || doing.OnHoldCardCount != 1 || cardIDs(doing.OnHoldCards) != f.CardInTriage.ID {
		t.Errorf("column %+v", doing)
	}
	if strings.Contains(cardIDs(doing.Cards), f.CardInTriage.ID) {
		t.Error("the card on hold is listed with the rest of the column")
	}
}

func TestMoveOnHoldNextToACardOnHold(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", f.CardInDoing.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)

	_, r := e.runJSON("", "cards", "move", f.CardInTriage.ID, "--column", "Doing", "--on-hold", "--before", f.CardInDoing.ID)
	expectCode(t, r, ExitOK)
	if body := e.lastWriteBody(); body["onHold"] != true || body["nextCardId"] != f.CardInDoing.ID {
		t.Errorf("body sent %v", body)
	}
	if got, want := cardIDs(columnOf(e, f.Doing.ID).OnHoldCards), f.CardInTriage.ID+","+f.CardInDoing.ID; got != want {
		t.Errorf("on hold %s, want %s", got, want)
	}
}

func TestMoveWithoutOnHoldTakesACardOffHold(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", f.CardInDoing.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "move", f.CardInDoing.ID, "--column", "Doing", "--bottom")
	expectCode(t, r, ExitOK)
	if _, sent := e.lastWriteBody()["onHold"]; sent {
		t.Error("an ordinary move sent onHold")
	}
	if decode(t, env.Data)["onHold"] != false || strings.Contains(env.Summary, "on hold") {
		t.Errorf("summary %q, card %s", env.Summary, env.Data)
	}
	if doing := columnOf(e, f.Doing.ID); doing.OnHoldCardCount != 0 || doing.CardCount != 2 {
		t.Errorf("column %+v", doing)
	}
}

// The server answers a move onto hold into a column without the area with a
// 404, which reads as a card that is gone; the CLI says what is missing.
func TestMoveOnHoldIntoAColumnWithoutTheAreaIsRefused(t *testing.T) {
	for _, test := range []struct{ column, hint string }{
		{"Doing", "minka columns on-hold "},
		{"triage", "Only a working column"},
		{"done", "Only a working column"},
	} {
		t.Run(test.column, func(t *testing.T) {
			e := newTestEnv(t)
			env, r := e.runJSON("", "cards", "move", e.fixture.CardInTriage.ID, "--column", test.column, "--on-hold")
			expectCode(t, r, ExitUsage)
			if !strings.Contains(env.Error, "no area for cards on hold") || !strings.Contains(env.Hint, test.hint) {
				t.Errorf("error %q, hint %q", env.Error, env.Hint)
			}
			if len(e.server.WritingRequests()) != 0 {
				t.Error("the card was moved")
			}
		})
	}
}

// The server puts a card whose neighbour stands in the other area at the top
// of its own area without saying so.
func TestMoveNextToACardInTheOtherAreaIsRefused(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", f.CardInDoing.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)
	second := columnOf(e, f.Doing.ID).Cards[0].ID
	writes := len(e.server.WritingRequests())

	_, r := e.runJSON("", "cards", "move", f.CardInTriage.ID, "--column", "Doing", "--on-hold", "--after", second)
	expectCode(t, r, ExitUsage)
	_, r = e.runJSON("", "cards", "move", f.CardInTriage.ID, "--column", "Doing", "--after", f.CardInDoing.ID)
	expectCode(t, r, ExitUsage)

	if len(e.server.WritingRequests()) != writes {
		t.Error("a card was moved")
	}
}

// The server answers a system column like one that does not exist.
func TestColumnsOnHoldRefusesASystemColumn(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "columns", "on-hold", "done", "--board", e.fixture.Board.ID)
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "Only a working column") {
		t.Errorf("hint %q", env.Hint)
	}
	if len(e.server.WritingRequests()) != 0 {
		t.Error("a request to switch was sent")
	}

	env, r = e.runJSON("", "columns", "on-hold", e.fixture.Triage.ID)
	expectCode(t, r, ExitNotFound)
	if env.Meta["apiCode"] != "notFound" || !strings.Contains(env.Hint, "not a working column") {
		t.Errorf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if columnOf(e, e.fixture.Triage.ID).OnHoldEnabled {
		t.Error("Triage got an area on hold")
	}
}

// An archived project says so before a system column answers like one that
// does not exist, as Minka asks after the write was refused.
func TestColumnsOnHoldOfTriageInAnArchivedProjectSaysItIsArchived(t *testing.T) {
	e := newTestEnv(t)
	e.fixture.Project.Archived = true

	env, r := e.runJSON("", "columns", "on-hold", e.fixture.Triage.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "projectArchived" {
		t.Errorf("apiCode %v", env.Meta["apiCode"])
	}
}

func TestColumnsOnHoldShowsTheWayToTheBoard(t *testing.T) {
	for _, args := range [][]string{{}, {"--off"}} {
		e := newTestEnv(t)
		env, r := e.runJSON("", append([]string{"columns", "on-hold", "Doing", "--board", e.fixture.Board.ID}, args...)...)
		expectCode(t, r, ExitOK)
		found := false
		for _, crumb := range env.Breadcrumbs {
			found = found || crumb.Cmd == "minka boards show "+e.fixture.Board.ID
		}
		if !found {
			t.Errorf("%v: breadcrumbs %+v", args, env.Breadcrumbs)
		}
	}
}

func TestColumnsOnHoldOfAForeignColumnIsNotFound(t *testing.T) {
	e := newTestEnv(t)
	foreign := e.fixture.OtherBoard.Columns[0]
	foreign.Type = api.ColumnWork

	_, r := e.runJSON("", "columns", "on-hold", foreign.ID)
	expectCode(t, r, ExitNotFound)
	if foreign.OnHoldEnabled {
		t.Error("a column of another project got an area on hold")
	}
}

func TestColumnsOnHoldNeedsTheBoardForATitle(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "columns", "on-hold", "Doing")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Error, "--board") {
		t.Errorf("error %q", env.Error)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

func TestTheBoardShowsTheCardsOnHoldApart(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", f.CardInTriage.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)

	r := e.run("", "boards", "show", f.Board.ID, "--styled")
	expectCode(t, r, ExitOK)
	held := strings.Index(r.stdout, "### On hold in Doing\n- "+f.CardInTriage.ID)
	if !strings.Contains(r.stdout, "(column, 2 cards, 1 on hold)") || held < 0 {
		t.Fatalf("board:\n%s", r.stdout)
	}
	if strings.Contains(r.stdout[:held], f.CardInTriage.ID) {
		t.Errorf("the card on hold is listed with the rest of the column:\n%s", r.stdout)
	}
	if strings.Count(r.stdout, "### On hold") != 1 {
		t.Errorf("a column without the area shows one:\n%s", r.stdout)
	}

	r = e.run("", "cards", "show", f.CardInTriage.ID, "--styled")
	if !strings.Contains(r.stdout, "Column: Doing (column, "+f.Doing.ID+")\nOn hold: yes\n") {
		t.Errorf("card:\n%s", r.stdout)
	}
}

// sendRaw sends what the CLI never sends, to see the fake answer it the way
// Minka does.
func sendRaw(e *testEnv, method, path, body string) (int, string) {
	e.t.Helper()
	request, _ := http.NewRequest(method, e.server.URL+"/api/v1"+path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+e.fixture.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		e.t.Fatal(err)
	}
	defer response.Body.Close()
	var answer struct {
		Error struct{ Code string } `json:"error"`
	}
	_ = json.NewDecoder(response.Body).Decode(&answer)
	return response.StatusCode, answer.Error.Code
}

func TestTheFakeRefusesAnOnHoldSwitchThatSaysNothing(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	onHold, moves := "/columns/"+f.Doing.ID+"/on-hold", "/cards/"+f.CardInTriage.ID+"/moves"

	if status, code := sendRaw(e, http.MethodPut, onHold, `{}`); status != 422 || code != "onHoldEnabledMissing" {
		t.Errorf("empty body: %d %s", status, code)
	}
	if status, code := sendRaw(e, http.MethodPut, onHold, `{"onHoldEnabled":"yes"}`); status != 400 || code != "malformedBody" {
		t.Errorf("a string for the flag: %d %s", status, code)
	}
	if status, code := sendRaw(e, http.MethodPost, moves, `{"columnId":"`+f.Doing.ID+`","onHold":1}`); status != 400 || code != "malformedBody" {
		t.Errorf("a number for onHold: %d %s", status, code)
	}
	if status, _ := sendRaw(e, http.MethodPost, moves, `{"columnId":"`+f.Doing.ID+`","onHold":true}`); status != 404 {
		t.Errorf("onto hold without the area: %d", status)
	}
}

// A neighbour from the rest of the column is no neighbour of a card on hold,
// so the card lands at the top of the cards on hold.
func TestTheFakePlacesACardOnHoldOnlyAmongTheCardsOnHold(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	second := columnOf(e, f.Doing.ID).Cards[1].ID
	for _, args := range [][]string{
		{"cards", "move", f.CardInDoing.ID, "--column", "Doing", "--on-hold", "--bottom"},
		{"cards", "move", second, "--column", "Doing", "--on-hold", "--bottom"},
		{"cards", "move", f.CardInTriage.ID, "--column", "Doing", "--bottom"},
	} {
		expectCode(t, e.run("", append(args, "--json")...), ExitOK)
	}

	status, _ := sendRaw(e, http.MethodPost, "/cards/"+second+"/moves", `{"columnId":"`+f.Doing.ID+`","onHold":true,"previousCardId":"`+f.CardInTriage.ID+`"}`)
	if status != 200 {
		t.Fatalf("move: %d", status)
	}
	doing := columnOf(e, f.Doing.ID)
	if got, want := cardIDs(doing.OnHoldCards), second+","+f.CardInDoing.ID; got != want {
		t.Errorf("on hold %s, want %s", got, want)
	}
	if cardIDs(doing.Cards) != f.CardInTriage.ID {
		t.Errorf("cards %s", cardIDs(doing.Cards))
	}
}

func TestACardRestoredFromTheTrashComesBackOffHold(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	switchOnHold(e)
	expectCode(t, e.run("", "cards", "move", f.CardInDoing.ID, "--column", "Doing", "--on-hold", "--json"), ExitOK)
	expectCode(t, e.run("", "cards", "delete", f.CardInDoing.ID, "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "restore", f.CardInDoing.ID)
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["onHold"] != false {
		t.Errorf("card %s", env.Data)
	}
	if doing := columnOf(e, f.Doing.ID); doing.OnHoldCardCount != 0 || doing.Cards[0].ID != f.CardInDoing.ID {
		t.Errorf("column %+v", doing)
	}
}
