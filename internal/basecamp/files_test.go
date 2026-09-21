package basecamp_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

const fileProject = "7005"

// The files of the file project, by what they are there to show.
var (
	sunsetKey  = fakebasecamp.FileKey(5101)
	beachKey   = fakebasecamp.FileKey(5102)
	waitingKey = fakebasecamp.FileKey(4711)
	scanKey    = fakebasecamp.FileKey(6001)
	// lastKey is the file a run over the file project fetches last.
	lastKey = fakebasecamp.FileKey(6002)
)

// extractInto runs an extraction into a directory, which may hold the bundle
// of a run before.
func extractInto(t *testing.T, account *fakebasecamp.Account, dir string, projects ...string) basecamp.Result {
	t.Helper()
	writer, err := bundle.Create(dir, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	result, err := extractor.Extract(t.Context(), projects, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Commit(bundle.Manifest{SourceInstance: result.SourceInstance}); err != nil {
		t.Fatal(err)
	}
	return result
}

func digestOf(t *testing.T, content io.Reader) (string, int64) {
	t.Helper()
	sum := sha256.New()
	size, err := io.Copy(sum, content)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sum.Sum(nil)), size
}

// everyFile reads every file of a bundle, by its path in it.
func everyFile(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		relative, _ := filepath.Rel(dir, path)
		files[filepath.ToSlash(relative)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

var attachmentRef = regexp.MustCompile(`<import-attachment ref="([^"]*)"`)

// bodiesOf are the texts of a bundle, by the kind and the ID of what carries
// them.
func bodiesOf(t *testing.T, dir string) map[string]string {
	t.Helper()
	bodies := map[string]string{}
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, path := range paths {
		kind := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		if kind == string(bundle.Attachments) {
			continue
		}
		for _, record := range recordsIn(t, path) {
			for _, field := range []string{"description", "body"} {
				if text, ok := record[field].(string); ok {
					bodies[kind+" "+record["sourceId"].(string)] = text
				}
			}
		}
	}
	return bodies
}

// holdsTogether checks a bundle the way the format asks: every record of the
// index finds its file with the size and the digest it declares, a file that
// is gone has none of the three, no file lies under files/ that no record
// names, every reference of a text has a record unless the run named it as
// missing, and every record is named by a reference or by a person's avatar.
func holdsTogether(t *testing.T, dir string, missing []basecamp.MissingFile) {
	t.Helper()
	records := map[string]bool{}
	declared := map[string]bool{}
	for _, record := range recordsOrNone(t, dir, bundle.Attachments) {
		id := record["sourceId"].(string)
		if records[id] {
			t.Errorf("%s has two records", id)
		}
		records[id] = true
		if record["gone"] == true {
			if record["file"] != nil || record["bytes"] != nil || record["sha256"] != nil {
				t.Errorf("%s is gone and still says file %v, bytes %v, sha256 %v", id, record["file"], record["bytes"], record["sha256"])
			}
			continue
		}
		if _, said := record["gone"]; said {
			t.Errorf("%s says gone: %v", id, record["gone"])
		}
		file, _ := record["file"].(string)
		if want := id + "/" + record["filename"].(string); file != want || !bundle.PlainName(record["filename"].(string)) {
			t.Errorf("%s lies at %q", id, file)
			continue
		}
		declared["files/"+file] = true
		stored, err := os.Open(filepath.Join(dir, "files", filepath.FromSlash(file)))
		if err != nil {
			t.Errorf("%s has no file: %v", id, err)
			continue
		}
		sum, size := digestOf(t, stored)
		stored.Close()
		if record["bytes"] != float64(size) || record["sha256"] != sum || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(sum) {
			t.Errorf("%s declares %v bytes and %v, its file has %d and %s", id, record["bytes"], record["sha256"], size, sum)
		}
	}
	for path := range everyFile(t, dir) {
		if strings.HasPrefix(path, "files/") && !declared[path] {
			t.Errorf("no record names the file of %s", strings.Split(path, "/")[1])
		}
	}

	named := map[string]bool{}
	for _, missed := range missing {
		if records[missed.AttachmentSourceID] {
			t.Errorf("%s is missing and has a record", missed.AttachmentSourceID)
		}
		named[missed.AttachmentSourceID] = true
	}
	for owner, body := range bodiesOf(t, dir) {
		for _, match := range attachmentRef.FindAllStringSubmatch(body, -1) {
			if !records[match[1]] && !named[match[1]] {
				t.Errorf("%s refers to %s, which has no record and is not missing", owner, match[1])
			}
			named[match[1]] = true
		}
	}
	for _, person := range recordsOrNone(t, dir, bundle.People) {
		if avatar, ok := person["avatar"].(string); ok {
			named[avatar] = true
		}
	}
	for id := range records {
		if !named[id] {
			t.Errorf("nothing names the record %s", id)
		}
	}
}

func TestEveryReferenceToAFileHasARecordAndItsBytes(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, fileProject)
	holdsTogether(t, dir, result.Missing)
	if len(result.Missing) != 0 {
		t.Fatalf("missing: %+v", result.Missing)
	}

	sum, _ := digestOf(t, fakebasecamp.FileContent(sunsetKey, 3000))
	want := map[string]any{
		"sourceId": "attachment-5101", "createdAt": "2023-05-02T08:00:00.000Z", "updatedAt": "2023-05-02T09:00:00.000Z", "createdBy": nil,
		"filename": "holiday photo.png", "contentType": "image/png", "bytes": float64(3000), "sha256": sum,
		"file": "attachment-5101/holiday photo.png", "ownerKind": "card", "ownerSourceId": "9901",
	}
	attachments := records(t, dir, bundle.Attachments)
	if got := recordWithID(t, attachments, "attachment-5101"); !reflect.DeepEqual(got, want) {
		t.Errorf("a file of a card:\n got %v\nwant %v", got, want)
	}

	// A file takes the times of what carries it, and names its kind and ID.
	for id, owner := range map[string][4]string{
		"attachment-5103": {"comment", "9911", "2023-05-02T10:00:00.000Z", "2023-05-02T11:00:00.000Z"},
		"attachment-5110": {"post", "9921", "2023-05-04T08:00:00.000Z", "2023-05-04T09:00:00.000Z"},
		"blob-" + scanKey: {"message", "9931", "2023-05-05T08:00:00.000Z", "2023-05-05T08:00:00.000Z"},
	} {
		got := recordWithID(t, attachments, id)
		if got["ownerKind"] != owner[0] || got["ownerSourceId"] != owner[1] || got["createdAt"] != owner[2] || got["updatedAt"] != owner[3] || got["createdBy"] != nil {
			t.Errorf("%s: %v", id, got)
		}
	}

	// A file of a line is named by the key of the stored file, and its
	// reference carries nothing besides.
	if got, want := bodiesOf(t, dir)["messages 9931"], `<import-attachment ref="blob-`+scanKey+`"></import-attachment><import-attachment ref="blob-`+fakebasecamp.FileKey(6002)+`"></import-attachment>`; got != want {
		t.Errorf("the line:\n got %q\nwant %q", got, want)
	}
	stored, err := os.ReadFile(filepath.Join(dir, "files", "blob-"+scanKey, "scan 1.pdf"))
	content, _ := io.ReadAll(fakebasecamp.FileContent(scanKey, 5000))
	if err != nil || string(stored) != string(content) {
		t.Errorf("the file of the line: %d bytes, %v", len(stored), err)
	}

	// What Basecamp says no type and no size of comes all the same.
	untyped, unsized := recordWithID(t, attachments, "attachment-5106"), recordWithID(t, attachments, "attachment-5109")
	if untyped["contentType"] != nil || unsized["bytes"] != float64(64) {
		t.Errorf("without a type: %v; without a size: %v", untyped["contentType"], unsized["bytes"])
	}

	if want := (basecamp.FileCounts{Expected: 13, BytesExpected: 15950, Bytes: 15114, Fetched: 12, Gone: 1, NamesChanged: 5, SecondOwners: 1}); result.Files != want {
		t.Errorf("counted %+v, want %+v", result.Files, want)
	}
}

func TestAFileIsFetchedFromTheStorageAddressItsKeyAndNameMake(t *testing.T) {
	account := fakebasecamp.New()
	extract(t, account, fileProject)

	for _, want := range []string{
		"https://storage.3.basecamp.com/4242/blobs/" + sunsetKey + "/download/holiday%20photo.png",
		"https://storage.3.basecamp.com/4242/blobs/" + fakebasecamp.FileKey(5103) + "/download/reports-2023-q1.pdf",
		"https://storage.3.basecamp.com/4242/blobs/" + fakebasecamp.FileKey(5104) + "/download/line%0Abreak.txt",
		// The name as the account writes it into its own address.
		"https://storage.3.basecamp.com/4242/blobs/" + fakebasecamp.FileKey(5106) + "/download/dots",
		"https://storage.3.basecamp.com/4242/blobs/" + scanKey + "/download/scan%201.pdf",
	} {
		if !slices.Contains(account.Downloads, want) {
			t.Errorf("%s was not asked for; asked for %v", want, account.Downloads)
		}
	}
	for _, address := range account.Downloads {
		if strings.Contains(address, "basecampapi") || strings.Contains(address, "preview") {
			t.Errorf("a file was asked for at %s", address)
		}
	}
}

func TestTheNameOfAFileInABundleIsAPlainOne(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), fileProject)
	attachments := records(t, dir, bundle.Attachments)

	for id, want := range map[string]string{
		"attachment-5103":                       "reports_2023_q1.pdf",
		"attachment-5104":                       "line_break_.txt",
		"attachment-5106":                       "file",
		"blob-" + fakebasecamp.FileKey(6002):    "a_b.png",
		"attachment-5101":                       "holiday photo.png",
		"attachment-5105":                       "",
		"blob-" + fakebasecamp.FileKey(6001)[:]: "scan 1.pdf",
	} {
		record := recordWithID(t, attachments, id)
		name := record["filename"].(string)
		if want != "" && name != want {
			t.Errorf("%s is called %q, want %q", id, name, want)
		}
		if record["file"] != id+"/"+name {
			t.Errorf("%s lies at %v", id, record["file"])
		}
		if _, err := os.Stat(filepath.Join(dir, "files", id, name)); err != nil {
			t.Errorf("%s: %v", id, err)
		}
	}
	long := recordWithID(t, attachments, "attachment-5105")["filename"].(string)
	if !strings.HasSuffix(long, ".jpeg") || !strings.HasPrefix(long, "ä long name") || len(long) > bundle.MaxNameLength || len(long) < bundle.MaxNameLength-4 {
		t.Errorf("the long name became %d bytes", len(long))
	}
	if result.Files.NamesChanged != 5 {
		t.Errorf("%d names changed, want 5", result.Files.NamesChanged)
	}
}

