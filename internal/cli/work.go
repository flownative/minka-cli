package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/work"
	"github.com/spf13/cobra"
)

const (
	// signalEvery is how often the cards of this machine are reported. The
	// server counts a signal for three minutes, so two may be lost before a
	// tile goes quiet.
	signalEvery = time.Minute

	// signalTimeout bounds one signal. It runs after every tool call of an
	// agent, so a host that does not answer may hold the work up for a moment
	// and no longer.
	signalTimeout = 5 * time.Second

	// waitForTheStore is how long a command a person or an agent typed waits
	// for the lock. The heartbeat waits not at all: where somebody else is at
	// the store, there will be another signal in a minute. A command that
	// reports on its own behalf waits like the command it is part of, because
	// for it there is no next time.
	waitForTheStore = 2 * time.Second
)

func (a *app) newWorkCmd() *cobra.Command {
	return group("work", "Say which cards you are working on, so the board shows them as alive",
		a.newWorkStartCmd(),
		a.newWorkStopCmd(),
		a.newWorkListCmd(),
	)
}

func (a *app) newWorkStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start <card>",
		Short: "Remember that you are working on a card",
		Long: `Remembers a card as one you are working on.

The board shows such a card as alive while minka keeps saying so. Saying so is
the job of 'minka heartbeat', which the hook of 'minka hooks install' runs
after every tool call: until one of them has run, 'work start' has changed
nothing but this machine's own list.

The list belongs to the machine, not to this session: every session on it
reports the same list together, and 'work stop' takes a card out again. So do
'cards move' and 'cards done': a card that is moved on is one nobody is at
work on any more, so take a card up in that order — move it first, then say
'work start'.`,
		Example: `  minka work start MNK-1A
  minka work start 0199... --json`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			card, err := s.findCard(a.ctx, args[0])
			if err != nil {
				return err
			}

			handle, err := a.openStore(waitForTheStore)
			if err != nil {
				return err
			}
			defer handle.Close()

			entry := handle.File.Host(s.host)
			forgotten := entry.ForgetWhatRanOut(a.now())
			entry.Start(work.Card{
				ID:        card.ID,
				ShortID:   shortIdentifierOf(card),
				Title:     card.Title,
				StartedAt: a.now(),
				Session:   a.agentSession(),
			})
			if err := handle.Save(); err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}

			return a.ok(a.workSuccess(entry, "Working on "+cardName(shortIdentifierOf(card), card.Title), nil, forgotten))
		},
	}
}

func (a *app) newWorkStopCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "stop [<card>]",
		Short: "Forget that you are working on a card",
		Long: `Takes a card out of the list this machine reports.

The next signal leaves it out, and the board stops showing it as alive at
once rather than after the three minutes a signal counts for.

Without a card it stops the one card that is remembered. Where several are,
it says which and changes nothing, because the list is shared with every other
session on this machine: name the card, or take them all with --all.

It asks nothing of the network: a card is found in the list under either of
the names it was remembered with, and one that is not remembered is nothing to
stop. It needs the host all the same, so a machine that is signed in nowhere
has nothing to answer about.`,
		Example: `  minka work stop MNK-1A
  minka work stop --all`,
		Args: optionalCardArg,
		RunE: func(cmd *cobra.Command, args []string) error {
			host, err := a.hostOfTheStore()
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
			entry, known := handle.File.Hosts[host]
			if !known {
				return a.ok(sayingFirst("Nothing to stop", a.workSuccess(&work.Host{}, "Nothing to stop", map[string]any{"stopped": cardList(nil)}, 0)))
			}
			forgotten := entry.ForgetWhatRanOut(a.now())

			var stopped []work.Card
			switch {
			case all:
				stopped = entry.Cards
				entry.Cards = nil
			case len(args) == 1:
				if card, found := namedInTheStore(entry, args[0]); found {
					entry.Stop(card.ID)
					stopped = []work.Card{card}
				}
			case len(entry.Cards) == 1:
				stopped = entry.Cards
				entry.Cards = nil
			case len(entry.Cards) > 1:
				return &Error{
					Code:    CodeUsage,
					Message: "this machine is working on " + strings.Join(namesOf(entry.Cards), ", ") + ", and another session may have put one of them there",
					Hint:    "Name the card to stop, or take them all with --all.",
				}
			}

			if len(stopped) > 0 || forgotten > 0 {
				if err := handle.Save(); err != nil {
					return &Error{Code: CodeConfig, Message: err.Error()}
				}
			}

			summary := "Nothing to stop"
			if len(stopped) > 0 {
				summary = "Stopped " + strings.Join(namesOf(stopped), ", ")
			}
			return a.ok(sayingFirst(summary, a.workSuccess(entry, summary, map[string]any{"stopped": cardList(stopped)}, forgotten)))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Stop every card this machine is working on")
	return cmd
}

