package fakeapi

import (
	"cmp"
	"encoding/base64"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// What the server keeps of a post, and how much of a page of them it serves.
const (
	maxPostBodyLength  = 100000
	postLeadLength     = 400
	postsOnAPage       = 50
	mostPostsOnAPage   = 100
	postsCursorLayout  = "2006-01-02T15:04:05.000000-07:00"
	postsCursorPattern = `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`
)

// Audiences a new post can reach, as the API names them.
var postAudiences = []string{"everybody", "chosen", "nobody"}

// postsLimit is what the server reads as the size of a page: digits alone.
var postsLimit = regexp.MustCompile(`^[0-9]{1,18}$`)

var postsCursorPostID = regexp.MustCompile(postsCursorPattern)

// PostsTool is a Posts tool of a project, with its posts in no particular
// order: a list orders them itself.
type PostsTool struct {
	ID        string
	ProjectID string
	Title     string

	// Archived is a tool whose status is no longer active: the project names
	// it among its tools no more, and at its own address it is read and
	// written as before.
	Archived bool

	// ShownToClients lets the clients of the project into the tool at all.
	ShownToClients bool

	Posts []*Post
}

// PostCategory is a kind of post a project offers; every Posts tool of the
// project shares the list.
type PostCategory struct {
	ID        string
	ProjectID string
	Emoji     string
	Name      string
}

// Post is one post of a tool. A draft is at no address, its author included,
// and a post in the trash answers like one that never was, except that
// archiving it says where it is.
type Post struct {
	ID         string
	Title      string
	Body       string // the HTML as stored
	CategoryID string // empty for none
	Draft      bool
	Trashed    bool

	// Archived is a post off the list and out of the count: readable with its
	// comments, and closed to every write but going into the trash.
	Archived     bool
	ArchivedAt   time.Time // the zero moment is a post put away before Minka noted one
	ArchivedByID string

	// PinnedAt puts the post at the top of its list, the one pinned last
	// first. Archiving leaves it as it is.
	PinnedAt *time.Time

	// ShownToClients says whether the clients of the project see the post.
	// Neither the API nor the web sets it: a post written there is shown to no
	// client.
	ShownToClients bool

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatorID string

	// SubscriberIDs are the people the post tells about itself, the writer
	// first.
	SubscriberIDs []string
	Comments      []*Comment
}

func (s *Server) registerPostsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/posts", s.authenticated(s.postsTools))
	mux.HandleFunc("GET /api/v1/projects/{projectId}/post-categories", s.authenticated(s.postCategories))
	mux.HandleFunc("GET /api/v1/posts-tools/{toolId}", s.authenticated(s.postsTool))
	mux.HandleFunc("GET /api/v1/posts-tools/{toolId}/posts", s.authenticated(s.postsPage))
	mux.HandleFunc("POST /api/v1/posts-tools/{toolId}/posts", s.authenticated(s.addPost))
	mux.HandleFunc("GET /api/v1/posts-tools/{toolId}/archive", s.authenticated(s.archivedPosts))
	mux.HandleFunc("GET /api/v1/posts/{postId}", s.authenticated(s.post))
	mux.HandleFunc("PATCH /api/v1/posts/{postId}", s.authenticated(s.updatePost))
	mux.HandleFunc("DELETE /api/v1/posts/{postId}", s.authenticated(s.removePost))
	mux.HandleFunc("POST /api/v1/posts/{postId}/pin", s.authenticated(s.pinPost))
	mux.HandleFunc("POST /api/v1/posts/{postId}/unpin", s.authenticated(s.unpinPost))
	mux.HandleFunc("POST /api/v1/posts/{postId}/archive", s.authenticated(s.archivePost))
	mux.HandleFunc("POST /api/v1/posts/{postId}/unarchive", s.authenticated(s.unarchivePost))

	s.registerCommentRoutes(mux, "posts/{postId}")
}

