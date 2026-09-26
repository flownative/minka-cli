package basecamp_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The Docs & Files of the fake stand in its eleventh project: the first tool of
// the dock holds every case, the second holds one document, the third is
// switched off.
const docsProject = "7011"

func docsDock(a *fakebasecamp.Account) *fakebasecamp.Project { return &a.Projects[10] }

// eachFolderOfTheFake visits every folder of the Docs & Files tools of the fake's
// docs project, the tool that is switched off included.
func eachFolderOfTheFake(a *fakebasecamp.Account, visit func(*fakebasecamp.Folder)) {
	var down func([]fakebasecamp.Folder)
	down = func(folders []fakebasecamp.Folder) {
		for i := range folders {
			visit(&folders[i])
			down(folders[i].Folders)
		}
	}
	for t := range docsDock(a).Dock {
		down(docsDock(a).Dock[t].Folders)
	}
}

// theFolder, theDocument and theUpload find an entry of the fake by its ID, for a
// test that changes it.
func theFolder(t *testing.T, a *fakebasecamp.Account, id int64) *fakebasecamp.Folder {
	t.Helper()
	var found *fakebasecamp.Folder
	eachFolderOfTheFake(a, func(folder *fakebasecamp.Folder) {
		if folder.ID == id {
			found = folder
		}
	})
	if found == nil {
		t.Fatalf("the docs project holds no folder %d", id)
	}
	return found
}

func theDocument(t *testing.T, a *fakebasecamp.Account, id int64) *fakebasecamp.Document {
	t.Helper()
	var found *fakebasecamp.Document
	visit := func(documents []fakebasecamp.Document) {
		for i := range documents {
			if documents[i].ID == id {
				found = &documents[i]
			}
		}
	}
	for t := range docsDock(a).Dock {
		visit(docsDock(a).Dock[t].Documents)
	}
	eachFolderOfTheFake(a, func(folder *fakebasecamp.Folder) { visit(folder.Documents) })
	if found == nil {
		t.Fatalf("the docs project holds no document %d", id)
	}
	return found
}

func theUpload(t *testing.T, a *fakebasecamp.Account, id int64) *fakebasecamp.UploadedFile {
	t.Helper()
	var found *fakebasecamp.UploadedFile
	visit := func(uploads []fakebasecamp.UploadedFile) {
		for i := range uploads {
			if uploads[i].ID == id {
				found = &uploads[i]
			}
		}
	}
	for t := range docsDock(a).Dock {
		visit(docsDock(a).Dock[t].Uploads)
	}
	eachFolderOfTheFake(a, func(folder *fakebasecamp.Folder) { visit(folder.Uploads) })
	if found == nil {
		t.Fatalf("the docs project holds no upload %d", id)
	}
	return found
}

// Every Docs & Files tool that is switched on is a tool of the kind files, and
// its folders, documents and uploads come with what stands in them: a document
// with its title and its text, the mentions and the files of that text rewritten
// as everywhere, an upload with the name of its file as its title.
func TestTheDocsAndFilesOfAToolComeAsFoldersDocumentsAndUploads(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	tools := records(t, dir, bundle.Tools)
	for _, id := range []string{"9210", "9211"} {
		if kind := recordWithID(t, tools, id)["kind"]; kind != bundle.ToolFiles {
			t.Errorf("the Docs & Files tool %s comes as a tool of the kind %v", id, kind)
		}
	}

	want := map[string]any{
		"sourceId": "9240", "createdAt": "2025-06-02T08:00:00.000Z", "updatedAt": "2025-06-02T09:00:00.000Z", "createdBy": "101",
		"projectSourceId": docsProject, "toolSourceId": "9210", "folderSourceId": "9220",
		"title": "Chapter one", "status": bundle.StatusActive, "visibleToClients": true,
		"body": `<div>How it works, says <import-mention ref="102">Grace Sample</import-mention></div>` +
			`<import-attachment ref="attachment-5401"></import-attachment>`,
	}
	if got := recordWithID(t, records(t, dir, bundle.Documents), "9240"); !reflect.DeepEqual(got, want) {
		t.Errorf("the document with everything on it:\n got %v\nwant %v", got, want)
	}

	folder := recordWithID(t, records(t, dir, bundle.Folders), "9220")
	if folder["title"] != "Handbook" || folder["parentSourceId"] != nil || folder["toolSourceId"] != "9210" {
		t.Errorf("the folder at the top of its tool: %v", folder)
	}

	upload := recordWithID(t, records(t, dir, bundle.Uploads), "9260")
	if upload["title"] != "The mark we use" || upload["file"] != "upload-9260" || upload["folderSourceId"] != "9220" {
		t.Errorf("the upload with its file: %v", upload)
	}

	// A comment and a boost hang on a document and on an upload by their own
	// words, and the boosts of a comment come behind those of its parent.
	comment := recordWithID(t, records(t, dir, bundle.Comments), "9271")
	if comment["parentKind"] != bundle.ParentDocument || comment["parentSourceId"] != "9240" {
		t.Errorf("the comment of a document hangs on %v %v", comment["parentKind"], comment["parentSourceId"])
	}
	boost := recordWithID(t, records(t, dir, bundle.Reactions), "9283")
	if boost["targetKind"] != bundle.TargetUpload || boost["targetSourceId"] != "9260" {
		t.Errorf("the boost of an upload hangs on %v %v", boost["targetKind"], boost["targetSourceId"])
	}
	// The files of a document and of the description of an upload belong to what
	// carries the text.
	attachments := records(t, dir, bundle.Attachments)
	if got := recordWithID(t, attachments, "upload-9260")["ownerKind"]; got != bundle.OwnerUpload {
		t.Errorf("the file of an upload says ownerKind %v, want upload", got)
	}
	if got := recordWithID(t, attachments, "attachment-5402")["ownerKind"]; got != bundle.OwnerDocument {
		t.Errorf("the file of a document says ownerKind %v, want document", got)
	}
}

