package basecamp_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The check-ins of the fake are in its seventh project: a questionnaire with one
// question in each frequency Basecamp sends, one whose schedule has run out and
// one nobody ever answered. The account lists them in another order than they
// were made in.
const checkinProject = "7007"

func questionnaire(a *fakebasecamp.Account) *fakebasecamp.Tool {
	return &a.Projects[6].Dock[0]
}

// theQuestion of the fake by its ID, because the order the account lists them in
// is not the order they were made in.
func theQuestion(a *fakebasecamp.Account, id int64) *fakebasecamp.Question {
	for i := range questionnaire(a).Questions {
		if questionnaire(a).Questions[i].ID == id {
			return &questionnaire(a).Questions[i]
		}
	}
	panic("the fake has no question " + strconv.FormatInt(id, 10))
}

// busyQuestion is the question with the most subscribers and the answers that
// carry a text, a file, a comment and boosts; weekendQuestion is asked every day
// on a Saturday and a Sunday, fortnightly every other week with answers to
// anchor it by.
func busyQuestion(a *fakebasecamp.Account) *fakebasecamp.Question    { return theQuestion(a, 9310) }
func weekendQuestion(a *fakebasecamp.Account) *fakebasecamp.Question { return theQuestion(a, 9311) }
func fortnightly(a *fakebasecamp.Account) *fakebasecamp.Question     { return theQuestion(a, 9312) }

func TestAQuestionnaireComesAsACheckinsTool(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)

	want := []map[string]any{
		{"sourceId": "9070", "createdAt": "2024-02-01T07:00:00.000Z", "updatedAt": "2024-02-28T12:00:00.000Z", "createdBy": "105",
			"projectSourceId": "7007", "kind": "checkins", "name": "Check-ins", "position": float64(1)},
	}
	if got := records(t, dir, bundle.Tools); !reflect.DeepEqual(got, want) {
		t.Errorf("tools:\n got %v\nwant %v", got, want)
	}
}

// The questions of a questionnaire come oldest first, each with the schedule
// Minka has words for and with whom it is asked of.
func TestAQuestionComesWithItsScheduleAndWhomItIsAskedOf(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), checkinProject)

	want := []map[string]any{
		{"sourceId": "9310", "createdAt": "2024-02-01T08:00:00.000Z", "updatedAt": "2024-02-01T08:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "title": "What did you work on today?",
			"rhythm": "daily", "weekdays": []any{float64(1), float64(2), float64(3), float64(4), float64(5)},
			"timeOfDay": "17:00", "startsOn": "2024-02-05",
			"asked": []any{"101", "102", "104"}, "paused": false, "status": "active"},
		{"sourceId": "9311", "createdAt": "2024-02-01T09:00:00.000Z", "updatedAt": "2024-02-01T09:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7007", "toolSourceId": "9070", "title": "Anything over the weekend?",
			"rhythm": "daily", "weekdays": []any{float64(6), float64(7)},
			"timeOfDay": "09:30", "startsOn": "2024-02-03",
			"asked": []any{"101"}, "paused": false, "status": "active"},
		{"sourceId": "9312", "createdAt": "2024-02-01T10:00:00.000Z", "updatedAt": "2024-02-01T10:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "title": "How is the roadmap?",
			"rhythm": "everyOtherWeek", "weekdays": []any{float64(3)},
			"timeOfDay": "10:00", "startsOn": "2024-02-28",
			"asked": []any{"101", "102"}, "paused": false, "status": "active"},
		{"sourceId": "9313", "createdAt": "2024-02-01T11:00:00.000Z", "updatedAt": "2024-02-01T11:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "title": "Anything for the retro?",
			"rhythm": "weekly", "weekdays": []any{float64(5)},
			"timeOfDay": "16:00", "startsOn": "2024-01-05",
			"asked": []any{"101", "102"}, "paused": true, "status": "active"},
		{"sourceId": "9315", "createdAt": "2024-02-01T13:00:00.000Z", "updatedAt": "2024-02-01T13:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7007", "toolSourceId": "9070", "title": "How was your week?",
			"rhythm": "weekly", "weekdays": []any{float64(1)},
			"timeOfDay": "09:00", "startsOn": "2024-02-05",
			"asked": []any{"101", "102"}, "paused": false, "status": "active"},
		{"sourceId": "9316", "createdAt": "2024-02-01T14:00:00.000Z", "updatedAt": "2024-02-01T14:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "title": "Anything for the month?",
			"rhythm": "monthly", "weekdays": []any{float64(1)},
			"timeOfDay": "08:00", "startsOn": "2024-02-01",
			"asked": []any{"101"}, "paused": false, "status": "active"},
	}
	if got := records(t, dir, bundle.Questions); !reflect.DeepEqual(got, want) {
		t.Errorf("questions:\n got %v\nwant %v", got, want)
	}
	// The one that nobody ever answered is the only one left out.
	if got, want := result.QuestionsLeftOut, []basecamp.ImpossibleQuestion{{QuestionSourceID: "9314", Field: "group_on"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("questions left out %v, want %v", got, want)
	}
}

