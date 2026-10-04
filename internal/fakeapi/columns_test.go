package fakeapi

import (
	"slices"
	"strings"
	"testing"
)

const clientToken = "minka_pat_client"

func columnIDs(board *Board) []string {
	var ids []string
	for _, c := range board.Columns {
		ids = append(ids, c.Type+":"+columnTitle(c))
	}
	return ids
}

func addColumn(t *testing.T, s *Server, f *Fixture, title string) *Column {
	t.Helper()
	if got := sendAs(t, s, f.Token, "POST", "/api/v1/boards/"+f.Board.ID+"/columns", "application/json", `{"title":"`+title+`"}`); got != "201" {
		t.Fatalf("adding %s: %s", title, got)
	}
	for _, c := range f.Board.Columns {
		if c.CustomTitle != nil && *c.CustomTitle == title {
			return c
		}
	}
	t.Fatalf("no column %s on the board", title)
	return nil
}

// A new column stands behind the working columns, before Not now and Done, in
// the colour the board holds least of, the earliest of the palette among
// equals and never white.
func TestTheFakeAddsAColumnBehindTheWorkingColumnsInTheRarestColour(t *testing.T) {
	s, f := New(t)
	red, white := "red", "white"
	f.Doing.Color = &red
	review := addColumn(t, s, f, "Review")
	if review.Color == nil || *review.Color != "orange" {
		t.Errorf("colour %v, want orange", review.Color)
	}
	review.Color = &white
	shipped := addColumn(t, s, f, "Shipped")
	if *shipped.Color != "orange" {
		t.Errorf("colour %v, want orange: white is not counted", *shipped.Color)
	}
	want := []string{"triage:Triage", "column:Doing", "column:Review", "column:Shipped", "not_now:Not now", "done:Done"}
	if got := columnIDs(f.Board); !slices.Equal(got, want) {
		t.Errorf("columns %v, want %v", got, want)
	}
}

// Every field is judged before anything is written, the title before the
// colour, and a body that names neither is refused as such. An absent title
// and a null one are not the same: null names the title, and a working column
// cannot go without one.
func TestTheFakeJudgesAColumnEditBeforeItWrites(t *testing.T) {
	s, f := New(t)
	path := "/api/v1/columns/" + f.Doing.ID
	for body, want := range map[string]string{
		`{}`:                                 "422 nothingToChange",
		`{"title":null}`:                     "422 columnTitleMissing",
		`{"title":"  ","color":"teal"}`:      "422 columnTitleMissing",
		`{"title":"Two\nlines"}`:             "422 columnTitleHasControlCharacters",
		`{"color":""}`:                       "422 columnColorUnknown",
		`{"color":"Red"}`:                    "422 columnColorUnknown",
		`{"title":1}`:                        "400 malformedBody",
		`{"title":"Fine","color":"teal"}`:    "422 columnColorUnknown",
		`{"title":"` + longTitle() + `"}`:    "422 columnTitleTooLong",
		`{"title":" ` + titleOf(200) + ` "}`: "200",
	} {
		before := *f.Doing.CustomTitle
		if got := sendAs(t, s, f.Token, "PATCH", path, "application/json", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
		if want != "200" && *f.Doing.CustomTitle != before {
			t.Errorf("%s renamed the column to %s", body, *f.Doing.CustomTitle)
		}
	}
}

func titleOf(n int) string {
	title := ""
	for range n {
		title += "ä"
	}
	return title
}

func longTitle() string { return titleOf(201) }

// Triage, Not now and Done take a title and give it back on an empty one or
// null; a colour for them is refused before the title is written.
func TestTheFakeLetsASystemColumnGoByItsTypeAgainAndRefusesItAColour(t *testing.T) {
	s, f := New(t)
	path := "/api/v1/columns/" + f.Triage.ID
	for _, reset := range []string{`""`, `null`, `"  "`} {
		if got := sendAs(t, s, f.Token, "PATCH", path, "application/json", `{"title":"Inbox"}`); got != "200" || columnTitle(f.Triage) != "Inbox" {
			t.Fatalf("renaming Triage: %s, %s", got, columnTitle(f.Triage))
		}
		if got := sendAs(t, s, f.Token, "PATCH", path, "application/json", `{"title":`+reset+`}`); got != "200" || f.Triage.CustomTitle != nil {
			t.Errorf("%s: %s, title %v", reset, got, f.Triage.CustomTitle)
		}
	}
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/columns/"+f.Done.ID, "application/json", `{"title":"Finished","color":"red"}`); got != "422 systemColumnHasNoColor" {
		t.Errorf("colouring Done: %s", got)
	}
	if f.Done.CustomTitle != nil || f.Done.Color != nil {
		t.Errorf("Done changed: %v, %v", f.Done.CustomTitle, f.Done.Color)
	}
}

