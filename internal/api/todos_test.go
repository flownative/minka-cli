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

// A page of one's own to-dos ends in nothing but being shorter than asked for,
// and the server cuts one above its largest down without a word. So no page
// beyond the bound is asked for from here, and the size always goes out.
func TestNoPageOfMyTodosIsAskedForBeyondWhatTheServerServes(t *testing.T) {
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		_, _ = io.WriteString(w, "[]")
	}))
	defer server.Close()
	client := New(server.URL, "minka_pat_x", "")

	for _, limit := range []int{0, -1, MyTodosPageMax + 1} {
		if _, err := client.MyTodos(context.Background(), limit, ""); !errors.Is(err, ErrMyTodosPageOutOfBounds) {
			t.Errorf("a page of %d: %v", limit, err)
		}
	}
	if len(queries) != 0 {
		t.Fatalf("requests went out for a page nobody may ask for: %q", queries)
	}
	if _, err := client.MyTodos(context.Background(), MyTodosPageMax, "0199"); err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0] != "after=0199&limit=100" {
		t.Fatalf("queries %q", queries)
	}
}

// Every field of a change goes out only where it is given, and each of the
// three kinds of nothing goes out as the one the server reads as nothing.
func TestAChangeOfATodoSaysWhatItTakesOff(t *testing.T) {
	empty, none := "", []string(nil)
	for name, c := range map[string]struct {
		input any
		want  string
	}{
		"nothing given":          {UpdateTodoInput{}, `{}`},
		"a day taken off":        {UpdateTodoInput{DueOn: &empty}, `{"dueOn":null}`},
		"everybody taken off":    {UpdateTodoInput{AssigneeIDs: &none, NotifiedIDs: &none}, `{"assigneeIds":[],"notifiedIds":[]}`},
		"the own group kept":     {MoveTodoInput{ListID: "l"}, `{"listId":"l"}`},
		"no group":               {MoveTodoInput{NoGroup: true}, `{"groupId":null}`},
		"a subtask's person off": {UpdateSubtaskInput{AssigneeID: &empty, DueOn: &empty}, `{"assigneeId":null,"dueOn":null}`},
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
