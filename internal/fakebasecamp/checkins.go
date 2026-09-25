package fakebasecamp

import (
	"fmt"
	"regexp"
	"strconv"
)

// Question is a question of a questionnaire, which Basecamp asks on a schedule.
type Question struct {
	ID int64
	// Status is "active" when empty.
	Status    string
	Title     string
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	Paused    bool
	Schedule  Schedule
	// Subscribers are the people the question is asked of.
	Subscribers []Person
	Answers     []Answer
	// AnswersURL replaces the address the question names its answers at.
	AnswersURL string
	// Twice has the list of questions hold this one twice, as a list that moves
	// under a walk does.
	Twice bool
}

// Schedule is when a question is asked. Days count Sunday as 0, as Basecamp
// does. WeekInstance, WeekInterval, MonthInterval and Duration are null in the
// answer when 0, as they are for a schedule that does not use them.
type Schedule struct {
	Frequency     string
	Days          []int
	Hour          int
	Minute        int
	StartDate     string
	EndDate       string
	WeekInstance  int
	WeekInterval  int
	MonthInterval int
	Duration      int
}

// Answer is what somebody answered a question on one day.
type Answer struct {
	ID int64
	// Status is "active" when empty.
	Status string
	// Content is rendered, as that of a comment is.
	Content string
	// GroupOn is the day the answer belongs to.
	GroupOn   string
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	Comments  []Comment
	Boosts    []Boost
	Files     []File
	// Twice has the list of answers hold this one twice.
	Twice bool
}

