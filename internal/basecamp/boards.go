package basecamp

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/flownative/minka-cli/internal/bundle"
)

// toolDetails is what a tool says about itself. Only a card table has lists.
type toolDetails struct {
	recording
	Lists []cardList `json:"lists"`
}

type cardList struct {
	recording
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
	// Position is missing on the columns every card table has.
	Position *int `json:"position"`
	// OnHold is the part of a column its cards can be set aside in. It has an
	// ID and a list of cards of its own, and is no column.
	OnHold *struct {
		ID int64 `json:"id"`
	} `json:"on_hold"`
}

type card struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
	// Content is the note as it is stored, Description the same note as
	// Basecamp renders it.
	Content     string `json:"content"`
	Description string `json:"description"`
	// Position is missing on a card that is completed, archived or trashed.
	Position *int `json:"position"`
	Parent   struct {
		ID int64 `json:"id"`
	} `json:"parent"`
	DueOn         *string     `json:"due_on"`
	Assignees     []person    `json:"assignees"`
	Completion    *completion `json:"completion"`
	CommentsCount int         `json:"comments_count"`
	BoostsCount   int         `json:"boosts_count"`
	SubtasksCount int         `json:"subtasks_count"`
	SubtasksURL   string      `json:"subtasks_url"`
	Steps         []step      `json:"steps"`
	Files         []bodyFile  `json:"description_attachments"`
}

type completion struct {
	CreatedAt string  `json:"created_at"`
	Creator   *person `json:"creator"`
}

type step struct {
	recording
	ID         int64       `json:"id"`
	Title      string      `json:"title"`
	Position   int         `json:"position"`
	Completion *completion `json:"completion"`
}

type comment struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	// Content is rendered; Basecamp hands out no stored form of a comment.
	Content     string     `json:"content"`
	Files       []bodyFile `json:"content_attachments"`
	BoostsCount int        `json:"boosts_count"`
}

type boost struct {
	ID        int64   `json:"id"`
	Content   string  `json:"content"`
	CreatedAt string  `json:"created_at"`
	Booster   *person `json:"booster"`
}

// columnRoles maps the kinds of list a card table has.
var columnRoles = map[string]string{
	"Kanban::Triage":       bundle.RoleTriage,
	"Kanban::NotNowColumn": bundle.RoleNotNow,
	"Kanban::Column":       bundle.RoleWork,
	"Kanban::DoneColumn":   bundle.RoleDone,
}

// roleOrder is the order the columns of a board are written in.
var roleOrder = map[string]int{bundle.RoleTriage: 0, bundle.RoleNotNow: 1, bundle.RoleWork: 2, bundle.RoleDone: 3}

var recordStatuses = map[string]bool{bundle.StatusActive: true, bundle.StatusArchived: true, bundle.StatusTrashed: true}

// board is a card table that is extracted as a tool.
type board struct {
	id      int64
	lists   []cardList
	columns []*column
}

// column collects the cards of one column in the order of their positions.
type column struct {
	id    int64
	cards []placedCard
}

type placedCard struct {
	card
	columnID int64
	position int
	onHold   bool
}

// place puts a card at the bottom of the column. A position counts from 1
// within the column and leaves no gap, whatever the status of a card: the
// applier orders the column by it. So a card does not keep the number of its
// source but its rank: the cards of the column by their position there, then
// the ones set aside, then the archived and the trashed, each group in the
// order Basecamp lists it in.
func (c *column) place(found card, onHold bool) {
	c.cards = append(c.cards, placedCard{card: found, columnID: c.id, position: len(c.cards) + 1, onHold: onHold})
}

// byPosition brings the cards of a list into the order of their positions.
// Cards without one, as in Done, follow in the order Basecamp lists them in.
func byPosition(cards []card) {
	sort.SliceStable(cards, func(i, j int) bool {
		a, b := cards[i].Position, cards[j].Position
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a < *b
	})
}

// fileOrder is the order the cards of a board are written in: oldest first,
// across all columns and whatever their status. The applier hands out the
// short identifiers of a board in this order, so they follow the order the
// cards were created in.
func fileOrder(cards []placedCard) error {
	return oldestFirst(cards, "card", func(c placedCard) (int64, string) { return c.ID, c.CreatedAt })
}

