package basecamp_test

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The upload of the fake that was given another file twice, once by somebody
// other than whoever made it, and whose list of versions names one more without
// its file.
const (
	replacedUpload = 9277
	versionsOfIt   = "buckets/7011/uploads/9277/versions.json"
)

// extractWithVersions is an extraction asked to bring the files the uploads
// showed before.
func extractWithVersions(t *testing.T, account *fakebasecamp.Account, projects ...string) (string, basecamp.Result) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	writer, err := bundle.Create(dir, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, UploadVersions: true,
		Now: func() time.Time { return runDay }}
	result, err := extractor.Extract(t.Context(), projects, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Commit(bundle.Manifest{SourceInstance: result.SourceInstance}); err != nil {
		t.Fatal(err)
	}
	return dir, result
}

// runWithVersions walks the projects asked to bring the earlier files and
// writes no bundle.
func runWithVersions(t *testing.T, account *fakebasecamp.Account, projects ...string) error {
	t.Helper()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, UploadVersions: true,
		Now: func() time.Time { return runDay }}
	_, err := extractor.Extract(t.Context(), projects, bundle.Discard())
	return err
}

// **Without being asked, a run brings no earlier file**: it asks for no list of
// versions and no version, so no token passes through minka, and the bundle and
// the report are those of an account whose uploads were never given another
// file, byte for byte.
func TestWithoutBeingAskedARunBringsNoEarlierFileAndWritesWhatItAlwaysHas(t *testing.T) {
	replaced := fakebasecamp.New()
	dir, result := extract(t, replaced, docsProject)

	never := fakebasecamp.New()
	upload := theUpload(t, never, replacedUpload)
	upload.EarlierFiles, upload.VersionID, upload.VersionCreatedAt = nil, 0, ""
	plainDir, plain := extract(t, never, docsProject)

	if got := asked(replaced, "/versions.json"); len(got) != 0 {
		t.Errorf("a run not asked for versions asked for %v", got)
	}
	if len(replaced.VersionDownloads) != 0 {
		t.Errorf("a run not asked for versions fetched %v", replaced.VersionDownloads)
	}
	if !reflect.DeepEqual(replaced.Requests, never.Requests) {
		t.Error("a run not asked for versions asked Basecamp for something else than over an account without any")
	}
	files, plainFiles := everyFile(t, dir), everyFile(t, plainDir)
	delete(files, "manifest.json")
	delete(plainFiles, "manifest.json")
	if !reflect.DeepEqual(files, plainFiles) {
		t.Error("a run not asked for versions wrote another bundle than over an account without any")
	}
	report, _ := json.Marshal(result)
	plainReport, _ := json.Marshal(plain)
	if string(report) != string(plainReport) || strings.Contains(string(report), `"versions"`) {
		t.Errorf("a run not asked for versions reports\n %s\nwant\n %s", report, plainReport)
	}

	asked := fakebasecamp.New()
	if _, result := extractWithVersions(t, asked, docsProject); result.Docs.Versions == nil || result.Docs.Versions.EarlierFiles != 2 {
		t.Errorf("a run asked for versions says %+v of them", result.Docs.Versions)
	}
}

