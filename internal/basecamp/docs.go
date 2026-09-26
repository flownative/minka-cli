package basecamp

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// DocsCounts says what became of the Docs & Files of a run.
type DocsCounts struct {
	// FoldersByStatus, DocumentsByStatus and UploadsByStatus count the entries of
	// the bundle: active, archived, trashed. An entry carries its own state and
	// never the state of the folder it stands in.
	FoldersByStatus   map[string]int `json:"foldersByStatus"`
	DocumentsByStatus map[string]int `json:"documentsByStatus"`
	UploadsByStatus   map[string]int `json:"uploadsByStatus"`
	// DocumentsWithoutWords counts the documents that came with a title alone.
	DocumentsWithoutWords int `json:"documentsWithoutWords"`
	// TitlesFromTheFileName counts the uploads the source calls nothing, whose
	// title is therefore the name of the file. NamesMadeIntoATitle counts those
	// whose title had to be folded into one line or cut short to make one Minka
	// takes, and WithoutAName those with neither a title nor a name, which carry
	// the title a run gives them.
	TitlesFromTheFileName int `json:"titlesFromTheFileName"`
	NamesMadeIntoATitle   int `json:"namesMadeIntoATitle"`
	WithoutAName          int `json:"withoutAName"`
	// WithoutAStoredFile are the uploads left out because Basecamp names no
	// stored file for them, by sourceId: what lies in a cloud is no file a run
	// can fetch, so the upload and everything pointing at it stay out.
	WithoutAStoredFile []string `json:"withoutAStoredFile"`
	// CommentsWithoutTheirUpload and BoostsWithoutTheirUpload count what fell
	// with such an upload.
	CommentsWithoutTheirUpload int `json:"commentsWithoutTheirUpload"`
	BoostsWithoutTheirUpload   int `json:"boostsWithoutTheirUpload"`
	// OnlyAFileNoRunWillBring are the records that lost a file no run will bring,
	// by sourceId: a document, or the description of an upload, whose whole text
	// was such a file, and an upload whose own file is one. The two end
	// differently, because the one field is optional and the other is not: the
	// text comes out and the document arrives without words, while the upload
	// cannot be written at all and Minka asks for its file again on every run.
	OnlyAFileNoRunWillBring []string `json:"onlyAFileNoRunWillBring"`
	// FoldersThatSaidNothingOfWhatIsInThem counts the folders the source has put
	// away that would not say what stands in them. What is in use below such a
	// folder reaches no bundle and no later run finds it either, and nothing can
	// count it: the number of folders is all a run can say about the hole.
	FoldersThatSaidNothingOfWhatIsInThem int `json:"foldersThatSaidNothingOfWhatIsInThem"`
	// LongerThanMinkaTakes counts the documents and the descriptions of uploads
	// that are longer than Minka holds. Minka passes such a record over on every
	// run, with everything hanging on it, and the text is not cut short.
	LongerThanMinkaTakes int `json:"longerThanMinkaTakes"`
	// LeftOut counts what a bundle does not carry of the Docs & Files tools, per
	// kind of record, and PlacesLeftOut names the folders and tools that took
	// entries with them.
	LeftOut       DocsLeftOut        `json:"leftOut"`
	PlacesLeftOut []DocsPlaceLeftOut `json:"placesLeftOut"`
}

// DocsLeftOut counts what a bundle does not carry of the Docs & Files tools, per
// kind of record: an entry whose status a bundle has no word for, one that hangs
// on a tool the run does not extract, and whatever stood in one of them and fell
// with it.
type DocsLeftOut struct {
	Folders   int `json:"folders"`
	Documents int `json:"documents"`
	Uploads   int `json:"uploads"`
}

// DocsPlaceLeftOut is a folder or a Docs & Files tool the bundle does not carry,
// with how many entries stood in it. The place is named, because a number alone
// says that something is gone and never which.
type DocsPlaceLeftOut struct {
	// Record is the kind of place: a folder or a Docs & Files tool.
	Record    string `json:"record"`
	SourceID  string `json:"sourceId"`
	Folders   int    `json:"folders"`
	Documents int    `json:"documents"`
	Uploads   int    `json:"uploads"`
}

