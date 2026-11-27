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
	ProjectID string             `json:"projectId"`
	Project   *ProjectReference  `json:"project"`
	Tool      FilesToolReference `json:"tool"`
	Folder    *FolderReference   `json:"folder"`    // nil at the top of the tool
	Parent    *FolderReference   `json:"parent"`    // the folder it stands in; nil at the top of the tool and for a folder there
	UpdatedAt *string            `json:"updatedAt"` // when the folder itself last changed; nil at the top of the tool
	// Archived says that this folder, or one above it, lies in the archive.
	// Its entries are then readable and take no change.
	Archived  bool                  `json:"archived"`
	Path      []FolderReference     `json:"path"` // from the top of the tool down to this folder, this one last
	Folders   []FolderListItem      `json:"folders"`
	Documents []DocumentListItem    `json:"documents"`
	Uploads   []FilesUploadListItem `json:"uploads"`
	// Order is "automatic" or "manual", empty from a Minka that does not say.
	// Arranged by hand, the three kinds stand mixed, in the order of Entries.
	Order   string                `json:"order"`
	IsBook  bool                  `json:"isBook"`
	Book    *FolderReference      `json:"book"`    // the book this place belongs to, itself or one above; nil outside a book
	Entries []FilesEntryReference `json:"entries"` // the folders, documents and uploads above in the order shown
	AppURL  string                `json:"appUrl"`
}

// FilesEntryReference names one entry of a place in the order it is shown.
type FilesEntryReference struct {
	Kind string `json:"kind"` // "folder", "document" or "upload"
	ID   string `json:"id"`
}

// FolderListItem is one folder as the folder above it lists it. What it holds
// is read by asking for the entries of the tool with its ID.
type FolderListItem struct {
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	EntryCount int             `json:"entryCount"` // the folders, documents and uploads directly in it
	IsBook     bool            `json:"isBook"`
	CreatedAt  string          `json:"createdAt"`
	Creator    PersonReference `json:"creator"`
	AppURL     string          `json:"appUrl"`
}

// DocumentListItem is one document as its folder lists it, without the body.
type DocumentListItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Lead  string `json:"lead"` // the first 240 characters of the body without markup, cut mid-word
	// InNavigation is nil from a Minka that does not say; false leaves the
	// document out of the navigation of the book it stands in.
	InNavigation *bool           `json:"inNavigation"`
	CommentCount int             `json:"commentCount"`
	Archived     bool            `json:"archived"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
	Creator      PersonReference `json:"creator"`
	Labels       []Label         `json:"labels"` // the tool's labels first, then the project's
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
	Labels       []Label         `json:"labels"` // the tool's labels first, then the project's
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
	// DecisionBlocks are the decisions blocks of the body in its order, the
	// copies of the lock while the document is locked.
	DecisionBlocks []DecisionBlock `json:"decisionBlocks"`
	// Archived says the document, or a folder above it, lies in the archive:
	// readable, and closed to every write.
	Archived bool `json:"archived"`
	// Locked says the title and the body take no write until somebody of the
	// team unlocks the document.
	Locked        bool                    `json:"locked"`
	LockedAt      *string                 `json:"lockedAt"`      // nil while it is not locked
	LockedBy      *PersonReference        `json:"lockedBy"`      // nil while it is not locked, or where that person is gone
	LockedVersion *LockedVersionReference `json:"lockedVersion"` // nil while it is not locked
	InNavigation  *bool                   `json:"inNavigation"`  // as on DocumentListItem
	Book          *FolderReference        `json:"book"`          // the book it stands in at any depth, nil outside one
	CommentCount  int                     `json:"commentCount"`
	CreatedAt     string                  `json:"createdAt"`
	EditedAt      *string                 `json:"editedAt"` // nil while nobody has written over it
	UpdatedAt     string                  `json:"updatedAt"`
	Creator       PersonReference         `json:"creator"`
	Comments      []Comment               `json:"comments"` // oldest first
	Labels        []Label                 `json:"labels"`   // the tool's labels first, then the project's
	AppURL        string                  `json:"appUrl"`
}

// LockedVersionReference is the version of a document that stands locked.
type LockedVersionReference struct {
	ID     *string `json:"id"` // nil where the text of the version was removed
	Number int     `json:"number"`
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
	Labels            []Label            `json:"labels"` // the tool's labels first, then the project's
	AppURL            string             `json:"appUrl"`
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
//
// InNavigation false leaves the document out of the navigation of the book it
// stands in. It is the team's alone: a client is answered 404 for the whole
// request.
type UpdateDocumentInput struct {
	Title             *string `json:"title,omitempty"`
	Body              *string `json:"body,omitempty"`
	ExpectedUpdatedAt *string `json:"expectedUpdatedAt,omitempty"`
	InNavigation      *bool   `json:"inNavigation,omitempty"`
}

// AddFilesUploadInput puts a stored file into a Docs & Files tool. BlobID
// names a file the writer stored in the tool's project that stands in no
// upload or text yet. A Title left out names the upload after the file, and an
// empty one is refused. Description is HTML and may be empty.
type AddFilesUploadInput struct {
	BlobID      string  `json:"blobId"`
	Title       *string `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	FolderID    string  `json:"folderId,omitempty"` // left out for the top of the tool
}

