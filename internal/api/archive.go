package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// ArchivePageDefault is the page the archive of a board gives without a limit,
// ArchivePageMax the largest page it serves.
//
// A limit above the maximum is not refused: the server cuts it down without a
// word. And the only sign the answer carries for the end of the list is fewer
// cards than were asked for. Whoever asks for more than the maximum therefore
// reads a full page as a short one and takes a hundred cards for the whole
// archive. Nothing here ever asks for more, so that answer cannot arise.
const (
	ArchivePageDefault = 50
	ArchivePageMax     = 100
)

// CodeArchivedCardsBeforeUnknown is what the server answers for a page that is
// to start behind a card which does not lie in the archive of that board. In a
// walk it is no end state: the card the cursor named left the archive while the
// walk was going through it.
const CodeArchivedCardsBeforeUnknown = "archivedCardsBeforeUnknown"

// ErrArchivePageOutOfBounds marks a page size this client will not ask for. It
// is a mistake of the caller and no answer of a server, which is why it is not
// an *Error.
var ErrArchivePageOutOfBounds = errors.New("a page of an archive holds between 1 and " + strconv.Itoa(ArchivePageMax) + " cards")

// ArchivedCards reads one page of the archive of a board, the card put away
// last first. A page after the first starts behind the card named in before.
//
// The limit is always sent and always within the bounds: the server's silent
// cut is unreachable from here.
func (c *Client) ArchivedCards(ctx context.Context, boardID, before string, limit int) (Result[[]ArchivedCard], error) {
	var result Result[[]ArchivedCard]
	if limit < 1 || limit > ArchivePageMax {
		return result, ErrArchivePageOutOfBounds
	}

	values := url.Values{}
	if before != "" {
		values.Set("before", before)
	}
	values.Set("limit", strconv.Itoa(limit))
	return request[[]ArchivedCard](ctx, c, http.MethodGet, "/boards/"+url.PathEscape(boardID)+"/archive?"+values.Encode(), nil)
}

// ArchiveRun is what one walk through the archive of a board brought back.
//
// Every card that lay in the archive for the whole walk is in it exactly once.
// It is no picture of one moment: a card put away while the walk was going on
// lands on the first page, which the walk has passed, and a card taken out of
// the archive leaves it without the API saying anything about either.
type ArchiveRun struct {
	Cards []ArchivedCard
	// Raw is the JSON of each card as the server sent it, in the order of
	// Cards, so that a field the API gains reaches a reader across several
	// pages as well.
	Raw []json.RawMessage
	// Pages counts the answers that carried cards.
	Pages int
	// Duplicates counts the cards a walk read a second time because a cursor did
	// not hold: the page that carried it on, and the page it began again with,
	// both lie over ground it had covered. They are listed once.
	Duplicates int
	// BeganAgain says that no card of a page was left in the archive to carry
	// the cursor, so the walk started over where it began.
	BeganAgain bool
	// TookTheEndOnTrust says that no card of the last page was left to ask
	// behind, so nothing but an answer that carried nothing stood for the end of
	// the list. A card going in a race reads exactly the same, and a card behind
	// it would have been missed, so this is the one walk that does not promise
	// every card which lay in the archive throughout.
	TookTheEndOnTrust bool
}