// An upload carries the files it showed before as its versions, oldest first
// and the file it shows now last, each earlier one a file of its own with the
// moment it was given and whoever gave it; nobody is named where that was
// whoever made the upload. The bytes are those of each version.
func TestAnUploadCarriesTheFilesItShowedBeforeOldestFirst(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extractWithVersions(t, account, docsProject)
	holdsTogether(t, dir, result.Missing)

	ada := "101"
	want := []any{
		map[string]any{"file": "upload-9277-version-9293", "createdAt": "2025-06-09T17:00:00.412Z", "createdBy": nil},
		map[string]any{"file": "upload-9277-version-9294", "createdAt": "2025-06-10T09:00:00.000Z", "createdBy": ada},
		map[string]any{"file": "upload-9277", "createdAt": "2025-06-12T09:00:00.000Z", "createdBy": nil},
	}
	uploads := records(t, dir, bundle.Uploads)
	if got := recordWithID(t, uploads, "9277")["versions"]; !reflect.DeepEqual(got, want) {
		t.Errorf("the versions of the upload:\n got %v\nwant %v", got, want)
	}
	// An upload never given another file carries none, rather than a list of
	// the one file it shows.
	for _, upload := range uploads {
		if _, carries := upload["versions"]; carries && upload["sourceId"] != "9277" {
			t.Errorf("the upload %v carries versions: %v", upload["sourceId"], upload["versions"])
		}
	}

	attachments := records(t, dir, bundle.Attachments)
	for ref, earlier := range map[string]struct {
		key    string
		name   string
		size   int64
		moment string
	}{
		"upload-9277-version-9293": {fakebasecamp.StoredKeyOfVersion(9293), "notes.pdf", 110, "2025-06-09T17:00:00.412Z"},
		"upload-9277-version-9294": {fakebasecamp.StoredKeyOfVersion(9294), "notes-draft.pdf", 120, "2025-06-10T09:00:00.000Z"},
	} {
		sum, _ := digestOf(t, fakebasecamp.FileContent(earlier.key, earlier.size))
		want := map[string]any{
			"sourceId": ref, "createdAt": earlier.moment, "updatedAt": earlier.moment, "createdBy": nil,
			"filename": earlier.name, "contentType": "application/pdf", "bytes": float64(earlier.size), "sha256": sum,
			"file": ref + "/" + earlier.name, "ownerKind": "upload", "ownerSourceId": "9277",
		}
		if got := recordWithID(t, attachments, ref); !reflect.DeepEqual(got, want) {
			t.Errorf("the earlier file %s:\n got %v\nwant %v", ref, got, want)
		}
	}
	recordWithID(t, records(t, dir, bundle.People), ada)

	versions := result.Docs.Versions
	if versions.UploadsWithEarlierFiles != 1 || versions.EarlierFiles != 2 {
		t.Errorf("the run counts %d earlier files of %d uploads, want 2 of 1", versions.EarlierFiles, versions.UploadsWithEarlierFiles)
	}
	if want := []basecamp.VersionWithoutAFile{{UploadSourceID: "9277", VersionSourceID: "9295"}}; !reflect.DeepEqual(versions.WithoutTheirFile, want) {
		t.Errorf("the versions named without their file are %v, want %v", versions.WithoutTheirFile, want)
	}
	if len(versions.HistoriesUnread) != 0 {
		t.Errorf("the run names %v as unread", versions.HistoriesUnread)
	}
}

// **An earlier file is never asked for by a route that can bring the file the
// upload shows now.** The Basecamp CLI serves the current file for the address of
// a version and ends well, so neither its size nor the end of the call proves
// anything: the earlier file here is as large as the current one, and only the
// route tells the two apart.
func TestAnEarlierFileIsFetchedAtItsVersionAndNeverByARouteThatBringsTheCurrentFile(t *testing.T) {
	account := fakebasecamp.New()
	theUpload(t, account, replacedUpload).EarlierFiles[0].Bytes = 130
	dir, result := extractWithVersions(t, account, docsProject)
	holdsTogether(t, dir, result.Missing)

	earlier, _ := digestOf(t, fakebasecamp.FileContent(fakebasecamp.StoredKeyOfVersion(9293), 130))
	current, _ := digestOf(t, fakebasecamp.FileContent(fakebasecamp.StoredKeyOfUpload(replacedUpload), 130))
	if earlier == current {
		t.Fatal("the fake serves one content for both files, so nothing here can tell them apart")
	}
	attachments := records(t, dir, bundle.Attachments)
	if got := recordWithID(t, attachments, "upload-9277-version-9293")["sha256"]; got != earlier {
		t.Errorf("the earlier file holds %v, want the bytes of its version and not %s of the current one", got, current)
	}
	if got := recordWithID(t, attachments, "upload-9277")["sha256"]; got != current {
		t.Errorf("the current file holds %v, want %s", got, current)
	}

	want := []string{
		"https://3.basecampapi.test/4242/buckets/7011/uploads/9277/versions/9293/download/notes.pdf",
		"https://3.basecampapi.test/4242/buckets/7011/uploads/9277/versions/9294/download/notes-draft.pdf",
	}
	if !reflect.DeepEqual(account.VersionDownloads, want) {
		t.Errorf("the versions asked for are %v, want %v", account.VersionDownloads, want)
	}
	if asked := slices.DeleteFunc(slices.Clone(account.UploadDownloads), func(a string) bool { return a != "7011/9277" }); len(asked) != 1 {
		t.Errorf("the upload was asked for by its identifier %d times, want once for the file it shows", len(asked))
	}
	for _, asked := range account.Downloads {
		if strings.Contains(asked, "9293") || strings.Contains(asked, "9294") {
			t.Errorf("an earlier file was asked for at the storage address %s", asked)
		}
	}
}

