package basecamp_test

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// badMoments are the shapes a run refuses wherever it reads a moment: a relative
// word, a time without the zone it is meant in, and something that is no time at
// all. Minka reads the first two as a moment of its own and would store the
// record at a time nobody meant, so this side refuses exactly what that side
// refuses. The value each maps to is what a message may say of it: a value of the
// shape of a moment out loud, any other not at all, because a field that holds no
// time holds whatever the source put there.
var badMoments = map[string]string{
	"a relative word":         "something that is no time",
	"a time without a zone":   "something that is no time",
	"no time at all":          "something that is no time",
	"whitespace around it":    "something that is no time",
	"another way round a day": "something that is no time",
	"a day no calendar has":   `"2026-02-30T17:00:00Z", which no calendar has`,
}

// theValues are the strings badMoments is keyed by. One that holds a name would
// make a message name somebody, so none of them does, and the shaped one cannot.
var theValues = map[string]string{
	"a relative word":         "tomorrow",
	"a time without a zone":   "2024-02-05T17:00:00.000",
	"no time at all":          "in the morning",
	"whitespace around it":    " 2024-02-05T17:00:00.000Z",
	"another way round a day": "05.02.2024 17:00 UTC",
	"a day no calendar has":   "2026-02-30T17:00:00Z",
}

// theRealForm is a moment the way Basecamp really writes one. Every case is held
// against it as well: a case that passes because the invented account never
// showed the field proves nothing.
const theRealForm = "2024-02-05T17:00:00.000Z"

// aMomentOfARecord is one field of one record of the invented account that
// carries a moment into the bundle.
type aMomentOfARecord struct {
	project string
	// set puts a value into that field.
	set func(*fakebasecamp.Account, string)
	// refusal is what the run says of a value that is no moment, up to the value
	// itself. It names the record and the field, so a message that says anything
	// else proves that the value reached the extractor somewhere other than
	// where it was meant to.
	refusal string
}