func (s *Server) postsTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	tools := []any{}
	for _, tool := range s.PostsTools {
		if tool.ProjectID == project.ID && !tool.Archived && (tool.ShownToClients || !s.isAClient(person.ID)) {
			tools = append(tools, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.postsToolURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, tools)
}

// postsTool reads a tool by its identifier alone, with the project it stands
// in. A tool no longer in use is read as well.
func (s *Server) postsTool(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visiblePostsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.toolResource(tool.ID, tool.ProjectID, tool.Title, s.postsToolURL(tool)))
}

// postCategories lists the kinds of post of a project in the order the
// members put them.
func (s *Server) postCategories(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	categories := []any{}
	for _, category := range s.PostCategories {
		if category.ProjectID == project.ID {
			categories = append(categories, map[string]any{"id": category.ID, "emoji": category.Emoji, "name": category.Name})
		}
	}
	writeJSON(w, http.StatusOK, categories)
}

// postsPage is one page of the posts of a tool that went out, the pinned ones
// first, the one pinned last on top, then the newest. The page after it starts
// behind the values of its last post, not behind the post, and next is null on
// the last page.
func (s *Server) postsPage(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visiblePostsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	limit := postsOnAPage
	if value := r.URL.Query().Get("limit"); value != "" {
		number, err := strconv.Atoi(value)
		if !postsLimit.MatchString(value) || err != nil || number < 1 {
			unprocessable(w, "postsLimitNotANumber", "The number of posts asked for has to be a whole number of at least one.")
			return
		}
		limit = min(number, mostPostsOnAPage)
	}
	var boundary *postsCursor
	if value := r.URL.Query().Get("before"); value != "" {
		cursor, ok := readPostsCursor(value)
		if !ok {
			unprocessable(w, "postsBeforeUnknown", `The page starts where the page before ended, as its "next" names it.`)
			return
		}
		boundary = &cursor
	}

	var listed []*Post
	for _, post := range tool.Posts {
		if post.Draft || post.Trashed || post.Archived {
			continue
		}
		if boundary == nil || boundary.isBefore(post) {
			listed = append(listed, post)
		}
	}
	slices.SortFunc(listed, inTheOrderOfTheList)

	more := len(listed) > limit
	listed = listed[:min(limit, len(listed))]
	posts := []any{}
	for _, post := range listed {
		posts = append(posts, s.postListItemResource(tool, post))
	}
	var next any
	if more {
		next = cursorOf(listed[len(listed)-1]).String()
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts, "next": next})
}

// inTheOrderOfTheList puts the pinned posts first, the one pinned last on top,
// and then the newest; the identifier ends the order so that equal moments read
// the same on every request.
func inTheOrderOfTheList(a, b *Post) int {
	switch {
	case a.PinnedAt != nil && b.PinnedAt == nil:
		return -1
	case a.PinnedAt == nil && b.PinnedAt != nil:
		return 1
	case a.PinnedAt != nil && !a.PinnedAt.Equal(*b.PinnedAt):
		return b.PinnedAt.Compare(*a.PinnedAt)
	case !a.CreatedAt.Equal(b.CreatedAt):
		return b.CreatedAt.Compare(a.CreatedAt)
	}
	return cmp.Compare(b.ID, a.ID)
}

// postsCursor is the place a page ends at: the values of its last post.
type postsCursor struct {
	pinnedAt  *time.Time
	createdAt time.Time
	postID    string
}

func cursorOf(post *Post) postsCursor {
	return postsCursor{pinnedAt: post.PinnedAt, createdAt: post.CreatedAt, postID: post.ID}
}

func (c postsCursor) String() string {
	pinned := ""
	if c.pinnedAt != nil {
		pinned = c.pinnedAt.Format(postsCursorLayout)
	}
	text := pinned + "|" + c.createdAt.Format(postsCursorLayout) + "|" + c.postID
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

// readPostsCursor reads a place the way the server reads one: three parts,
// the moments written exactly as the server writes them, the last one an
// identifier. Anything else is no place.
func readPostsCursor(value string) (postsCursor, bool) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(value, "="))
	if err != nil {
		return postsCursor{}, false
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != 3 || !postsCursorPostID.MatchString(parts[2]) {
		return postsCursor{}, false
	}
	cursor := postsCursor{postID: parts[2]}
	if parts[0] != "" {
		pinnedAt, ok := momentOfACursor(parts[0])
		if !ok {
			return postsCursor{}, false
		}
		cursor.pinnedAt = &pinnedAt
	}
	createdAt, ok := momentOfACursor(parts[1])
	if !ok {
		return postsCursor{}, false
	}
	cursor.createdAt = createdAt
	return cursor, true
}