// Basecamp answers every page of the versions of an upload with the whole list,
// so the list is read as one answer: a walk by pages would take the second page
// for a list that moved under it and end the run.
func TestTheVersionsOfAnUploadAreReadAsOneAnswer(t *testing.T) {
	account := fakebasecamp.New()
	if _, result := extractWithVersions(t, account, docsProject); result.Docs.Versions.EarlierFiles != 2 {
		t.Fatalf("the run brought %d earlier files, want 2", result.Docs.Versions.EarlierFiles)
	}
	if got := asked(account, "/versions.json"); !slices.Contains(got, versionsOfIt) {
		t.Errorf("the versions were asked for as %v, want %s", got, versionsOfIt)
	}
	for _, request := range account.Requests {
		if strings.Contains(request, "/versions.json?") {
			t.Errorf("the versions were asked for by page: %s", request)
		}
	}
}

// The versions are asked for of an upload the bundle carries and of nothing else:
// one left out takes its earlier files with it, and nothing is asked about it.
func TestOnlyAnUploadTheBundleCarriesIsAskedForItsVersions(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extractWithVersions(t, account, docsProject)

	var want []string
	for _, upload := range records(t, dir, bundle.Uploads) {
		want = append(want, "buckets/7011/uploads/"+upload["sourceId"].(string)+"/versions.json")
	}
	got := asked(account, "/versions.json")
	slices.Sort(want)
	slices.Sort(got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the versions asked for are\n %v\nwant those of the uploads written\n %v", got, want)
	}
}

// A list that does not name the file the upload shows now first and once, or
// names a version without an identifier of its own, cannot say which file is
// the current one. The upload comes with that file alone, named, and nothing of
// the list is fetched: a version written for the wrong file would put the file
// the upload shows among the earlier ones.
func TestAListOfVersionsThatDoesNotSayWhichFileIsCurrentLeavesTheEarlierFilesOut(t *testing.T) {
	current := func(entry map[string]any, is bool) {
		entry["upload"].(map[string]any)["current"] = is
	}
	for what, reshape := range map[string]func([]map[string]any) []map[string]any{
		"none is current": func(listed []map[string]any) []map[string]any {
			current(listed[0], false)
			return listed
		},
		"an earlier one is current as well": func(listed []map[string]any) []map[string]any {
			current(listed[2], true)
			return listed
		},
		"the current one is not first": func(listed []map[string]any) []map[string]any {
			return append(listed[1:], listed[0])
		},
		"the current one has no file": func(listed []map[string]any) []map[string]any {
			delete(listed[0], "upload")
			return listed
		},
		"a version without an identifier": func(listed []map[string]any) []map[string]any {
			delete(listed[1], "id")
			return listed
		},
		"one version twice": func(listed []map[string]any) []map[string]any {
			listed[2]["id"] = listed[1]["id"]
			return listed
		},
		"nothing at all": func([]map[string]any) []map[string]any { return []map[string]any{} },
	} {
		account := fakebasecamp.New()
		account.ReshapeVersions = func(upload int64, listed []map[string]any) []map[string]any {
			if upload != replacedUpload {
				return listed
			}
			return reshape(listed)
		}
		dir, result := extractWithVersions(t, account, docsProject)

		if _, carries := recordWithID(t, records(t, dir, bundle.Uploads), "9277")["versions"]; carries {
			t.Errorf("%s: the upload carries versions", what)
		}
		want := []basecamp.UnreadHistory{{UploadSourceID: "9277", ProjectSourceID: "7011", Why: basecamp.HistoryNotUnderstood}}
		if !reflect.DeepEqual(result.Docs.Versions.HistoriesUnread, want) {
			t.Errorf("%s: the run names %v as unread, want %v", what, result.Docs.Versions.HistoriesUnread, want)
		}
		if len(account.VersionDownloads) != 0 || result.Docs.Versions.EarlierFiles != 0 {
			t.Errorf("%s: the run fetched %v and counts %d earlier files", what, account.VersionDownloads, result.Docs.Versions.EarlierFiles)
		}
		holdsTogether(t, dir, result.Missing)
	}
}