// Basecamp counts Sunday as 0 and Saturday as 6, Minka Monday as 1 and Sunday
// as 7: so every day but Sunday keeps its number, and only Sunday moves. A
// formula that adds one to every day makes a set of weekdays Minka takes
// without a word, so nothing but a test says which day a question is asked on.
func TestOnlySundayMovesWhenAWeekdayBecomesMinkas(t *testing.T) {
	for basecampDay, minkaDay := range map[int]float64{0: 7, 1: 1, 2: 2, 3: 3, 4: 4, 5: 5, 6: 6} {
		account := fakebasecamp.New()
		weekendQuestion(account).Schedule.Days = []int{basecampDay}
		dir, _ := extract(t, account, checkinProject)

		question := recordWithID(t, records(t, dir, bundle.Questions), "9311")
		if got, want := question["weekdays"], []any{minkaDay}; !reflect.DeepEqual(got, want) {
			t.Errorf("Basecamp's day %d comes as %v, want %v", basecampDay, got, want)
		}
	}

	// Both days of a weekend in one schedule: Saturday stays, Sunday moves.
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)
	question := recordWithID(t, records(t, dir, bundle.Questions), "9311")
	if got, want := question["weekdays"], []any{float64(6), float64(7)}; !reflect.DeepEqual(got, want) {
		t.Errorf("a Saturday and a Sunday come as %v, want %v", got, want)
	}
}

// A day named twice is one day, and the days come in order whatever order
// Basecamp lists them in: Minka takes the weekdays of a question as a set.
func TestTheWeekdaysOfAQuestionComeOnceAndInOrder(t *testing.T) {
	account := fakebasecamp.New()
	busyQuestion(account).Schedule.Days = []int{5, 0, 3, 5, 0}
	dir, _ := extract(t, account, checkinProject)

	question := recordWithID(t, records(t, dir, bundle.Questions), "9310")
	if got, want := question["weekdays"], []any{float64(3), float64(5), float64(7)}; !reflect.DeepEqual(got, want) {
		t.Errorf("weekdays %v, want %v", got, want)
	}
}

// Every frequency Basecamp is known to send has a rhythm of Minka's. A question
// asked every day, one asked on certain days and one asked every week all carry
// the days they are asked on, so the three are the same thing; a frequency
// nobody here knows leaves the question out and is reported with its field,
// rather than a schedule nobody chose being written.
func TestEveryFrequencyBasecampSendsHasARhythmOfMinkas(t *testing.T) {
	several := []int{1, 4}
	one := []int{4}
	cases := map[string]struct {
		frequency string
		days      []int
		rhythm    string
	}{
		"every day on several days":        {"every_day", several, bundle.RhythmDaily},
		"every day on one day":             {"every_day", one, bundle.RhythmWeekly},
		"on certain days, several":         {"on_certain_days", several, bundle.RhythmDaily},
		"on certain days, one":             {"on_certain_days", one, bundle.RhythmWeekly},
		"every week on several days":       {"every_week", several, bundle.RhythmDaily},
		"every week on one day":            {"every_week", one, bundle.RhythmWeekly},
		"every month on the first weekday": {"every_month", one, bundle.RhythmMonthly},
	}
	for what, one := range cases {
		account := fakebasecamp.New()
		weekendQuestion(account).Schedule.Frequency, weekendQuestion(account).Schedule.Days = one.frequency, one.days
		dir, _ := extract(t, account, checkinProject)

		question := recordWithID(t, records(t, dir, bundle.Questions), "9311")
		if question["rhythm"] != one.rhythm {
			t.Errorf("%s: the rhythm is %v, want %s", what, question["rhythm"], one.rhythm)
		}
	}

	// The fake holds one question in every frequency, so a later hand cannot
	// take a word out of the table without a test saying so.
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)
	questions := records(t, dir, bundle.Questions)
	for id, rhythm := range map[string]string{
		"9310": bundle.RhythmDaily, "9311": bundle.RhythmDaily, "9312": bundle.RhythmEveryOtherWeek,
		"9313": bundle.RhythmWeekly, "9315": bundle.RhythmWeekly, "9316": bundle.RhythmMonthly,
	} {
		if got := recordWithID(t, questions, id)["rhythm"]; got != rhythm {
			t.Errorf("question %s has the rhythm %v, want %s", id, got, rhythm)
		}
	}
}

