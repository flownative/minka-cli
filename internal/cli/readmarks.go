package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/readmark"
)

// ifLatestNone is what --if-latest takes for a card without comments.
const ifLatestNone = "none"

func (a *app) readMarkPath() (string, error) {
	path, err := config.Path(a.getenv)
	if err != nil {
		return "", err
	}
	return readmark.Path(path), nil
}

// knownComments are the comments of a card this machine has shown to the
// reader of the session. Marks that cannot be read know nothing, which shows
// every comment as unread rather than hiding one; the notice says why.
func (a *app) knownComments(s *session, cardID string) (map[string]bool, []string) {
	path, err := a.readMarkPath()
	if err == nil {
		var known map[string]bool
		if known, err = readmark.Known(path, s.host, s.reader, cardID); err == nil {
			return known, nil
		}
	}
	return map[string]bool{}, []string{"which comments were shown here cannot be read, so every comment counts as unread: " + err.Error()}
}

// rememberComments notes comments of a card as shown or written here. A mark
// that cannot be written fails nothing: what was read was read, and the cost is
// that the comments are shown as unread again, which the notice says.
func (a *app) rememberComments(s *session, cardID string, ids []string) []string {
	if len(ids) == 0 {
		return nil
	}
	path, err := a.readMarkPath()
	if err == nil {
		ctx, cancel := context.WithTimeout(a.ctx, a.markWait)
		err = readmark.Remember(ctx, path, s.host, s.reader, cardID, ids, a.now())
		cancel()
	}
	if err != nil {
		return []string{"which comments were shown here could not be noted, so they count as unread again: " + err.Error()}
	}
	return nil
}

// rememberShown notes every comment of a card as shown.
func (a *app) rememberShown(s *session, card api.Card) []string {
	return a.rememberComments(s, card.ID, commentIDs(card.Comments))
}

// requireRead says whether a comment needs the comments of its card read
// first. Without a configuration file nothing was ever switched on. A session
// whose token comes from the environment does not depend on the file, so a
// file it cannot read leaves the setting off, with a notice, rather than
// stopping a comment that worked without it.
func (a *app) requireRead(s *session) (bool, []string, error) {
	_, file, err := a.loadConfig()
	if err != nil {
		if s.tokenSource == tokenFromEnvironment {
			return false, []string{"comments.requireRead is taken as off, because the configuration file cannot be read: " + err.Error()}, nil
		}
		return false, nil, err
	}
	return file.Comments.RequireRead, nil, nil
}

func commentIDs(comments []api.Comment) []string {
	ids := make([]string, len(comments))
	for i, comment := range comments {
		ids[i] = comment.ID
	}
	return ids
}

// newestComment is the ID of the last comment as Minka orders them, oldest
// first by the moment they were written; none where there is no comment.
func newestComment(comments []api.Comment) string {
	if len(comments) == 0 {
		return ifLatestNone
	}
	return comments[len(comments)-1].ID
}

// commentsOfCard picks the comments out of the card as it was served, so that
// they are handed over with every field the API sends.
func commentsOfCard(raw json.RawMessage) ([]json.RawMessage, error) {
	var card struct {
		Comments []json.RawMessage `json:"comments"`
	}
	if err := json.Unmarshal(raw, &card); err != nil {
		return nil, fmt.Errorf("reading the comments of the card: %w", err)
	}
	if card.Comments == nil {
		card.Comments = []json.RawMessage{}
	}
	return card.Comments, nil
}

// someOf keeps the served comments whose ID the filter takes, in their order.
func someOf(card api.Card, raw []json.RawMessage, keep func(api.Comment) bool) ([]api.Comment, []json.RawMessage) {
	comments := []api.Comment{}
	served := []json.RawMessage{}
	for i, comment := range card.Comments {
		if keep(comment) && i < len(raw) {
			comments = append(comments, comment)
			served = append(served, raw[i])
		}
	}
	return comments, served
}

// commentBreadcrumbs are those of a card whose comments were just read, with
// the comment that answers them guarded by the newest of them.
func commentBreadcrumbs(card api.Card) []Breadcrumb {
	crumbs := cardBreadcrumbs(card.ID)
	for i := range crumbs {
		if crumbs[i].Action == "comment" {
			crumbs[i].Cmd += " --if-latest " + newestComment(card.Comments)
			crumbs[i].Description = "Comment on the card, unless somebody commented since"
		}
	}
	return crumbs
}

func renderCommentsOrNone(w io.Writer, comments []api.Comment, none string) {
	if len(comments) == 0 {
		fmt.Fprintln(w, none)
		return
	}
	renderEntryComments(w, comments)
}
