package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/newsmark"
	"github.com/spf13/cobra"
)

func (a *app) newNotificationsCmd() *cobra.Command {
	return group("notifications", "Wait for what is new for you",
		a.newNotificationsWaitCmd(),
	)
}

// newsFilter is what --board and --column let through. A line that names no
// card on a board passes only where neither is given.
type newsFilter struct {
	boards  []string
	columns []string // IDs in lowercase, or titles
}

func (f newsFilter) isEmpty() bool {
	return len(f.boards) == 0 && len(f.columns) == 0
}

func (f newsFilter) lets(line api.Notification) bool {
	if f.isEmpty() {
		return true
	}
	if line.Card == nil {
		return false
	}
	if len(f.boards) > 0 && !slices.Contains(f.boards, line.Card.BoardID) {
		return false
	}
	if len(f.columns) > 0 && !slices.ContainsFunc(f.columns, func(column string) bool {
		return column == line.Card.Column.ID || strings.EqualFold(column, line.Card.Column.Title)
	}) {
		return false
	}
	return true
}

func (a *app) newNotificationsWaitCmd() *cobra.Command {
	var boards, columns []string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait until something new arrives for you, then print it and exit",
		Long: fmt.Sprintf(`Waits until something new arrives in the bell of the signed-in account —
a card of yours was moved, somebody commented, mentioned or assigned you —
prints one line per notification and exits 0. With --json, data is the list
of notifications as Minka sent them.

Use it instead of polling a board: the request is held open by Minka and
answers the moment something arrives, so a wait costs nothing while it waits.
In Claude Code, run it with run_in_background: its exit wakes the agent.

Every session goes on from where it last stopped, so nothing that came in
between is missed: the position is kept per account and session beside the
configuration file. The session is --session, else $%s, else the one
position of this machine for no session. The first wait of a session starts
from now. The position is kept only after the lines are printed, so a wait
that is cut off prints them again rather than losing them. Two waits of the
same session at once may both print the same lines.

--board and --column (an ID or a title, each repeatable) keep only lines
about cards on that board or in that column, where the card stands now; a
line about anything else is left out under either. What they leave out is
read all the same: the next wait of the session does not show it again.

Without --timeout it waits as long as it takes, asking Minka every %d
seconds at most; each ask counts as a sign of life for the boards you
coordinate. With --timeout it exits 12 (timeout) when nothing arrived in
that time. A network failure exits 6, a server error 7, both retryable; the
position is safe. Where Minka no longer knows the position, the wait starts
from now again and says so in a notice: what came in between is not shown.
An ask that comes back empty before its time, from a server or a proxy that
does not hold it, is followed by a pause of 5 seconds that grows to a minute.`, sessionVariable, api.NotificationsHeldAtMost),
		Example: `  minka notifications wait --json
  minka notifications wait --board 0199... --column "Als nächstes" --json
  minka notifications wait --timeout 30m --json`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.checkTheSessionFlag(cmd); err != nil {
				return err
			}
			if cmd.Flags().Changed("timeout") && timeout <= 0 {
				return usageErrorf(cmd, "--timeout is a duration such as 30s, 10m or 2h, longer than nothing")
			}
			filter := newsFilter{}
			for i := range boards {
				if err := requireID(cmd, "--board", &boards[i]); err != nil {
					return err
				}
				filter.boards = append(filter.boards, boards[i])
			}
			for _, column := range columns {
				if id, ok := asID(column); ok {
					column = id
				}
				filter.columns = append(filter.columns, column)
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			path, err := config.Path(a.getenv)
			if err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}
			w := &newsWait{a: a, s: s, path: newsmark.Path(path), session: a.agentSession(), filter: filter,
				again: waitAgain(a.sessionFlag, filter.boards, columns, timeout)}
			if timeout > 0 {
				w.deadline = a.now().Add(timeout)
			}
			return w.run(timeout)
		},
	}
	cmd.Flags().StringArrayVar(&boards, "board", nil, "Only lines about cards on this board, by ID (repeatable)")
	cmd.Flags().StringArrayVar(&columns, "column", nil, "Only lines about cards in this column, by ID or title (repeatable)")
	cmd.Flags().DurationVar(&timeout, "timeout", 0, "Exit 12 when nothing arrived in this time, such as 30m (default: wait as long as it takes)")
	a.addSessionFlag(cmd)
	return cmd
}