// The one rule that is the opposite of the rule everywhere else: a folder, a
// document and an upload say whether clients see them at **every** depth, exactly
// as the source has it, because Minka reads the word below the top of a tool as
// well. Something hidden there is what makes Minka hide the folder at the top with
// everything in it; an entry that came without the word would be imported in the
// open under a folder clients see, and what a client has seen cannot be taken
// back.
//
// Nothing of it is counted as a word left to what the entry hangs on: an entry of
// Docs & Files decides for itself wherever it stands.
func TestEveryEntryOfDocsAndFilesSaysWhatTheSourceSaysAboutClientsAtEveryDepth(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), docsProject)

	// The folder at the top is shown, the folder inside it is hidden, and so are
	// the document and the upload standing in that folder.
	for _, entry := range []struct {
		kind bundle.Kind
		id   string
		want any
	}{
		{bundle.Folders, "9220", true},
		{bundle.Folders, "9221", false},
		{bundle.Documents, "9240", true},
		{bundle.Documents, "9241", false},
		{bundle.Uploads, "9260", true},
		{bundle.Uploads, "9265", false},
	} {
		record := recordWithID(t, records(t, dir, entry.kind), entry.id)
		said, says := record["visibleToClients"]
		if !says {
			t.Errorf("%s %s says nothing about clients, and the source says %v", entry.kind, entry.id, entry.want)
			continue
		}
		if said != entry.want {
			t.Errorf("%s %s says %v about clients, want %v", entry.kind, entry.id, said, entry.want)
		}
	}

	// Where the source says nothing, nothing is written: in doubt, private.
	for _, entry := range []struct {
		kind bundle.Kind
		id   string
	}{{bundle.Folders, "9228"}, {bundle.Documents, "9244"}, {bundle.Uploads, "9267"}} {
		record := recordWithID(t, records(t, dir, entry.kind), entry.id)
		if said, says := record["visibleToClients"]; says {
			t.Errorf("%s %s says %v about clients although the source says nothing", entry.kind, entry.id, said)
		}
	}

	if result.VisibilityLeftToTheContainer != 0 {
		t.Errorf("%d words about clients were left to a container, and an entry of Docs & Files decides for itself at every depth",
			result.VisibilityLeftToTheContainer)
	}
}

// The word of a nested entry is the only thing that hides a folder at the top:
// where the source hides an entry below a folder it shows, the bundle has to say
// so at that entry. This holds the field down at every depth of the fake, so that
// a run which wrote it at the top alone goes red.
func TestNoEntryBelowATopFolderIsSilentAboutClientsWhereTheSourceSpeaks(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, docsProject)

	said := map[string]bool{}
	for _, kind := range []bundle.Kind{bundle.Folders, bundle.Documents, bundle.Uploads} {
		for _, record := range recordsOrNone(t, dir, kind) {
			if _, says := record["visibleToClients"]; says {
				said[record["sourceId"].(string)] = true
			}
		}
	}
	for _, id := range []string{"9220", "9221", "9240", "9241", "9260", "9265"} {
		if !said[id] {
			t.Errorf("entry %s is silent about clients although the source is not", id)
		}
	}

	// And the word follows the source when the source changes it: an entry the
	// source hides below a shown top says false, whatever its depth and whatever
	// became of it.
	hidden := false
	theDocument(t, account, 9255).VisibleToClients = &hidden
	dir, _ = extract(t, account, docsProject)
	if got := recordWithID(t, records(t, dir, bundle.Documents), "9255")["visibleToClients"]; got != false {
		t.Errorf("a document on the shelf that is hidden from clients says %v", got)
	}
}

// A folder stands in the file after the folder it is in, because the applier
// stops at a folder whose parent it has not seen yet. Each kind of a folder
// stands there from the bottom up, because Minka puts every new entry on top of
// its kind: so the last line of a folder is the one on top, and the order the
// source shows comes out again.
func TestTheFoldersOfATheBundleStandParentsFirstAndEachKindFromTheBottomUp(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	folders := records(t, dir, bundle.Folders)
	if got := sourceIDs(folders); !reflect.DeepEqual(got, []string{"9222", "9223", "9228", "9220", "9221"}) {
		t.Errorf("the folders stand in the order %v", got)
	}
	standsAt := map[string]int{}
	for i, record := range folders {
		standsAt[record["sourceId"].(string)] = i
	}
	for _, record := range folders {
		parent, inAFolder := record["parentSourceId"].(string)
		if inAFolder && standsAt[parent] > standsAt[record["sourceId"].(string)] {
			t.Errorf("folder %s stands before its parent %s", record["sourceId"], parent)
		}
	}

	// Per folder and per kind, the bundle is the source's order upside down, and
	// what has left the folder goes first, where it cannot push the rest about.
	if got := sourceIDs(records(t, dir, bundle.Documents)); !reflect.DeepEqual(got,
		[]string{"9246", "9247", "9245", "9244", "9255", "9252", "9240", "9241", "9248"}) {
		t.Errorf("the documents stand in the order %v", got)
	}
	if got := sourceIDs(records(t, dir, bundle.Uploads)); !reflect.DeepEqual(got,
		[]string{"9269", "9276", "9264", "9263", "9267", "9270", "9260", "9265"}) {
		t.Errorf("the uploads stand in the order %v", got)
	}
}

// There is no position in the format: Minka reads the order of the file and
// earns a warning for every record that carries one.
func TestNoEntryOfDocsAndFilesCarriesAPosition(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	for _, kind := range []bundle.Kind{bundle.Folders, bundle.Documents, bundle.Uploads} {
		for _, record := range records(t, dir, kind) {
			if _, said := record["position"]; said {
				t.Errorf("%s %s carries a position", kind, record["sourceId"])
			}
		}
	}
}