// One day of the week is the rhythm weekly, several are daily: Minka's daily
// means on each of the chosen weekdays, so nothing is lost either way.
func TestOneWeekdayIsWeeklyAndSeveralAreDaily(t *testing.T) {
	account := fakebasecamp.New()
	weekendQuestion(account).Schedule.Days = []int{2}
	dir, _ := extract(t, account, checkinProject)

	questions := records(t, dir, bundle.Questions)
	if got := recordWithID(t, questions, "9311")["rhythm"]; got != bundle.RhythmWeekly {
		t.Errorf("a question asked on one day has the rhythm %v, want %v", got, bundle.RhythmWeekly)
	}
	if got := recordWithID(t, questions, "9310")["rhythm"]; got != bundle.RhythmDaily {
		t.Errorf("a question asked on five days has the rhythm %v, want %v", got, bundle.RhythmDaily)
	}
}

// Whom a question is asked of comes from its subscribers, which is whom
// Basecamp hands it to: somebody who never answered is asked all the same, and
// somebody who answered without being asked is not.
//
// Only a member of the project the bundle carries can be asked, who is still
// there and who has an address: Minka hands a question to nobody else, so
// naming anybody else would say it is asked of somebody who never gets it.
func TestWhoIsAskedComesFromTheSubscribersAndNotFromWhoAnswered(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, checkinProject)

	// Of the six subscribers, three can be named: one is no member of the
	// project, one has left the account, one comes without an address.
	question := recordWithID(t, records(t, dir, bundle.Questions), "9310")
	if got, want := question["asked"], []any{"101", "102", "104"}; !reflect.DeepEqual(got, want) {
		t.Errorf("asked %v, want %v", got, want)
	}
	want := basecamp.PeopleLeftOff{NotInTheProject: 1, Retired: 1, WithoutAnAddress: 1}
	if result.AskedLeftOut != want {
		t.Errorf("people left out of asked %+v, want %+v", result.AskedLeftOut, want)
	}

	// The one who answered without being asked is in the bundle and not asked.
	for _, answer := range records(t, dir, bundle.Answers) {
		if answer["sourceId"] == "9322" && answer["createdBy"] != "105" {
			t.Errorf("the answer of somebody who is not asked is by %v", answer["createdBy"])
		}
	}
	if asked := question["asked"].([]any); slices.Contains(asked, "105") {
		t.Errorf("somebody who only answered is asked: %v", asked)
	}
	// The question nobody ever answered is left out before anybody is asked.
	if got, want := asked(account, "subscription.json"), 6; len(got) != want {
		t.Errorf("the subscribers were asked for with %v, want %d requests", got, want)
	}

	// Asked last, because it is a request of its own: the account answered with
	// the subscribers in another order than their IDs and one of them twice, so
	// that the order and the dedupe of asked are held here too. Read from the
	// answer the run was given and not from what the account holds.
	listed := idsAnswered(t, account, "buckets/7007/recordings/9310/subscription.json")
	each := map[string]bool{}
	for _, id := range listed {
		each[id] = true
	}
	if slices.IsSorted(listed) || len(each) == len(listed) {
		t.Fatalf("the account answered with the subscribers sorted or each once: %v", listed)
	}
}

// Everybody a bundle names as asked is somebody it carries, is still there and
// has an address: nobody else can be asked in Minka.
func TestNobodyIsAskedWhomMinkaCouldNotAsk(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)

	people := map[string]map[string]any{}
	for _, person := range records(t, dir, bundle.People) {
		people[person["sourceId"].(string)] = person
	}
	members := recordWithID(t, records(t, dir, bundle.Projects), "7007")["members"].([]any)

	for _, question := range records(t, dir, bundle.Questions) {
		for _, one := range question["asked"].([]any) {
			id := one.(string)
			person, carried := people[id]
			switch {
			case !carried:
				t.Errorf("question %v asks %s, who is not in the bundle", question["sourceId"], id)
			case !slices.Contains(members, one):
				t.Errorf("question %v asks %s, who is no member of the project", question["sourceId"], id)
			case person["retired"] == true:
				t.Errorf("question %v asks %s, who has left", question["sourceId"], id)
			case person["email"] == nil:
				t.Errorf("question %v asks %s, who has no address", question["sourceId"], id)
			}
		}
	}
}

