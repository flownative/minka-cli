package fakebasecamp

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
)

// Folder is a folder of a Docs & Files tool: what Basecamp calls a vault. The
// tool itself is the vault at the top, so the folders, documents and uploads
// standing directly in the tool hang on the dock entry.
type Folder struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the folder says
	// about it, wherever it is listed.
	Status      string
	ShownStatus string
	Title       string
	Creator     *Person
	CreatedAt   string
	UpdatedAt   string
	// Folders, Documents and Uploads are what stands in the folder, in the order
	// the account lists them: the topmost first, as Basecamp shows them.
	Folders   []Folder
	Documents []Document
	Uploads   []UploadedFile
	// Twice has the account list this one twice wherever it lists it, as a list
	// that moves under a walk does.
	Twice bool
	// VisibleToClients is what the folder says about clients, at the top of its
	// tool and below it alike.
	VisibleToClients *bool
}

// Document is a document of a Docs & Files tool.
type Document struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the document says
	// about it, wherever it is listed.
	Status      string
	ShownStatus string
	Title       string
	// Content is the document as Basecamp renders it.
	Content   string
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	Files     []File
	Comments  []Comment
	Boosts    []Boost
	Twice     bool
	// VisibleToClients is what the document says about clients, at the top of
	// its tool and below it alike.
	VisibleToClients *bool
}

// UploadedFile is an upload of a Docs & Files tool: one stored file with a title
// and a description. Basecamp names the stored file in the address it is
// downloaded from and nowhere else, as it does for a file dropped into a chat,
// and the key of that file is made from the upload's own ID here.
type UploadedFile struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the upload says
	// about it, wherever it is listed.
	Status      string
	ShownStatus string
	// Title is what Basecamp calls the upload, which it fills from the name of
	// the file and anybody can rename; Filename is the name of the file itself.
	Title       string
	Description string
	// Position is the place the source shows the upload at, which the format has
	// no word for and no run reads.
	Position    int
	Filename    string
	ContentType string
	Bytes       int64
	// NoSize leaves the size out of what the account says about the file.
	NoSize bool
	// Gone is a file the account lists and no longer has.
	Gone bool
	// URLName replaces the name in the addresses of the file, and DownloadURL the
	// whole address the account says the file is downloaded from, which no run
	// asks for: an upload's bytes come by its identifier.
	URLName     string
	DownloadURL string
	// InTheCloud is an upload Basecamp holds no stored file for, one that lives
	// in a Google Drive or another cloud: the answer names no file to download.
	InTheCloud bool
	Creator    *Person
	CreatedAt  string
	UpdatedAt  string
	// Files are the files embedded in the description.
	Files    []File
	Comments []Comment
	Boosts   []Boost
	Twice    bool
	// VisibleToClients is what the upload says about clients, at the top of its
	// tool and below it alike.
	VisibleToClients *bool
}