// The status of an entry is its own and is never the status of its folder: what
// stands in a folder the source archived or threw away comes in as it is and goes
// along through the folder, and a run that copied the folder's word onto it would
// put it away one by one and never bring it back with the folder.
func TestTheStatusOfAnEntryIsItsOwnAndNeverItsFoldersS(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), docsProject)

	if got := recordWithID(t, records(t, dir, bundle.Folders), "9222")["status"]; got != bundle.StatusArchived {
		t.Errorf("the folder on the shelf comes as %v", got)
	}
	// On the shelf and in the trash inside a folder that is in use: the entry
	// carries what became of **it**, and the folder above it is untouched.
	if got := recordWithID(t, records(t, dir, bundle.Documents), "9255")["status"]; got != bundle.StatusArchived {
		t.Errorf("the document on the shelf inside a folder in use comes as %v", got)
	}
	if got := recordWithID(t, records(t, dir, bundle.Uploads), "9270")["status"]; got != bundle.StatusTrashed {
		t.Errorf("the upload in the trash inside a folder in use comes as %v", got)
	}
	if got := recordWithID(t, records(t, dir, bundle.Folders), "9220")["status"]; got != bundle.StatusActive {
		t.Errorf("the folder holding them comes as %v, want active", got)
	}
	if got := result.Docs.FoldersByStatus; !reflect.DeepEqual(got,
		map[string]int{bundle.StatusActive: 3, bundle.StatusArchived: 1, bundle.StatusTrashed: 1}) {
		t.Errorf("the folders by status are %v", got)
	}
	if got := result.Docs.DocumentsByStatus; !reflect.DeepEqual(got,
		map[string]int{bundle.StatusActive: 6, bundle.StatusArchived: 2, bundle.StatusTrashed: 1}) {
		t.Errorf("the documents by status are %v", got)
	}
	if got := result.Docs.UploadsByStatus; !reflect.DeepEqual(got,
		map[string]int{bundle.StatusActive: 6, bundle.StatusArchived: 1, bundle.StatusTrashed: 1}) {
		t.Errorf("the uploads by status are %v", got)
	}
}

// The title of an upload is **what the source calls it**, which Basecamp fills
// from the name of the file and anybody can rename; the name of the file stands
// in where the source calls it nothing, because Minka derives nothing from a file
// and refuses an upload without a title. A title too long is shortened **the way
// Minka shortens the name of a file** for an upload somebody makes there: the
// extension stays and the stem before it is cut to make room, with no ellipsis.
func TestAnUploadCarriesTheTitleTheSourceGivesItAndElseTheNameOfItsFile(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), docsProject)

	uploads := records(t, dir, bundle.Uploads)
	// Renamed at the source: the title is the source's and not the file's name.
	if got := recordWithID(t, uploads, "9260")["title"]; got != "The mark we use" {
		t.Errorf("a renamed upload carries the title %v, want what the source calls it", got)
	}
	// Called nothing at the source: the name of the file stands in.
	if got := recordWithID(t, uploads, "9267")["title"]; got != "minutes.pdf" {
		t.Errorf("an upload the source calls nothing carries the title %v, want the name of its file", got)
	}
	if got := recordWithID(t, uploads, "9264")["title"]; got != "(no name)" {
		t.Errorf("an upload with neither a title nor a name carries the title %v", got)
	}
	if result.Docs.NamesMadeIntoATitle != 1 || result.Docs.WithoutAName != 1 {
		t.Errorf("%d titles made to fit and %d with neither a title nor a name, want one of each",
			result.Docs.NamesMadeIntoATitle, result.Docs.WithoutAName)
	}
	if result.Docs.TitlesFromTheFileName != 2 {
		t.Errorf("%d uploads take the name of their file as their title, want the two the source calls nothing",
			result.Docs.TitlesFromTheFileName)
	}

	// The rule read off ItemTitle::fromFilename: a dot and up to eight letters
	// or digits at the end are the extension and stay, the stem is cut to make
	// room, and a name whose end is no extension of that shape is cut like any
	// other text. Nothing ends in an ellipsis: Minka writes none either.
	for name, want := range map[string]string{
		strings.Repeat("a", 260) + ".pdf":       strings.Repeat("a", 196) + ".pdf",
		strings.Repeat("a", 260):                strings.Repeat("a", 200),
		strings.Repeat("a", 260) + ".toolong":   strings.Repeat("a", 192) + ".toolong",
		strings.Repeat("a", 260) + ".extension": strings.Repeat("a", 200),
		strings.Repeat("ä", 260) + ".png":       strings.Repeat("ä", 196) + ".png",
	} {
		account := fakebasecamp.New()
		theUpload(t, account, 9267).Title, theUpload(t, account, 9267).Filename = "", name
		dir, _ := extract(t, account, docsProject)
		if got := recordWithID(t, records(t, dir, bundle.Uploads), "9267")["title"]; got != want {
			t.Errorf("a name of %d characters became a title of %d, want the stem cut and the extension kept",
				len([]rune(name)), len([]rune(got.(string))))
		}
	}
	if got := recordWithID(t, uploads, "9263")["title"].(string); !strings.HasSuffix(got, ".jpeg") || strings.Contains(got, "…") {
		t.Errorf("the long name of the account lost its extension or gained an ellipsis")
	}

	// Whatever the source calls a file, what comes out is a title Minka takes.
	for _, name := range []string{"", "  ", "a\nb", strings.Repeat("ä", 500), string([]byte{0xff, 'x'}), ".pdf",
		strings.Repeat("ä", 500) + ".pdf"} {
		account := fakebasecamp.New()
		theUpload(t, account, 9267).Title, theUpload(t, account, 9267).Filename = "", name
		dir, _ := extract(t, account, docsProject)
		title := recordWithID(t, records(t, dir, bundle.Uploads), "9267")["title"].(string)
		switch {
		case strings.TrimSpace(title) == "":
			t.Errorf("the name %q became a title of nothing", name)
		case len([]rune(title)) > 200:
			t.Errorf("the name %q became a title of %d characters", name, len([]rune(title)))
		case strings.ContainsFunc(title, unicode.IsControl):
			t.Errorf("the name %q became a title holding a character nobody can see", name)
		}
	}
}

// An upload Basecamp names no stored file for — one that lives in a cloud — is
// left out and named by ID, and its comments and its boosts go with it rather
// than being asked for: a comment pointing at an upload the bundle does not carry
// would stop the applier.
func TestAnUploadWithoutAStoredFileIsLeftOutWithEverythingPointingAtIt(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, docsProject)

	for _, record := range records(t, dir, bundle.Uploads) {
		if record["sourceId"] == "9262" {
			t.Error("the upload Basecamp names no file for is in the bundle")
		}
	}
	if got := result.Docs.WithoutAStoredFile; !reflect.DeepEqual(got, []string{"9262"}) {
		t.Errorf("the uploads without a stored file are %v, want the one in the cloud", got)
	}
	if result.Docs.CommentsWithoutTheirUpload != 1 || result.Docs.BoostsWithoutTheirUpload != 1 {
		t.Errorf("%d comments and %d boosts fell with it, want one of each",
			result.Docs.CommentsWithoutTheirUpload, result.Docs.BoostsWithoutTheirUpload)
	}
	for _, record := range records(t, dir, bundle.Comments) {
		if record["parentSourceId"] == "9262" {
			t.Errorf("comment %s hangs on an upload the bundle does not carry", record["sourceId"])
		}
	}
	for _, record := range records(t, dir, bundle.Reactions) {
		if record["targetSourceId"] == "9262" {
			t.Errorf("boost %s hangs on an upload the bundle does not carry", record["sourceId"])
		}
	}
	// Neither list is asked for: nothing of such an upload costs a request.
	for _, asked := range account.Requests {
		if strings.Contains(asked, "/9262/") {
			t.Errorf("the run asked for %s of an upload it leaves out", asked)
		}
	}
}

