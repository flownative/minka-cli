package api

import (
	"context"
	"net/http"
	"net/url"
)

// The Docs & Files tool of a project: its folders, the documents in them and
// the uploads, which are files standing on their own.

// FilesTool is one Docs & Files tool of a project.
type FilesTool struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

// FilesToolReference names the tool an entry stands in.
type FilesToolReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// FolderReference is a folder by its title: where an entry stands, or a step
// on the way down to it.
type FolderReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// FilesEntries is what stands at the top of a tool or in one of its folders,
// one folder deep. Nothing of it is paged and nothing is cut.
type FilesEntries struct {
	Tool   FilesToolReference `json:"tool"`
	Folder *FolderReference   `json:"folder"` // nil at the top of the tool
	// Archived says that this folder, or one above it, lies in the archive.
	// Its entries are then readable and take no change.
	Archived  bool                  `json:"archived"`
	Path      []FolderReference     `json:"path"` // from the top of the tool down to this folder, this one last
	Folders   []FolderListItem      `json:"folders"`
	Documents []DocumentListItem    `json:"documents"`
	Uploads   []FilesUploadListItem `json:"uploads"`
	AppURL    string                `json:"appUrl"`
}

// FolderListItem is one folder as the folder above it lists it. What it holds
// is read by asking for the entries of the tool with its ID.
type FolderListItem struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	EntryCount int             `json:"entryCount"` // the folders, documents and uploads directly in it
	CreatedAt  string          `json:"createdAt"`
	Creator    PersonReference `json:"creator"`
	AppURL     string          `json:"appUrl"`
}

// DocumentListItem is one document as its folder lists it, without the body.
type DocumentListItem struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Lead         string          `json:"lead"` // the first 240 characters of the body without markup, cut mid-word
	CommentCount int             `json:"commentCount"`
	Archived     bool            `json:"archived"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
	Creator      PersonReference `json:"creator"`
	AppURL       string          `json:"appUrl"`
}

// FilesUploadListItem is one upload as its folder lists it: its title and the
// file it stands for.
type FilesUploadListItem struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	File         Attachment      `json:"file"`
	CommentCount int             `json:"commentCount"`
	Archived     bool            `json:"archived"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
	Creator      PersonReference `json:"creator"`
	AppURL       string          `json:"appUrl"`
}

// Document is one published document with everything said under it. UpdatedAt
// is the moment it was last written and the value a guarded write sends back;
// it is kept to the whole second.
type Document struct {
	ID         string             `json:"id"`
	ProjectID  string             `json:"projectId"`
	ToolID     string             `json:"toolId"`
	Project    ProjectReference   `json:"project"`
	Tool       FilesToolReference `json:"tool"`
	Folder     *FolderReference   `json:"folder"` // nil at the top of the tool
	Path       []FolderReference  `json:"path"`
	Title      string             `json:"title"`
	Body       string             `json:"body"`       // the stored HTML, empty for a document that is only a title
	BodySource string             `json:"bodySource"` // the body as the editor opens it
	// Attachments are the files hanging in the body, and in no comment.
	Attachments []Attachment `json:"attachments"`
	// Archived says the document, or a folder above it, lies in the archive:
	// readable, and closed to every write.
	Archived     bool            `json:"archived"`
	CommentCount int             `json:"commentCount"`
	CreatedAt    string          `json:"createdAt"`
	EditedAt     *string         `json:"editedAt"` // nil while nobody has written over it
	UpdatedAt    string          `json:"updatedAt"`
	Creator      PersonReference `json:"creator"`
	Comments     []Comment       `json:"comments"` // oldest first
	AppURL       string          `json:"appUrl"`
}

// FilesUpload is a file standing on its own in a Docs & Files tool: what was
// said about it and everything said under it. File.ID is what a download asks
// for.
type FilesUpload struct {
	ID                string             `json:"id"`
	ProjectID         string             `json:"projectId"`
	ToolID            string             `json:"toolId"`
	Project           ProjectReference   `json:"project"`
	Tool              FilesToolReference `json:"tool"`
	Folder            *FolderReference   `json:"folder"`
	Path              []FolderReference  `json:"path"`
	Title             string             `json:"title"`
	File              Attachment         `json:"file"`
	Description       string             `json:"description"`
	DescriptionSource string             `json:"descriptionSource"`
	Attachments       []Attachment       `json:"attachments"`
	Archived          bool               `json:"archived"`
	CommentCount      int                `json:"commentCount"`
	CreatedAt         string             `json:"createdAt"`
	EditedAt          *string            `json:"editedAt"`
	UpdatedAt         string             `json:"updatedAt"`
	Creator           PersonReference    `json:"creator"`
	Comments          []Comment          `json:"comments"`
	AppURL            string             `json:"appUrl"`
}