// vaultFolder is a folder of a Docs & Files tool. Basecamp calls it a vault and
// makes the tool itself the vault at the top, so a folder standing directly in
// the tool names the dock entry as its parent.
//
// A folder takes no comment and no reaction, here as in Minka, so neither is
// read and neither is asked for.
type vaultFolder struct {
	recording
	ID     int64   `json:"id"`
	Status string  `json:"status"`
	Title  string  `json:"title"`
	Parent hangsOn `json:"parent"`
}

// vaultDocument is a document of a Docs & Files tool. Content is the document as
// Basecamp renders it; there is no stored form of it.
type vaultDocument struct {
	recording
	ID            int64      `json:"id"`
	Status        string     `json:"status"`
	Title         string     `json:"title"`
	Content       string     `json:"content"`
	Files         []bodyFile `json:"content_attachments"`
	Parent        hangsOn    `json:"parent"`
	CommentsCount int        `json:"comments_count"`
	BoostsCount   int        `json:"boosts_count"`
}

// vaultUpload is an upload of a Docs & Files tool: one stored file with a title
// and a description. Basecamp gives the file no ID of its own and names it in
// the address it is downloaded from, as it does for a file dropped into a chat.
//
// Title is what the upload is called, which Basecamp fills from the name of the
// file and lets anybody rename; Filename is the name of the file itself and
// stands in where the title is empty.
type vaultUpload struct {
	recording
	ID          int64  `json:"id"`
	Status      string `json:"status"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// Files are the files embedded in the description.
	Files       []bodyFile `json:"description_attachments"`
	Parent      hangsOn    `json:"parent"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	ByteSize    *int64     `json:"byte_size"`
	DownloadURL string     `json:"download_url"`

	CommentsCount int `json:"comments_count"`
	BoostsCount   int `json:"boosts_count"`
}

// A record of Basecamp that says nothing of what became of it is in use, as the
// format reads an absent status.
func (f vaultFolder) status() string   { return statusOrInUse(f.Status) }
func (d vaultDocument) status() string { return statusOrInUse(d.Status) }
func (u vaultUpload) status() string   { return statusOrInUse(u.Status) }

// noName is the title an upload gets whose file Basecamp names nothing at all. A
// title cannot be empty here, and a file without a name is a file all the same.
const noName = "(no name)"

// docsPlace is one vault the bundle carries: the top of a Docs & Files tool, or a
// folder of one. It holds what stands in it in the order the bundle writes it.
type docsPlace struct {
	id int64
	// folder is the record of the place, and nil at the top of a tool, which is
	// the dock entry and no folder of its own; in is the vault above it.
	folder    *vaultFolder
	in        int64
	documents []vaultDocument
	uploads   []vaultUpload
}

// lostVaultPlace is a folder or a Docs & Files tool the bundle does not carry.
// Whatever stood in it falls with it, and a walk of its own counts what that was.
type lostVaultPlace struct {
	record string
	id     int64
	// aFolder tells a folder, whose lists a run may still ask for, from a tool
	// that is switched off, which a run never reads.
	aFolder bool
}

// docsWalk is what the walk over the Docs & Files tools of one project carries
// along.
type docsWalk struct {
	bucket    int64
	projectID string
	// foldersGone, documentsGone and uploadsGone are the records the project has
	// put away, by the vault they stand in: a vault lists what is in use alone,
	// and the archived and the trashed records of a project stand in one list per
	// kind across all its Docs & Files tools, those that are switched off
	// included.
	foldersGone   map[int64][]vaultFolder
	documentsGone map[int64][]vaultDocument
	uploadsGone   map[int64][]vaultUpload
	// metFolders, metDocuments and metUploads are the records the walk has met. A
	// walk is no snapshot: Basecamp can list a record twice, and a bundle writes
	// it once.
	metFolders   map[int64]bool
	metDocuments map[int64]bool
	metUploads   map[int64]bool
	// carried are the vaults the bundle holds, so that a record naming one of
	// them is placed and a record naming anything else falls with its place.
	carried map[int64]bool
	// places are the vaults of one tool in the order the bundle writes them.
	places []*docsPlace
	// lost are the places the bundle does not carry.
	lost []lostVaultPlace
}