// A stored file has one place in Minka, so the file of an upload never carries
// the reference a text of the bundle uses for a file: the text would claim the
// blob, and the upload would stay out with a warning nobody reads.
func TestTheFileOfAnUploadIsNeverTheReferenceOfAFileInAText(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	inATextOrAnUpload := map[string]string{}
	for _, record := range records(t, dir, bundle.Uploads) {
		inATextOrAnUpload[record["file"].(string)] = "the file of upload " + record["sourceId"].(string)
	}
	for _, kind := range []bundle.Kind{bundle.Documents, bundle.Uploads, bundle.Comments} {
		for _, record := range recordsOrNone(t, dir, kind) {
			text, _ := record["body"].(string)
			if description, has := record["description"].(string); has {
				text += description
			}
			for _, ref := range referencesIn(text) {
				if where, claimed := inATextOrAnUpload[ref]; claimed {
					t.Errorf("%s %s names %s in its text, which is %s", kind, record["sourceId"], ref, where)
				}
			}
		}
	}
}

// referencesIn are the files a text of a bundle refers to.
func referencesIn(text string) []string {
	var refs []string
	for _, part := range strings.Split(text, `<import-attachment ref="`)[1:] {
		if end := strings.Index(part, `"`); end >= 0 {
			refs = append(refs, part[:end])
		}
	}
	return refs
}

// A folder whose status a bundle has no word for is left out, and everything
// below it falls with it: a folder, a document and an upload the run would
// otherwise have written under a place that is not there. The place is named with
// how much fell, because a number alone says that something is gone and never
// which.
func TestAFolderTheBundleCannotCarryTakesEverythingBelowItAndIsNamed(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), docsProject)

	for kind, id := range map[bundle.Kind]string{bundle.Folders: "9226", bundle.Documents: "9243", bundle.Uploads: "9266"} {
		for _, record := range records(t, dir, kind) {
			if record["sourceId"] == id {
				t.Errorf("%s %s stood in a folder the bundle leaves out and is in the bundle", kind, id)
			}
		}
	}
	held := DocsPlace(t, result, "9224")
	if held.Record != "folder" || held.Folders != 1 || held.Documents != 1 || held.Uploads != 1 {
		t.Errorf("with the folder the bundle leaves out fell %v", held)
	}
	// The word that made it impossible is counted with every other such record.
	found := false
	for _, one := range result.RecordsLeftOut {
		if one.Record == "folder" && one.Field == "status" && one.Value == "quarantined" && one.LeftOut == 1 {
			found = true
		}
	}
	if !found {
		t.Errorf("the word that left the folder out is not counted: %v", result.RecordsLeftOut)
	}
}

// DocsPlace is the place of a run's report that took entries with it.
func DocsPlace(t *testing.T, result basecamp.Result, id string) basecamp.DocsPlaceLeftOut {
	t.Helper()
	for _, place := range result.Docs.PlacesLeftOut {
		if place.SourceID == id {
			return place
		}
	}
	t.Fatalf("no place %s took entries with it: %v", id, result.Docs.PlacesLeftOut)
	return basecamp.DocsPlaceLeftOut{}
}

// An entry of a place no walk ever sees is counted all the same, with no place to
// name it against: a folder that is **in use** behind a Docs & Files tool that is
// switched off stands in none of the project's lists of records it has put away,
// and the tool is never read, so nothing can place the archived document inside
// it. Counted and not named is all a run can do; not counted at all would be a
// bundle saying it holds more of the source than it does.
func TestAnEntryOfAPlaceNoWalkEverSeesIsCounted(t *testing.T) {
	_, result := extract(t, fakebasecamp.New(), docsProject)

	// No place of the report is that folder: the walk never met it, so there is
	// nothing to name the document against.
	for _, place := range result.Docs.PlacesLeftOut {
		if place.SourceID == "9230" {
			t.Error("the folder in use behind the switch is named as a place, and no walk can see it")
		}
	}

	// Take the document out of the account and the number drops: it is that
	// record the count is about, and no other.
	account := fakebasecamp.New()
	theFolder(t, account, 9230).Documents = nil
	_, without := extract(t, account, docsProject)
	if without.Docs.LeftOut.Documents != result.Docs.LeftOut.Documents-1 {
		t.Errorf("without the document behind the switch %d documents are left out, want one fewer than %d",
			without.Docs.LeftOut.Documents, result.Docs.LeftOut.Documents)
	}
}

// A Docs & Files tool the source has switched off is not read at all, and nothing
// that stands in it reaches the bundle. What the project's own lists of records it
// has put away hand out of such a tool is counted and named with the tool it fell
// with, so that no run is silent about it.
func TestNothingOfADocsAndFilesToolThatIsSwitchedOffReachesTheBundle(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, docsProject)

	for _, record := range records(t, dir, bundle.Tools) {
		if record["sourceId"] == "9212" {
			t.Error("the Docs & Files tool that is switched off is a tool of the bundle")
		}
	}
	for kind, id := range map[bundle.Kind]string{bundle.Folders: "9227", bundle.Documents: "9251"} {
		for _, record := range records(t, dir, kind) {
			if record["sourceId"] == id {
				t.Errorf("%s %s of a tool that is switched off is in the bundle", kind, id)
			}
		}
	}
	closed := DocsPlace(t, result, "9212")
	if closed.Record != "Docs & Files tool" || closed.Folders != 1 || closed.Documents != 1 {
		t.Errorf("with the tool that is switched off fell %v", closed)
	}
	// Its own lists are never asked for.
	for _, asked := range account.Requests {
		if strings.Contains(asked, "/vaults/9212/") {
			t.Errorf("the run asked for %s of a tool that is switched off", asked)
		}
	}
	// And the tool is counted among the skipped ones.
	skipped := false
	for _, one := range result.Skipped {
		if one.Kind == "vault" && !one.Enabled && one.Count == 1 {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("the Docs & Files tool that is switched off is not counted as skipped: %v", result.Skipped)
	}
}