// A schedule Minka has no words for leaves the question out and names the field
// that made it impossible. Its answers go with it: an answer without its
// question has nothing to hang on.
func TestAScheduleMinkaCannotExpressLeavesTheQuestionOutWithItsAnswers(t *testing.T) {
	today := runDay.Format("2006-01-02")
	cases := map[string]struct {
		field  string
		change func(*fakebasecamp.Question)
	}{
		"the first weekday of a month": {"week_instance", func(q *fakebasecamp.Question) { q.Schedule.WeekInstance = 1 }},
		"every third week":             {"week_interval", func(q *fakebasecamp.Question) { q.Schedule.WeekInterval = 3 }},
		"every second month":           {"month_interval", func(q *fakebasecamp.Question) { q.Schedule.MonthInterval = 2 }},
		"a question that runs a while": {"duration", func(q *fakebasecamp.Question) { q.Schedule.Duration = 30 }},
		"a frequency nobody knows":     {"frequency", func(q *fakebasecamp.Question) { q.Schedule.Frequency = "every_n_weeks" }},
		"no day at all":                {"days", func(q *fakebasecamp.Question) { q.Schedule.Days = []int{} }},
		"a day of no week":             {"days", func(q *fakebasecamp.Question) { q.Schedule.Days = []int{7} }},
		"a month on several weekdays": {"days", func(q *fakebasecamp.Question) {
			q.Schedule.Frequency, q.Schedule.Days = "every_month", []int{1, 3}
		}},
		"every other week on several weekdays": {"days", func(q *fakebasecamp.Question) {
			q.Schedule.Frequency, q.Schedule.Days = "every_other_week", []int{1, 3}
		}},
		"a schedule that ends later": {"end_date", func(q *fakebasecamp.Question) { q.Schedule.EndDate = "2026-06-01" }},
		"a schedule that ends today": {"end_date", func(q *fakebasecamp.Question) { q.Schedule.EndDate = today }},
		"an end that is no day":      {"end_date", func(q *fakebasecamp.Question) { q.Schedule.EndDate = "2024-3-1" }},
		"a start that is no day":     {"start_date", func(q *fakebasecamp.Question) { q.Schedule.StartDate = "2024-2-5" }},
		"an end in the year 0":       {"end_date", func(q *fakebasecamp.Question) { q.Schedule.EndDate = "0000-01-01" }},
		"a start in the year 0":      {"start_date", func(q *fakebasecamp.Question) { q.Schedule.StartDate = "0000-01-01" }},
		"an hour of no day":          {"hour", func(q *fakebasecamp.Question) { q.Schedule.Hour = 24 }},
		"a minute of no hour":        {"minute", func(q *fakebasecamp.Question) { q.Schedule.Minute = 60 }},
	}

	for what, one := range cases {
		account := fakebasecamp.New()
		one.change(busyQuestion(account))
		dir, result := extract(t, account, checkinProject)

		want := basecamp.ImpossibleQuestion{QuestionSourceID: "9310", Field: one.field}
		if !slices.Contains(result.QuestionsLeftOut, want) {
			t.Errorf("%s: the questions left out are %v, want %v among them", what, result.QuestionsLeftOut, want)
		}
		if slices.Contains(sourceIDs(records(t, dir, bundle.Questions)), "9310") {
			t.Errorf("%s: the question is in the bundle", what)
		}
		for _, answer := range recordsOrNone(t, dir, bundle.Answers) {
			if answer["questionSourceId"] == "9310" {
				t.Errorf("%s: answer %v of the question is in the bundle", what, answer["sourceId"])
			}
		}
	}
}

// A question whose schedule has run out comes paused: it asks nobody any more,
// and Minka's schedule has no end to say so with. One the source has paused
// itself comes paused as well.
func TestAQuestionThatAsksNobodyAnyMoreComesPaused(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)
	questions := records(t, dir, bundle.Questions)
	if recordWithID(t, questions, "9313")["paused"] != true {
		t.Error("a question whose schedule has run out is not paused")
	}
	if recordWithID(t, questions, "9310")["paused"] != false {
		t.Error("a question that goes on being asked is paused")
	}

	account := fakebasecamp.New()
	busyQuestion(account).Paused = true
	dir, _ = extract(t, account, checkinProject)
	if recordWithID(t, records(t, dir, bundle.Questions), "9310")["paused"] != true {
		t.Error("a question the source has paused is not paused")
	}
}

// Minka counts the weeks of a question asked every other week from the first
// matching day at or after startsOn, and Basecamp says nothing about which of
// two weeks it asks in. So the day the question was last answered on, stepped
// back to the weekday it is asked on, is what startsOn says.
func TestAQuestionAskedEveryOtherWeekIsAnchoredOnTheDayItWasLastAnswered(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), checkinProject)

	// The later of its two answers belongs to a Wednesday, which is the day it
	// is asked on.
	if got := recordWithID(t, records(t, dir, bundle.Questions), "9312")["startsOn"]; got != "2024-02-28" {
		t.Errorf("startsOn %v, want the day of the last answer", got)
	}
	want := []basecamp.InferredAnchor{{QuestionSourceID: "9312", From: "2024-02-28"}}
	if !reflect.DeepEqual(result.AnchorsInferred, want) {
		t.Errorf("anchors inferred %v, want %v", result.AnchorsInferred, want)
	}

	// An answer may sit on a day the question never asked on; the anchor steps
	// back to the week that answer was given in.
	account := fakebasecamp.New()
	fortnightly(account).Answers[1].GroupOn = "2024-03-01"
	dir, _ = extract(t, account, checkinProject)
	if got := recordWithID(t, records(t, dir, bundle.Questions), "9312")["startsOn"]; got != "2024-02-28" {
		t.Errorf("startsOn %v, want the Wednesday of the week the answer was given in", got)
	}
}