// everyMomentOfABundle is every field the extractor reads a moment from, one per
// kind of record of a bundle and per field of a kind. A kind that is added here
// cannot slip through the rule unnoticed.
var everyMomentOfABundle = map[string]aMomentOfARecord{
	"a person was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { a.People[1].CreatedAt = v },
		"person 101 was created"},
	"a person was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { a.People[1].UpdatedAt = v },
		"person 101 was updated"},
	"a project was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { a.Projects[2].CreatedAt = v },
		"project 7003 was created"},
	"a project was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { a.Projects[2].UpdatedAt = v },
		"project 7003 was updated"},
	"a tool was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { roadmap(a).CreatedAt = v },
		"tool 9020 was created"},
	"a tool was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { roadmap(a).UpdatedAt = v },
		"tool 9020 was updated"},
	"a column was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { doing(a).CreatedAt = v },
		"column 9104 was created"},
	"a column was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { doing(a).UpdatedAt = v },
		"column 9104 was updated"},
	"a card was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).CreatedAt = v },
		"card 9201 was created"},
	"a card was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).UpdatedAt = v },
		"card 9201 was updated"},
	"a card was completed": {boardProject,
		func(a *fakebasecamp.Account, v string) { completedCard(a).Completion.At = v },
		"card 9206 was completed"},
	// A card that stands anywhere but in Done is completed by nothing a bundle
	// writes. Its moment comes along all the same, and the applier refuses it.
	"a card outside Done was completed": {boardProject,
		func(a *fakebasecamp.Account, v string) {
			busyCard(a).Completion = &fakebasecamp.Completion{At: v, By: &fakebasecamp.Person{ID: 102, Name: "Grace Sample", Email: "grace@example.test"}}
		},
		"card 9201 was completed"},
	"a step was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).Steps[0].CreatedAt = v },
		"step 9402 was created"},
	"a step was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).Steps[0].UpdatedAt = v },
		"step 9402 was updated"},
	"a step was completed": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).Steps[1].Completion.At = v },
		"step 9401 was completed"},
	"a comment was created": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).Comments[0].CreatedAt = v },
		"comment 9301 was created"},
	"a comment was updated": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).Comments[0].UpdatedAt = v },
		"comment 9301 was updated"},
	"a boost was given": {boardProject,
		func(a *fakebasecamp.Account, v string) { busyCard(a).Boosts[0].CreatedAt = v },
		"boost 9503 was created"},
	"a kind of post was created": {talkProject,
		func(a *fakebasecamp.Account, v string) { a.Projects[3].Categories[0].CreatedAt = v },
		"category 9602 was created"},
	"a kind of post was updated": {talkProject,
		func(a *fakebasecamp.Account, v string) { a.Projects[3].Categories[0].UpdatedAt = v },
		"category 9602 was updated"},
	"a post was created": {talkProject,
		func(a *fakebasecamp.Account, v string) { pitches(a).Posts[0].CreatedAt = v },
		"post 9711 was created"},
	"a post was updated": {talkProject,
		func(a *fakebasecamp.Account, v string) { pitches(a).Posts[0].UpdatedAt = v },
		"post 9711 was updated"},
	"a line was written": {talkProject,
		func(a *fakebasecamp.Account, v string) { campfire(a).Lines[0].CreatedAt = v },
		"line 9801 was created"},
	"a line was changed": {talkProject,
		func(a *fakebasecamp.Account, v string) { campfire(a).Lines[0].UpdatedAt = v },
		"line 9801 was updated"},
	"a question was created": {checkinProject,
		func(a *fakebasecamp.Account, v string) { busyQuestion(a).CreatedAt = v },
		"question 9310 was created"},
	"a question was updated": {checkinProject,
		func(a *fakebasecamp.Account, v string) { busyQuestion(a).UpdatedAt = v },
		"question 9310 was updated"},
	"an answer was created": {checkinProject,
		func(a *fakebasecamp.Account, v string) { busyQuestion(a).Answers[0].CreatedAt = v },
		"answer 9320 was created"},
	"an answer was updated": {checkinProject,
		func(a *fakebasecamp.Account, v string) { busyQuestion(a).Answers[0].UpdatedAt = v },
		"answer 9320 was updated"},
	"a to-do list was created": {todoProject,
		func(a *fakebasecamp.Account, v string) { theList(a, 9401).CreatedAt = v },
		"to-do list 9401 was created"},
	"a to-do list was updated": {todoProject,
		func(a *fakebasecamp.Account, v string) { theList(a, 9401).UpdatedAt = v },
		"to-do list 9401 was updated"},
	"a group was created": {todoProject,
		func(a *fakebasecamp.Account, v string) { theList(a, 9416).CreatedAt = v },
		"group 9416 was created"},
	"a group was updated": {todoProject,
		func(a *fakebasecamp.Account, v string) { theList(a, 9416).UpdatedAt = v },
		"group 9416 was updated"},
	"a to-do was created": {todoProject,
		func(a *fakebasecamp.Account, v string) { theTodo(a, 9421).CreatedAt = v },
		"to-do 9421 was created"},
	"a to-do was updated": {todoProject,
		func(a *fakebasecamp.Account, v string) { theTodo(a, 9421).UpdatedAt = v },
		"to-do 9421 was updated"},
	"a to-do was completed": {todoProject,
		func(a *fakebasecamp.Account, v string) { theTodo(a, 9422).Completion.At = v },
		"to-do 9422 was completed"},
	// A to-do that is open is completed by nothing a bundle writes, and the
	// moment it says it was done still reaches the moment it was last touched.
	"an open to-do was completed": {todoProject,
		func(a *fakebasecamp.Account, v string) {
			theTodo(a, 9421).Completion = &fakebasecamp.Completion{At: v, By: &fakebasecamp.Person{ID: 102, Name: "Grace Sample", Email: "grace@example.test"}}
		},
		"to-do 9421 was completed"},
}

// completedCard is the card of the fake that Basecamp says was completed.
func completedCard(a *fakebasecamp.Account) *fakebasecamp.Card { return &roadmap(a).Lists[4].Cards[0] }