// Minka takes neither a comment nor a reaction of a folder and no later run ever
// will, so a bundle carries none and a run does not even ask: a record that waited
// there for ever would be counted as waiting on every run.
func TestNoCommentAndNoBoostOfAFolderIsWrittenOrAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, docsProject)

	folders := map[string]bool{}
	for _, record := range records(t, dir, bundle.Folders) {
		folders[record["sourceId"].(string)] = true
	}
	for _, record := range records(t, dir, bundle.Comments) {
		if folders[record["parentSourceId"].(string)] {
			t.Errorf("comment %s hangs on a folder", record["sourceId"])
		}
	}
	for _, record := range records(t, dir, bundle.Reactions) {
		if folders[record["targetSourceId"].(string)] {
			t.Errorf("boost %s hangs on a folder", record["sourceId"])
		}
	}
	for _, asked := range account.Requests {
		for id := range folders {
			if strings.Contains(asked, "/recordings/"+id+"/") {
				t.Errorf("the run asked for %s of a folder", asked)
			}
		}
	}
}

// A document whose whole text is a file no run will ever bring **arrives
// wordless**: the tag left in would cost the document altogether, because Minka
// keeps nothing of such a text and asks for the file again on every run, so the
// title, the comments and the boosts would arrive on no run while the file is
// gone. It gets no line of the run's own as a mail does — there the body is a
// field that must not be empty, and here it is not.
//
// An upload is the one of the three that cannot come without its file, the field
// being required, and it is named for that.
func TestADocumentThatSaysNothingButAFileThatNeverComesArrivesWordless(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), docsProject)

	held := recordWithID(t, records(t, dir, bundle.Documents), "9245")
	if held["body"] != nil {
		t.Errorf("the document of nothing but a file carries %v, want no words at all", held["body"])
	}
	if held["title"] != "The picture" || held["status"] != bundle.StatusActive {
		t.Errorf("the wordless document lost its title or its status: %v", held)
	}
	// It arrives, and so does everything that hangs on it, which is the whole
	// point of taking the tag out.
	if result.Docs.DocumentsWithoutWords != 1 {
		t.Errorf("%d documents came without words, want the one", result.Docs.DocumentsWithoutWords)
	}
	if got := recordWithID(t, records(t, dir, bundle.Uploads), "9276")["file"]; got != "upload-9276" {
		t.Errorf("the upload whose file is gone names %v", got)
	}
	if got := result.Docs.OnlyAFileNoRunWillBring; !reflect.DeepEqual(got, []string{"9245", "9276"}) {
		t.Errorf("the records that stand or fall with a file that never comes are %v", got)
	}

	// A document that says something beside the file keeps its words, loses the
	// file alone, and is named for nothing: Minka writes it as it stands. The
	// words are put behind the tag as well as before it, because a reader that
	// gave up at the first file would never see them.
	tag := `<import-attachment ref="attachment-5402"></import-attachment>`
	words := `<div>See this</div>`
	for _, before := range []bool{true, false} {
		account := fakebasecamp.New()
		embed := theDocument(t, account, 9245).Content
		body := tag + words
		if before {
			theDocument(t, account, 9245).Content, body = words+embed, words+tag
		} else {
			theDocument(t, account, 9245).Content = embed + words
		}
		dir, result = extract(t, account, docsProject)
		if got := result.Docs.OnlyAFileNoRunWillBring; !reflect.DeepEqual(got, []string{"9276"}) {
			t.Errorf("a document with words beside its file is named among %v", got)
		}
		if got := recordWithID(t, records(t, dir, bundle.Documents), "9245")["body"]; got != body {
			t.Errorf("a document with words beside its file carries %v, want %v", got, body)
		}
	}

	// A file this run merely could not fetch is no file that will never come:
	// the upload waits for it, and the run after this one brings both.
	account := fakebasecamp.New()
	theUpload(t, account, 9276).Gone = false
	account.FailFileOf[fakebasecamp.StoredKeyOfUpload(9276)] = &basecamp.Error{Kind: basecamp.Network, Message: "the network failed"}
	_, result = extract(t, account, docsProject)
	if got := result.Docs.OnlyAFileNoRunWillBring; !reflect.DeepEqual(got, []string{"9245"}) {
		t.Errorf("an upload whose file only failed this time is named among %v", got)
	}
}

// A document or a description Minka will pass over for its length is counted: it
// is lost there on every run, with its comments and its boosts, and the text is
// not cut short.
func TestADocumentLongerThanMinkaHoldsIsCounted(t *testing.T) {
	account := fakebasecamp.New()
	theDocument(t, account, 9244).Content = strings.Repeat("a", 1000001)
	theUpload(t, account, 9267).Description = strings.Repeat("b", 1000001)
	_, result := extract(t, account, docsProject)

	if result.Docs.LongerThanMinkaTakes != 2 {
		t.Errorf("%d texts longer than Minka holds, want the document and the description", result.Docs.LongerThanMinkaTakes)
	}
	// The text comes as it is: the longest of the kind is as long as the source
	// wrote it, and nothing was cut.
	if result.LongestBody[bundle.Documents] != 1000001 || result.LongestBody[bundle.Uploads] != 1000001 {
		t.Errorf("the longest document is %d and the longest description %d",
			result.LongestBody[bundle.Documents], result.LongestBody[bundle.Uploads])
	}
	for _, kind := range []bundle.Kind{bundle.Documents, bundle.Uploads} {
		if got := result.LongestBodyRecords[kind]; got.SourceID == "" || got.In != "project" {
			t.Errorf("the longest %s names no record: %v", kind, got)
		}
	}
}

// A walk is no snapshot: a list that moved under it can hand one record out
// twice, and the bundle writes it once.
func TestAnEntryTheSourceListsTwiceIsWrittenOnce(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	for _, kind := range []bundle.Kind{bundle.Folders, bundle.Documents, bundle.Uploads} {
		seen := map[string]int{}
		for _, record := range records(t, dir, kind) {
			seen[record["sourceId"].(string)]++
		}
		for id, times := range seen {
			if times > 1 {
				t.Errorf("%s %s stands in the bundle %d times", kind, id, times)
			}
		}
	}
}

