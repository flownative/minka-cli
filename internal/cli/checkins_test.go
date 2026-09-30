package cli

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/fakeapi"
)

func TestTheCheckinsToolsOfAProjectAreListed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	e.server.CheckinsTools = append(e.server.CheckinsTools, &fakeapi.CheckinsTool{ID: unknownID, ProjectID: f.Project.ID, Title: "Old check-ins", Archived: true})

	env := runOK(t, e, "checkins", "tools", f.Project.ID)
	tools := listOfObjects(t, env.Data)
	if len(tools) != 1 || tools[0]["id"] != f.CheckinsTool.ID || tools[0]["title"] != "Check-ins" {
		t.Fatalf("tools %v: a tool out of use is not listed", tools)
	}
	// Out of use, a tool is still read at its own address.
	runOK(t, e, "checkins", "list", unknownID)

	_, r := e.runJSON("", "checkins", "tools", f.OtherProject.ID)
	expectCode(t, r, ExitNotFound)
}

func TestTheQuestionsOfAToolAreListedOldestFirstWithWhomTheyShowAsAsked(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "list", f.CheckinsTool.ID)
	questions := listOfObjects(t, env.Data)
	var ids []string
	for _, question := range questions {
		ids = append(ids, question["id"].(string))
	}
	if !slices.Equal(ids, []string{f.DailyQuestion.ID, f.WeeklyQuestion.ID, f.PausedQuestion.ID}) {
		t.Fatalf("questions %v", ids)
	}
	// The client is asked and not shown the question, so it names them nowhere.
	if asked := idsOf(t, questions[0], "asked"); !slices.Equal(asked, []string{f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("asked %v", asked)
	}
	schedule := questions[0]["schedule"].(map[string]any)
	if schedule["rhythm"] != "daily" || schedule["timeOfDay"] != "17:00" || schedule["startsOn"] != "2026-09-01" {
		t.Fatalf("schedule %v", schedule)
	}

	text := e.run("", "checkins", "list", f.CheckinsTool.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{
		"What did you do today? (daily on Mon, Tue, Wed, Thu, Fri at 17:00)",
		"How was the month? (monthly on the first Fri at 16:00, paused)",
	} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}

	for _, tool := range []string{f.OtherCheckinsTool.ID, unknownID} {
		env, r := e.runJSON("", "checkins", "list", tool)
		expectCode(t, r, ExitNotFound)
		if env.Hint != checkinsToolNotFoundHint {
			t.Fatalf("hint %q", env.Hint)
		}
	}
}

func TestAQuestionIsReadWithItsScheduleAndThePeopleItAsks(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "show", f.WeeklyQuestion.ID)
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "pause"), "checkins pause "+f.WeeklyQuestion.ID) {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}
	if !strings.Contains(crumbFor(t, runOK(t, e, "checkins", "show", f.PausedQuestion.ID).Breadcrumbs, "resume"), "checkins resume") {
		t.Fatal("a paused question offers no resume")
	}

	text := e.run("", "checkins", "show", f.DailyQuestion.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"# What did you do today?", "Asked: daily on Mon, Tue, Wed, Thu, Fri at 17:00", "Started: 2026-09-01", "Paused: no", "Asks: Anke, Bruno"} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}

	f.WeeklyQuestion.Trashed = true
	for _, id := range []string{f.WeeklyQuestion.ID, f.OtherQuestion.ID, unknownID} {
		env, r := e.runJSON("", "checkins", "show", id)
		expectCode(t, r, ExitNotFound)
		if env.Hint != questionNotFoundHint {
			t.Fatalf("hint %q", env.Hint)
		}
	}
}

func TestAQuestionIsSetUpWithWhatWasGivenAndMinkaKeepsItInItsOwnForm(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "checkins", "create", "--tool", f.CheckinsTool.ID, "--title", " What did you learn? ", "--rhythm", "daily",
		"--weekday", "fri,Monday", "--weekday", "3", "--weekday", "fri", "--time", "9:00", "--ask", "me", "--ask", f.Anke.ID, "--ask", "me")
	expectCode(t, r, ExitOK)
	body := e.lastWriteBody()
	if body["title"] != " What did you learn? " || body["rhythm"] != "daily" || body["timeOfDay"] != "9:00" {
		t.Fatalf("sent %v", body)
	}
	// Everything goes out as it was given: taking a weekday or a person once is
	// Minka's rule.
	if days, _ := json.Marshal(body["weekdays"]); string(days) != "[5,1,3,5]" {
		t.Fatalf("sent weekdays %s", days)
	}
	if people := stringsIn(body["askedIds"]); !slices.Equal(people, []string{f.Bruno.ID, f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("sent people %v", people)
	}
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPost || last.Path != "/api/v1/checkins/"+f.CheckinsTool.ID+"/questions" {
		t.Fatalf("sent %s %s", last.Method, last.Path)
	}

	question := decode(t, env.Data)
	schedule := question["schedule"].(map[string]any)
	if days, _ := json.Marshal(schedule["weekdays"]); string(days) != "[1,3,5]" || schedule["timeOfDay"] != "09:00" || schedule["startsOn"] != "2026-09-13" {
		t.Fatalf("schedule %v", schedule)
	}
	if question["title"] != "What did you learn?" || env.Notice != "" {
		t.Fatalf("question %v, notice %q", question, env.Notice)
	}
	if asked := idsOf(t, question, "asked"); !slices.Equal(asked, []string{f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("asked %v", asked)
	}
	if !strings.Contains(env.Summary, "daily on Mon, Wed, Fri at 09:00") {
		t.Fatalf("summary %q", env.Summary)
	}
}

// A weekday is a number or a name. A number goes out as it is, so that Minka
// says what it makes of one outside the week; a word that names no weekday
// cannot be sent at all.
func TestAWeekdayGoesOutAsANumberAndAWordThatIsNoneIsRefused(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "checkins", "create", "--tool", f.CheckinsTool.ID, "--title", "Q", "--rhythm", "daily", "--weekday", "8", "--time", "09:00", "--ask", "me")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "questionScheduleInvalid" || !strings.Contains(env.Hint, "1 (Monday) to 7") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	if days, _ := json.Marshal(e.lastWriteBody()["weekdays"]); string(days) != "[8]" {
		t.Fatalf("sent weekdays %s", days)
	}

	writes := len(e.server.WritingRequests())
	for _, args := range [][]string{
		{"checkins", "create", "--tool", f.CheckinsTool.ID, "--title", "Q", "--weekday", "someday"},
		{"checkins", "edit", f.DailyQuestion.ID, "--weekday", "mon,funday"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.WritingRequests()) != writes {
		t.Fatal("a weekday that is none went out")
	}
}

// The ID of a tool names no project, so a new question takes a person by ID or
// as me, and nothing is looked up.
func TestANewQuestionAsksByIDOrMe(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	requests := len(e.server.Requests())
	env, r := e.runJSON("", "checkins", "create", "--tool", f.CheckinsTool.ID, "--title", "Q", "--ask", "anke@example.com")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "people list") || len(e.server.Requests()) != requests {
		t.Fatalf("hint %q, %d requests", env.Hint, len(e.server.Requests())-requests)
	}
}

