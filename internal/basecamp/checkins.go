package basecamp

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	// The zone database travels in the binary: which day a record belongs to
	// must not depend on what a machine happens to have installed.
	_ "time/tzdata"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
)

// question is a question of a questionnaire, which a bundle calls a question of
// a Check-ins tool.
type question struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	// Paused is a question that asks nobody until somebody starts it again.
	Paused       bool     `json:"paused"`
	Schedule     schedule `json:"schedule"`
	AnswersCount int      `json:"answers_count"`
	// AnswersURL is where the answers of the question are listed, at an address
	// of the account and not of the project.
	AnswersURL string `json:"answers_url"`
}

// schedule is when Basecamp asks a question. Days count Sunday as 0 and
// Saturday as 6. WeekInstance, WeekInterval, MonthInterval and Duration are
// what a Basecamp schedule can say and Minka's cannot; each is null wherever a
// schedule does not use it.
type schedule struct {
	Frequency     string `json:"frequency"`
	Days          []int  `json:"days"`
	Hour          int    `json:"hour"`
	Minute        int    `json:"minute"`
	StartDate     string `json:"start_date"`
	EndDate       string `json:"end_date"`
	WeekInstance  *int   `json:"week_instance"`
	WeekInterval  *int   `json:"week_interval"`
	MonthInterval *int   `json:"month_interval"`
	Duration      *int   `json:"duration"`
}

// answer is what somebody answered a question on one day.
type answer struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// Content is rendered, as that of a comment is.
	Content string     `json:"content"`
	Files   []bodyFile `json:"content_attachments"`
	// GroupOn is the day the answer belongs to; Basecamp leaves it out of an
	// answer that belongs to no day of its own.
	GroupOn       string `json:"group_on"`
	CommentsCount int    `json:"comments_count"`
	BoostsCount   int    `json:"boosts_count"`
}

// answered is an answer with the day it belongs to.
type answered struct {
	answer
	on time.Time
}

// subscription is who a recording is handed to.
type subscription struct {
	Subscribers []person `json:"subscribers"`
}

// ImpossibleQuestion is a question whose schedule Minka has no words for,
// named with the field that makes it impossible. Such a question is left out
// with its answers.
type ImpossibleQuestion struct {
	QuestionSourceID string `json:"questionSourceId"`
	Field            string `json:"field"`
}

// InferredAnchor is a question asked every other week whose startsOn was taken
// from the day of an answer, because Basecamp says nothing about which of two
// weeks it asks in.
type InferredAnchor struct {
	QuestionSourceID string `json:"questionSourceId"`
	From             string `json:"from"`
}

// NobodyAsked is a question that comes asking nobody at all, with how many of
// the people the source hands it to a bundle could not name as asked. Such a
// question asks nobody for ever, and `asked` is what the first run makes of it.
type NobodyAsked struct {
	QuestionSourceID string `json:"questionSourceId"`
	LeftOut          int    `json:"leftOut"`
}

// AskedLeftOut counts, over all questions, the people a question is asked of
// that a bundle cannot name as being asked.
type AskedLeftOut struct {
	NotInTheProject  int `json:"notInTheProject"`
	Retired          int `json:"retired"`
	WithoutAnAddress int `json:"withoutAnAddress"`
}

// dayFormat is how both Basecamp and Minka write a day.
const dayFormat = "2006-01-02"

// maxTitle is the length of a title Minka takes, in characters.
const maxTitle = 200

// rhythms are the frequencies of a Basecamp schedule that Minka has a rhythm
// for, each named by the rhythm it comes to where several weekdays are chosen.
// A question asked every day, one asked on certain days and one asked every week
// all carry the days they are asked on, so the three are the same thing; where
// one weekday is chosen the rhythm is weekly, which says the same as daily does
// of that one day. Every other frequency leaves the question out and is
// reported: whoever adds a word here has seen what Basecamp sends under it.
var rhythms = map[string]string{
	"every_day":        bundle.RhythmDaily,
	"on_certain_days":  bundle.RhythmDaily,
	"every_week":       bundle.RhythmDaily,
	"every_other_week": bundle.RhythmEveryOtherWeek,
	"every_month":      bundle.RhythmMonthly,
}

