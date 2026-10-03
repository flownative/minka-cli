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
		`{"color":null}`:                     "422 nothingToChange",
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

// A neighbour that is no identifier is 404. One that is no other working
// column of the board is passed over: the next one counts then, and without
// one the column goes first. A column that stands where it is asked to go is
// answered 200 without a write, even in an archived project.
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
		t.Errorf("a column of another board: %s, %v", got, order())
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

	f.Project.Archived = true
	if got := move(shipped, `{}`); got != "200" {
		t.Errorf("a column where it stands, in an archived project: %s", got)
	}
	if got := move(review, `{}`); got != "422 projectArchived" {
		t.Errorf("a move in an archived project: %s", got)
	}
}

// The write is the team's. A client sees the board, so the body is judged for
// a client too, and what Minka tells after a failed write it tells a client as
// well: the archive, and what a system column refuses. Anything else is the
// 404 of an outsider, and what writes nothing answers 200.
func TestTheFakeLetsAClientChangeNoColumn(t *testing.T) {
	s, f := New(t)
	s.Tokens[clientToken] = f.Client.ID
	columns := "/api/v1/boards/" + f.Board.ID + "/columns"
	doing := "/api/v1/columns/" + f.Doing.ID

	for _, c := range []struct{ method, path, body, want string }{
		{"POST", columns, `{"title":"Mine"}`, "404"},
		{"POST", columns, `{"title":""}`, "422 columnTitleMissing"},
		{"PATCH", doing, `{"title":"Mine"}`, "404"},
		{"PATCH", doing, `{"color":"red"}`, "404"},
		{"PATCH", doing, `{"title":"Doing"}`, "200"},
		{"PATCH", "/api/v1/columns/" + f.Done.ID, `{"color":"red"}`, "422 systemColumnHasNoColor"},
		{"POST", doing + "/moves", `{}`, "200"},
		{"POST", "/api/v1/columns/" + f.NotNow.ID + "/moves", `{}`, "422 systemColumnCannotMove"},
		{"DELETE", doing, ``, "404"},
		{"DELETE", "/api/v1/columns/" + f.Triage.ID, ``, "422 systemColumnCannotBeRemoved"},
	} {
		if got := sendAs(t, s, clientToken, c.method, c.path, "application/json", c.body); got != c.want {
			t.Errorf("%s %s %s: %s, want %s", c.method, c.path, c.body, got, c.want)
		}
	}
	if *f.Doing.CustomTitle != "Doing" || f.Doing.Color != nil || len(f.Board.Columns) != 4 {
		t.Errorf("the client changed the board: %v", columnIDs(f.Board))
	}

	f.Project.Archived = true
	if got := sendAs(t, s, clientToken, "PATCH", doing, "application/json", `{"title":"Mine"}`); got != "422 projectArchived" {
		t.Errorf("a client in an archived project: %s", got)
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