// extractDocs adds the folders, documents and uploads of every Docs & Files tool
// and what hangs on them, project by project and in the order of the tools. A
// project whose Docs & Files are all switched off is walked as well, so that what
// no bundle carries of them is counted rather than passed over in silence.
func (e *Extractor) extractDocs(ctx context.Context, w *bundle.Writer) error {
	for _, t := range e.talks {
		if len(t.everyVault) == 0 {
			continue
		}
		if err := e.extractDocsOfProject(ctx, w, t); err != nil {
			return err
		}
	}
	return nil
}

// extractDocsOfProject adds the Docs & Files tools of one project.
//
// Basecamp lists what is in use in a vault, folder by folder, and the archived
// and the trashed records of a project in one list per kind across all its Docs &
// Files tools, those that are switched off included. Such a record names the
// vault it stands in and nothing else, so every folder the bundle carries is
// known before the first entry of a tool is written, and an entry naming anything
// else is left out with the place it stood in.
//
// A walk is no snapshot: a folder that is moved between two requests can be
// missed, or met twice, and then it is written once.
func (e *Extractor) extractDocsOfProject(ctx context.Context, w *bundle.Writer, t talk) error {
	walk := &docsWalk{
		bucket: t.bucket, projectID: sourceID(t.bucket),
		foldersGone: map[int64][]vaultFolder{}, documentsGone: map[int64][]vaultDocument{}, uploadsGone: map[int64][]vaultUpload{},
		metFolders: map[int64]bool{}, metDocuments: map[int64]bool{}, metUploads: map[int64]bool{},
		carried: map[int64]bool{},
	}
	folders, err := shelvedInVaults[vaultFolder](ctx, e.Source, t.bucket, "Vault")
	if err != nil {
		return err
	}
	for _, found := range folders {
		walk.foldersGone[found.Parent.ID] = append(walk.foldersGone[found.Parent.ID], found)
	}
	// A vault record that names no vault above it is the vault at the top of a
	// tool and no folder of one: the dock entry itself, which the source lists
	// that way. It stands in nothing, so nothing can place it; it becomes a place
	// of its own, counted with what stood in it, rather than falling out of the
	// walk without a word.
	sort.SliceStable(walk.foldersGone[0], func(i, j int) bool { return walk.foldersGone[0][i].ID < walk.foldersGone[0][j].ID })
	for _, found := range walk.foldersGone[0] {
		walk.lost = append(walk.lost, lostVaultPlace{record: "folder", id: found.ID, aFolder: true})
		walk.metFolders[found.ID] = true
	}
	delete(walk.foldersGone, 0)
	documents, err := shelvedInVaults[vaultDocument](ctx, e.Source, t.bucket, "Document")
	if err != nil {
		return err
	}
	for _, found := range documents {
		walk.documentsGone[found.Parent.ID] = append(walk.documentsGone[found.Parent.ID], found)
	}
	uploads, err := shelvedInVaults[vaultUpload](ctx, e.Source, t.bucket, "Upload")
	if err != nil {
		return err
	}
	for _, found := range uploads {
		walk.uploadsGone[found.Parent.ID] = append(walk.uploadsGone[found.Parent.ID], found)
	}

	for _, id := range t.vaults {
		walk.places = nil
		if err := e.walkVault(ctx, walk, id, 0, nil); err != nil {
			return err
		}
		for _, place := range walk.places {
			if err := e.writeVaultPlace(ctx, w, walk, id, place); err != nil {
				return err
			}
		}
	}

	// A Docs & Files tool the run does not extract takes everything in it: the
	// tool is the record a bundle cannot carry, and one that is switched off is a
	// tool the run does not read at all.
	closed := make([]int64, 0, len(t.everyVault))
	for id := range t.everyVault {
		if !walk.carried[id] {
			closed = append(closed, id)
		}
	}
	sort.Slice(closed, func(i, j int) bool { return closed[i] < closed[j] })
	for _, id := range closed {
		walk.lost = append(walk.lost, lostVaultPlace{record: "Docs & Files tool", id: id})
	}
	return e.countWhatFellWithAVault(ctx, walk)
}

