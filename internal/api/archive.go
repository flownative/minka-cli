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
	// Duplicates counts the cards that came a second time because a page had
	// to start behind an earlier card of the page before it. They are listed
	// once.
	Duplicates int
	// BeganAgain says that no card of a page was left in the archive to carry
	// the cursor, so the walk started over where it began.
	BeganAgain bool
}

// WholeArchiveOf walks the archive of a board to its end, a page of pageSize at
// a time, starting behind the card named in before.
//
// Two things about the address shape the walk. It says the end of the list by
// nothing but a page shorter than asked for, so a full page is always followed
// by one more request. And its cursor holds only while the card it names lies
// in the archive: once somebody takes that card out, the next page is refused,
// or, where the card goes between the server's check and its read, comes back
// empty and reads like the end. The walk therefore keeps the ids of the whole
// page it last read and falls back through them, asks an empty answer after a
// full page a second time, and counts what it swallowed.
func (c *Client) WholeArchiveOf(ctx context.Context, boardID, before string, pageSize int) (ArchiveRun, error) {
	var run ArchiveRun
	seen := map[string]bool{}
	start := before
	cursor := before
	// The ids of the page just read, the last one first: the first is the
	// cursor of the next page, the rest are what it falls back through.
	var candidates []string
	shortPageBehind := false
	emptyAnswerBehind := false

	for {
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
				emptyAnswerBehind = false
				continue
			}
			// Beginning again is the last resort, and it is taken once: a walk
			// that keeps reaching a card it may not read behind would otherwise
			// go round for ever.
			if run.BeganAgain || cursor == start {
				return run, err
			}
			run.BeganAgain = true
			cursor, shortPageBehind, emptyAnswerBehind = start, false, false
			continue
		}

		cards := page.Value
		if len(cards) == 0 {
			// Nothing is read into an empty answer that could as well be the
			// cursor's card going in the moment between the check and the read:
			// the same cursor is asked once more, and only a second empty
			// answer closes the list. Where no cursor was sent, or the page
			// before was already shorter than asked for, there is nothing to
			// doubt.
			if cursor == "" || shortPageBehind || emptyAnswerBehind {
				return run, nil
			}
			emptyAnswerBehind = true
			continue
		}
		emptyAnswerBehind = false
		run.Pages++

		raw, err := eachCardOf(page.Raw, len(cards))
		if err != nil {
			return run, err
		}
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
		candidates = make([]string, 0, len(cards))
		for i := len(cards) - 1; i >= 0; i-- {
			candidates = append(candidates, cards[i].ID)
		}
		cursor, candidates = candidates[0], candidates[1:]
	}
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
