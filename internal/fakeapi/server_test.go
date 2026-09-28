package fakeapi

import (
	"net/http"
	"strings"
	"testing"
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
