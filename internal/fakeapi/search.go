package fakeapi

import (
	"html"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/richtext"
)

// What a search serves: a page of a fixed size, a term cut to a length, an
// excerpt of about this many characters, and no page behind the deepest
// offset.
const (
	searchPageSize      = 25
	searchTermMaxLength = 200
	searchExcerptLength = 160
	searchDeepestOffset = 500
)

// searchKinds are the words "kinds" takes. Each names a tool, and a comment
// counts toward the tool of what it stands under.
var searchKinds = []string{"board", "posts", "todos", "files", "calendar", "chat", "conversation"}

// The kinds of hit whose word differs from the one of their item type.
const (
	searchHitTodoList  = "todoList"
	searchHitTodoGroup = "todoGroup"
)

// searchOffsetForm is an offset Minka takes: at most six digits, or none.
var searchOffsetForm = regexp.MustCompile(`^\d{0,6}$`)

// headingForm finds the headings of a body in the order they stand.
var headingForm = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)

func (s *Server) registerSearchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/search", s.authenticated(s.search))
}

// searchSection is the part of a document from one heading to the next, or
// the part before the first heading, which names no heading.
type searchSection struct {
	heading string
	anchor  string
	text    string // what the term is looked for in, the heading included
	excerpt string // what the excerpt is made of: the heading stands beside it
}

type searchHit struct {
	resource  map[string]any
	id        string
	inTitle   bool
	updatedAt string
}

// searchFilter is what a search is narrowed to besides its term.
type searchFilter struct {
	kinds     []string // nil for every kind
	projectID string
	// withinFiles is a search within a Files tool, a folder or both: it finds
	// documents and uploads there, and neither folders nor comments.
	withinFiles bool
	tool        *FilesTool
	folderID    string
}

func (f searchFilter) includes(kind string) bool {
	return f.kinds == nil || slices.Contains(f.kinds, kind)
}

