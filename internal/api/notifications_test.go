package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// deadlineOf records the deadline a request went out with and answers an
// empty page.
type deadlineOf struct {
	deadline time.Time
	found    bool
}

func (d *deadlineOf) RoundTrip(request *http.Request) (*http.Response, error) {
	d.deadline, d.found = request.Context().Deadline()
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"notifications":[],"cursor":"c1"}`)),
		Request:    request,
	}, nil
}

// The safeguard: a request the server holds open for the wait is given more
// time than the wait, and still a bound. The timeout of every other request
// would cut a wait of five minutes off after thirty seconds.
func TestAWaitIsGivenMoreTimeThanItWaits(t *testing.T) {
	c := New("https://minka.example.com", "token", "test")
	recorder := &deadlineOf{}
	c.waits.Transport = recorder
	c.http.Transport = recorder

	before := time.Now()
	if _, err := c.Notifications(t.Context(), "c0", NotificationsWaitMax, NotificationsPageMax); err != nil {
		t.Fatal(err)
	}
	if !recorder.found {
		t.Fatal("the wait went out without a deadline")
	}
	if left := recorder.deadline.Sub(before); left <= NotificationsWaitMax*time.Second || left > NotificationsWaitMax*time.Second+time.Minute {
		t.Fatalf("the wait may take %s, want a little more than %ds", left, NotificationsWaitMax)
	}
}

// A page is Minka's own shape, the lines under notifications and the
// position beside them, and each line is handed on as it came.
func TestAPageOfNotificationsIsReadInMinkasShape(t *testing.T) {
	page := Result[NotificationsPage]{Raw: []byte(`{"notifications":[{"id":"n1","cursor":"c1","summary":"one","extra":true}],"cursor":"c1"}`)}
	if err := json.Unmarshal(page.Raw, &page.Value); err != nil {
		t.Fatal(err)
	}
	if page.Value.Cursor != "c1" || len(page.Value.Notifications) != 1 || page.Value.Notifications[0].Summary != "one" {
		t.Fatalf("page %+v", page.Value)
	}
	lines, err := LinesOf(page)
	if err != nil || len(lines) != 1 || !strings.Contains(string(lines[0]), `"extra":true`) {
		t.Fatalf("lines %s, %v", lines, err)
	}
}
