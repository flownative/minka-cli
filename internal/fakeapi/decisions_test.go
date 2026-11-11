package fakeapi

import (
	"encoding/json"
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
		{"a status that is none beside a stranger", tool, `{"title":"T","status":"Holds","decidedBy":[{"personId":"` + f.Stranger.ID + `"}]}`, "422 decisionStatusUnknown"},
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
