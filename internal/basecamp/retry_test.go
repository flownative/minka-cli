package basecamp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// flaky fails its first requests with the failures given and answers every
// later one. It ends a test that asks far more often than any rule allows,
// so that a loop without an end is a failure and not a test that hangs.
type flaky struct {
	t        *testing.T
	failures []error
	// always is the failure of every request once the others are used up.
	always   func(request int) error
	requests []string
}

func (f *flaky) Get(_ context.Context, path string) ([]byte, error) {
	f.requests = append(f.requests, path)
	if len(f.requests) > 3*MaxAttempts {
		f.t.Fatalf("%s was asked %d times", path, len(f.requests))
	}
	if len(f.requests) <= len(f.failures) {
		return nil, f.failures[len(f.requests)-1]
	}
	if f.always != nil {
		return nil, f.always(len(f.requests))
	}
	return []byte(`{"answered":true}`), nil
}

// pauses records what was waited for and waits for nothing.
type pauses struct {
	asked []time.Duration
}

func (p *pauses) pause(ctx context.Context, duration time.Duration) error {
	p.asked = append(p.asked, duration)
	return ctx.Err()
}

func TestTheAttemptsAndThePausesAreTheNamedOnes(t *testing.T) {
	if MaxAttempts != 4 {
		t.Errorf("a request is asked %d times at most, want 4", MaxAttempts)
	}
	want := []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}
	if !slices.Equal(RetryPauses[:], want) {
		t.Errorf("pauses %v, want %v", RetryPauses, want)
	}
	for i := 1; i < len(RetryPauses); i++ {
		if RetryPauses[i] <= RetryPauses[i-1] {
			t.Errorf("pause %d does not grow: %v", i+1, RetryPauses)
		}
	}
}

func TestARequestThatFailsForAReasonThatMayPassIsAskedAgain(t *testing.T) {
	for _, kind := range []Kind{Network, RateLimited, Unavailable} {
		t.Run(string(kind), func(t *testing.T) {
			failed := &Error{Kind: kind, Message: "what went wrong"}
			source := &flaky{t: t, failures: []error{failed, failed}}
			waited := &pauses{}
			retrying := NewRetrying(source, waited.pause)

			content, err := retrying.Get(t.Context(), "people.json?page=1")
			if err != nil || string(content) != `{"answered":true}` {
				t.Fatalf("got %s, %v", content, err)
			}
			if want := []string{"people.json?page=1", "people.json?page=1", "people.json?page=1"}; !slices.Equal(source.requests, want) {
				t.Errorf("asked %v, want %v", source.requests, want)
			}
			if want := []time.Duration{RetryPauses[0], RetryPauses[1]}; !slices.Equal(waited.asked, want) {
				t.Errorf("waited %v, want %v", waited.asked, want)
			}
			if retrying.Retried() != 2 {
				t.Errorf("counted %d requests asked again, want 2", retrying.Retried())
			}
		})
	}
}

func TestARequestThatAnswersIsAskedOnce(t *testing.T) {
	source := &flaky{t: t}
	waited := &pauses{}
	retrying := NewRetrying(source, waited.pause)

	for _, path := range []string{"people.json?page=1", "people.json?page=2"} {
		if _, err := retrying.Get(t.Context(), path); err != nil {
			t.Fatal(err)
		}
	}
	if len(source.requests) != 2 || len(waited.asked) != 0 || retrying.Retried() != 0 {
		t.Errorf("asked %v, waited %v, counted %d", source.requests, waited.asked, retrying.Retried())
	}
}

func TestAFailureThatDoesNotPassIsAskedOnce(t *testing.T) {
	failures := map[string]error{
		"not installed": &Error{Kind: NotInstalled, Message: "what went wrong"},
		"not found":     &Error{Kind: NotFound, Message: "what went wrong"},
		"not signed in": &Error{Kind: AuthRequired, Message: "what went wrong"},
		"forbidden":     &Error{Kind: Forbidden, Message: "what went wrong"},
		"interrupted":   &Error{Kind: Interrupted, Message: "what went wrong"},
		"API":           &Error{Kind: API, Message: "what went wrong"},
		"of no class":   errors.New("what went wrong"),
	}
	for name, failed := range failures {
		t.Run(name, func(t *testing.T) {
			source := &flaky{t: t, failures: []error{failed}}
			waited := &pauses{}
			retrying := NewRetrying(source, waited.pause)

			_, err := retrying.Get(t.Context(), "people.json")
			if err != failed {
				t.Errorf("got %v, want the failure as it came", err)
			}
			if len(source.requests) != 1 || len(waited.asked) != 0 || retrying.Retried() != 0 {
				t.Errorf("asked %v, waited %v, counted %d", source.requests, waited.asked, retrying.Retried())
			}
		})
	}
}