// UpdateFilesUploadInput changes the title or the description of an upload.
// What it leaves out stays; an empty Description empties it, an empty Title is
// refused. The file of an upload is not among what it can change.
//
// ExpectedUpdatedAt works as it does for a document.
type UpdateFilesUploadInput struct {
	Title             *string `json:"title,omitempty"`
	Description       *string `json:"description,omitempty"`
	ExpectedUpdatedAt *string `json:"expectedUpdatedAt,omitempty"`
}

// FilesTools lists the Docs & Files tools of a project, in the order of the
// project page.
func (c *Client) FilesTools(ctx context.Context, projectID string) (Result[[]FilesTool], error) {
	return request[[]FilesTool](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/files", nil)
}

// FilesTool reads a Docs & Files tool by its identifier alone.
func (c *Client) FilesTool(ctx context.Context, toolID string) (Result[Tool], error) {
	return request[Tool](ctx, c, http.MethodGet, "/files-tools/"+url.PathEscape(toolID), nil)
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

// AddFilesUpload puts a stored file on top of a tool, or of the folder the
// input names. The file cannot go into a second upload.
func (c *Client) AddFilesUpload(ctx context.Context, toolID string, input AddFilesUploadInput) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPost, "/files-tools/"+url.PathEscape(toolID)+"/uploads", input)
}

// UpdateFilesUpload writes over the title or the description of an upload. An
// upload written over since the moment the input names is left as it stands
// and answered with 409 uploadChanged, whose body carries it under "upload".
func (c *Client) UpdateFilesUpload(ctx context.Context, uploadID string, input UpdateFilesUploadInput) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPatch, "/files-uploads/"+url.PathEscape(uploadID), input)
}

// DocumentVersionLine is one version of a document as its history lists it,
// without the text. Number counts from 1; Current marks the newest, which is
// what the document says now.
type DocumentVersionLine struct {
	ID         string           `json:"id"`
	DocumentID string           `json:"documentId"`
	Number     int              `json:"number"`
	Current    bool             `json:"current"`
	Author     *PersonReference `json:"author"` // nil where nothing kept who wrote it
	WrittenAt  string           `json:"writtenAt"`
}

// DocumentVersion is one version of a document as it was written, with the
// files its body names.
type DocumentVersion struct {
	ID          string           `json:"id"`
	DocumentID  string           `json:"documentId"`
	Number      int              `json:"number"`
	Current     bool             `json:"current"`
	Title       string           `json:"title"`
	Body        string           `json:"body"` // the stored HTML
	Attachments []Attachment     `json:"attachments"`
	Author      *PersonReference `json:"author"` // nil where nothing kept who wrote it
	WrittenAt   string           `json:"writtenAt"`
	AppURL      string           `json:"appUrl"`
}