// Versions Basecamp will not hand out are named with why, as a list of who
// follows a record is, and the upload comes with the file it shows; any other
// failure of the list ends the run as every request does.
func TestVersionsBasecampWillNotHandOutAreNamedAndAnyOtherFailureEndsTheRun(t *testing.T) {
	for kind, why := range map[basecamp.Kind]basecamp.WhyAHistoryIsUnread{
		basecamp.NotFound:  basecamp.HistoryNotFound,
		basecamp.Forbidden: basecamp.HistoryForbidden,
	} {
		account := fakebasecamp.New()
		account.Fail[versionsOfIt] = &basecamp.Error{Kind: kind, Message: "Basecamp: no"}
		dir, result := extractWithVersions(t, account, docsProject)

		want := []basecamp.UnreadHistory{{UploadSourceID: "9277", ProjectSourceID: "7011", Why: why}}
		if !reflect.DeepEqual(result.Docs.Versions.HistoriesUnread, want) {
			t.Errorf("%s: the run names %v as unread, want %v", kind, result.Docs.Versions.HistoriesUnread, want)
		}
		if _, carries := recordWithID(t, records(t, dir, bundle.Uploads), "9277")["versions"]; carries {
			t.Errorf("%s: the upload carries versions", kind)
		}
	}

	account := fakebasecamp.New()
	account.Fail[versionsOfIt] = &basecamp.Error{Kind: basecamp.API, Message: "Basecamp: something else"}
	var failed *basecamp.Error
	if err := runWithVersions(t, account, docsProject); !errors.As(err, &failed) || failed.Kind != basecamp.API {
		t.Errorf("a list of versions that failed otherwise ended the run with %v, want its failure", err)
	}
}

