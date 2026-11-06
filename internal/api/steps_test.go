package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	wordsStep = `{"id":"s1","title":"Measure","completed":false,"assignees":[],"dueOn":null,"card":null}`
	childStep = `{"id":"s2","title":"Tent","completed":false,"assignees":[],"dueOn":null,"card":{"kind":"card","id":"c2","shortId":"MNK-2","title":"Tent"}}`
	thirdStep = `{"id":"s3","title":"Call","completed":true,"assignees":[],"dueOn":null,"card":null}`
)

// answering is a Minka that answers every new step with status and body, and
// a look-up of a short identifier with 404.
func answering(t *testing.T, status int, body string) *Client {
	t.Helper()
	return answeringAndLooking(t, status, body, "")
}

// answeringAndLooking is a Minka that answers every new step with status and
// body, and a look-up of a short identifier with lookedUp, or 404 where it is
// empty.
func answeringAndLooking(t *testing.T, status int, body, lookedUp string) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			if lookedUp == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(lookedUp))
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return New(server.URL, "minka_pat_x", "")
}

// sameJSON says whether two JSON texts hold the same value.
func sameJSON(t *testing.T, got json.RawMessage, want string) bool {
	t.Helper()
	var a, b any
	if json.Unmarshal(got, &a) != nil || json.Unmarshal([]byte(want), &b) != nil {
		return false
	}
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// A step answers as itself, and from a Minka that answers with the card the
// step is taken out of it: the last one of a new step in words, the child's
// of a card step, new or standing, found by the name it was given or, under
// a former prefix of its board, by the card that name leads to. Raw is the
// step either way.
func TestANewStepIsReadFromBothAnswers(t *testing.T) {
	card := func(steps ...string) string {
		return `{"id":"c1","title":"Party","column":{"id":"k","title":"Triage"},"steps":[` + strings.Join(steps, ",") + `]}`
	}
	addStep := func(c *Client) (Result[Step], error) { return c.AddStep(context.Background(), "c1", "Call") }
	addCard := func(named string) func(*Client) (Result[Step], error) {
		return func(c *Client) (Result[Step], error) { return c.AddCardStep(context.Background(), "c1", named) }
	}
	for _, c := range []struct {
		name     string
		status   int
		body     string
		lookedUp string
		add      func(*Client) (Result[Step], error)
		want     string
	}{
		{"a step in words", 201, wordsStep, "", addStep, wordsStep},
		{"a card step", 201, childStep, "", addCard("MNK-2"), childStep},
		{"a card step that stood", 200, childStep, "", addCard("c2"), childStep},
		{"the card of a step in words", 201, card(wordsStep, childStep, thirdStep), "", addStep, thirdStep},
		{"the card of a card step", 201, card(wordsStep, childStep), "", addCard("C2"), childStep},
		{"the card of a card step with a step added after it", 201, card(childStep, thirdStep), "", addCard("c2"), childStep},
		{"the card of a card step that stood, by its ID", 200, card(childStep, thirdStep), "", addCard("C2"), childStep},
		{"the card of a card step that stood, by its short identifier", 200, card(wordsStep, childStep, thirdStep), "", addCard("mnk-2"), childStep},
		{"the card of a card step under a former prefix", 201, card(childStep, thirdStep), `{"id":"c2"}`, addCard("old-2"), childStep},
		{"the card of a card step that stood under a former prefix", 200, card(wordsStep, childStep, thirdStep), `{"id":"c2"}`, addCard("OLD-2"), childStep},
	} {
		got, err := c.add(answeringAndLooking(t, c.status, c.body, c.lookedUp))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		var want Step
		_ = json.Unmarshal([]byte(c.want), &want)
		if got.Value.ID != want.ID || got.Value.Title != want.Title || got.StatusCode != c.status || !sameJSON(t, got.Raw, c.want) {
			t.Errorf("%s: %+v, raw %s", c.name, got.Value, got.Raw)
		}
	}
}

// An answer without the step is no answer to read a step from, and the
// message says whether the step was written.
func TestAnAnswerWithoutTheStepIsUnexpected(t *testing.T) {
	const written, notWritten = "Minka wrote the step", "nothing was written"
	for name, c := range map[string]struct {
		add  func() (Result[Step], error)
		says string
	}{
		"no steps at all": {func() (Result[Step], error) {
			return answering(t, 201, `{"id":"c1","steps":[]}`).AddStep(context.Background(), "c1", "x")
		}, written},
		"a new child not among them": {func() (Result[Step], error) {
			return answering(t, 201, `{"id":"c1","steps":[`+wordsStep+`]}`).AddCardStep(context.Background(), "c1", "c2")
		}, written},
		"a standing child not among them": {func() (Result[Step], error) {
			return answering(t, 200, `{"id":"c1","steps":[`+wordsStep+`]}`).AddCardStep(context.Background(), "c1", "MNK-2")
		}, notWritten},
		"a name that leads to a card not among them": {func() (Result[Step], error) {
			return answeringAndLooking(t, 200, `{"id":"c1","steps":[`+wordsStep+`]}`, `{"id":"c9"}`).AddCardStep(context.Background(), "c1", "OLD-2")
		}, notWritten},
		"no JSON object": {func() (Result[Step], error) {
			return answering(t, 201, `[]`).AddStep(context.Background(), "c1", "x")
		}, written},
		"a step that is no step": {func() (Result[Step], error) {
			return answering(t, 201, `{"id":"s1","title":5}`).AddStep(context.Background(), "c1", "x")
		}, written},
	} {
		_, err := c.add()
		var apiErr *Error
		if !errors.As(err, &apiErr) || apiErr.Code != CodeUnexpectedResponse || !strings.Contains(apiErr.Message, c.says) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