// checkinProject is a project with a questionnaire that holds every case: one
// question in each frequency Basecamp is known to send, the one asked on certain
// days with more subscribers than a bundle can name, the one asked every day on
// a Saturday and a Sunday, the one asked every other week with answers to anchor
// it by, one whose schedule has ended, and one asked every other week that
// nobody ever answered. Among the answers are one by somebody
// the question is not asked of and one that says nothing.
//
// The questionnaire was made by somebody who is no member of the project any
// more, and who is a subscriber of its first question: so whoever is asked has
// to be held against the members of the project, and not merely against whom
// the bundle happens to carry.
func checkinProject(ada, grace, linus, edsger, niklaus, deleted Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2024-02-%02dT%02d:00:00.000Z", day, hour) }

	// Ada lives east of Basecamp's UTC, so a day taken from a time of hers is
	// hers and not Basecamp's.
	ada.TimeZone = "Europe/Berlin"

	chart := File{ID: 5031, SGID: SGID("gid://bc3/Attachment/5031?expires_in"), Key: FileKey(5031), Filename: "hours.png", ContentType: "image/png", Bytes: 256}

	today := Question{
		ID: 9310, Title: "What did you work on today?", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8),
		Schedule:    Schedule{Frequency: "on_certain_days", Days: []int{1, 2, 3, 4, 5}, Hour: 17, StartDate: "2024-02-05"},
		Subscribers: []Person{ada, grace, linus, edsger, deleted, niklaus},
		Answers: []Answer{
			{ID: 9320, GroupOn: "2024-02-05", Creator: &ada, CreatedAt: at(5, 17), UpdatedAt: at(5, 17), Files: []File{chart},
				Content: `<div>Reviewed the plan with ` + Mention(SGID("gid://bc3/Person/102?expires_in"), "", 102, "Grace Sample") + `</div>` + EmbeddedFile(chart.SGID, "hours.png"),
				Comments: []Comment{
					{ID: 9350, Creator: &grace, CreatedAt: at(5, 18), UpdatedAt: at(5, 18), Content: `<div>Thanks</div>`,
						Boosts: []Boost{{ID: 9361, Content: "🙏", CreatedAt: at(5, 19), Booster: &ada}}},
				},
				Boosts: []Boost{{ID: 9360, Content: "👍", CreatedAt: at(5, 18), Booster: &edsger}}},
			{ID: 9321, GroupOn: "2024-02-05", Creator: &grace, CreatedAt: at(5, 18), UpdatedAt: at(5, 18), Content: `<div>Packed the boxes</div>`},
			// Somebody the question is not asked of answers it all the same.
			{ID: 9322, GroupOn: "2024-02-06", Creator: &niklaus, CreatedAt: at(6, 9), UpdatedAt: at(6, 9), Content: `<div>Joined from the outside</div>`},
			{ID: 9323, GroupOn: "2024-02-06", Creator: &edsger, CreatedAt: at(6, 10), UpdatedAt: at(6, 10), Content: `<div> </div>`},
		},
	}

	weekend := Question{
		ID: 9311, Title: "Anything over the weekend?", Creator: &grace, CreatedAt: at(1, 9), UpdatedAt: at(1, 9),
		Schedule:    Schedule{Frequency: "every_day", Days: []int{0, 6}, Hour: 9, Minute: 30, StartDate: "2024-02-03"},
		Subscribers: []Person{ada},
	}

	roadmap := Question{
		ID: 9312, Title: "How is the roadmap?", Creator: &ada, CreatedAt: at(1, 10), UpdatedAt: at(1, 10),
		Schedule:    Schedule{Frequency: "every_other_week", Days: []int{3}, Hour: 10, StartDate: "2024-01-03"},
		Subscribers: []Person{ada, grace},
		Answers: []Answer{
			{ID: 9325, GroupOn: "2024-02-14", Creator: &grace, CreatedAt: at(14, 10), UpdatedAt: at(14, 10), Content: `<div>On course</div>`},
			{ID: 9326, GroupOn: "2024-02-28", Creator: &ada, CreatedAt: at(28, 10), UpdatedAt: at(28, 10), Content: `<div>Still on course</div>`},
		},
	}

	retro := Question{
		ID: 9313, Title: "Anything for the retro?", Creator: &ada, CreatedAt: at(1, 11), UpdatedAt: at(1, 11),
		Schedule:    Schedule{Frequency: "on_certain_days", Days: []int{5}, Hour: 16, StartDate: "2024-01-05", EndDate: "2024-03-01"},
		Subscribers: []Person{ada, grace},
		Answers: []Answer{
			{ID: 9327, GroupOn: "2024-02-02", Creator: &ada, CreatedAt: at(2, 16), UpdatedAt: at(2, 16), Content: `<div>The deploys</div>`},
		},
	}

	weekly := Question{
		ID: 9315, Title: "How was your week?", Creator: &grace, CreatedAt: at(1, 13), UpdatedAt: at(1, 13),
		Schedule:    Schedule{Frequency: "every_week", Days: []int{1}, Hour: 9, StartDate: "2024-02-05"},
		Subscribers: []Person{ada, grace},
	}

	monthly := Question{
		ID: 9316, Title: "Anything for the month?", Creator: &ada, CreatedAt: at(1, 14), UpdatedAt: at(1, 14),
		Schedule:    Schedule{Frequency: "every_month", Days: []int{1}, Hour: 8, StartDate: "2024-02-01"},
		Subscribers: []Person{ada},
	}

	unanswered := Question{
		ID: 9314, Title: "Fortnightly and never answered", Creator: &grace, CreatedAt: at(1, 12), UpdatedAt: at(1, 12),
		Schedule:    Schedule{Frequency: "every_other_week", Days: []int{4}, Hour: 11, StartDate: "2024-01-04"},
		Subscribers: []Person{grace},
	}

	checkins := tool(9070, "questionnaire", "questionnaires", "Check-ins", true, 1, &niklaus)
	checkins.CreatedAt, checkins.UpdatedAt = at(1, 7), at(28, 12)
	// Not the order they were made in: Basecamp lists what it likes, and the
	// bundle writes them oldest first.
	checkins.Questions = []Question{today, retro, weekend, monthly, roadmap, unanswered, weekly}

	return Project{
		ID: 7007, Name: "Check-in Project", Status: "active",
		CreatedAt: at(1, 7), UpdatedAt: at(28, 12),
		Members: []int64{101, 102, 103, 104, 802},
		Dock:    []Tool{checkins},
	}
}

var (
	questionsPattern    = regexp.MustCompile(`^buckets/([0-9]+)/questionnaires/([0-9]+)/questions\.json$`)
	answersPattern      = regexp.MustCompile(`^buckets/([0-9]+)/questions/([0-9]+)/answers\.json$`)
	subscriptionPattern = regexp.MustCompile(`^buckets/([0-9]+)/recordings/([0-9]+)/subscription\.json$`)
)