// A moment of a record is read where it is read from Basecamp, and one that is
// no moment ends the run with the record and the field it stood in.
func TestAMomentThatIsNoMomentEndsTheRun(t *testing.T) {
	for where, field := range everyMomentOfABundle {
		// The precondition: with the shape Basecamp really sends, the field
		// passes. Without it, a run that ends says nothing about the shape.
		account := fakebasecamp.New()
		field.set(account, theRealForm)
		if err := run(t, account, field.project); err != nil {
			t.Errorf("%s: the shape Basecamp sends was refused: %v", where, err)
			continue
		}

		for what, said := range badMoments {
			account := fakebasecamp.New()
			field.set(account, theValues[what])
			err := run(t, account, field.project)
			var failed *basecamp.Error
			if !errors.As(err, &failed) {
				t.Errorf("%s, %s: got %v, want the run to end", where, what, err)
				continue
			}
			if failed.Kind != basecamp.API {
				t.Errorf("%s, %s: the run ended as %v, want an api_error", where, what, failed.Kind)
			}
			want := field.refusal + " at " + said
			if !strings.HasSuffix(failed.Message, want) {
				t.Errorf("%s, %s: got %q, want a message that ends in %q", where, what, failed.Message, want)
			}
		}
	}
}

// theShapeTheApplierHolds is the expression the applier reads a moment of a
// bundle with. A run must take nothing it refuses, which is what this test holds
// every string against; the run refuses more of its own, namely a moment of that
// shape that no calendar has.
var theShapeTheApplierHolds = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

// Which strings are a moment and which are none, string by string, and none of
// those a run takes is one the applier would turn down: a string the two sides
// read differently is where a time changes its meaning without a word.
func TestTheStringsThatAreAMomentAndTheStringsThatAreNone(t *testing.T) {
	for value, aMoment := range map[string]bool{
		"2024-02-05T17:00:00.000Z":    true,
		"2024-02-05T17:00:00Z":        true,
		"2024-02-05T17:00:00+01:00":   true,
		"2024-02-05T17:00:00-08:00":   true,
		"2024-02-05T17:00:00.123456Z": true,
		"2024-02-29T17:00:00Z":        true,
		"2024-02-05t17:00:00z":        false,
		"2024-02-05T17:00:00.000z":    false,
		"2024-02-05 17:00:00Z":        false,
		"2024-02-05T17:00:00":         false,
		"2024-02-05":                  false,
		"2024-02-05T17:00:00.Z":       false,
		"2024-02-05T17:00:00Z ":       false,
		"now":                         false,
		"tomorrow":                    false,
		"+1 day":                      false,
		"2023-02-29T17:00:00Z":        false,
		"2024-13-01T17:00:00Z":        false,
		"2024-02-05T25:00:00Z":        false,
		"2024-02-05T17:00:60Z":        false,
		"2024-02-05T17:00:00+25:00":   false,
		"1700000000":                  false,
	} {
		account := fakebasecamp.New()
		busyCard(account).UpdatedAt = value
		err := run(t, account, boardProject)
		if aMoment && err != nil {
			t.Errorf("%q is a moment and was refused: %v", value, err)
		}
		if !aMoment && err == nil {
			t.Errorf("%q is no moment and was taken", value)
		}
		if err == nil && !theShapeTheApplierHolds.MatchString(value) {
			t.Errorf("%q was taken although the applier refuses that shape", value)
		}
	}
}