// oldestFirst orders records by when they were created, and by their IDs where
// two were created in the same moment. A time is read for ordering only: a
// bundle carries it the way Basecamp wrote it.
func oldestFirst[T any](items []T, what string, key func(T) (id int64, createdAt string)) error {
	_, err := oldestFirstWithMoments(items, what, key)
	return err
}

// oldestFirstWithMoments orders like oldestFirst and hands out the moments it
// read, by ID, for whoever goes on to compare them.
func oldestFirstWithMoments[T any](items []T, what string, key func(T) (id int64, createdAt string)) (map[int64]time.Time, error) {
	created := make(map[int64]time.Time, len(items))
	for _, item := range items {
		id, createdAt := key(item)
		parsed, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp says that %s %d was created at something that is no time", what, id)}
		}
		created[id] = parsed
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, _ := key(items[i])
		b, _ := key(items[j])
		if !created[a].Equal(created[b]) {
			return created[a].Before(created[b])
		}
		return a < b
	})
	return created, nil
}

// extractBoards adds the columns and cards of the boards of one project, and
// what hangs on a card: steps, comments and boosts.
//
// Basecamp lists the archived and the trashed cards of a project in one list
// across all its card tables, those that are switched off included, and a card
// names no table. So the lists of all boards are known before the first card
// is written, and a card whose list belongs to none of them is left out: a
// card of a board that is not extracted, or of a column that is gone.
//
// A walk is no snapshot: a card that is moved between two requests can be
// missed, or met twice, and then it is written once.
func (e *Extractor) extractBoards(ctx context.Context, w *bundle.Writer, bucket int64, boards []*board) error {
	if len(boards) == 0 {
		return nil
	}
	projectID := sourceID(bucket)

	type place struct {
		column *column
		onHold bool
	}
	places := map[int64]place{}
	placed := map[int64]bool{}
	take := func(found card, at place) error {
		if placed[found.ID] {
			return nil
		}
		placed[found.ID] = true
		carried, err := e.met("card", found.ID, found.Status, found.recording, "project", projectID)
		if err != nil || !carried {
			return err
		}
		at.column.place(found, at.onHold)
		return nil
	}

	for _, b := range boards {
		if err := e.writeColumns(w, projectID, b); err != nil {
			return err
		}
		for i, list := range b.lists {
			places[list.ID] = place{column: b.columns[i]}
			if list.OnHold != nil {
				places[list.OnHold.ID] = place{column: b.columns[i], onHold: true}
			}
		}
	}

	// A card says itself where it is. One that a list holds and that names
	// another place comes with that place, or after all of them.
	var elsewhere []card
	for _, b := range boards {
		for _, list := range b.lists {
			ids := []int64{list.ID}
			if list.OnHold != nil {
				ids = append(ids, list.OnHold.ID)
			}
			for _, id := range ids {
				cards, err := getList[card](ctx, e.Source, fmt.Sprintf("buckets/%d/card_tables/lists/%d/cards.json", bucket, id))
				if err != nil {
					return err
				}
				byPosition(cards)
				for _, found := range cards {
					if at, known := places[found.Parent.ID]; known && at != places[id] {
						elsewhere = append(elsewhere, found)
						continue
					}
					if err := take(found, places[id]); err != nil {
						return err
					}
				}
			}
		}
	}
	for _, found := range elsewhere {
		if err := take(found, places[found.Parent.ID]); err != nil {
			return err
		}
	}

	for _, status := range []string{bundle.StatusArchived, bundle.StatusTrashed} {
		cards, err := getList[card](ctx, e.Source, fmt.Sprintf("projects/recordings.json?type=Kanban::Card&bucket=%d&status=%s", bucket, status))
		if err != nil {
			return err
		}
		for _, found := range cards {
			at, extracted := places[found.Parent.ID]
			if !extracted {
				e.cardsLeftOut++
				continue
			}
			if err := take(found, at); err != nil {
				return err
			}
		}
	}

	for _, b := range boards {
		var cards []placedCard
		for _, c := range b.columns {
			cards = append(cards, c.cards...)
		}
		if err := fileOrder(cards); err != nil {
			return err
		}
		for _, found := range cards {
			if err := e.writeCard(ctx, w, bucket, sourceID(b.id), found); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeColumns adds the columns of a board: Triage, Not now, the columns
// somebody made in the order of their positions, Done. The position of a
// column is its number in that order, because Basecamp gives the three columns
// every card table has none.
//
// A kind of list a bundle has no role for is left out and counted, and the board
// comes with the columns that are left; it takes the cards standing in it with
// it, which are never asked for. A second Triage, Not now or Done ends the run
// instead: the applier stops at one, and putting the cards of the second
// somewhere else would be a board nobody had.
func (e *Extractor) writeColumns(w *bundle.Writer, projectID string, b *board) error {
	roles := map[string]int{}
	carried := make([]cardList, 0, len(b.lists))
	for _, list := range b.lists {
		role, known := columnRoles[list.Type]
		if !known {
			e.leaveOut("column", "type", list.Type, "board", sourceID(b.id))
			continue
		}
		// A board has one Triage, one Not now and one Done, and the applier stops
		// at a second one.
		if roles[role]++; role != bundle.RoleWork && roles[role] > 1 {
			return &Error{Kind: API, Message: fmt.Sprintf("board %d has more than one column of the role %s", b.id, role)}
		}
		carried = append(carried, list)
	}
	b.lists = carried
	sort.SliceStable(b.lists, func(i, j int) bool {
		a, c := b.lists[i], b.lists[j]
		if roleOrder[columnRoles[a.Type]] != roleOrder[columnRoles[c.Type]] {
			return roleOrder[columnRoles[a.Type]] < roleOrder[columnRoles[c.Type]]
		}
		if a.Position != nil && c.Position != nil && *a.Position != *c.Position {
			return *a.Position < *c.Position
		}
		return a.ID < c.ID
	})

	for i, list := range b.lists {
		record, err := e.record("column", list.ID, list.recording)
		if err != nil {
			return err
		}
		err = w.Add(bundle.Columns, bundle.Column{
			Record:          record,
			ProjectSourceID: projectID,
			ToolSourceID:    sourceID(b.id),
			Name:            list.Title,
			Position:        i + 1,
			Role:            columnRoles[list.Type],
		})
		if err != nil {
			return err
		}
		b.columns = append(b.columns, &column{id: list.ID})
	}
	return nil
}

// met holds a record to what a bundle can carry where the walk first meets it,
// for everything that is listed with a status. The first value says whether the
// record is carried; a run reaches nothing that hangs on one it is not.
//
// A status a bundle has no word for leaves the record out and is counted: a
// bundle has no field to say such a status in, so the record is lost whatever
// the run does, and going on costs it and nothing else. Times that are missing
// or are no times end the run, because a record without them would be put in
// the wrong place.
func (e *Extractor) met(what string, id int64, status string, origin recording, in, inSourceID string) (bool, error) {
	if !recordStatuses[status] {
		e.leaveOut(what, "status", status, in, inSourceID)
		return false, nil
	}
	return true, requireTimestamps(fmt.Sprintf("%s %d", what, id), origin.CreatedAt, origin.UpdatedAt)
}

// leaveOut notes a record a bundle cannot carry, counted per kind of record,
// per word of the source that made it impossible and per place it stood in. A
// number nobody writes down is a number nobody can check, and the word is the
// only thing that tells a sound effect from something that matters.
func (e *Extractor) leaveOut(record, field, value, in, inSourceID string) {
	for i := range e.recordsLeftOut {
		if one := &e.recordsLeftOut[i]; one.Record == record && one.Field == field && one.Value == value && one.InSourceID == inSourceID {
			one.LeftOut++
			return
		}
	}
	e.recordsLeftOut = append(e.recordsLeftOut,
		LeftOutRecords{Record: record, Field: field, Value: value, In: in, InSourceID: inSourceID, LeftOut: 1})
}

// record is what every record of a bundle carries, taken from what Basecamp
// says about where something comes from. Every kind gets it here, so that none
// comes without its times and nobody is named who is not in the bundle.
func (e *Extractor) record(what string, id int64, origin recording) (bundle.Record, error) {
	if err := requireTimestamps(fmt.Sprintf("%s %d", what, id), origin.CreatedAt, origin.UpdatedAt); err != nil {
		return bundle.Record{}, err
	}
	creator, err := e.people.refer(origin.Creator, false)
	if err != nil {
		return bundle.Record{}, err
	}
	return bundle.Record{SourceID: sourceID(id), CreatedAt: origin.CreatedAt, UpdatedAt: origin.UpdatedAt, CreatedBy: creator}, nil
}

func (e *Extractor) writeCard(ctx context.Context, w *bundle.Writer, bucket int64, toolID string, found placedCard) error {
	projectID := sourceID(bucket)

	origin, err := e.record("card", found.ID, found.recording)
	if err != nil {
		return err
	}
	assignees := append([]person(nil), found.Assignees...)
	sort.Slice(assignees, func(i, j int) bool { return assignees[i].ID < assignees[j].ID })
	assigneeIDs := make([]string, 0, len(assignees))
	for i := range assignees {
		reference, err := e.people.refer(&assignees[i], false)
		if err != nil {
			return err
		}
		if reference != nil {
			assigneeIDs = append(assigneeIDs, *reference)
		}
	}
	completedAt, completedBy, err := e.completed(found.Completion)
	if err != nil {
		return err
	}
	description, files, err := e.rewriteBody(found.Content, found.Description, found.Files)
	if err != nil {
		return err
	}

	record := bundle.Card{
		Record:          origin,
		ProjectSourceID: projectID,
		ToolSourceID:    toolID,
		ColumnSourceID:  sourceID(found.columnID),
		Title:           found.Title,
		Position:        found.position,
		DueOn:           found.DueOn,
		CompletedAt:     completedAt,
		CompletedBy:     completedBy,
		Assignees:       assigneeIDs,
		Status:          found.Status,
		OnHold:          found.onHold,
	}
	// A note without anything in it is no note.
	if description != "" {
		record.Description = &description
	}
	if err := w.Add(bundle.Cards, record); err != nil {
		return err
	}
	e.measure(bundle.Cards, description, LongestBodyRecord{Record: "card", SourceID: origin.SourceID, In: "project", InSourceID: projectID})
	e.cardsByStatus[found.Status]++
	if found.onHold {
		e.cardsOnHold++
	}
	if err := e.attachFiles(ctx, w, bundle.OwnerCard, origin, files); err != nil {
		return err
	}

	if err := e.writeSteps(ctx, w, projectID, toolID, found.card); err != nil {
		return err
	}
	return e.writeDiscussion(ctx, w, bucket, bundle.ParentCard, bundle.TargetCard, found.ID, found.CommentsCount, found.BoostsCount)
}

// writeDiscussion adds what hangs on a card or a post: its comments, its
// boosts, and the boosts of its comments, which stand behind its own.
func (e *Extractor) writeDiscussion(ctx context.Context, w *bundle.Writer, bucket int64, parentKind, targetKind string, id int64, commentsCount, boostsCount int) error {
	comments, err := e.writeComments(ctx, w, bucket, parentKind, id, commentsCount)
	if err != nil {
		return err
	}
	if boostsCount > 0 {
		if err := e.writeBoosts(ctx, w, bucket, targetKind, id); err != nil {
			return err
		}
	}
	for _, commented := range comments {
		if commented.BoostsCount > 0 {
			if err := e.writeBoosts(ctx, w, bucket, bundle.TargetComment, commented.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// completed says when and by whom something was completed. Basecamp says so in
// a record of its own, which is there only for what is completed.
func (e *Extractor) completed(c *completion) (at, by *string, err error) {
	if c == nil || c.CreatedAt == "" {
		return nil, nil, nil
	}
	by, err = e.people.refer(c.Creator, false)
	return &c.CreatedAt, by, err
}

// writeSteps adds the steps of a card in the order of their positions. A card
// shows its steps itself; only when it has more than it shows is the list of
// them asked for. Whom a step is assigned to and when it is due is left out:
// a step in Minka has neither.
func (e *Extractor) writeSteps(ctx context.Context, w *bundle.Writer, projectID, toolID string, found card) error {
	steps := found.Steps
	if found.SubtasksCount > len(steps) {
		path, err := e.relativePath(fmt.Sprintf("the steps of card %d", found.ID), found.SubtasksURL)
		if err != nil {
			return err
		}
		if steps, err = getList[step](ctx, e.Source, path); err != nil {
			return err
		}
	}
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].Position != steps[j].Position {
			return steps[i].Position < steps[j].Position
		}
		return steps[i].ID < steps[j].ID
	})

	for _, s := range steps {
		origin, err := e.record("step", s.ID, s.recording)
		if err != nil {
			return err
		}
		completedAt, completedBy, err := e.completed(s.Completion)
		if err != nil {
			return err
		}
		err = w.Add(bundle.Steps, bundle.Step{
			Record:          origin,
			ProjectSourceID: projectID,
			ToolSourceID:    toolID,
			CardSourceID:    sourceID(found.ID),
			Title:           s.Title,
			Position:        s.Position,
			CompletedAt:     completedAt,
			CompletedBy:     completedBy,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// writeComments adds the comments of something, oldest first, and returns the
// ones it wrote in that order. What is commented on says how many it has, so
// without any it costs no request.
//
// A comment in the trash is left out and counted: somebody threw it away, and
// a bundle has no place to say so. An active one comes, and an archived one,
// as an archived card has them.
func (e *Extractor) writeComments(ctx context.Context, w *bundle.Writer, bucket int64, parentKind string, parent int64, count int) ([]comment, error) {
	if count == 0 {
		return nil, nil
	}
	listed, err := getList[comment](ctx, e.Source, fmt.Sprintf("buckets/%d/recordings/%d/comments.json", bucket, parent))
	if err != nil {
		return nil, err
	}
	comments := make([]comment, 0, len(listed))
	for _, c := range listed {
		carried, err := e.met("comment", c.ID, c.Status, c.recording, "project", sourceID(bucket))
		if err != nil {
			return nil, err
		}
		if !carried {
			continue
		}
		if c.Status == bundle.StatusTrashed {
			e.commentsTrashed++
			continue
		}
		comments = append(comments, c)
	}
	if err := oldestFirst(comments, "comment", func(c comment) (int64, string) { return c.ID, c.CreatedAt }); err != nil {
		return nil, err
	}

	for _, c := range comments {
		origin, err := e.record("comment", c.ID, c.recording)
		if err != nil {
			return nil, err
		}
		body, files, err := e.rewriteBody(c.Content, "", c.Files)
		if err != nil {
			return nil, err
		}
		err = w.Add(bundle.Comments, bundle.Comment{
			Record:          origin,
			ProjectSourceID: sourceID(bucket),
			ParentKind:      parentKind,
			ParentSourceID:  sourceID(parent),
			Body:            body,
		})
		if err != nil {
			return nil, err
		}
		e.measure(bundle.Comments, body, LongestBodyRecord{Record: "comment", SourceID: origin.SourceID, In: "project", InSourceID: sourceID(bucket)})
		if err := e.attachFiles(ctx, w, bundle.OwnerComment, origin, files); err != nil {
			return nil, err
		}
	}
	return comments, nil
}

// writeBoosts adds the boosts of a card, a post, a comment or a line as
// reactions, oldest first. A boost does not name what it answers; whoever asks
// for it knows. It is written as it is, a text of some words included: what
// Minka takes for a reaction is the applier's to decide.
func (e *Extractor) writeBoosts(ctx context.Context, w *bundle.Writer, bucket int64, targetKind string, target int64) error {
	boosts, err := getList[boost](ctx, e.Source, fmt.Sprintf("buckets/%d/recordings/%d/boosts.json", bucket, target))
	if err != nil {
		return err
	}
	if err := oldestFirst(boosts, "boost", func(b boost) (int64, string) { return b.ID, b.CreatedAt }); err != nil {
		return err
	}

	for _, b := range boosts {
		booster, err := e.people.refer(b.Booster, false)
		if err != nil {
			return err
		}
		// A reaction is the one record that has to say whose it is.
		if booster == nil {
			return &Error{Kind: API, Message: fmt.Sprintf("Basecamp does not say who gave boost %d", b.ID)}
		}
		err = w.Add(bundle.Reactions, bundle.Reaction{
			Record:         bundle.Record{SourceID: sourceID(b.ID), CreatedAt: b.CreatedAt, UpdatedAt: b.CreatedAt, CreatedBy: booster},
			TargetKind:     targetKind,
			TargetSourceID: sourceID(target),
			Content:        b.Content,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