// walkVault reads one vault and everything below it: its folders, its documents
// and its uploads, each kind in the order the bundle writes them, and then every
// folder it carries. The place of a folder is added before the walk goes into it,
// so a parent always stands before its child.
//
// A folder that is not in use may answer that it is not there: Basecamp hands out
// no list of what stands in a folder it has thrown away. What is in use below
// such a folder is then beyond the reach of any run, while what the source has
// put away comes from the project's own lists either way.
func (e *Extractor) walkVault(ctx context.Context, walk *docsWalk, id, in int64, folder *vaultFolder) error {
	walk.carried[id] = true
	place := &docsPlace{id: id, in: in, folder: folder}
	walk.places = append(walk.places, place)
	inUse := folder == nil || folder.status() == bundle.StatusActive

	shownFolders, foldersGone, err := listedUnlessItIsGone[vaultFolder](ctx, e.Source, fmt.Sprintf("buckets/%d/vaults/%d/vaults.json", walk.bucket, id), inUse)
	if err != nil {
		return err
	}
	shownDocuments, documentsGone, err := listedUnlessItIsGone[vaultDocument](ctx, e.Source, fmt.Sprintf("buckets/%d/vaults/%d/documents.json", walk.bucket, id), inUse)
	if err != nil {
		return err
	}
	shownUploads, uploadsGone, err := listedUnlessItIsGone[vaultUpload](ctx, e.Source, fmt.Sprintf("buckets/%d/vaults/%d/uploads.json", walk.bucket, id), inUse)
	if err != nil {
		return err
	}
	// A folder that will not say what stands in it is a hole nobody can count:
	// what is in use below it reaches no bundle and no later run finds it
	// either, so the run says how many such folders there were.
	if foldersGone || documentsGone || uploadsGone {
		e.docs.FoldersThatSaidNothingOfWhatIsInThem++
	}

	for _, found := range inFileOrder(walk.documentsGone[id], shownDocuments) {
		if aSecondTime(walk.metDocuments, found.ID) {
			continue
		}
		taken, err := e.met("document", found.ID, found.status(), found.recording, "project", walk.projectID)
		if err != nil {
			return err
		}
		if !taken {
			e.docs.LeftOut.Documents++
			continue
		}
		place.documents = append(place.documents, found)
	}
	for _, found := range inFileOrder(walk.uploadsGone[id], shownUploads) {
		if aSecondTime(walk.metUploads, found.ID) {
			continue
		}
		taken, err := e.met("upload", found.ID, found.status(), found.recording, "project", walk.projectID)
		if err != nil {
			return err
		}
		if !taken {
			e.docs.LeftOut.Uploads++
			continue
		}
		place.uploads = append(place.uploads, found)
	}

	for _, found := range inFileOrder(walk.foldersGone[id], shownFolders) {
		if aSecondTime(walk.metFolders, found.ID) {
			continue
		}
		taken, err := e.met("folder", found.ID, found.status(), found.recording, "project", walk.projectID)
		if err != nil {
			return err
		}
		if !taken {
			walk.lost = append(walk.lost, lostVaultPlace{record: "folder", id: found.ID, aFolder: true})
			continue
		}
		below := found
		if err := e.walkVault(ctx, walk, found.ID, id, &below); err != nil {
			return err
		}
	}
	return nil
}