// A client or an agent of one is refused like a stranger where the question is
// not shown to clients, and nothing is written.
func TestAClientTheQuestionIsNotShownToIsRefusedWhenNamed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	agent := &fakeapi.Person{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e503", Name: "Claude (Nele)", Email: "claude-nele@agents.invalid", PrincipalID: f.Client.ID}
	e.server.People[agent.ID] = agent
	f.Project.MemberIDs = append(f.Project.MemberIDs, agent.ID)
	questions := len(f.CheckinsTool.Questions)

	for _, person := range []string{f.Client.ID, agent.ID} {
		env, r := e.runJSON("", "checkins", "create", "--tool", f.CheckinsTool.ID, "--title", "Q", "--rhythm", "weekly", "--weekday", "mon",
			"--time", "09:00", "--ask", person, "--ask", "me")
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePersonNotAProjectMember || !strings.Contains(env.Hint, "agent") {
			t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
		}
	}
	if len(f.CheckinsTool.Questions) != questions {
		t.Fatal("a question refused was set up")
	}

	before := slices.Clone(f.DailyQuestion.AskedIDs)
	env, r := e.runJSON("", "checkins", "edit", f.DailyQuestion.ID, "--ask", "Nele", "--ask", "anke@example.com")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonNotAProjectMember || !slices.Equal(f.DailyQuestion.AskedIDs, before) {
		t.Fatalf("apiCode %v, asked %v", env.Meta["apiCode"], f.DailyQuestion.AskedIDs)
	}

	// Once the question is shown to clients, the client is asked.
	f.DailyQuestion.ShownToClients = true
	env = runOK(t, e, "checkins", "edit", f.DailyQuestion.ID, "--ask", f.Client.ID)
	if env.Notice != "" || !slices.Equal(idsOf(t, decode(t, env.Data), "asked"), []string{f.Client.ID}) {
		t.Fatalf("notice %q, data %s", env.Notice, env.Data)
	}
}

func TestAnEditOfAQuestionSendsOnlyWhatIsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "edit", f.DailyQuestion.ID, "--title", "What did you get done?")
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "What did you get done?" {
		t.Fatalf("sent %v", body)
	}
	if writes := e.server.WritingRequests(); writes[len(writes)-1].Method != http.MethodPatch {
		t.Fatalf("sent %s", writes[len(writes)-1].Method)
	}
	if !strings.Contains(env.Summary, "daily on Mon, Tue, Wed, Thu, Fri at 17:00") || env.Notice != "" {
		t.Fatalf("summary %q, notice %q", env.Summary, env.Notice)
	}

	runOK(t, e, "checkins", "edit", f.DailyQuestion.ID, "--rhythm", "weekly", "--weekday", "fri", "--time", "16:30")
	if body := e.lastWriteBody(); body["rhythm"] != "weekly" || body["timeOfDay"] != "16:30" || len(body) != 3 {
		t.Fatalf("sent %v", body)
	}
	if f.DailyQuestion.Rhythm != "weekly" || !slices.Equal(f.DailyQuestion.Weekdays, []int{5}) || f.DailyQuestion.StartsOn != "2026-09-01" {
		t.Fatalf("question %+v", f.DailyQuestion)
	}

	requests := len(e.server.Requests())
	_, r := e.runJSON("", "checkins", "edit", f.DailyQuestion.ID)
	expectCode(t, r, ExitUsage)
	if len(e.server.Requests()) != requests {
		t.Fatal("an edit without a change went out")
	}
}