func (a *app) newWorkListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the cards this machine is working on",
		Long: `Shows what this machine reports as being worked on.

The list is the one every session on this machine shares, and it is read from
disk alone: the server cannot be asked what it holds.

Going out and arriving are two moments. 'sentAt' says when a signal last went
out, 'reportedAt' when one last arrived, and 'failure' what the host made of
the last one that did not. A signal that never got as far as a request —
nobody signed in, the store held by somebody else — leaves nothing behind;
'minka heartbeat --json' says what it made of the moment.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			host, err := a.hostOfTheStore()
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
			entry, known := handle.File.Hosts[host]
			if !known {
				entry = &work.Host{}
			}
			forgotten := entry.ForgetWhatRanOut(a.now())
			summary := fmt.Sprintf("%d cards", len(entry.Cards))
			if len(entry.Cards) == 1 {
				summary = "1 card"
			}
			return a.ok(a.workSuccess(entry, summary+" on "+host, nil, forgotten))
		},
	}
}

func (a *app) newHeartbeatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "heartbeat",
		Short: "Report the cards this machine is working on",
		Long: `Reports the cards of 'minka work start' to Minka, so their tiles stay alive.

It is made to run after every tool call of an agent, which 'minka hooks
install' sets up. With nothing remembered it returns at once and asks nothing
of the network; otherwise it reports at most once a minute, and again right
away whenever the list has changed.