// An earlier file that does not come is what any file that does not come is: one
// the source no longer has stands in the bundle as gone, one that failed is named
// among the missing files with its upload as owner, and one of another size than
// its version says is no file. Its entry stays among the versions either way —
// Minka leaves such an entry out with a warning, and a later run brings what may
// still come.
func TestAnEarlierFileThatDoesNotComeKeepsItsEntryAndIsNamed(t *testing.T) {
	entries := func(t *testing.T, dir string) []string {
		var files []string
		for _, entry := range recordWithID(t, records(t, dir, bundle.Uploads), "9277")["versions"].([]any) {
			files = append(files, entry.(map[string]any)["file"].(string))
		}
		return files
	}
	all := []string{"upload-9277-version-9293", "upload-9277-version-9294", "upload-9277"}

	gone := fakebasecamp.New()
	theUpload(t, gone, replacedUpload).EarlierFiles[1].Gone = true
	dir, result := extractWithVersions(t, gone, docsProject)
	if record := recordWithID(t, records(t, dir, bundle.Attachments), "upload-9277-version-9294"); record["gone"] != true {
		t.Errorf("an earlier file the source no longer has came as %v, want gone", record)
	}
	if got := entries(t, dir); !reflect.DeepEqual(got, all) {
		t.Errorf("with an earlier file gone the versions are %v, want %v", got, all)
	}
	holdsTogether(t, dir, result.Missing)

	for what, change := range map[string]func(*fakebasecamp.Account){
		"the download failed": func(a *fakebasecamp.Account) {
			a.FailFileOf[fakebasecamp.StoredKeyOfVersion(9294)] = &basecamp.Error{Kind: basecamp.Network, Message: "the network failed during the download"}
		},
		"fewer bytes came than the version says": func(a *fakebasecamp.Account) {
			a.ServeBytes[fakebasecamp.StoredKeyOfVersion(9294)] = 119
		},
		"more bytes came than the version says": func(a *fakebasecamp.Account) {
			a.ServeBytes[fakebasecamp.StoredKeyOfVersion(9294)] = 121
		},
	} {
		account := fakebasecamp.New()
		change(account)
		dir, result := extractWithVersions(t, account, docsProject)

		named := theMissingFile(result.Missing, "upload-9277-version-9294")
		if named == nil || named.OwnerKind != bundle.OwnerUpload || named.OwnerSourceID != "9277" || named.Reason == "" {
			t.Errorf("%s: the earlier file is named as %+v among %v", what, named, result.Missing)
		}
		for _, record := range records(t, dir, bundle.Attachments) {
			if record["sourceId"] == "upload-9277-version-9294" {
				t.Errorf("%s: the earlier file has a record: %v", what, record)
			}
		}
		if got := entries(t, dir); !reflect.DeepEqual(got, all) {
			t.Errorf("%s: the versions are %v, want %v", what, got, all)
		}
		holdsTogether(t, dir, result.Missing)
	}
}

// The address an earlier file is asked for at is the version's own on the API of
// the account, and the one Basecamp gives is taken only where it is exactly that.
// Anything else is refused by this run before it asks — named as a failure it
// cannot place, never as a file the source refused — and never asked for: the
// address the upload's own file is downloaded from would bring the current file.
func TestAnEarlierFileIsAskedForAtTheAddressOfItsVersionAndNoOther(t *testing.T) {
	for what, address := range map[string]string{
		"the address of the upload's own file": "https://3.basecampapi.test/4242/buckets/7011/uploads/9277/download/notes-draft.pdf",
		"the address of another version":       "https://3.basecampapi.test/4242/buckets/7011/uploads/9277/versions/9293/download/notes-draft.pdf",
		"the address of another upload":        "https://3.basecampapi.test/4242/buckets/7011/uploads/9260/versions/9294/download/notes-draft.pdf",
		"the address of another account":       "https://3.basecampapi.test/4243/buckets/7011/uploads/9277/versions/9294/download/notes-draft.pdf",
		"the address of the app":               "https://storage.app.basecamp.test/4242/buckets/7011/uploads/9277/versions/9294/download/notes-draft.pdf",
		"a name that is a path of its own":     "https://3.basecampapi.test/4242/buckets/7011/uploads/9277/versions/9294/download/..%2F..%2Fx",
		"a name followed by a query":           "https://3.basecampapi.test/4242/buckets/7011/uploads/9277/versions/9294/download/notes-draft.pdf?x=1",
		"no name at all":                       "https://3.basecampapi.test/4242/buckets/7011/uploads/9277/versions/9294/download/",
	} {
		account := fakebasecamp.New()
		account.ReshapeVersions = func(upload int64, listed []map[string]any) []map[string]any {
			if upload == replacedUpload {
				listed[2]["upload"].(map[string]any)["download_url"] = address
			}
			return listed
		}
		dir, result := extractWithVersions(t, account, docsProject)

		named := theMissingFile(result.Missing, "upload-9277-version-9294")
		if named == nil || named.Why != basecamp.NotUnderstood {
			t.Errorf("%s: the earlier file is named as %+v, want a failure this run cannot place", what, named)
		}
		for _, asked := range account.VersionDownloads {
			if strings.Contains(asked, "notes-draft") || strings.Contains(asked, "9294") {
				t.Errorf("%s: the earlier file was asked for at %s", what, asked)
			}
		}
		if slices.Contains(account.UploadDownloads, "7011/9277") && countOf(account.UploadDownloads, "7011/9277") != 1 {
			t.Errorf("%s: the upload was asked for by its identifier more than once", what)
		}
		holdsTogether(t, dir, result.Missing)
	}
}

