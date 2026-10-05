package basecamp_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// aTitledRecord is a record of the source whose title or name a test gives a
// value Minka would refuse.
type aTitledRecord struct {
	record  string
	id      int64
	project string
	kind    bundle.Kind
	field   string
	none    string
	set     func(*testing.T, *fakebasecamp.Account, string)
}

func titledRecords() []aTitledRecord {
	return []aTitledRecord{
		{"to-do list", 9401, todoProject, bundle.TodoLists, "title", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { theList(a, 9401).Title = v }},
		{"group", 9411, todoProject, bundle.TodoGroups, "title", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { theList(a, 9411).Title = v }},
		{"to-do", 9421, todoProject, bundle.Todos, "title", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { theTodo(a, 9421).Title = v }},
		{"question", 9310, checkinProject, bundle.Questions, "title", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { busyQuestion(a).Title = v }},
		{"folder", 9220, docsProject, bundle.Folders, "title", "(no title)",
			func(t *testing.T, a *fakebasecamp.Account, v string) { theFolder(t, a, 9220).Title = v }},
		{"document", 9244, docsProject, bundle.Documents, "title", "(no title)",
			func(t *testing.T, a *fakebasecamp.Account, v string) { theDocument(t, a, 9244).Title = v }},
		{"card", 9201, boardProject, bundle.Cards, "title", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { roadmap(a).Lists[2].Cards[0].Title = v }},
		{"column", 9104, boardProject, bundle.Columns, "name", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { roadmap(a).Lists[2].Title = v }},
		{"tool", 9020, boardProject, bundle.Tools, "name", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { roadmap(a).Title = v }},
		{"project", 7003, boardProject, bundle.Projects, "name", "(no name)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { a.Projects[2].Name = v }},
		{"post", 9640, talkProject, bundle.Posts, "title", "(no title)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { pitches(a).Posts[0].Subject = v }},
		{"category", 9603, talkProject, bundle.Categories, "name", "(no name)",
			func(_ *testing.T, a *fakebasecamp.Account, v string) { a.Projects[3].Categories[1].Name = v }},
	}
}

// Every title and name Minka would refuse is made into one it takes, the record
// comes, the report names it by its kind and its identifier without saying the
// title, and the run goes on: a walk of hours must not end on one character
// nobody can see. Minka trims a title, takes at most 200 characters of it and
// no control character, and refuses one that is empty.
func TestATitleMinkaWouldRefuseIsMadeToFitAndTheRunGoesOn(t *testing.T) {
	long := strings.Repeat("ä", 201)
	cases := []struct {
		name, value string
		want        func(none string) string
		why         []string
	}{
		{"nothing at all", "", func(none string) string { return none }, []string{"none"}},
		{"whitespace alone", "   ", func(none string) string { return none }, []string{"none"}},
		{"nothing but characters nobody can see", "\x01\x7f", func(none string) string { return none }, []string{"none"}},
		{"a character nobody sees", "Print the\x01invitations", func(string) string { return "Print the invitations" }, []string{"folded"}},
		{"a line break", "one\ntwo", func(string) string { return "one two" }, []string{"folded"}},
		{"more than 200 letters", long, func(string) string { return strings.Repeat("ä", 199) + "…" }, []string{"cut"}},
		{"both", "a\tb" + long, func(string) string { return "a b" + strings.Repeat("ä", 196) + "…" }, []string{"folded", "cut"}},
	}
	for _, of := range titledRecords() {
		for _, c := range cases {
			t.Run(of.record+"/"+c.name, func(t *testing.T) {
				account := fakebasecamp.New()
				if of.record == "post" {
					of.id = pitches(account).Posts[0].ID
				}
				of.set(t, account, c.value)
				dir, result := extract(t, account, of.project)

				id := strconv.FormatInt(of.id, 10)
				if got := recordWithID(t, records(t, dir, of.kind), id)[of.field]; got != c.want(of.none) {
					t.Errorf("the %s comes as %q, want %q", of.field, got, c.want(of.none))
				}
				want := basecamp.TitleMadeToFit{Record: of.record, SourceID: id, Why: c.why}
				if !slices.ContainsFunc(result.TitlesMadeToFit, func(one basecamp.TitleMadeToFit) bool {
					return one.Record == want.Record && one.SourceID == want.SourceID && slices.Equal(one.Why, want.Why)
				}) {
					t.Errorf("the report names %+v, want %+v among them", result.TitlesMadeToFit, want)
				}
			})
		}
	}
}

// A title Minka takes as it stands is written as it stands and named nowhere:
// 200 letters with whitespace around them, which Minka takes off as well, a
// character Minka reads as no control character, a no-break space, which PHP's
// trim leaves, and a quote pasted from Word as a control character, which
// Minka reads as the quote it stands for.
func TestATitleMinkaTakesIsLeftAsItIs(t *testing.T) {
	for value, want := range map[string]string{
		"  " + strings.Repeat("ä", 200) + "  ": strings.Repeat("ä", 200),
		"Zero\u200bwidth":                      "Zero\u200bwidth",
		"Line\u2028separator":                  "Line\u2028separator",
		"\u00a0Plan\u00a0":                     "\u00a0Plan\u00a0",
		"Don\u0092t":                           "Don\u0092t",
	} {
		for _, of := range titledRecords() {
			account := fakebasecamp.New()
			if of.record == "post" {
				of.id = pitches(account).Posts[0].ID
			}
			of.set(t, account, value)
			dir, result := extract(t, account, of.project)
			id := strconv.FormatInt(of.id, 10)
			if got := recordWithID(t, records(t, dir, of.kind), id)[of.field]; got != want {
				t.Errorf("%s: the %s %q comes as %q", of.record, of.field, value, got)
			}
			if len(result.TitlesMadeToFit) != 0 {
				t.Errorf("%s: the report names %+v", of.record, result.TitlesMadeToFit)
			}
		}
	}
}

// A column Minka keeps itself may come without a name, which keeps the name
// Minka gives it; only a working column is given one of the run's own.
func TestASystemColumnWithoutANameKeepsMinkasName(t *testing.T) {
	for _, name := range []string{"  ", "\x01\u0081"} {
		account := fakebasecamp.New()
		roadmap(account).Lists[0].Title = name
		dir, result := extract(t, account, boardProject)
		if got := recordWithID(t, records(t, dir, bundle.Columns), "9103")["name"]; got != "" {
			t.Errorf("a triage column named %q comes as %q", name, got)
		}
		if len(result.TitlesMadeToFit) != 0 {
			t.Errorf("%q: the report names %+v", name, result.TitlesMadeToFit)
		}
	}
}

// A step holds a sentence and gets 500 characters, as Minka gives it.
func TestAStepIsMadeToFitTheRoomOfAStep(t *testing.T) {
	steps := func(a *fakebasecamp.Account) *fakebasecamp.Step { return &roadmap(a).Lists[2].Cards[0].Steps[0] }

	account := fakebasecamp.New()
	steps(account).Title = strings.Repeat("a", 500)
	dir, result := extract(t, account, boardProject)
	if got := recordWithID(t, records(t, dir, bundle.Steps), "9402")["title"]; got != strings.Repeat("a", 500) {
		t.Errorf("a step of 500 characters comes as %d", len(got.(string)))
	}
	if len(result.TitlesMadeToFit) != 0 {
		t.Errorf("the report names %+v", result.TitlesMadeToFit)
	}

	account = fakebasecamp.New()
	steps(account).Title = "Colour\x00the " + strings.Repeat("a", 500)
	dir, result = extract(t, account, boardProject)
	got := recordWithID(t, records(t, dir, bundle.Steps), "9402")["title"].(string)
	if !strings.HasPrefix(got, "Colour the ") || len([]rune(got)) != 500 || !strings.HasSuffix(got, "…") {
		t.Errorf("a step too long with a character nobody sees comes as %q", got)
	}
	want := []basecamp.TitleMadeToFit{{Record: "step", SourceID: "9402", Why: []string{"folded", "cut"}}}
	if !slices.EqualFunc(result.TitlesMadeToFit, want, func(a, b basecamp.TitleMadeToFit) bool {
		return a.Record == b.Record && a.SourceID == b.SourceID && slices.Equal(a.Why, b.Why)
	}) {
		t.Errorf("the report names %+v, want %+v", result.TitlesMadeToFit, want)
	}

	account = fakebasecamp.New()
	steps(account).Title = " "
	dir, _ = extract(t, account, boardProject)
	if got := recordWithID(t, records(t, dir, bundle.Steps), "9402")["title"]; got != "(no title)" {
		t.Errorf("a step without words comes as %q", got)
	}
}

// The title of an event is held as every title is, in a run that brings
// calendars.
func TestTheTitleOfAnEventIsMadeToFit(t *testing.T) {
	account := withCalendar()
	account.Calendar().Entries[2].Summary = "Stand\x07up"
	dir, result := extractWithCalendars(t, account, calendarProject)
	if got := recordWithID(t, records(t, dir, bundle.Events), "9613")["title"]; got != "Stand up" {
		t.Errorf("the event comes as %q", got)
	}
	if !slices.ContainsFunc(result.TitlesMadeToFit, func(one basecamp.TitleMadeToFit) bool {
		return one.Record == "event" && one.SourceID == "9613"
	}) {
		t.Errorf("the report names %+v", result.TitlesMadeToFit)
	}
}

// An event that is left out carries no title into the bundle, so the report
// names none of it among the titles made to fit: the title is held only once
// the event is written.
func TestAnEventLeftOutNamesNoTitle(t *testing.T) {
	account := withCalendar()
	for i := range account.Calendar().Entries {
		entry := &account.Calendar().Entries[i]
		switch entry.ID {
		case 9619: // said in moments, left out
			entry.Summary = "Said\x07in moments"
		case 9613:
			entry.Summary, entry.EndsAt = "Ends\x07early", "2000-01-01T00:00:00.000Z"
		}
	}
	_, result := extractWithCalendars(t, account, calendarProject)
	if len(result.TitlesMadeToFit) != 0 {
		t.Errorf("the report names %+v", result.TitlesMadeToFit)
	}
	calendar := result.Calendars.Calendars[0]
	if !slices.Equal(calendar.AllDayInInstants, []string{"9619"}) || !slices.Equal(calendar.TimesMinkaRefuses, []string{"9613"}) {
		t.Errorf("left out: %v and %v", calendar.AllDayInInstants, calendar.TimesMinkaRefuses)
	}
}

// The name of a person is held as a title is: Minka takes a person's name by the
// same rule.
func TestTheNameOfAPersonIsMadeToFit(t *testing.T) {
	account := fakebasecamp.New()
	for i := range account.People {
		if account.People[i].ID == 101 {
			account.People[i].Name = "Ada\x1bExample"
		}
	}
	dir, result := extract(t, account, boardProject)
	if got := recordWithID(t, records(t, dir, bundle.People), "101")["name"]; got != "Ada Example" {
		t.Errorf("the person comes as %q", got)
	}
	if !slices.ContainsFunc(result.TitlesMadeToFit, func(one basecamp.TitleMadeToFit) bool {
		return one.Record == "person" && one.SourceID == "101"
	}) {
		t.Errorf("the report names %+v", result.TitlesMadeToFit)
	}
}