func momentOfACursor(text string) (time.Time, bool) {
	moment, err := time.Parse(postsCursorLayout, text)
	if err != nil || moment.Format(postsCursorLayout) != text {
		return time.Time{}, false
	}
	return moment, true
}

// isBefore says whether a post comes after the place in the list: a pinned
// place is followed by later pinned posts and every unpinned one, an unpinned
// place by later unpinned ones alone.
func (c postsCursor) isBefore(post *Post) bool {
	behind := func(createdAt time.Time, id string) bool {
		return createdAt.Before(c.createdAt) || createdAt.Equal(c.createdAt) && id < c.postID
	}
	if c.pinnedAt == nil {
		return post.PinnedAt == nil && behind(post.CreatedAt, post.ID)
	}
	if post.PinnedAt == nil {
		return true
	}
	return post.PinnedAt.Before(*c.pinnedAt) || post.PinnedAt.Equal(*c.pinnedAt) && behind(post.CreatedAt, post.ID)
}

// archivedPosts lists the archive of a tool, the one archived last first and
// one put away before Minka noted a moment last.
func (s *Server) archivedPosts(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visiblePostsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	var archived []*Post
	for _, post := range tool.Posts {
		if post.Archived && !post.Draft && !post.Trashed {
			archived = append(archived, post)
		}
	}
	slices.SortFunc(archived, func(a, b *Post) int {
		switch {
		case a.ArchivedAt.IsZero() != b.ArchivedAt.IsZero():
			if a.ArchivedAt.IsZero() {
				return 1
			}
			return -1
		case !a.ArchivedAt.Equal(b.ArchivedAt):
			return b.ArchivedAt.Compare(a.ArchivedAt)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	list := []any{}
	for _, post := range archived {
		list = append(list, map[string]any{
			"id": post.ID, "projectId": tool.ProjectID, "title": post.Title, "category": s.categoryBadge(post.CategoryID),
			"archivedAt": timestampOrNil(nilIfZero(post.ArchivedAt)), "archivedBy": s.personReferenceOrNil(post.ArchivedByID),
			"appUrl": s.postURL(tool, post),
		})
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) post(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.postResource(tool, post))
}

// addPost sends a post out, refusing in the order the controller refuses:
// the tool, then what the request says, then what the write meets, and last
// the people it tells, one at a time.
func (s *Server) addPost(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool := s.visiblePostsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok || !validEntryTitle(w, "post", deref(title)) {
		return
	}
	body, ok := stringField(w, fields, "body")
	if !ok {
		return
	}
	stored, ok := storedPostBody(w, deref(body))
	if !ok {
		return
	}
	categoryID, ok := postCategoryField(w, fields)
	if !ok {
		return
	}
	audience, ok := stringField(w, fields, "audience")
	if !ok {
		return
	}
	if !slices.Contains(postAudiences, deref(audience)) {
		unprocessable(w, "postAudienceMissing", `A post says whom it reaches: "everybody", "chosen" with "notifyIds", or "nobody".`)
		return
	}
	notifyIDs, ok := stringListField(w, fields, "notifyIds")
	if !ok {
		return
	}
	// Every identifier is read before the audience is held against the list,
	// and one that is none is answered like somebody outside the project.
	for _, personID := range notifyIDs {
		if !uuidPattern.MatchString(personID) {
			unprocessable(w, "personNotAProjectMember", "Only a member of the project can be told about a post.")
			return
		}
	}
	if deref(audience) == "chosen" && len(notifyIDs) == 0 {
		unprocessable(w, "notifyIdsMissing", `An audience of "chosen" names the people in "notifyIds".`)
		return
	}
	if deref(audience) != "chosen" && len(notifyIDs) > 0 {
		unprocessable(w, "notifyIdsWithoutChosenAudience", `Only an audience of "chosen" takes "notifyIds".`)
		return
	}
	// The write meets the project before the kind: an archived project is the
	// answer whatever kind was named.
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if categoryID != "" && !s.offersTheKind(tool.ProjectID, categoryID) {
		postCategoryUnknown(w)
		return
	}
	post := &Post{
		ID: newID(), Title: phpTrim(deref(title)), Body: stored, CategoryID: categoryID,
		CreatedAt: s.now, UpdatedAt: s.now, CreatorID: person.ID,
	}
	subscribers, ok := s.peopleAPostTells(w, person, tool, post, deref(audience), notifyIDs)
	if !ok {
		return
	}
	post.SubscriberIDs = subscribers
	tool.Posts = append(tool.Posts, post)
	writeJSON(w, http.StatusCreated, s.postResource(tool, post))
}

// peopleAPostTells is the list a new post tells about itself: the writer, and
// then whom the audience names. Of the people chosen, each is met in turn and
// asked first whether they belong to the project and then whether the post is
// shown to them. Whoever is on the list already, the writer from the start, is
// passed over, so a person named twice is told once.
func (s *Server) peopleAPostTells(w http.ResponseWriter, writer *Person, tool *PostsTool, post *Post, audience string, chosen []string) ([]string, bool) {
	project := s.projectWithID(tool.ProjectID)
	told := []string{writer.ID}
	switch audience {
	case "everybody":
		for _, memberID := range project.MemberIDs {
			if !slices.Contains(told, memberID) {
				told = append(told, memberID)
			}
		}
	case "chosen":
		for _, personID := range chosen {
			if slices.Contains(told, personID) {
				continue
			}
			if !slices.Contains(project.MemberIDs, personID) {
				unprocessable(w, "personNotAProjectMember", "Only a member of the project can be told about a post.")
				return nil, false
			}
			told = append(told, personID)
			if s.isAClient(personID) && !(tool.ShownToClients && post.ShownToClients) {
				unprocessable(w, "personCannotSeeIt", "Somebody chosen is a client of the project, and this post is not shown to clients.")
				return nil, false
			}
		}
	}
	return told, true
}

// updatePost writes over the title, the body and the kind of a post. What the
// request leaves out stays, a kind named as null or empty is taken off, and
// the same text again is answered like a write.
func (s *Server) updatePost(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		notFound(w)
		return
	}
	title, ok := stringField(w, fields, "title")
	if !ok {
		return
	}
	newTitle := post.Title
	if title != nil {
		if !validEntryTitle(w, "post", *title) {
			return
		}
		newTitle = phpTrim(*title)
	}
	newBody := post.Body
	if _, given := fields["body"]; given {
		body, ok := stringField(w, fields, "body")
		if !ok {
			return
		}
		if newBody, ok = storedPostBody(w, deref(body)); !ok {
			return
		}
	}
	newCategoryID := post.CategoryID
	if _, given := fields["categoryId"]; given {
		if newCategoryID, ok = postCategoryField(w, fields); !ok {
			return
		}
	}

	if s.refusedAsAClosedPost(w, tool, post) {
		return
	}
	if newCategoryID != "" && !s.offersTheKind(tool.ProjectID, newCategoryID) {
		postCategoryUnknown(w)
		return
	}
	if newTitle != post.Title || newBody != post.Body || newCategoryID != post.CategoryID {
		post.Title, post.Body, post.CategoryID = newTitle, newBody, newCategoryID
		post.UpdatedAt = s.aMomentAfter(post.UpdatedAt)
	}
	writeJSON(w, http.StatusOK, s.postResource(tool, post))
}

// removePost puts a post into the trash, from the list or from the archive,
// and the archive notes go with it.
func (s *Server) removePost(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	post.Trashed, post.Archived, post.ArchivedAt, post.ArchivedByID = true, false, time.Time{}, ""
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// pinPost puts a post at the top of its list; one pinned already is answered
// the same and keeps its place.
func (s *Server) pinPost(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		notFound(w)
		return
	}
	if s.refusedAsAClosedPost(w, tool, post) {
		return
	}
	if post.PinnedAt == nil {
		now := s.aMomentAfterEveryPin(tool)
		post.PinnedAt = &now
	}
	writeJSON(w, http.StatusOK, s.postResource(tool, post))
}

func (s *Server) unpinPost(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		notFound(w)
		return
	}
	if s.refusedAsAClosedPost(w, tool, post) {
		return
	}
	post.PinnedAt = nil
	writeJSON(w, http.StatusOK, s.postResource(tool, post))
}

// archivePost takes a post into the archive of its tool. A post that is not
// found is looked for in the trash, and one there is named as such before
// anything else is asked.
func (s *Server) archivePost(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		if s.trashedPostSeenBy(r.PathValue("postId"), person) {
			unprocessable(w, "postInTrash", "This post is in the trash. It is archived once it is back among the posts.")
			return
		}
		notFound(w)
		return
	}
	if s.refusedAsAClosedPost(w, tool, post) {
		return
	}
	post.Archived, post.ArchivedAt, post.ArchivedByID = true, s.now, person.ID
	writeJSON(w, http.StatusOK, s.postResource(tool, post))
}