// A version that gives its file no size is not asked for: the size is the one
// proof that what came is the file of that version. It is named as a failure
// nobody can place, and its entry stays.
func TestAnEarlierFileWithoutASizeIsNeverAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	account.ReshapeVersions = func(upload int64, listed []map[string]any) []map[string]any {
		if upload == replacedUpload {
			delete(listed[2]["upload"].(map[string]any), "byte_size")
		}
		return listed
	}
	dir, result := extractWithVersions(t, account, docsProject)

	if named := theMissingFile(result.Missing, "upload-9277-version-9294"); named == nil || named.Why != basecamp.NotUnderstood {
		t.Errorf("an earlier file without a size is named as %+v, want a failure nobody can place", named)
	}
	for _, asked := range account.VersionDownloads {
		if strings.Contains(asked, "/versions/9294/") {
			t.Errorf("an earlier file without a size was asked for at %s", asked)
		}
	}
	holdsTogether(t, dir, result.Missing)
}

// A dry run fetches no earlier file and holds every one to its address all the
// same, so that one no run could ask for is named before anybody spends an hour.
func TestADryRunNamesAnEarlierFileNoRunCouldAskFor(t *testing.T) {
	account := fakebasecamp.New()
	account.ReshapeVersions = func(upload int64, listed []map[string]any) []map[string]any {
		if upload == replacedUpload {
			listed[2]["upload"].(map[string]any)["download_url"] = "https://3.basecampapi.test/4242/buckets/7011/uploads/9277/download/notes-draft.pdf"
		}
		return listed
	}
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, UploadVersions: true,
		Now: func() time.Time { return runDay }}
	result, err := extractor.Extract(t.Context(), []string{docsProject}, bundle.Discard())
	if err != nil {
		t.Fatal(err)
	}

	if named := theMissingFile(result.Missing, "upload-9277-version-9294"); named == nil || named.Why != basecamp.NotUnderstood {
		t.Errorf("a dry run names the earlier file as %+v, want a failure nobody can place", named)
	}
	if theMissingFile(result.Missing, "upload-9277-version-9293") != nil {
		t.Error("a dry run names an earlier file whose address can be asked for")
	}
	if len(account.VersionDownloads) != 0 || len(account.UploadDownloads) != 0 {
		t.Errorf("a dry run fetched %v and %v", account.VersionDownloads, account.UploadDownloads)
	}
}

func countOf(asked []string, one string) int {
	count := 0
	for _, request := range asked {
		if request == one {
			count++
		}
	}
	return count
}

// A version names the moment it was given as a time a bundle takes, or the run
// ends, as it does for a record whose time is none.
func TestAVersionWhoseMomentIsNoMomentEndsTheRun(t *testing.T) {
	account := fakebasecamp.New()
	theUpload(t, account, replacedUpload).EarlierFiles[0].CreatedAt = "2025-02-30T10:00:00.000Z"
	var failed *basecamp.Error
	if err := runWithVersions(t, account, docsProject); !errors.As(err, &failed) || failed.Kind != basecamp.API || !strings.Contains(failed.Message, "version 9293 of upload 9277") {
		t.Errorf("a version of no moment ended the run with %v", err)
	}
}
