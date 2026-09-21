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

	"github.com/flownative/minka-cli/internal/bundle"
)

// SourceName is what a bundle from Basecamp calls its source.
const SourceName = "basecamp"

type person struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	EmailAddress string `json:"email_address"`
	AvatarURL    string `json:"avatar_url"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
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

	people  *registry
	account string
	apiRoot string
	skipped map[skippedKey]int
	missing []MissingFile
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
// gets the same records, except that no avatar is downloaded.
func (e *Extractor) Extract(ctx context.Context, projectIDs []string, w *bundle.Writer) (Result, error) {
	e.people = newRegistry()
	e.skipped = map[skippedKey]int{}
	e.missing = []MissingFile{}
	e.account, e.apiRoot = "", ""

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

	avatars, err := e.writePeople(ctx, w)
	if err != nil {
		return Result{}, err
	}
	return Result{
		SourceInstance:   e.account,
		Skipped:          e.skippedTools(),
		Avatars:          avatars,
		Missing:          e.missing,
		InvalidAddresses: e.people.invalidAddresses(),
		PeopleLeftOut:    e.people.leftOut(),
	}, nil
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
	for i := range members {
		reference, err := e.people.refer(&members[i], true)
		if err != nil {
			return err
		}
		if reference != nil {
			memberIDs = append(memberIDs, *reference)
		}
	}

	projectID := sourceID(p.ID)
	err = w.Add(bundle.Projects, bundle.Project{
		Record:      bundle.Record{SourceID: projectID, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt},
		Name:        p.Name,
		Description: textToHTML(p.Description),
		Status:      p.Status,
		Members:     memberIDs,
	})
	if err != nil {
		return err
	}

	dock := append([]dockEntry(nil), p.Dock...)
	sort.Slice(dock, func(i, j int) bool { return dock[i].ID < dock[j].ID })
	for _, entry := range dock {
		kind, known := toolKinds[entry.Name]
		if !known || !entry.Enabled {
			e.skipped[skippedKey{kind: entry.Name, enabled: entry.Enabled}]++
			continue
		}
		path, err := e.relativePath(entry.URL)
		if err != nil {
			return err
		}
		origin, err := getOne[recording](ctx, e.Source, path)
		if err != nil {
			return err
		}
		if err := requireTimestamps(fmt.Sprintf("tool %d of project %d", entry.ID, id), origin.CreatedAt, origin.UpdatedAt); err != nil {
			return err
		}
		creator, err := e.people.refer(origin.Creator, false)
		if err != nil {
			return err
		}
		err = w.Add(bundle.Tools, bundle.Tool{
			Record:          bundle.Record{SourceID: sourceID(entry.ID), CreatedAt: origin.CreatedAt, UpdatedAt: origin.UpdatedAt, CreatedBy: creator},
			ProjectSourceID: projectID,
			Kind:            kind,
			Name:            entry.Title,
			Position:        entry.Position,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

var projectStatuses = map[string]bool{"active": true, "archived": true, "trashed": true}

// requireTimestamps refuses a record without its times. The applier decides by
// updatedAt whether a run changes a record, so a bundle never carries a guess.
func requireTimestamps(what, createdAt, updatedAt string) error {
	if createdAt == "" || updatedAt == "" {
		return &Error{Kind: API, Message: "Basecamp does not say when " + what + " was created and updated"}
	}
	return nil
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
func (e *Extractor) relativePath(address string) (string, error) {
	path, found := strings.CutPrefix(address, e.apiRoot)
	if !found || !relativePathPattern.MatchString(path) {
		return "", &Error{Kind: API, Message: "Basecamp names a tool at an address outside the account: " + address}
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
			Retired: !seen.listed,
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
				Retryable:          failed.Kind == Network || failed.Kind == RateLimited,
			})
			return nil, nil
		}
		stored, err := w.AddFile(record.SourceID, fetched.Filename, fetched.Content)
		if err != nil {
			return nil, err
		}
		record.Filename, record.ContentType = fetched.Filename, fetched.ContentType
		record.File, record.Bytes, record.SHA256 = stored.File, stored.Bytes, stored.SHA256
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