// Every time of every record is read where it is read: a folder, a document or an
// upload the source says no time of, or a time that is none of, ends the run
// naming the record and the field, because the applier decides by updatedAt
// whether a run changes a record and a bundle never carries a guess.
func TestATimeOfAnEntryThatIsNoTimeEndsTheRun(t *testing.T) {
	for name, broken := range map[string]func(*fakebasecamp.Account){
		"a folder without a time":      func(a *fakebasecamp.Account) { theFolder(t, a, 9220).UpdatedAt = "" },
		"a folder at no time":          func(a *fakebasecamp.Account) { theFolder(t, a, 9220).CreatedAt = "yesterday" },
		"a document without a time":    func(a *fakebasecamp.Account) { theDocument(t, a, 9244).CreatedAt = "" },
		"a document at no time":        func(a *fakebasecamp.Account) { theDocument(t, a, 9244).UpdatedAt = "2025-06-07T08:00:00" },
		"an upload without a time":     func(a *fakebasecamp.Account) { theUpload(t, a, 9267).UpdatedAt = "" },
		"an upload on a day that is n": func(a *fakebasecamp.Account) { theUpload(t, a, 9267).CreatedAt = "2025-02-30T08:00:00Z" },
	} {
		account := fakebasecamp.New()
		broken(account)
		failed := extractExpectingAnError(t, account, docsProject)
		if failed.Kind != basecamp.API {
			t.Errorf("%s ended the run as %v, want an api_error", name, failed.Kind)
		}
	}
}

// A title Minka refuses ends the run instead of reaching it: a folder and a
// document carry what somebody typed, and somebody can put that right at the
// source. No message quotes it.
func TestATitleOfAFolderOrADocumentMinkaRefusesEndsTheRun(t *testing.T) {
	for name, broken := range map[string]func(*fakebasecamp.Account){
		"a folder without a title":     func(a *fakebasecamp.Account) { theFolder(t, a, 9220).Title = "  " },
		"a folder with a long title":   func(a *fakebasecamp.Account) { theFolder(t, a, 9220).Title = strings.Repeat("a", 201) },
		"a document without a title":   func(a *fakebasecamp.Account) { theDocument(t, a, 9244).Title = "" },
		"a document with a line break": func(a *fakebasecamp.Account) { theDocument(t, a, 9244).Title = "one\ntwo" },
	} {
		account := fakebasecamp.New()
		broken(account)
		failed := extractExpectingAnError(t, account, docsProject)
		if failed.Kind != basecamp.API {
			t.Errorf("%s ended the run as %v, want an api_error", name, failed.Kind)
		}
		if strings.Contains(failed.Message, "one\ntwo") || strings.Contains(failed.Message, strings.Repeat("a", 201)) {
			t.Errorf("%s: the message quotes the title", name)
		}
	}
}