// The weekdays an edit leaves out are the question's own, so a question asked
// on several days refuses another rhythm without a weekday.
func TestARhythmWithoutAWeekdayIsHeldAgainstTheQuestionsOwn(t *testing.T) {
	e := newTestEnv(t)

	env, r := e.runJSON("", "checkins", "edit", e.fixture.DailyQuestion.ID, "--rhythm", "weekly")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != "questionScheduleInvalid" || !strings.Contains(env.Hint, "question's own") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// An edit without --ask leaves the people asked as they are, also on a
// question that shows nobody; an empty set is refused on a new question.
func TestAnEditWithoutPeopleKeepsWhoIsAsked(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.WeeklyQuestion.AskedIDs = []string{f.Client.ID, f.Stranger.ID}

	runOK(t, e, "checkins", "edit", f.WeeklyQuestion.ID, "--title", "Renamed")
	if _, named := e.lastWriteBody()["askedIds"]; named {
		t.Fatalf("an edit without --ask sent the people: %v", e.lastWriteBody())
	}
	if !slices.Equal(f.WeeklyQuestion.AskedIDs, []string{f.Client.ID, f.Stranger.ID}) || f.WeeklyQuestion.Title != "Renamed" {
		t.Fatalf("question %+v", f.WeeklyQuestion)
	}
	before := slices.Clone(f.DailyQuestion.AskedIDs)
	runOK(t, e, "checkins", "edit", f.DailyQuestion.ID, "--time", "08:00")
	if !slices.Equal(f.DailyQuestion.AskedIDs, before) {
		t.Fatalf("asked %v, was %v", f.DailyQuestion.AskedIDs, before)
	}

	env, r := e.runJSON("", "checkins", "create", "--tool", f.CheckinsTool.ID, "--title", "Q", "--rhythm", "daily", "--weekday", "1", "--time", "09:00")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeQuestionNobodyAsked || !strings.Contains(env.Hint, "--ask") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

// --ask takes off only whom Minka shows as asked, so the people read from a
// question and named back take nobody off: not a member who left the project,
// and not a client the question is not shown to.
func TestThePeopleShownAndNamedBackTakeNobodyOff(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.DailyQuestion.AskedIDs = append(f.DailyQuestion.AskedIDs, f.Stranger.ID)

	shown := idsOf(t, decode(t, runOK(t, e, "checkins", "show", f.DailyQuestion.ID).Data), "asked")
	if slices.Contains(shown, f.Client.ID) || slices.Contains(shown, f.Stranger.ID) {
		t.Fatalf("shown as asked: %v", shown)
	}
	var back []string
	for _, id := range shown {
		back = append(back, "--ask", id)
	}
	runOK(t, e, append([]string{"checkins", "edit", f.DailyQuestion.ID}, back...)...)
	for _, id := range []string{f.Anke.ID, f.Bruno.ID, f.Client.ID, f.Stranger.ID} {
		if !slices.Contains(f.DailyQuestion.AskedIDs, id) {
			t.Fatalf("%s taken off: %v", id, f.DailyQuestion.AskedIDs)
		}
	}

	runOK(t, e, "checkins", "edit", f.DailyQuestion.ID, "--ask", f.Anke.ID)
	if slices.Contains(f.DailyQuestion.AskedIDs, f.Bruno.ID) || !slices.Contains(f.DailyQuestion.AskedIDs, f.Client.ID) || !slices.Contains(f.DailyQuestion.AskedIDs, f.Stranger.ID) {
		t.Fatalf("asked %v, want Bruno alone taken off", f.DailyQuestion.AskedIDs)
	}
}

// Every other week is counted from the first of the question's weekday on or
// after the day it started, which no edit moves, so the weekday decides which
// weeks are asked. An edit of its rhythm or its weekday names them, also where
// the question was asked every other week before, and the dates it names are
// the ones Minka answers in.
func TestAnEditOfEveryOtherWeekSaysWhichWeeksItAsksIn(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	// The question started on Tuesday, 2026-09-01.
	env := runOK(t, e, "checkins", "edit", f.WeeklyQuestion.ID, "--rhythm", "everyOtherWeek")
	if !strings.Contains(env.Notice, "counted from 2026-09-07, the first Mon on or after 2026-09-01") || !strings.Contains(env.Notice, "2026-09-07, 2026-09-21, 2026-10-05") {
		t.Fatalf("notice %q", env.Notice)
	}
	if !strings.Contains(env.Summary, "first on 2026-09-07") {
		t.Fatalf("summary %q", env.Summary)
	}
	if day := decode(t, runOK(t, e, "checkins", "answers", "create", f.WeeklyQuestion.ID, "Monday").Data)["groupOn"]; day != "2026-09-07" {
		t.Fatalf("an answer on Monday's weeks belongs to %v", day)
	}

	env = runOK(t, e, "checkins", "edit", f.WeeklyQuestion.ID, "--weekday", "wed")
	if !strings.Contains(env.Notice, "counted from 2026-09-02, the first Wed on or after 2026-09-01") || !strings.Contains(env.Notice, "2026-09-02, 2026-09-16, 2026-09-30") {
		t.Fatalf("notice %q", env.Notice)
	}
	if day := decode(t, runOK(t, e, "checkins", "answers", "create", f.WeeklyQuestion.ID, "Wednesday").Data)["groupOn"]; day != "2026-09-02" {
		t.Fatalf("an answer on Wednesday's weeks belongs to %v", day)
	}
	if env := runOK(t, e, "checkins", "edit", f.WeeklyQuestion.ID, "--title", "Renamed"); env.Notice != "" {
		t.Fatalf("notice %q on an edit that leaves the schedule", env.Notice)
	}
	if env := runOK(t, e, "checkins", "edit", f.DailyQuestion.ID, "--rhythm", "weekly", "--weekday", "mon"); env.Notice != "" {
		t.Fatalf("notice %q on a weekly question", env.Notice)
	}
}

func TestPausingAndResumingSucceedTwice(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for i := 0; i < 2; i++ {
		env := runOK(t, e, "checkins", "pause", f.DailyQuestion.ID)
		if decode(t, env.Data)["paused"] != true || !strings.Contains(env.Summary, "is paused") {
			t.Fatalf("pause %d: %s, summary %q", i, env.Data, env.Summary)
		}
	}
	for i := 0; i < 2; i++ {
		env := runOK(t, e, "checkins", "resume", f.DailyQuestion.ID)
		if decode(t, env.Data)["paused"] != false || !strings.Contains(env.Summary, "asked again") {
			t.Fatalf("resume %d: %s, summary %q", i, env.Data, env.Summary)
		}
	}
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPost || !strings.HasSuffix(last.Path, "/questions/"+f.DailyQuestion.ID+"/resume") {
		t.Fatalf("sent %s %s", last.Method, last.Path)
	}
}

// Minka holds the body of a question before it looks for the tool, and asks
// whom the question asks only after it found it: nobody, then the archived
// project, then a stranger.
func TestTheRefusalsOfANewQuestionComeInMinkasOrder(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	valid := func(tool string, more ...string) []string {
		return append([]string{"checkins", "create", "--tool", tool, "--title", "Q", "--rhythm", "daily", "--weekday", "mon", "--time", "09:00"}, more...)
	}

	for name, c := range map[string]struct {
		args []string
		exit int
		code string
	}{
		"a title before the tool":            {[]string{"checkins", "create", "--tool", unknownID, "--title", " ", "--ask", f.Stranger.ID}, ExitValidation, "questionTitleMissing"},
		"the weekdays before the tool":       {[]string{"checkins", "create", "--tool", unknownID, "--title", "Q", "--time", "09:00"}, ExitValidation, apiCodeQuestionWeekdaysMissing},
		"the time before the rhythm":         {[]string{"checkins", "create", "--tool", unknownID, "--title", "Q", "--weekday", "1", "--time", "25:00"}, ExitValidation, apiCodeQuestionTimeInvalid},
		"no rhythm before the tool":          {[]string{"checkins", "create", "--tool", unknownID, "--title", "Q", "--weekday", "1", "--time", "09:00"}, ExitValidation, apiCodeQuestionScheduleInvalid},
		"the tool before nobody":             {valid(unknownID), ExitNotFound, ""},
		"the tool before a stranger":         {valid(f.OtherCheckinsTool.ID, "--ask", f.Stranger.ID), ExitNotFound, ""},
		"nobody":                             {valid(f.CheckinsTool.ID), ExitValidation, apiCodeQuestionNobodyAsked},
		"a stranger":                         {valid(f.CheckinsTool.ID, "--ask", f.Stranger.ID), ExitValidation, apiCodePersonNotAProjectMember},
		"somebody that is no ID, before 404": {[]string{"checkins", "create", "--tool", f.CheckinsTool.ID, "--title", " ", "--ask", "Nele"}, ExitUsage, ""},
	} {
		env, r := e.runJSON("", c.args...)
		if r.code != c.exit || c.code != "" && env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: exit %d, apiCode %v, want %d %s", name, r.code, env.Meta["apiCode"], c.exit, c.code)
		}
		if c.code == apiCodePersonNotAProjectMember && env.Hint != askedPeopleHint {
			t.Fatalf("%s: hint %q", name, env.Hint)
		}
	}

	f.Project.Archived = true
	for name, c := range map[string]struct {
		args []string
		code string
	}{
		"nobody before the archive":     {valid(f.CheckinsTool.ID), apiCodeQuestionNobodyAsked},
		"the archive before a stranger": {valid(f.CheckinsTool.ID, "--ask", f.Stranger.ID), apiCodeProjectArchived},
	} {
		env, r := e.runJSON("", c.args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
	if len(f.CheckinsTool.Questions) != 3 {
		t.Fatal("a refused question was set up")
	}
}

// An edit is held against the question it finds: the wording before it, the
// schedule after it, and where it names somebody by name, address or as me the
// question is read first, so that one not there answers first.
func TestTheRefusalsOfAnEditComeInMinkasOrderOrAfterTheRead(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, c := range map[string]struct {
		args []string
		exit int
		code string
	}{
		"the wording before the question":  {[]string{"checkins", "edit", unknownID, "--title", ""}, ExitValidation, "questionTitleMissing"},
		"the question before the schedule": {[]string{"checkins", "edit", unknownID, "--rhythm", "fortnightly"}, ExitNotFound, ""},
		"the question before nobody":       {[]string{"checkins", "edit", unknownID, "--ask", unknownID}, ExitNotFound, ""},
		"read first for me":                {[]string{"checkins", "edit", unknownID, "--title", "", "--ask", "me"}, ExitNotFound, ""},
		"read first for a name":            {[]string{"checkins", "edit", unknownID, "--title", "", "--ask", "Anke"}, ExitNotFound, ""},
		"an empty time":                    {[]string{"checkins", "edit", f.DailyQuestion.ID, "--time", ""}, ExitValidation, apiCodeQuestionTimeInvalid},
		"an empty set of weekdays":         {[]string{"checkins", "edit", f.DailyQuestion.ID, "--weekday", ""}, ExitValidation, apiCodeQuestionWeekdaysMissing},
		"a stranger":                       {[]string{"checkins", "edit", f.DailyQuestion.ID, "--ask", f.Stranger.ID}, ExitValidation, apiCodePersonNotAProjectMember},
	} {
		env, r := e.runJSON("", c.args...)
		if r.code != c.exit || c.code != "" && env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: exit %d, apiCode %v, want %d %s", name, r.code, env.Meta["apiCode"], c.exit, c.code)
		}
		if c.exit == ExitNotFound && env.Hint != questionNotFoundHint {
			t.Fatalf("%s: hint %q", name, env.Hint)
		}
	}

	f.Project.Archived = true
	for name, args := range map[string][]string{
		"a stranger":            {"checkins", "edit", f.DailyQuestion.ID, "--ask", f.Stranger.ID},
		"a paused one paused":   {"checkins", "pause", f.PausedQuestion.ID},
		"a running one resumed": {"checkins", "resume", f.DailyQuestion.ID},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeProjectArchived {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
	for _, args := range [][]string{
		{"checkins", "edit", f.DailyQuestion.ID, "--title", ""},
		{"checkins", "edit", f.DailyQuestion.ID, "--weekday", "9"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] == apiCodeProjectArchived {
			t.Fatalf("%v: the archive answered before the body", args)
		}
	}
	for _, args := range [][]string{
		{"checkins", "pause", unknownID},
		{"checkins", "edit", unknownID, "--title", "Renamed"},
		{"checkins", "answers", "create", unknownID, "Late"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
	}
}

func TestTheAnswersToAQuestionComeAPageAtATimeTheNewestDayFirst(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID)
	if ids := answerIDs(t, env.Data); !slices.Equal(ids, []string{f.AnkesAnswer.ID, f.BrunosAnswer.ID, f.AnkesEarlierAnswer.ID}) || env.Notice != "" {
		t.Fatalf("answers %v, notice %q", ids, env.Notice)
	}
	if query := lastQuery(e); query != "limit=50" {
		t.Fatalf("asked for %q", query)
	}

	env = runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID, "--limit", "2")
	next := "minka checkins answers list " + f.DailyQuestion.ID + " --before " + f.BrunosAnswer.ID
	if ids := answerIDs(t, env.Data); len(ids) != 2 || !strings.Contains(env.Notice, next) || crumbFor(t, env.Breadcrumbs, "next") != next {
		t.Fatalf("answers %v, notice %q", ids, env.Notice)
	}
	env = runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID, "--limit", "2", "--before", f.BrunosAnswer.ID)
	if ids := answerIDs(t, env.Data); !slices.Equal(ids, []string{f.AnkesEarlierAnswer.ID}) || env.Notice != "" {
		t.Fatalf("answers %v, notice %q", ids, env.Notice)
	}

	// A person given by name is looked up among the members of the project,
	// and the next page keeps the person and the day.
	env = runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID, "--person", "Anke", "--on", "2026-09-11", "--limit", "1")
	if ids := answerIDs(t, env.Data); !slices.Equal(ids, []string{f.AnkesAnswer.ID}) {
		t.Fatalf("answers %v", ids)
	}
	if !strings.Contains(env.Notice, "--person "+f.Anke.ID+" --on 2026-09-11 --before "+f.AnkesAnswer.ID) {
		t.Fatalf("notice %q", env.Notice)
	}
	if ids := answerIDs(t, runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID, "--person", "me").Data); !slices.Equal(ids, []string{f.BrunosAnswer.ID}) {
		t.Fatalf("my answers %v", ids)
	}

	text := e.run("", "checkins", "answers", "list", f.DailyQuestion.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"## 2026-09-11", "- " + f.BrunosAnswer.ID + "  Bruno: Counted the tents.", "## 2026-09-10"} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}
}