// Without an answer there is nothing to anchor a question asked every other
// week on, and Minka would ask in the wrong week half the time. So it is left
// out instead of guessed.
func TestAQuestionAskedEveryOtherWeekAndNeverAnsweredIsLeftOut(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, checkinProject)

	if slices.Contains(sourceIDs(records(t, dir, bundle.Questions)), "9314") {
		t.Error("a question nobody ever answered is in the bundle")
	}
	want := basecamp.ImpossibleQuestion{QuestionSourceID: "9314", Field: "group_on"}
	if !slices.Contains(result.QuestionsLeftOut, want) {
		t.Errorf("the questions left out are %v, want %v among them", result.QuestionsLeftOut, want)
	}
	// A question nobody answered costs no request for its answers.
	if got := asked(account, "questions/9314/answers.json"); len(got) != 0 {
		t.Errorf("the answers of a question that has none were asked for: %v", got)
	}
}

// A walk is no snapshot: a list that moves under it can hold a record twice,
// and a sourceId that stands twice in one file is worse than a refusal, because
// the applier silently keeps one of the two.
func TestAQuestionAndAnAnswerMetTwiceAreWrittenOnce(t *testing.T) {
	account := fakebasecamp.New()
	weekendQuestion(account).Twice = true
	busyQuestion(account).Answers[1].Twice = true
	dir, _ := extract(t, account, checkinProject)

	for _, kind := range []bundle.Kind{bundle.Questions, bundle.Answers} {
		ids := sourceIDs(records(t, dir, kind))
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				t.Errorf("%s holds %s twice: %v", kind, id, ids)
			}
			seen[id] = true
		}
	}
}

// An answer comes with the day it belongs to and its text, and what hangs on it
// hangs on the answer: its comments, its boosts and its files.
func TestAnAnswerComesWithItsDayItsTextAndWhatHangsOnIt(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)

	want := []map[string]any{
		{"sourceId": "9320", "createdAt": "2024-02-05T17:00:00.000Z", "updatedAt": "2024-02-05T17:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "questionSourceId": "9310", "groupOn": "2024-02-05",
			"body": `<div>Reviewed the plan with <import-mention ref="102">Grace Sample</import-mention></div><import-attachment ref="attachment-5031"></import-attachment>`},
		{"sourceId": "9321", "createdAt": "2024-02-05T18:00:00.000Z", "updatedAt": "2024-02-05T18:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7007", "toolSourceId": "9070", "questionSourceId": "9310", "groupOn": "2024-02-05",
			"body": "<div>Packed the boxes</div>"},
		{"sourceId": "9322", "createdAt": "2024-02-06T09:00:00.000Z", "updatedAt": "2024-02-06T09:00:00.000Z", "createdBy": "105",
			"projectSourceId": "7007", "toolSourceId": "9070", "questionSourceId": "9310", "groupOn": "2024-02-06",
			"body": "<div>Joined from the outside</div>"},
		{"sourceId": "9325", "createdAt": "2024-02-14T10:00:00.000Z", "updatedAt": "2024-02-14T10:00:00.000Z", "createdBy": "102",
			"projectSourceId": "7007", "toolSourceId": "9070", "questionSourceId": "9312", "groupOn": "2024-02-14",
			"body": "<div>On course</div>"},
		{"sourceId": "9326", "createdAt": "2024-02-28T10:00:00.000Z", "updatedAt": "2024-02-28T10:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "questionSourceId": "9312", "groupOn": "2024-02-28",
			"body": "<div>Still on course</div>"},
		{"sourceId": "9327", "createdAt": "2024-02-02T16:00:00.000Z", "updatedAt": "2024-02-02T16:00:00.000Z", "createdBy": "101",
			"projectSourceId": "7007", "toolSourceId": "9070", "questionSourceId": "9313", "groupOn": "2024-02-02",
			"body": "<div>The deploys</div>"},
	}
	if got := records(t, dir, bundle.Answers); !reflect.DeepEqual(got, want) {
		t.Errorf("answers:\n got %v\nwant %v", got, want)
	}

	comment := recordWithID(t, records(t, dir, bundle.Comments), "9350")
	if comment["parentKind"] != bundle.ParentAnswer || comment["parentSourceId"] != "9320" {
		t.Errorf("the comment of an answer hangs on %v %v", comment["parentKind"], comment["parentSourceId"])
	}
	reactions := records(t, dir, bundle.Reactions)
	if got := recordWithID(t, reactions, "9360"); got["targetKind"] != bundle.TargetAnswer || got["targetSourceId"] != "9320" {
		t.Errorf("the boost of an answer answers %v %v", got["targetKind"], got["targetSourceId"])
	}
	if got := recordWithID(t, reactions, "9361"); got["targetKind"] != bundle.TargetComment {
		t.Errorf("the boost of a comment of an answer answers %v", got["targetKind"])
	}
	file := recordWithID(t, records(t, dir, bundle.Attachments), "attachment-5031")
	if file["ownerKind"] != bundle.OwnerAnswer || file["ownerSourceId"] != "9320" {
		t.Errorf("the file of an answer belongs to %v %v", file["ownerKind"], file["ownerSourceId"])
	}
}