// extractCheckins adds the questions of every questionnaire and what was
// answered to them, questionnaire by questionnaire in the order of the projects
// and their tools.
func (e *Extractor) extractCheckins(ctx context.Context, w *bundle.Writer) error {
	for _, t := range e.talks {
		for _, questionnaire := range t.questionnaires {
			if err := e.extractQuestions(ctx, w, t.bucket, questionnaire); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractQuestions adds the questions of one questionnaire, oldest first. A
// questionnaire lists the questions it asks; a walk is no snapshot, so a
// question met twice is written once.
func (e *Extractor) extractQuestions(ctx context.Context, w *bundle.Writer, bucket, tool int64) error {
	listed, err := getList[question](ctx, e.Source, fmt.Sprintf("buckets/%d/questionnaires/%d/questions.json", bucket, tool))
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	questions := make([]question, 0, len(listed))
	for _, found := range listed {
		if seen[found.ID] {
			continue
		}
		seen[found.ID] = true
		carried, err := e.met("question", found.ID, found.Status, found.recording, "project", sourceID(bucket))
		if err != nil {
			return err
		}
		if !carried {
			continue
		}
		questions = append(questions, found)
	}
	if err := oldestFirst(questions, "question", func(q question) (int64, string) { return q.ID, q.CreatedAt }); err != nil {
		return err
	}

	for _, found := range questions {
		if err := e.writeQuestion(ctx, w, bucket, tool, found); err != nil {
			return err
		}
	}
	return nil
}

// writeQuestion adds a question with its schedule and whom it is asked of.
//
// A question whose schedule Minka has no words for is left out and counted with
// the field that made it impossible, and its answers go with it: an answer
// without its question has nothing to hang on.
func (e *Extractor) writeQuestion(ctx context.Context, w *bundle.Writer, bucket, tool int64, found question) error {
	title, err := itemTitle("question", found.ID, found.Title)
	if err != nil {
		return err
	}
	when, impossible, err := scheduleOf(found, e.now().UTC())
	if err != nil {
		return err
	}
	if impossible != "" {
		e.leaveQuestionOut(found.ID, impossible)
		return nil
	}

	answers, err := e.answersOf(ctx, found, sourceID(bucket))
	if err != nil {
		return err
	}
	if when.anchored {
		anchor, answered := lastDayAnswered(answers)
		if !answered {
			e.leaveQuestionOut(found.ID, "group_on")
			return nil
		}
		when.startsOn = anchoredOn(anchor, when.weekdays[0])
		e.anchorsInferred = append(e.anchorsInferred, InferredAnchor{QuestionSourceID: sourceID(found.ID), From: anchor.Format(dayFormat)})
	}
	asked, leftOut, err := e.askedOf(ctx, bucket, found.ID)
	if err != nil {
		return err
	}
	if len(asked) == 0 {
		e.nobodyAsked = append(e.nobodyAsked, NobodyAsked{QuestionSourceID: sourceID(found.ID), LeftOut: leftOut})
	}

	origin, err := e.record("question", found.ID, found.recording)
	if err != nil {
		return err
	}
	err = w.Add(bundle.Questions, bundle.Question{
		Record:          origin,
		ProjectSourceID: sourceID(bucket),
		ToolSourceID:    sourceID(tool),
		Title:           title,
		Rhythm:          when.rhythm,
		Weekdays:        when.weekdays,
		TimeOfDay:       when.timeOfDay,
		StartsOn:        when.startsOn,
		Asked:           asked,
		Paused:          when.paused,
		// What the source says of a question comes through: one it has shelved
		// would otherwise arrive active and start asking people again.
		Status: found.Status,
	})
	if err != nil {
		return err
	}
	return e.writeAnswers(ctx, w, bucket, tool, found.ID, answers)
}

// writeAnswers adds the answers of a question, oldest first, each with its
// files, its comments and its boosts. Whatever hangs on a check-in hangs on an
// answer: a question carries none of it, and the format has no word for a
// comment or a reaction of one.
//
// An answer that says nothing is left out and counted: Minka refuses a body
// that says nothing, and nobody answered anything there.
func (e *Extractor) writeAnswers(ctx context.Context, w *bundle.Writer, bucket, tool, question int64, answers []answered) error {
	for _, one := range answers {
		body, files, err := e.rewriteBody(one.Content, "", one.Files)
		if err != nil {
			return err
		}
		if saysNothing(body) {
			e.answersWithoutABody++
			continue
		}
		origin, err := e.record("answer", one.ID, one.recording)
		if err != nil {
			return err
		}
		err = w.Add(bundle.Answers, bundle.Answer{
			Record:           origin,
			ProjectSourceID:  sourceID(bucket),
			ToolSourceID:     sourceID(tool),
			QuestionSourceID: sourceID(question),
			GroupOn:          one.on.Format(dayFormat),
			Body:             body,
		})
		if err != nil {
			return err
		}
		e.measure(bundle.Answers, body, LongestBodyRecord{Record: "answer", SourceID: origin.SourceID, In: "project", InSourceID: sourceID(bucket)})
		if err := e.attachFiles(ctx, w, bundle.OwnerAnswer, origin, files); err != nil {
			return err
		}
		err = e.writeDiscussion(ctx, w, bucket, bundle.ParentAnswer, bundle.TargetAnswer, one.ID, one.CommentsCount, one.BoostsCount)
		if err != nil {
			return err
		}
	}
	return nil
}

// leaveQuestionOut notes a question the bundle does not carry. A number nobody
// writes down is a number nobody can check.
func (e *Extractor) leaveQuestionOut(id int64, field string) {
	e.questionsLeftOut = append(e.questionsLeftOut, ImpossibleQuestion{QuestionSourceID: sourceID(id), Field: field})
}

// itemTitle is the title of a record as a bundle carries it, without the
// whitespace around it that Minka takes off anyway. A title Minka refuses ends
// the run instead of reaching it, as it would end the applier's. No message
// quotes a title: it is what somebody wrote.
func itemTitle(what string, id int64, value string) (string, error) {
	title := strings.TrimSpace(value)
	switch {
	case title == "":
		return "", &Error{Kind: API, Message: fmt.Sprintf("%s %d has no title", what, id)}
	case utf8.RuneCountInString(title) > maxTitle:
		return "", &Error{Kind: API, Message: fmt.Sprintf("the title of %s %d is longer than the %d characters a title may have", what, id, maxTitle)}
	case strings.ContainsFunc(title, unicode.IsControl):
		return "", &Error{Kind: API, Message: fmt.Sprintf("the title of %s %d holds a character nobody can see", what, id)}
	}
	return title, nil
}

// checkinSchedule is a Basecamp schedule in the words Minka has.
type checkinSchedule struct {
	rhythm    string
	weekdays  []int
	timeOfDay string
	startsOn  string
	paused    bool
	// anchored tells that startsOn has yet to be taken from an answer: Minka
	// counts the weeks of an everyOtherWeek question from it.
	anchored bool
}

// scheduleOf says a Basecamp schedule in the words Minka has, or names the one
// field that makes it a schedule Minka cannot express.
//
// A schedule that has run out comes paused: the question asks nobody any more,
// and Minka's schedule has no end to say so with. One that ends later is left
// out altogether, because the question would go on asking for ever. Whether it
// has run out is held against the day the run is made, read as Basecamp writes
// its times: in UTC.
func scheduleOf(found question, now time.Time) (checkinSchedule, string, error) {
	s := found.Schedule
	for _, beyond := range []struct {
		field string
		value *int
	}{
		{"week_instance", s.WeekInstance},
		{"week_interval", s.WeekInterval},
		{"month_interval", s.MonthInterval},
		{"duration", s.Duration},
	} {
		if beyond.value != nil && *beyond.value != 0 {
			return checkinSchedule{}, beyond.field, nil
		}
	}

	when := checkinSchedule{paused: found.Paused}
	if s.EndDate != "" {
		end, aDate := aDay(s.EndDate)
		if !aDate || !end.Before(dayOfMoment(now)) {
			return checkinSchedule{}, "end_date", nil
		}
		when.paused = true
	}

	weekdays, canBeAsked := weekdaysOfMinka(s.Days)
	if !canBeAsked {
		return checkinSchedule{}, "days", nil
	}
	rhythm, known := rhythms[s.Frequency]
	if !known {
		return checkinSchedule{}, "frequency", nil
	}
	// Minka's daily and its weekly are the same thing where one weekday is
	// chosen, and weekly is the word for it.
	if rhythm == bundle.RhythmDaily && len(weekdays) == 1 {
		rhythm = bundle.RhythmWeekly
	}
	if rhythm != bundle.RhythmDaily && len(weekdays) != 1 {
		return checkinSchedule{}, "days", nil
	}
	if s.Hour < 0 || s.Hour > 23 {
		return checkinSchedule{}, "hour", nil
	}
	if s.Minute < 0 || s.Minute > 59 {
		return checkinSchedule{}, "minute", nil
	}
	when.rhythm, when.weekdays = rhythm, weekdays
	when.timeOfDay = fmt.Sprintf("%02d:%02d", s.Hour, s.Minute)
	when.anchored = rhythm == bundle.RhythmEveryOtherWeek

	// A bundle always says which day a rhythm is counted from: without it Minka
	// takes the day of createdAt in the offset the bundle wrote, which is
	// Basecamp's UTC, so a question made late in the evening would start a day
	// late.
	if s.StartDate != "" {
		if _, aDate := aDay(s.StartDate); !aDate {
			return checkinSchedule{}, "start_date", nil
		}
		when.startsOn = s.StartDate
		return when, "", nil
	}
	made, err := dayOf("question", found.ID, found.CreatedAt, found.Creator)
	if err != nil {
		return checkinSchedule{}, "", err
	}
	when.startsOn = made.Format(dayFormat)
	return when, "", nil
}

// weekdayOfMinka turns a weekday of Basecamp into one of Minka. Basecamp counts
// Sunday as 0 and Saturday as 6, Minka Monday as 1 and Sunday as 7: so every
// day but Sunday has the same number in both, and only Sunday moves.
func weekdayOfMinka(day int) int {
	if day == 0 {
		return 7
	}
	return day
}

// weekdaysOfMinka turns the days of a schedule into Minka's, each once and in
// order. Minka refuses an empty list, so a schedule that names no day is one it
// cannot express.
func weekdaysOfMinka(days []int) ([]int, bool) {
	seen := map[int]bool{}
	var weekdays []int
	for _, day := range days {
		if day < 0 || day > 6 {
			return nil, false
		}
		if minka := weekdayOfMinka(day); !seen[minka] {
			seen[minka] = true
			weekdays = append(weekdays, minka)
		}
	}
	if len(weekdays) == 0 {
		return nil, false
	}
	sort.Ints(weekdays)
	return weekdays, true
}

// anchoredOn is the day Minka counts the weeks of an everyOtherWeek question
// from. Minka asks on the first matching day at or after startsOn and then
// every second week, so the day a question was last answered on, stepped back
// to the weekday it is asked on, puts Minka in the week Basecamp is in.
func anchoredOn(day time.Time, weekday int) string {
	// Go counts the days of a week the way Basecamp does.
	back := (weekdayOfMinka(int(day.Weekday())) - weekday + 7) % 7
	return day.AddDate(0, 0, -back).Format(dayFormat)
}

// lastDayAnswered is the most recent day an answer of a question belongs to.
func lastDayAnswered(answers []answered) (time.Time, bool) {
	var latest time.Time
	found := false
	for _, one := range answers {
		if !found || one.on.After(latest) {
			latest, found = one.on, true
		}
	}
	return latest, found
}

// askedOf are the people a question is asked of, in the order of their IDs.
// Basecamp knows them as the subscribers of the question, which is whom it is
// handed to, and not whoever happened to answer it. A later run never touches
// them again, so the first one decides.
//
// Only somebody the bundle carries as a member of the project can be asked in
// Minka, and only somebody Minka can hand a question to: whoever is not a
// member, whoever has left and whoever comes without an address is left out and
// counted. Minka's own run would otherwise end in a failure that names no file
// and no line, while its dry run stayed green. The second number is how many of
// the subscribers of this one question fell out that way.
//
// They come in the order of their IDs and each once, however Basecamp lists
// them: two runs over an unchanged account write the same bytes.
func (e *Extractor) askedOf(ctx context.Context, bucket, question int64) ([]string, int, error) {
	handedTo, err := getOne[subscription](ctx, e.Source, fmt.Sprintf("buckets/%d/recordings/%d/subscription.json", bucket, question))
	if err != nil {
		return nil, 0, err
	}

	members := e.members[bucket]
	taken := map[int64]bool{}
	leftOut := 0
	var ids []int64
	for _, subscriber := range handedTo.Subscribers {
		if subscriber.ID == 0 || taken[subscriber.ID] {
			continue
		}
		taken[subscriber.ID] = true
		// Whoever the bundle has in the project came in where the project was
		// written, so a member of it is somebody it knows.
		known, _ := e.people.known(subscriber.ID)
		switch {
		case !members[subscriber.ID]:
			e.askedLeftOut.NotInTheProject++
			leftOut++
		case known.retired():
			e.askedLeftOut.Retired++
			leftOut++
		case known.EmailAddress == "":
			e.askedLeftOut.WithoutAnAddress++
			leftOut++
		default:
			ids = append(ids, subscriber.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	asked := make([]string, 0, len(ids))
	for _, id := range ids {
		asked = append(asked, sourceID(id))
	}
	return asked, leftOut, nil
}

// answersOf are the answers of a question, oldest first, each with the day it
// belongs to. A question says how many answers it has, so one nobody answered
// costs no request. Basecamp names the list at an address of the account, which
// is read the way the steps of a card are.
//
// An answer in the trash is left out and counted, as a comment in the trash is:
// somebody threw it away, and a bundle has no place to say so.
func (e *Extractor) answersOf(ctx context.Context, found question, in string) ([]answered, error) {
	if found.AnswersCount == 0 {
		return nil, nil
	}
	path, err := e.relativePath(fmt.Sprintf("the answers of question %d", found.ID), found.AnswersURL)
	if err != nil {
		return nil, err
	}
	listed, err := getList[answer](ctx, e.Source, path)
	if err != nil {
		return nil, err
	}

	seen := map[int64]bool{}
	answers := make([]answer, 0, len(listed))
	for _, one := range listed {
		if seen[one.ID] {
			continue
		}
		seen[one.ID] = true
		carried, err := e.met("answer", one.ID, one.Status, one.recording, "project", in)
		if err != nil {
			return nil, err
		}
		if !carried {
			continue
		}
		if one.Status == bundle.StatusTrashed {
			e.answersTrashed++
			continue
		}
		answers = append(answers, one)
	}
	if err := oldestFirst(answers, "answer", func(a answer) (int64, string) { return a.ID, a.CreatedAt }); err != nil {
		return nil, err
	}

	all := make([]answered, 0, len(answers))
	for _, one := range answers {
		on, err := e.dayAnsweredOn(one)
		if err != nil {
			return nil, err
		}
		all = append(all, answered{answer: one, on: on})
	}
	return all, nil
}

// dayAnsweredOn is the day an answer belongs to, as Basecamp has it. It is taken
// over and never worked out from the schedule.
//
// Somebody who answers late answers for a day nobody was asked on, and the day
// they picked is the one the answer belongs to: the source decides that and not
// the schedule. Minka stores the day as it comes and holds it against nothing,
// so working it out here would move every such answer to a day nobody meant.
//
// An answer Basecamp names no day for takes the day it was written, and is
// counted: one answer of another day than the source meant is worth knowing
// about, however few of them there are.
func (e *Extractor) dayAnsweredOn(one answer) (time.Time, error) {
	if one.GroupOn == "" {
		e.answersWithoutADay++
		return dayOf("answer", one.ID, one.CreatedAt, one.Creator)
	}
	day, aDate := aDay(one.GroupOn)
	if !aDate {
		return time.Time{}, &Error{Kind: API, Message: fmt.Sprintf("answer %d belongs to %s", one.ID, noDay(one.GroupOn))}
	}
	return day, nil
}

// dayOf is the day something was written where Basecamp names no day of its
// own. Basecamp writes every time as UTC and Minka knows no zone of an
// organisation, so the day is the one whoever wrote the record saw: their own
// zone, as Basecamp names it. Without a zone, or with one nobody knows, the run
// ends instead of guessing: a day that is off by one is a day nobody can tell
// from a right one afterwards.
func dayOf(what string, id int64, timestamp string, author *person) (time.Time, error) {
	moment, err := momentOf(fmt.Sprintf("%s %d", what, id), "created", timestamp)
	if err != nil {
		return time.Time{}, err
	}
	if author == nil || author.TimeZone == "" {
		return time.Time{}, &Error{
			Kind:    API,
			Message: fmt.Sprintf("Basecamp names no day for %s %d, and no zone of whoever wrote it to take the day from", what, id),
		}
	}
	zone, err := time.LoadLocation(author.TimeZone)
	if err != nil {
		return time.Time{}, &Error{
			Kind:    API,
			Message: fmt.Sprintf("Basecamp names no day for %s %d, and names the zone of whoever wrote it as one nobody knows", what, id),
		}
	}
	return dayOfMoment(moment.In(zone)), nil
}

// dayOfMoment is the day a moment falls on, in the zone the moment is written
// in.
func dayOfMoment(moment time.Time) time.Time {
	year, month, day := moment.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, moment.Location())
}

// aDay reads a day the way both Basecamp and Minka write one: four digits, two
// and two. Go reads a date more loosely than that, and Minka refuses whatever
// it does not write itself, so the reading has to come back as it went in.
func aDay(value string) (time.Time, bool) {
	day, err := time.Parse(dayFormat, value)
	if err != nil || day.Format(dayFormat) != value {
		return time.Time{}, false
	}
	return day, true
}

// dayShape is the shape of a day, whether or not a calendar has that day. What
// has it may be said out loud, as a moment of the shape of one may. Nothing here
// bounds the length the way a moment needs: this shape admits ten characters and
// no other number.
var dayShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// noDay is how a message says a value that is no day, by the rule that holds for
// a moment: the value where it has the shape of one, because a reader can make
// nothing of the thirtieth of February otherwise, and nothing of it where it has
// another shape and may hold whatever the source put there.
func noDay(value string) string {
	if dayShape.MatchString(value) {
		return strconv.Quote(value) + ", which no calendar has"
	}
	return "something that is no day"
}