// A page of answers ends in nothing but being short, and Minka cuts a larger
// one down in silence, so no size outside the bound goes out.
func TestAPageOfAnswersOutsideTheBoundIsAUsageError(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	requests := len(e.server.Requests())
	for _, args := range [][]string{
		{"checkins", "answers", "list", f.DailyQuestion.ID, "--limit", "0"},
		{"checkins", "answers", "list", f.DailyQuestion.ID, "--limit", "101"},
		{"checkins", "answers", "list", f.DailyQuestion.ID, "--before", "the last one"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != requests {
		t.Fatal("a page nobody may ask for went out")
	}
	runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID, "--limit", "100")
	if query := lastQuery(e); query != "limit=100" {
		t.Fatalf("asked for %q", query)
	}
}

// The question is looked for before the page is read: a day that is none and a
// boundary of another question answer only for a question that is there.
func TestTheRefusalsOfAPageOfAnswersComeAfterTheQuestion(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, c := range map[string]struct {
		args []string
		exit int
		code string
	}{
		"the question before the day":    {[]string{"checkins", "answers", "list", unknownID, "--on", "11.09.2026"}, ExitNotFound, ""},
		"the question before the person": {[]string{"checkins", "answers", "list", unknownID, "--person", "Anke"}, ExitNotFound, ""},
		"a day that is none":             {[]string{"checkins", "answers", "list", f.DailyQuestion.ID, "--on", "11.09.2026"}, ExitValidation, apiCodeAnswerDayNotADate},
		"another question's answer":      {[]string{"checkins", "answers", "list", f.WeeklyQuestion.ID, "--before", f.BrunosAnswer.ID}, ExitValidation, apiCodeAnswersBeforeUnknown},
	} {
		env, r := e.runJSON("", c.args...)
		if r.code != c.exit || c.code != "" && env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: exit %d, apiCode %v", name, r.code, env.Meta["apiCode"])
		}
		switch c.code {
		case "":
			if env.Hint != questionNotFoundHint {
				t.Fatalf("%s: hint %q", name, env.Hint)
			}
		case apiCodeAnswerDayNotADate:
			if !strings.Contains(env.Hint, "--on") {
				t.Fatalf("%s: hint %q", name, env.Hint)
			}
		case apiCodeAnswersBeforeUnknown:
			if !strings.Contains(env.Hint, "minka checkins answers list "+f.WeeklyQuestion.ID) {
				t.Fatalf("%s: hint %q", name, env.Hint)
			}
		}
	}

	// An answer in the trash still marks where a page ends.
	f.BrunosAnswer.Trashed = true
	env := runOK(t, e, "checkins", "answers", "list", f.DailyQuestion.ID, "--before", f.BrunosAnswer.ID)
	if ids := answerIDs(t, env.Data); !slices.Equal(ids, []string{f.AnkesEarlierAnswer.ID}) {
		t.Fatalf("answers %v", ids)
	}
}

