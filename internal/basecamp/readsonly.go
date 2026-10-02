package basecamp

import (
	"context"
	"fmt"
	"regexp"
)

// readsThatWrite are the addresses Basecamp answers by writing at the source: a
// single day of a series is a recording Basecamp makes the first time somebody
// asks for it, and the entry of a series answers with such a day. The entries
// of a calendar are read from their list alone, so no address of a single
// entry is ever one a run needs.
var readsThatWrite = regexp.MustCompile(`(^|/)schedule_entries/`)

// readsOnly is the source every request of a run goes through. It refuses an
// address that would write at the source before it is asked for, whatever part
// of the run asks, so that a run stays a read of the account.
type readsOnly struct {
	source Source
}

func (r readsOnly) Get(ctx context.Context, path string) ([]byte, error) {
	if readsThatWrite.MatchString(path) {
		return nil, &Error{Kind: API, Message: fmt.Sprintf("a run never asks for %s: Basecamp makes a record of a single day of a schedule where somebody asks for it", path)}
	}
	return r.source.Get(ctx, path)
}