func TestAFileNamedTwiceHasOneRecordAndIsFetchedOnce(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, fileProject)

	// Both references stay, through whichever storage host the text named.
	body := bodiesOf(t, dir)["cards 9901"]
	if strings.Count(body, `ref="attachment-5101"`) != 2 {
		t.Errorf("the note: %q", body)
	}
	asked := 0
	for _, address := range account.Downloads {
		if strings.Contains(address, sunsetKey) {
			asked++
		}
	}
	owners := 0
	for _, record := range records(t, dir, bundle.Attachments) {
		if record["sourceId"] == "attachment-5101" {
			owners++
			// The post names the file as well; the card met it first.
			if record["ownerKind"] != "card" || record["ownerSourceId"] != "9901" {
				t.Errorf("the file belongs to %v %v", record["ownerKind"], record["ownerSourceId"])
			}
		}
	}
	if asked != 1 || owners != 1 || result.Files.SecondOwners != 1 {
		t.Errorf("asked for %d times, %d records, %d second owners", asked, owners, result.Files.SecondOwners)
	}
	if !strings.Contains(bodiesOf(t, dir)["posts 9921"], `ref="attachment-5101"`) {
		t.Error("the post lost its reference")
	}
	if result.Bodies.Files != 13+2 {
		t.Errorf("%d references, want 15", result.Bodies.Files)
	}
}

