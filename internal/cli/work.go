package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/work"
	"github.com/spf13/cobra"
)

const (
	// signalEvery is how often the cards of a session are reported. The
	// server counts a signal for three minutes, so two may be lost before a
	// tile goes quiet.
	signalEvery = time.Minute

	// signalTimeout bounds one signal. It runs after every tool call of an
	// agent, so a host that does not answer may hold the work up for a moment
	// and no longer.
	signalTimeout = 5 * time.Second

	// waitForTheStore is how long a command a person or an agent typed waits
	// for the lock. For such a command there is no next time, so it waits its
	// turn behind every other minka at the store. Each of them holds the lock
	// for one read and one write of a small file, which a line of sessions
	// starting at once gets through in well under a second; only a holder that
	// hangs, or a disk that does not answer, keeps it waiting this long. A
	// command that reports on its own behalf waits like the command it is part
	// of when it claims the signal.
	waitForTheStore = 30 * time.Second

	// aBeatWaitsForTheStore is how long what is written beside a beat waits for
	// the lock: the outcome of a signal. Nobody typed it and the next beat comes
	// within a minute, while a beat that waited longer would pile up behind the
	// ones after it. The claim of a beat waits not at all, for the same reason.
	aBeatWaitsForTheStore = 2 * time.Second
)

func (a *app) newWorkCmd() *cobra.Command {
	return group("work", "Say which cards you are working on, so the board shows them as alive",
		a.newWorkStartCmd(),
		a.newWorkStopCmd(),
		a.newWorkListCmd(),
	)
}

func (a *app) newWorkStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start <card>",
		Short: "Remember that you are working on a card",
		Long: `Remembers a card as one you are working on.

The board shows such a card as alive while minka keeps saying so. Saying so is
the job of 'minka heartbeat', which the mod of 'minka hooks install' runs from
inside the Claude Code process after every step and every thirty seconds: until
one of them has run, 'work start' has changed nothing but this session's own
list.

The list belongs to the session that took the card up, the one Claude Code
names in $CLAUDE_CODE_SESSION_ID or --session names, and only that session's
heartbeat reports it: when the session ends, its cards go dark with it instead
of being reported by every other session on the machine. Without a session,
the card goes into the list of no session, which only a heartbeat without one
reports.

'work stop' takes a card out again. So do 'cards move' and 'cards done': a card
that is moved on is one nobody is at work on any more, so take a card up in
that order — move it first, then say 'work start'.

A card in the archive of its board is refused: it can be read, and no board
shows a card in an archive as alive.`,
		Example: `  minka work start MNK-1A
  minka work start 0199... --json`,
		Args: requireArgs("cardId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.checkTheSessionFlag(cmd); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			card, err := s.findCard(a.ctx, args[0])
			if err != nil {
				return err
			}
			// A card in the archive of its board can be read and not reported:
			// the host leaves it out of every set at work. It is refused here,
			// at the one moment somebody can do something about it, rather
			// than taken up and let go again by the next beat, which says
			// nothing to anybody.
			if card.Archived {
				return &Error{
					Code:    CodeUsage,
					Message: "the card lies in the archive of its board, and no board shows a card in an archive as alive",
					Hint:    "Take the card out of the archive, then say: minka work start " + args[0],
				}
			}

			handle, err := a.openStore(waitForTheStore)
			if err != nil {
				return err
			}
			defer handle.Close()

			session := a.agentSession()
			host := handle.File.Host(s.host)
			forgotten, _ := host.ForgetWhatRanOut(session, a.now())
			entry := host.List(session)
			entry.Beat(a.now())
			entry.Start(work.Card{
				ID:        card.ID,
				ShortID:   shortIdentifierOf(card),
				Title:     card.Title,
				StartedAt: a.now(),
			})
			if err := handle.Save(); err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}

			started := a.workSuccess(host, session, "Working on "+cardName(shortIdentifierOf(card), card.Title), nil, forgotten)
			started.notices = append(started.notices, noticesAboutTheCard(entry.LetGo, card.ID)...)
			if session == work.NoSession && a.getenv(claudeCodeVariable) == "1" {
				started.notices = append(started.notices, "Claude Code names no session here, so the card is in the list of no session, which the mod of a session never reports; set $"+sessionVariable+" or pass --session for a list of its own")
			}
			return a.ok(started)
		},
	}
	a.addSessionFlag(cmd)
	return cmd
}

