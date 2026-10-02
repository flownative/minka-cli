package fakebasecamp

import (
	"fmt"
	"regexp"
	"strconv"
)

// Entry is an entry of a calendar, shaped after the list bodies Basecamp was
// seen to answer: an all-day entry says its days as plain dates, the last day
// included, a timed one two moments in UTC with milliseconds, and an entry that
// does not repeat carries no key for a schedule at all.
type Entry struct {
	ID int64
	// Status is "active" when empty. ListedIn is the list that names the entry
	// where that is not the list of its status: "" for the list in use, or
	// "trashed" or "archived".
	Status   string
	ListedIn string
	// AlsoListedIn is a second list that names the entry, as a list does that
	// the entry moved into between two requests.
	AlsoListedIn string
	Summary      string
	Creator      *Person
	CreatedAt    string
	UpdatedAt    string
	// Description is rich text, as the description of a to-do is. Files are the
	// files embedded in it; the key that lists them is named as a to-do's is,
	// which no body of an entry has shown yet.
	Description string
	Files       []File
	AllDay      bool
	StartsAt    string
	EndsAt      string
	// Participants are listed in the order a test gives them.
	Participants []Person
	// JoinURL and Highlighted were never seen on a real entry; their keys are
	// there only where a test sets them.
	JoinURL     string
	Highlighted bool
	// VisibleToClients is what the entry says about clients.
	VisibleToClients *bool
	// Repetition is the schedule of an entry that repeats, with the hour, the
	// minute and the length Basecamp gives it. OddRepetition is sent in its
	// place as it stands.
	Repetition    *Repetition
	OddRepetition map[string]any
	Comments      []Comment
	Boosts        []Boost
	Subscribers   []Person
	// DayComments are comments written under single days of the series, which
	// the account names only in the list of every comment of the project.
	DayComments []DayComment
}

// DayComment is a comment under one day of a series. DayID is the recording
// Basecamp made of that day.
type DayComment struct {
	ID    int64
	DayID int64
}

func (e *Entry) status() string {
	if e.Status == "" {
		return "active"
	}
	return e.Status
}

// listedIn is the list of a calendar that names the entry.
func (e *Entry) listedIn() string {
	if e.ListedIn != "" {
		return e.ListedIn
	}
	return e.status()
}

var (
	entriesPattern         = regexp.MustCompile(`^buckets/([0-9]+)/schedules/([0-9]+)/entries\.json(?:\?status=(trashed|archived))?$`)
	projectCommentsPattern = regexp.MustCompile(`^projects/recordings\.json\?type=Comment&bucket=([0-9]+)$`)
	// readsThatWrite are the addresses Basecamp answers by making a record of a
	// single day of a series: the day itself, and the entry of a series, which
	// answers with its first day.
	readsThatWrite = regexp.MustCompile(`(^|/)buckets/([0-9]+)/schedule_entries/([0-9]+)(/occurrences/([0-9]{8}))?\.json$`)
)

// CalendarProject is the twelfth project, which a test adds where it reads
// calendars: no other run meets it. Its one calendar holds every case of an
// entry: all-day of one day and of several, timed, a series every other week
// as the account was seen to hold one, a series of somebody who names no zone,
// one in the trash, one on the shelf, one that is drafted and an all-day one
// said in moments.
const CalendarProject = 7012