func TestAReferenceSaysWhatSomebodySaidAboutAFile(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), fileProject)
	bodies := bodiesOf(t, dir)

	wantNote := `<div><import-attachment ref="attachment-5101" caption="Sunset &amp; &#34;sea&#34; &lt;3" presentation="gallery"></import-attachment>` +
		// The caption stands in the rendered note alone; another layout than a gallery is none.
		`<import-attachment ref="attachment-5102" caption="The beach"></import-attachment>` +
		`<import-attachment ref="attachment-5101"></import-attachment></div>`
	if got := bodies["cards 9901"]; got != wantNote {
		t.Errorf("the note:\n got %q\nwant %q", got, wantNote)
	}
	if got := bodies["posts 9921"]; !strings.Contains(got, `<import-attachment ref="attachment-5110" presentation="gallery"></import-attachment>`) {
		t.Errorf("the post: %q", got)
	}
	odd := bodies["cards 9902"]
	if !strings.Contains(odd, `<import-attachment ref="attachment-5107"></import-attachment>`) || strings.Contains(odd, "goes on") {
		t.Errorf("a caption that is too long stayed: %q", odd)
	}
	if result.Bodies.CaptionsDropped != 1 {
		t.Errorf("%d captions dropped, want 1", result.Bodies.CaptionsDropped)
	}
}