// UploadVersion is one file an upload showed or shows. Current marks the file
// it shows now; Author is who gave it that file.
type UploadVersion struct {
	ID        string          `json:"id"`
	UploadID  string          `json:"uploadId"`
	Number    int             `json:"number"`
	Current   bool            `json:"current"`
	File      Attachment      `json:"file"`
	Author    PersonReference `json:"author"`
	WrittenAt string          `json:"writtenAt"`
}

// AddUploadVersionInput gives an upload another file. BlobID names a file the
// writer stored in the upload's project that stands nowhere yet; an earlier
// file of the same upload stands somewhere. ExpectedUpdatedAt works as it does
// for a change of the upload; left out, Minka expects the upload as the
// request found it.
type AddUploadVersionInput struct {
	BlobID            string  `json:"blobId"`
	ExpectedUpdatedAt *string `json:"expectedUpdatedAt,omitempty"`
}

// DocumentVersions lists the versions of a document, oldest first. They are
// the team's alone: a client of the project is answered 404.
func (c *Client) DocumentVersions(ctx context.Context, documentID string) (Result[[]DocumentVersionLine], error) {
	return request[[]DocumentVersionLine](ctx, c, http.MethodGet, "/documents/"+url.PathEscape(documentID)+"/versions", nil)
}

// DocumentVersionOf reads one version of a document.
func (c *Client) DocumentVersionOf(ctx context.Context, documentID, versionID string) (Result[DocumentVersion], error) {
	return request[DocumentVersion](ctx, c, http.MethodGet, "/documents/"+url.PathEscape(documentID)+"/versions/"+url.PathEscape(versionID), nil)
}

// UploadVersions lists the files an upload was given, oldest first, the
// team's alone as the versions of a document are.
func (c *Client) UploadVersions(ctx context.Context, uploadID string) (Result[[]UploadVersion], error) {
	return request[[]UploadVersion](ctx, c, http.MethodGet, "/files-uploads/"+url.PathEscape(uploadID)+"/versions", nil)
}

// AddUploadVersion gives an upload another file: 201 with the upload, or 200
// with it unchanged where the file is the one it shows already.
func (c *Client) AddUploadVersion(ctx context.Context, uploadID string, input AddUploadVersionInput) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPost, "/files-uploads/"+url.PathEscape(uploadID)+"/versions", input)
}

// ArchivedFilesEntry is one folder, document or upload in the archive of a
// Docs & Files tool, from whichever folder. The entry itself is read at its
// own address.
type ArchivedFilesEntry struct {
	ID         string           `json:"id"`
	ProjectID  string           `json:"projectId"`
	Kind       string           `json:"kind"` // "folder", "document" or "upload"
	Title      string           `json:"title"`
	ArchivedAt string           `json:"archivedAt"`
	ArchivedBy *PersonReference `json:"archivedBy"` // nil where nobody is known
	// ArchivedWithAFolderAbove says that a folder above it lies in the archive
	// too, and only that folder brings it back.
	ArchivedWithAFolderAbove bool   `json:"archivedWithAFolderAbove"`
	AppURL                   string `json:"appUrl"`
}

// CreateFolderInput makes a folder on top of a tool, or of the folder FolderID
// names. VisibleToClients shows it to the clients of the project at once; it
// counts only at the top of the tool and only from the team.
type CreateFolderInput struct {
	Title            string `json:"title"`
	FolderID         string `json:"folderId,omitempty"` // left out for the top of the tool
	VisibleToClients bool   `json:"visibleToClients,omitempty"`
}

// UpdateFolderInput renames a folder, marks it as a book or takes the mark
// away. What it leaves out stays; an empty Title is refused. IsBook is the
// team's alone.
type UpdateFolderInput struct {
	Title  *string `json:"title,omitempty"`
	IsBook *bool   `json:"isBook,omitempty"`
}