func (a *app) newWorkStopCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "stop [<card>]",
		Short: "Forget that you are working on a card",
		Long: `Takes a card out of the list this session reports.

The next signal of the session leaves it out, and the board stops showing it
as alive at once rather than after the three minutes a signal counts for.

Without a card it stops the one card this session has in hand. Where it has
several, it says which and changes nothing: name the card, or take all of this
session's cards with --all.

A card named that this session does not hold is taken out of the list of
whichever other session on this machine holds it: whoever stops a card has let
go of it. The board hears of that with the next signal of that session, or
forgets the card three minutes after its last one.

It asks nothing of the network: a card is found in the list under either of
the names it was remembered with, and one that is not remembered is nothing to
stop. It needs the host all the same, so a machine that is signed in nowhere
has nothing to answer about.`,
		Example: `  minka work stop MNK-1A
  minka work stop --all`,
		Args: optionalCardArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.checkTheSessionFlag(cmd); err != nil {
				return err
			}
			hostName, err := a.hostOfTheStore()
			if err != nil {
				return err
			}
			handle, err := a.openStore(waitForTheStore)
			if err != nil {
				return err
			}
			defer handle.Close()

			// A host that holds nothing gets no entry of its own here: there is
			// nothing to stop, and nothing to write down about it either.
			session := a.agentSession()
			host, known := handle.File.Hosts[hostName]
			if !known {
				return a.ok(sayingFirst("Nothing to stop", a.workSuccess(&work.Host{}, session, "Nothing to stop", map[string]any{"stopped": cardList(nil)}, 0)))
			}
			forgotten, swept := host.ForgetWhatRanOut(session, a.now())
			entry, held := host.Sessions[session]
			if !held {
				entry = &work.List{}
			}
			beaten := held && entry.Beat(a.now())

			var stopped []work.Card
			switch {
			case all:
				stopped = entry.Cards
				entry.Cards = nil
			case len(args) == 1:
				if card, found := namedInTheList(entry, args[0]); found {
					entry.Stop(card.ID)
					stopped = []work.Card{card}
				} else if card, found := stopElsewhere(host, args[0]); found {
					stopped = []work.Card{card}
				}
			case len(entry.Cards) == 1:
				stopped = entry.Cards
				entry.Cards = nil
			case len(entry.Cards) > 1:
				return &Error{
					Code:    CodeUsage,
					Message: "this session is working on " + strings.Join(namesOf(entry.Cards), ", "),
					Hint:    "Name the card to stop, or take them all with --all.",
				}
			}

			if len(stopped) > 0 || swept || beaten {
				if err := handle.Save(); err != nil {
					return &Error{Code: CodeConfig, Message: err.Error()}
				}
			}

			summary := "Nothing to stop"
			if len(stopped) > 0 {
				summary = "Stopped " + strings.Join(namesOf(stopped), ", ")
			}
			answer := sayingFirst(summary, a.workSuccess(host, session, summary, map[string]any{"stopped": cardList(stopped)}, forgotten))
			if len(args) == 1 {
				// A card that is nothing to stop may be one the host would not
				// have, and then this is where somebody learns it.
				answer.notices = append(answer.notices, noticesAboutTheCard(entry.LetGo, args[0])...)
			}
			return a.ok(answer)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Stop every card this session is working on")
	a.addSessionFlag(cmd)
	return cmd
}

// stopElsewhere takes a card out of the list of every other session on the host
// that holds it, and answers with the card as one of them remembered it.
func stopElsewhere(host *work.Host, name string) (work.Card, bool) {
	var stopped work.Card
	found := false
	for _, list := range host.Sessions {
		if card, held := namedInTheList(list, name); held {
			list.Stop(card.ID)
			stopped, found = card, true
		}
	}
	return stopped, found
}

func (a *app) newWorkListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Show the cards this session is working on",
		Long: `Shows what this session reports as being worked on, and what the other
sessions on this machine have in hand.

'cards' is the list of this session, the one its heartbeat reports; 'session'
says which session that is, null for the list of no session. 'elsewhere' are
the cards of every other session on this machine, each with its session: they
are reported by those sessions alone. Everything is read from disk: the server
cannot be asked what it holds.

Going out and arriving are two moments. 'sentAt' says when a signal last went
out, 'reportedAt' when one last arrived, and 'failure' what the host made of
the last one that did not. A signal that never got as far as a request —
nobody signed in, the store held by somebody else — leaves nothing behind;
'minka heartbeat --json' says what it made of the moment.

'letGo' are the cards the host would not have, with the word it gave for each.
A card in the archive of its board is one of them: it is out of the list, and
this is where it says why, for eight hours after it happened.`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.checkTheSessionFlag(cmd); err != nil {
				return err
			}
			hostName, err := a.hostOfTheStore()
			if err != nil {
				return err
			}
			handle, err := a.openStore(waitForTheStore)
			if err != nil {
				return err
			}
			defer handle.Close()

			// What ran out is gone for every reader; the next command that
			// writes the store takes it out of the file as well.
			session := a.agentSession()
			host, known := handle.File.Hosts[hostName]
			if !known {
				host = &work.Host{}
			}
			forgotten, _ := host.ForgetWhatRanOut(session, a.now())
			entry := listOf(host, session)
			summary := fmt.Sprintf("%d cards", len(entry.Cards))
			if len(entry.Cards) == 1 {
				summary = "1 card"
			}
			listed := a.workSuccess(host, session, summary+" on "+hostName, nil, forgotten)
			listed.notices = append(listed.notices, letGoNotices(entry.LetGo)...)
			return a.ok(listed)
		},
	}
	a.addSessionFlag(cmd)
	return cmd
}