// docsProject is a project with every case of a Docs & Files tool: two tools
// that are switched on and one that is switched off, folders at the top and one
// inside another, a folder on the shelf and one in the trash each holding an
// entry that is in use, a folder of a status nobody knows with a folder, a
// document and an upload falling with it, documents and uploads at the top of
// the tool, a document that is nothing but a file the source no longer has, a
// document and an upload of a status nobody knows, an upload Basecamp holds no
// stored file for with a comment and a boost on it, one whose file name is
// longer than a title holds, one without a name at all, records the account
// lists twice, an upload the source has renamed and one it calls nothing, an
// upload on the shelf, one in the trash inside a folder that is
// in use, a document on the shelf inside a folder that is in use, one whose file
// the source no longer has, a document embedding a file
// whose identifier is the identifier of an upload, a folder in use behind the
// switched-off tool that no walk can see, and comments and boosts on a document
// and on an upload.
//
// The folder inside the first one is the case the whole tool turns on: it is
// hidden from clients below a folder the source shows, which is what makes Minka
// hide the folder above it with everything in it.
func docsProject(ada, grace, former Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2025-06-%02dT%02d:00:00.000Z", day, hour) }
	made := tool
	tool = func(id int64, name, kind, title string, enabled bool, position int, creator *Person) Tool {
		t := made(id, name, kind, title, enabled, position, creator)
		t.CreatedAt, t.UpdatedAt = at(1, 7), at(20, 12)
		return t
	}
	shown, hidden := true, false

	chart := File{ID: 5401, SGID: SGID("gid://bc3/Attachment/5401?expires_in"), Key: FileKey(5401),
		Filename: "chart.png", ContentType: "image/png", Bytes: 2048}
	shredded := File{ID: 5402, SGID: SGID("gid://bc3/Attachment/5402?expires_in"), Key: FileKey(5402),
		Filename: "shredded.png", ContentType: "image/png", Bytes: 1024, Gone: true}

	// A document with everything one carries: a mention, an embedded file,
	// comments with a boost, and a boost of its own.
	chapter := Document{
		ID: 9240, Title: "Chapter one", Creator: &ada, CreatedAt: at(2, 8), UpdatedAt: at(2, 9),
		VisibleToClients: &shown,
		Content: `<div>How it works, says ` + Mention(SGID("gid://bc3/Person/102?expires_in"), "", 102, "Grace Sample") +
			`</div>` + Embed{File: chart}.HTML(),
		Files: []File{chart},
		Comments: []Comment{
			{ID: 9271, Creator: &grace, CreatedAt: at(2, 10), UpdatedAt: at(2, 10), Content: `<div>Clear enough</div>`,
				Boosts: []Boost{{ID: 9282, Content: "🎉", CreatedAt: at(2, 12), Booster: &ada}}},
			{ID: 9272, Creator: &former, CreatedAt: at(2, 11), UpdatedAt: at(2, 11), Content: `<div>Filed</div>`},
		},
		Boosts: []Boost{{ID: 9281, Content: "👍", CreatedAt: at(2, 13), Booster: &grace}},
	}
	// A file whose identifier is the identifier of an upload of the same tool.
	// Basecamp counts attachments and recordings in tables of their own, so the
	// two can be the same number, and then a reference made of the number alone
	// would name one file for two records.
	// Its stored file is its own: one blob is one name and one size, whoever
	// points at it, so the key is no upload's.
	sameNumber := File{ID: 9260, SGID: SGID("gid://bc3/Attachment/9260?expires_in"), Key: FileKey(7260),
		Filename: "table.png", ContentType: "image/png", Bytes: 640}
	appendix := Document{ID: 9252, Title: "Appendix", Creator: &grace, CreatedAt: at(2, 14), UpdatedAt: at(2, 14),
		Content: `<div>The rest</div>` + Embed{File: sameNumber}.HTML(), Files: []File{sameNumber}}

	// Renamed at the source: what it is called is not the name of its file, and
	// the title is the source's to give.
	logo := UploadedFile{
		ID: 9260, Title: "The mark we use", Filename: "logo.png", ContentType: "image/png", Bytes: 4096, Position: 1,
		Creator: &ada, CreatedAt: at(3, 8), UpdatedAt: at(3, 9), VisibleToClients: &shown,
		Description: `<div>The one we use</div>`,
		Comments: []Comment{{ID: 9273, Creator: &grace, CreatedAt: at(3, 10), UpdatedAt: at(3, 10),
			Content: `<div>Looks right</div>`}},
		Boosts: []Boost{{ID: 9283, Content: "👏", CreatedAt: at(3, 11), Booster: &grace}},
	}

	// Hidden from clients, below a folder the source shows: the one record this
	// tool exists to carry over.
	nested := Folder{ID: 9221, Title: "Drafts", Creator: &ada, CreatedAt: at(1, 9), UpdatedAt: at(1, 9),
		VisibleToClients: &hidden, Twice: true,
		Documents: []Document{{ID: 9241, Title: "Not for outside", Creator: &ada, CreatedAt: at(4, 8), UpdatedAt: at(4, 8),
			VisibleToClients: &hidden, Content: `<div>Inside only</div>`}},
		Uploads: []UploadedFile{{ID: 9265, Title: "sketch.png", Filename: "sketch.png", ContentType: "image/png", Bytes: 512,
			Creator: &ada, CreatedAt: at(4, 9), UpdatedAt: at(4, 9), VisibleToClients: &hidden}},
	}

	// In the trash inside a folder that is in use, which the project's own list of
	// records it has put away hands out and the folder does not.
	binned := UploadedFile{ID: 9270, Status: "trashed", Title: "draft.pdf", Filename: "draft.pdf",
		ContentType: "application/pdf", Bytes: 192, Creator: &ada, CreatedAt: at(3, 12), UpdatedAt: at(5, 13)}
	// On the shelf inside a folder that is in use, which the project's own list of
	// records it has put away hands out and the folder's own list does not: an
	// entry keeps its own state wherever it stands.
	shelved := Document{ID: 9255, Status: "archived", Title: "Of another year", Creator: &grace,
		CreatedAt: at(2, 16), UpdatedAt: at(5, 16), Content: `<div>Kept</div>`}
	handbook := Folder{ID: 9220, Title: "Handbook", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8),
		VisibleToClients: &shown,
		Folders:          []Folder{nested},
		Documents:        []Document{chapter, appendix, shelved},
		Uploads:          []UploadedFile{logo, binned},
	}
	press := Folder{ID: 9228, Title: "Press", Creator: &grace, CreatedAt: at(1, 10), UpdatedAt: at(1, 10)}
	// On the shelf and in the trash, each holding an entry that is in use: an
	// entry keeps its own state and goes along through the folder.
	// A folder the source has put away hands out no list of what stands in it, so
	// the document and the upload in use below these two are beyond the reach of
	// any run: they stand here to be out of reach, and the run says how many such
	// folders there were.
	finished := Folder{ID: 9222, Status: "archived", Title: "Finished", Creator: &ada, CreatedAt: at(1, 11), UpdatedAt: at(5, 11),
		Documents: []Document{{ID: 9242, Title: "Last year", Creator: &ada, CreatedAt: at(5, 8), UpdatedAt: at(5, 8),
			Content: `<div>Done with</div>`}}}
	dropped := Folder{ID: 9223, Status: "trashed", Title: "Thrown out", Creator: &ada, CreatedAt: at(1, 12), UpdatedAt: at(5, 12),
		Uploads: []UploadedFile{{ID: 9261, Title: "old.pdf", Filename: "old.pdf", ContentType: "application/pdf", Bytes: 256,
			Creator: &ada, CreatedAt: at(5, 9), UpdatedAt: at(5, 9)}}}
	// A word a bundle has none for, on a folder that is in use: it is left out,
	// and the folder, the document and the upload standing in it fall with it.
	held := Folder{ID: 9224, ShownStatus: "quarantined", Title: "Held back", Creator: &ada, CreatedAt: at(1, 13), UpdatedAt: at(1, 13),
		Folders: []Folder{{ID: 9226, Title: "Below the one held back", Creator: &ada, CreatedAt: at(1, 14), UpdatedAt: at(1, 14)}},
		Documents: []Document{{ID: 9243, Title: "Below as well", Creator: &ada, CreatedAt: at(6, 8), UpdatedAt: at(6, 8),
			Content: `<div>Nobody sees this</div>`}},
		Uploads: []UploadedFile{{ID: 9266, Title: "held.pdf", Filename: "held.pdf", ContentType: "application/pdf", Bytes: 128,
			Creator: &ada, CreatedAt: at(6, 9), UpdatedAt: at(6, 9)}},
	}

	docs := tool(9210, "vault", "vaults", "Docs & Files", true, 1, &ada)
	docs.CreatedAt = "2025-06-01T07:00:00.100Z"
	docs.Folders = []Folder{handbook, press, finished, dropped, held}
	docs.Documents = []Document{
		{ID: 9244, Title: "Read me first", Creator: &ada, CreatedAt: at(7, 8), UpdatedAt: at(7, 8), Twice: true,
			Content: `<div>Start here</div>`},
		// Nothing but a file the source no longer has: Minka keeps nothing of
		// such a document and waits for a file that will never come.
		{ID: 9245, Title: "The picture", Creator: &ada, CreatedAt: at(7, 9), UpdatedAt: at(7, 9),
			Content: Embed{File: shredded}.HTML(), Files: []File{shredded}},
		{ID: 9246, Status: "archived", Title: "Last quarter", Creator: &grace, CreatedAt: at(7, 10), UpdatedAt: at(8, 10),
			Content: `<div>Over</div>`},
		{ID: 9247, Status: "trashed", Title: "Wrong place", Creator: &grace, CreatedAt: at(7, 11), UpdatedAt: at(8, 11),
			Content: `<div>Gone</div>`},
		// A word a bundle has none for, on a document that is in use.
		{ID: 9250, ShownStatus: "reviewing", Title: "Under review", Creator: &ada, CreatedAt: at(7, 12), UpdatedAt: at(7, 12),
			Content: `<div>Waiting</div>`},
	}
	docs.Uploads = []UploadedFile{
		// Called nothing at the source, so the name of its file stands in.
		{ID: 9267, Title: "", Filename: "minutes.pdf", ContentType: "application/pdf", Bytes: 1536,
			Twice: true, Creator: &grace, CreatedAt: at(9, 8), UpdatedAt: at(9, 8),
			Description: `<div>Of the last meeting, see ` + Embed{File: chart}.HTML() + `</div>`, Files: []File{chart}},
		// In a cloud, so Basecamp names no file: the upload is left out, and the
		// comment and the boost on it go with it.
		{ID: 9262, Title: "Plan in the cloud", InTheCloud: true, Creator: &ada, CreatedAt: at(9, 9), UpdatedAt: at(9, 9),
			Comments: []Comment{{ID: 9274, Creator: &grace, CreatedAt: at(9, 10), UpdatedAt: at(9, 10), Content: `<div>Open it there</div>`}},
			Boosts:   []Boost{{ID: 9284, Content: "🤔", CreatedAt: at(9, 11), Booster: &grace}}},
		// A file name longer than a title holds, and one that is no name at all.
		{ID: 9263, Title: LongName, Filename: LongName, URLName: "long.jpeg", ContentType: "image/jpeg", Bytes: 700,
			Creator: &ada, CreatedAt: at(9, 12), UpdatedAt: at(9, 12)},
		{ID: 9264, Title: "", Filename: "", URLName: "unnamed", ContentType: "application/octet-stream", Bytes: 64,
			Creator: &ada, CreatedAt: at(9, 13), UpdatedAt: at(9, 13)},
		// A word a bundle has none for, on an upload that is in use.
		{ID: 9268, ShownStatus: "reviewing", Title: "checked.pdf", Filename: "checked.pdf", ContentType: "application/pdf", Bytes: 320,
			Creator: &ada, CreatedAt: at(9, 14), UpdatedAt: at(9, 14)},
		// On the shelf at the top of the tool.
		{ID: 9269, Status: "archived", Title: "last-year.pdf", Filename: "last-year.pdf", ContentType: "application/pdf", Bytes: 480,
			Creator: &grace, CreatedAt: at(9, 15), UpdatedAt: at(10, 15)},
		// The one file the upload stands for is one the source no longer has, so
		// no run will ever bring it and Minka keeps nothing of the upload.
		{ID: 9276, Title: "lost.pdf", Filename: "lost.pdf", ContentType: "application/pdf", Bytes: 220, Gone: true,
			Creator: &ada, CreatedAt: at(9, 16), UpdatedAt: at(9, 16)},
	}

	// A second tool of the same kind: a project may hold several.
	shared := tool(9211, "vault", "vaults", "Shared files", true, 2, &grace)
	shared.Documents = []Document{{ID: 9248, Title: "For everybody", Creator: &grace, CreatedAt: at(10, 8), UpdatedAt: at(10, 8),
		Content: `<div>Shared</div>`}}

	// Switched off, and so never read: what stands in it must reach no bundle,
	// and what the project's own lists of archived records hand out of it is
	// left out and counted.
	closed := tool(9212, "vault", "vaults", "Old files", false, 0, &ada)
	closed.Folders = []Folder{
		{ID: 9227, Status: "archived", Title: "Behind a switch", Creator: &ada, CreatedAt: at(11, 8), UpdatedAt: at(11, 9),
			Documents: []Document{{ID: 9253, Status: "archived", Title: "Nobody reads this", Creator: &ada, CreatedAt: at(11, 10), UpdatedAt: at(11, 11),
				Content: `<div>Nothing</div>`}}},
		// In use, and so in none of the project's lists of records it has put
		// away: behind a switch a run does not read, it is a folder no walk ever
		// sees, and the archived document in it names a vault nothing can place.
		{ID: 9230, Title: "In use behind a switch", Creator: &ada, CreatedAt: at(11, 14), UpdatedAt: at(11, 14),
			Documents: []Document{{ID: 9254, Status: "archived", Title: "Under a folder nothing sees", Creator: &ada,
				CreatedAt: at(11, 15), UpdatedAt: at(11, 16), Content: `<div>Nothing either</div>`}}},
	}
	closed.Documents = []Document{{ID: 9251, Status: "archived", Title: "Behind a switch as well", Creator: &ada,
		CreatedAt: at(11, 12), UpdatedAt: at(11, 13), Content: `<div>Nothing either</div>`}}

	return Project{
		ID: 7011, Name: "Docs Project", Status: "active",
		CreatedAt: "2025-06-01T07:00:00.000Z", UpdatedAt: "2025-06-20T12:00:00.000Z",
		Members: []int64{101, 102},
		Dock:    []Tool{docs, shared, closed},
	}
}

