package basecamp

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
)

// SourceName is what a bundle from Basecamp calls its source.
const SourceName = "basecamp"

type person struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	EmailAddress string `json:"email_address"`
	AvatarURL    string `json:"avatar_url"`
	// PersonableType is the kind of person: a user, a bot, what is left of
	// somebody who was deleted, and others.
	PersonableType string `json:"personable_type"`
	// TimeZone is the zone the person lives in, which is the zone a day of
	// theirs is read in where the source names none.
	TimeZone  string `json:"time_zone"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type project struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Status      string      `json:"status"`
	CreatedAt   string      `json:"created_at"`
	UpdatedAt   string      `json:"updated_at"`
	URL         string      `json:"url"`
	Dock        []dockEntry `json:"dock"`
}

type dockEntry struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Enabled  bool   `json:"enabled"`
	Position int    `json:"position"`
	URL      string `json:"url"`
}

// recording is what every record of Basecamp says about its origin.
type recording struct {
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	Creator   *person `json:"creator"`
}

// toolKinds maps the tools of a Basecamp project that Minka has a counterpart
// for. Everything else in a dock is skipped.
var toolKinds = map[string]string{
	"kanban_board":  bundle.ToolBoard,
	"chat":          bundle.ToolChat,
	"message_board": bundle.ToolPosts,
	"questionnaire": bundle.ToolCheckins,
	"todoset":       bundle.ToolTodos,
}

// SkippedTools counts dock entries of one kind that a run left out, either
// because Minka has nothing like them or because they are switched off.
type SkippedTools struct {
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
	Count   int    `json:"count"`
}

// MissingFile is an attachment a run could not fetch. Its record is left out
// of the bundle and whatever pointed to it points to nothing, so the bundle
// stays whole; a later run brings the file when the source hands it out.
type MissingFile struct {
	AttachmentSourceID string `json:"attachmentSourceId"`
	OwnerKind          string `json:"ownerKind"`
	OwnerSourceID      string `json:"ownerSourceId"`
	Reason             string `json:"reason"`
	// Retryable tells that the failure may pass by itself.
	Retryable bool `json:"retryable"`
}

// TodosLeftOut counts what a bundle does not carry of the To-dos tools of a run,
// per kind of record: a list, a group or a to-do whose status a bundle has no word
// for, one that hangs on a tool the run does not extract, and whatever stood in
// one of them and fell with it.
type TodosLeftOut struct {
	Lists  int `json:"lists"`
	Groups int `json:"groups"`
	Todos  int `json:"todos"`
}

// TodoPlaceLeftOut is a list or a group the bundle does not carry, with how many
// groups and to-dos stood in it. The place is named, because a number alone says
// that something is gone and never which.
type TodoPlaceLeftOut struct {
	// Record is the kind of place: a to-do list or a group.
	Record   string `json:"record"`
	SourceID string `json:"sourceId"`
	Groups   int    `json:"groups"`
	Todos    int    `json:"todos"`
}

// LeftOutRecords counts the records of one kind that a bundle cannot carry
// because the source said a word a bundle has none for. Such records are left
// out and the run goes on; what is lost stays nameable here.
type LeftOutRecords struct {
	// Record is the kind of record: card, post, comment, question, answer,
	// column or line.
	Record string `json:"record"`
	// Field is where the source said the word, Value what it said. Neither is
	// anything somebody wrote: a status, a kind of line, a kind of column.
	Field string `json:"field"`
	Value string `json:"value"`
	// In is the kind of record they stood in and InSourceID which one: the chat
	// of a line, the board of a column, the project of everything else.
	In         string `json:"in"`
	InSourceID string `json:"inSourceId"`
	LeftOut    int    `json:"leftOut"`
}

// LongestBodyRecord names the record whose text is the longest of its kind, so
// that whoever reads the number can look at the record at the source. It says
// nothing of the text: a kind of record, an identifier, the place it stands in
// and the length.
type LongestBodyRecord struct {
	// Record is the kind of record: card, comment, post, line, answer.
	Record   string `json:"record"`
	SourceID string `json:"sourceId"`
	// In is the kind of record it stands in and InSourceID which one: the chat
	// of a line, the project of everything else.
	In         string `json:"in"`
	InSourceID string `json:"inSourceId"`
	// Characters is how long the text is, the number LongestBody holds for the
	// kind.
	Characters int `json:"characters"`
}

// Result is what a run reports besides the counts of the bundle.
type Result struct {
	// SourceInstance is the ID of the Basecamp account.
	SourceInstance string
	Skipped        []SkippedTools
	Avatars        int
	Missing        []MissingFile
	// InvalidAddresses are the IDs of the people who came without an e-mail
	// address because the one the source holds is none.
	InvalidAddresses []string
	// PeopleLeftOut counts the people of the account nothing in the bundle
	// refers to, who are therefore not in it.
	PeopleLeftOut int
	// CreatorProbesRefused counts the dock entries asked for the creator of a
	// project that Basecamp refused the identity. They are passed over, so a
	// project may come without a creator it would otherwise have.
	CreatorProbesRefused int
	// CardsByStatus counts the cards of the bundle: active, archived, trashed.
	CardsByStatus map[string]int
	// CardsOnHold counts the cards that were set aside within their column.
	CardsOnHold int
	// CardsLeftOut counts archived and trashed cards whose column is in no
	// board that is extracted: a board that is switched off, a column that is
	// gone.
	CardsLeftOut int
	// CommentsTrashed counts the comments left out because they are in the
	// trash.
	CommentsTrashed int
	// PostsByStatus counts the posts of the bundle: active, archived, trashed.
	PostsByStatus map[string]int
	// PostsLeftOut counts archived and trashed posts of a message board that
	// is not extracted.
	PostsLeftOut int
	// PostsWithEmptyBody counts the posts that came with a title alone.
	PostsWithEmptyBody int
	// MessagesByType counts the lines of the bundle by what they were in the
	// chat: text, richText, upload, integration.
	MessagesByType map[string]int
	// MessagesWithEmptyBody counts the lines somebody wrote that say nothing,
	// which are left out.
	MessagesWithEmptyBody int
	// UploadLinesLeftOut counts the lines that were files alone, of which
	// Basecamp names none in a way a bundle can refer to.
	UploadLinesLeftOut int
	// RecordsLeftOut are the records a bundle cannot carry because the source
	// said a word a bundle has none for, counted per kind of record, per word
	// and per place.
	RecordsLeftOut []LeftOutRecords
	// TodoListsByStatus counts the lists of the bundle: active, archived,
	// trashed.
	TodoListsByStatus map[string]int
	// TodosByStatus counts the to-dos of the bundle: active, trashed. The shelf
	// is the list's alone.
	TodosByStatus map[string]int
	// TodosLeftOut counts what a bundle does not carry of the To-dos tools, per
	// kind of record, and TodoPlacesLeftOut names the lists and groups that took
	// records with them.
	TodosLeftOut      TodosLeftOut
	TodoPlacesLeftOut []TodoPlaceLeftOut
	// TodosNotDone counts the to-dos the source has open. A later run reopens
	// nothing, so one that is done in Minka stays done.
	TodosNotDone int
	// TodosWithPeople counts the to-dos that name somebody as an assignee or as
	// somebody to tell when it is done. A later run leaves the people of a to-do
	// as they stand in Minka, so a change at the source never arrives.
	TodosWithPeople int
	// TodosWithSubtasks counts the to-dos that had subtasks and SubtasksLeftOut
	// how many there were: a bundle carries none.
	TodosWithSubtasks int
	SubtasksLeftOut   int
	// QuestionsLeftOut are the questions left out because Minka has no words
	// for their schedule, each with the field that made it impossible.
	QuestionsLeftOut []ImpossibleQuestion
	// AnchorsInferred are the questions asked every other week whose startsOn
	// was taken from the day an answer belongs to.
	AnchorsInferred []InferredAnchor
	// NobodyAsked are the questions that come asking nobody at all, each with
	// how many people the bundle could not name as asked.
	NobodyAsked []NobodyAsked
	// AskedLeftOut counts the people a question is asked of that a bundle
	// cannot say are asked.
	AskedLeftOut AskedLeftOut
	// AnswersWithoutABody counts the answers that say nothing and
	// AnswersTrashed those in the trash; both are left out. AnswersWithoutADay
	// counts those Basecamp names no day for, which take the day they were
	// written on.
	AnswersWithoutABody int
	AnswersWithoutADay  int
	AnswersTrashed      int
	// LongestBody is the longest text of a kind, in characters, and
	// LongestBodyRecords the record it was measured on, per kind. A kind whose
	// longest text has no characters names no record.
	LongestBody        map[bundle.Kind]int
	LongestBodyRecords map[bundle.Kind]LongestBodyRecord
	Bodies             BodyCounts
	Files              FileCounts
}

// Extractor walks projects of a Basecamp account and adds what it finds to a
// bundle.
//
// People are collected while the run goes on and written last: only then is
// it known whom the records refer to, and somebody who left the account turns
// up only as the creator of one.
type Extractor struct {
	Source  Source
	Avatars AvatarFetcher
	Files   Files
	// MessagesPerPart is how many lines of a chat one file of a bundle holds
	// at most; messagesPerPart when 0.
	MessagesPerPart int
	// Now is the moment the run is made, which a schedule that has run out is
	// held against; the clock of the machine when nil.
	Now func() time.Time

	people  *registry
	account string
	apiRoot string
	skipped map[skippedKey]int
	missing []MissingFile
	// creatorProbesRefused counts the dock entries asked for the creator of a
	// project that the identity may not see.
	creatorProbesRefused int
	// talks are the message boards, chats and questionnaires of the projects
	// walked so far.
	talks []talk
	// members names, per project, whom the bundle carries as a member of it.
	// Only they can be asked a question of that project.
	members map[int64]map[int64]bool

	cardsByStatus      map[string]int
	cardsOnHold        int
	cardsLeftOut       int
	commentsTrashed    int
	postsByStatus      map[string]int
	postsLeftOut       int
	postsWithEmptyBody int
	messagesByType     map[string]int
	uploadLinesLeftOut int
	recordsLeftOut     []LeftOutRecords

	messagesWithEmptyBody int
	longestBody           map[bundle.Kind]int
	longestBodyRecords    map[bundle.Kind]LongestBodyRecord
	bodies                BodyCounts

	todoListsByStatus map[string]int
	todosByStatus     map[string]int
	todosLeftOut      TodosLeftOut
	todoPlacesLeftOut []TodoPlaceLeftOut
	todosNotDone      int
	todosWithPeople   int
	todosWithSubtasks int
	subtasksLeftOut   int

	questionsLeftOut    []ImpossibleQuestion
	anchorsInferred     []InferredAnchor
	nobodyAsked         []NobodyAsked
	askedLeftOut        AskedLeftOut
	answersWithoutABody int
	answersWithoutADay  int
	answersTrashed      int

	// fileOwners names the first owner of every file the walk has met, and
	// fileRecords are the records of those files in the order they were met.
	fileOwners  map[string]string
	fileRecords []bundle.Attachment
	files       FileCounts
}

// talk names the tools of one project that hold posts, lines, questions and
// to-dos, each kind in the order of the tools.
type talk struct {
	bucket         int64
	messageBoards  []int64
	chats          []int64
	questionnaires []int64
	todosets       []int64
	// everyTodoset is every dock entry of a project that is a To-dos tool, those
	// that are switched off included. A list names only the tool it stands in, so
	// what hangs on none of them is no list but a group.
	everyTodoset map[int64]bool
}

type skippedKey struct {
	kind    string
	enabled bool
}

var projectIDPattern = regexp.MustCompile(`^[0-9]{1,18}$`)

// ValidProjectID tells whether a value has the shape of a Basecamp ID: digits,
// and no more of them than a number holds. Only such a value may become part
// of a path.
func ValidProjectID(value string) bool {
	return projectIDPattern.MatchString(value)
}

// Extract walks the projects and adds their records to w. A discarding Writer
// gets the same records, except that no avatar and no file is downloaded.
//
// The walk goes kind by kind: the projects with their tools and boards, then
// the message boards of all of them, then the chats, then the questionnaires.
// So the comments and the reactions of a bundle stand in the order of what they
// hang on: those of cards, those of posts, those of lines, those of answers.
func (e *Extractor) Extract(ctx context.Context, projectIDs []string, w *bundle.Writer) (Result, error) {
	e.people = newRegistry()
	e.skipped = map[skippedKey]int{}
	e.missing = []MissingFile{}
	e.creatorProbesRefused = 0
	e.account, e.apiRoot = "", ""
	e.cardsByStatus = map[string]int{bundle.StatusActive: 0, bundle.StatusArchived: 0, bundle.StatusTrashed: 0}
	e.cardsOnHold, e.cardsLeftOut, e.commentsTrashed, e.bodies = 0, 0, 0, BodyCounts{}
	e.talks, e.members = nil, map[int64]map[int64]bool{}
	e.questionsLeftOut, e.anchorsInferred, e.nobodyAsked = []ImpossibleQuestion{}, []InferredAnchor{}, []NobodyAsked{}
	e.askedLeftOut, e.answersWithoutABody, e.answersWithoutADay, e.answersTrashed = AskedLeftOut{}, 0, 0, 0
	e.postsByStatus = map[string]int{bundle.StatusActive: 0, bundle.StatusArchived: 0, bundle.StatusTrashed: 0}
	e.postsLeftOut, e.postsWithEmptyBody, e.uploadLinesLeftOut, e.messagesWithEmptyBody = 0, 0, 0, 0
	e.recordsLeftOut = []LeftOutRecords{}
	e.messagesByType = map[string]int{}
	for _, name := range lineTypes {
		e.messagesByType[name] = 0
	}
	e.todoListsByStatus = map[string]int{bundle.StatusActive: 0, bundle.StatusArchived: 0, bundle.StatusTrashed: 0}
	e.todosByStatus = map[string]int{bundle.StatusActive: 0, bundle.StatusTrashed: 0}
	e.todosLeftOut, e.todoPlacesLeftOut = TodosLeftOut{}, []TodoPlaceLeftOut{}
	e.todosNotDone, e.todosWithPeople = 0, 0
	e.todosWithSubtasks, e.subtasksLeftOut = 0, 0
	e.longestBody = map[bundle.Kind]int{bundle.Cards: 0, bundle.Comments: 0, bundle.Posts: 0, bundle.Messages: 0, bundle.Answers: 0,
		bundle.TodoLists: 0, bundle.Todos: 0}
	e.longestBodyRecords = map[bundle.Kind]LongestBodyRecord{}
	e.fileOwners, e.fileRecords, e.files = map[string]string{}, nil, FileCounts{}

	ids, err := sortedIDs(projectIDs)
	if err != nil {
		return Result{}, err
	}

	listed, err := getList[person](ctx, e.Source, "people.json")
	if err != nil {
		return Result{}, err
	}
	// Before anything else is asked for: a run that cannot end well ends here.
	if err := e.people.listAccount(listed); err != nil {
		return Result{}, err
	}

	for _, id := range ids {
		if err := e.extractProject(ctx, id, w); err != nil {
			return Result{}, err
		}
	}
	for _, t := range e.talks {
		if err := e.extractPosts(ctx, w, t.bucket, t.messageBoards); err != nil {
			return Result{}, err
		}
	}
	if err := e.extractChats(ctx, w); err != nil {
		return Result{}, err
	}
	if err := e.extractCheckins(ctx, w); err != nil {
		return Result{}, err
	}
	if err := e.extractTodos(ctx, w); err != nil {
		return Result{}, err
	}

	avatars, err := e.writePeople(ctx, w)
	if err != nil {
		return Result{}, err
	}
	// The index of the files begins with the avatars, in the order of the
	// people, and goes on with the files in the order the walk met them.
	for _, record := range e.fileRecords {
		if err := w.Add(bundle.Attachments, record); err != nil {
			return Result{}, err
		}
	}
	return Result{
		SourceInstance:       e.account,
		Skipped:              e.skippedTools(),
		Avatars:              avatars,
		Missing:              e.missing,
		InvalidAddresses:     e.people.invalidAddresses(),
		PeopleLeftOut:        e.people.leftOut(),
		CreatorProbesRefused: e.creatorProbesRefused,
		CardsByStatus:        e.cardsByStatus,
		CardsOnHold:          e.cardsOnHold,
		CardsLeftOut:         e.cardsLeftOut,
		CommentsTrashed:      e.commentsTrashed,

		PostsByStatus:      e.postsByStatus,
		PostsLeftOut:       e.postsLeftOut,
		PostsWithEmptyBody: e.postsWithEmptyBody,
		MessagesByType:     e.messagesByType,
		UploadLinesLeftOut: e.uploadLinesLeftOut,
		RecordsLeftOut:     e.recordsLeftOut,

		TodoListsByStatus: e.todoListsByStatus,
		TodosByStatus:     e.todosByStatus,
		TodosLeftOut:      e.todosLeftOut,
		TodoPlacesLeftOut: e.todoPlacesLeftOut,
		TodosNotDone:      e.todosNotDone,
		TodosWithPeople:   e.todosWithPeople,
		TodosWithSubtasks: e.todosWithSubtasks,
		SubtasksLeftOut:   e.subtasksLeftOut,

		QuestionsLeftOut:    e.questionsLeftOut,
		AnchorsInferred:     e.anchorsInferred,
		NobodyAsked:         e.nobodyAsked,
		AskedLeftOut:        e.askedLeftOut,
		AnswersWithoutABody: e.answersWithoutABody,
		AnswersWithoutADay:  e.answersWithoutADay,
		AnswersTrashed:      e.answersTrashed,

		MessagesWithEmptyBody: e.messagesWithEmptyBody,
		LongestBody:           e.longestBody,
		LongestBodyRecords:    e.longestBodyRecords,
		Bodies:                e.bodies,
		Files:                 e.files,
	}, nil
}

// now is the moment of the run.
func (e *Extractor) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}
	return e.Now()
}

func sortedIDs(values []string) ([]int64, error) {
	seen := map[int64]bool{}
	var ids []int64
	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || !ValidProjectID(value) {
			return nil, fmt.Errorf("%q is not the ID of a Basecamp project", value)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (e *Extractor) extractProject(ctx context.Context, id int64, w *bundle.Writer) error {
	p, err := getOne[project](ctx, e.Source, fmt.Sprintf("projects/%d.json", id))
	if err != nil {
		var failed *Error
		if errors.As(err, &failed) && failed.Kind == NotFound {
			return &Error{Kind: NotFound, Message: fmt.Sprintf("Basecamp has no project %d that you can see", id)}
		}
		return err
	}
	if err := e.rememberAccount(p); err != nil {
		return err
	}
	if !projectStatuses[p.Status] {
		return &Error{Kind: API, Message: fmt.Sprintf("project %d has the status %q, which a bundle does not know", id, p.Status)}
	}
	if err := requireTimestamps(fmt.Sprintf("project %d", id), p.CreatedAt, p.UpdatedAt); err != nil {
		return err
	}

	members, err := getList[person](ctx, e.Source, fmt.Sprintf("projects/%d/people.json", id))
	if err != nil {
		return err
	}
	memberIDs := make([]string, 0, len(members))
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	e.members[id] = map[int64]bool{}
	for i := range members {
		reference, err := e.people.refer(&members[i], true)
		if err != nil {
			return err
		}
		if reference != nil {
			memberIDs = append(memberIDs, *reference)
			e.members[id][members[i].ID] = true
		}
	}

	type extractedTool struct {
		entry   dockEntry
		details toolDetails
	}
	var tools []extractedTool
	var made []recording
	var others []dockEntry
	// Every To-dos tool of the project, those that are switched off included: a
	// list of one that is not extracted still has to be told from a group.
	everyTodoset := map[int64]bool{}
	dock := append([]dockEntry(nil), p.Dock...)
	sort.SliceStable(dock, func(i, j int) bool { return dock[i].ID < dock[j].ID })
	for _, entry := range dock {
		if toolKinds[entry.Name] == bundle.ToolTodos {
			everyTodoset[entry.ID] = true
		}
		if _, known := toolKinds[entry.Name]; !known || !entry.Enabled {
			e.skipped[skippedKey{kind: entry.Name, enabled: entry.Enabled}]++
			others = append(others, entry)
			continue
		}
		path, err := e.relativePath(fmt.Sprintf("tool %d", entry.ID), entry.URL)
		if err != nil {
			return err
		}
		details, err := getOne[toolDetails](ctx, e.Source, path)
		if err != nil {
			return err
		}
		tools = append(tools, extractedTool{entry: entry, details: details})
		made = append(made, details.recording)
	}

	creator, err := e.projectCreator(ctx, p, made, others)
	if err != nil {
		return err
	}
	projectID := sourceID(p.ID)
	err = w.Add(bundle.Projects, bundle.Project{
		Record:      bundle.Record{SourceID: projectID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt, CreatedBy: creator},
		Name:        p.Name,
		Description: textToHTML(p.Description),
		Status:      p.Status,
		Members:     memberIDs,
	})
	if err != nil {
		return err
	}

	var boards []*board
	talks := talk{bucket: id, everyTodoset: everyTodoset}
	for _, tool := range tools {
		record, err := e.record("tool", tool.entry.ID, tool.details.recording)
		if err != nil {
			return err
		}
		kind := toolKinds[tool.entry.Name]
		err = w.Add(bundle.Tools, bundle.Tool{
			Record:          record,
			ProjectSourceID: projectID,
			Kind:            kind,
			Name:            tool.entry.Title,
			Position:        tool.entry.Position,
		})
		if err != nil {
			return err
		}
		switch kind {
		case bundle.ToolBoard:
			boards = append(boards, &board{id: tool.entry.ID, lists: tool.details.Lists})
		case bundle.ToolPosts:
			talks.messageBoards = append(talks.messageBoards, tool.entry.ID)
		case bundle.ToolChat:
			talks.chats = append(talks.chats, tool.entry.ID)
		case bundle.ToolCheckins:
			talks.questionnaires = append(talks.questionnaires, tool.entry.ID)
		case bundle.ToolTodos:
			talks.todosets = append(talks.todosets, tool.entry.ID)
		}
	}
	e.talks = append(e.talks, talks)
	return e.extractBoards(ctx, w, id, boards)
}

const (
	// projectCreationWindow is how long after a project a tool may have been
	// made to count as made together with it. Basecamp makes the tools of a new
	// project within a fraction of a second; a tool somebody adds by hand comes
	// minutes later at the earliest.
	projectCreationWindow = 5 * time.Second
	// maxCreatorProbes is how many tools a run asks for besides the ones it
	// extracts, to find who created a project.
	maxCreatorProbes = 3
)

// projectCreator finds who created a project. Basecamp does not say, but it
// makes the first tools of a project in the same moment and in the name of
// whoever created it. So the creator of a tool that was made with the project
// is the creator of the project; a tool added later, by whomever, says
// nothing. Without such a tool the project comes without a creator.
//
// The tools the run extracts are looked at first, in the order of their IDs,
// so that the oldest of them decides wherever several were made with the
// project. Only when none of them answers are a few of the others asked for,
// those that are switched off included, which answer like any other. They go
// in the order of their IDs, which is the order they were made in, up to the
// first that answers the question.
//
// Such a tool is a question on the side, not a record of the bundle: one that
// is not there, and one the identity may not see, is passed over and the run
// takes what the rest gives. A single entry somebody may not open says nothing
// about the project, whereas an identity that may see nothing is refused where
// the run asks for the people of the account and for the project itself. A
// refusal that is passed over is counted, so that no run stays silent about it.
func (e *Extractor) projectCreator(ctx context.Context, p project, extracted []recording, others []dockEntry) (*string, error) {
	created, isAMoment := aMoment(p.CreatedAt)
	if !isAMoment {
		return nil, nil
	}
	madeWithProject := func(tool recording) bool {
		made, isAMoment := aMoment(tool.CreatedAt)
		if !isAMoment || tool.Creator == nil {
			return false
		}
		after := made.Sub(created)
		return after >= 0 && after <= projectCreationWindow
	}

	for _, tool := range extracted {
		if madeWithProject(tool) {
			return e.people.refer(tool.Creator, false)
		}
	}
	probes := 0
	for _, entry := range others {
		if probes == maxCreatorProbes {
			break
		}
		path, err := e.relativePath(fmt.Sprintf("tool %d", entry.ID), entry.URL)
		if err != nil {
			continue
		}
		probes++
		tool, err := getOne[recording](ctx, e.Source, path)
		var failed *Error
		if errors.As(err, &failed) && (failed.Kind == NotFound || failed.Kind == Forbidden) {
			if failed.Kind == Forbidden {
				e.creatorProbesRefused++
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		if madeWithProject(tool) {
			return e.people.refer(tool.Creator, false)
		}
	}
	return nil, nil
}

var projectStatuses = map[string]bool{"active": true, "archived": true, "trashed": true}

// requireTimestamps refuses a record without its times and one whose times are
// no times. The applier decides by updatedAt whether a run changes a record, so
// a bundle never carries a guess.
func requireTimestamps(what, createdAt, updatedAt string) error {
	var missing []string
	if createdAt == "" {
		missing = append(missing, "created")
	}
	if updatedAt == "" {
		missing = append(missing, "updated")
	}
	if len(missing) > 0 {
		return &Error{Kind: API, Message: "Basecamp does not say when " + what + " was " + strings.Join(missing, " and ")}
	}
	if _, err := momentOf(what, "created", createdAt); err != nil {
		return err
	}
	_, err := momentOf(what, "updated", updatedAt)
	return err
}

// momentShape is the shape the applier holds a moment of a bundle to. What has
// that shape is made of digits and the punctuation of a time, so a message may
// say it out loud, and it is the reading of the calendar alone that refused it.
var momentShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

// momentOf reads a time of a record and names the record and the field, so that
// a run says where in the source the repair belongs.
//
// What stood there is said only where it has the shape of a moment: the
// thirtieth of February reads like a time and is none, and a reader who is not
// told the value has nothing to go on. A value of any other shape is not
// repeated, because a field that holds no time holds whatever the source put
// there, a name among it, and no message of a run says a name.
func momentOf(what, field, value string) (time.Time, error) {
	moment, isAMoment := aMoment(value)
	if !isAMoment {
		return time.Time{}, &Error{
			Kind:    API,
			Message: fmt.Sprintf("Basecamp says that %s was %s at %s", what, field, noTime(value)),
		}
	}
	return moment, nil
}

// noTime is how a message says a value that is no moment.
func noTime(value string) string {
	if momentShape.MatchString(value) {
		return strconv.Quote(value) + ", which no calendar has"
	}
	return "something that is no time"
}

// aMoment reads a moment the way Basecamp writes one: a date, a time of day, a
// fraction of a second where there is one, and the zone it is meant in, with
// nothing around it and nothing else in its place. Everything a bundle carries
// a time for is read here, and it is read strictly, because the applier draws
// the line in the same place and a line two programs draw differently is no
// line but the place where a time silently changes its meaning.
//
// What a looser reading takes is a relative word such as "now" or "tomorrow",
// and a time without a zone, which is somebody's guess at a zone. Either would
// store the record at a moment nobody meant, and nothing afterwards tells that
// from the truth. Refused besides is a moment of the right shape that no
// calendar has, the thirtieth of February among them.
func aMoment(value string) (time.Time, bool) {
	moment, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return moment, true
}

var projectAddressPattern = regexp.MustCompile(`^(https://[^/?#]+/([0-9]+)/)projects/([0-9]+)\.json$`)

// rememberAccount takes the account from the address of a project:
// https://3.basecampapi.com/<account>/projects/<id>.json. The account names
// the bundle and its address is what every other address is held against, so
// an address of another shape is refused, and so is a project of another
// account than the ones before it.
func (e *Extractor) rememberAccount(p project) error {
	match := projectAddressPattern.FindStringSubmatch(p.URL)
	if match == nil || match[3] != sourceID(p.ID) {
		return &Error{Kind: API, Message: fmt.Sprintf("the address of project %d does not name its account", p.ID)}
	}
	if e.apiRoot != "" && e.apiRoot != match[1] {
		return &Error{Kind: API, Message: fmt.Sprintf("project %d belongs to another account than the projects before it", p.ID)}
	}
	e.apiRoot, e.account = match[1], match[2]
	return nil
}

var relativePathPattern = regexp.MustCompile(`^[a-z0-9_]+(/[a-z0-9_]+)*\.json$`)

// relativePath turns an address out of an API answer into a path of the
// account. The Basecamp CLI signs every request it makes, so only paths of the
// account reach it: an address that leads anywhere else is refused instead of
// fetched.
//
// The address is not quoted: it is whatever the answer held.
func (e *Extractor) relativePath(what, address string) (string, error) {
	path, found := strings.CutPrefix(address, e.apiRoot)
	if !found || !relativePathPattern.MatchString(path) {
		return "", &Error{Kind: API, Message: "Basecamp names " + what + " at an address outside the account"}
	}
	return path, nil
}

// writePeople adds everybody the run has seen, each with their avatar.
func (e *Extractor) writePeople(ctx context.Context, w *bundle.Writer) (int, error) {
	avatars := 0
	for _, seen := range e.people.sorted() {
		id := sourceID(seen.ID)
		record := bundle.Person{
			Record:  bundle.Record{SourceID: id, CreatedAt: seen.CreatedAt, UpdatedAt: seen.UpdatedAt},
			Name:    seen.Name,
			Retired: seen.retired(),
		}
		if seen.EmailAddress != "" {
			record.Email = &seen.EmailAddress
		}

		if seen.AvatarURL != "" {
			avatar := bundle.Attachment{
				Record:        bundle.Record{SourceID: "avatar-" + id, CreatedAt: seen.CreatedAt, UpdatedAt: seen.UpdatedAt, CreatedBy: &id},
				OwnerKind:     bundle.OwnerPerson,
				OwnerSourceID: id,
			}
			reference, err := e.attach(ctx, w, avatar, func(ctx context.Context) (download, error) {
				fetched, err := e.Avatars.Fetch(ctx, seen.AvatarURL)
				return download{Filename: avatarFilenames[fetched.ContentType], ContentType: fetched.ContentType, Content: fetched.Content}, err
			})
			if err != nil {
				return 0, err
			}
			record.Avatar = reference
			if reference != nil {
				avatars++
			}
		}

		if err := w.Add(bundle.People, record); err != nil {
			return 0, err
		}
	}
	return avatars, nil
}

type download struct {
	Filename    string
	ContentType string
	Content     []byte
}

// attach fetches the file of an attachment, stores it and adds the record. It
// returns the reference the owner carries, which is nil when the file could
// not be fetched: one file that is not to be had must not cost a run that may
// have taken hours, so it is noted as missing and the run goes on. Only an
// interruption ends the run, and an error of the bundle itself.
//
// A discarding Writer fetches nothing and counts the record.
func (e *Extractor) attach(ctx context.Context, w *bundle.Writer, record bundle.Attachment, fetch func(context.Context) (download, error)) (*string, error) {
	if !w.Discards() {
		fetched, err := fetch(ctx)
		if err != nil {
			var failed *Error
			if !errors.As(err, &failed) || failed.Kind == Interrupted || ctx.Err() != nil {
				return nil, err
			}
			e.missing = append(e.missing, MissingFile{
				AttachmentSourceID: record.SourceID,
				OwnerKind:          record.OwnerKind,
				OwnerSourceID:      record.OwnerSourceID,
				Reason:             failed.Message,
				Retryable:          failed.Transient(),
			})
			return nil, nil
		}
		stored, err := w.AddFile(record.SourceID, fetched.Filename, fetched.Content)
		if err != nil {
			return nil, err
		}
		record.Filename, record.ContentType = fetched.Filename, &fetched.ContentType
		record.Holds(stored)
	}
	if err := w.Add(bundle.Attachments, record); err != nil {
		return nil, err
	}
	return &record.SourceID, nil
}

func (e *Extractor) skippedTools() []SkippedTools {
	skipped := make([]SkippedTools, 0, len(e.skipped))
	for key, count := range e.skipped {
		skipped = append(skipped, SkippedTools{Kind: key.kind, Enabled: key.enabled, Count: count})
	}
	sort.Slice(skipped, func(i, j int) bool {
		if skipped[i].Kind != skipped[j].Kind {
			return skipped[i].Kind < skipped[j].Kind
		}
		return skipped[i].Enabled && !skipped[j].Enabled
	})
	return skipped
}

// measure notes how long a text of a kind is, in characters, and which record
// it was: whoever sets the limits of where a bundle goes has to know the
// longest text, and a number that names no record is a number nobody can
// check. Of two texts of the same length the one met first keeps the place.
func (e *Extractor) measure(kind bundle.Kind, text string, of LongestBodyRecord) {
	length := utf8.RuneCountInString(text)
	if length <= e.longestBody[kind] {
		return
	}
	of.Characters = length
	e.longestBody[kind], e.longestBodyRecords[kind] = length, of
}

func sourceID(id int64) string {
	return strconv.FormatInt(id, 10)
}

var paragraphBreak = regexp.MustCompile(`\n{2,}`)

// textToHTML turns the plain text of Basecamp into the HTML a bundle carries:
// a paragraph per block, a line break per line.
func textToHTML(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if text == "" {
		return ""
	}
	var out strings.Builder
	for _, paragraph := range paragraphBreak.Split(text, -1) {
		out.WriteString("<p>")
		out.WriteString(strings.ReplaceAll(html.EscapeString(strings.TrimSpace(paragraph)), "\n", "<br>"))
		out.WriteString("</p>")
	}
	return out.String()
}