func TestAFileBasecampNoLongerHasGetsARecordThatSaysSo(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, fileProject)

	want := map[string]any{
		"sourceId": "attachment-5108", "createdAt": "2023-05-03T08:00:00.000Z", "updatedAt": "2023-05-03T08:00:00.000Z", "createdBy": nil,
		"filename": "shredded.pdf", "contentType": "application/pdf", "bytes": nil, "sha256": nil, "file": nil, "gone": true,
		"ownerKind": "card", "ownerSourceId": "9902",
	}
	if got := recordWithID(t, records(t, dir, bundle.Attachments), "attachment-5108"); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if result.Files.Gone != 1 || len(result.Missing) != 0 {
		t.Errorf("%d gone, missing %+v", result.Files.Gone, result.Missing)
	}
	if _, err := os.Stat(filepath.Join(dir, "files", "attachment-5108")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a file that is gone left something behind: %v", err)
	}
	if !strings.Contains(bodiesOf(t, dir)["cards 9902"], `ref="attachment-5108"`) {
		t.Error("the reference to the file that is gone is gone as well")
	}
}

func TestAFileThatCannotBeFetchedIsLeftOutAndTheRunGoesOn(t *testing.T) {
	account := fakebasecamp.New()
	account.FailFileOf[waitingKey] = &basecamp.Error{Kind: basecamp.Network, Message: "the network failed during the download"}
	account.FailFileOf[beachKey] = &basecamp.Error{Kind: basecamp.Forbidden, Message: "Basecamp forbids the download"}
	account.ServeBytes[scanKey] = 4999
	account.ServeBytes[fakebasecamp.FileKey(6002)] = 251
	// Without a size said, whatever comes is the file.
	account.ServeBytes[fakebasecamp.FileKey(5109)] = 7
	dir, result := extract(t, account, fileProject)

	want := []basecamp.MissingFile{
		{AttachmentSourceID: "attachment-5102", OwnerKind: "card", OwnerSourceID: "9901", Reason: "Basecamp forbids the download"},
		{AttachmentSourceID: "attachment-4711", OwnerKind: "card", OwnerSourceID: "9902", Reason: "the network failed during the download", Retryable: true},
		{AttachmentSourceID: "blob-" + scanKey, OwnerKind: "message", OwnerSourceID: "9931", Reason: "the download brought 4999 bytes of a file of 5000", Retryable: true},
		{AttachmentSourceID: "blob-" + fakebasecamp.FileKey(6002), OwnerKind: "message", OwnerSourceID: "9931", Reason: "the download brought 251 bytes of a file of 250"},
	}
	if !reflect.DeepEqual(result.Missing, want) {
		t.Errorf("missing:\n got %+v\nwant %+v", result.Missing, want)
	}
	for _, missed := range want {
		// Neither a record nor the half of a file stays, and the reference does.
		if _, err := os.Stat(filepath.Join(dir, "files", missed.AttachmentSourceID)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s left something under files/: %v", missed.AttachmentSourceID, err)
		}
	}
	holdsTogether(t, dir, result.Missing)
	if got := recordWithID(t, records(t, dir, bundle.Attachments), "attachment-5109"); got["bytes"] != float64(7) {
		t.Errorf("a file without a size: %v", got)
	}
	// The run went on: what follows a failure came.
	if result.Files.Fetched != 8 || result.Files.Expected != 13 || len(records(t, dir, bundle.Messages)) != 2 {
		t.Errorf("counted %+v", result.Files)
	}
}

