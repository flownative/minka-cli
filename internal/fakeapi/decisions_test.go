package fakeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// A decision from before its tool had statuses holds, and the filter by the
// status of role holds lists it; the archive is listed by no filter.
func TestADecisionWithoutAStatusIsListedAsInForce(t *testing.T) {
	s, f := New(t)
	f.ProposedDecision.StatusID = ""
	list := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"

	for _, query := range []string{"?status=holds", "?status=" + f.Decisions.StatusOfRole("holds").ID} {
		request, _ := http.NewRequest(http.MethodGet, s.URL+list+query, nil)
		request.Header.Set("Authorization", "Bearer "+f.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var lines []map[string]any
		_ = json.NewDecoder(response.Body).Decode(&lines)
		_ = response.Body.Close()
		var ids []string
		for _, line := range lines {
			ids = append(ids, line["id"].(string))
		}
		if !slices.Equal(ids, []string{f.ProposedDecision.ID, f.Decision.ID}) {
			t.Errorf("%s: %v, want the decision without a status and the one in force, none of the archive", query, ids)
		}
	}
}

// A new decision is refused in the order the controller asks: the tool, each
// field in turn, the project, and last the subject before the earlier one.
func TestANewDecisionIsRefusedInTheOrderMinkaAsks(t *testing.T) {
	s, f := New(t)
	tool := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"
	nowhere := "/api/v1/decisions-tools/" + newID() + "/decisions"
	unknownCard := `{"kind":"card","id":"` + newID() + `"}`

	for _, c := range []struct{ name, path, body, want string }{
		{"no JSON for a tool never made", nowhere, `{`, "400 malformedBody"},
		{"no sentence for a tool never made", nowhere, `{"title":""}`, "404"},
		{"no sentence and no status", tool, `{"title":"","status":"decided"}`, "422 decisionTitleMissing"},
		{"a status that is none beside a stranger", tool, `{"title":"T","status":"In force","decidedBy":[{"personId":"` + f.Stranger.ID + `"}]}`, "422 decisionStatusUnknown"},
		{"a stranger beside votes too long", tool, `{"title":"T","decidedBy":[{"personId":"` + f.Stranger.ID + `"}],"votes":"` + strings.Repeat("x", 101) + `"}`, "422 personNotAProjectMember"},
		{"both relations beside a subject of no shape", tool, `{"title":"T","supersedes":"` + f.Decision.ID + `","revokes":"` + f.Decision.ID + `","appliesTo":{"kind":"todo"}}`, "422 decisionEarlierTwice"},
		{"a subject of no shape beside a day that is none", tool, `{"title":"T","appliesTo":{"kind":"todo"},"validUntil":"soon"}`, "400 malformedBody"},
		{"an unknown subject beside an unknown earlier one", tool, `{"title":"T","appliesTo":` + unknownCard + `,"supersedes":"` + newID() + `"}`, "422 decisionAppliesToUnknown"},
		{"an earlier one of the archive", tool, `{"title":"T","supersedes":"` + f.ArchivedDecision.ID + `"}`, "422 decisionEarlierUnknown"},
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodPost, c.path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	f.Project.Archived = true
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, tool, `{"title":"T","appliesTo":`+unknownCard+`}`); got != "422 projectArchived" {
		t.Errorf("an unknown subject in an archived project: %s", got)
	}
}

// A later decision moves the earlier one only while it holds, and a change of
// the status alone makes no version.
func TestTheEarlierDecisionFollowsOnlyALaterOneInForce(t *testing.T) {
	s, f := New(t)
	tool := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"

	got, later := answerTo(t, s, f.Token, http.MethodPost, tool, `{"title":"Tuesdays","status":"proposed","supersedes":"`+f.Decision.ID+`"}`)
	if got != "201" || f.Decision.StatusID != f.Decisions.StatusOfRole("holds").ID {
		t.Fatalf("%s: the earlier one moved before the later one holds", got)
	}
	address := "/api/v1/decisions/" + later["id"].(string)
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"status":"holds"}`); got != "200" || f.Decision.StatusID != f.Decisions.StatusOfRole("superseded").ID {
		t.Fatalf("%s: the earlier one did not follow", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"status":"rejected"}`); got != "200" || f.Decision.StatusID != f.Decisions.StatusOfRole("superseded").ID {
		t.Fatalf("%s: the earlier one moved back", got)
	}
	written := f.Decisions.Decisions[len(f.Decisions.Decisions)-1]
	if len(written.Versions) != 1 {
		t.Fatalf("%d versions after changes of the status alone", len(written.Versions))
	}

	// The same decision named in the other relation is related anew.
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"status":"holds"}`); got != "200" {
		t.Fatalf("status: %s", got)
	}
	f.Decision.StatusID = f.Decisions.StatusOfRole("holds").ID
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"revokes":"`+f.Decision.ID+`"}`); got != "200" || f.Decision.StatusID != f.Decisions.StatusOfRole("revoked").ID {
		t.Fatalf("%s: the earlier one did not follow into the other relation", got)
	}
}

