package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

// A to-do series makes a to-do again and again on the days of its schedule.
// It is read through its list and never deleted, only stopped; the to-dos it
// made stay ordinary to-dos that name it.

// TodoSeries is one series with what its occurrences start from and when the
// next one comes. The days are Minka's own reckoning, on the installation's
// today.
type TodoSeries struct {
	ID        string              `json:"id"`
	ProjectID string              `json:"projectId"`
	ToolID    string              `json:"toolId"`
	ListID    string              `json:"listId"`
	GroupID   *string             `json:"groupId"` // nil where it makes its to-dos loose in the list
	Project   ProjectReference    `json:"project"`
	Tool      TodosToolReference  `json:"tool"`
	List      TodoListReference   `json:"list"`
	Group     *TodoGroupReference `json:"group"`
	Title     string              `json:"title"`
	Notes     string              `json:"notes"` // HTML
	Schedule  RepetitionSchedule  `json:"schedule"`
	LeadDays  int                 `json:"leadDays"` // how many days before its day an occurrence shows
	// State is running, stopped, or finished: not stopped, but with no
	// further day before its end.
	State        string  `json:"state"`
	EndedAt      *string `json:"endedAt"`
	NextOccursOn *string `json:"nextOccursOn"` // nil while stopped or finished
	// NextShowsOn is the day the next occurrence appears in the list, nil
	// where it comes with the next round or is there already, and while the
	// list is archived.
	NextShowsOn *string           `json:"nextShowsOn"`
	CanShowNow  bool              `json:"canShowNow"`
	Assignees   []PersonReference `json:"assignees"`
	Notified    []PersonReference `json:"notified"`
	CreatedAt   string            `json:"createdAt"`
	Creator     PersonReference   `json:"creator"`
	UpdatedAt   string            `json:"updatedAt"` // what an edit expects
	AppURL      string            `json:"appUrl"`
}

// RepetitionSchedule is on which days a series falls. A field its rhythm does
// not use is nil or empty.
type RepetitionSchedule struct {
	Rhythm   string  `json:"rhythm"`   // daily, weekly, monthlyOnDay, monthlyOnWeekday or yearly
	Weekdays []int   `json:"weekdays"` // ISO numbers, Monday 1; daily and weekly
	Interval int     `json:"interval"` // in weeks or months
	Day      *int    `json:"day"`      // of the month; monthlyOnDay and yearly
	Weekday  *int    `json:"weekday"`  // monthlyOnWeekday
	Instance *int    `json:"instance"` // 1 to 4, or -1 for the last; monthlyOnWeekday
	Month    *int    `json:"month"`    // yearly
	StartsOn string  `json:"startsOn"`
	EndsOn   *string `json:"endsOn"` // the last day an occurrence may fall on
}

// TodoSeriesReference names the series a to-do is an occurrence of and the day
// it stands for.
type TodoSeriesReference struct {
	ID       string  `json:"id"`
	OccursOn *string `json:"occursOn"`
}

// ScheduleInput is what a request says about a schedule. Only the fields set
// go out: Minka takes the rest from the stored schedule, or from the first
// day, so a client that echoed them would write over what it never meant to
// change. An empty EndsOn is sent as null, which takes the end off.
type ScheduleInput struct {
	Rhythm   *string
	Weekdays *[]int
	Interval *int
	Day      *int
	Weekday  *int
	Instance *int
	Month    *int
	StartsOn *string
	EndsOn   *string
}

// IsEmpty says whether the input names no field at all.
func (in ScheduleInput) IsEmpty() bool {
	return in == ScheduleInput{}
}

func (in ScheduleInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Rhythm != nil {
		fields["rhythm"] = *in.Rhythm
	}
	if in.Weekdays != nil {
		fields["weekdays"] = append([]int{}, *in.Weekdays...)
	}
	for name, value := range map[string]*int{"interval": in.Interval, "day": in.Day, "weekday": in.Weekday, "instance": in.Instance, "month": in.Month} {
		if value != nil {
			fields[name] = *value
		}
	}
	if in.StartsOn != nil {
		fields["startsOn"] = *in.StartsOn
	}
	if in.EndsOn != nil {
		fields["endsOn"] = nullIfEmpty(*in.EndsOn)
	}
	return json.Marshal(fields)
}