// writeVaultPlace adds one vault of a tool: the folder itself where the place is
// one, then its documents and then its uploads.
func (e *Extractor) writeVaultPlace(ctx context.Context, w *bundle.Writer, walk *docsWalk, tool int64, place *docsPlace) error {
	var folderID *string
	if place.folder != nil {
		if err := e.writeFolder(w, walk, tool, place.in, *place.folder); err != nil {
			return err
		}
		named := sourceID(place.folder.ID)
		folderID = &named
	}
	for _, found := range place.documents {
		if err := e.writeDocument(ctx, w, walk, tool, folderID, found); err != nil {
			return err
		}
	}
	for _, found := range place.uploads {
		if err := e.writeUpload(ctx, w, walk, tool, folderID, found); err != nil {
			return err
		}
	}
	return nil
}

// writeFolder adds one folder. Its word about clients is its own at every depth:
// Minka reads it below the top of a tool as well, where it is the one thing that
// can hide a folder at the top the source shows, and an entry that left the word
// out would arrive in the open under a folder clients see.
//
// Neither a comment nor a boost of a folder is written or asked for: Minka takes
// neither of a folder, and Basecamp gives a folder neither.
func (e *Extractor) writeFolder(w *bundle.Writer, walk *docsWalk, tool, in int64, found vaultFolder) error {
	title, err := itemTitle("folder", found.ID, found.Title)
	if err != nil {
		return err
	}
	origin, err := e.record("folder", found.ID, found.recording)
	if err != nil {
		return err
	}
	record := bundle.Folder{
		Record:           origin,
		ProjectSourceID:  walk.projectID,
		ToolSourceID:     sourceID(tool),
		Title:            title,
		Status:           found.status(),
		VisibleToClients: visibleToClientsOf(found.recording),
	}
	if in != tool {
		named := sourceID(in)
		record.ParentSourceID = &named
	}
	if err := w.Add(bundle.Folders, record); err != nil {
		return err
	}
	e.docs.FoldersByStatus[found.status()]++
	return nil
}

// writeDocument adds one document with its text, its files, its comments and its
// boosts. Its status is its own: a document in a folder the source archived or
// threw away carries active and goes along through its folder, and a run that
// wrote the folder's word onto it would put it away one by one and never bring it
// back with the folder.
func (e *Extractor) writeDocument(ctx context.Context, w *bundle.Writer, walk *docsWalk, tool int64, folderID *string, found vaultDocument) error {
	title, err := itemTitle("document", found.ID, found.Title)
	if err != nil {
		return err
	}
	origin, err := e.record("document", found.ID, found.recording)
	if err != nil {
		return err
	}
	body, files, err := e.rewriteBody(found.Content, "", found.Files)
	if err != nil {
		return err
	}
	// Before the body is weighed: only what became of the files says whether
	// what is left of the document reaches Minka at all.
	if err := e.attachFiles(ctx, w, bundle.OwnerDocument, origin, files); err != nil {
		return err
	}
	body = e.wordlessWhereItsOnlyFileNeverComes(body, origin.SourceID)

	record := bundle.Document{
		Record:           origin,
		ProjectSourceID:  walk.projectID,
		ToolSourceID:     sourceID(tool),
		FolderSourceID:   folderID,
		Title:            title,
		Status:           found.status(),
		VisibleToClients: visibleToClientsOf(found.recording),
	}
	// A document without words is a document nobody has written anything in yet,
	// which Minka takes.
	if body != "" {
		record.Body = &body
	} else {
		e.docs.DocumentsWithoutWords++
	}
	if err := w.Add(bundle.Documents, record); err != nil {
		return err
	}
	e.measure(bundle.Documents, body, LongestBodyRecord{Record: "document", SourceID: origin.SourceID, In: "project", InSourceID: walk.projectID})
	if longerThanMinkaTakes(body) {
		e.docs.LongerThanMinkaTakes++
	}
	e.docs.DocumentsByStatus[found.status()]++
	return e.writeDiscussion(ctx, w, walk.bucket, bundle.ParentDocument, bundle.TargetDocument, found.ID, found.CommentsCount, found.BoostsCount)
}