func TestTheAttemptsAreBoundedAndTheLastFailureComesOut(t *testing.T) {
	source := &flaky{t: t, always: func(request int) error {
		return &Error{Kind: Unavailable, Message: fmt.Sprintf("failure %d", request)}
	}}
	waited := &pauses{}
	retrying := NewRetrying(source, waited.pause)

	_, err := retrying.Get(t.Context(), "people.json")
	expectKind(t, err, Unavailable)
	if err.Error() != "failure 4" {
		t.Errorf("got %q, want the last failure", err)
	}
	var failed *Error
	if !errors.As(err, &failed) || !failed.Transient() {
		t.Errorf("the last failure does not say that it may pass: %v", err)
	}
	if len(source.requests) != 4 {
		t.Errorf("asked %d times, want 4", len(source.requests))
	}
	if want := []time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}; !slices.Equal(waited.asked, want) {
		t.Errorf("waited %v, want %v", waited.asked, want)
	}
	if retrying.Retried() != 3 {
		t.Errorf("counted %d requests asked again, want 3", retrying.Retried())
	}

	// The next request has all its attempts again.
	source.requests, source.always = nil, nil
	if _, err := retrying.Get(t.Context(), "projects/7001.json"); err != nil {
		t.Fatal(err)
	}
}

func TestARunStoppedDuringAPauseIsInterruptedWithoutAFurtherRequest(t *testing.T) {
	for name, stop := range map[string]func(ctx context.Context, cancel func()) error{
		"a pause that says so": func(ctx context.Context, cancel func()) error {
			cancel()
			return ctx.Err()
		},
		"a pause that says nothing": func(_ context.Context, cancel func()) error {
			cancel()
			return nil
		},
		"a pause that ends before the context does": func(context.Context, func()) error {
			return context.Canceled
		},
	} {
		t.Run(name, func(t *testing.T) {
			source := &flaky{t: t, always: func(int) error { return &Error{Kind: Network, Message: "connection reset"} }}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			retrying := NewRetrying(source, func(ctx context.Context, _ time.Duration) error {
				return stop(ctx, cancel)
			})

			_, err := retrying.Get(ctx, "people.json")
			expectKind(t, err, Interrupted)
			if len(source.requests) != 1 {
				t.Errorf("asked %d times, want no request after the run was stopped", len(source.requests))
			}
			if retrying.Retried() != 0 {
				t.Errorf("counted %d requests asked again", retrying.Retried())
			}
		})
	}
}

// everyThird passes a request on its third attempt, so that none of them uses
// up its attempts.
type everyThird struct {
	asked map[string]int
}

func (s *everyThird) Get(_ context.Context, path string) ([]byte, error) {
	s.asked[path]++
	if s.asked[path]%3 != 0 {
		return nil, &Error{Kind: RateLimited, Message: "slow down", Hint: "what to do next"}
	}
	return []byte(`{"answered":true}`), nil
}

func TestTheRetriesOfARunAreBounded(t *testing.T) {
	if MaxRetriesOfARun != 100 {
		t.Errorf("a run asks %d requests again at most, want 100", MaxRetriesOfARun)
	}
	source := &everyThird{asked: map[string]int{}}
	waited := &pauses{}
	retrying := NewRetrying(source, waited.pause)

	// Two retries a request: fifty requests use up what a run has.
	for request := 1; request <= 50; request++ {
		if _, err := retrying.Get(t.Context(), fmt.Sprintf("projects/%d.json", request)); err != nil {
			t.Fatalf("request %d: %v", request, err)
		}
	}
	if retrying.Retried() != 100 {
		t.Fatalf("counted %d requests asked again, want 100", retrying.Retried())
	}

	_, err := retrying.Get(t.Context(), "projects/51.json")
	expectKind(t, err, RateLimited)
	var failed *Error
	if !errors.As(err, &failed) || !failed.Transient() || failed.Hint != "what to do next" {
		t.Errorf("the failure lost its class or its hint: %v", err)
	}
	if err.Error() != "slow down; Basecamp kept failing: 100 requests of this run were asked again already, and no further one is" {
		t.Errorf("the message does not say that Basecamp kept failing: %q", err)
	}
	if source.asked["projects/51.json"] != 1 || len(waited.asked) != 100 || retrying.Retried() != 100 {
		t.Errorf("asked %d times, waited %d times, counted %d", source.asked["projects/51.json"], len(waited.asked), retrying.Retried())
	}

	// A request that answers goes on as before.
	source.asked["projects/52.json"] = 2
	if _, err := retrying.Get(t.Context(), "projects/52.json"); err != nil {
		t.Errorf("a request that answers: %v", err)
	}
}

