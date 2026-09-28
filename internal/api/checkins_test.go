package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A page of answers ends in nothing but being shorter than asked for, and the
// server cuts one above its largest down without a word. So no page beyond
// the bound is asked for from here, and the size always goes out.
func TestNoPageOfAnswersIsAskedForBeyondWhatTheServerServes(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = io.WriteString(w, "[]")
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	for _, limit := range []int{0, -1, CheckinAnswersPageMax + 1} {
		if _, err := client.CheckinAnswers(context.Background(), "q", CheckinAnswersQuery{Limit: limit}); !errors.Is(err, ErrCheckinAnswersPageOutOfBounds) {
			t.Errorf("a page of %d: %v", limit, err)
		}
	}
	if len(queries) != 0 {
		t.Fatalf("requests went out for a page nobody may ask for: %q", queries)
	}
	query := CheckinAnswersQuery{PersonID: "p", On: "2026-09-11", Before: "a", Limit: CheckinAnswersPageMax}
	if _, err := client.CheckinAnswers(context.Background(), "q", query); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0] != "before=a&limit=100&on=2026-09-11&personId=p" {
		t.Fatalf("queries %q", queries)
	}
}

// A change of a question sends only what is given, and a set of weekdays or of
// people goes out as a list even when it is empty.
func TestAChangeOfAQuestionSendsOnlyWhatIsGiven(t *testing.T) {
	rhythm, none, noDays := "weekly", []string(nil), []int(nil)
	for name, c := range map[string]struct {
		input any
		want  string
	}{
		"nothing given":   {UpdateCheckinQuestionInput{}, `{}`},
		"a rhythm":        {UpdateCheckinQuestionInput{Rhythm: &rhythm}, `{"rhythm":"weekly"}`},
		"empty sets":      {UpdateCheckinQuestionInput{Weekdays: &noDays, AskedIDs: &none}, `{"askedIds":[],"weekdays":[]}`},
		"a new question":  {CreateCheckinQuestionInput{Title: "Q"}, `{"title":"Q"}`},
		"an answer":       {CreateCheckinAnswerInput{Body: "A"}, `{"body":"A"}`},
		"a day answered":  {CreateCheckinAnswerInput{Body: "b", AskedFor: "2026-09-07"}, `{"body":"b","askedFor":"2026-09-07"}`},
		"a whole weekday": {CreateCheckinQuestionInput{Title: "Q", Weekdays: []int{1, 3}}, `{"title":"Q","weekdays":[1,3]}`},
	} {
		encoded, err := json.Marshal(c.input)
		if err != nil {
			t.Fatal(err)
		}
		if string(encoded) != c.want {
			t.Errorf("%s: %s, want %s", name, encoded, c.want)
		}
	}
}
