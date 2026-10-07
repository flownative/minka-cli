package fakeapi

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// The server refuses a body with anything behind its one value, and so does
// the fake, on every address that reads a body.
func TestABodyWithSomethingBehindItIsMalformed(t *testing.T) {
	server, fixture := New(t)

	for body, want := range map[string]int{
		`{}]`:   http.StatusBadRequest,
		`{}}`:   http.StatusBadRequest,
		`{} {}`: http.StatusBadRequest,
		"{} \n": http.StatusUnprocessableEntity,
	} {
		request, err := http.NewRequest(http.MethodPatch, server.URL+"/api/v1/todo-lists/"+fixture.TodoList.ID, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+fixture.Token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("%q: %d, want %d", body, response.StatusCode, want)
		}
	}
}

// A comment under a card written again with the same text is a write like any
// other: Minka answers 200 and moves its editedAt to the moment of the request.
func TestTheSameTextOfACardCommentAgainMovesItsEditedAt(t *testing.T) {
	s, f := New(t)
	comment := f.CardInTriage.Comments[0]
	path := "/api/v1/cards/" + f.CardInTriage.ID + "/comments/" + comment.ID

	if got, _ := answerTo(t, s, f.Token, http.MethodPatch, path, `{"body":"Still open"}`); got != "200" {
		t.Fatalf("the first change: %s", got)
	}
	body := comment.Body
	s.now = s.now.Add(time.Hour)
	got, answer := answerTo(t, s, f.Token, http.MethodPatch, path, `{"body":"Still open"}`)
	if got != "200" {
		t.Fatalf("the same text again: %s", got)
	}
	if comment.Body != body || comment.EditedAt == nil || !comment.EditedAt.Equal(s.now) {
		t.Errorf("the same text again kept the moment of the first change: %v", comment.EditedAt)
	}
	if answer["editedAt"] != timestamp(s.now) {
		t.Errorf("the answer says editedAt %v, want %s", answer["editedAt"], timestamp(s.now))
	}
}

// Signing out revokes the token the request carries and no other; the same
// token is turned away afterwards, the revocation included.
func TestARevokedTokenIsTurnedAwayAndNoOther(t *testing.T) {
	s, f := New(t)

	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, "/api/v1/me/token", ""); got != "204" {
		t.Fatalf("revoking: %s, want 204", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodGet, "/api/v1/me", ""); got != "401 unauthenticated" {
		t.Errorf("the revoked token reading: %s, want 401", got)
	}
	if got, _ := answerTo(t, s, f.Token, http.MethodDelete, "/api/v1/me/token", ""); got != "401 unauthenticated" {
		t.Errorf("the revoked token revoking again: %s, want 401", got)
	}
	if got, _ := answerTo(t, s, f.StrangerToken, http.MethodGet, "/api/v1/me", ""); got != "200" {
		t.Errorf("another token: %s, want 200", got)
	}
}