// unarchivePost brings a post back among the posts; a pinned one comes back
// pinned.
func (s *Server) unarchivePost(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, post := s.visiblePost(r.PathValue("postId"), person)
	if post == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if !post.Archived {
		unprocessable(w, "postNotArchived", "This post is not archived.")
		return
	}
	post.Archived, post.ArchivedAt, post.ArchivedByID = false, time.Time{}, ""
	writeJSON(w, http.StatusOK, s.postResource(tool, post))
}

// refusedAsAClosedPost says why nothing was written on a post: the project is
// archived, or the post itself is.
func (s *Server) refusedAsAClosedPost(w http.ResponseWriter, tool *PostsTool, post *Post) bool {
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return true
	}
	if post.Archived {
		unprocessable(w, "postArchived", "This post is archived. Nothing is written on it until it is back among the posts.")
		return true
	}
	return false
}

// storedPostBody is a body as the server keeps it, or the refusal it draws. A
// post that goes out says something, in words or in a file.
func storedPostBody(w http.ResponseWriter, value string) (string, bool) {
	stored, ok := storedRichText(w, "postBodyTooLong", "The body of a post is too long.", maxPostBodyLength, value)
	if !ok {
		return "", false
	}
	if saysNothing(stored) {
		unprocessable(w, "postBodyMissing", "A post that goes out says something, in words or in a picture.")
		return "", false
	}
	return stored, true
}