// WholeArchiveOf walks the archive of a board to its end, a page of pageSize at
// a time, starting behind the card named in before.
//
// Two things about the address shape the walk. It says the end of the list by
// nothing but a page shorter than asked for, which is why no page of cards ends
// the walk and one request more is always owed. And its cursor holds only while
// the card it names lies in the archive: once somebody takes that card out, the
// next page is refused, or, where the card goes between the server's check and
// its read, comes back empty and reads like the end. The walk therefore keeps
// the ids of the whole page it last read and falls back through them, meets an
// empty answer behind a full page with the page behind an earlier card of that
// page, which settles either way whether the list has ended, and counts what it
// swallowed.
//
// Nothing of the walk rests on the answers being what the address promises: an
// answer that does not carry it past a cursor it has already asked with ends it
// rather than sending the same request again for ever. And where no card of the
// last page is left to ask behind, there is nothing to check the end against, so
// the walk says of that end that it took it on trust.
func (c *Client) WholeArchiveOf(ctx context.Context, boardID, before string, pageSize int) (ArchiveRun, error) {
	var run ArchiveRun
	seen := map[string]bool{}
	start := before
	cursor := before
	// The ids of the page just read, the last one first: the first is the
	// cursor of the next page, the rest are what it falls back through.
	var candidates []string
	// The cursors asked with since the walk last began, which bound it.
	asked := map[string]bool{}
	shortPageBehind := false
	emptyAnswerBehind := false
	// Set while a page is asked for to tell the end of the list from a cursor
	// that has gone: an answer that brings no card of its own says the list had
	// ended where the empty answer before it said.
	settlingTheEnd := false

	for {
		asked[cursor] = true
		page, err := c.ArchivedCards(ctx, boardID, cursor, pageSize)
		if err != nil {
			if !isBeforeUnknown(err) {
				return run, err
			}
			// The card the cursor named has left the archive. Another card of
			// the same page carries on from where it stood, so that a busy
			// archive does not send the walk back to the first page; only when
			// none of them is left does it start over, and it says so.
			if len(candidates) > 0 {
				cursor, candidates = candidates[0], candidates[1:]
				continue
			}
			// Beginning again is the last resort, and it is taken once: a walk
			// that keeps reaching a card it may not read behind would otherwise
			// go round for ever.
			if run.BeganAgain || cursor == start {
				return run, err
			}
			run.BeganAgain = true
			cursor, candidates, shortPageBehind, emptyAnswerBehind, settlingTheEnd = start, nil, false, false, false
			clear(asked)
			continue
		}

		cards := page.Value
		if len(cards) == 0 {
			// Nothing is read into an empty answer that could as well be the
			// cursor's card going in the moment between the check and the read.
			// The page behind an earlier card of the last page settles it: where
			// the list has really ended, it brings that page's last card and
			// nothing beyond it. Where no cursor was sent, or the page before
			// was already shorter than asked for, there is nothing to doubt.
			if cursor == "" || shortPageBehind {
				return run, nil
			}
			if len(candidates) > 0 {
				settlingTheEnd = true
				cursor, candidates = candidates[0], candidates[1:]
				continue
			}
			// Where no card of the last page is left to ask behind, the weakest
			// probe there is has to do: the same cursor once more, and a second
			// empty answer closes the list. It closes it unchecked, and the walk
			// carries that back rather than passing it off as the end.
			if emptyAnswerBehind {
				run.TookTheEndOnTrust = true
				return run, nil
			}
			emptyAnswerBehind = true
			continue
		}
		emptyAnswerBehind = false

		raw, err := eachCardOf(page.Raw, len(cards))
		if err != nil {
			return run, err
		}
		if settlingTheEnd {
			// Only a page shorter than asked for reaches the end of the list, and
			// only one that brings nothing new says the empty answer before it was
			// that end. A full page proves nothing however well known its cards
			// are: a walk that began again has read far beyond the page it checks
			// against, and the list goes on behind it.
			if len(cards) < pageSize && !holdsACardNotYetRead(cards, seen) {
				return run, nil
			}
			settlingTheEnd = false
		}
		run.Pages++
		for i, card := range cards {
			if seen[card.ID] {
				run.Duplicates++
				continue
			}
			seen[card.ID] = true
			run.Cards = append(run.Cards, card)
			run.Raw = append(run.Raw, raw[i])
		}

		shortPageBehind = len(cards) < pageSize
		if next := cards[len(cards)-1].ID; asked[next] {
			return run, &Error{
				Method:     http.MethodGet,
				StatusCode: http.StatusOK,
				Code:       "unexpectedResponse",
				Message:    "The archive of this board answered a page that ends on the card an earlier page was asked behind, so reading on would ask the same question for ever.",
			}
		}
		candidates = make([]string, 0, len(cards))
		for i := len(cards) - 1; i >= 0; i-- {
			candidates = append(candidates, cards[i].ID)
		}
		cursor, candidates = candidates[0], candidates[1:]
	}
}

// holdsACardNotYetRead says whether a page brings anything a walk has not read.
// It tells a page of the archive from an answer that only confirms where the
// list ends.
func holdsACardNotYetRead(cards []ArchivedCard, seen map[string]bool) bool {
	for _, card := range cards {
		if !seen[card.ID] {
			return true
		}
	}
	return false
}

// isBeforeUnknown says whether an answer refused the card a page was to start
// behind. Only that refusal sends the walk to another cursor; every other one
// ends it.
func isBeforeUnknown(err error) bool {
	var apiErr *Error
	return errors.As(err, &apiErr) && apiErr.Code == CodeArchivedCardsBeforeUnknown
}

// eachCardOf cuts the JSON of a page into the JSON of its cards, which is what
// a reader of several pages is handed instead of one answer of the server.
func eachCardOf(raw json.RawMessage, cards int) ([]json.RawMessage, error) {
	var each []json.RawMessage
	if err := json.Unmarshal(raw, &each); err != nil || len(each) != cards {
		return nil, &Error{
			Method:     http.MethodGet,
			StatusCode: http.StatusOK,
			Code:       "unexpectedResponse",
			Message:    "The server answered a page of the archive with something that is not a list of cards.",
		}
	}
	return each, nil
}