func TestAStoppedDownloadEndsTheRunAndLeavesNothing(t *testing.T) {
	for name, prepare := range map[string]func(*fakebasecamp.Account, context.CancelFunc){
		"the download says so": func(account *fakebasecamp.Account, _ context.CancelFunc) {
			account.FailFileOf[beachKey] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}
		},
		"the run is stopped while a download fails": func(account *fakebasecamp.Account, stop context.CancelFunc) {
			account.FailFileOf[beachKey] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
			account.BeforeDownload = func(key string) {
				if key == beachKey {
					stop()
				}
			}
		},
		// Behind the last file nothing asks the source again, so only the
		// download itself can say that the run was stopped.
		"the run is stopped during its last download": func(account *fakebasecamp.Account, stop context.CancelFunc) {
			account.FailFileOf[lastKey] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
			account.BeforeDownload = func(key string) {
				if key == lastKey {
					stop()
				}
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			account := fakebasecamp.New()
			ctx, stop := context.WithCancel(t.Context())
			defer stop()
			prepare(account, stop)

			writer, err := bundle.Create(filepath.Join(parent, "bundle"), basecamp.SourceName)
			if err != nil {
				t.Fatal(err)
			}
			extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
			_, err = extractor.Extract(ctx, []string{fileProject}, writer)
			writer.Abort()

			var failed *basecamp.Error
			if !errors.As(err, &failed) || failed.Kind != basecamp.Interrupted {
				t.Fatalf("got %v, want interrupted", err)
			}
			if entries, _ := os.ReadDir(parent); len(entries) != 0 {
				t.Errorf("the run left %v", entries)
			}
			if last := account.Downloads[len(account.Downloads)-1]; !strings.Contains(last, beachKey) && !strings.Contains(last, lastKey) {
				t.Errorf("the run went on to %s", last)
			}
		})
	}
}

// A bundle that cannot be written ends a run, whatever it was writing.
func TestAFileTheBundleCannotStoreEndsTheRun(t *testing.T) {
	account := fakebasecamp.New()
	account.FailFileOf[beachKey] = errors.New("no space left on device")
	writer, err := bundle.Create(filepath.Join(t.TempDir(), "bundle"), basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	_, err = extractor.Extract(t.Context(), []string{fileProject}, writer)
	var failed *basecamp.Error
	if err == nil || errors.As(err, &failed) {
		t.Errorf("got %v, want the failure of the bundle", err)
	}
}

func TestAFileWithoutAnAddressItCanBeFetchedFromIsMissingAndNeverAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	card := &account.Projects[4].Dock[0].Lists[0].Cards[0]
	card.Files[1].Key = "k-one"
	// A name that, without its escapes, is a path into another account.
	card.Comments[0].Files[0].URLName = "../../../../999/people.json"
	chat := &account.Projects[4].Dock[2]
	chat.Lines[0].Uploads[1].URLName = "a/b.png"
	dir, result := extract(t, account, fileProject)

	want := []basecamp.MissingFile{
		{AttachmentSourceID: "attachment-5102", OwnerKind: "card", OwnerSourceID: "9901", Reason: "the key of the stored file has no form that can be asked for"},
		{AttachmentSourceID: "attachment-5103", OwnerKind: "comment", OwnerSourceID: "9911", Reason: "the address Basecamp gives the file cannot be asked for safely"},
		{AttachmentSourceID: "blob-" + fakebasecamp.FileKey(6002), OwnerKind: "message", OwnerSourceID: "9931", Reason: "the address Basecamp gives the file cannot be asked for safely"},
	}
	if !reflect.DeepEqual(result.Missing, want) {
		t.Fatalf("missing:\n got %+v\nwant %+v", result.Missing, want)
	}
	for _, address := range account.Downloads {
		for _, part := range []string{"k-one", fakebasecamp.FileKey(5103), fakebasecamp.FileKey(6002), "999", "%2F"} {
			if strings.Contains(address, part) {
				t.Errorf("asked for %s", address)
			}
		}
	}
	if result.Files.Gone != 1 {
		t.Errorf("%d files are called gone, want the one Basecamp no longer has", result.Files.Gone)
	}
	holdsTogether(t, dir, result.Missing)
}