// search finds what the person may read, refusing in the order the
// controller does: the kinds, the project, the tool and folder, the term, the
// offset. A Minka that searches only within files asks for a scope first and
// knows neither kinds nor a project.
func (s *Server) search(w http.ResponseWriter, r *http.Request, person *Person) {
	query := r.URL.Query()
	toolID := strings.ToLower(query.Get("toolId"))
	folderID := strings.ToLower(query.Get("folderId"))
	filter := searchFilter{withinFiles: toolID != "" || folderID != "", folderID: folderID}
	if s.SearchesOnlyWithinFiles {
		if !filter.withinFiles {
			unprocessable(w, "searchScopeMissing", `A search names "toolId", "folderId" or both.`)
			return
		}
	} else {
		kinds, ok := searchKindsOf(query.Get("kinds"))
		if !ok {
			unprocessable(w, "searchKindsUnknown", `"kinds" takes "board", "posts", "todos", "files", "calendar", "chat", "conversation", separated by commas.`)
			return
		}
		filter.kinds = kinds
		if projectID := query.Get("projectId"); projectID != "" {
			filter.projectID = strings.ToLower(projectID)
			if !uuidPattern.MatchString(filter.projectID) || s.visibleProject(filter.projectID, person) == nil {
				notFound(w)
				return
			}
		}
	}
	if filter.withinFiles {
		tool, ok := s.searchScope(toolID, folderID, person)
		if !ok {
			notFound(w)
			return
		}
		filter.tool = tool
	}
	asked := []rune(phpTrim(query.Get("q")))
	term := phpTrim(string(asked[:min(searchTermMaxLength, len(asked))]))
	if term == "" {
		unprocessable(w, "searchTermMissing", `A search names what it looks for in "q".`)
		return
	}
	offsetText := query.Get("offset")
	if !searchOffsetForm.MatchString(offsetText) {
		unprocessable(w, "searchOffsetNotANumber", `The "offset" of a search is the "next" of the page before.`)
		return
	}
	offset, _ := strconv.Atoi(offsetText)
	if offset > searchDeepestOffset && !s.SearchesOnlyWithinFiles {
		unprocessable(w, "searchOffsetTooLarge", `The "offset" of a search goes up to 500; narrow the search to see further.`)
		return
	}

	finder := &searchFinder{s: s, person: person, filter: filter, needle: strings.ToLower(term)}
	if filter.withinFiles {
		finder.entriesOf(filter.tool)
	} else {
		finder.everywhere()
	}
	hits := finder.hits
	// The identifier settles a tie, as it does in Minka's statement.
	recent := query.Get("sort") == "recent"
	slices.SortFunc(hits, func(a, b searchHit) int {
		if !recent && a.inTitle != b.inTitle {
			if a.inTitle {
				return -1
			}
			return 1
		}
		if order := strings.Compare(b.updatedAt, a.updatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.id, b.id)
	})

	page := []any{}
	for i := offset; i < len(hits) && i < offset+searchPageSize; i++ {
		resource := hits[i].resource
		if s.SearchesOnlyWithinFiles {
			resource = maps.Clone(resource)
			for _, field := range []string{"targetKind", "targetId", "shortId", "shortUrl", "apiUrl"} {
				delete(resource, field)
			}
		}
		page = append(page, resource)
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": page, "next": s.searchNext(offset, len(hits))})
}

// searchNext is the offset of the page after this one, and nil on the last
// one. Minka names no page it would refuse to serve.
func (s *Server) searchNext(offset, total int) any {
	next := offset + searchPageSize
	if total <= next || next > searchDeepestOffset && !s.NamesPagesBeyondTheDeepestOffset && !s.SearchesOnlyWithinFiles {
		return nil
	}
	return strconv.Itoa(next)
}

// searchKindsOf reads the kinds of a search. Blank is every kind; an empty
// part between commas is no kind.
func searchKindsOf(kinds string) ([]string, bool) {
	if phpTrim(kinds) == "" {
		return nil, true
	}
	var read []string
	for _, kind := range strings.Split(kinds, ",") {
		kind = phpTrim(kind)
		if !slices.Contains(searchKinds, kind) {
			return nil, false
		}
		read = append(read, kind)
	}
	return read, true
}

// searchScope is the tool the search stays in. What is malformed, not there,
// not to be read, or a folder of another tool than the one named, is no scope.
func (s *Server) searchScope(toolID, folderID string, person *Person) (*FilesTool, bool) {
	var tool *FilesTool
	if toolID != "" {
		if !uuidPattern.MatchString(toolID) {
			return nil, false
		}
		if tool = s.visibleFilesTool(toolID, person); tool == nil {
			return nil, false
		}
	}
	if folderID == "" {
		return tool, true
	}
	if !uuidPattern.MatchString(folderID) {
		return nil, false
	}
	var toolOfFolder *FilesTool
	for _, candidate := range s.FilesTools {
		if folderIn(candidate, folderID) != nil {
			toolOfFolder = s.visibleFilesTool(candidate.ID, person)
		}
	}
	if toolOfFolder == nil || tool != nil && tool != toolOfFolder {
		return nil, false
	}
	if folderInUse(toolOfFolder, folderID) == nil || s.isAClient(person.ID) && !shownToClients(toolOfFolder, folderID) {
		return nil, false
	}
	return toolOfFolder, true
}

// searchItem is a hit before it is described: an item, a comment under one or
// a line of a chat.
type searchItem struct {
	kind, id string
	// targetKind and targetID name what the addresses lead to; empty for the
	// item itself.
	targetKind, targetID string
	title, excerpt       string
	section              any
	projectID, toolID    string // empty for a private conversation
	shortID              string
	updatedAt            time.Time
	archived             bool
	appURL               string
	apiPath              string // below /api/v1/; empty where the API shows the item nowhere on its own
	inTitle              bool
}

// searchHitOf is a hit as Minka describes it.
func (s *Server) searchHitOf(item searchItem) searchHit {
	if item.targetKind == "" {
		item.targetKind, item.targetID = item.kind, item.id
	}
	var project, shortURL, apiURL any
	if p := s.projectWithID(item.projectID); p != nil {
		project = p.Name
	}
	if item.shortID != "" {
		shortURL = s.URL + "/go/card/" + item.shortID
	}
	if item.apiPath != "" {
		apiURL = s.URL + "/api/v1/" + item.apiPath
	}
	return searchHit{id: item.id, inTitle: item.inTitle, updatedAt: timestamp(item.updatedAt), resource: map[string]any{
		"kind": item.kind, "id": item.id, "targetKind": item.targetKind, "targetId": item.targetID,
		"title": item.title, "excerpt": item.excerpt, "section": item.section,
		"projectId": nullable(item.projectID), "toolId": nullable(item.toolID), "project": project,
		"shortId": nullable(item.shortID), "shortUrl": shortURL, "updatedAt": timestamp(item.updatedAt),
		"archived": item.archived, "appUrl": item.appURL, "apiUrl": apiURL,
	}}
}

// searchFinder collects the hits of one search.
type searchFinder struct {
	s      *Server
	person *Person
	filter searchFilter
	needle string
	hits   []searchHit
}

func (f *searchFinder) holds(text string) bool {
	return strings.Contains(strings.ToLower(text), f.needle)
}

// searches says whether the items of a project are searched: the person is
// a member, the project is in use and no template library, and it is the
// project asked for, where one is.
func (f *searchFinder) searches(projectID string) bool {
	project := f.s.visibleProject(projectID, f.person)
	return project != nil && !project.Archived && !project.TemplateLibrary && (f.filter.projectID == "" || f.filter.projectID == projectID)
}

// item adds an item that holds the term in its title or its text.
func (f *searchFinder) item(item searchItem, text string) {
	item.inTitle = f.holds(item.title)
	if !item.inTitle && !f.holds(text) {
		return
	}
	item.excerpt = excerpt(text, f.needle)
	f.hits = append(f.hits, f.s.searchHitOf(item))
}

// line adds a comment or a message that holds the term in its text. The title
// of what it stands under does not make it a hit.
func (f *searchFinder) line(item searchItem, text string) {
	if !f.holds(text) {
		return
	}
	item.excerpt = excerpt(text, f.needle)
	f.hits = append(f.hits, f.s.searchHitOf(item))
}

// comments adds the comments under an item, which count toward the item's
// tool and lead to the item, opened at the comment.
func (f *searchFinder) comments(kind string, comments []*Comment, parent searchItem) {
	if !f.filter.includes(kind) {
		return
	}
	for _, comment := range comments {
		hit := parent
		hit.kind, hit.id, hit.targetKind, hit.targetID = "comment", comment.ID, parent.kind, parent.id
		hit.updatedAt, hit.appURL = comment.CreatedAt, parent.appURL+"#comment-"+comment.ID
		f.line(hit, plainText(comment.Body))
	}
}

// everywhere finds across every project the person is a member of and in
// their own private conversations.
func (f *searchFinder) everywhere() {
	f.cards()
	f.posts()
	f.todos()
	for _, tool := range f.s.FilesTools {
		if f.s.visibleFilesTool(tool.ID, f.person) != nil {
			f.entriesOf(tool)
		}
	}
	f.events()
	f.chats()
}

// cards finds the cards on a board out of the trash and in its archive.
func (f *searchFinder) cards() {
	s := f.s
	found := func(board *Board, card *Card, archived bool) {
		if board == nil || board.Trashed || !f.searches(board.ProjectID) {
			return
		}
		hit := searchItem{kind: "card", id: card.ID, title: card.Title, projectID: board.ProjectID, toolID: board.ID,
			shortID: card.ShortID, updatedAt: card.CreatedAt, archived: archived, appURL: s.cardURL(board, card), apiPath: "cards/" + card.ID}
		if f.filter.includes("board") {
			f.item(hit, plainText(card.Description))
		}
		f.comments("board", card.Comments, hit)
	}
	for _, board := range s.Boards {
		for _, column := range board.Columns {
			for _, card := range column.Cards {
				found(board, card, false)
			}
		}
	}
	for _, archived := range s.Archive {
		found(s.boardOfColumn(archived.Column), archived.Card, true)
	}
}

// posts finds what went out, in the list or the archive; a client only what
// is shown to the clients.
func (f *searchFinder) posts() {
	s := f.s
	for _, tool := range s.PostsTools {
		if s.visiblePostsTool(tool.ID, f.person) == nil || !f.searches(tool.ProjectID) {
			continue
		}
		for _, post := range tool.Posts {
			if post.Draft || post.Trashed || s.isAClient(f.person.ID) && !post.ShownToClients {
				continue
			}
			hit := searchItem{kind: "post", id: post.ID, title: post.Title, projectID: tool.ProjectID, toolID: tool.ID,
				updatedAt: lastChange(post.CreatedAt, post.UpdatedAt), archived: post.Archived, appURL: s.postURL(tool, post), apiPath: "posts/" + post.ID}
			if f.filter.includes("posts") {
				f.item(hit, plainText(post.Body))
			}
			f.comments("posts", post.Comments, hit)
		}
	}
}

// todos finds lists, groups and to-dos out of the trash. What stands in a
// list in the archive is found, and marked as archived with it.
func (f *searchFinder) todos() {
	s := f.s
	for _, tool := range s.readableTodosTools() {
		if !f.searches(tool.ProjectID) {
			continue
		}
		for _, list := range tool.Lists {
			if list.Trashed || !s.showsTheList(list, f.person) {
				continue
			}
			listHit := searchItem{kind: searchHitTodoList, id: list.ID, title: list.Title, projectID: tool.ProjectID, toolID: tool.ID,
				updatedAt: list.ArchivedAt, archived: list.Archived, appURL: s.todoListURL(tool, list), apiPath: "todo-lists/" + list.ID}
			if f.filter.includes("todos") {
				f.item(listHit, plainText(list.Description))
				for _, group := range list.Groups {
					if !group.Trashed {
						f.item(searchItem{kind: searchHitTodoGroup, id: group.ID, title: group.Title, projectID: tool.ProjectID, toolID: tool.ID,
							archived: list.Archived, appURL: s.todosToolURL(tool) + "/items/" + group.ID}, "")
					}
				}
			}
			f.comments("todos", list.Comments, listHit)
			for _, todo := range list.Todos {
				if todo.Trashed || todo.GroupID != "" && groupOf(list, todo.GroupID) == nil {
					continue
				}
				hit := searchItem{kind: "todo", id: todo.ID, title: todo.Title, projectID: tool.ProjectID, toolID: tool.ID,
					updatedAt: todo.CreatedAt, archived: list.Archived, appURL: s.todoURL(tool, todo), apiPath: "todos/" + todo.ID}
				if f.filter.includes("todos") {
					f.item(hit, plainText(todo.Notes))
				}
				// A comment is marked by its own item's archive, and a to-do is never in one.
				parent := hit
				parent.archived = false
				f.comments("todos", todo.Comments, parent)
			}
		}
	}
}

// entriesOf finds the folders, the published documents and the uploads of a
// tool with their comments; within a scope only the documents and uploads
// there. What stands in the archive or below an archived folder is found and
// marked.
func (f *searchFinder) entriesOf(tool *FilesTool) {
	s := f.s
	if !f.searches(tool.ProjectID) {
		return
	}
	client := s.isAClient(f.person.ID)
	inScope := func(entryFolderID string) bool {
		if folderIsInTheTrash(tool, entryFolderID) || client && !shownToClients(tool, entryFolderID) {
			return false
		}
		return f.filter.folderID == "" || folderChainHolds(tool, entryFolderID, func(folder *Folder) bool { return folder.ID == f.filter.folderID })
	}
	files := f.filter.includes("files")
	withComments := !f.filter.withinFiles

	if files && !f.filter.withinFiles {
		for _, folder := range tool.Folders {
			if folderIsInTheTrash(tool, folder.ID) || client && !shownToClients(tool, folder.ID) {
				continue
			}
			f.item(searchItem{kind: "folder", id: folder.ID, title: folder.Title, projectID: tool.ProjectID, toolID: tool.ID,
				updatedAt: folder.updatedAt(), archived: folderIsOutOfUse(tool, folder.ID),
				appURL: s.filesToolURL(tool) + "/folders/" + folder.ID, apiPath: "files-folders/" + folder.ID}, "")
		}
	}
	for _, document := range tool.Documents {
		if document.Draft || document.Trashed || !inScope(document.FolderID) {
			continue
		}
		hit := searchItem{kind: "document", id: document.ID, title: document.Title, projectID: tool.ProjectID, toolID: tool.ID,
			updatedAt: document.UpdatedAt, archived: document.Archived || folderIsOutOfUse(tool, document.FolderID),
			appURL: s.documentURL(tool, document), apiPath: "documents/" + document.ID}
		// A comment is marked by the archive of its item alone, not by that of a folder above.
		if withComments {
			parent := hit
			parent.archived = document.Archived
			f.comments("files", document.Comments, parent)
		}
		if !files {
			continue
		}
		hit.inTitle = f.holds(document.Title)
		sections := sectionsOf(document.Body)
		found := slices.IndexFunc(sections, func(section searchSection) bool { return f.holds(section.text) })
		if !hit.inTitle && found < 0 {
			continue
		}
		excerptOf := plainText(document.Body)
		if found >= 0 && sections[found].anchor != "" {
			hit.section = map[string]any{"heading": sections[found].heading, "anchor": sections[found].anchor}
			excerptOf, hit.appURL = sections[found].excerpt, hit.appURL+"#"+sections[found].anchor
		}
		hit.excerpt = excerpt(excerptOf, f.needle)
		f.hits = append(f.hits, s.searchHitOf(hit))
	}
	for _, upload := range tool.Uploads {
		if upload.Trashed || !inScope(upload.FolderID) {
			continue
		}
		hit := searchItem{kind: "upload", id: upload.ID, title: upload.Title, projectID: tool.ProjectID, toolID: tool.ID,
			updatedAt: upload.UpdatedAt, archived: upload.Archived || folderIsOutOfUse(tool, upload.FolderID),
			appURL: s.filesUploadURL(tool, upload), apiPath: "files-uploads/" + upload.ID}
		if withComments {
			parent := hit
			parent.archived = upload.Archived
			f.comments("files", upload.Comments, parent)
		}
		if !files {
			continue
		}
		filename := ""
		if file := s.storedFileOf(tool.ProjectID, upload.BlobID); file != nil {
			filename = file.Filename
		}
		text := plainText(upload.Description)
		hit.inTitle = f.holds(upload.Title)
		if !hit.inTitle && !f.holds(text+" "+filename) {
			continue
		}
		// An upload without a description shows the name of its file.
		if text == "" {
			text = filename
		}
		hit.excerpt = excerpt(text, f.needle)
		f.hits = append(f.hits, s.searchHitOf(hit))
	}
}

// events finds an event out of the trash once, a series included.
func (f *searchFinder) events() {
	s := f.s
	for _, tool := range s.Calendars {
		if s.visibleCalendar(tool.ID, f.person) == nil || !f.searches(tool.ProjectID) {
			continue
		}
		for _, event := range tool.Events {
			if !s.seesEvent(tool, event, f.person) {
				continue
			}
			hit := searchItem{kind: "event", id: event.ID, title: event.Title, projectID: tool.ProjectID, toolID: tool.ID,
				updatedAt: event.CreatedAt, appURL: s.eventURL(tool, event), apiPath: "events/" + event.ID}
			if f.filter.includes("calendar") {
				f.item(hit, plainText(event.Description))
			}
			f.comments("calendar", event.Comments, hit)
		}
	}
}

// chats finds the lines not taken back of the chats of projects and of the
// person's own private conversations. A conversation belongs to no project,
// so a search within one leaves them out.
func (f *searchFinder) chats() {
	s := f.s
	for _, chat := range s.Chats {
		var line searchItem
		if chat.isConversation() {
			if !f.filter.includes("conversation") || f.filter.projectID != "" || !slices.Contains(chat.Partners, f.person.ID) {
				continue
			}
			line = searchItem{kind: "conversation", title: s.otherPartnerOf(chat, f.person), appURL: s.URL + "/messages/" + chat.ID}
		} else {
			if !f.filter.includes("chat") || s.visibleChat(chat.ID, f.person) == nil || !f.searches(chat.ProjectID) {
				continue
			}
			line = searchItem{kind: "chat", title: chat.Name, projectID: chat.ProjectID, toolID: chat.ID,
				appURL: s.URL + "/projects/" + chat.ProjectID + "/chats/" + chat.ID}
		}
		line.targetKind, line.targetID, line.apiPath = line.kind, chat.ID, "chats/"+chat.ID+"/messages"
		for _, message := range chat.transcript() {
			hit := line
			hit.id, hit.updatedAt, hit.appURL = message.ID, message.CreatedAt, line.appURL+"#chat-message-"+message.ID
			f.line(hit, message.BodyText)
		}
	}
}

// otherPartnerOf names a conversation after the first other person in it by
// name.
func (s *Server) otherPartnerOf(chat *Chat, person *Person) string {
	var names []string
	for _, id := range chat.Partners {
		if id != person.ID && s.People[id] != nil {
			names = append(names, s.People[id].Name)
		}
	}
	slices.Sort(names)
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// lastChange is when something was last written: when it was made, until it
// is changed.
func lastChange(created, updated time.Time) time.Time {
	if updated.IsZero() {
		return created
	}
	return updated
}

// sectionsOf splits a body at its headings: the text before the first one,
// then each heading with what follows it up to the next.
func sectionsOf(body string) []searchSection {
	matches := headingForm.FindAllStringSubmatchIndex(body, -1)
	headings := make([]string, len(matches))
	for i, match := range matches {
		headings[i] = plainText(body[match[2]:match[3]])
	}
	anchors := richtext.HeadingAnchors(headings)

	sections := []searchSection{}
	start := len(body)
	if len(matches) > 0 {
		start = matches[0][0]
	}
	if before := plainText(body[:start]); before != "" || len(matches) == 0 {
		sections = append(sections, searchSection{text: before, excerpt: before})
	}
	for i, match := range matches {
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		text := plainText(body[match[0]:end])
		sections = append(sections, searchSection{heading: headings[i], anchor: anchors[i], text: text,
			excerpt: strings.TrimLeft(strings.TrimPrefix(text, headings[i]), "\n ")})
	}
	return sections
}

// plainText is a piece of HTML as words, without markup.
func plainText(markup string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagForm.ReplaceAllString(markup, " "))), " ")
}

// excerpt is the text around the first place the term stands, or its start
// where the term stands only in the title.
func excerpt(text, needle string) string {
	runes := []rune(text)
	start := 0
	if at := strings.Index(strings.ToLower(text), needle); at > 0 {
		start = min(len(runes), max(0, len([]rune(strings.ToLower(text)[:at]))-searchExcerptLength/4))
	}
	end := min(len(runes), start+searchExcerptLength)
	return string(runes[start:end])
}