// Nothing hangs on a question: the format has no word for a comment or a
// reaction of one, and a record that named one would wait for ever in Minka.
func TestNothingHangsOnAQuestionItself(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, checkinProject)

	for _, comment := range records(t, dir, bundle.Comments) {
		if comment["parentKind"] == "question" {
			t.Errorf("comment %v hangs on a question", comment["sourceId"])
		}
	}
	for _, reaction := range records(t, dir, bundle.Reactions) {
		if reaction["targetKind"] == "question" {
			t.Errorf("reaction %v answers a question", reaction["sourceId"])
		}
	}
	for _, id := range sourceIDs(records(t, dir, bundle.Questions)) {
		for _, hanging := range []string{"/comments.json", "/boosts.json"} {
			if got := asked(account, "recordings/"+id+hanging); len(got) != 0 {
				t.Errorf("a question was asked for what hangs on it: %v", got)
			}
		}
	}
}

// Minka refuses a body that says nothing, and nobody answered anything there.
func TestAnAnswerThatSaysNothingIsLeftOutAndCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), checkinProject)

	if slices.Contains(sourceIDs(records(t, dir, bundle.Answers)), "9323") {
		t.Error("an answer that says nothing is in the bundle")
	}
	if result.AnswersWithoutABody != 1 {
		t.Errorf("answers that say nothing: %d, want 1", result.AnswersWithoutABody)
	}
}

func TestAnAnswerInTheTrashIsLeftOutAndCounted(t *testing.T) {
	account := fakebasecamp.New()
	busyQuestion(account).Answers[1].Status = bundle.StatusTrashed
	dir, result := extract(t, account, checkinProject)

	if slices.Contains(sourceIDs(records(t, dir, bundle.Answers)), "9321") {
		t.Error("an answer in the trash is in the bundle")
	}
	if result.AnswersTrashed != 1 {
		t.Errorf("answers in the trash: %d, want 1", result.AnswersTrashed)
	}
}

// Where Basecamp names no day for an answer, the day is the one whoever wrote
// it saw: their own zone, and not the UTC every time of Basecamp comes in.
func TestAnAnswerWithoutADayTakesTheDayItWasWrittenInTheZoneOfItsAuthor(t *testing.T) {
	account := fakebasecamp.New()
	// Half past midnight where the author lives, half past ten the evening
	// before where Basecamp writes its times.
	answer := &busyQuestion(account).Answers[0]
	answer.GroupOn, answer.CreatedAt, answer.UpdatedAt = "", "2024-07-05T22:30:00.000Z", "2024-07-05T22:30:00.000Z"
	dir, result := extract(t, account, checkinProject)

	if got := recordWithID(t, records(t, dir, bundle.Answers), "9320")["groupOn"]; got != "2024-07-06" {
		t.Errorf("the answer belongs to %v, want the day its author saw", got)
	}
	if result.AnswersWithoutADay != 1 {
		t.Errorf("answers without a day of their own: %d, want 1", result.AnswersWithoutADay)
	}
}

// A day that is off by one is a day nobody can tell from a right one
// afterwards, so a run that cannot say which day an answer belongs to ends.
func TestAnAnswerWithoutADayAndWithoutAZoneEndsTheRun(t *testing.T) {
	for what, zone := range map[string]string{"no zone at all": "", "a zone nobody knows": "Mars/Olympus"} {
		account := fakebasecamp.New()
		answer := &busyQuestion(account).Answers[1]
		answer.GroupOn, answer.Creator.TimeZone = "", zone

		err := extractExpectingAnError(t, account, checkinProject)
		if !strings.Contains(err.Error(), "9321") {
			t.Errorf("%s: the run ended with %q, which does not name the answer", what, err)
		}
	}
}

// A question and an answer are held to what a bundle can carry where the walk
// first meets them: a status a bundle has no word for leaves the record out and
// is counted, before what hangs on it is asked for. The status of a question
// reaches the bundle, so a word nobody knows would otherwise reach the applier.
func TestAStatusNoBundleKnowsLeavesTheQuestionOrTheAnswerOut(t *testing.T) {
	cases := map[string]struct {
		left   basecamp.LeftOutRecords
		gone   string
		kind   bundle.Kind
		change func(*fakebasecamp.Account)
	}{
		"a question": {basecamp.LeftOutRecords{Record: "question", Field: "status", Value: "draft", In: "project", InSourceID: "7007", LeftOut: 1},
			"9311", bundle.Questions,
			func(a *fakebasecamp.Account) { weekendQuestion(a).Status = "draft" }},
		"an answer": {basecamp.LeftOutRecords{Record: "answer", Field: "status", Value: "pending", In: "project", InSourceID: "7007", LeftOut: 1},
			"9321", bundle.Answers,
			func(a *fakebasecamp.Account) { busyQuestion(a).Answers[1].Status = "pending" }},
	}
	for what, one := range cases {
		account := fakebasecamp.New()
		one.change(account)
		dir, result := extract(t, account, checkinProject)

		if want := []basecamp.LeftOutRecords{one.left}; !reflect.DeepEqual(result.RecordsLeftOut, want) {
			t.Errorf("%s: the records left out are %v, want %v", what, result.RecordsLeftOut, want)
		}
		if got := sourceIDs(records(t, dir, one.kind)); slices.Contains(got, one.gone) {
			t.Errorf("%s: %s came into the bundle: %v", what, one.gone, got)
		}
		if asked := asked(account, "recordings/"+one.gone+"/"); len(asked) != 0 {
			t.Errorf("%s: what hangs on %s was asked for: %v", what, one.gone, asked)
		}
	}
}