// listOf is the list of a session as a command reads it, an empty one where the
// session holds nothing. It is never put into the host: reading a list does not
// make one.
func listOf(host *work.Host, session string) *work.List {
	if list, held := host.Sessions[session]; held {
		return list
	}
	return &work.List{}
}

func (a *app) newHeartbeatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "heartbeat",
		Short: "Report the cards this session is working on",
		Long: `Reports the cards of 'minka work start' to Minka, so their tiles stay alive.

It is made to run from inside the Claude Code process of an agent, which the
mod of 'minka hooks install' does after every step and every thirty seconds.
With nothing remembered it returns at once and asks nothing of the network;
otherwise it reports at most once a minute, and again right away whenever the
list has changed. The board forgets a card three minutes after the last signal,
so whatever else runs it has to run it at least that often.

It reports the list of one session and nothing else, under a client of that
session's own: the session --session names, else $CLAUDE_CODE_SESSION_ID, the
same one a command typed in that session goes by. Without either it reports
the list of no session. 'session' says which it was.

It always exits 0 and says nothing unless asked with --json, --quiet or on a
terminal: a beat that fails loudly after every tool call would cost the agent
its attention for something it cannot act on. Asked, it says in 'reason' what
it made of the moment, with the status and the code of the API where a request
was turned away; what the last signal that went out made of it also stands in
'minka work list'.

A card the host will not have — one that was archived while somebody had it in
hand — is taken out of the list of this session, so the cards beside it go on
being reported, and the shorter list goes out with the next beat. It stands in
'letGo', here and in 'minka work list'. A word this CLI does not know takes no
card out of anything and is reported as it came.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			var beat signalled
			var session string
			switch {
			case a.sessionFlag != "" && !work.IsSession(a.sessionFlag):
				// Refused without a word, like everything else a beat meets:
				// reporting under a name it was not given would be worse.
				beat = signalled{reason: "--session names no session"}
			case a.fromMod && a.agentSession() == work.NoSession:
				// The mod reports one session, and the list of no session is
				// nobody's to keep alight from inside a process.
				beat = signalled{reason: "the mod names no session"}
			default:
				session = a.agentSession()
				beat = a.signal(session, 0)
			}
			if beat.cards == nil {
				beat.cards = []string{}
			}

			if a.nobodyIsListening() {
				return nil
			}
			summary := heartbeatSummary(beat.sent, beat.reason, beat.cards)
			return a.ok(success{
				data: map[string]any{"sent": beat.sent, "reason": beat.reason, "cardIds": beat.cards,
					"session": nullable(session), "letGo": letGoList(beat.letGo)},
				summary: summary,
				notices: letGoNotices(beat.letGo),
				text:    func(w io.Writer) { fmt.Fprintln(w, summary) },
			})
		},
	}
	cmd.Flags().BoolVar(&a.fromMod, "from-mod", false, "The beat comes from the mod inside a Claude Code process; minka's own business")
	_ = cmd.Flags().MarkHidden("from-mod")
	cmd.Flags().BoolVar(&a.idle, "idle", false, "The beat says the process is alive, not that anybody is at work; minka's own business")
	_ = cmd.Flags().MarkHidden("idle")
	a.addSessionFlag(cmd)
	return cmd
}

// nobodyIsListening says whether this run has nobody to say anything to: no flag
// asked for output and no terminal to print it on. A beat that speaks after
// every tool call costs the agent its attention for something it cannot act on.
func (a *app) nobodyIsListening() bool {
	return !a.jsonFlag && !a.quietFlag && !a.styledFlag && !a.stdoutIsTerminal()
}

// signalled is what became of one signal: whether it went out, why it did not,
// which cards the host took, and what it said about the ones it did not.
type signalled struct {
	sent   bool
	reason string
	cards  []string
	letGo  []work.LetGoCard
}

// signal reports the cards of one session and says what it did. It reports
// that session's list and no other, under that session's client: the server
// keeps one set per client, so a session that carried the cards of another
// would keep them alight after that one died.
//
// Nothing it meets is a failure of the command: not being signed in, a host
// that does not answer, a token a person owns. Whoever reads a beat's exit
// code is in the middle of other work, and none of these is something they
// could do anything about from there.
func (a *app) signal(session string, wait time.Duration) signalled {
	path, err := a.storePath()
	if err != nil || work.Untouched(path) {
		return signalled{reason: "nothing is remembered on this machine"}
	}
	s, err := a.session()
	if err != nil {
		return signalled{reason: "not signed in"}
	}

	claimed, reason := a.claimTheSignal(path, s.host, session, wait)
	if claimed == nil {
		return signalled{reason: reason}
	}

	ctx, cancel := context.WithTimeout(a.ctx, signalTimeout)
	defer cancel()
	leftOut, err := s.client.NoteAtWork(ctx, claimed.clientID, claimed.cards)
	noted := a.noteTheOutcome(path, s.host, session, err, leftOut)
	if err != nil {
		// The failure as the list keeps it, so that the one word an agent
		// reads here names the case: the status and the code of the API are
		// what tells a revoked token from a person's one.
		return signalled{reason: err.Error(), cards: claimed.cards}
	}
	return signalled{sent: true, cards: whatTheHostTook(claimed.cards, leftOut), letGo: noted}
}

// whatTheHostTook is the set that is now at work on the host: what was sent,
// without the cards the host named as left out.
func whatTheHostTook(sent []string, leftOut []api.LeftOutCard) []string {
	took := make([]string, 0, len(sent))
	for _, id := range sent {
		if !slices.ContainsFunc(leftOut, func(c api.LeftOutCard) bool { return c.CardID == id }) {
			took = append(took, id)
		}
	}
	return took
}

// claim is a signal that is to go out: who says it and what it says.
type claim struct {
	clientID string
	cards    []string
}

// claimTheSignal decides under the lock whether a signal goes out now, and
// writes down that it does before it goes.
//
// The moment is claimed before the request rather than after it, because every
// signal is a process of its own: two that start together must not both
// report, and one that fails must not have the next tool call try again at
// once. It claims nothing where nothing is to be sent, and says why.
func (a *app) claimTheSignal(path, hostName, session string, wait time.Duration) (*claim, string) {
	handle, err := work.Open(path, wait)
	if err != nil {
		return nil, "the store is busy or unreadable"
	}
	defer handle.Close()

	host, known := handle.File.Hosts[hostName]
	if !known {
		return nil, "nothing is remembered for " + hostName
	}
	// What has run out is given up before anybody says they are still at it, so
	// that a beat which comes after the window cannot raise a card that is
	// over. The sign of life stands whatever the throttle then makes of the
	// moment: it is about the work, not about the host's ear.
	_, changed := host.ForgetWhatRanOut(session, a.now())
	if a.somebodyIsAtWork(host, session) {
		changed = true
	}
	entry, held := host.Sessions[session]
	if !held {
		return nil, a.saving(handle, changed, "nothing is remembered for this session on "+hostName)
	}
	if a.idle {
		if why := nobodyIsAtWorkOn(entry, a.now()); why != "" {
			return nil, a.saving(handle, changed, why)
		}
	}
	cards := entry.CardIDs()

	// Nothing to say, and nothing said before that would have to be taken
	// back: this is where the common case ends, before the network.
	if len(cards) == 0 && len(entry.SentCards) == 0 {
		return nil, a.saving(handle, changed, "nothing is remembered for this session on "+hostName)
	}
	if slices.Equal(cards, entry.SentCards) && a.now().Sub(entry.SentAt) < signalEvery {
		return nil, a.saving(handle, changed, "the last signal is younger than a minute")
	}

	clientID, err := a.clientID(session)
	if err != nil {
		// The one thing that can go wrong while the store is open and the host
		// is known, so it is the one thing a list can be told about.
		entry.Failure = err.Error()
		entry.FailedAt = a.now()
		return nil, a.saving(handle, true, err.Error())
	}

	entry.SentAt = a.now()
	entry.SentCards = cards
	if err := handle.Save(); err != nil {
		return nil, err.Error()
	}
	return &claim{clientID: clientID, cards: cards}, ""
}

// somebodyIsAtWork writes down what a beat says about the work behind it: that
// the session is still at the cards it took up, and that it was at the store at
// all. It answers whether anything was written. A session that holds nothing
// gets no list for this: there is nothing of its own to keep alive.
//
// A beat that only says the process is alive says neither: it is the work that
// keeps a card, and a process on a timer of its own is no work. Otherwise a
// Claude Code left open would keep its cards glowing through a working day
// while nobody is at them.
func (a *app) somebodyIsAtWork(host *work.Host, session string) bool {
	if a.idle {
		return false
	}
	entry, held := host.Sessions[session]
	if !held {
		return false
	}
	written := host.StillAtIt(session, a.now())
	marked := entry.Beat(a.now())
	return written || marked
}

// saving writes the store where the sweep changed something and hands the
// reason through, so that a signal that sends nothing still tidies up.
func (a *app) saving(handle *work.Handle, changed bool, reason string) string {
	if changed {
		if err := handle.Save(); err != nil {
			return err.Error()
		}
	}
	return reason
}

// noteTheOutcome writes down how the signal fared, so that a list somebody
// reads later can say why the board shows nothing. It is the one thing this
// command gives up on quietly: the work itself is done, and the store may well
// be held by the next session.
//
// A list that is gone by now — one whose empty set went out and was swept —
// gets nothing written: there is nobody left to read it.
func (a *app) noteTheOutcome(path, hostName, session string, failed error, leftOut []api.LeftOutCard) []work.LetGoCard {
	handle, err := work.Open(path, aBeatWaitsForTheStore)
	if err != nil {
		return nil
	}
	defer handle.Close()

	var noted []work.LetGoCard
	host, known := handle.File.Hosts[hostName]
	if !known {
		return nil
	}
	entry, held := host.Sessions[session]
	if !held {
		return nil
	}
	if failed != nil {
		entry.Failure = failed.Error()
		entry.FailedAt = a.now()
	} else {
		noted = a.writeDownWhatTheHostWouldNotHave(entry, leftOut)
		entry.ReportedAt = a.now()
		entry.Failure = ""
		entry.FailedAt = time.Time{}
	}
	_ = handle.Save()
	return noted
}

// writeDownWhatTheHostWouldNotHave takes the cards the host will not have out
// of the list, so that the next signal carries what it does take and the tiles
// of the others stay alight. It answers with the notes it wrote, which carry
// the names: after a card is out of the list, nothing else names it.
//
// A word this CLI does not know takes no card out of anything. It is written
// down and the card stays, because a list of this session is not the host's to
// shorten with a word nobody here can read. Such a set is not sent again before
// the minute is out either: the list is what it was, and the host has said what
// it thinks of it.
//
// What was sent stays what was sent. A list that is now shorter than the last
// signal is a list that has changed, and a changed list is reported at once, so
// the corrected set is at the host with the next beat rather than a minute
// later.
func (a *app) writeDownWhatTheHostWouldNotHave(entry *work.List, leftOut []api.LeftOutCard) []work.LetGoCard {
	var noted []work.LetGoCard
	for _, left := range leftOut {
		if left.Reason != api.ReasonCardArchived {
			noted = append(noted, entry.NoteNotTaken(left.CardID, left.Reason, a.now()))
			continue
		}
		noted = append(noted, entry.LetGoOf(left.CardID, left.Reason, a.now()))
	}
	return noted
}

// noLongerInHand is what a command says when it took the card out of the lists
// of this machine on its own. Nothing changes a list without a word, and the
// word says how far the change has got.
func noLongerInHand(told bool) string {
	if told {
		return "the card is no longer among the cards anybody on this machine is working on, and the board was told at once"
	}
	return "the card is no longer among the cards anybody on this machine is working on; the board hears of it with the next signal of the session that held it"
}

// letGoOfTheCard takes a card out of every list of this machine and reports the
// list of this session at once, so that a card somebody has moved on stops
// glowing instead of waiting for the signal after the next tool call. It says
// whether the card was in a list at all, and whether the board was told.
//
// The list of another session is not reported from here: a signal carries the
// whole list it reports, and this session would be keeping that session's other
// cards alight. That session's next beat reports the shorter list, and where it
// has none left, the server forgets the card three minutes after its last one.
//
// Whatever it meets on the way it passes over in silence: the card has been
// moved, and neither a store another session holds at that moment nor a signal
// that does not get through makes that untrue.
func (a *app) letGoOfTheCard(host, cardID string) (letGo, told bool) {
	session := a.agentSession()
	gone, mine := a.takeOutOfTheLists(host, session, cardID)
	if !gone || !mine {
		return gone, false
	}
	return true, a.signal(session, waitForTheStore).sent
}

// takeOutOfTheLists is the half that holds the lock, kept apart so that it is
// given back before the signal, which takes it for itself. It says whether the
// card was in any list, and whether in the one of this session.
func (a *app) takeOutOfTheLists(hostName, session, cardID string) (gone, mine bool) {
	handle, err := a.openStore(waitForTheStore)
	if err != nil {
		return false, false
	}
	defer handle.Close()

	host, known := handle.File.Hosts[hostName]
	if !known {
		return false, false
	}
	// Whoever writes the store sweeps it, so that what ran out is gone for the
	// next reader as well; the card of this command is a different question.
	_, swept := host.ForgetWhatRanOut(session, a.now())
	for name, list := range host.Sessions {
		if list.Stop(cardID) {
			gone = true
			mine = mine || name == session
		}
	}
	if !gone && !swept {
		return false, false
	}
	if handle.Save() != nil {
		return false, false
	}
	return gone, mine
}

// clientID is the client a session reports as, made from the name this
// installation of minka goes by. That name is made once and kept in the
// configuration; every caller holds the lock on the store, so two signals at the
// same moment cannot make two of them.
func (a *app) clientID(session string) (string, error) {
	base, err := a.nameOfThisInstallation()
	if err != nil {
		return "", err
	}
	return clientIDOf(base, session), nil
}

// nameOfThisInstallation is the name the configuration keeps, made on the first
// signal that needs it.
func (a *app) nameOfThisInstallation() (string, error) {
	path, file, err := a.loadConfig()
	if err != nil {
		return "", err
	}
	if file.ClientID != "" {
		return file.ClientID, nil
	}
	made, err := config.NewClientID()
	if err != nil {
		return "", err
	}
	file.ClientID = made
	if err := file.Save(path); err != nil {
		return "", err
	}
	return made, nil
}

func (a *app) storePath() (string, error) {
	path, err := config.Path(a.getenv)
	if err != nil {
		return "", &Error{Code: CodeConfig, Message: err.Error()}
	}
	return work.Path(path), nil
}

func (a *app) openStore(wait time.Duration) (*work.Handle, error) {
	path, err := a.storePath()
	if err != nil {
		return nil, err
	}
	handle, err := work.Open(path, wait)
	if err != nil {
		e := &Error{Code: CodeConfig, Message: err.Error()}
		if errors.Is(err, work.ErrBusy) {
			e.Hint = "Another minka is writing " + path + " right now. Try again."
		}
		return nil, e
	}
	return handle, nil
}

// hostOfTheStore is the host whose cards a command works on. It needs no token:
// what is remembered lies here, and only the signal goes anywhere.
func (a *app) hostOfTheStore() (string, error) {
	host, err := a.explicitHost()
	if err != nil {
		return "", err
	}
	if host != "" {
		return host, nil
	}
	path, file, err := a.loadConfig()
	if err != nil {
		return "", err
	}
	host, err = defaultHostOf(file, path)
	if err != nil {
		return "", err
	}
	if host == "" {
		return "", noHostError(file)
	}
	return host, nil
}

// defaultHostOf is the host of the configuration, read the way a sign-in writes
// it, and empty where the configuration names none. It is a key of the store, so
// every process has to make the same string of it: a host written into the file
// by hand, with a slash at the end or a capital in it, would have one process
// looking at another list than the next.
//
// A value that cannot be read at all is no host, and it is refused here rather
// than handed on. Handed on, it would stand in the session of every command that
// follows and from there reach whatever prints a host.
func defaultHostOf(file *config.File, path string) (string, error) {
	if file.DefaultHost == "" {
		return "", nil
	}
	host, err := config.NormalizeHost(file.DefaultHost)
	if err != nil {
		shown := config.HostForOutput(file.DefaultHost)
		return "", &Error{
			Code:    CodeConfig,
			Message: "the default host in " + path + " cannot be read: " + err.Error(),
			Hint:    "Correct \"defaultHost\" in that file, or sign in again: minka auth login --host " + hostToPaste(shown),
		}
	}

	return host, nil
}

// findCard reads a card by its ID or by the short identifier people call it
// by, in one request either way.
//
// A card is looked up rather than taken as it was typed, so that what cannot
// be seen is said here: the server passes such a card over without a word, and
// a list that quietly holds a card nobody will ever see is worse than a
// refusal at the moment somebody asks for it.
func (s *session) findCard(ctx context.Context, name string) (api.Card, error) {
	var result api.Result[api.Card]
	var err error
	if uuidPattern.MatchString(name) {
		result, err = s.client.Card(ctx, name)
	} else {
		result, err = s.client.CardByShortIdentifier(ctx, strings.ToUpper(name))
	}
	if err != nil {
		e := s.fail(err)
		if e.Code == CodeNotFound {
			e.Hint = "There is no such card, or it is in the trash or in a project you are not a member of. Minka shows nobody a card they may not see, and passes it over without a word when an agent says it is working on it."
		}
		return api.Card{}, e
	}
	if !uuidPattern.MatchString(result.Value.ID) {
		return api.Card{}, &Error{Code: CodeAPI, Message: s.host + " answered for " + name + " with a card that has no ID"}
	}
	return result.Value, nil
}

// workSuccess is the answer of every command that changes or shows the list.
// The subject here is the list of a session and not a resource of the API, so it
// is put together here; a command that has more to say adds it.
//
// What the other sessions of the machine hold stands beside it, so that whoever
// looks can tell a card nobody holds from one another session reports.
func (a *app) workSuccess(host *work.Host, session, summary string, more map[string]any, forgotten int) success {
	entry := listOf(host, session)
	elsewhere := cardsElsewhere(host, session)
	data := map[string]any{
		"session":    nullable(session),
		"cards":      cardList(entry.Cards),
		"elsewhere":  elsewhere,
		"letGo":      letGoList(entry.LetGo),
		"sentAt":     momentOrNil(entry.SentAt),
		"reportedAt": momentOrNil(entry.ReportedAt),
		"failure":    nullable(entry.Failure),
	}
	for name, value := range more {
		data[name] = value
	}

	// Whether anything reports at all is a different question from what the
	// host makes of it. Where a signal went out and was turned away, advising
	// the mod would send somebody after a problem they do not have.
	s := success{data: data, summary: summary}
	if len(entry.Cards) > 0 && entry.SentAt.IsZero() {
		s.notices = append(s.notices, "nothing has reported this list yet, so no board shows these cards as alive; run: minka hooks install")
	}
	if entry.Failure != "" {
		s.notices = append(s.notices, "the last signal did not arrive: "+entry.Failure)
	}
	if forgotten > 0 {
		s.notices = append(s.notices, fmt.Sprintf("%s given up after %.0f hours without a sign of life; on a card you are still at work on, say minka work start again",
			plural(forgotten, "card was", "cards were"), work.ForgottenAfter.Hours()))
	}
	s.breadcrumbs = []Breadcrumb{
		{Action: "heartbeat", Cmd: "minka heartbeat --json", Description: "Report the list to Minka now"},
		{Action: "hooks", Cmd: "minka hooks status --json", Description: "See whether the mod reports it from inside Claude Code"},
	}
	s.text = func(w io.Writer) {
		if len(entry.Cards) == 0 {
			fmt.Fprintln(w, "No card.")
		}
		for _, card := range entry.Cards {
			fmt.Fprintf(w, "%s  %s\n", cardName(card.ShortID, card.Title), card.StartedAt.Format(time.RFC3339))
		}
		switch {
		case !entry.ReportedAt.IsZero():
			fmt.Fprintf(w, "\nLast reported %s\n", entry.ReportedAt.Format(time.RFC3339))
		case !entry.SentAt.IsZero():
			fmt.Fprintf(w, "\nLast sent %s, and nothing has arrived yet\n", entry.SentAt.Format(time.RFC3339))
		}
		if len(elsewhere) > 0 {
			fmt.Fprintln(w, "\nOther sessions on this machine report:")
			for _, name := range otherSessions(host, session) {
				for _, card := range host.Sessions[name].Cards {
					fmt.Fprintf(w, "%s  %s\n", cardName(card.ShortID, card.Title), card.StartedAt.Format(time.RFC3339))
				}
			}
		}
	}
	return s
}

// cardsElsewhere are the cards of every session but the one named, each with the
// session that holds it, in the order of the sessions' names so that two looks
// at the same store read alike.
func cardsElsewhere(host *work.Host, session string) []map[string]any {
	list := []map[string]any{}
	for _, name := range otherSessions(host, session) {
		for _, card := range cardList(host.Sessions[name].Cards) {
			card["session"] = nullable(name)
			list = append(list, card)
		}
	}
	return list
}

// sayingFirst puts a line in front of what a command prints for a person. The
// list a command answers with shows what is left, not what it did: whoever
// stopped a card would otherwise have to work out from the rest whether the
// right one is gone.
func sayingFirst(line string, s success) success {
	rest := s.text
	s.text = func(w io.Writer) {
		fmt.Fprintln(w, line)
		rest(w)
	}
	return s
}

// letGoList is what the list and a heartbeat say about the cards a host did not
// take: the note as it stands, so that an agent reads the word of the host
// itself and not only what this CLI made of it.
func letGoList(notes []work.LetGoCard) []map[string]any {
	list := make([]map[string]any, 0, len(notes))
	for _, note := range notes {
		list = append(list, map[string]any{
			"id": note.ID, "shortId": nullable(note.ShortID), "title": note.Title,
			"reason": note.Reason, "at": note.At.UTC().Format(time.RFC3339),
		})
	}
	return list
}

// noticesAboutTheCard says what a host would not have of the one card a command
// names, and nothing about the others. A note stands for hours, and a command
// that takes a card up or puts it down would otherwise say the same line about
// somebody else's card every time.
func noticesAboutTheCard(notes []work.LetGoCard, name string) []string {
	var about []work.LetGoCard
	for _, note := range notes {
		if strings.EqualFold(note.ID, name) || (note.ShortID != "" && strings.EqualFold(note.ShortID, name)) {
			about = append(about, note)
		}
	}
	return letGoNotices(about)
}

// letGoNotices says in words what a host would not have, one line per card. The
// card is named here because nothing else names it any more: it is out of the
// list, and a list says what is in it.
func letGoNotices(notes []work.LetGoCard) []string {
	lines := make([]string, 0, len(notes))
	for _, note := range notes {
		name := cardName(note.ShortID, note.Title)
		if name == "" {
			name = note.ID
		}
		if note.Reason == api.ReasonCardArchived {
			lines = append(lines, name+" lies in the archive of its board, so it is out of the list of this session and no board shows it as alive")
			continue
		}
		lines = append(lines, "the host would not take "+name+", and calls it "+note.Reason+
			": the card stays in the list, because minka acts on no word of a host it does not know")
	}
	return lines
}

func cardList(cards []work.Card) []map[string]any {
	list := make([]map[string]any, 0, len(cards))
	for _, card := range cards {
		list = append(list, map[string]any{
			"id": card.ID, "shortId": nullable(card.ShortID), "title": card.Title,
			"startedAt": card.StartedAt.UTC().Format(time.RFC3339),
		})
	}
	return list
}

// otherSessions are the names of every session of the host but the one named,
// sorted.
func otherSessions(host *work.Host, session string) []string {
	names := slices.Sorted(maps.Keys(host.Sessions))
	return slices.DeleteFunc(names, func(name string) bool { return name == session })
}

// namedInTheList finds a card of a list by its ID or by the short identifier it
// was remembered under, so that stopping needs no request.
func namedInTheList(entry *work.List, name string) (work.Card, bool) {
	for _, card := range entry.Cards {
		if strings.EqualFold(card.ID, name) || (card.ShortID != "" && strings.EqualFold(card.ShortID, name)) {
			return card, true
		}
	}
	return work.Card{}, false
}

func namesOf(cards []work.Card) []string {
	names := make([]string, 0, len(cards))
	for _, card := range cards {
		names = append(names, cardName(card.ShortID, card.Title))
	}
	return names
}

// cardName is what a card is called in a line somebody reads: its short
// identifier where a board shows one, and its title either way.
func cardName(shortID, title string) string {
	if shortID != "" {
		return shortID + " " + title
	}
	return title
}

// shortIdentifierOf is the name a board gives a card, empty where it shows
// none.
func shortIdentifierOf(card api.Card) string {
	if card.ShortID == nil {
		return ""
	}
	return *card.ShortID
}

func heartbeatSummary(sent bool, reason string, cards []string) string {
	if sent {
		return fmt.Sprintf("Reported %d cards", len(cards))
	}
	return "Nothing was reported: " + reason
}

// optionalCardArg takes one card or none.
func optionalCardArg(cmd *cobra.Command, args []string) error {
	if len(args) > 1 {
		return usageErrorf(cmd, "unexpected argument %q", args[1])
	}
	if len(args) == 1 {
		return requireID(cmd, "cardId", &args[0])
	}
	return nil
}

func momentOrNil(moment time.Time) any {
	if moment.IsZero() {
		return nil
	}
	return moment.UTC().Format(time.RFC3339)
}

func nullable(text string) any {
	if text == "" {
		return nil
	}
	return text
}

// sessionVariable is where Claude Code names its session to the commands its
// tools run.
const sessionVariable = "CLAUDE_CODE_SESSION_ID"

// claudeCodeVariable is set by Claude Code for every command it runs.
const claudeCodeVariable = "CLAUDECODE"

// agentSession is the session a command speaks for: the one it was told with
// --session, else the one Claude Code names in the environment of the commands
// it runs, which a command typed through its Bash tool inherits. The flag is
// for the mod, which reports from inside a session and is told the name, and
// that name is the one the environment gives the Bash tool. Where neither
// names a session, a command speaks for the list of no session.
func (a *app) agentSession() string {
	if a.sessionFlag != "" {
		return a.sessionFlag
	}
	if named := a.getenv(sessionVariable); work.IsSession(named) {
		return named
	}
	return work.NoSession
}

// checkTheSessionFlag refuses a --session that cannot stand for a session.
func (a *app) checkTheSessionFlag(cmd *cobra.Command) error {
	if a.sessionFlag == "" || work.IsSession(a.sessionFlag) {
		return nil
	}
	return usageErrorf(cmd, "--session takes 1 to 128 letters, digits, \".\", \"-\" and \"_\", not %q", a.sessionFlag)
}

// addSessionFlag gives a command --session, for whatever reports from inside a
// session and is told its name rather than finding it in the environment.
func (a *app) addSessionFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&a.sessionFlag, "session", "", "The session to speak for (default: $"+sessionVariable+")")
}

// clientIDOf is the client a session reports as. The list of no session goes
// by the name this installation was given; a session goes by that name and a
// digest of its own, so that every session has a set of its own on the server,
// no session's name reaches the server, and the same session is the same client
// with every signal. Shortened where needed, it stays within the 64 letters the
// server takes.
func clientIDOf(base, session string) string {
	if session == work.NoSession {
		return base
	}
	digest := sha256.Sum256([]byte(session))
	const room = 64 - 1 - 16
	if len(base) > room {
		base = base[:room]
	}
	return base + "." + hex.EncodeToString(digest[:8])
}