var (
	vaultsPattern          = regexp.MustCompile(`^buckets/([0-9]+)/vaults/([0-9]+)/vaults\.json$`)
	vaultDocumentsPattern  = regexp.MustCompile(`^buckets/([0-9]+)/vaults/([0-9]+)/documents\.json$`)
	vaultUploadsPattern    = regexp.MustCompile(`^buckets/([0-9]+)/vaults/([0-9]+)/uploads\.json$`)
	vaultRecordingsPattern = regexp.MustCompile(`^projects/recordings\.json\?type=(Vault|Document|Upload)&bucket=([0-9]+)&status=(archived|trashed)$`)
)

// docs answers the paths of Docs & Files tools. The second value tells whether
// the path was one of them.
//
// The three lists of a vault hold what is in use, as Basecamp shows them, and
// the archived and the trashed records of the project stand in one list per kind
// across all its Docs & Files tools, those that are switched off included.
func (a *Account) docs(path string, page int) ([]byte, bool) {
	switch {
	case vaultsPattern.MatchString(path):
		match := vaultsPattern.FindStringSubmatch(path)
		if p, in, known := a.theVault(match[1], match[2]); known && in.answers {
			var found []map[string]any
			for i := range in.folders {
				folder := &in.folders[i]
				if folder.status() != "active" {
					continue
				}
				found = append(found, a.folderJSON(p, in.id, folder))
				if folder.Twice {
					found = append(found, a.folderJSON(p, in.id, folder))
				}
			}
			return a.pageOf(found, page), true
		}
	case vaultDocumentsPattern.MatchString(path):
		match := vaultDocumentsPattern.FindStringSubmatch(path)
		if p, in, known := a.theVault(match[1], match[2]); known && in.answers {
			var found []map[string]any
			for i := range in.documents {
				document := &in.documents[i]
				if document.status() != "active" {
					continue
				}
				found = append(found, a.documentJSON(p, in.id, document))
				if document.Twice {
					found = append(found, a.documentJSON(p, in.id, document))
				}
			}
			return a.pageOf(found, page), true
		}
	case vaultUploadsPattern.MatchString(path):
		match := vaultUploadsPattern.FindStringSubmatch(path)
		if p, in, known := a.theVault(match[1], match[2]); known && in.answers {
			var found []map[string]any
			for i := range in.uploads {
				upload := &in.uploads[i]
				if upload.status() != "active" {
					continue
				}
				found = append(found, a.uploadedFileJSON(p, in.id, upload))
				if upload.Twice {
					found = append(found, a.uploadedFileJSON(p, in.id, upload))
				}
			}
			return a.pageOf(found, page), true
		}
	case vaultRecordingsPattern.MatchString(path):
		match := vaultRecordingsPattern.FindStringSubmatch(path)
		if p := a.project(match[2]); p != nil {
			var found []map[string]any
			a.eachVault(p, func(in vault) {
				switch match[1] {
				case "Vault":
					for i := range in.folders {
						if folder := &in.folders[i]; folder.status() == match[3] {
							found = append(found, a.folderJSON(p, in.id, folder))
							if folder.Twice {
								found = append(found, a.folderJSON(p, in.id, folder))
							}
						}
					}
				case "Document":
					for i := range in.documents {
						if document := &in.documents[i]; document.status() == match[3] {
							found = append(found, a.documentJSON(p, in.id, document))
						}
					}
				default:
					for i := range in.uploads {
						if upload := &in.uploads[i]; upload.status() == match[3] {
							found = append(found, a.uploadedFileJSON(p, in.id, upload))
						}
					}
				}
			})
			// One list across all Docs & Files tools, whatever vault a record
			// stands in.
			sort.SliceStable(found, func(i, j int) bool { return found[i]["id"].(int64) > found[j]["id"].(int64) })
			return a.pageOf(found, page), true
		}
	}
	return nil, false
}