func TestAnAnswerIsReadWithItsQuestionAndItsComments(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "answers", "show", f.BrunosAnswer.ID)
	answer := decode(t, env.Data)
	if answer["groupOn"] != "2026-09-11" || answer["questionId"] != f.DailyQuestion.ID || answer["editedAt"] != nil {
		t.Fatalf("answer %v", answer)
	}
	if !strings.Contains(env.Summary, "Bruno") || !strings.Contains(env.Summary, "2026-09-11") {
		t.Fatalf("summary %q", env.Summary)
	}

	text := e.run("", "checkins", "answers", "show", f.BrunosAnswer.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"# What did you do today?", "For: 2026-09-11", "Counted the tents.", "## Comments (1)", "Thank you!"} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text output lacks %q:\n%s", want, text.stdout)
		}
	}

	f.WeeklyQuestion.Answers = []*fakeapi.CheckinAnswer{{ID: f.OtherCheckinsTool.ID, AuthorID: f.Bruno.ID, GroupOn: "2026-09-07", Body: "<p>Gone</p>"}}
	f.WeeklyQuestion.Trashed = true
	for _, id := range []string{f.OtherQuestion.Answers[0].ID, unknownID, f.OtherCheckinsTool.ID} {
		env, r := e.runJSON("", "checkins", "answers", "show", id)
		expectCode(t, r, ExitNotFound)
		if env.Hint != answerNotFoundHint {
			t.Fatalf("hint %q", env.Hint)
		}
	}
}