// EntryKind says which kind of entry of a Docs & Files tool an identifier
// names. The two have addresses of their own, and the identifier of one is
// nothing at the address of the other.
type EntryKind string

const (
	EntryDocument EntryKind = "document"
	EntryUpload   EntryKind = "upload"
)

// Entry names one entry of a Docs & Files tool, which is what the comments
// under it hang on.
type Entry struct {
	Kind EntryKind
	ID   string
}

func (e Entry) path() string {
	if e.Kind == EntryUpload {
		return "/files-uploads/" + url.PathEscape(e.ID)
	}
	return "/documents/" + url.PathEscape(e.ID)
}

// CreateDocumentInput writes a published document. Body is HTML and may be
// empty; a body that carries no element the server serves is stored as the
// characters it consists of, without a word about it. Audience is required:
// "everybody", "chosen" with NotifyIDs, or "nobody".
type CreateDocumentInput struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Audience  string   `json:"audience"`
	FolderID  string   `json:"folderId,omitempty"` // left out for the top of the tool
	NotifyIDs []string `json:"notifyIds,omitempty"`
}

// UpdateDocumentInput changes a document. What it leaves out stays as it is;
// an empty Body empties the body, an empty Title is refused.
//
// ExpectedUpdatedAt is the updatedAt the writer read: the document is written
// over only where it still stands at that moment. Left out there is no
// expectation and the last write wins. Null would mean the same here — unlike
// the notes of a card, where it means that no notes exist yet — so a write
// without an expectation leaves the field out rather than sending null.
type UpdateDocumentInput struct {
	Title             *string `json:"title,omitempty"`
	Body              *string `json:"body,omitempty"`
	ExpectedUpdatedAt *string `json:"expectedUpdatedAt,omitempty"`
}

// FilesTools lists the Docs & Files tools of a project, in the order of the
// project page.
func (c *Client) FilesTools(ctx context.Context, projectID string) (Result[[]FilesTool], error) {
	return request[[]FilesTool](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/files", nil)
}

// FilesEntriesOf reads one folder of a tool, or the top of it where the
// folder is empty. It reaches one folder deep: what stands in a folder below
// is read by asking again with that folder.
func (c *Client) FilesEntriesOf(ctx context.Context, toolID, folderID string) (Result[FilesEntries], error) {
	path := "/files-tools/" + url.PathEscape(toolID) + "/entries"
	if folderID != "" {
		path += "?folderId=" + url.QueryEscape(folderID)
	}
	return request[FilesEntries](ctx, c, http.MethodGet, path, nil)
}

// Document reads one published document. A draft is at no address, its author
// included.
func (c *Client) Document(ctx context.Context, documentID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodGet, "/documents/"+url.PathEscape(documentID), nil)
}

func (c *Client) CreateDocument(ctx context.Context, toolID string, input CreateDocumentInput) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, "/files-tools/"+url.PathEscape(toolID)+"/documents", input)
}

// UpdateDocument writes over a document. A document written over since the
// moment the input names is left as it stands and answered with 409
// documentChanged, whose body carries it under "document".
func (c *Client) UpdateDocument(ctx context.Context, documentID string, input UpdateDocumentInput) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPatch, "/documents/"+url.PathEscape(documentID), input)
}

// FilesUploadOf reads one upload of a Docs & Files tool.
func (c *Client) FilesUploadOf(ctx context.Context, uploadID string) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodGet, "/files-uploads/"+url.PathEscape(uploadID), nil)
}

func (c *Client) WriteEntryComment(ctx context.Context, entry Entry, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost, entry.path()+"/comments", map[string]string{"body": body})
}

// EditEntryComment changes a comment of the person of the token.
func (c *Client) EditEntryComment(ctx context.Context, entry Entry, commentID, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPatch, entry.path()+"/comments/"+url.PathEscape(commentID), map[string]string{"body": body})
}

// DeleteEntryComment removes a comment of the person of the token. A removed
// comment leaves nothing behind, so removing it again answers 404.
func (c *Client) DeleteEntryComment(ctx context.Context, entry Entry, commentID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, entry.path()+"/comments/"+url.PathEscape(commentID), nil)
	return err
}

// ReactToEntryComment leaves a reaction on a comment, 201 for a new one and
// 200 where the same one of yours stood already.
func (c *Client) ReactToEntryComment(ctx context.Context, entry Entry, commentID, content string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost,
		entry.path()+"/comments/"+url.PathEscape(commentID)+"/reactions", map[string]string{"content": content})
}

// UnreactFromEntryComment takes a reaction off a comment, the way it comes off
// a comment on a card.
func (c *Client) UnreactFromEntryComment(ctx context.Context, entry Entry, commentID, content string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete,
		entry.path()+"/comments/"+url.PathEscape(commentID)+"/reactions?content="+url.QueryEscape(content), nil)
	return err
}