// The pause after an ask that came back early and empty: a server that does
// not hold a request, or a proxy in front of it that answers for it, would
// otherwise be asked again at once, thousands of times a second.
const (
	earlyAnswerPause    = 5 * time.Second
	earlyAnswerPauseMax = time.Minute
	heldLongEnough      = 5 * time.Second
)

// newsWait is one run of notifications wait.
type newsWait struct {
	a        *app
	s        *session
	path     string
	session  string
	filter   newsFilter
	deadline time.Time // zero for no end
	again    string    // the command line that waits for what comes next

	// stored is the position this run read or last wrote. The mark is moved on
	// from it alone; once another process has moved the mark past it, this run
	// moves it no further and leaves the mark to that one.
	stored  string
	notices []string
	// passedBy counts the lines the filters left out.
	passedBy int
	// pause is how long to wait after the next ask that comes back early and
	// empty; it grows with every such ask and starts anew with one that held.
	pause time.Duration
}

func (w *newsWait) run(timeout time.Duration) error {
	position, err := newsmark.Read(w.path, w.s.host, w.s.reader, w.session, w.a.now())
	switch {
	case errors.Is(err, newsmark.ErrUnreadable):
		w.notice("the positions in " + w.path + " cannot be read, so the wait started from now and the file is written anew; every session on this machine starts from now")
	case err != nil:
		return &Error{Code: CodeConfig, Message: err.Error(), Hint: "Check that " + w.path + " can be read; deleting it makes every session start from now."}
	case position.Forgotten:
		w.notice(fmt.Sprintf("the position of this session was last kept more than %d days ago and is forgotten, so the wait started from now", int(newsmark.ForgottenAfter.Hours()/24)))
	}

	cursor := position.Cursor
	if position.Found {
		w.stored = cursor
	} else if cursor, err = w.startFromNow(); err != nil {
		return err
	}
	justStarted := !position.Found

	for {
		wait, ok := w.nextWait()
		if !ok {
			return w.timedOut(timeout)
		}
		asked := w.a.now()
		page, err := w.s.client.Notifications(w.a.ctx, cursor, wait, api.NotificationsPageMax)
		if isCursorUnknown(err) {
			// A position handed out a moment ago that is unknown at once will
			// be unknown however often the wait starts anew.
			if justStarted {
				return &Error{Code: CodeAPI, Message: w.s.host + " does not know the position it handed out a moment ago", APICode: api.CodeCursorUnknown, Notices: w.passedByNotice()}
			}
			w.notice("Minka no longer knows where this session stopped reading, so the wait started from now; what came in between is not shown")
			if cursor, err = w.startFromNow(); err != nil {
				return err
			}
			justStarted = true
			continue
		}
		if err != nil {
			return w.s.fail(err)
		}
		justStarted = false

		lines, raw, next, err := w.drain(page, cursor)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			cursor = next
			if err := w.pauseIfEarly(asked, wait); err != nil {
				return err
			}
			continue
		}
		w.pause = 0

		var kept []api.Notification
		var keptRaw []json.RawMessage
		for i, line := range lines {
			if w.filter.lets(line) {
				kept = append(kept, line)
				keptRaw = append(keptRaw, raw[i])
			}
		}
		w.passedBy += len(lines) - len(kept)
		if len(kept) == 0 {
			// Nothing to print, but read: the mark moves on so that the next
			// wait of the session does not go through it again.
			if err := w.moveOn(next); err != nil {
				return err
			}
			cursor = next
			continue
		}

		if err := w.a.ok(w.result(kept, keptRaw)); err != nil {
			return err
		}
		// The lines are out. A mark that cannot be written now only means
		// that the next wait prints them again, which is said where it does
		// not spoil the answer.
		if err := w.moveOn(next); err != nil {
			w.a.warn([]string{"the position of this session could not be kept, so the next wait shows these lines again: " + err.Error()})
		}
		return nil
	}
}