// Without a day, an answer belongs to the latest day the question was asked;
// with one, to that day, which has to be one the question was put to the
// writer for.
func TestAnAnswerBelongsToTheDayAskedFor(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "answers", "create", f.DailyQuestion.ID, "Counted\nthe benches")
	answer := decode(t, env.Data)
	if answer["groupOn"] != "2026-09-11" || !strings.Contains(env.Summary, "for 2026-09-11") {
		t.Fatalf("answer %v, summary %q", answer, env.Summary)
	}
	body := e.lastWriteBody()
	if body["body"] != "<p>Counted<br>the benches</p>" {
		t.Fatalf("sent %v", body)
	}
	if _, given := body["askedFor"]; given {
		t.Fatal("a day went out that was not given")
	}

	// The same day again is an answer of its own.
	runOK(t, e, "checkins", "answers", "create", f.DailyQuestion.ID, "And the chairs")
	if len(f.DailyQuestion.Answers) != 5 {
		t.Fatalf("%d answers", len(f.DailyQuestion.Answers))
	}

	if reminders := listOfObjects(t, runOK(t, e, "checkins", "reminders").Data); len(reminders) != 1 {
		t.Fatalf("reminders %v", reminders)
	}
	env = runOK(t, e, "checkins", "answers", "create", f.WeeklyQuestion.ID, "The CLI", "--asked-for", "2026-09-07")
	if decode(t, env.Data)["groupOn"] != "2026-09-07" || e.lastWriteBody()["askedFor"] != "2026-09-07" {
		t.Fatalf("answer %s", env.Data)
	}
	if reminders := listOfObjects(t, runOK(t, e, "checkins", "reminders").Data); len(reminders) != 0 {
		t.Fatalf("an answered reminder is open: %v", reminders)
	}

	// A paused question is answered as well.
	runOK(t, e, "checkins", "answers", "create", f.PausedQuestion.ID, "A good month", "--asked-for", "2026-09-04")
}