// A decision from before its tool had statuses holds without a status of role
// holds, so naming an earlier decision moves nothing.
func TestALaterDecisionWithoutAStatusMovesNothing(t *testing.T) {
	s, f := New(t)
	f.ProposedDecision.StatusID = ""

	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, "/api/v1/decisions/"+f.ProposedDecision.ID, `{"supersedes":"`+f.Decision.ID+`"}`); got != "200" {
		t.Fatalf("relating: %s", got)
	}
	if f.Decision.StatusID != f.Decisions.StatusOfRole("holds").ID {
		t.Fatal("the earlier decision moved for a decision without a status")
	}
}

// An earlier decision that went into the archive since stays where it is
// when the later one comes into force.
func TestAnEarlierDecisionOfTheArchiveDoesNotFollow(t *testing.T) {
	s, f := New(t)
	later := &Decision{ID: newID(), Title: "Later", StatusID: f.Decisions.StatusOfRole("proposed").ID,
		EarlierID: f.ArchivedDecision.ID, Relation: "supersedes", CreatedAt: s.now, UpdatedAt: s.now, CreatorID: f.Bruno.ID}
	f.Decisions.Decisions = append(f.Decisions.Decisions, later)

	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, "/api/v1/decisions/"+later.ID, `{"status":"holds"}`); got != "200" {
		t.Fatalf("status: %s", got)
	}
	if f.ArchivedDecision.StatusID != f.Decisions.StatusOfRole("holds").ID {
		t.Fatal("a decision of the archive moved")
	}
}