// notice says something once, however often it happens in a run.
func (w *newsWait) notice(text string) {
	if !slices.Contains(w.notices, text) {
		w.notices = append(w.notices, text)
	}
}

// pauseIfEarly waits before the next ask where this one came back empty in
// less than half the time it asked to be held, and in less than
// heldLongEnough. Half, not a second less: the last ask of a timeout may ask
// for a single second, and an answer at once to that is as early as any. And
// never more than heldLongEnough, because an installation may hold a request
// for less than it was asked, as Minka does past its setting; an answer that
// was held that long is a server that holds, and the next ask costs nothing
// worth a pause. The pause grows from
// earlyAnswerPause to earlyAnswerPauseMax, by up to a quarter more at random
// so that the waits of many machines do not fall into step, and never
// reaches past the end of the wait.
func (w *newsWait) pauseIfEarly(asked time.Time, wait int) error {
	held := w.a.now().Sub(asked)
	if 2*held >= time.Duration(wait)*time.Second || held >= heldLongEnough {
		w.pause = 0
		return nil
	}
	if w.pause == 0 {
		w.pause = earlyAnswerPause
	}
	pause := w.pause + time.Duration(rand.Int64N(int64(w.pause/4)+1))
	w.pause = min(2*w.pause, earlyAnswerPauseMax)
	if !w.deadline.IsZero() {
		pause = min(pause, max(w.deadline.Sub(w.a.now()), 0))
	}
	if err := w.a.pause(w.a.ctx, pause); err != nil {
		return w.s.fail(&api.NetworkError{Method: "GET", Err: err})
	}
	return nil
}

// startFromNow asks where the bell stands and keeps that as the position of
// the session.
func (w *newsWait) startFromNow() (string, error) {
	page, err := w.s.client.Notifications(w.a.ctx, "", 0, api.NotificationsPageMax)
	if err != nil {
		return "", w.s.fail(err)
	}
	cursor := page.Value.Cursor
	if cursor == "" {
		return "", &Error{Code: CodeAPI, Message: w.s.host + " answered the list of notifications without a position to wait from"}
	}
	if err := w.moveOn(cursor); err != nil {
		return "", err
	}
	return cursor, nil
}

// nextWait is how long the next ask may be held: as long as Minka holds one
// by default, or what is left of the time given, rounded up to a whole
// second. Asking for more would gain nothing, since Minka answers empty after
// its own time, and would make an answer that held for all of it look early.
func (w *newsWait) nextWait() (int, bool) {
	if w.deadline.IsZero() {
		return api.NotificationsHeldAtMost, true
	}
	left := w.deadline.Sub(w.a.now())
	if left <= 0 {
		return 0, false
	}
	return min(api.NotificationsHeldAtMost, int(math.Ceil(left.Seconds()))), true
}

// drain reads on without waiting while a page comes back full, so that a run
// prints everything that has piled up and not just the first page of it. The
// position behind a page is the one it names, else that of its last line.
//
// An answer that carries lines and does not move past the position it was
// asked from ends the run: printing them would print them again in every run
// after it. A position refused on a following page ends it as any refusal
// does, the mark where it was; the next wait asks from there and starts from
// now where Minka no longer knows it.
func (w *newsWait) drain(page api.Result[api.NotificationsPage], after string) ([]api.Notification, []json.RawMessage, string, error) {
	var lines []api.Notification
	var raw []json.RawMessage
	asked := map[string]bool{after: true}
	for {
		each, err := api.LinesOf(page)
		if err != nil {
			return nil, nil, "", w.s.fail(err)
		}
		lines = append(lines, page.Value.Notifications...)
		raw = append(raw, each...)
		next := page.Value.Cursor
		if count := len(page.Value.Notifications); next == "" && count > 0 {
			next = page.Value.Notifications[count-1].Cursor
		}
		if next == "" {
			return nil, nil, "", &Error{Code: CodeAPI, Message: w.s.host + " answered the list of notifications without a position to read on from"}
		}
		if len(page.Value.Notifications) > 0 && next == after {
			return nil, nil, "", &Error{Code: CodeAPI, Message: w.s.host + " answered notifications that do not lead past the position they were asked from"}
		}
		if len(page.Value.Notifications) < api.NotificationsPageMax {
			return lines, raw, next, nil
		}
		// A full page that hands back a position already asked from would
		// ask the same question for ever.
		if asked[next] {
			return nil, nil, "", &Error{Code: CodeAPI, Message: w.s.host + " answered a full page of notifications that leads back to a position already read from"}
		}
		asked[next] = true
		after = next
		if page, err = w.s.client.Notifications(w.a.ctx, next, 0, api.NotificationsPageMax); err != nil {
			return nil, nil, "", w.s.fail(err)
		}
	}
}