// A null colour makes a working column white, which is how a column without a
// colour looks; Triage, Not now and Done refuse it as any colour, after the
// archive, and keep the title sent with it.
func TestTheFakeTakesAColourAwayWithNull(t *testing.T) {
	s, f := New(t)
	red := "red"
	f.Doing.Color = &red
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/columns/"+f.Doing.ID, "application/json", `{"color":null}`); got != "200" || deref(f.Doing.Color) != "white" {
		t.Errorf("taking the colour off Doing: %s, %v", got, deref(f.Doing.Color))
	}
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/columns/"+f.Doing.ID, "application/json", `{"title":"Busy"}`); got != "200" || deref(f.Doing.Color) != "white" {
		t.Errorf("a rename without a colour: %s, %v", got, deref(f.Doing.Color))
	}
	for _, column := range []*Column{f.Triage, f.NotNow, f.Done} {
		if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/columns/"+column.ID, "application/json", `{"title":"Mine","color":null}`); got != "422 systemColumnHasNoColor" {
			t.Errorf("taking the colour off %s: %s", column.Type, got)
		}
		if column.CustomTitle != nil || column.Color != nil {
			t.Errorf("%s changed: %v, %v", column.Type, column.CustomTitle, column.Color)
		}
	}

	f.Project.Archived = true
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/columns/"+f.Done.ID, "application/json", `{"color":null}`); got != "422 projectArchived" {
		t.Errorf("taking the colour off Done in an archived project: %s", got)
	}
}