// Without a pause of its own a retrying source waits by the clock, and for as
// long as it says: a run that is over after 50 ms has not come to a second
// request by then.
func TestWithoutAPauseOfItsOwnARequestIsAskedAgainOnlyAfterThePause(t *testing.T) {
	source := &flaky{t: t, always: func(int) error { return &Error{Kind: Network, Message: "connection reset"} }}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := NewRetrying(source, nil).Get(ctx, "people.json")
	expectKind(t, err, Interrupted)
	if len(source.requests) != 1 {
		t.Errorf("asked %d times within 50 ms, want once: the first pause is %v", len(source.requests), RetryPauses[0])
	}
}

func TestThePauseByTheClockLastsAsLongAsItSays(t *testing.T) {
	pause := 20 * time.Millisecond
	started := time.Now()
	if err := waitFor(t.Context(), pause); err != nil {
		t.Fatal(err)
	}
	if waited := time.Since(started); waited < pause {
		t.Errorf("waited %v, want at least %v", waited, pause)
	}
}

func TestThePauseByTheClockEndsWithTheContext(t *testing.T) {
	stopped, cancel := context.WithCancel(t.Context())
	cancel()
	done := make(chan error, 1)
	go func() { done <- waitFor(stopped, time.Hour) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a pause that was stopped says it was waited for")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the pause was waited for instead of stopped")
	}

	if err := waitFor(t.Context(), time.Millisecond); err != nil {
		t.Errorf("a pause that was waited for: %v", err)
	}

	// Without a pause of its own, a retrying source waits by the clock.
	retrying := NewRetrying(&flaky{t: t, always: func(int) error { return &Error{Kind: Network, Message: "connection reset"} }}, nil)
	go func() {
		_, err := retrying.Get(stopped, "people.json")
		done <- err
	}()
	select {
	case err := <-done:
		expectKind(t, err, Interrupted)
	case <-time.After(3 * time.Second):
		t.Fatal("the pause was waited for instead of stopped")
	}
}

func TestAFailureOfTheAPIThatMayPassIsToldFromOneThatStays(t *testing.T) {
	answer := func(code, message string, retryable bool) []byte {
		return []byte(fmt.Sprintf(`{"ok":false,"error":%q,"code":%q,"retryable":%v,"meta":{"request_id":"r-1"}}`, message, code, retryable))
	}
	cases := []struct {
		name      string
		exit      int
		code      string
		message   string
		retryable bool
		want      Kind
	}{
		{"a gateway that lost Basecamp", 7, "api_error", "Request failed (HTTP 525)", false, Unavailable},
		{"a status of a server", 7, "api_error", "Request failed (HTTP 500)", false, Unavailable},
		{"the last status of a server", 7, "api_error", "Request failed (HTTP 599)", false, Unavailable},
		{"a server error", 7, "api_error", "Server error (500)", false, Unavailable},
		{"a gateway error", 7, "api_error", "Gateway error (502)", false, Unavailable},
		{"a request that timed out", 7, "api_error", "Request failed (HTTP 408)", false, Unavailable},
		{"too many requests", 7, "api_error", "Request failed (HTTP 429)", false, Unavailable},
		{"what the Basecamp CLI calls passing", 7, "api_error", "circuit breaker is open", true, Unavailable},
		{"a failure object without a code", 7, "", "Request failed (HTTP 503)", false, Unavailable},

		{"a request Basecamp refuses", 7, "api_error", "Request failed (HTTP 422)", false, API},
		{"a bad request", 7, "api_error", "Request failed (HTTP 400)", false, API},
		{"a status just below a server's", 7, "api_error", "Request failed (HTTP 499)", false, API},
		{"a status that is gone", 7, "api_error", "Request failed (HTTP 410)", false, API},
		{"a message without a status", 7, "api_error", "something is wrong", false, API},
		{"a text that quotes a status", 7, "api_error", "the webhook answered Request failed (HTTP 503) yesterday", false, API},
		{"a text that ends in a status", 7, "api_error", "as seen in Gateway error (502)", false, API},
		{"a status with more behind it", 7, "api_error", "Request failed (HTTP 503) for good", false, API},
		{"a number that is no status", 7, "api_error", "Request failed (HTTP 5000)", false, API},
		{"a fixed form with a line break behind it", 7, "api_error", "Request failed (HTTP 525)\n", false, API},
		{"a fixed form with a space before it", 7, "api_error", " Request failed (HTTP 525)", false, API},
		{"a fixed form with a space behind it", 7, "api_error", "Request failed (HTTP 525) ", false, API},
		{"a status beyond those of a server", 7, "api_error", "Request failed (HTTP 600)", false, API},
		{"a code of another class", 7, "validation", "Request failed (HTTP 503)", true, API},
		{"another exit code", 9, "api_error", "Request failed (HTTP 503)", true, API},
		{"a usage error", 1, "usage", "Request failed (HTTP 503)", false, API},
	}
	for _, c := range cases {
		got := failure(c.exit, answer(c.code, c.message, c.retryable), nil)
		if got.Kind != c.want {
			t.Errorf("%s: kind %q, want %q", c.name, got.Kind, c.want)
		}
		if got.Transient() != (c.want == Unavailable) {
			t.Errorf("%s: may pass %v", c.name, got.Transient())
		}
		if got.Message != "Basecamp: "+c.message {
			t.Errorf("%s: message %q", c.name, got.Message)
		}
	}

	// A class the Basecamp CLI names stays what it is, whatever else it says.
	for exit, want := range map[int]Kind{2: NotFound, 3: AuthRequired, 4: Forbidden} {
		code := failureKinds[exit].code
		if got := failure(exit, answer(code, "Request failed (HTTP 503)", true), nil); got.Kind != want || got.Transient() {
			t.Errorf("exit %d: kind %q, may pass %v", exit, got.Kind, got.Transient())
		}
	}
	// A crash says nothing about Basecamp, whatever it prints.
	if got := failure(7, nil, []byte("Request failed (HTTP 503)")); got.Kind != API {
		t.Errorf("a crash: kind %q", got.Kind)
	}
}