// Somebody may have uploaded a small file of JSON that reads like a failure
// of the Basecamp CLI. With the size Basecamp gives it, the size decides and
// nothing else; only without one does the look of it count.
func TestWhatLooksLikeAFailureIsTheFileWhenItHasTheSizeBasecampGives(t *testing.T) {
	unsizedKey := fakebasecamp.FileKey(5109)
	account := fakebasecamp.New()
	account.FailFileOf[beachKey] = basecamp.ErrLooksLikeAFailure
	account.FailFileOf[unsizedKey] = basecamp.ErrLooksLikeAFailure
	// Of another size than Basecamp gives, it is what it looks like.
	account.FailFileOf[waitingKey] = basecamp.ErrLooksLikeAFailure
	account.ServeBytes[waitingKey] = 170
	dir, result := extract(t, account, fileProject)

	sum, _ := digestOf(t, fakebasecamp.FileContent(beachKey, 2000))
	if got := recordWithID(t, records(t, dir, bundle.Attachments), "attachment-5102"); got["bytes"] != float64(2000) || got["sha256"] != sum {
		t.Errorf("the file of the size Basecamp gives: %v", got)
	}
	want := []basecamp.MissingFile{
		{AttachmentSourceID: "attachment-5109", OwnerKind: "card", OwnerSourceID: "9902", Reason: basecamp.ErrLooksLikeAFailure.Message},
		{AttachmentSourceID: "attachment-4711", OwnerKind: "card", OwnerSourceID: "9902", Reason: "the download brought 170 bytes of a file of 600", Retryable: true},
	}
	if !reflect.DeepEqual(result.Missing, want) {
		t.Errorf("missing:\n got %+v\nwant %+v", result.Missing, want)
	}
	holdsTogether(t, dir, result.Missing)
}

// Without a sign-in no further file would come either, and a run that went on
// would name every one of them as missing.
func TestADownloadWithoutASignInEndsTheRun(t *testing.T) {
	parent := t.TempDir()
	account := fakebasecamp.New()
	account.FailFileOf[beachKey] = &basecamp.Error{Kind: basecamp.AuthRequired, Message: "the Basecamp CLI is not signed in"}
	writer, err := bundle.Create(filepath.Join(parent, "bundle"), basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	_, err = extractor.Extract(t.Context(), []string{fileProject}, writer)
	writer.Abort()

	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.AuthRequired {
		t.Fatalf("got %v, want a run that ends for want of a sign-in", err)
	}
	if last := account.Downloads[len(account.Downloads)-1]; !strings.Contains(last, beachKey) {
		t.Errorf("the run went on to %s", last)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Errorf("the run left %v", entries)
	}

	// What Basecamp forbids is one file's trouble.
	account = fakebasecamp.New()
	account.FailFileOf[beachKey] = &basecamp.Error{Kind: basecamp.Forbidden, Message: "Basecamp forbids the download"}
	if _, result := extract(t, account, fileProject); len(result.Missing) != 1 {
		t.Errorf("missing: %+v", result.Missing)
	}
}

// The applier stops at an ID that stands twice in the index.
func TestTheIndexNamesEveryIDOnce(t *testing.T) {
	account := fakebasecamp.New()
	// A stored file that is dropped into a chat once more, twice in one line.
	chat := &account.Projects[4].Dock[2]
	again := chat.Lines[0].Uploads[0]
	chat.Lines = append(chat.Lines, fakebasecamp.Line{ID: 9933, Type: fakebasecamp.UploadLine, Creator: chat.Lines[0].Creator,
		CreatedAt: "2023-05-05T10:00:00.000Z", UpdatedAt: "2023-05-05T10:00:00.000Z", Uploads: []fakebasecamp.Upload{again, again}})
	dir, result := extract(t, account, fileProject, boardProject, talkProject)

	seen := map[string]bool{}
	for _, record := range records(t, dir, bundle.Attachments) {
		id := record["sourceId"].(string)
		if seen[id] {
			t.Errorf("%s stands in the index twice", id)
		}
		seen[id] = true
	}
	if !seen["avatar-101"] || !seen["blob-"+scanKey] || !seen["attachment-5101"] || result.Files.SecondOwners != 3 {
		t.Errorf("the index: %v; %d second owners", seen, result.Files.SecondOwners)
	}
}

func TestTheIndexBeginsWithTheAvatarsAndGoesOnInTheOrderOfTheWalk(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), fileProject, boardProject, talkProject)
	var got []string
	for _, record := range records(t, dir, bundle.Attachments) {
		got = append(got, record["sourceId"].(string))
	}
	want := []string{
		"avatar-99", "avatar-101", "avatar-102", "avatar-103", "avatar-105", "avatar-801", "avatar-802",
		// The boards of the projects in the order of their IDs, a card before
		// its comments, then the posts, then the chats.
		"attachment-5001", "attachment-5003",
		"attachment-5101", "attachment-5102", "attachment-5103", "attachment-5104",
		"attachment-5105", "attachment-5106", "attachment-5107", "attachment-5108", "attachment-5109", "attachment-4711",
		"attachment-5011", "attachment-5110",
		"blob-0a1b2c3d-0001-4000-8000-00000000000a", "blob-0a1b2c3d-0002-4000-8000-00000000000b", "blob-0a1b2c3d-0003-4000-8000-00000000000c",
		"blob-" + scanKey, "blob-" + fakebasecamp.FileKey(6002),
	}
	if !slices.Equal(got, want) {
		t.Errorf("the index:\n got %v\nwant %v", got, want)
	}
}