// A neighbour that is no identifier is 404, and one on another board is
// refused. One out of sight, a system column of the board and the column
// itself are passed over: the next one counts then, and without one the
// column goes first. A column that stands where it is asked to go is answered
// 200 without a write while the project is in use.
func TestTheFakeMovesAColumnByItsWorkingNeighbours(t *testing.T) {
	s, f := New(t)
	review := addColumn(t, s, f, "Review")
	shipped := addColumn(t, s, f, "Shipped")
	move := func(column *Column, body string) string {
		return sendAs(t, s, f.Token, "POST", "/api/v1/columns/"+column.ID+"/moves", "application/json", body)
	}
	order := func() []string {
		var titles []string
		for _, c := range f.Board.Columns {
			if c.Type == "column" {
				titles = append(titles, columnTitle(c))
			}
		}
		return titles
	}

	if got := move(f.Doing, `{"nextColumnId":"nonsense"}`); got != "404" {
		t.Errorf("a neighbour that is no identifier: %s", got)
	}
	if got := move(f.Doing, `{"previousColumnId":"`+shipped.ID+`"}`); got != "200" || !slices.Equal(order(), []string{"Review", "Shipped", "Doing"}) {
		t.Errorf("behind Shipped: %s, %v", got, order())
	}
	if got := move(f.Doing, `{"previousColumnId":"`+f.NotNow.ID+`","nextColumnId":"`+shipped.ID+`"}`); got != "200" || !slices.Equal(order(), []string{"Review", "Doing", "Shipped"}) {
		t.Errorf("a system column before Shipped: %s, %v", got, order())
	}
	if got := move(review, `{"previousColumnId":"`+f.OtherBoard.Columns[0].ID+`"}`); got != "200" || !slices.Equal(order(), []string{"Review", "Doing", "Shipped"}) {
		t.Errorf("a column out of sight: %s, %v", got, order())
	}
	if got := move(review, `{"nextColumnId":"`+review.ID+`"}`); got != "200" || !slices.Equal(order(), []string{"Review", "Doing", "Shipped"}) {
		t.Errorf("the column itself: %s, %v", got, order())
	}
	second := &Board{ID: newID(), ProjectID: f.Project.ID, Title: "Second", Columns: []*Column{{ID: newID(), Type: "triage"}, {ID: newID(), Type: "column"}}}
	s.Boards = append(s.Boards, second)
	for _, neighbour := range second.Columns {
		for _, body := range []string{`{"previousColumnId":"` + neighbour.ID + `"}`, `{"previousColumnId":"` + shipped.ID + `","nextColumnId":"` + strings.ToUpper(neighbour.ID) + `"}`} {
			if got := move(review, body); got != "422 neighbourColumnOnAnotherBoard" || !slices.Equal(order(), []string{"Review", "Doing", "Shipped"}) {
				t.Errorf("%s on another board: %s, %v", body, got, order())
			}
		}
	}
	// Minka reads a neighbour in any case.
	if got := move(review, `{"previousColumnId":"`+strings.ToUpper(shipped.ID)+`"}`); got != "200" || !slices.Equal(order(), []string{"Doing", "Shipped", "Review"}) {
		t.Errorf("behind Shipped named in capitals: %s, %v", got, order())
	}
	if got := move(review, `{"nextColumnId":"`+strings.ToUpper(f.Doing.ID)+`"}`); got != "200" || !slices.Equal(order(), []string{"Review", "Doing", "Shipped"}) {
		t.Errorf("before Doing named in capitals: %s, %v", got, order())
	}
	if got := move(shipped, `{}`); got != "200" || !slices.Equal(order(), []string{"Shipped", "Review", "Doing"}) {
		t.Errorf("to the front: %s, %v", got, order())
	}
	if got := move(f.NotNow, `{"nextColumnId":"`+f.Doing.ID+`"}`); got != "422 systemColumnCannotMove" {
		t.Errorf("moving Not now: %s", got)
	}

	// The neighbour on another board is told before the archive and before
	// what a system column refuses.
	if got := move(f.NotNow, `{"nextColumnId":"`+second.Columns[1].ID+`"}`); got != "422 neighbourColumnOnAnotherBoard" {
		t.Errorf("moving Not now next to another board: %s", got)
	}
	f.Project.Archived = true
	if got := move(review, `{"nextColumnId":"`+second.Columns[1].ID+`"}`); got != "422 neighbourColumnOnAnotherBoard" {
		t.Errorf("a neighbour on another board, in an archived project: %s", got)
	}
	if got := move(shipped, `{}`); got != "422 projectArchived" {
		t.Errorf("a column where it stands, in an archived project: %s", got)
	}
	if got := move(review, `{}`); got != "422 projectArchived" {
		t.Errorf("a move in an archived project: %s", got)
	}
}

// The write is the team's. Whatever a column write names, a client of the
// project hears what it would hear of a column that does not exist: nothing
// of a field, a system column, an archived project or a write that would
// change nothing.
func TestTheFakeAnswersAClientLikeAColumnThatDoesNotExist(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	columns := "/api/v1/boards/" + f.Board.ID + "/columns"
	doing := "/api/v1/columns/" + f.Doing.ID
	writes := []struct{ method, path, body string }{
		{"POST", columns, `{"title":"Mine"}`},
		{"POST", columns, `{"title":""}`},
		{"POST", columns, `{"title":1}`},
		{"PATCH", doing, `{"title":"Mine"}`},
		{"PATCH", doing, `{"title":"Doing"}`},
		{"PATCH", doing, `{"title":""}`},
		{"PATCH", doing, `{"color":"red"}`},
		{"PATCH", doing, `{"color":"teal"}`},
		{"PATCH", doing, `{"color":null}`},
		{"PATCH", doing, `{}`},
		{"PATCH", "/api/v1/columns/" + f.Done.ID, `{"color":"red"}`},
		{"POST", doing + "/moves", `{}`},
		{"POST", doing + "/moves", `{"previousColumnId":"` + f.OtherBoard.Columns[0].ID + `"}`},
		{"POST", "/api/v1/columns/" + f.NotNow.ID + "/moves", `{}`},
		{"DELETE", doing, ``},
		{"DELETE", "/api/v1/columns/" + f.Triage.ID, ``},
		{"PUT", "/api/v1/columns/" + f.Triage.ID + "/on-hold", `{"onHoldEnabled":true}`},
		{"PUT", doing + "/on-hold", `{}`},
	}

	for _, archived := range []bool{false, true} {
		f.Project.Archived = archived
		for _, c := range writes {
			if got := sendAs(t, s, clientToken, c.method, c.path, "application/json", c.body); got != "404" {
				t.Errorf("%s %s %s, archived %v: %s, want 404", c.method, c.path, c.body, archived, got)
			}
		}
	}
	if *f.Doing.CustomTitle != "Doing" || f.Doing.Color != nil || f.Doing.OnHoldEnabled || len(f.Board.Columns) != 4 {
		t.Errorf("the client changed the board: %v", columnIDs(f.Board))
	}
}