// vault is one place of a Docs & Files tool that holds entries: the tool itself,
// which is the vault at the top, or one of its folders.
//
// answers tells that the account hands out the three lists of the vault at all. A
// folder the source has put away hands out none of them and answers not-found for
// each, which is what the real account does for every archived folder, so what is
// in use below such a folder is beyond the reach of any run.
type vault struct {
	id        int64
	answers   bool
	folders   []Folder
	documents []Document
	uploads   []UploadedFile
}

// eachVault visits every vault of a project: the top of each Docs & Files tool,
// those that are switched off included, and every folder below it, however
// deep.
func (a *Account) eachVault(p *Project, visit func(vault)) {
	var down func(Folder)
	down = func(folder Folder) {
		visit(vault{id: folder.ID, answers: folder.status() == "active",
			folders: folder.Folders, documents: folder.Documents, uploads: folder.Uploads})
		for _, below := range folder.Folders {
			down(below)
		}
	}
	for t := range p.Dock {
		tool := &p.Dock[t]
		if tool.Type != "vaults" {
			continue
		}
		visit(vault{id: tool.ID, answers: true, folders: tool.Folders, documents: tool.Documents, uploads: tool.Uploads})
		for _, folder := range tool.Folders {
			down(folder)
		}
	}
}

// theVault finds the vault of a project a path names: a Docs & Files tool or one
// of its folders.
func (a *Account) theVault(project, id string) (*Project, vault, bool) {
	p := a.project(project)
	if p == nil {
		return nil, vault{}, false
	}
	var found *vault
	a.eachVault(p, func(in vault) {
		if found == nil && strconv.FormatInt(in.id, 10) == id {
			here := in
			found = &here
		}
	})
	if found == nil {
		return nil, vault{}, false
	}
	return p, *found, true
}