// AddCalendarProject adds the project of the calendar to the account.
func (a *Account) AddCalendarProject() {
	ada, grace, edsger := a.People[1], a.People[0], a.People[2]
	berlin := ada
	berlin.TimeZone = "Europe/Berlin"
	at := func(day, hour int) string { return fmt.Sprintf("2026-09-%02dT%02d:00:00.000Z", day, hour) }
	agenda := File{ID: 5071, SGID: SGID("gid://bc3/Attachment/5071?expires_in"), Key: FileKey(5071), Filename: "agenda.png", ContentType: "image/png", Bytes: 64}
	yes := true

	calendar := Tool{ID: 9601, Name: "schedule", Type: "schedules", Title: "Schedule", Enabled: true, Position: 1, Creator: &berlin,
		CreatedAt: "2022-05-01T10:00:00.000Z", UpdatedAt: "2025-05-01T10:00:00.000Z", IncludeDueAssignments: &yes}
	calendar.Entries = []Entry{
		{ID: 9611, Summary: "Company day", Creator: &berlin, CreatedAt: at(1, 8), UpdatedAt: at(1, 9),
			Description: `<div>Bring ` + Mention(SGID("gid://bc3/Person/102?expires_in"), "", 102, "Grace Sample") + `</div>` + EmbeddedFile(agenda.SGID, "agenda.png"),
			Files:       []File{agenda},
			AllDay:      true, StartsAt: "2020-02-01", EndsAt: "2020-02-01",
			Participants: []Person{ada}, Subscribers: []Person{ada, grace},
			Comments: []Comment{{ID: 9631, Creator: &grace, CreatedAt: at(2, 9), UpdatedAt: at(2, 9), Content: `<div>See you there</div>`,
				Boosts: []Boost{{ID: 9642, Content: "🎉", CreatedAt: at(2, 11), Booster: &ada}}}},
			Boosts: []Boost{{ID: 9641, Content: "👍", CreatedAt: at(2, 10), Booster: &grace}}},
		{ID: 9612, Summary: "Holidays", Creator: &berlin, CreatedAt: at(1, 10), UpdatedAt: at(1, 10),
			AllDay: true, StartsAt: "2018-12-24", EndsAt: "2018-12-28",
			// Not in the order of their IDs, and one of them twice.
			Participants: []Person{grace, ada, grace}, VisibleToClients: &yes,
			JoinURL: "https://meet.example.test/holidays", Highlighted: true},
		{ID: 9613, Summary: "Workshop", Creator: &berlin, CreatedAt: at(1, 11), UpdatedAt: at(1, 11),
			StartsAt: "2026-06-16T13:00:00.000Z", EndsAt: "2026-06-16T16:00:00.000Z"},
		// The series the account was seen to hold: every other week on Fridays at
		// 18:00 in Berlin, half an hour, without an end. Who follows it is who it
		// is for, as on every entry where the two were compared.
		{ID: 9614, Summary: "Review", Creator: &berlin, CreatedAt: at(2, 8), UpdatedAt: at(2, 8),
			StartsAt: "2026-10-02T16:00:00.000Z", EndsAt: "2026-10-02T16:30:00.000Z",
			Participants: []Person{ada, edsger}, Subscribers: []Person{ada, edsger},
			Repetition: &Repetition{Frequency: "every_other_week", Days: []int{5}, StartDate: "2026-10-02",
				Hour: interval(18), Minute: interval(0), Duration: interval(1800)},
			DayComments: []DayComment{{ID: 9661, DayID: 9701}, {ID: 9662, DayID: 9701}, {ID: 9663, DayID: 9702}}},
		// The same series by somebody who names no zone: its hour holds in a zone
		// nobody can name, so it comes once.
		{ID: 9615, Summary: "Stand-up", Creator: &grace, CreatedAt: at(2, 9), UpdatedAt: at(2, 9),
			StartsAt: "2026-10-02T07:00:00.000Z", EndsAt: "2026-10-02T07:15:00.000Z",
			Repetition: &Repetition{Frequency: "every_week", Days: []int{5}, StartDate: "2026-10-02",
				Hour: interval(9), Minute: interval(0), Duration: interval(900)}},
		{ID: 9616, Status: "trashed", Summary: "Cancelled", Creator: &berlin, CreatedAt: at(3, 8), UpdatedAt: at(4, 8),
			StartsAt: "2026-07-01T08:00:00.000Z", EndsAt: "2026-07-01T09:00:00.000Z", Subscribers: []Person{ada},
			Comments: []Comment{{ID: 9632, Creator: &ada, CreatedAt: at(3, 9), UpdatedAt: at(3, 9), Content: `<div>Why?</div>`}}},
		{ID: 9617, Status: "archived", Summary: "Old", Creator: &berlin, CreatedAt: at(3, 10), UpdatedAt: at(4, 10),
			StartsAt: "2019-07-01", EndsAt: "2019-07-01", AllDay: true, Subscribers: []Person{ada},
			Comments: []Comment{{ID: 9633, Creator: &ada, CreatedAt: at(3, 11), UpdatedAt: at(3, 11), Content: `<div>Gone</div>`}}},
		{ID: 9618, Status: "drafted", ListedIn: "active", Summary: "Draft", Creator: &berlin, CreatedAt: at(3, 12), UpdatedAt: at(3, 12),
			StartsAt: "2026-08-01", EndsAt: "2026-08-01", AllDay: true},
		{ID: 9619, Summary: "Said in moments", Creator: &berlin, CreatedAt: at(3, 13), UpdatedAt: at(3, 13),
			AllDay: true, StartsAt: "2026-08-03T00:00:00.000Z", EndsAt: "2026-08-03T00:00:00.000Z", Subscribers: []Person{ada},
			Comments: []Comment{{ID: 9634, Creator: &ada, CreatedAt: at(3, 14), UpdatedAt: at(3, 14), Content: `<div>Hm</div>`}}},
	}

	a.Projects = append(a.Projects, Project{
		ID: CalendarProject, Name: "Calendar Project", Status: "active",
		CreatedAt: "2022-05-01T09:00:00.000Z", UpdatedAt: "2025-06-01T09:00:00.000Z",
		Members: []int64{ada.ID, grace.ID, edsger.ID},
		Dock:    []Tool{calendar},
	})
}

// Calendar is the calendar of the calendar project.
func (a *Account) Calendar() *Tool {
	for i := range a.Projects {
		if a.Projects[i].ID == CalendarProject {
			return &a.Projects[i].Dock[0]
		}
	}
	return nil
}