// saysNothing is a body without a word and without anything attached.
func saysNothing(body string) bool {
	return strings.TrimSpace(plainTextOf(body)) == "" && !strings.Contains(body, "<action-text-attachment")
}

func plainTextOf(body string) string {
	return html.UnescapeString(tagForm.ReplaceAllString(body, ""))
}

// postCategoryField reads the kind a post is filed under: empty for none,
// whether the field is left out, null or empty, and refused as unknown where
// it is no identifier at all.
func postCategoryField(w http.ResponseWriter, fields map[string]any) (string, bool) {
	value, ok := stringField(w, fields, "categoryId")
	if !ok {
		return "", false
	}
	if deref(value) == "" {
		return "", true
	}
	if !uuidPattern.MatchString(*value) {
		postCategoryUnknown(w)
		return "", false
	}
	return *value, true
}

func postCategoryUnknown(w http.ResponseWriter) {
	unprocessable(w, "postCategoryUnknown", "This project offers no such kind of post.")
}

func (s *Server) offersTheKind(projectID, categoryID string) bool {
	return slices.ContainsFunc(s.PostCategories, func(category *PostCategory) bool {
		return category.ID == categoryID && category.ProjectID == projectID
	})
}

// categoryBadge is a kind as a post names it: its emoji and its name, or null.
func (s *Server) categoryBadge(categoryID string) any {
	for _, category := range s.PostCategories {
		if category.ID == categoryID {
			return category.Emoji + " " + category.Name
		}
	}
	return nil
}

// aMomentAfter is now, or a moment after the one given where the clock of the
// fake stands still, so that a write is always later than what it writes over.
func (s *Server) aMomentAfter(moment time.Time) time.Time {
	return later(s.now, moment.Add(time.Microsecond))
}