func TestABigFileComesAsItIs(t *testing.T) {
	const size = 6<<20 + 11
	account := fakebasecamp.New()
	chat := &account.Projects[4].Dock[2]
	chat.Lines[0].Uploads[0].Bytes = size
	dir, result := extract(t, account, fileProject)

	sum, _ := digestOf(t, fakebasecamp.FileContent(scanKey, size))
	record := recordWithID(t, records(t, dir, bundle.Attachments), "blob-"+scanKey)
	if record["bytes"] != float64(size) || record["sha256"] != sum {
		t.Errorf("the record says %v bytes and %v", record["bytes"], record["sha256"])
	}
	holdsTogether(t, dir, result.Missing)
}

func TestADiscardedRunDownloadsNoFileAndSaysWhatARunWouldFetch(t *testing.T) {
	account := fakebasecamp.New()
	writer := bundle.Discard()
	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
	result, err := extractor.Extract(t.Context(), []string{fileProject}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if len(account.Downloads) != 0 {
		t.Errorf("a dry run downloaded %v", account.Downloads)
	}
	if want := (basecamp.FileCounts{Expected: 13, BytesExpected: 15950, NamesChanged: 5, SecondOwners: 1}); result.Files != want {
		t.Errorf("counted %+v, want %+v", result.Files, want)
	}
	if got := writer.Counts()[bundle.Attachments]; got != 13+2 {
		t.Errorf("%d attachments counted, want the 13 files and 2 avatars", got)
	}
}

func TestASecondRunTakesItsFilesFromTheBundleItReplaces(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	first := extractInto(t, fakebasecamp.New(), dir, fileProject, boardProject, talkProject)
	before := everyFile(t, dir)

	account := fakebasecamp.New()
	second := extractInto(t, account, dir, fileProject, boardProject, talkProject)
	if !reflect.DeepEqual(everyFile(t, dir), before) {
		t.Error("the second run wrote another bundle than the first")
	}
	// A file that was gone is asked for again: it may be back.
	if len(account.Downloads) != 1 || !strings.Contains(account.Downloads[0], fakebasecamp.FileKey(5108)) {
		t.Errorf("the second run downloaded %v", account.Downloads)
	}
	if first.Files.Reused != 0 || first.Files.Fetched != 18 || second.Files.Reused != 18 || second.Files.Fetched != 0 ||
		second.Files.Bytes != first.Files.Bytes || second.Files.Gone != 1 {
		t.Errorf("the first run counted %+v, the second %+v", first.Files, second.Files)
	}
	// An avatar is fetched anew: somebody may have another picture by now.
	if len(account.AvatarRequests) != 7 {
		t.Errorf("the second run fetched %d avatars", len(account.AvatarRequests))
	}
	holdsTogether(t, dir, second.Missing)
	if entries, _ := os.ReadDir(filepath.Dir(dir)); len(entries) != 1 {
		t.Errorf("next to the bundle: %v", entries)
	}
}

func TestOnlyAFileThatIsWhatItWasIsTakenOver(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	account := fakebasecamp.New()
	account.FailFileOf[waitingKey] = &basecamp.Error{Kind: basecamp.Network, Message: "connection reset"}
	extractInto(t, account, dir, fileProject)
	untouched := everyFile(t, dir)

	// Somebody changed a file of the bundle, another has another size at
	// Basecamp by now, and one failed the first time.
	tampered := filepath.Join(dir, "files", "attachment-5101", "holiday photo.png")
	content, err := os.ReadFile(tampered)
	if err != nil {
		t.Fatal(err)
	}
	content[0] ^= 0xff
	if err := os.WriteFile(tampered, content, 0o600); err != nil {
		t.Fatal(err)
	}
	again := fakebasecamp.New()
	again.Projects[4].Dock[0].Lists[0].Cards[0].Files[1].Bytes = 2001
	result := extractInto(t, again, dir, fileProject)

	var asked []string
	for _, address := range again.Downloads {
		asked = append(asked, strings.Split(address, "/")[5])
	}
	slices.Sort(asked)
	want := []string{sunsetKey, beachKey, fakebasecamp.FileKey(5108), waitingKey}
	slices.Sort(want)
	if !slices.Equal(asked, want) {
		t.Errorf("downloaded again %v, want %v", asked, want)
	}
	if result.Files.Reused != 9 || result.Files.Fetched != 3 {
		t.Errorf("counted %+v", result.Files)
	}
	holdsTogether(t, dir, result.Missing)
	after := everyFile(t, dir)
	if after["files/attachment-5101/holiday photo.png"] != untouched["files/attachment-5101/holiday photo.png"] {
		t.Error("the file somebody changed was taken over")
	}
	if len(after["files/attachment-5102/beach.png"]) != 2001 || len(after["files/attachment-4711/waiting.png"]) != 600 {
		t.Error("a file of another size, or one that was missing, did not come anew")
	}
}

func TestAnAvatarIsNeverTakenOver(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bundle")
	extractInto(t, fakebasecamp.New(), dir, "7002")

	// The index of the old bundle is as good for the avatar as for any file.
	account := fakebasecamp.New()
	extractInto(t, account, dir, "7002")
	if len(account.AvatarRequests) != 1 {
		t.Errorf("the second run fetched %d avatars, want the one anew", len(account.AvatarRequests))
	}
	var index []map[string]any
	for _, record := range records(t, dir, bundle.Attachments) {
		index = append(index, record)
	}
	if len(index) != 1 || index[0]["sourceId"] != "avatar-101" {
		t.Fatalf("the index: %v", index)
	}
	raw, _ := json.Marshal(index[0])
	if strings.Contains(string(raw), `"gone"`) {
		t.Errorf("an avatar says whether it is gone: %s", raw)
	}
}

func TestTwoRunsOverTheSameFilesWriteTheSameBytes(t *testing.T) {
	first, _ := extract(t, fakebasecamp.New(), fileProject)
	second, _ := extract(t, fakebasecamp.New(), fileProject)
	if !reflect.DeepEqual(everyFile(t, first), everyFile(t, second)) {
		t.Error("two runs wrote different bundles")
	}

	small := fakebasecamp.New()
	small.PageSize = 1
	third, _ := extract(t, small, fileProject)
	if !reflect.DeepEqual(everyFile(t, first), everyFile(t, third)) {
		t.Error("a run over smaller pages wrote another bundle")
	}
}

func TestTheFilesOfABundleAreForTheirOwnerOnly(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), fileProject)
	err := filepath.WalkDir(filepath.Join(dir, "files"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		want := os.FileMode(0o600)
		if entry.IsDir() {
			want = 0o700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s has the permissions %04o", strings.TrimPrefix(path, dir), info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