// calendarJSON adds to a calendar what it says about itself beside every
// tool's fields.
func (a *Account) calendarJSON(p *Project, tool Tool, answer map[string]any) {
	if tool.IncludeDueAssignments != nil {
		answer["include_due_assignments"] = *tool.IncludeDueAssignments
	}
	answer["entries_count"] = len(tool.Entries)
	answer["entries_url"] = fmt.Sprintf("%sschedules/%d/entries.json", a.root(), tool.ID)
}

// calendars answers the paths of calendars. The second value tells whether the
// path was one of them.
func (a *Account) calendars(path string, page int) ([]byte, bool) {
	switch {
	case entriesPattern.MatchString(path):
		match := entriesPattern.FindStringSubmatch(path)
		p := a.project(match[1])
		if p == nil {
			return nil, false
		}
		listed := match[3]
		if listed == "" {
			listed = "active"
		}
		for t := range p.Dock {
			tool := &p.Dock[t]
			if tool.Type != "schedules" || strconv.FormatInt(tool.ID, 10) != match[2] {
				continue
			}
			found := []map[string]any{}
			for i := range tool.Entries {
				if entry := &tool.Entries[i]; entry.listedIn() == listed || entry.AlsoListedIn == listed {
					found = append(found, a.entryJSON(p, entry))
				}
			}
			return a.pageOf(found, page), true
		}
	case projectCommentsPattern.MatchString(path):
		p := a.project(projectCommentsPattern.FindStringSubmatch(path)[1])
		if p == nil {
			return nil, false
		}
		// Only the comments of calendars: a run asks this list for nothing else.
		// That it names a comment under a single day, with the day as its parent,
		// is unmeasured: no such comment has been seen.
		found := []map[string]any{}
		a.eachEntry(p, func(entry *Entry) {
			for _, comment := range entry.Comments {
				answer := a.commentJSON(p, comment)
				answer["parent"] = map[string]any{"id": entry.ID, "type": "Schedule::Entry"}
				found = append(found, answer)
			}
			for _, comment := range entry.DayComments {
				found = append(found, map[string]any{"id": comment.ID, "type": "Comment", "status": "active",
					"parent": map[string]any{"id": comment.DayID, "type": "Schedule::Entry::Occurrence"}})
			}
		})
		return a.pageOf(found, page), true
	}
	return nil, false
}

// readThatWrites answers an address Basecamp answers by making a record of a
// single day, and notes that it was asked: no run may ask it. The answer is a
// day as Basecamp gives one, so that a run that asked would go on as if
// nothing had happened, the way it would against the real account.
func (a *Account) readThatWrites(path string) ([]byte, bool) {
	match := readsThatWrite.FindStringSubmatch(path)
	if match == nil {
		return nil, false
	}
	a.ReadsThatWrite = append(a.ReadsThatWrite, path)
	return encode(map[string]any{"id": 9799, "type": "Schedule::Entry::Occurrence", "status": "active",
		"parent": map[string]any{"id": match[3], "type": "Schedule::Entry"}}), true
}

// eachEntry visits the entries of every calendar of a project.
func (a *Account) eachEntry(p *Project, visit func(*Entry)) {
	for t := range p.Dock {
		for i := range p.Dock[t].Entries {
			visit(&p.Dock[t].Entries[i])
		}
	}
}

func (a *Account) entryJSON(p *Project, e *Entry) map[string]any {
	participants := []map[string]any{}
	for _, participant := range e.Participants {
		participants = append(participants, a.person(participant))
	}
	answer := map[string]any{
		"id": e.ID, "type": "Schedule::Entry", "status": e.status(),
		"created_at": e.CreatedAt, "updated_at": e.UpdatedAt,
		"summary": e.Summary, "title": e.Summary, "description": e.Description,
		"description_attachments": a.filesJSON(e.Files),
		"all_day":                 e.AllDay, "starts_at": e.StartsAt, "ends_at": e.EndsAt,
		"participants":   participants,
		"url":            fmt.Sprintf("%sbuckets/%d/schedule_entries/%d.json", a.root(), p.ID, e.ID),
		"comments_count": len(e.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, e.ID),
		"subscription_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/subscription.json", a.root(), p.ID, e.ID),
		"boosts_count":     len(e.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, e.ID),
	}
	if e.Creator != nil {
		answer["creator"] = a.person(*e.Creator)
	}
	if e.VisibleToClients != nil {
		answer["visible_to_clients"] = *e.VisibleToClients
	}
	if e.JoinURL != "" {
		answer["join_url"] = e.JoinURL
	}
	if e.Highlighted {
		answer["highlighted"] = true
	}
	switch {
	case e.OddRepetition != nil:
		answer["recurrence_schedule"] = e.OddRepetition
	case e.Repetition != nil:
		answer["recurrence_schedule"] = repetitionJSON(e.Repetition)
	}
	return answer
}