// Minka holds the day of an answer before it looks for the question, the body
// after it, and whether the day was asked only then, before the archive.
func TestTheRefusalsOfAnAnswerComeInMinkasOrder(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for name, c := range map[string]struct {
		args []string
		exit int
		code string
	}{
		"the day before the question":  {[]string{"checkins", "answers", "create", unknownID, "Hello", "--asked-for", "tomorrow"}, ExitValidation, apiCodeAnswerDayNotADate},
		"the question before the body": {[]string{"checkins", "answers", "create", unknownID, ""}, ExitNotFound, ""},
		"nothing said":                 {[]string{"checkins", "answers", "create", f.WeeklyQuestion.ID, "", "--asked-for", "2026-09-08"}, ExitValidation, apiCodeAnswerMissing},
		"a day not asked":              {[]string{"checkins", "answers", "create", f.WeeklyQuestion.ID, "Hello", "--asked-for", "2026-09-08"}, ExitValidation, apiCodeAnswerDayNotAsked},
		"a day asked of somebody else": {[]string{"checkins", "answers", "create", f.DailyQuestion.ID, "Hello", "--asked-for", "2026-09-09"}, ExitValidation, apiCodeAnswerDayNotAsked},
	} {
		env, r := e.runJSON("", c.args...)
		if r.code != c.exit || c.code != "" && env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: exit %d, apiCode %v", name, r.code, env.Meta["apiCode"])
		}
		switch c.code {
		case apiCodeAnswerDayNotAsked:
			if !strings.Contains(env.Hint, "checkins reminders") {
				t.Fatalf("%s: hint %q", name, env.Hint)
			}
		case apiCodeAnswerDayNotADate:
			if !strings.Contains(env.Hint, "--asked-for") {
				t.Fatalf("%s: hint %q", name, env.Hint)
			}
		case apiCodeAnswerMissing:
			if !strings.Contains(env.Hint, "text or a file") {
				t.Fatalf("%s: hint %q", name, env.Hint)
			}
		}
	}

	f.Project.Archived = true
	for name, c := range map[string]struct {
		args []string
		code string
	}{
		"a day not asked before the archive": {[]string{"checkins", "answers", "create", f.WeeklyQuestion.ID, "Hello", "--asked-for", "2026-09-08"}, apiCodeAnswerDayNotAsked},
		"the archive for a day asked":        {[]string{"checkins", "answers", "create", f.WeeklyQuestion.ID, "Hello", "--asked-for", "2026-09-07"}, apiCodeProjectArchived},
	} {
		env, r := e.runJSON("", c.args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
	if len(f.WeeklyQuestion.Answers) != 0 {
		t.Fatal("a refused answer was written")
	}
}

func TestAnAnswerSentAsHTMLWithoutMarkupIsSaidToStayCharacters(t *testing.T) {
	e := newTestEnv(t)

	env := runOK(t, e, "checkins", "answers", "create", e.fixture.DailyQuestion.ID, "# Heading", "--format", "html")
	if !strings.Contains(env.Notice, "kept as the characters") {
		t.Fatalf("notice %q", env.Notice)
	}
	if env := runOK(t, e, "checkins", "answers", "create", e.fixture.DailyQuestion.ID, "<p>Fine</p>", "--format", "html"); env.Notice != "" {
		t.Fatalf("notice %q", env.Notice)
	}
}

// Only the writer changes an answer; anybody else's is answered as if it were
// not there. The same text again succeeds and changes nothing.
func TestOnlyTheWriterChangesAnAnswer(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "answers", "edit", f.BrunosAnswer.ID, "Counted the tents and the benches.")
	answer := decode(t, env.Data)
	if answer["body"] != "<p>Counted the tents and the benches.</p>" || answer["editedAt"] == nil || answer["groupOn"] != "2026-09-11" {
		t.Fatalf("answer %v", answer)
	}
	writes := e.server.WritingRequests()
	if last := writes[len(writes)-1]; last.Method != http.MethodPatch || last.Path != "/api/v1/answers/"+f.BrunosAnswer.ID {
		t.Fatalf("sent %s %s", last.Method, last.Path)
	}
	editedAt := *f.BrunosAnswer.EditedAt
	runOK(t, e, "checkins", "answers", "edit", f.BrunosAnswer.ID, "Counted the tents and the benches.")
	if !f.BrunosAnswer.EditedAt.Equal(editedAt) {
		t.Fatal("the same text again changed the answer")
	}

	for _, id := range []string{f.AnkesAnswer.ID, unknownID} {
		env, r := e.runJSON("", "checkins", "answers", "edit", id, "Mine now")
		expectCode(t, r, ExitNotFound)
		if env.Hint != answerWriterHint {
			t.Fatalf("hint %q", env.Hint)
		}
	}
	if f.AnkesAnswer.Body != "<p>Fetched the speakers.</p>" {
		t.Fatal("somebody else's answer changed")
	}

	env, r := e.runJSON("", "checkins", "answers", "edit", f.BrunosAnswer.ID, "<p> </p>", "--format", "html")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeAnswerMissing {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	f.Project.Archived = true
	env, r = e.runJSON("", "checkins", "answers", "edit", f.BrunosAnswer.ID, "Counted the tents and the benches.")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeProjectArchived {
		t.Fatalf("the same text in an archived project: apiCode %v", env.Meta["apiCode"])
	}
}

func TestAnAnswerWithFilesIsNotReplacedByOneWithout(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.BrunosAnswer.Body = `<p>See <action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment></p>`

	env, r := e.runJSON("", "checkins", "answers", "edit", f.BrunosAnswer.ID, "Plain words")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || !strings.Contains(env.Hint, "bodySource") {
		t.Fatalf("hint %q", env.Hint)
	}
	for _, write := range e.server.WritingRequests() {
		if write.Method == http.MethodPatch {
			t.Fatal("the answer was replaced")
		}
	}

	env = runOK(t, e, "checkins", "answers", "edit", f.BrunosAnswer.ID, "Plain words", "--drop-attachments")
	if !strings.Contains(env.Notice, "dropped") {
		t.Fatalf("notice %q", env.Notice)
	}
}

func TestTheRemindersAreTheOpenOnesTheOneAskedLastFirst(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "checkins", "reminders")
	reminders := listOfObjects(t, env.Data)
	if len(reminders) != 1 || reminders[0]["askedFor"] != "2026-09-07" || env.Notice != "" {
		t.Fatalf("reminders %v, notice %q", reminders, env.Notice)
	}
	if question := reminders[0]["question"].(map[string]any); question["id"] != f.WeeklyQuestion.ID {
		t.Fatalf("question %v", question)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "answer"), "--asked-for") {
		t.Fatalf("breadcrumbs %+v", env.Breadcrumbs)
	}

	// Asked for a later day than the one answered, the daily question is open
	// again, and it was asked last.
	f.DailyQuestion.Reminders = append(f.DailyQuestion.Reminders, &fakeapi.CheckinReminder{PersonID: f.Bruno.ID, AskedFor: "2026-09-14", AskedAt: time.Date(2026, 9, 14, 17, 0, 0, 0, time.UTC)})
	reminders = listOfObjects(t, runOK(t, e, "checkins", "reminders").Data)
	if len(reminders) != 2 || reminders[0]["askedFor"] != "2026-09-14" {
		t.Fatalf("reminders %v", reminders)
	}

	f.Project.Archived = true
	if reminders := listOfObjects(t, runOK(t, e, "checkins", "reminders").Data); len(reminders) != 0 {
		t.Fatalf("an archived project reminds: %v", reminders)
	}
}

// Minka hands over at most a hundred reminders and has no page beyond them, so
// a list that long says there may be more.
func TestAFullListOfRemindersSaysThereMayBeMore(t *testing.T) {
	for _, c := range []struct {
		questions int
		notice    bool
	}{{99, false}, {100, true}, {101, true}} {
		e := newTestEnv(t)
		f := e.fixture
		for i := 0; i < c.questions; i++ {
			f.CheckinsTool.Questions = append(f.CheckinsTool.Questions, &fakeapi.CheckinQuestion{
				ID: strconv.Itoa(i), Title: "Q", Rhythm: "daily", Weekdays: []int{1}, TimeOfDay: "09:00", StartsOn: "2026-09-01",
				AskedIDs: []string{f.Bruno.ID}, Reminders: []*fakeapi.CheckinReminder{{PersonID: f.Bruno.ID, AskedFor: "2026-09-07"}},
			})
		}
		f.WeeklyQuestion.Paused = true

		env := runOK(t, e, "checkins", "reminders")
		if n := len(listOfObjects(t, env.Data)); n != min(c.questions, 100) {
			t.Fatalf("%d questions: %d reminders", c.questions, n)
		}
		if got := strings.Contains(env.Notice, "at most 100") && strings.Contains(env.Notice, "asked earlier"); got != c.notice {
			t.Fatalf("%d questions: notice %q", c.questions, env.Notice)
		}
	}
}

