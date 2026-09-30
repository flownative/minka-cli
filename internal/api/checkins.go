package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// The Check-ins tool of a project: questions asked of chosen people on a
// schedule, the answers to them, and the reminders of the person of the token.
// A question and an answer are addressed by their own identifier alone.

// CheckinsTool is one Check-ins tool of a project.
type CheckinsTool struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

// CheckinsToolReference names the tool a question stands in.
type CheckinsToolReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// CheckinSchedule says when a question is asked. The time of day holds in the
// zone of each person asked, so the schedule names none.
type CheckinSchedule struct {
	Rhythm    string `json:"rhythm"`    // daily, weekly, everyOtherWeek or monthly
	Weekdays  []int  `json:"weekdays"`  // ISO numbers, Monday 1 to Sunday 7; exactly one unless daily
	TimeOfDay string `json:"timeOfDay"` // HH:MM on the 24-hour clock
	StartsOn  string `json:"startsOn"`  // YYYY-MM-DD, what every other week is counted from
}

// CheckinQuestion is one question with its schedule and the people it asks.
type CheckinQuestion struct {
	ID        string                `json:"id"`
	ProjectID string                `json:"projectId"`
	ToolID    string                `json:"toolId"`
	Project   ProjectReference      `json:"project"`
	Tool      CheckinsToolReference `json:"tool"`
	Title     string                `json:"title"`
	Schedule  CheckinSchedule       `json:"schedule"`
	Paused    bool                  `json:"paused"`
	// Asked are the people asked who are still members and may see the
	// question, in the order of their names.
	Asked  []PersonReference `json:"asked"`
	AppURL string            `json:"appUrl"`
}

// CheckinQuestionReference names the question an answer or a reminder
// belongs to.
type CheckinQuestionReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// CheckinAnswerLine is one answer as the answers to a question name it; its
// source, its files and its comments come with the answer's own address.
type CheckinAnswerLine struct {
	ID           string          `json:"id"`
	QuestionID   string          `json:"questionId"`
	GroupOn      string          `json:"groupOn"` // the day it belongs to, YYYY-MM-DD
	Body         string          `json:"body"`
	CommentCount int             `json:"commentCount"`
	CreatedAt    string          `json:"createdAt"`
	EditedAt     *string         `json:"editedAt"` // nil unless its writer changed it
	Author       PersonReference `json:"author"`
	AppURL       string          `json:"appUrl"`
}

// CheckinAnswer is one answer with everything said under it.
type CheckinAnswer struct {
	ID           string                   `json:"id"`
	ProjectID    string                   `json:"projectId"`
	ToolID       string                   `json:"toolId"`
	QuestionID   string                   `json:"questionId"`
	Project      ProjectReference         `json:"project"`
	Tool         CheckinsToolReference    `json:"tool"`
	Question     CheckinQuestionReference `json:"question"`
	GroupOn      string                   `json:"groupOn"`
	Body         string                   `json:"body"`
	BodySource   string                   `json:"bodySource"` // the body as the editor opens it
	Attachments  []Attachment             `json:"attachments"`
	CommentCount int                      `json:"commentCount"`
	CreatedAt    string                   `json:"createdAt"`
	EditedAt     *string                  `json:"editedAt"`
	Author       PersonReference          `json:"author"`
	Comments     []Comment                `json:"comments"` // oldest first
	AppURL       string                   `json:"appUrl"`
}

// CheckinReminder is a question put to the person of the token for one day and
// not answered for it yet.
type CheckinReminder struct {
	Question CheckinQuestionReference `json:"question"`
	Project  ProjectReference         `json:"project"`
	Tool     CheckinsToolReference    `json:"tool"`
	AskedFor string                   `json:"askedFor"` // the day asked for, YYYY-MM-DD
	AskedAt  string                   `json:"askedAt"`
	AppURL   string                   `json:"appUrl"`
}

// CreateCheckinQuestionInput sets up a question. What is left empty is left
// out of the body, and the server says what it misses.
type CreateCheckinQuestionInput struct {
	Title     string   `json:"title"`
	Rhythm    string   `json:"rhythm,omitempty"`
	Weekdays  []int    `json:"weekdays,omitempty"`
	TimeOfDay string   `json:"timeOfDay,omitempty"`
	AskedIDs  []string `json:"askedIds,omitempty"`
}

// UpdateCheckinQuestionInput changes a question. A nil field is left out and
// stays as it is. Weekdays and AskedIDs are the whole set the question is to
// carry.
type UpdateCheckinQuestionInput struct {
	Title     *string
	Rhythm    *string
	Weekdays  *[]int
	TimeOfDay *string
	AskedIDs  *[]string
}

func (in UpdateCheckinQuestionInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Title != nil {
		fields["title"] = *in.Title
	}
	if in.Rhythm != nil {
		fields["rhythm"] = *in.Rhythm
	}
	if in.Weekdays != nil {
		fields["weekdays"] = append([]int{}, *in.Weekdays...)
	}
	if in.TimeOfDay != nil {
		fields["timeOfDay"] = *in.TimeOfDay
	}
	if in.AskedIDs != nil {
		fields["askedIds"] = append([]string{}, *in.AskedIDs...)
	}
	return json.Marshal(fields)
}

// CreateCheckinAnswerInput answers a question. Body is HTML. AskedFor names
// the day of a reminder; left empty, the answer belongs to the latest day the
// question was asked.
type CreateCheckinAnswerInput struct {
	Body     string `json:"body"`
	AskedFor string `json:"askedFor,omitempty"`
}