// The questions of a questionnaire are written oldest first, whatever order
// Basecamp lists them in: the applier reads the order of a file, and a run over
// an unchanged account has to write the same bytes twice.
func TestTheQuestionsOfAQuestionnaireAreWrittenOldestFirst(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, checkinProject)

	// Read from the answers the account gave, not from what it holds: a source
	// that listed them oldest first would leave nothing for the order to do.
	listed := idsAnswered(t, account, "buckets/7007/questionnaires/9070/questions.json")
	if slices.IsSorted(listed) {
		t.Fatalf("the account listed the questions in the order of their age: %v", listed)
	}

	written := sourceIDs(records(t, dir, bundle.Questions))
	if want := []string{"9310", "9311", "9312", "9313", "9315", "9316"}; !slices.Equal(written, want) {
		t.Errorf("the questions are written %v, want %v", written, want)
	}
}

// Basecamp names the answers of a question at an address of the account, which
// is read the way the steps of a card are: an address that leads anywhere else
// is refused instead of fetched, because the Basecamp CLI signs every request it
// makes.
func TestAnswersAtAnAddressOutsideTheAccountEndTheRun(t *testing.T) {
	for what, address := range map[string]string{
		"another host":      "https://elsewhere.example.test/4242/buckets/7007/questions/9310/answers.json",
		"another account":   "https://3.basecampapi.test/9999/buckets/7007/questions/9310/answers.json",
		"a path of its own": "https://3.basecampapi.test/4242/buckets/7007/questions/9310/answers.json?page=7",
	} {
		account := fakebasecamp.New()
		busyQuestion(account).AnswersURL = address

		err := extractExpectingAnError(t, account, checkinProject)
		if !strings.Contains(err.Error(), "9310") {
			t.Errorf("%s: the run ended with %q, which does not name the question", what, err)
		}
		if strings.Contains(err.Error(), address) {
			t.Errorf("%s: the failure quotes the address", what)
		}
	}
}

// Minka takes a day the way both it and Basecamp write one, and refuses
// anything else.
func TestADayOfAnotherShapeEndsTheRun(t *testing.T) {
	for _, c := range badDays {
		account := fakebasecamp.New()
		busyQuestion(account).Answers[0].GroupOn = c.value

		err := extractExpectingAnError(t, account, checkinProject)
		if want := "answer 9320 belongs to " + c.said; !strings.HasSuffix(err.Error(), want) {
			t.Errorf("the run ended with %q, want a message that ends in %q", err, want)
		}
	}
}

// A bundle always says which day a rhythm is counted from: without it Minka
// takes the day of createdAt in the offset the bundle wrote, which is
// Basecamp's UTC, so a question made late in the evening would start a day
// late.
func TestStartsOnIsWrittenEvenWhereBasecampNamesNoStart(t *testing.T) {
	account := fakebasecamp.New()
	question := busyQuestion(account)
	question.Schedule.StartDate = ""
	question.CreatedAt, question.UpdatedAt = "2024-07-01T23:30:00.000Z", "2024-07-01T23:30:00.000Z"
	dir, _ := extract(t, account, checkinProject)

	if got := recordWithID(t, records(t, dir, bundle.Questions), "9310")["startsOn"]; got != "2024-07-02" {
		t.Errorf("startsOn %v, want the day its author saw", got)
	}
}

// What the source says of a question comes through: one it has shelved would
// otherwise arrive active and start asking people again.
func TestTheStatusOfAQuestionIsTheOneTheSourceHas(t *testing.T) {
	for _, status := range []string{bundle.StatusActive, bundle.StatusArchived, bundle.StatusTrashed} {
		account := fakebasecamp.New()
		weekendQuestion(account).Status = status
		dir, _ := extract(t, account, checkinProject)

		if got := recordWithID(t, records(t, dir, bundle.Questions), "9311")["status"]; got != status {
			t.Errorf("a question the source calls %s comes as %v", status, got)
		}
	}
}