// writeUpload adds one upload: its file, its title, its description, its comments
// and its boosts. The title is the name of the file, because Minka derives
// nothing from a file and refuses an upload without a title.
//
// An upload Basecamp names no stored file for is left out and named, and its
// comments and its boosts go with it rather than being asked for: what lies in a
// cloud is no file any run can fetch, and a comment pointing at an upload the
// bundle does not carry would stop the applier.
//
// Only the file the source shows now comes. The earlier copies of it stay at the
// source: the format keeps a field free for them and reads it nowhere, and the
// upload says neither how many there are nor which.
func (e *Extractor) writeUpload(ctx context.Context, w *bundle.Writer, walk *docsWalk, tool int64, folderID *string, found vaultUpload) error {
	file, aFile := fileOfAnUpload(found, walk.bucket)
	if !aFile {
		e.docs.WithoutAStoredFile = append(e.docs.WithoutAStoredFile, sourceID(found.ID))
		e.docs.CommentsWithoutTheirUpload += found.CommentsCount
		e.docs.BoostsWithoutTheirUpload += found.BoostsCount
		return nil
	}
	// What the source calls the upload, and the name of the file where it calls
	// it nothing: Basecamp fills the title from the name and lets anybody rename
	// it, so the title is the one to take and the name is what stands in.
	called := found.Title
	if strings.TrimSpace(called) == "" {
		called = found.Filename
		e.docs.TitlesFromTheFileName++
	}
	title, withoutAName, changed := aFileNameAsATitle(called, noName)
	if withoutAName {
		e.docs.WithoutAName++
	}
	if changed {
		e.docs.NamesMadeIntoATitle++
	}
	origin, err := e.record("upload", found.ID, found.recording)
	if err != nil {
		return err
	}
	description, embedded, err := e.rewriteBody(found.Description, "", found.Files)
	if err != nil {
		return err
	}
	if err := e.attachFile(ctx, w, bundle.OwnerUpload, origin, file); err != nil {
		return err
	}
	if err := e.attachFiles(ctx, w, bundle.OwnerUpload, origin, embedded); err != nil {
		return err
	}
	description = e.wordlessWhereItsOnlyFileNeverComes(description, origin.SourceID)
	// An upload whose own file will never come is the one record of the three
	// that cannot be written without it: the field is required, so nothing but
	// the file makes the upload, and Minka keeps nothing of it until the source
	// hands the file back.
	if e.theFileNeverArrives(file.ref) {
		e.docs.OnlyAFileNoRunWillBring = append(e.docs.OnlyAFileNoRunWillBring, origin.SourceID)
	}

	record := bundle.Upload{
		Record:           origin,
		ProjectSourceID:  walk.projectID,
		ToolSourceID:     sourceID(tool),
		FolderSourceID:   folderID,
		Title:            title,
		File:             file.ref,
		Status:           found.status(),
		VisibleToClients: visibleToClientsOf(found.recording),
	}
	// An upload without a description has none.
	if description != "" {
		record.Description = &description
	}
	if err := w.Add(bundle.Uploads, record); err != nil {
		return err
	}
	e.measure(bundle.Uploads, description, LongestBodyRecord{Record: "upload", SourceID: origin.SourceID, In: "project", InSourceID: walk.projectID})
	if longerThanMinkaTakes(description) {
		e.docs.LongerThanMinkaTakes++
	}
	e.docs.UploadsByStatus[found.status()]++
	return e.writeDiscussion(ctx, w, walk.bucket, bundle.ParentUpload, bundle.TargetUpload, found.ID, found.CommentsCount, found.BoostsCount)
}