// CheckinAnswersQuery narrows and pages the answers to a question. Before is
// the last answer of the page before.
type CheckinAnswersQuery struct {
	PersonID string
	On       string
	Before   string
	Limit    int
}

// CheckinAnswersPageMax is the largest page of answers the server serves. It
// cuts a larger one down to this without a word, and the answer carries no
// sign of its end but being shorter than asked for.
const CheckinAnswersPageMax = 100

// ErrCheckinAnswersPageOutOfBounds is a page size the server would refuse or
// cut.
var ErrCheckinAnswersPageOutOfBounds = errors.New("a page of answers holds 1 to 100 answers")

// CheckinRemindersMax is the most reminders the server hands out. There is no
// page after it.
const CheckinRemindersMax = 100

// CheckinsTools lists the Check-ins tools of a project that are in use, in the
// order of the project page.
func (c *Client) CheckinsTools(ctx context.Context, projectID string) (Result[[]CheckinsTool], error) {
	return request[[]CheckinsTool](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/checkins", nil)
}

// CheckinsTool reads a Check-ins tool by its identifier alone.
func (c *Client) CheckinsTool(ctx context.Context, toolID string) (Result[Tool], error) {
	return request[Tool](ctx, c, http.MethodGet, checkinsToolPath(toolID), nil)
}

// CheckinQuestions lists the questions of a tool, the oldest first.
func (c *Client) CheckinQuestions(ctx context.Context, toolID string) (Result[[]CheckinQuestion], error) {
	return request[[]CheckinQuestion](ctx, c, http.MethodGet, checkinsToolPath(toolID)+"/questions", nil)
}

// CreateCheckinQuestion sets up a question; it starts on the writer's today.
func (c *Client) CreateCheckinQuestion(ctx context.Context, toolID string, input CreateCheckinQuestionInput) (Result[CheckinQuestion], error) {
	return request[CheckinQuestion](ctx, c, http.MethodPost, checkinsToolPath(toolID)+"/questions", input)
}

func (c *Client) CheckinQuestion(ctx context.Context, questionID string) (Result[CheckinQuestion], error) {
	return request[CheckinQuestion](ctx, c, http.MethodGet, questionPath(questionID), nil)
}

func (c *Client) UpdateCheckinQuestion(ctx context.Context, questionID string, input UpdateCheckinQuestionInput) (Result[CheckinQuestion], error) {
	return request[CheckinQuestion](ctx, c, http.MethodPatch, questionPath(questionID), input)
}

// PauseCheckinQuestion stops asking a question; a paused one stays as it is.
func (c *Client) PauseCheckinQuestion(ctx context.Context, questionID string) (Result[CheckinQuestion], error) {
	return request[CheckinQuestion](ctx, c, http.MethodPost, questionPath(questionID)+"/pause", nil)
}

// ResumeCheckinQuestion asks a paused question again; a running one stays as
// it is.
func (c *Client) ResumeCheckinQuestion(ctx context.Context, questionID string) (Result[CheckinQuestion], error) {
	return request[CheckinQuestion](ctx, c, http.MethodPost, questionPath(questionID)+"/resume", nil)
}

// CheckinAnswers reads one page of the answers to a question, the newest day
// first. The limit is always sent and always within the bounds, so the
// server's silent cut is unreachable from here.
func (c *Client) CheckinAnswers(ctx context.Context, questionID string, query CheckinAnswersQuery) (Result[[]CheckinAnswerLine], error) {
	if query.Limit < 1 || query.Limit > CheckinAnswersPageMax {
		return Result[[]CheckinAnswerLine]{}, ErrCheckinAnswersPageOutOfBounds
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(query.Limit))
	if query.PersonID != "" {
		values.Set("personId", query.PersonID)
	}
	if query.On != "" {
		values.Set("on", query.On)
	}
	if query.Before != "" {
		values.Set("before", query.Before)
	}
	return request[[]CheckinAnswerLine](ctx, c, http.MethodGet, questionPath(questionID)+"/answers?"+values.Encode(), nil)
}

// CreateCheckinAnswer answers a question as the person of the token.
func (c *Client) CreateCheckinAnswer(ctx context.Context, questionID string, input CreateCheckinAnswerInput) (Result[CheckinAnswer], error) {
	return request[CheckinAnswer](ctx, c, http.MethodPost, questionPath(questionID)+"/answers", input)
}

func (c *Client) CheckinAnswer(ctx context.Context, answerID string) (Result[CheckinAnswer], error) {
	return request[CheckinAnswer](ctx, c, http.MethodGet, answerPath(answerID), nil)
}

// UpdateCheckinAnswer gives an answer of one's own a new body. The day it
// belongs to stays.
func (c *Client) UpdateCheckinAnswer(ctx context.Context, answerID, body string) (Result[CheckinAnswer], error) {
	return request[CheckinAnswer](ctx, c, http.MethodPatch, answerPath(answerID), map[string]string{"body": body})
}

// MyCheckinReminders lists the reminders the person of the token has not
// answered yet, across all projects, the one asked last first.
func (c *Client) MyCheckinReminders(ctx context.Context) (Result[[]CheckinReminder], error) {
	return request[[]CheckinReminder](ctx, c, http.MethodGet, "/me/checkin-reminders", nil)
}

func checkinsToolPath(toolID string) string {
	return "/checkins/" + url.PathEscape(toolID)
}

func questionPath(questionID string) string {
	return "/questions/" + url.PathEscape(questionID)
}

func answerPath(answerID string) string {
	return "/answers/" + url.PathEscape(answerID)
}