func (f *Folder) status() string {
	if f.Status == "" {
		return "active"
	}
	return f.Status
}

// shownStatus is what the folder says about its status where it is listed, which
// a test can make something a bundle has no word for.
func (f *Folder) shownStatus() string {
	if f.ShownStatus != "" {
		return f.ShownStatus
	}
	return f.status()
}

func (d *Document) status() string {
	if d.Status == "" {
		return "active"
	}
	return d.Status
}

func (d *Document) shownStatus() string {
	if d.ShownStatus != "" {
		return d.ShownStatus
	}
	return d.status()
}

func (u *UploadedFile) status() string {
	if u.Status == "" {
		return "active"
	}
	return u.Status
}

func (u *UploadedFile) shownStatus() string {
	if u.ShownStatus != "" {
		return u.ShownStatus
	}
	return u.status()
}

func (u *UploadedFile) urlName() string {
	if u.URLName != "" {
		return u.URLName
	}
	return u.Filename
}

// StoredKeyOfUpload is what the account serves the bytes of an upload's file
// under, made from the upload's own ID. Basecamp names no such key on an upload
// and nothing of a run ever sees this one: it is how a test says which file is to
// fail or to come out at another size.
func StoredKeyOfUpload(id int64) string { return FileKey(id) }

func (a *Account) folderJSON(p *Project, in int64, folder *Folder) map[string]any {
	answer := map[string]any{
		"id": folder.ID, "status": folder.shownStatus(), "title": folder.Title, "type": "Vault",
		"parent": map[string]any{"id": in, "type": "Vault"},
		// A folder of Basecamp takes no comment and no boost, so it has none of
		// either and neither list is ever worth asking for.
		"comments_count": 0, "boosts_count": 0,
		"vaults_url":    fmt.Sprintf("%sbuckets/%d/vaults/%d/vaults.json", a.root(), p.ID, folder.ID),
		"documents_url": fmt.Sprintf("%sbuckets/%d/vaults/%d/documents.json", a.root(), p.ID, folder.ID),
		"uploads_url":   fmt.Sprintf("%sbuckets/%d/vaults/%d/uploads.json", a.root(), p.ID, folder.ID),
		"created_at":    folder.CreatedAt, "updated_at": folder.UpdatedAt,
	}
	// The vault at the top of a tool is listed without a parent, and the account
	// can be told to list every vault that way.
	if a.VaultsWithoutAParent {
		delete(answer, "parent")
	}
	if folder.VisibleToClients != nil {
		answer["visible_to_clients"] = *folder.VisibleToClients
	}
	if folder.Creator != nil {
		answer["creator"] = a.person(*folder.Creator)
	}
	return answer
}