// wordlessWhereItsOnlyFileNeverComes is the text of a document, or the
// description of an upload, with the tag of a file no run will ever bring taken
// out where the text is nothing else — which leaves nothing, so the record
// arrives without words. It says which records those were.
//
// The tag left in would cost the record altogether: Minka keeps nothing of a text
// that is nothing but a file it has not got, and asks for the file again on every
// run, so the title, the comments and the boosts of such a document would arrive
// on no run while the file is gone — and the whole case is that it stays gone. The
// field takes nothing, a wordless document is one Minka writes, and so the
// document comes and says nothing, which is the truth about it.
//
// **No line of the run's own** as a forwarded mail gets: there the body is a field
// that must not be empty, so an invented sentence was the only way the mail came
// at all. Here it is not, and a document is read as something a person wrote.
//
// A file this run merely could not fetch is no file that will never come: the tag
// stays, and the run after this one writes the document whole.
func (e *Extractor) wordlessWhereItsOnlyFileNeverComes(text, sourceID string) string {
	filesThatStayOut, embedded, empty := e.whatWouldArriveOf(text)
	if !empty || filesThatStayOut == 0 || len(embedded) > 0 {
		return text
	}
	e.docs.OnlyAFileNoRunWillBring = append(e.docs.OnlyAFileNoRunWillBring, sourceID)
	return ""
}

// fileOfAnUpload is the one file an upload stands for, as a run brings it, and
// whether the record names a file at all.
//
// **It is the record that says so and never the shape of the address**, and the
// bytes are asked for **by the identifier of the upload and its project**:
// Basecamp names no stored file on such a record and answers the address it does
// give with a redirect to a store of its own that no run follows, so there is no
// storage address to build and none is looked for. The
// `…/blobs/{key}/download/{name}` shape belongs to a file inside a text, which
// carries its key; reading an upload as if it did leaves every file of a tool out
// of a bundle, counted and named and gone. An upload that lies in a cloud names
// no file at all, and that is the one this leaves out.
//
// The reference is the upload's own and never the key of a file: a stored file
// has one place in Minka, so a text that embedded the same file and an upload
// that stood for it would be one reference used twice, the text would claim it,
// and the upload would stay out.
func fileOfAnUpload(found vaultUpload, bucket int64) (sourceFile, bool) {
	if found.DownloadURL == "" {
		return sourceFile{}, false
	}
	return sourceFile{
		ref: fmt.Sprintf("upload-%d", found.ID), upload: found.ID, bucket: bucket,
		filename: found.Filename, contentType: found.ContentType, byteSize: found.ByteSize, downloadURL: found.DownloadURL,
	}, true
}

// shelvedInVaults are the records of one kind of the Docs & Files tools of a
// project that are on the shelf or in the trash, whatever tool or folder they
// stand in.
func shelvedInVaults[T any](ctx context.Context, source Source, bucket int64, kind string) ([]T, error) {
	var shelved []T
	for _, status := range []string{bundle.StatusArchived, bundle.StatusTrashed} {
		listed, err := getList[T](ctx, source, fmt.Sprintf("projects/recordings.json?type=%s&bucket=%d&status=%s", kind, bucket, status))
		if err != nil {
			return nil, err
		}
		shelved = append(shelved, listed...)
	}
	return shelved, nil
}