// A value is said only while it is short. The shape of a moment takes a fraction
// of a second of any length, and a message that floods a log is a harm of its own,
// however little it gives away.
func TestALongValueIsNotSaidHoweverWellShapedItIs(t *testing.T) {
	// Forty characters of the shape of a moment, and forty-one: the run refuses
	// both, because no calendar has the thirtieth of February.
	fraction := func(digits int) string {
		return "2026-02-30T17:00:00." + strings.Repeat("1", digits) + "Z"
	}
	for what, c := range map[string]struct{ value, said string }{
		"as long as a message says":  {fraction(19), `"` + fraction(19) + `", which no calendar has`},
		"one character too long":     {fraction(20), "something that is no time"},
		"long enough to drown a log": {fraction(100000), "something that is no time"},
	} {
		if what == "as long as a message says" && len(c.value) != 40 {
			t.Fatalf("the case meant to be as long as a message says is %d characters", len(c.value))
		}
		account := fakebasecamp.New()
		busyCard(account).UpdatedAt = c.value
		err := run(t, account, boardProject)
		var failed *basecamp.Error
		if !errors.As(err, &failed) {
			t.Errorf("%s: got %v, want the run to end", what, err)
			continue
		}
		want := "card 9201 was updated at " + c.said
		if !strings.HasSuffix(failed.Message, want) {
			t.Errorf("%s: got %q, want a message that ends in %q", what, failed.Message, want)
		}
		if len(failed.Message) > 200 {
			t.Errorf("%s: the message is %d characters long", what, len(failed.Message))
		}
	}
}

// badDays are the shapes a run refuses wherever it reads a day, each with what a
// message may say of it, by the rule that holds for a moment: Minka writes and
// reads a day one way only, and a day read another way is somebody else's day.
var badDays = map[string]struct{ value, said string }{
	"a relative word":       {"tomorrow", "something that is no day"},
	"a moment":              {theRealForm, "something that is no day"},
	"the day another way":   {"01.04.2023", "something that is no day"},
	"no day at all":         {"next spring", "something that is no day"},
	"a day no calendar has": {"2026-02-30", `"2026-02-30", which no calendar has`},
}

// The day a card is due is held to the shape a day has, the way the day of a
// question and of an answer is.
func TestADayACardIsDueThatIsNoDayEndsTheRun(t *testing.T) {
	account := fakebasecamp.New()
	busyCard(account).DueOn = "2023-04-01"
	if err := run(t, account, boardProject); err != nil {
		t.Fatalf("the shape Basecamp sends was refused: %v", err)
	}

	for what, c := range badDays {
		account := fakebasecamp.New()
		busyCard(account).DueOn = c.value
		err := run(t, account, boardProject)
		var failed *basecamp.Error
		if !errors.As(err, &failed) {
			t.Errorf("%s: got %v, want the run to end", what, err)
			continue
		}
		want := "card 9201 is due on " + c.said
		if failed.Kind != basecamp.API || !strings.HasSuffix(failed.Message, want) {
			t.Errorf("%s: got %v %q, want an api_error ending in %q", what, failed.Kind, failed.Message, want)
		}
	}
}

// The days a to-do runs are held to the same shape and said the same way, so
// that one extractor does not say a day two ways.
func TestTheDaysOfAToDoThatAreNoDaysEndTheRun(t *testing.T) {
	for what, c := range badDays {
		for _, field := range []struct {
			name string
			set  func(*fakebasecamp.Todo, string)
		}{
			{"starts on", func(one *fakebasecamp.Todo, v string) { one.StartsOn = v }},
			{"is due on", func(one *fakebasecamp.Todo, v string) { one.DueOn = v }},
		} {
			account := fakebasecamp.New()
			one := theTodo(account, 9421)
			one.StartsOn, one.DueOn = "", ""
			field.set(one, c.value)
			err := run(t, account, todoProject)
			var failed *basecamp.Error
			if !errors.As(err, &failed) {
				t.Errorf("%s %s: got %v, want the run to end", field.name, what, err)
				continue
			}
			want := "to-do 9421 " + field.name + " " + c.said
			if failed.Kind != basecamp.API || !strings.HasSuffix(failed.Message, want) {
				t.Errorf("%s %s: got %v %q, want an api_error ending in %q", field.name, what, failed.Kind, failed.Message, want)
			}
		}
	}
}

// run walks the projects and writes no bundle: what a time is worth is decided
// before anything is written.
func run(t *testing.T, account *fakebasecamp.Account, projects ...string) error {
	t.Helper()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account,
		Now: func() time.Time { return runDay }}
	_, err := extractor.Extract(t.Context(), projects, bundle.Discard())
	return err
}