// A change meets an unknown earlier decision before an unknown subject, and
// an empty relation beside the other is both named.
func TestAChangeIsRefusedInTheOrderMinkaAsks(t *testing.T) {
	s, f := New(t)
	address := "/api/v1/decisions/" + f.ProposedDecision.ID
	unknownCard := `{"kind":"card","id":"` + newID() + `"}`

	for _, c := range []struct{ name, body, want string }{
		{"an empty relation beside the other", `{"supersedes":"","revokes":"` + f.Decision.ID + `"}`, "422 decisionEarlierTwice"},
		{"an unknown subject beside an unknown earlier one", `{"appliesTo":` + unknownCard + `,"supersedes":"` + newID() + `"}`, "422 decisionEarlierUnknown"},
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// Who decided is named once each, a name in any case, before the limit of
// fifty is counted.
func TestWhoDecidedIsNamedOnceEach(t *testing.T) {
	s, f := New(t)
	names := []string{`{"personId":"` + f.Anke.ID + `"}`, `{"personId":"` + f.Anke.ID + `"}`}
	for i := 0; i < 50; i++ {
		names = append(names, `{"name":"Board"}`, `{"name":"BOARD"}`)
	}
	got, decision := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/decisions-tools/"+f.Decisions.ID+"/decisions",
		`{"title":"T","decidedBy":[`+strings.Join(names, ",")+`]}`)
	if deciders, _ := decision["decidedBy"].([]any); got != "201" || len(deciders) != 2 {
		t.Fatalf("%s: decidedBy %v", got, decision["decidedBy"])
	}
}

// The audience is read after the days and checked whatever the status, and a
// decision tells only where it comes newly into a status of role proposed or
// holds. Whether the people chosen may be told is asked only then.
func TestADecisionTellsItsAudienceOnlyWhereItIsProposedOrComesIntoForce(t *testing.T) {
	s, f := New(t)
	tool := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"

	for _, c := range []struct{ name, body, want string }{
		{"a day that is none beside an audience that is none", `{"title":"T","reviewOn":"soon","audience":"all"}`, "422 decisionReviewOnNotADay"},
		{"an audience that is none", `{"title":"T","audience":"all"}`, "422 decisionAudienceUnknown"},
		{"a list that is none", `{"title":"T","audience":"chosen","notifyIds":"` + f.Anke.ID + `"}`, "400 malformedBody"},
		{"a name that is no ID beside people without chosen", `{"title":"T","notifyIds":["Anke"]}`, "422 personNotAProjectMember"},
		{"chosen without people", `{"title":"T","audience":"chosen","notifyIds":[]}`, "422 notifyIdsMissing"},
		{"people without chosen", `{"title":"T","notifyIds":["` + f.Anke.ID + `"]}`, "422 notifyIdsWithoutChosenAudience"},
		{"a stranger chosen", `{"title":"T","audience":"chosen","notifyIds":["` + f.Stranger.ID + `"]}`, "422 personNotAProjectMember"},
		{"a client chosen for a decision not shown to clients", `{"title":"T","audience":"chosen","notifyIds":["` + f.Client.ID + `"]}`, "422 personCannotSeeIt"},
		{"a client chosen for a decision shown to clients", `{"title":"T","visibleToClients":true,"audience":"chosen","notifyIds":["` + f.Client.ID + `"]}`, "201"},
		{"a stranger chosen for a draft, which tells nobody", `{"title":"T","status":"draft","audience":"chosen","notifyIds":["` + f.Stranger.ID + `"]}`, "201"},
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodPost, tool, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	got, written := answerTo(t, s, f.Token, http.MethodPost, tool, `{"title":"T","status":"proposed","audience":"chosen","notifyIds":["`+strings.ToUpper(f.Anke.ID)+`"]}`)
	if got != "201" {
		t.Fatalf("proposed: %s", got)
	}
	decision := f.Decisions.Decisions[len(f.Decisions.Decisions)-1]
	address := "/api/v1/decisions/" + written["id"].(string)
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"status":"proposed","audience":"nobody"}`); got != "200" {
		t.Fatalf("the status kept: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"title":"T2","audience":"all"}`); got != "422 decisionAudienceUnknown" {
		t.Fatalf("a save that keeps the status checks the audience all the same: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"title":"T2","audience":"chosen","notifyIds":["`+f.Stranger.ID+`"]}`); got != "200" {
		t.Fatalf("a save that keeps the status asks nobody chosen: %s", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, address, `{"status":"holds","audience":"nobody"}`); got != "200" {
		t.Fatalf("into force: %s", got)
	}
	want := []DecisionAnnouncement{{Role: "proposed", Audience: "chosen", NotifyIDs: []string{f.Anke.ID}}, {Role: "holds", Audience: "nobody"}}
	if len(decision.Announcements) != len(want) {
		t.Fatalf("announcements %+v, want %+v", decision.Announcements, want)
	}
	for i := range want {
		if decision.Announcements[i].Role != want[i].Role || decision.Announcements[i].Audience != want[i].Audience || !slices.Equal(decision.Announcements[i].NotifyIDs, want[i].NotifyIDs) {
			t.Errorf("announcement %d: %+v, want %+v", i, decision.Announcements[i], want[i])
		}
	}
}

// A decision is found by its short identifier in any case and under a former
// prefix, under the conditions of its page; every miss is the same 404.
func TestADecisionIsFoundByItsShortIdentifier(t *testing.T) {
	s, f := New(t)
	s.Tokens["minka_pat_client"] = f.Client.ID
	f.Decisions.Prefix, f.Decisions.FormerPrefixes = "ENT", []string{"DEC"}
	f.Decision.ShortID, f.DraftDecision.ShortID = "ENT-2", "ENT-4"

	for _, c := range []struct{ name, token, identifier, want string }{
		{"in lowercase", f.Token, "ent-2", f.Decision.ID},
		{"under the former prefix", f.Token, "DEC-2", f.Decision.ID},
		{"a draft for the team", f.Token, "ENT-4", f.DraftDecision.ID},
		{"a decision shown to clients for a client", "minka_pat_client", "ENT-2", f.Decision.ID},
		{"a draft for a client", "minka_pat_client", "ENT-4", ""},
		{"of another project", f.Token, "E-1", ""},
		{"unknown", f.Token, "ENT-99", ""},
		{"of the wrong shape", f.Token, "kein", ""},
		{"a card's", f.Token, f.CardInTriage.ShortID, ""},
	} {
		got, answer := answerTo(t, s, c.token, http.MethodGet, "/api/v1/decisions/by-short-id/"+c.identifier, "")
		if c.want == "" {
			if got != "404" {
				t.Errorf("%s: %s, want 404", c.name, got)
			}
			continue
		}
		shortID, _ := answer["shortId"].(string)
		shortURL, _ := answer["shortUrl"].(string)
		if got != "200" || answer["id"] != c.want || shortID == "" || !strings.HasSuffix(shortURL, "/go/decision/"+shortID) {
			t.Errorf("%s: %s %v", c.name, got, answer)
		}
	}
}

// The status of a decision keeps who set it: the writer, whoever changed it,
// and the writer of a later decision that moved it; a save that keeps the
// status keeps who set it. The list narrows to those an agent set by hand.
func TestADecisionKeepsWhoSetItsStatus(t *testing.T) {
	s, f := New(t)
	list := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"
	if got, _ := answerTo(t, s, f.Token, http.MethodGet, list+"?setBy=human", ""); got != "422 decisionSetByUnknown" {
		t.Fatalf("setBy=human: %s", got)
	}
	setByAnAgent := func() []string {
		request, _ := http.NewRequest(http.MethodGet, s.URL+list+"?setBy=agent", nil)
		request.Header.Set("Authorization", "Bearer "+f.Token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var lines []map[string]any
		_ = json.NewDecoder(response.Body).Decode(&lines)
		var ids []string
		for _, line := range lines {
			ids = append(ids, line["id"].(string))
		}
		return ids
	}
	if got := setByAnAgent(); !slices.Equal(got, []string{f.ProposedDecision.ID}) {
		t.Fatalf("set by an agent: %v", got)
	}
	f.ProposedDecision.StatusSetBy.Minka = true
	if got := setByAnAgent(); len(got) != 0 {
		t.Fatalf("a status Minka moved counts as set by an agent: %v", got)
	}

	address := "/api/v1/decisions/" + f.Decision.ID
	if got, _ := answerTo(t, s, f.AgentToken, http.MethodPatch, address, `{"title":"We meet on Mondays at six","status":"holds"}`); got != "200" || f.Decision.StatusSetBy.PersonID != f.Bruno.ID {
		t.Fatalf("%s: a save that keeps the status changed who set it", got)
	}
	if got, answer := answerTo(t, s, f.AgentToken, http.MethodPatch, address, `{"status":"done"}`); got != "200" || answer["statusSetBy"].(map[string]any)["agent"] != true {
		t.Fatalf("%s: %v", got, answer["statusSetBy"])
	}
	got, later := answerTo(t, s, f.Token, http.MethodPost, list, `{"title":"Tuesdays","supersedes":"`+f.ProposedDecision.ID+`"}`)
	if got != "201" || f.ProposedDecision.StatusSetBy.PersonID != f.Bruno.ID || f.ProposedDecision.StatusSetBy.Minka {
		t.Fatalf("%s: the earlier one was moved by %+v", got, f.ProposedDecision.StatusSetBy)
	}
	if later["shortId"] != "DEC-5" {
		t.Fatalf("the next number: %v", later["shortId"])
	}
	written := f.Decisions.Decisions[len(f.Decisions.Decisions)-1]
	if setBy, _ := later["statusSetBy"].(map[string]any); written.StatusSetBy == nil || written.StatusSetBy.PersonID != f.Bruno.ID || setBy["agent"] != false {
		t.Fatalf("a new decision names %v as having set its status", later["statusSetBy"])
	}
}

// The people chosen are met in the order given, the writer passed over, and
// the first who cannot be told decides. A change meets them before the
// earlier decision and the subject, a new decision after both.
func TestThePeopleChosenAreRefusedWhereTheHandlerMeetsThem(t *testing.T) {
	s, f := New(t)
	tool := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions"
	address := "/api/v1/decisions/" + f.ProposedDecision.ID
	unknownEarlier := `"supersedes":"` + newID() + `"`
	people := func(ids ...string) string {
		return `"audience":"chosen","notifyIds":["` + strings.Join(ids, `","`) + `"]`
	}

	for _, c := range []struct{ name, method, path, body, want string }{
		{"a change: a stranger beside an unknown earlier one", http.MethodPatch, address, `{"status":"holds",` + unknownEarlier + `,` + people(f.Stranger.ID) + `}`, "422 personNotAProjectMember"},
		{"a new one: a stranger beside an unknown earlier one", http.MethodPost, tool, `{"title":"T",` + unknownEarlier + `,` + people(f.Stranger.ID) + `}`, "422 decisionEarlierUnknown"},
		{"a client before a stranger", http.MethodPost, tool, `{"title":"T",` + people(f.Client.ID, f.Stranger.ID) + `}`, "422 personCannotSeeIt"},
		{"a stranger before a client", http.MethodPost, tool, `{"title":"T",` + people(f.Stranger.ID, f.Client.ID) + `}`, "422 personNotAProjectMember"},
		{"the writer and somebody twice", http.MethodPost, tool, `{"title":"T",` + people(f.Bruno.ID, f.Anke.ID, f.Anke.ID) + `}`, "201"},
	} {
		if got, _ := answerTo(t, s, f.Token, c.method, c.path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// A decision goes into the history by its "#" name or its short address,
// never into the consequences, and an entry naming a card or a decision
// carries its short identifier.
func TestAnEntryNamesADecisionByItsShortIdentifier(t *testing.T) {
	s, f := New(t)
	f.CardInDoing.ShortID = "MNK-9"
	history := "/api/v1/decisions/" + f.ProposedDecision.ID + "/history"
	consequences := "/api/v1/decisions/" + f.ProposedDecision.ID + "/consequences"

	for _, c := range []struct{ name, path, address, want string }{
		{"a # name", history, "#dec-2", "DEC-2"},
		{"a short address", history, s.URL + "/go/decision/DEC-1", "DEC-1"},
		{"a card", consequences, "#MNK-9", "MNK-9"},
	} {
		got, entry := answerTo(t, s, f.Token, http.MethodPost, c.path, `{"address":"`+c.address+`"}`)
		if got != "201" || entry["shortId"] != c.want {
			t.Errorf("%s: %s %v", c.name, got, entry)
		}
	}
	decisionPage := "/projects/" + f.Project.ID + "/decisions/" + f.Decisions.ID + "/" + f.Decision.ID
	if got, entry := answerTo(t, s, f.Token, http.MethodPost, history, `{"address":"`+s.URL+decisionPage+`"}`); got != "201" || entry["kind"] != "decision" || entry["shortId"] != "DEC-2" {
		t.Errorf("the page of a decision: %s %v", got, entry)
	}
	for _, address := range []string{"#DEC-2", s.URL + "/go/decision/DEC-2", s.URL + decisionPage} {
		if got, _ := answerTo(t, s, f.Token, http.MethodPost, consequences, `{"address":"`+address+`"}`); got != "422 entryTargetUnknown" {
			t.Errorf("%s as a consequence: %s", address, got)
		}
	}
}

// A status is named by its identifier or its role in any case, and a change
// of a decision of the archive is refused as such once the project takes
// writes.
func TestAStatusIsNamedInAnyCaseAndTheArchiveTakesNoChange(t *testing.T) {
	s, f := New(t)
	if got, _ := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/decisions-tools/"+f.Decisions.ID+"/decisions?status=HOLDS", ""); got != "200" {
		t.Errorf("a role in capitals in the list: %s", got)
	}
	if got, answer := answerTo(t, s, f.Token, http.MethodPatch, "/api/v1/decisions/"+f.ProposedDecision.ID, `{"status":"Rejected"}`); got != "200" || answer["status"].(map[string]any)["role"] != "rejected" {
		t.Errorf("a role in capitals: %s %v", got, answer["status"])
	}

	archived := "/api/v1/decisions/" + f.ArchivedDecision.ID
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, archived, `{"title":"Renamed"}`); got != "422 decisionArchived" {
		t.Errorf("a change of a decision of the archive: %s", got)
	}
	f.Project.Archived = true
	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, archived, `{"title":"Renamed"}`); got != "422 projectArchived" {
		t.Errorf("a change of a decision of the archive in an archived project: %s", got)
	}
}

// Several decisions are written down in their order with what they share,
// numbered one after the other, each with its place on the agenda, and the
// list keeps that order; one written down alone has no place and no labels.
func TestSeveralDecisionsAreWrittenDownInTheirOrder(t *testing.T) {
	s, f := New(t)
	label := s.AddLabel(f.Project.ID, "", "Budget", "red")
	batch := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions/batch"

	got, answer := answerTo(t, s, f.Token, http.MethodPost, batch, `{"titles":[" The budget passes ","The grant is approved","December is off"],"decidedOn":"2026-11-10","decidedBy":[{"name":"Council"}],"labelIds":["`+label.ID+`"],"reason":"ignored"}`)
	if got != "201" {
		t.Fatalf("%s %v", got, answer)
	}
	written, _ := answer["decisions"].([]any)
	var titles, shortIDs []string
	var places []float64
	for _, entry := range written {
		decision := entry.(map[string]any)
		titles = append(titles, decision["title"].(string))
		shortIDs = append(shortIDs, decision["shortId"].(string))
		places = append(places, decision["agendaNumber"].(float64))
		if decision["decidedOn"] != "2026-11-10" || decision["reason"] != nil || len(decision["labels"].([]any)) != 1 {
			t.Errorf("not what they share: %v", decision)
		}
	}
	if !slices.Equal(titles, []string{"The budget passes", "The grant is approved", "December is off"}) || !slices.Equal(places, []float64{1, 2, 3}) {
		t.Errorf("titles %v, places %v", titles, places)
	}
	if len(shortIDs) != 3 || shortIDs[1] == shortIDs[0] || shortIDs[2] == shortIDs[1] {
		t.Errorf("not numbered one after the other: %v", shortIDs)
	}

	_, alone := answerTo(t, s, f.Token, http.MethodPost, "/api/v1/decisions-tools/"+f.Decisions.ID+"/decisions", `{"title":"Alone","decidedOn":"2026-11-10","labelIds":["`+label.ID+`"]}`)
	if alone["agendaNumber"] != nil || len(alone["labels"].([]any)) != 0 {
		t.Errorf("a decision written down alone: agenda %v, labels %v", alone["agendaNumber"], alone["labels"])
	}

	request, _ := http.NewRequest(http.MethodGet, s.URL+"/api/v1/decisions-tools/"+f.Decisions.ID+"/decisions", nil)
	request.Header.Set("Authorization", "Bearer "+f.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&lines)
	_ = response.Body.Close()
	var listed []string
	for _, line := range lines {
		if line["decidedOn"] == "2026-11-10" {
			listed = append(listed, line["title"].(string))
		}
	}
	if !slices.Equal(listed, []string{"The budget passes", "The grant is approved", "December is off", "Alone"}) {
		t.Errorf("listed %v, want the agenda's order and the one alone after it", listed)
	}
}

// The list orders decisions written down together by their place on the
// agenda, not by the order they are kept in.
func TestTheListOrdersByTheAgendaNotByStorage(t *testing.T) {
	s, f := New(t)
	batch := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions/batch"
	if got, _ := answerTo(t, s, f.Token, http.MethodPost, batch, `{"titles":["First","Second","Third"],"decidedOn":"2026-11-10"}`); got != "201" {
		t.Fatal(got)
	}
	slices.Reverse(f.Decisions.Decisions)

	request, _ := http.NewRequest(http.MethodGet, s.URL+"/api/v1/decisions-tools/"+f.Decisions.ID+"/decisions", nil)
	request.Header.Set("Authorization", "Bearer "+f.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var lines []map[string]any
	_ = json.NewDecoder(response.Body).Decode(&lines)
	_ = response.Body.Close()
	var listed []string
	for _, line := range lines {
		if line["decidedOn"] == "2026-11-10" {
			listed = append(listed, line["title"].(string))
		}
	}
	if !slices.Equal(listed, []string{"First", "Second", "Third"}) {
		t.Fatalf("listed %v", listed)
	}
}

// Several decisions are refused as a whole in the order the controller
// asks, the sentences each at its place, and nothing is written.
func TestSeveralDecisionsAreRefusedAsAWhole(t *testing.T) {
	s, f := New(t)
	batch := "/api/v1/decisions-tools/" + f.Decisions.ID + "/decisions/batch"
	nowhere := "/api/v1/decisions-tools/" + newID() + "/decisions/batch"
	high := s.AddLabel(f.Project.ID, "", "Priority::High", "red")
	low := s.AddLabel(f.Project.ID, "", "Priority::Low", "blue")
	boardLabel := s.AddLabel(f.Project.ID, f.Board.ID, "Board only", "red")
	unknownCard := `{"kind":"card","id":"` + newID() + `"}`
	before := len(f.Decisions.Decisions)

	for _, c := range []struct{ name, path, body, want string }{
		{"no sentences for a tool never made", nowhere, `{"titles":[]}`, "404"},
		{"no list", batch, `{"titles":"One"}`, "400 malformedBody"},
		{"a list of more than text", batch, `{"titles":["One",2]}`, "400 malformedBody"},
		{"an empty list beside a status that is none", batch, `{"titles":[],"status":"nope"}`, "422 decisionsMissing"},
		{"too many", batch, `{"titles":[` + strings.TrimSuffix(strings.Repeat(`"x",`, 101), ",") + `]}`, "422 decisionsTooMany"},
		{"a refused sentence beside a status that is none", batch, `{"titles":["Good","  "],"status":"nope"}`, "422 decisionLinesRefused"},
		{"a status that is none beside a label never made", batch, `{"titles":["Good"],"status":"nope","labelIds":["` + newID() + `"]}`, "422 decisionStatusUnknown"},
		{"labels of no list beside an unknown subject", batch, `{"titles":["Good"],"labelIds":"x","appliesTo":` + unknownCard + `}`, "400 malformedBody"},
		{"a label of a board", batch, `{"titles":["Good"],"labelIds":["` + boardLabel.ID + `"]}`, "422 labelUnknown"},
		{"an unknown subject beside two of a scope", batch, `{"titles":["Good"],"appliesTo":` + unknownCard + `,"labelIds":["` + high.ID + `","` + low.ID + `"]}`, "422 decisionAppliesToUnknown"},
		{"two of a scope beside a stranger", batch, `{"titles":["Good"],"labelIds":["` + high.ID + `","` + low.ID + `"],"audience":"chosen","notifyIds":["` + f.Stranger.ID + `"]}`, "422 labelsShareAScope"},
		{"a stranger to tell", batch, `{"titles":["Good"],"audience":"chosen","notifyIds":["` + f.Stranger.ID + `"]}`, "422 personNotAProjectMember"},
	} {
		if got, _ := answerTo(t, s, f.Token, http.MethodPost, c.path, c.body); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}

	_, answer := answerTo(t, s, f.Token, http.MethodPost, batch, `{"titles":["Good","`+strings.Repeat("x", 201)+`"," ","Also good","Tab\there"]}`)
	refusal, _ := answer["error"].(map[string]any)
	lines, _ := refusal["lines"].([]any)
	var refused []string
	for _, line := range lines {
		entry := line.(map[string]any)
		refused = append(refused, fmt.Sprint(entry["index"], " ", entry["code"]))
	}
	if !slices.Equal(refused, []string{"1 decisionTitleTooLong", "2 decisionTitleMissing", "4 decisionTitleHasControlCharacters"}) {
		t.Errorf("the lines refused: %v", refused)
	}

	clientToken := "minka_pat_client"
	s.Tokens[clientToken] = f.Client.ID
	if got, _ := answerTo(t, s, clientToken, http.MethodPost, batch, `{"titles":["Good"],"labelIds":["`+high.ID+`"]}`); got != "404" {
		t.Errorf("a client sending labels: %s", got)
	}
	if len(f.Decisions.Decisions) != before {
		t.Errorf("%d decisions written by refused requests", len(f.Decisions.Decisions)-before)
	}
}