// countWhatFellWithAVault counts the folders, documents and uploads that stood in
// a folder or a Docs & Files tool the bundle does not carry, and names the place
// they fell with.
//
// They are asked for although none of them is written: what is left out is
// counted, and a record that falls with its place would otherwise be gone
// without a number. A folder of such a folder is a place of its own, so the walk
// grows while it goes. A **tool** is asked nothing: one that is switched off is a
// tool the run does not read, and what the project's own lists of records it has
// put away hand out of such a tool is counted where it is met.
func (e *Extractor) countWhatFellWithAVault(ctx context.Context, walk *docsWalk) error {
	for i := 0; i < len(walk.lost); i++ {
		place := walk.lost[i]
		fell := DocsPlaceLeftOut{Record: place.record, SourceID: sourceID(place.id)}
		if place.aFolder {
			e.docs.LeftOut.Folders++
			if err := e.countWhatStandsInAFolderLeftOut(ctx, walk, place.id, &fell); err != nil {
				return err
			}
		}
		for _, found := range walk.foldersGone[place.id] {
			if !aSecondTime(walk.metFolders, found.ID) {
				fell.Folders++
				walk.lost = append(walk.lost, lostVaultPlace{record: "folder", id: found.ID, aFolder: true})
			}
		}
		for _, found := range walk.documentsGone[place.id] {
			if !aSecondTime(walk.metDocuments, found.ID) {
				fell.Documents++
				e.docs.LeftOut.Documents++
			}
		}
		for _, found := range walk.uploadsGone[place.id] {
			if !aSecondTime(walk.metUploads, found.ID) {
				fell.Uploads++
				e.docs.LeftOut.Uploads++
			}
		}

		// A place that took nothing with it says nothing worth reading.
		if fell.Folders > 0 || fell.Documents > 0 || fell.Uploads > 0 {
			e.docs.PlacesLeftOut = append(e.docs.PlacesLeftOut, fell)
		}
	}

	// An entry of a place the walk never saw at all, such as a folder in use of a
	// tool that is switched off.
	e.docs.LeftOut.Folders += countWhatWasNeverPlaced(walk.foldersGone, walk.metFolders, func(f vaultFolder) int64 { return f.ID })
	e.docs.LeftOut.Documents += countWhatWasNeverPlaced(walk.documentsGone, walk.metDocuments, func(d vaultDocument) int64 { return d.ID })
	e.docs.LeftOut.Uploads += countWhatWasNeverPlaced(walk.uploadsGone, walk.metUploads, func(u vaultUpload) int64 { return u.ID })
	return nil
}

// countWhatStandsInAFolderLeftOut asks a folder the bundle does not carry what
// stands in it, so that what falls with it is counted. A folder of it is a place
// of its own and is added to the walk.
func (e *Extractor) countWhatStandsInAFolderLeftOut(ctx context.Context, walk *docsWalk, id int64, fell *DocsPlaceLeftOut) error {
	folders, _, err := listedUnlessItIsGone[vaultFolder](ctx, e.Source, fmt.Sprintf("buckets/%d/vaults/%d/vaults.json", walk.bucket, id), false)
	if err != nil {
		return err
	}
	for _, found := range folders {
		if !aSecondTime(walk.metFolders, found.ID) {
			fell.Folders++
			walk.lost = append(walk.lost, lostVaultPlace{record: "folder", id: found.ID, aFolder: true})
		}
	}
	documents, _, err := listedUnlessItIsGone[vaultDocument](ctx, e.Source, fmt.Sprintf("buckets/%d/vaults/%d/documents.json", walk.bucket, id), false)
	if err != nil {
		return err
	}
	for _, found := range documents {
		if !aSecondTime(walk.metDocuments, found.ID) {
			fell.Documents++
			e.docs.LeftOut.Documents++
		}
	}
	uploads, _, err := listedUnlessItIsGone[vaultUpload](ctx, e.Source, fmt.Sprintf("buckets/%d/vaults/%d/uploads.json", walk.bucket, id), false)
	if err != nil {
		return err
	}
	for _, found := range uploads {
		if !aSecondTime(walk.metUploads, found.ID) {
			fell.Uploads++
			e.docs.LeftOut.Uploads++
		}
	}
	return nil
}

// countWhatWasNeverPlaced counts the records the source has put away that no
// place of the walk ever claimed, so that nothing of a tool the run does not read
// is lost without a number.
func countWhatWasNeverPlaced[T any](gone map[int64][]T, met map[int64]bool, id func(T) int64) int {
	count := 0
	for _, records := range gone {
		for _, found := range records {
			if !aSecondTime(met, id(found)) {
				count++
			}
		}
	}
	return count
}