// RepeatTodoInput makes a to-do of the list the first occurrence of a series.
// A schedule left out is weekly on the weekday of the to-do's due day, and a
// lead left out is Minka's, seven days.
type RepeatTodoInput struct {
	TodoID   string         `json:"todoId"`
	Schedule *ScheduleInput `json:"schedule,omitempty"`
	LeadDays *int           `json:"leadDays,omitempty"`
}

// UpdateTodoSeriesInput changes a series for the occurrences made from now on.
// A nil field is left out and stays as it is. A list of people is the whole
// set, so an empty one takes everybody off: it is sent as [] and never as
// null, which Minka refuses. ExpectedUpdatedAt writes only over the series as
// it was read.
type UpdateTodoSeriesInput struct {
	Title             *string
	Schedule          *ScheduleInput
	LeadDays          *int
	AssigneeIDs       *[]string
	NotifiedIDs       *[]string
	ExpectedUpdatedAt *string
}

func (in UpdateTodoSeriesInput) MarshalJSON() ([]byte, error) {
	fields := map[string]any{}
	if in.Title != nil {
		fields["title"] = *in.Title
	}
	if in.Schedule != nil {
		fields["schedule"] = *in.Schedule
	}
	if in.LeadDays != nil {
		fields["leadDays"] = *in.LeadDays
	}
	if in.AssigneeIDs != nil {
		fields["assigneeIds"] = append([]string{}, *in.AssigneeIDs...)
	}
	if in.NotifiedIDs != nil {
		fields["notifiedIds"] = append([]string{}, *in.NotifiedIDs...)
	}
	if in.ExpectedUpdatedAt != nil {
		fields["expectedUpdatedAt"] = *in.ExpectedUpdatedAt
	}
	return json.Marshal(fields)
}

// TodoSeriesOfList lists every series of a list, stopped ones too, the one
// made first first.
func (c *Client) TodoSeriesOfList(ctx context.Context, listID string) (Result[[]TodoSeries], error) {
	return request[[]TodoSeries](ctx, c, http.MethodGet, todoListPath(listID)+"/series", nil)
}

// RepeatTodo makes a series of a to-do of the list; the series starts on the
// to-do's due day.
func (c *Client) RepeatTodo(ctx context.Context, listID string, input RepeatTodoInput) (Result[TodoSeries], error) {
	return request[TodoSeries](ctx, c, http.MethodPost, todoListPath(listID)+"/series", input)
}

func (c *Client) TodoSeries(ctx context.Context, seriesID string) (Result[TodoSeries], error) {
	return request[TodoSeries](ctx, c, http.MethodGet, todoSeriesPath(seriesID), nil)
}

// UpdateTodoSeries changes a series. One written since the moment the input
// expects is left as it stands and answered with 409 todoSeriesChanged, whose
// body carries it under "series".
func (c *Client) UpdateTodoSeries(ctx context.Context, seriesID string, input UpdateTodoSeriesInput) (Result[TodoSeries], error) {
	return request[TodoSeries](ctx, c, http.MethodPatch, todoSeriesPath(seriesID), input)
}

// StopTodoSeries ends a series; the to-dos it made stay.
func (c *Client) StopTodoSeries(ctx context.Context, seriesID string) (Result[TodoSeries], error) {
	return request[TodoSeries](ctx, c, http.MethodPost, todoSeriesPath(seriesID)+"/stop", nil)
}

// ResumeTodoSeries continues a stopped series with its first day after today.
func (c *Client) ResumeTodoSeries(ctx context.Context, seriesID string) (Result[TodoSeries], error) {
	return request[TodoSeries](ctx, c, http.MethodPost, todoSeriesPath(seriesID)+"/resume", nil)
}

// ShowTodoSeriesNow makes the upcoming occurrence at once. The answer is the
// series; the new to-do is named by the Location header alone.
func (c *Client) ShowTodoSeriesNow(ctx context.Context, seriesID string) (Result[TodoSeries], error) {
	return request[TodoSeries](ctx, c, http.MethodPost, todoSeriesPath(seriesID)+"/show-now", nil)
}

func todoSeriesPath(seriesID string) string {
	return "/todo-series/" + url.PathEscape(seriesID)
}