func (a *Account) documentJSON(p *Project, in int64, document *Document) map[string]any {
	answer := map[string]any{
		"id": document.ID, "status": document.shownStatus(), "title": document.Title, "type": "Document",
		"content": document.Content, "content_attachments": a.filesJSON(document.Files),
		"parent":         map[string]any{"id": in, "type": "Vault"},
		"comments_count": len(document.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, document.ID),
		"boosts_count": len(document.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, document.ID),
		"created_at": document.CreatedAt, "updated_at": document.UpdatedAt,
	}
	if document.VisibleToClients != nil {
		answer["visible_to_clients"] = *document.VisibleToClients
	}
	if document.Creator != nil {
		answer["creator"] = a.person(*document.Creator)
	}
	return answer
}

// uploadedFileJSON is an upload as Basecamp answers one, with every key a
// record of the account was read to carry. An upload that lives in a cloud names
// no stored file, so neither the name, the size, the type nor the address of one
// is in the answer.
//
// The keys the extractor reads nothing from are here so that it is seen reading
// nothing from them: `position`, which the format has no word for, and
// `inherits_status`, `height` and `width`, which a bundle says nothing of.
func (a *Account) uploadedFileJSON(p *Project, in int64, upload *UploadedFile) map[string]any {
	answer := map[string]any{
		"id": upload.ID, "status": upload.shownStatus(), "title": upload.Title, "type": "Upload",
		"description": upload.Description, "description_attachments": a.filesJSON(upload.Files),
		"parent":         map[string]any{"id": in, "type": "Vault"},
		"bucket":         map[string]any{"id": p.ID, "name": p.Name, "type": "Project"},
		"comments_count": len(upload.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, upload.ID),
		"boosts_count": len(upload.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, upload.ID),
		"subscription_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/subscription.json", a.root(), p.ID, upload.ID),
		"bookmark_url":     fmt.Sprintf("%sbuckets/%d/recordings/%d/bookmark.json", a.root(), p.ID, upload.ID),
		"url":              fmt.Sprintf("%sbuckets/%d/uploads/%d.json", a.root(), p.ID, upload.ID),
		"app_url":          fmt.Sprintf("https://3.basecamp.test/%s/buckets/%d/uploads/%d", a.ID, p.ID, upload.ID),
		"position":         upload.Position,
		"inherits_status":  true,
		"created_at":       upload.CreatedAt, "updated_at": upload.UpdatedAt,
	}
	if !upload.InTheCloud {
		answer["filename"] = upload.Filename
		answer["content_type"] = upload.ContentType
		answer["byte_size"] = upload.Bytes
		if upload.NoSize {
			answer["byte_size"] = nil
		}
		answer["width"], answer["height"] = 640, 480
		// The shape the account really answers with: the API's own address, no
		// `blobs` segment and no key of a stored file anywhere. Asked for, it
		// answers with a redirect to a store of its own, which nothing here
		// follows — the bytes of an upload come by its identifier.
		answer["download_url"] = fmt.Sprintf("%sbuckets/%d/uploads/%d/download/%s",
			a.root(), p.ID, upload.ID, url.PathEscape(upload.urlName()))
		if upload.DownloadURL != "" {
			answer["download_url"] = upload.DownloadURL
		}
		answer["app_download_url"] = fmt.Sprintf("https://3.basecamp.test/%s/buckets/%d/uploads/%d/download/%s",
			a.ID, p.ID, upload.ID, url.PathEscape(upload.urlName()))
	}
	if upload.VisibleToClients != nil {
		answer["visible_to_clients"] = *upload.VisibleToClients
	}
	if upload.Creator != nil {
		answer["creator"] = a.person(*upload.Creator)
	}
	return answer
}