// MoveFilesEntryInput names where an entry moves to. FolderID nil is the top
// of the tool and travels as null: a move without the field is refused.
//
// PreviousEntryID and NextEntryID place the entry next to a neighbour in a
// place arranged by hand; where both are named, the previous one decides. A
// neighbour that does not stand in the place counts as none.
//
// ToolID moves the entry into another Docs & Files tool, FolderID then naming
// a folder of that tool. There it lands on top of its kind, and Minka refuses
// neighbours.
type MoveFilesEntryInput struct {
	FolderID        *string `json:"folderId"`
	ToolID          string  `json:"toolId,omitempty"`
	PreviousEntryID *string `json:"previousEntryId,omitempty"`
	NextEntryID     *string `json:"nextEntryId,omitempty"`
}

// ChangeFilesOrderInput says how the top of a tool, or the folder FolderID
// names, orders its entries: "automatic" or "manual".
type ChangeFilesOrderInput struct {
	Order    string `json:"order"`
	FolderID string `json:"folderId,omitempty"` // left out for the top of the tool
}

// Book is the whole tree of a book, or of any folder: its chapters and pages
// in reading order, without bodies or what lies in the archive. The tree of a
// folder holds its uploads too, a book none.
type Book struct {
	ID      string             `json:"id"`
	Title   string             `json:"title"`
	Tool    FilesToolReference `json:"tool"`
	Order   string             `json:"order"` // "automatic" or "manual"
	Entries []BookEntry        `json:"entries"`
	AppURL  string             `json:"appUrl"`
}

// BookEntry is a chapter, which is a folder with entries of its own, a page,
// which is a document, or in the tree of a folder an upload.
type BookEntry struct {
	Kind         string      `json:"kind"` // "chapter", "page" or "upload"
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Order        string      `json:"order,omitempty"`   // a chapter's
	Entries      []BookEntry `json:"entries,omitempty"` // a chapter's
	InNavigation bool        `json:"inNavigation"`      // a page's
	// UpdatedAt is when a page or an upload last changed, empty from a Minka
	// that does not say.
	UpdatedAt string `json:"updatedAt,omitempty"`
	// Version is the number of a page's newest version: every change of its
	// title or body makes a new one. Nil for a client, who reads no versions,
	// and from a Minka that does not say.
	Version     *int        `json:"version,omitempty"`
	File        *Attachment `json:"file,omitempty"`        // an upload's
	DownloadURL string      `json:"downloadUrl,omitempty"` // an upload's: the bytes of its original
	AppURL      string      `json:"appUrl"`
}

// FilesArchive lists the archive of a Docs & Files tool at any depth, the one
// archived last first. Nothing of it is paged.
func (c *Client) FilesArchive(ctx context.Context, toolID string) (Result[[]ArchivedFilesEntry], error) {
	return request[[]ArchivedFilesEntry](ctx, c, http.MethodGet, "/files-tools/"+url.PathEscape(toolID)+"/archive", nil)
}

// CreateFolder answers with the new folder in the shape FilesEntriesOf reads
// one.
func (c *Client) CreateFolder(ctx context.Context, toolID string, input CreateFolderInput) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPost, "/files-tools/"+url.PathEscape(toolID)+"/folders", input)
}

func (c *Client) UpdateFolder(ctx context.Context, folderID string, input UpdateFolderInput) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPatch, folderPath(folderID), input)
}

// FilesFolder reads one folder by its identifier alone, in the shape
// FilesEntriesOf reads it, with the folder it stands in.
func (c *Client) FilesFolder(ctx context.Context, folderID string) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodGet, folderPath(folderID), nil)
}

// TreeOf reads any folder as one tree in the shape of a book, starting at the
// folder, with its uploads.
func (c *Client) TreeOf(ctx context.Context, folderID string) (Result[Book], error) {
	return request[Book](ctx, c, http.MethodGet, folderPath(folderID)+"/tree", nil)
}

