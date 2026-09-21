package basecamp

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// RetryPauses are the pauses before a request is asked again, in their order.
// They grow, because what failed a moment ago most likely still fails, and
// they stay short next to a run of hours.
var RetryPauses = [...]time.Duration{2 * time.Second, 10 * time.Second, 30 * time.Second}

// MaxAttempts is how often one request is asked at most.
const MaxAttempts = len(RetryPauses) + 1

// MaxRetriesOfARun is how often one run asks again at most, every further
// attempt of every request counted. A source that fails now and then stays
// far below it. One that fails all the time would stretch a run by the pauses
// of thousands of requests, so such a run ends instead.
const MaxRetriesOfARun = 100

// Pause waits for a duration, or until the context is done, which it answers
// with an error.
type Pause func(ctx context.Context, duration time.Duration) error

// Retrying is a Source that asks another Source again when a request failed
// for a reason that may pass. A run makes thousands of requests over hours,
// and one of them that fails for a moment must not cost all the others.
//
// Every other failure comes through at once, and so does the last one when
// the attempts of a request or the retries of the run are used up. It asks
// one request at a time, like the Source behind it, and is not made for
// several goroutines.
type Retrying struct {
	source  Source
	pause   Pause
	retried int
}

// NewRetrying wraps a Source. A nil pause waits by the clock.
func NewRetrying(source Source, pause Pause) *Retrying {
	if pause == nil {
		pause = waitFor
	}
	return &Retrying{source: source, pause: pause}
}

func (r *Retrying) Get(ctx context.Context, path string) ([]byte, error) {
	for attempt := 1; ; attempt++ {
		content, err := r.source.Get(ctx, path)
		var failed *Error
		if err == nil || !errors.As(err, &failed) || !failed.Transient() || attempt >= MaxAttempts {
			return content, err
		}
		if r.retried >= MaxRetriesOfARun {
			return nil, &Error{
				Kind:    failed.Kind,
				Message: fmt.Sprintf("%s; Basecamp kept failing: %d requests of this run were asked again already, and no further one is", failed.Message, MaxRetriesOfARun),
				Hint:    failed.Hint,
			}
		}
		// A run that is stopped while it waits ends now, not after the pause
		// and not with one more request.
		if r.pause(ctx, RetryPauses[attempt-1]) != nil || ctx.Err() != nil {
			return nil, &Error{Kind: Interrupted, Message: "interrupted"}
		}
		r.retried++
	}
}

// Retried is how many requests were asked again so far, each further attempt
// counted.
func (r *Retrying) Retried() int {
	return r.retried
}

func waitFor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