// A dry run walks the same way and brings no file. Two things it cannot know,
// because all it has of a file is the size the source gives it: which record
// stands or falls with a file the source no longer has, and therefore which
// document arrives wordless for it.
func TestADryRunOfDocsAndFilesWalksTheSameWay(t *testing.T) {
	account := fakebasecamp.New()
	_, wet := extract(t, account, docsProject)

	dry := fakebasecamp.New()
	writer := bundle.Discard()
	extractor := basecamp.Extractor{Source: dry, Avatars: dry, Files: dry}
	result, err := extractor.Extract(t.Context(), []string{docsProject}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if result.Docs.DocumentsWithoutWords != wet.Docs.DocumentsWithoutWords-1 {
		t.Errorf("a dry run says %d documents came without words and a run says %d, want one fewer",
			result.Docs.DocumentsWithoutWords, wet.Docs.DocumentsWithoutWords)
	}
	want := asADryRunKnowsIt(wet).Docs
	want.DocumentsWithoutWords = result.Docs.DocumentsWithoutWords
	if got := result.Docs; !reflect.DeepEqual(got, want) {
		t.Errorf("a dry run says of the Docs & Files\n got %v\nwant %v", got, want)
	}
	if len(dry.Downloads) > 0 {
		t.Errorf("a dry run downloaded %d files", len(dry.Downloads))
	}
	if !reflect.DeepEqual(dry.Requests, account.Requests) {
		t.Error("a dry run asked Basecamp for something else than a run")
	}
}

// A bundle is closed under its own references or it is no bundle: a folder, a
// document or an upload naming a folder the bundle does not carry stops the
// applier where it reads the field, after a walk of hours, and so does a comment
// or a boost naming an entry that is not there. This holds every reference of
// the Docs & Files down at once.
func TestEveryReferenceOfDocsAndFilesLeadsToARecordOfTheBundle(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	of := func(kind bundle.Kind) map[string]bool {
		known := map[string]bool{}
		for _, record := range recordsOrNone(t, dir, kind) {
			known[record["sourceId"].(string)] = true
		}
		return known
	}
	folders, documents, uploads := of(bundle.Folders), of(bundle.Documents), of(bundle.Uploads)
	tools, projects, files := of(bundle.Tools), of(bundle.Projects), of(bundle.Attachments)
	entries := map[string]map[string]bool{
		bundle.OwnerDocument: documents, bundle.OwnerUpload: uploads, "folder": folders,
	}

	for kind, in := range map[bundle.Kind]string{bundle.Folders: "parentSourceId", bundle.Documents: "folderSourceId", bundle.Uploads: "folderSourceId"} {
		for _, record := range records(t, dir, kind) {
			if !tools[record["toolSourceId"].(string)] {
				t.Errorf("%s %v names a tool that is not in the bundle", kind, record["sourceId"])
			}
			if !projects[record["projectSourceId"].(string)] {
				t.Errorf("%s %v names a project that is not in the bundle", kind, record["sourceId"])
			}
			if folder, inAFolder := record[in].(string); inAFolder && !folders[folder] {
				t.Errorf("%s %v stands in folder %s, which is not in the bundle", kind, record["sourceId"], folder)
			}
		}
	}
	for _, record := range records(t, dir, bundle.Uploads) {
		if !files[record["file"].(string)] {
			t.Errorf("upload %v names a file that has no record in the bundle", record["sourceId"])
		}
	}
	for _, record := range records(t, dir, bundle.Comments) {
		if known, ofADocsEntry := entries[record["parentKind"].(string)]; ofADocsEntry && !known[record["parentSourceId"].(string)] {
			t.Errorf("comment %v hangs on a %v that is not in the bundle", record["sourceId"], record["parentKind"])
		}
	}
	for _, record := range records(t, dir, bundle.Reactions) {
		if known, ofADocsEntry := entries[record["targetKind"].(string)]; ofADocsEntry && !known[record["targetSourceId"].(string)] {
			t.Errorf("boost %v hangs on a %v that is not in the bundle", record["sourceId"], record["targetKind"])
		}
	}
	// Every file of a document or an upload belongs to one that is in the bundle.
	for _, record := range records(t, dir, bundle.Attachments) {
		if known, ofADocsEntry := entries[record["ownerKind"].(string)]; ofADocsEntry && !known[record["ownerSourceId"].(string)] {
			t.Errorf("file %v belongs to a %v that is not in the bundle", record["sourceId"], record["ownerKind"])
		}
	}
}

// A folder the source has put away may hand out no list of what stands in it:
// that is passed over, and what is in use below such a folder is then beyond the
// reach of any run, while what the source has put away comes from the project's
// own lists either way. A folder that is in use has to answer, and a run that is
// told there is no such list ends rather than writing a folder the source shows
// as an empty one.
func TestAListOfAFolderThatIsPutAwayMayBeGoneAndOneOfAFolderInUseMayNot(t *testing.T) {
	gone := func() error { return &basecamp.Error{Kind: basecamp.NotFound, Message: "Basecamp: resource not found"} }

	account := fakebasecamp.New()
	for _, path := range []string{"documents", "uploads", "vaults"} {
		account.Fail["buckets/7011/vaults/9222/"+path+".json"] = gone()
		account.Fail["buckets/7011/vaults/9223/"+path+".json"] = gone()
	}
	dir, result := extract(t, account, docsProject)
	// The folders themselves still come: the project's own lists of records it
	// has put away hand them out, and no list of theirs is needed for that.
	folders := records(t, dir, bundle.Folders)
	for _, id := range []string{"9222", "9223"} {
		recordWithID(t, folders, id)
	}
	// What is in use below them reaches no bundle, and nothing can count it:
	// the run says how many folders would not say what stands in them.
	for kind, id := range map[bundle.Kind]string{bundle.Documents: "9242", bundle.Uploads: "9261"} {
		for _, record := range records(t, dir, kind) {
			if record["sourceId"] == id {
				t.Errorf("%s %s came out of a folder that would not say what stands in it", kind, id)
			}
		}
	}
	if result.Docs.FoldersThatSaidNothingOfWhatIsInThem != 2 {
		t.Errorf("%d folders would not say what stands in them, want the two",
			result.Docs.FoldersThatSaidNothingOfWhatIsInThem)
	}

	// The same of a folder that is in use ends the run: a folder the source
	// shows and the bundle writes empty is a folder somebody will look into.
	account = fakebasecamp.New()
	account.Fail["buckets/7011/vaults/9220/documents.json"] = gone()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	_, err := extractor.Extract(t.Context(), []string{docsProject}, bundle.Discard())
	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.NotFound {
		t.Errorf("a list of a folder in use that is not there ended the run as %v, want not found", err)
	}
}

// The file of an upload is fetched **by the identifier of the upload and its
// project**, and the address the source gives is asked for by nothing: Basecamp
// names no stored file on such a record, and the address it does give answers with
// a redirect to a store of its own that no run follows. Whatever shape that
// address has, the file comes.
//
// This is the rule a bundle of the real account was measured against: reading the
// address for a key of a stored file, as a file inside a text is read, left every
// upload of every tool out of the bundle, counted as an upload the source names no
// file for, and gone.
func TestTheFileOfAnUploadIsFetchedByItsIdentifierAndNotByAnyAddress(t *testing.T) {
	for what, address := range map[string]string{
		"the address the account really gives":     "",
		"an address of a store and not of the app": "https://storage.3.basecamp.com/4242/blobs/5a1b2c3d-9267-4000-8000-000000009267/download/x.pdf",
		"an address with no key in it":             "https://storage.3.basecamp.com/4242/uploads/9267/download/x.pdf",
		"an address of no shape anybody has seen":  "https://example.test/whatever",
		"no address of a shape at all":             "not an address",
	} {
		account := fakebasecamp.New()
		if address != "" {
			theUpload(t, account, 9267).DownloadURL = address
		}
		dir, result := extract(t, account, docsProject)

		upload := recordWithID(t, records(t, dir, bundle.Uploads), "9267")
		if upload["file"] != "upload-9267" {
			t.Errorf("%s: the upload names %v, want its file", what, upload["file"])
		}
		file := recordWithID(t, records(t, dir, bundle.Attachments), "upload-9267")
		if file["gone"] != nil || file["bytes"] != float64(1536) {
			t.Errorf("%s: the file came as %v, want the bytes the source says it has", what, file)
		}
		if len(result.Docs.WithoutAStoredFile) != 1 || result.Docs.WithoutAStoredFile[0] != "9262" {
			t.Errorf("%s: the uploads said to name no stored file are %v, want the one in the cloud alone",
				what, result.Docs.WithoutAStoredFile)
		}
		// Asked for by the two identifiers, and at no address.
		if !slices.Contains(account.UploadDownloads, "7011/9267") {
			t.Errorf("%s: the file was not asked for by the upload and its project: %v", what, account.UploadDownloads)
		}
		for _, asked := range account.Downloads {
			if strings.Contains(asked, "9267") {
				t.Errorf("%s: the file was asked for at the address %s", what, asked)
			}
		}
	}
}

// A vault record that names no vault above it is the vault at the **top of a
// tool** and no folder of one: the source lists every dock entry that way. Such a
// record stands in nothing and nothing can place it, so it becomes a place of its
// own, counted and named with whatever stood in it, rather than falling out of the
// walk without a word.
//
// No record of the account has that shape, because only the records the source has
// put away are read and no whole Docs & Files tool of it is put away. The shape is
// invented, and a fixture makes it by taking a folder's parent off.
func TestAVaultRecordThatNamesNoVaultAboveItIsAPlaceOfItsOwn(t *testing.T) {
	_, normal := extract(t, fakebasecamp.New(), docsProject)

	account := fakebasecamp.New()
	account.VaultsWithoutAParent = true
	dir, result := extract(t, account, docsProject)

	// The two folders the source has put away name no vault above them now, so
	// neither is in the bundle and both are counted among the folders it does not
	// carry — where before they were carried.
	for _, record := range records(t, dir, bundle.Folders) {
		if record["sourceId"] == "9222" || record["sourceId"] == "9223" {
			t.Errorf("folder %v names no vault above it and is in the bundle all the same", record["sourceId"])
		}
	}
	if result.Docs.LeftOut.Folders != normal.Docs.LeftOut.Folders+2 {
		t.Errorf("%d folders are left out and %d were before, want two more: the ones that name no vault",
			result.Docs.LeftOut.Folders, normal.Docs.LeftOut.Folders)
	}
	// And they are places, so anything the source had put away inside them would
	// be counted against them rather than dropped without a number.
	asked := 0
	for _, request := range account.Requests {
		if strings.Contains(request, "/vaults/9222/") || strings.Contains(request, "/vaults/9223/") {
			asked++
		}
	}
	if asked == 0 {
		t.Error("neither was asked what stood in it, so nothing of them could be counted")
	}
}

// What is in use stands in the file from the bottom up, so Minka puts it back in
// the source's order; what the source has put away stands **before** all of it and
// in the source's own order, not upside down. Nothing promises more than that: an
// entry that has left its place shows in no list Minka orders, and reversing those
// too would be a promise the code does not keep.
func TestOnlyWhatIsInUseStandsInTheFileFromTheBottomUp(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), docsProject)

	// The account lists the documents it has put away newest first, as it lists
	// every record of that kind, and the bundle keeps that order for them while
	// turning the ones in use round.
	standsAt := map[string]int{}
	for i, record := range records(t, dir, bundle.Documents) {
		standsAt[record["sourceId"].(string)] = i
	}
	for _, order := range [][]string{
		// Put away at the top of the tool, in the order the account lists them.
		{"9246", "9247"},
		// In use at the top of the tool, the other way round from the account.
		{"9245", "9244"},
		// And everything put away stands before everything in use.
		{"9247", "9245"},
	} {
		if standsAt[order[0]] >= standsAt[order[1]] {
			t.Errorf("document %s stands at %d and %s at %d, want the first before the second",
				order[0], standsAt[order[0]], order[1], standsAt[order[1]])
		}
	}
}