// countingStandIn takes the place of the Basecamp CLI and keeps a line per
// call next to itself, so that a test sees how often it was run.
func countingStandIn(t *testing.T) (binary string, calls func() int) {
	t.Helper()
	binary = standIn(t)
	script := `#!/bin/sh
for last; do :; done
echo "$last" >> "$0.calls"
calls=$(wc -l < "$0.calls")
case "$last" in
	flaky.json)
		if [ "$calls" -le 2 ]; then
			echo '{"ok":false,"error":"Request failed (HTTP 525)","code":"api_error","retryable":false}'; exit 7
		fi
		echo '{"answered":true}' ;;
	gateway.json) echo '{"ok":false,"error":"Gateway error (503)","code":"api_error","retryable":true}'; exit 7 ;;
	refused.json) echo '{"ok":false,"error":"Request failed (HTTP 422)","code":"api_error","retryable":false}'; exit 7 ;;
	text.json) echo "Welcome to Basecamp" ;;
	crash.json) echo "Request failed (HTTP 503)" >&2; exit 7 ;;
	oom.json) kill -KILL $$ ;;
esac
`
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return binary, func() int {
		content, _ := os.ReadFile(binary + ".calls")
		return strings.Count(string(content), "\n")
	}
}

func TestTheBasecampCLIIsRunAgainOnlyAfterAFailureThatMayPass(t *testing.T) {
	cases := []struct {
		path    string
		calls   int
		retried int
		kind    Kind
	}{
		{"flaky.json", 3, 2, ""},
		{"gateway.json", MaxAttempts, MaxAttempts - 1, Unavailable},
		{"refused.json", 1, 0, API},
		{"text.json", 1, 0, API},
		{"crash.json", 1, 0, API},
		{"oom.json", 1, 0, API},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			binary, calls := countingStandIn(t)
			waited := &pauses{}
			retrying := NewRetrying(CLI{Binary: binary}, waited.pause)

			content, err := retrying.Get(t.Context(), c.path)
			if c.kind == "" {
				if err != nil || string(content) != "{\"answered\":true}\n" {
					t.Fatalf("got %q, %v", content, err)
				}
			} else {
				expectKind(t, err, c.kind)
			}
			if calls() != c.calls || retrying.Retried() != c.retried || len(waited.asked) != c.retried {
				t.Errorf("ran the Basecamp CLI %d times, counted %d, waited %v; want %d runs and %d of them again",
					calls(), retrying.Retried(), waited.asked, c.calls, c.retried)
			}
		})
	}
}