// A title the applier refuses ends the run where the walk meets it, as a kind
// of post without a name does.
func TestATitleNoBundleCanCarryEndsTheRun(t *testing.T) {
	cases := map[string]string{
		"nothing at all":          "",
		"whitespace alone":        "   ",
		"more than 200 letters":   strings.Repeat("ä", 201),
		"a character nobody sees": "What did you \x01 do?",
	}
	for what, title := range cases {
		account := fakebasecamp.New()
		busyQuestion(account).Title = title

		err := extractExpectingAnError(t, account, checkinProject)
		if !strings.Contains(err.Error(), "9310") {
			t.Errorf("%s: the run ended with %q, which does not name the question", what, err)
		}
		if strings.Contains(err.Error(), title) && title != "" {
			t.Errorf("%s: the failure quotes the title", what)
		}
	}

	// A title of exactly 200 letters, and one whose whitespace makes it longer,
	// come through.
	account := fakebasecamp.New()
	busyQuestion(account).Title = "  " + strings.Repeat("ä", 200) + "  "
	dir, _ := extract(t, account, checkinProject)
	if got := recordWithID(t, records(t, dir, bundle.Questions), "9310")["title"]; got != strings.Repeat("ä", 200) {
		t.Errorf("a title of 200 letters comes as %d characters", len([]rune(got.(string))))
	}
}

// Every reference a record of a check-in carries leads to a record of the
// bundle.
func TestEveryRecordOfACheckinNamesRecordsOfTheBundle(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), checkinProject)

	tools := sourceIDs(records(t, dir, bundle.Tools))
	projects := sourceIDs(records(t, dir, bundle.Projects))
	people := sourceIDs(records(t, dir, bundle.People))
	questions := sourceIDs(records(t, dir, bundle.Questions))

	named := func(known []string, value any) bool {
		id, isText := value.(string)
		return isText && slices.Contains(known, id)
	}
	for _, question := range records(t, dir, bundle.Questions) {
		if !named(projects, question["projectSourceId"]) || !named(tools, question["toolSourceId"]) {
			t.Errorf("question %v names the project %v and the tool %v", question["sourceId"], question["projectSourceId"], question["toolSourceId"])
		}
		if by := question["createdBy"]; by != nil && !named(people, by) {
			t.Errorf("question %v was created by %v, who is not in the bundle", question["sourceId"], by)
		}
	}
	for _, answer := range records(t, dir, bundle.Answers) {
		if !named(questions, answer["questionSourceId"]) {
			t.Errorf("answer %v names the question %v, which is not in the bundle", answer["sourceId"], answer["questionSourceId"])
		}
		if !named(tools, answer["toolSourceId"]) || !named(projects, answer["projectSourceId"]) {
			t.Errorf("answer %v names the tool %v of the project %v", answer["sourceId"], answer["toolSourceId"], answer["projectSourceId"])
		}
		if by := answer["createdBy"]; by != nil && !named(people, by) {
			t.Errorf("answer %v was written by %v, who is not in the bundle", answer["sourceId"], by)
		}
	}
}

// extractExpectingAnError runs an extraction that has to end, and hands out the
// failure it ended with.
func extractExpectingAnError(t *testing.T, account *fakebasecamp.Account, projects ...string) *basecamp.Error {
	t.Helper()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Now: func() time.Time { return runDay }}
	_, err := extractor.Extract(t.Context(), projects, bundle.Discard())

	var failed *basecamp.Error
	if !errors.As(err, &failed) {
		t.Fatalf("the run ended with %v, want a failure of the API", err)
	}
	if failed.Kind != basecamp.API {
		t.Errorf("the failure is of the kind %s, want %s", failed.Kind, basecamp.API)
	}
	return failed
}

// sourceOf is the ID of a record of the account as a bundle writes it.
func sourceOf(id int64) string {
	return strconv.FormatInt(id, 10)
}

// idsAnswered are the IDs the account answered a path with, in that order, over
// every page of it. Whatever a fake holds, this is what a run was given.
func idsAnswered(t *testing.T, account *fakebasecamp.Account, path string) []string {
	t.Helper()
	var ids []string
	for page := 1; ; page++ {
		content, err := account.Get(t.Context(), fmt.Sprintf("%s?page=%d", path, page))
		if err != nil {
			t.Fatal(err)
		}
		var listed []struct {
			ID          int64 `json:"id"`
			Subscribers []struct {
				ID int64 `json:"id"`
			} `json:"subscribers"`
		}
		// A subscription is one object and no list, and names the people in it.
		var one struct {
			Subscribers []struct {
				ID int64 `json:"id"`
			} `json:"subscribers"`
		}
		if json.Unmarshal(content, &listed) != nil {
			if err := json.Unmarshal(content, &one); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			for _, subscriber := range one.Subscribers {
				ids = append(ids, sourceOf(subscriber.ID))
			}
			return ids
		}
		if len(listed) == 0 {
			return ids
		}
		for _, entry := range listed {
			ids = append(ids, sourceOf(entry.ID))
		}
	}
}
