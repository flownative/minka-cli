package basecamp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// message is a message of a message board, which a bundle calls a post.
type message struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// Subject is the title somebody gave; what Basecamp calls the title has the
	// emoji of the category in front of it.
	Subject string `json:"subject"`
	// Content is rendered, as that of a comment is.
	Content string     `json:"content"`
	Files   []bodyFile `json:"content_attachments"`
	// Category is missing on a message without one.
	Category *category `json:"category"`
	Parent   struct {
		ID int64 `json:"id"`
	} `json:"parent"`
	CommentsCount int `json:"comments_count"`
	BoostsCount   int `json:"boosts_count"`
}

// category is a kind of message. The list of a project says when one was made
// and changed; a message shows of its category only what it is called.
type category struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Icon      string `json:"icon"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// extractPosts adds the kinds of post of one project and the posts of its
// message boards, with what hangs on a post: comments and boosts.
//
// Basecamp lists the messages of a board itself, and the archived and the
// trashed ones of a project in one list across all its boards, those that are
// switched off included. Such a message names its board; one of a board that
// is not extracted is left out and counted. A draft is in no list.
//
// The posts of a board are written oldest first and whatever their status,
// the boards in the order of the tools. A walk is no snapshot: a message met
// twice is written once.
func (e *Extractor) extractPosts(ctx context.Context, w *bundle.Writer, bucket int64, boards []int64) error {
	if len(boards) == 0 {
		return nil
	}

	posts := map[int64][]message{}
	extracted := map[int64]bool{}
	placed := map[int64]bool{}
	take := func(found message, board int64) error {
		if placed[found.ID] {
			return nil
		}
		placed[found.ID] = true
		carried, err := e.met("post", found.ID, found.Status, found.recording, "project", sourceID(bucket))
		if err != nil || !carried {
			return err
		}
		posts[board] = append(posts[board], found)
		return nil
	}

	for _, board := range boards {
		extracted[board] = true
		listed, err := getList[message](ctx, e.Source, fmt.Sprintf("buckets/%d/message_boards/%d/messages.json", bucket, board))
		if err != nil {
			return err
		}
		for _, found := range listed {
			if err := take(found, board); err != nil {
				return err
			}
		}
	}
	for _, status := range []string{bundle.StatusArchived, bundle.StatusTrashed} {
		listed, err := getList[message](ctx, e.Source, fmt.Sprintf("projects/recordings.json?type=Message&bucket=%d&status=%s", bucket, status))
		if err != nil {
			return err
		}
		for _, found := range listed {
			if !extracted[found.Parent.ID] {
				e.postsLeftOut++
				continue
			}
			if err := take(found, found.Parent.ID); err != nil {
				return err
			}
		}
	}

	var written []message
	for _, board := range boards {
		if err := oldestFirst(posts[board], "post", func(m message) (int64, string) { return m.ID, m.CreatedAt }); err != nil {
			return err
		}
		written = append(written, posts[board]...)
	}
	if err := e.writeCategories(ctx, w, bucket, written); err != nil {
		return err
	}
	for _, board := range boards {
		for _, found := range posts[board] {
			if err := e.writePost(ctx, w, bucket, sourceID(board), found); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeCategories adds the kinds of post of a project: those its list holds,
// and those only a post shows. Basecamp lists the categories of the first
// message board of a project alone; a further board has its own, which are in
// no list and cannot be asked for, so what a post shows of its category is all
// there is: such a category takes the time of the oldest post that carries it.
// Nobody is named as the creator of a category. One without a name ends the
// run, as it would end the applier's; an icon comes as it is, empty or not.
//
// They are written in the order of their names, whatever the case of a
// letter, and by their IDs where two are called the same; the position is the
// number in that order, because Basecamp gives none.
//
// The list has no pages and answers any page with all of it, so it is read
// once, without a page.
func (e *Extractor) writeCategories(ctx context.Context, w *bundle.Writer, bucket int64, posts []message) error {
	listed, err := getOne[[]category](ctx, e.Source, fmt.Sprintf("buckets/%d/categories.json", bucket))
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	var categories []category
	named := func(c category) error {
		if strings.TrimSpace(c.Name) == "" {
			return &Error{Kind: API, Message: fmt.Sprintf("category %d has no name", c.ID)}
		}
		return nil
	}
	for _, c := range listed {
		if known[c.ID] {
			continue
		}
		if err := requireTimestamps(fmt.Sprintf("category %d", c.ID), c.CreatedAt, c.UpdatedAt); err != nil {
			return err
		}
		if err := named(c); err != nil {
			return err
		}
		known[c.ID] = true
		categories = append(categories, c)
	}

	byAge := append([]message(nil), posts...)
	if err := oldestFirst(byAge, "post", func(m message) (int64, string) { return m.ID, m.CreatedAt }); err != nil {
		return err
	}
	for _, post := range byAge {
		if c := post.Category; c != nil && c.ID != 0 && !known[c.ID] {
			if err := named(*c); err != nil {
				return err
			}
			known[c.ID] = true
			categories = append(categories, category{ID: c.ID, Name: c.Name, Icon: c.Icon, CreatedAt: post.CreatedAt, UpdatedAt: post.CreatedAt})
		}
	}

	sort.SliceStable(categories, func(i, j int) bool {
		a, b := strings.ToLower(categories[i].Name), strings.ToLower(categories[j].Name)
		if a != b {
			return a < b
		}
		return categories[i].ID < categories[j].ID
	})
	for i, c := range categories {
		err := w.Add(bundle.Categories, bundle.Category{
			Record:          bundle.Record{SourceID: sourceID(c.ID), CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt},
			ProjectSourceID: sourceID(bucket),
			Emoji:           c.Icon,
			Name:            c.Name,
			Position:        i + 1,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// pinnedAt is when a post was put at the top of its board. Basecamp tells of
// no message whether it is pinned, let alone since when.
var pinnedAt *string

// writePost adds a post with its comments and boosts. A message without a
// text comes with an empty body, as it is: whether a post may say nothing is
// the applier's to decide, and the run counts them.
func (e *Extractor) writePost(ctx context.Context, w *bundle.Writer, bucket int64, toolID string, found message) error {
	origin, err := e.record("post", found.ID, found.recording)
	if err != nil {
		return err
	}
	mine := owns(bundle.OwnerPost, found.ID)
	body, files, err := e.rewriteBody(mine, found.Content, "", found.Files)
	if err != nil {
		return err
	}

	record := bundle.Post{
		Record:           origin,
		ProjectSourceID:  sourceID(bucket),
		ToolSourceID:     toolID,
		Title:            found.Subject,
		Body:             body,
		PinnedAt:         pinnedAt,
		Status:           found.Status,
		VisibleToClients: visibleToClientsOf(found.recording),
	}
	if record.Subscribers, err = e.subscribersOf(ctx, "post", bucket, found.ID, found.Status); err != nil {
		return err
	}
	if found.Category != nil && found.Category.ID != 0 {
		id := sourceID(found.Category.ID)
		record.CategorySourceID = &id
	}
	if err := w.Add(bundle.Posts, record); err != nil {
		return err
	}
	e.measure(bundle.Posts, body, LongestBodyRecord{Record: "post", SourceID: origin.SourceID, In: "project", InSourceID: sourceID(bucket)})
	e.postsByStatus[found.Status]++
	if saysNothing(body) {
		e.postsWithEmptyBody++
	}
	if err := e.attachFiles(ctx, w, mine, origin, files); err != nil {
		return err
	}

	return e.writeDiscussion(ctx, w, bucket, bundle.ParentPost, bundle.TargetPost, found.ID, found.CommentsCount, found.BoostsCount)
}