// BookOf reads the book a folder belongs to, the folder itself or one above
// it, as one tree. A folder in no book is 404.
func (c *Client) BookOf(ctx context.Context, folderID string) (Result[Book], error) {
	return request[Book](ctx, c, http.MethodGet, folderPath(folderID)+"/book", nil)
}

// ChangeFilesOrder answers with the place in the shape FilesEntriesOf reads
// it, also where it ordered so already.
func (c *Client) ChangeFilesOrder(ctx context.Context, toolID string, input ChangeFilesOrderInput) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPut, "/files-tools/"+url.PathEscape(toolID)+"/order", input)
}

// RestoreDocumentVersion writes what an earlier version said as the newest
// version of its document. One that says what the document says already is
// answered the same and adds no version.
func (c *Client) RestoreDocumentVersion(ctx context.Context, documentID, versionID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/versions/"+url.PathEscape(versionID)+"/restore", nil)
}

// The writes that put a tool in order, for each kind of entry: into the
// trash and back, into the archive and back, and to another place in the
// same tool. A folder answers with what stands in it.

func (c *Client) DeleteDocument(ctx context.Context, documentID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, documentPath(documentID), nil)
	return err
}

func (c *Client) RestoreDocument(ctx context.Context, documentID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/restore", nil)
}

func (c *Client) ArchiveDocument(ctx context.Context, documentID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/archive", nil)
}

func (c *Client) UnarchiveDocument(ctx context.Context, documentID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/unarchive", nil)
}

func (c *Client) LockDocument(ctx context.Context, documentID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/lock", nil)
}

func (c *Client) UnlockDocument(ctx context.Context, documentID string) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/unlock", nil)
}

func (c *Client) MoveDocument(ctx context.Context, documentID string, input MoveFilesEntryInput) (Result[Document], error) {
	return request[Document](ctx, c, http.MethodPost, documentPath(documentID)+"/moves", input)
}

func (c *Client) DeleteFilesUpload(ctx context.Context, uploadID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, filesUploadPath(uploadID), nil)
	return err
}

func (c *Client) RestoreFilesUpload(ctx context.Context, uploadID string) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPost, filesUploadPath(uploadID)+"/restore", nil)
}

func (c *Client) ArchiveFilesUpload(ctx context.Context, uploadID string) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPost, filesUploadPath(uploadID)+"/archive", nil)
}

func (c *Client) UnarchiveFilesUpload(ctx context.Context, uploadID string) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPost, filesUploadPath(uploadID)+"/unarchive", nil)
}

func (c *Client) MoveFilesUpload(ctx context.Context, uploadID string, input MoveFilesEntryInput) (Result[FilesUpload], error) {
	return request[FilesUpload](ctx, c, http.MethodPost, filesUploadPath(uploadID)+"/moves", input)
}

func (c *Client) DeleteFolder(ctx context.Context, folderID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, folderPath(folderID), nil)
	return err
}

func (c *Client) RestoreFolder(ctx context.Context, folderID string) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPost, folderPath(folderID)+"/restore", nil)
}

func (c *Client) ArchiveFolder(ctx context.Context, folderID string) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPost, folderPath(folderID)+"/archive", nil)
}

func (c *Client) UnarchiveFolder(ctx context.Context, folderID string) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPost, folderPath(folderID)+"/unarchive", nil)
}

func (c *Client) MoveFolder(ctx context.Context, folderID string, input MoveFilesEntryInput) (Result[FilesEntries], error) {
	return request[FilesEntries](ctx, c, http.MethodPost, folderPath(folderID)+"/moves", input)
}

func documentPath(documentID string) string {
	return "/documents/" + url.PathEscape(documentID)
}

func filesUploadPath(uploadID string) string {
	return "/files-uploads/" + url.PathEscape(uploadID)
}

func folderPath(folderID string) string {
	return "/files-folders/" + url.PathEscape(folderID)
}