// An entry standing inside a folder that says nothing about whether clients see
// it is counted, because the silence can cost the one rule this tool is written
// for: nothing is written, Minka reads such an entry as one that changes nothing
// above it, and a folder at the top that ought to be hidden from clients stays
// shown. A number is all a run can do about it, and a run that said nothing would
// let that harm arrive through the safest-looking answer the source has.
//
// At the top of a tool silence costs nothing — Minka reads an absent word as
// private — so it is not counted there.
func TestAnEntryInsideAFolderThatSaysNothingAboutClientsIsNamed(t *testing.T) {
	// Three entries of the account stand inside a folder and say nothing: the two
	// documents of the folder at the top that nobody gave the word, and the upload
	// in its trash.
	_, before := extract(t, fakebasecamp.New(), docsProject)
	if len(before.Docs.SilentAboutClientsInsideAFolder) != 3 {
		t.Errorf("%d entries inside a folder are silent about clients, want the three the source says nothing of",
			len(before.Docs.SilentAboutClientsInsideAFolder))
	}

	// Take the word off the three that stand inside the folder below the top, and
	// those are counted too.
	account := fakebasecamp.New()
	theFolder(t, account, 9221).VisibleToClients = nil
	theDocument(t, account, 9241).VisibleToClients = nil
	theUpload(t, account, 9265).VisibleToClients = nil
	dir, result := extract(t, account, docsProject)
	if len(result.Docs.SilentAboutClientsInsideAFolder) != len(before.Docs.SilentAboutClientsInsideAFolder)+3 {
		t.Errorf("%d entries inside a folder are silent about clients and %d were before, want three more: the folder, the document and the upload",
			len(result.Docs.SilentAboutClientsInsideAFolder), len(before.Docs.SilentAboutClientsInsideAFolder))
	}
	// Each is **named** and not only counted: the one thing to do about such an
	// entry is to go and compare it against the source, which nobody can do with a
	// number. The kind, the record and its project, and never a title.
	named := map[string]basecamp.EntrySilentAboutClients{}
	for _, entry := range result.Docs.SilentAboutClientsInsideAFolder {
		named[entry.SourceID] = entry
	}
	for id, record := range map[string]string{"9221": "folder", "9241": "document", "9265": "upload"} {
		entry, there := named[id]
		switch {
		case !there:
			t.Errorf("entry %s is silent about clients and is not named: %v", id, result.Docs.SilentAboutClientsInsideAFolder)
		case entry.Record != record:
			t.Errorf("entry %s is named a %s, want a %s", id, entry.Record, record)
		case entry.ProjectSourceID != docsProject:
			t.Errorf("entry %s is named in project %s, want %s", id, entry.ProjectSourceID, docsProject)
		}
	}

	// Nothing is written for them: in doubt private is Minka's to decide.
	for kind, id := range map[bundle.Kind]string{bundle.Folders: "9221", bundle.Documents: "9241", bundle.Uploads: "9265"} {
		if _, says := recordWithID(t, records(t, dir, kind), id)["visibleToClients"]; says {
			t.Errorf("%s %s says something about clients although the source says nothing", kind, id)
		}
	}

	// At the top of a tool the same silence is not counted: Minka reads an absent
	// word as private there and nothing above it can be lost.
	account = fakebasecamp.New()
	theFolder(t, account, 9220).VisibleToClients = nil
	_, result = extract(t, account, docsProject)
	if len(result.Docs.SilentAboutClientsInsideAFolder) != len(before.Docs.SilentAboutClientsInsideAFolder) {
		t.Errorf("%d entries are counted where only the top of a tool fell silent, want the %d there were",
			len(result.Docs.SilentAboutClientsInsideAFolder), len(before.Docs.SilentAboutClientsInsideAFolder))
	}
}