func TestAQuestionOfAnArchivedProjectIsStillRead(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Project.Archived = true

	for _, args := range [][]string{
		{"checkins", "tools", f.Project.ID},
		{"checkins", "list", f.CheckinsTool.ID},
		{"checkins", "show", f.DailyQuestion.ID},
		{"checkins", "answers", "list", f.DailyQuestion.ID},
		{"checkins", "answers", "show", f.BrunosAnswer.ID},
	} {
		runOK(t, e, args...)
	}
}

func TestTheCatalogListsTheCheckinsCommands(t *testing.T) {
	e := newTestEnv(t)
	env := runOK(t, e, "commands")
	for _, want := range []string{"minka checkins reminders", "minka checkins answers create", "minka checkins pause"} {
		if !strings.Contains(string(env.Data), `"`+want+`"`) {
			t.Fatalf("the catalog lacks %q", want)
		}
	}
}

func answerIDs(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var ids []string
	for _, answer := range listOfObjects(t, raw) {
		ids = append(ids, answer["id"].(string))
	}
	return ids
}

// lastQuery is the query string of the last request the fake received.
func lastQuery(e *testEnv) string {
	requests := e.server.Requests()
	return requests[len(requests)-1].Query
}

// Somebody else's answer is not there to Minka, and that comes before the guard
// over files this side keeps: exit 2, and nothing is written.
func TestSomebodyElsesAnswerWithFilesIsNotFoundBeforeTheGuard(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.AnkesAnswer.Body = `<p>See <action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Bruno"></action-text-attachment></p>`

	for _, args := range [][]string{
		{"checkins", "answers", "edit", f.AnkesAnswer.ID, "Mine now"},
		{"checkins", "answers", "edit", f.AnkesAnswer.ID, "Mine now", "--drop-attachments"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
		if env.Hint != answerWriterHint || !strings.Contains(env.Error, "Anke") {
			t.Fatalf("error %q, hint %q", env.Error, env.Hint)
		}
	}
	for _, write := range e.server.WritingRequests() {
		if write.Method == http.MethodPatch {
			t.Fatal("somebody else's answer was sent")
		}
	}
}

// A body that says nothing comes before the archived project, and an edit of
// a question that shows nobody meets the archive like any other.
func TestWhatIsWrongWithAnEditComesBeforeTheArchivedProject(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.WeeklyQuestion.AskedIDs = []string{f.Client.ID}
	f.Project.Archived = true

	for name, c := range map[string]struct {
		args []string
		code string
	}{
		"an edit of a question that shows nobody": {[]string{"checkins", "edit", f.WeeklyQuestion.ID, "--title", "Renamed"}, apiCodeProjectArchived},
		"an answer changed to nothing":            {[]string{"checkins", "answers", "edit", f.BrunosAnswer.ID, "<p> </p>", "--format", "html"}, apiCodeAnswerMissing},
	} {
		env, r := e.runJSON("", c.args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != c.code {
			t.Fatalf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
}

// Should Minka ever take a write and not show somebody named as asked, the
// notice names them once, however often they were named, and nobody shown.
func TestSomebodyNamedAndNotAskedIsNamedOnce(t *testing.T) {
	question := api.CheckinQuestion{Asked: []api.PersonReference{{ID: "id-shown"}}}

	notices := askedNotices([]string{"id-missing", "id-shown", "id-missing"}, question)
	if len(notices) != 1 || strings.Count(notices[0], "id-missing") != 1 || strings.Contains(notices[0], "id-shown") || !strings.Contains(notices[0], "not ask") {
		t.Fatalf("notices %q", notices)
	}
	if notices := askedNotices([]string{"id-shown"}, question); notices != nil {
		t.Fatalf("notices %q for somebody shown", notices)
	}
}

// An agent that stepped down stays a member of the projects archived then, and
// Minka lists it among the people asked there.
func TestAnAgentThatSteppedDownIsListedWhereItIsStillAMember(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.DailyQuestion.AskedIDs = append(f.DailyQuestion.AskedIDs, f.RetiredAgent.ID)
	f.Project.Archived = true

	if asked := idsOf(t, decode(t, runOK(t, e, "checkins", "show", f.DailyQuestion.ID).Data), "asked"); !slices.Contains(asked, f.RetiredAgent.ID) {
		t.Fatalf("asked %v", asked)
	}
}

// Minka reads a weekday written with a fraction as no whole number, whatever
// its value. The CLI never sends one; the fake has to answer it as Minka does.
func TestTheFakeRefusesAWeekdayWrittenWithAFraction(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for body, want := range map[string]int{
		`{"title":"Q","rhythm":"weekly","weekdays":[1.0],"timeOfDay":"09:00","askedIds":["` + f.Bruno.ID + `"]}`: http.StatusBadRequest,
		`{"title":"Q","rhythm":"weekly","weekdays":[1],"timeOfDay":"09:00","askedIds":["` + f.Bruno.ID + `"]}`:   http.StatusCreated,
	} {
		request, err := http.NewRequest(http.MethodPost, e.server.URL+"/api/v1/checkins/"+f.CheckinsTool.ID+"/questions", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+f.Token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Fatalf("%s: %d, want %d", body, response.StatusCode, want)
		}
	}
}