// In an archived project the title is judged first, and the same title again
// is a write all the same, refused as one: Minka finds no unchanged title in a
// project that is not in use.
func TestTheFakeTellsTheArchiveAfterTheValues(t *testing.T) {
	s, f := New(t)
	f.Project.Archived = true
	doing := "/api/v1/columns/" + f.Doing.ID
	for body, want := range map[string]string{
		`{"title":""}`:      "422 columnTitleMissing",
		`{"color":"teal"}`:  "422 columnColorUnknown",
		`{"title":"Doing"}`: "422 projectArchived",
		`{"color":"red"}`:   "422 projectArchived",
	} {
		if got := sendAs(t, s, f.Token, "PATCH", doing, "application/json", body); got != want {
			t.Errorf("%s: %s, want %s", body, got, want)
		}
	}
	if got := sendAs(t, s, f.Token, "PATCH", "/api/v1/columns/"+f.Done.ID, "application/json", `{"color":"red"}`); got != "422 projectArchived" {
		t.Errorf("colouring Done in an archived project: %s", got)
	}
	if got := sendAs(t, s, f.Token, "POST", "/api/v1/boards/"+f.Board.ID+"/columns", "application/json", `{"title":"New"}`); got != "422 projectArchived" {
		t.Errorf("adding a column in an archived project: %s", got)
	}
}

// A column in the trash takes its cards with it: they are at no address, a
// card of it in the trash cannot come back before it, and a card of it in the
// archive comes back into the first working column.
func TestTheFakeTakesTheCardsAwayWithTheirColumn(t *testing.T) {
	s, f := New(t)
	review := addColumn(t, s, f, "Review")
	onTheColumn := &Card{ID: newID(), Title: "On the column", CreatedAt: s.now, CreatorID: f.Bruno.ID}
	inTheTrash := &Card{ID: newID(), Title: "In the trash", CreatedAt: s.now, CreatorID: f.Bruno.ID}
	inTheArchive := &Card{ID: newID(), Title: "In the archive", CreatedAt: s.now, CreatorID: f.Bruno.ID}
	review.Cards = []*Card{onTheColumn, inTheTrash, inTheArchive}
	s.PutInTheArchive(inTheArchive)
	if got := sendAs(t, s, f.Token, "DELETE", "/api/v1/cards/"+inTheTrash.ID, "application/json", ``); got != "204" {
		t.Fatalf("trashing the card: %s", got)
	}

	if got := sendAs(t, s, f.Token, "DELETE", "/api/v1/columns/"+review.ID, "application/json", ``); got != "204" {
		t.Fatalf("trashing the column: %s", got)
	}
	if got := sendAs(t, s, f.Token, "DELETE", "/api/v1/columns/"+review.ID, "application/json", ``); got != "404" {
		t.Errorf("trashing it again: %s", got)
	}
	if got := sendAs(t, s, f.Token, "GET", "/api/v1/cards/"+onTheColumn.ID, "application/json", ``); got != "404" {
		t.Errorf("a card of the column: %s", got)
	}
	if got := sendAs(t, s, f.Token, "POST", "/api/v1/cards/"+inTheTrash.ID+"/restore", "application/json", ``); got != "404" {
		t.Errorf("restoring a card of the column: %s", got)
	}
	if got := sendAs(t, s, f.Token, "GET", "/api/v1/cards/"+inTheArchive.ID, "application/json", ``); got != "200" {
		t.Errorf("a card of the column in the archive: %s", got)
	}
	if got := sendAs(t, s, f.Token, "POST", "/api/v1/cards/"+inTheArchive.ID+"/unarchive", "application/json", ``); got != "200" || f.Doing.Cards[0] != inTheArchive {
		t.Errorf("unarchiving it: %s, first in Doing %s", got, f.Doing.Cards[0].Title)
	}
}