// moveOn keeps a position as the mark of the session. Where another process
// moved the mark meanwhile, nothing is written and this run moves it no
// further.
func (w *newsWait) moveOn(to string) error {
	ctx, cancel := context.WithTimeout(w.a.ctx, w.a.markWait)
	defer cancel()
	written, err := newsmark.MoveOn(ctx, w.path, w.s.host, w.s.reader, w.session, w.stored, to, w.a.now())
	if err != nil {
		return &Error{Code: CodeConfig, Message: "keeping the position of this session in " + w.path + ": " + err.Error(), Hint: "Nothing was printed and nothing was lost. Check that the file and its directory can be written."}
	}
	if written {
		w.stored = to
	}
	return nil
}

func (w *newsWait) passedByNotice() []string {
	if w.passedBy == 0 {
		return w.notices
	}
	return append(slices.Clone(w.notices), plural(w.passedBy, "notification", "notifications")+" left out by --board or --column, and read all the same")
}

func (w *newsWait) result(lines []api.Notification, raw []json.RawMessage) success {
	s := success{
		data:    raw,
		summary: plural(len(lines), "new notification", "new notifications"),
		notices: w.passedByNotice(),
		breadcrumbs: []Breadcrumb{
			{Action: "wait", Cmd: w.again, Description: "Wait for what comes next, with the same session and filters"},
		},
		text: func(out io.Writer) {
			for _, line := range lines {
				fmt.Fprintln(out, newsLine(line))
			}
		},
	}
	for _, line := range lines {
		if line.Subject.Type == "card" {
			s.breadcrumbs = append(s.breadcrumbs, Breadcrumb{Action: "show", Cmd: "minka cards show " + line.Subject.ID, Description: "Show the card of a notification"})
			break
		}
	}
	return s
}

// waitAgain is the command line of a wait with the session and the filters of
// this one, so that following it neither changes the position nor shows what
// this one left out.
func waitAgain(sessionFlag string, boards, columns []string, timeout time.Duration) string {
	words := []string{"minka", "notifications", "wait"}
	for _, board := range boards {
		words = append(words, "--board", shellWord(board))
	}
	for _, column := range columns {
		words = append(words, "--column", shellWord(column))
	}
	if sessionFlag != "" {
		words = append(words, "--session", shellWord(sessionFlag))
	}
	if timeout > 0 {
		words = append(words, "--timeout", timeout.String())
	}
	return strings.Join(append(words, "--json"), " ")
}

// newsLine is one notification as a person reads it: what it is about, then
// what happened and who did it, in Minka's words.
func newsLine(line api.Notification) string {
	about := oneLine(line.Subject.Title)
	if line.Subject.ShortID != nil && *line.Subject.ShortID != "" {
		about = oneLine(*line.Subject.ShortID) + " " + about
	}
	if about == "" {
		return oneLine(line.Summary)
	}
	return about + ": " + oneLine(line.Summary)
}

func (w *newsWait) timedOut(timeout time.Duration) error {
	return &Error{
		Code:      CodeTimeout,
		Message:   "nothing new within " + timeout.String(),
		Hint:      "Run the wait again; it goes on from where this one stopped.",
		Retryable: true,
		Notices:   w.passedByNotice(),
	}
}

func isCursorUnknown(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.Code == api.CodeCursorUnknown
}