// aMomentAfterEveryPin is the moment a post is pinned at: after every other
// pin of the tool, so that the one pinned last stands on top although the
// clock of the fake stands still.
func (s *Server) aMomentAfterEveryPin(tool *PostsTool) time.Time {
	moment := s.now
	for _, post := range tool.Posts {
		if post.PinnedAt != nil && !post.PinnedAt.Before(moment) {
			moment = post.PinnedAt.Add(time.Microsecond)
		}
	}
	return moment
}

// visiblePostsTool finds a tool in a project the person is a member of, and
// for a client only a tool the clients are let into.
func (s *Server) visiblePostsTool(toolID string, person *Person) *PostsTool {
	for _, tool := range s.PostsTools {
		if tool.ID == toolID && s.visibleProject(tool.ProjectID, person) != nil && (tool.ShownToClients || !s.isAClient(person.ID)) {
			return tool
		}
	}
	return nil
}

// visiblePost finds a post that went out by its identifier alone, as the API
// addresses it, whether it stands in the list or in the archive. A draft and a
// post in the trash are at no address.
func (s *Server) visiblePost(postID string, person *Person) (*PostsTool, *Post) {
	tool, post := s.postWithID(postID, person)
	if post == nil || post.Draft || post.Trashed {
		return nil, nil
	}
	return tool, post
}

// trashedPostSeenBy says whether a post that went out lies in the trash where
// the person asking would see it. A draft there is never named.
func (s *Server) trashedPostSeenBy(postID string, person *Person) bool {
	_, post := s.postWithID(postID, person)
	return post != nil && post.Trashed && !post.Draft
}

func (s *Server) postWithID(postID string, person *Person) (*PostsTool, *Post) {
	if !uuidPattern.MatchString(postID) {
		return nil, nil
	}
	for _, tool := range s.PostsTools {
		for _, post := range tool.Posts {
			if post.ID != postID {
				continue
			}
			if s.visibleProject(tool.ProjectID, person) == nil {
				return nil, nil
			}
			return tool, post
		}
	}
	return nil, nil
}

func (s *Server) postListItemResource(tool *PostsTool, post *Post) map[string]any {
	return map[string]any{
		"id": post.ID, "projectId": tool.ProjectID, "title": post.Title, "lead": postLeadOf(post.Body),
		"category": s.categoryBadge(post.CategoryID), "categoryId": nullable(post.CategoryID),
		"pinned": post.PinnedAt != nil, "commentCount": len(post.Comments),
		"createdAt": timestamp(post.CreatedAt), "creator": s.personReference(post.CreatorID),
		"appUrl": s.postURL(tool, post),
	}
}

func (s *Server) postResource(tool *PostsTool, post *Post) map[string]any {
	return map[string]any{
		"id": post.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"title": post.Title, "body": post.Body, "bodySource": post.Body,
		"attachments": s.attachmentsIn(tool.ProjectID, post.Body),
		"category":    s.categoryBadge(post.CategoryID), "categoryId": nullable(post.CategoryID),
		"archived": post.Archived, "pinned": post.PinnedAt != nil, "commentCount": len(post.Comments),
		"createdAt": timestamp(post.CreatedAt), "editedAt": editedAt(post.CreatedAt, post.UpdatedAt),
		"creator":  s.personReference(post.CreatorID),
		"comments": s.commentResources(tool.ProjectID, post.Comments),
		"appUrl":   s.postURL(tool, post),
	}
}

// postLeadOf is the first words of a body without markup, cut where the count
// runs out.
func postLeadOf(body string) string {
	text := strings.TrimFunc(plainTextOf(body), unicode.IsSpace)
	if utf8.RuneCountInString(text) <= postLeadLength {
		return text
	}
	return strings.TrimFunc(string([]rune(text)[:postLeadLength]), unicode.IsSpace)
}

func (s *Server) postsToolURL(tool *PostsTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/posts/" + tool.ID
}

func (s *Server) postURL(tool *PostsTool, post *Post) string {
	return s.postsToolURL(tool) + "/" + post.ID
}