// checkins answers the paths of questionnaires. The second value tells whether
// the path was one of them.
func (a *Account) checkins(path string, page int) ([]byte, bool) {
	switch {
	case questionsPattern.MatchString(path):
		match := questionsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for t := range p.Dock {
				tool := &p.Dock[t]
				if tool.Type != "questionnaires" || strconv.FormatInt(tool.ID, 10) != match[2] {
					continue
				}
				var found []map[string]any
				for i := range tool.Questions {
					question := &tool.Questions[i]
					found = append(found, a.questionJSON(p, tool, question))
					if question.Twice {
						found = append(found, a.questionJSON(p, tool, question))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case answersPattern.MatchString(path):
		match := answersPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			if question, known := a.questionOf(p, match[2]); known {
				var found []map[string]any
				// Newest first, as Basecamp lists what hangs on a recording.
				for i := len(question.Answers) - 1; i >= 0; i-- {
					found = append(found, a.answerJSON(p, question, question.Answers[i]))
					if question.Answers[i].Twice {
						found = append(found, a.answerJSON(p, question, question.Answers[i]))
					}
				}
				return a.pageOf(found, page), true
			}
		}
	case subscriptionPattern.MatchString(path):
		match := subscriptionPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			if question, known := a.questionOf(p, match[2]); known {
				subscribers := []map[string]any{}
				for _, subscriber := range question.Subscribers {
					subscribers = append(subscribers, a.person(subscriber))
				}
				// The subscription of a recording is no list and has no pages.
				return encode(map[string]any{
					"subscribed": true, "count": len(question.Subscribers),
					"url": a.root() + path, "subscribers": subscribers,
				}), true
			}
		}
	}
	return nil, false
}

// questionOf finds a question of a project by its ID.
func (a *Account) questionOf(p *Project, id string) (*Question, bool) {
	for t := range p.Dock {
		for i := range p.Dock[t].Questions {
			if strconv.FormatInt(p.Dock[t].Questions[i].ID, 10) == id {
				return &p.Dock[t].Questions[i], true
			}
		}
	}
	return nil, false
}

func (q *Question) status() string {
	if q.Status == "" {
		return "active"
	}
	return q.Status
}

func (n *Answer) status() string {
	if n.Status == "" {
		return "active"
	}
	return n.Status
}

func (a *Account) questionJSON(p *Project, tool *Tool, q *Question) map[string]any {
	orNull := func(value int) any {
		if value == 0 {
			return nil
		}
		return value
	}
	textOrNull := func(value string) any {
		if value == "" {
			return nil
		}
		return value
	}
	answersURL := q.AnswersURL
	if answersURL == "" {
		answersURL = fmt.Sprintf("%sbuckets/%d/questions/%d/answers.json", a.root(), p.ID, q.ID)
	}

	answer := map[string]any{
		"id": q.ID, "status": q.status(), "title": q.Title, "type": "Question", "paused": q.Paused,
		"parent": map[string]any{"id": tool.ID, "type": "Questionnaire", "title": tool.Title},
		"schedule": map[string]any{
			"frequency": q.Schedule.Frequency, "days": q.Schedule.Days,
			"hour": q.Schedule.Hour, "minute": q.Schedule.Minute,
			"start_date": textOrNull(q.Schedule.StartDate), "end_date": textOrNull(q.Schedule.EndDate),
			"week_instance": orNull(q.Schedule.WeekInstance), "week_interval": orNull(q.Schedule.WeekInterval),
			"month_interval": orNull(q.Schedule.MonthInterval), "duration": orNull(q.Schedule.Duration),
		},
		"answers_count": len(q.Answers), "answers_url": answersURL,
		"comments_count":   0,
		"subscription_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/subscription.json", a.root(), p.ID, q.ID),
		"created_at":       q.CreatedAt, "updated_at": q.UpdatedAt,
	}
	if q.Creator != nil {
		answer["creator"] = a.person(*q.Creator)
	}
	return answer
}

func (a *Account) answerJSON(p *Project, q *Question, n Answer) map[string]any {
	var groupOn any
	if n.GroupOn != "" {
		groupOn = n.GroupOn
	}
	answer := map[string]any{
		"id": n.ID, "status": n.status(), "title": "Answer", "group_on": groupOn,
		"content": n.Content, "content_attachments": a.filesJSON(n.Files),
		"parent":         map[string]any{"id": q.ID, "type": "Question", "title": q.Title},
		"comments_count": len(n.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, n.ID),
		"boosts_count": len(n.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, n.ID),
		"created_at": n.CreatedAt, "updated_at": n.UpdatedAt,
	}
	if n.Creator != nil {
		answer["creator"] = a.person(*n.Creator)
	}
	return answer
}