It always exits 0 and says nothing unless asked with --json, --quiet or on a
terminal: a hook that fails loudly after every tool call would cost the agent
its attention for something it cannot act on. Asked, it says in 'reason' what
it made of the moment, with the status and the code of the API where a request
was turned away; what the last signal that went out made of it also stands in
'minka work list'.`,
		Args: requireArgs(),
		RunE: func(*cobra.Command, []string) error {
			sent, reason, cards := a.signal(0)
			if cards == nil {
				cards = []string{}
			}

			if !a.jsonFlag && !a.quietFlag && !a.styledFlag && !a.stdoutIsTerminal() {
				return nil
			}
			return a.ok(success{
				data:    map[string]any{"sent": sent, "reason": reason, "cardIds": cards},
				summary: heartbeatSummary(sent, reason, cards),
				text:    func(w io.Writer) { fmt.Fprintln(w, heartbeatSummary(sent, reason, cards)) },
			})
		},
	}
}

// signal reports this machine's cards and says what it did.
//
// Nothing it meets is a failure of the command: not being signed in, a host
// that does not answer, a token a person owns. Whoever reads a hook's exit
// code is in the middle of other work, and none of these is something they
// could do anything about from there.
func (a *app) signal(wait time.Duration) (sent bool, reason string, cards []string) {
	path, err := a.storePath()
	if err != nil || work.Untouched(path) {
		return false, "nothing is remembered on this machine", nil
	}
	s, err := a.session()
	if err != nil {
		return false, "not signed in", nil
	}

	claimed, reason := a.claimTheSignal(path, s.host, wait)
	if claimed == nil {
		return false, reason, nil
	}

	ctx, cancel := context.WithTimeout(a.ctx, signalTimeout)
	defer cancel()
	err = s.client.NoteAtWork(ctx, claimed.clientID, claimed.cards)
	a.noteTheOutcome(path, s.host, err)
	if err != nil {
		// The failure as the list keeps it, so that the one word an agent
		// reads here names the case: the status and the code of the API are
		// what tells a revoked token from a person's one.
		return false, err.Error(), claimed.cards
	}
	return true, "", claimed.cards
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
func (a *app) claimTheSignal(path, host string, wait time.Duration) (*claim, string) {
	handle, err := work.Open(path, wait)
	if err != nil {
		return nil, "the store is busy or unreadable"
	}
	defer handle.Close()

	entry, known := handle.File.Hosts[host]
	if !known {
		return nil, "nothing is remembered for " + host
	}
	// The session says it is still at its cards before anything else, because
	// that holds whether or not a signal goes out from here: the throttle is
	// about the host's ear, and this is about the work.
	changed := entry.ForgetWhatRanOut(a.now()) > 0
	if entry.StillAtIt(a.agentSession(), a.now()) {
		changed = true
	}
	cards := entry.CardIDs()

	// Nothing to say, and nothing said before that would have to be taken
	// back: this is where the common case ends, before the network.
	if len(cards) == 0 && len(entry.SentCards) == 0 {
		return nil, a.saving(handle, changed, "nothing is remembered for "+host)
	}
	if slices.Equal(cards, entry.SentCards) && a.now().Sub(entry.SentAt) < signalEvery {
		return nil, a.saving(handle, changed, "the last signal is younger than a minute")
	}

	clientID, err := a.clientID()
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
func (a *app) noteTheOutcome(path, host string, failed error) {
	handle, err := work.Open(path, waitForTheStore)
	if err != nil {
		return
	}
	defer handle.Close()

	entry := handle.File.Host(host)
	if failed != nil {
		entry.Failure = failed.Error()
		entry.FailedAt = a.now()
	} else {
		entry.ReportedAt = a.now()
		entry.Failure = ""
		entry.FailedAt = time.Time{}
	}
	_ = handle.Save()
}

// noLongerInHand is what a command says when it took the card out of the list
// of this machine on its own. The list is shared with every other session on
// the machine, so nothing changes it without a word, and the word says how far
// the change has got.
func noLongerInHand(told bool) string {
	if told {
		return "the card is no longer among the cards this machine is working on, and the board was told at once"
	}
	return "the card is no longer among the cards this machine is working on; the board hears of it with the next signal"
}

// letGoOfTheCard takes a card out of the list of this machine and reports the
// list at once, so that a card somebody has moved on stops glowing instead of
// waiting for the signal after the next tool call. It says whether the card was
// in the list at all, and whether the report got through.
//
// Whatever it meets on the way it passes over in silence: the card has been
// moved, and neither a store another session holds at that moment nor a signal
// that does not get through makes that untrue.
func (a *app) letGoOfTheCard(host, cardID string) (letGo, told bool) {
	if !a.takeOutOfTheList(host, cardID) {
		return false, false
	}
	sent, _, _ := a.signal(waitForTheStore)
	return true, sent
}

// takeOutOfTheList is the half that holds the lock, kept apart so that it is
// given back before the signal, which takes it for itself.
func (a *app) takeOutOfTheList(host, cardID string) bool {
	handle, err := a.openStore(waitForTheStore)
	if err != nil {
		return false
	}
	defer handle.Close()

	entry, known := handle.File.Hosts[host]
	if !known {
		return false
	}
	// Whoever writes the store sweeps it, so that what ran out is gone for the
	// next reader as well; the card of this command is a different question.
	swept := entry.ForgetWhatRanOut(a.now()) > 0
	gone := entry.Stop(cardID)
	if !gone && !swept {
		return false
	}
	return handle.Save() == nil && gone
}

// clientID is the name this installation of minka goes by. It is made once and
// kept in the configuration; every caller holds the lock on the store, so two
// signals at the same moment cannot make two of them.
func (a *app) clientID() (string, error) {
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
	_, file, err := a.loadConfig()
	if err != nil {
		return "", err
	}
	if file.DefaultHost == "" {
		return "", &Error{Code: CodeAuth, Message: "not signed in to any Minka host", Hint: "Run: minka auth login --host https://minka.example.com"}
	}
	return file.DefaultHost, nil
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
// The subject here is the list of this machine and not a resource of the API,
// so it is put together here; a command that has more to say adds it.
func (a *app) workSuccess(entry *work.Host, summary string, more map[string]any, forgotten int) success {
	data := map[string]any{
		"cards":      cardList(entry.Cards),
		"sentAt":     momentOrNil(entry.SentAt),
		"reportedAt": momentOrNil(entry.ReportedAt),
		"failure":    nullable(entry.Failure),
	}
	for name, value := range more {
		data[name] = value
	}

	// Whether anything reports at all is a different question from what the
	// host makes of it. Where a signal went out and was turned away, advising
	// the hook would send somebody after a problem they do not have.
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
		{Action: "hooks", Cmd: "minka hooks status --json", Description: "See whether a hook reports it after every tool call"},
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
	}
	return s
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

// namedInTheStore finds a card of the list by its ID or by the short
// identifier it was remembered under, so that stopping needs no request.
func namedInTheStore(entry *work.Host, name string) (work.Card, bool) {
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
		return requireID(cmd, "cardId", args[0])
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

// session_ is what a session of an agent calls itself, where it says so in the
// environment. It binds a card to whoever took it up: the list belongs to the
// machine and a signal carries all of it, so a sign of life without a name
// would make one session's work keep every other session's cards alive.
//
// A card taken up where nothing says who is asking — a person at a terminal —
// carries no name and goes by the moment it was started.
func (a *app) agentSession() string {
	return a.getenv("CLAUDE_CODE_SESSION_ID")
}
