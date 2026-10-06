package basecamp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
)

// Files hands out the files Basecamp stores for an account, by the two routes a
// file of a bundle can come by.
type Files interface {
	// Download writes the file at a storage address into a writer, as it comes.
	// A file Basecamp no longer has is a NotFound. What was written before a
	// failure is no file, with one exception: ErrLooksLikeAFailure comes with
	// everything that was to come, for whoever knows better what the file is.
	Download(ctx context.Context, address string, into io.Writer) error
	// DownloadUpload writes the one file of an upload of a Docs & Files tool,
	// asked for by the identifier of the upload and the project it stands in.
	// Such a file has no storage address anywhere: the record names no stored
	// file, and the address it gives leads to a store of its own that no run
	// follows. It answers the same way Download does.
	DownloadUpload(ctx context.Context, project, upload int64, into io.Writer) error
	// DownloadVersion writes an earlier file of an upload, asked for at the
	// address of that version on the API, which no other route may stand in
	// for: both of the others bring the file the upload shows now. It answers
	// the way Download does.
	DownloadVersion(ctx context.Context, address string, into io.Writer) error
}

// ErrLooksLikeAFailure is a download that ended well and brought nothing but
// what the Basecamp CLI prints when it fails. That may be the file, a small
// one of JSON that somebody uploaded, so all of it was written; only the size
// Basecamp gives the file can tell.
var ErrLooksLikeAFailure = &Error{Kind: API, Message: "the Basecamp CLI printed a failure in place of the file and ended as if it had brought it"}

// FileCounts says what became of the files the texts and the chats of a run
// refer to. Every number about fetching counts one file of the source once,
// however many references name it and however many records own it;
// OwnersBeyondTheFirst alone counts owners.
type FileCounts struct {
	// Expected are the files a run sets out to bring, and BytesExpected is
	// what Basecamp says they weigh together.
	Expected      int
	BytesExpected int64
	// Bytes is what the files of the bundle weigh, fetched or taken over.
	Bytes int64
	// Fetched were downloaded, Reused were taken over from the bundle the run
	// replaces, Gone are those Basecamp no longer has.
	Fetched int
	Reused  int
	Gone    int
	// NamesChanged are the files whose name could not stay as it was.
	NamesChanged int
	// OwnersBeyondTheFirst counts the owners a file of the source turned up
	// under after the one it was fetched for: one file, a record per owner, and
	// the same bytes under each of them. It counts owners met and not records
	// written, so a file no owner of which got one is counted here all the same.
	OwnersBeyondTheFirst int
}

// fileOfTheSource is one file of the source and what the bundle has made of it.
// Every owner of a file gets a record of its own, so one file can stand behind
// several of them, and the first of those records is the one whose bytes the
// others share.
type fileOfTheSource struct {
	// records counts the records handed out for the file, the first included.
	records int
	// holder is the record the bytes were brought for, and stored says where
	// they lie, once a record of the file has them.
	holder string
	stored bundle.StoredFile
	held   bool
	// gone tells that Basecamp no longer has the file, and failed why this run
	// could not fetch it, with why whose word that was: either way no owner of it
	// gets bytes, and asking a second time would pay for the same answer twice.
	gone   bool
	failed *Error
	why    WhyAFileIsMissing
	// decided says that a record of the file has been through all of that, so
	// that a further owner knows whether there is anything to share.
	decided bool
}

// sourceFile is a file of the source as a run brings it: one a text or a line
// refers to, named by the key of its stored file, or the one file of an upload of
// a Docs & Files tool, which is named by nothing and fetched by the identifiers
// of the upload and its project.
type sourceFile struct {
	// ref is the ID of the attachment record, which the reference carries.
	ref         string
	key         string
	filename    string
	contentType string
	// byteSize is nil where Basecamp does not say how large the file is.
	byteSize    *int64
	downloadURL string
	// upload is the upload of a Docs & Files tool whose one file this is and
	// bucket the project it stands in, both zero for every other file. Basecamp
	// gives such a file no key and answers its own address with a redirect to a
	// store no run follows, so the bytes are asked for by these two.
	upload int64
	bucket int64
	// version is the version of that upload an earlier file was given in, zero
	// for every other file. Such a file is asked for at the address of the
	// version on the API, built from downloadURL, and by no other route.
	version int64
	// sharesWith names the file of the source, as the source names it, where
	// this record is a further owner of one file and takes the bytes another
	// record of the bundle already holds. Empty for the first owner, whose ref
	// is the source's own name for the file.
	sharesWith string
}

// byTheUpload tells a file that is fetched by the identifier of its upload from
// one that is fetched at an address, and anEarlierFile one that was an upload's
// before it was given another.
func (f sourceFile) byTheUpload() bool   { return f.upload != 0 && f.version == 0 }
func (f sourceFile) anEarlierFile() bool { return f.version != 0 }

// ofTheSource is the file as the source names it, which is the same name for
// every owner of it, where ref is the record's own.
func (f sourceFile) ofTheSource() string {
	if f.sharesWith != "" {
		return f.sharesWith
	}
	return f.ref
}

// fileOwner is the record a file of the bundle belongs to: the kind of record
// and the source's identifier of it, which is what its attachment record says
// and what names the record of a file under a further owner.
type fileOwner struct {
	kind     string
	sourceID string
}

// owns names the owner of the files of one record.
func owns(kind string, id int64) fileOwner {
	return fileOwner{kind: kind, sourceID: sourceID(id)}
}

// claimFile hands out the identifier the record of a file carries in the bundle
// and says where its bytes come from. A body that names one file twice names one
// record of it: within is what this body has claimed already, by the source's
// own name for the file, and nothing for a file that stands beside a text rather
// than in one.
//
// A file that turns up under a **further owner** gets a record of its own, with
// the bytes of the first. One file stands in one place in Minka — a blob belongs
// to the project of the record that brought it in, and only a blob of that
// project is ever attached to a place in it — so a record naming another's file
// is shown none of it where the two stand in different projects, and one whose
// body is nothing else shows nothing at all.
//
// Such a record is named after its owner and not after the order the walk met
// it in, because Minka takes a record over by its identifier: an identifier that
// moved when another record moved would leave the one an earlier run wrote
// standing and store the bytes again. The two are parted by a dot, which no name
// of the source holds — an identifier of Basecamp is digits, and the key of a
// stored file is held to letters, digits, a hyphen and an underscore before it
// becomes one (blobKeyPattern) — so no such record can carry the name of another
// file.
func (e *Extractor) claimFile(file sourceFile, within map[string]sourceFile, owner fileOwner) (sourceFile, bool) {
	if already, claimed := within[file.ref]; claimed {
		return already, false
	}
	ofTheSource := file.ref
	met := e.theFile(ofTheSource)
	met.records++
	if met.records > 1 {
		e.files.OwnersBeyondTheFirst++
		file.sharesWith = ofTheSource
		file.ref = fmt.Sprintf("%s.%s-%s", ofTheSource, owner.kind, owner.sourceID)
	}
	if within != nil {
		within[ofTheSource] = file
	}
	return file, true
}

// theFile is what the bundle has made of one file of the source, by the source's
// own name for it.
func (e *Extractor) theFile(ofTheSource string) *fileOfTheSource {
	if e.filesOfTheSource == nil {
		e.filesOfTheSource = map[string]*fileOfTheSource{}
	}
	met := e.filesOfTheSource[ofTheSource]
	if met == nil {
		met = &fileOfTheSource{}
		e.filesOfTheSource[ofTheSource] = met
	}
	return met
}

// storageHost is the host a storage address is written with. The Basecamp CLI
// takes a download only from an address of this kind, and sends the request
// to the API it is signed in at, whatever the host says.
const storageHost = "storage.3.basecamp.com"

// storedKey is the form of the key of a stored file: a UUID.
const storedKey = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

var storedKeyPattern = regexp.MustCompile(`^` + storedKey + `$`)

// storageAddressPattern is what reaches the Basecamp CLI as the address of a
// file, and nothing else does: https, a storage host of Basecamp, the
// account, the key of a stored file and one segment for the name.
var storageAddressPattern = regexp.MustCompile(
	`^https://storage\.(?:[a-z0-9-]+\.)*basecamp\.com/[0-9]+/blobs/` + storedKey +
		`/download/([A-Za-z0-9._~!$&'()*+,;=:@%-]+)$`)

// checkStorageAddress refuses whatever is no storage address of a file. The
// Basecamp CLI signs the request it makes for an address, so the check stands
// right before the call, whoever built the address. A refusal never quotes
// it: the address of a file names the file.
func checkStorageAddress(address string) error {
	refused := &Error{Kind: API, Message: "a file is named at an address that is no storage address of Basecamp"}
	match := storageAddressPattern.FindStringSubmatch(address)
	if match == nil {
		return refused
	}
	// The Basecamp CLI asks for the path as it reads once the escapes are
	// taken away. A name that holds a slash then is a path of its own, which
	// leads out of the file and as far as another account.
	if name, err := url.PathUnescape(match[1]); err != nil || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return refused
	}
	return nil
}

// storageAddress builds the address a file is fetched from: the account of
// the run, the key of the stored file and the name as the source writes it
// into its own address, else the escaped name. Nothing else of what the
// source says becomes part of it, and the name only as far as the address
// with it is one that can be asked for.
//
// A file without such an address cannot be fetched by any run, and the reason
// says which part is in the way.
func (e *Extractor) storageAddress(file sourceFile) (string, error) {
	if !storedKeyPattern.MatchString(file.key) {
		return "", &Error{Kind: API, Message: "the key of the stored file has no form that can be asked for"}
	}
	name := url.PathEscape(file.filename)
	if parsed, err := url.Parse(file.downloadURL); err == nil {
		prefix := "/" + e.account + "/blobs/" + file.key + "/download/"
		if segment, found := strings.CutPrefix(parsed.EscapedPath(), prefix); found && !strings.Contains(segment, "/") {
			name = segment
		}
	}
	address := "https://" + storageHost + "/" + e.account + "/blobs/" + file.key + "/download/" + name
	if checkStorageAddress(address) != nil {
		return "", &Error{Kind: API, Message: "the address Basecamp gives the file cannot be asked for safely"}
	}
	return address, nil
}

const (
	// maxCaptionLength is how long a caption may be to stay on a reference,
	// in characters.
	maxCaptionLength = 1000
	// maxExtensionLength is how much of the end of a name counts as its
	// extension when the name is shortened, in bytes.
	maxExtensionLength = 32
	fallbackName       = "file"
)

// plainName makes the name of a file one the applier takes and a file system
// stores: a single segment of a path without control characters, of 255
// characters at most and of no more bytes than that, which is where file
// systems draw the line. It tells whether the name had to change. A name that
// is too long is shortened before its extension, which says what kind of file
// it is.
func plainName(name string) (string, bool) {
	plain := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, strings.ToValidUTF8(name, "_"))
	if plain == "" || plain == "." || plain == ".." {
		plain = fallbackName
	}
	if len(plain) > bundle.MaxNameLength {
		extension := path.Ext(plain)
		if len(extension) > maxExtensionLength || extension == plain {
			extension = ""
		}
		stem := ""
		for _, r := range strings.TrimSuffix(plain, extension) {
			if len(stem)+utf8.RuneLen(r)+len(extension) > bundle.MaxNameLength {
				break
			}
			stem += string(r)
		}
		plain = stem + extension
	}
	return plain, plain != name
}

// largestFileMinkaTakes is the ceiling one file meets on the way in, in bytes.
// Above it the file stays outside there however whole it is here, so a record
// whose body is nothing but that file arrives empty. An instance may be told
// another number; this is the one the format names, and a run can be told
// another, which is how a test reaches what lies above it without a gibibyte.
const largestFileMinkaTakes = 1 << 30

// attachFiles brings the files a text refers to, in the order of their
// references, as attachments of whatever carries the text. times is the record
// that carries them, whose moments a file of it takes.
func (e *Extractor) attachFiles(ctx context.Context, w *bundle.Writer, owner fileOwner, times bundle.Record, files []sourceFile) error {
	for _, file := range files {
		if err := e.attachFile(ctx, w, owner, times, file); err != nil {
			return err
		}
	}
	return nil
}

// attachFile brings one file: shared with a record of the bundle that holds it
// already, taken over from the bundle the run replaces, or else downloaded,
// straight into the bundle. Its record waits for the end of the run, because
// the index begins with the avatars, which come last.
//
// One file of the source is fetched once and weighs once in the report, however
// many records own it; every owner of it gets a record, and one beyond the first
// adds nothing but itself. A record takes the times of its owner, because
// Basecamp gives a file none, and names nobody: whoever wrote the text brought
// it.
//
// Every file is fetched, whatever its size and its type: what a Minka takes
// is the applier's to decide. A file Basecamp no longer has gets a record that
// says so, since no later run will find it. One that fails for any other
// reason leaves no record and is noted as missing, and the run goes on. What
// ends it is an interruption, a bundle that cannot be written, and a Basecamp
// CLI that is not signed in, which no further file would survive either.
//
// A discarding Writer fetches nothing and counts the record.
func (e *Extractor) attachFile(ctx context.Context, w *bundle.Writer, owner fileOwner, times bundle.Record, file sourceFile) error {
	if e.attached[file.ref] {
		return nil
	}
	e.attached[file.ref] = true

	name, changed := plainName(file.filename)
	record := bundle.Attachment{
		Record:        bundle.Record{SourceID: file.ref, CreatedAt: times.CreatedAt, UpdatedAt: times.UpdatedAt},
		Filename:      name,
		OwnerKind:     owner.kind,
		OwnerSourceID: owner.sourceID,
	}
	if file.contentType != "" {
		record.ContentType = &file.contentType
	}
	missing := func(failed *Error, why WhyAFileIsMissing) {
		e.missing = append(e.missing, MissingFile{
			AttachmentSourceID: file.ref,
			OwnerKind:          owner.kind,
			OwnerSourceID:      owner.sourceID,
			Reason:             failed.Message,
			Why:                why,
		})
	}
	met := e.theFile(file.ofTheSource())
	// A further owner of one file takes what the bundle already holds. Where the
	// record that claimed the file first never came to be, because the record it
	// hung on was left out, this one brings the bytes and the next shares with it.
	if file.sharesWith != "" && met.decided {
		return e.shareTheFile(w, record, met, missing)
	}
	met.holder = file.ref
	defer func() { met.decided = true }()

	e.files.Expected++
	if file.byteSize != nil {
		e.files.BytesExpected += *file.byteSize
	}
	if changed {
		e.files.NamesChanged++
	}
	if w.Discards() {
		// All a dry run knows of a file is what the source says about it — the
		// size, and whether the address it gives can be asked for at all. The
		// second is asked here and not left to the run, because a dry run is what
		// somebody looks at before spending an hour, and a file no address can be
		// built for is one no run will ever bring. The file of an upload has no
		// address to weigh: there is nothing a dry run can ask about it.
		if file.byteSize != nil && *file.byteSize > e.LargestFile {
			e.filesThatNeverArrive[file.ref] = true
		}
		if !file.byTheUpload() {
			// Through the same door as a run, and read by the same answer, so that
			// no class of failure means one thing here and another there. Nothing is
			// fetched: what the door hands back is called by nobody.
			if _, err := e.bringing(ctx, file); err != nil {
				ended := howTheDownloadEnded(err)
				switch {
				case ended.endsTheRun:
					return err
				case ended.failed != nil:
					e.theFileDidNotCome(file, met, ended.failed, ended.why, missing)
				}
				return nil
			}
		}
		e.fileRecords = append(e.fileRecords, record)
		return nil
	}

	stored, reused, err := w.ReuseFile(file.ref, name)
	if err != nil {
		return err
	}
	// A file never changes under its ID, so one of another size is another
	// file than the one Basecamp lists.
	if reused && file.byteSize != nil && stored.Bytes != *file.byteSize {
		if err := w.DropFile(file.ref); err != nil {
			return err
		}
		reused = false
	}
	if reused {
		e.files.Reused++
	} else {
		bring, err := e.bringing(ctx, file)
		if err == nil {
			stored, err = w.StoreFile(file.ref, name, func(into io.Writer) error {
				err := bring(into)
				// A file of the size Basecamp gives it is the file, whatever it
				// looks like: the comparison below decides, and nothing else.
				if errors.Is(err, ErrLooksLikeAFailure) && file.byteSize != nil {
					return nil
				}
				return err
			})
		}
		ended := howTheDownloadEnded(err)
		switch {
		case err == nil:
		case ctx.Err() != nil:
			return &Error{Kind: Interrupted, Message: "interrupted"}
		case ended.endsTheRun:
			return err
		case ended.gone:
			record.Gone = true
			met.gone = true
			e.files.Gone++
			e.filesThatNeverArrive[file.ref] = true
			e.fileRecords = append(e.fileRecords, record)
			return nil
		default:
			e.theFileDidNotCome(file, met, ended.failed, ended.why, missing)
			return nil
		}
		if file.byteSize != nil && stored.Bytes != *file.byteSize {
			if err := w.DropFile(file.ref); err != nil {
				return err
			}
			differs := sizeDiffers(stored.Bytes, *file.byteSize)
			e.theFileDidNotCome(file, met, differs, whyTheSourceGaveNoFile(differs), missing)
			return nil
		}
		e.files.Fetched++
	}

	if err := w.KeepFile(file.ref, name, stored); err != nil {
		return err
	}
	e.files.Bytes += stored.Bytes
	if stored.Bytes > e.LargestFile {
		e.filesThatNeverArrive[file.ref] = true
	}
	met.stored, met.held = stored, true
	record.Holds(stored)
	e.fileRecords = append(e.fileRecords, record)
	return nil
}

// shareTheFile writes the record of a further owner of one file: the same bytes
// under the identifier that owner's body names, linked from what the run brought
// for the record that holds them, because two records must never mean two
// downloads.
//
// What became of the file becomes of every owner of it at once: one the source no
// longer has is gone for all of them, and one this run could not fetch is missing
// for all of them, with the reason the first got.
func (e *Extractor) shareTheFile(w *bundle.Writer, record bundle.Attachment, met *fileOfTheSource, missing func(*Error, WhyAFileIsMissing)) error {
	if e.filesThatNeverArrive[met.holder] {
		e.filesThatNeverArrive[record.SourceID] = true
	}
	switch {
	case met.gone:
		record.Gone = true
		e.fileRecords = append(e.fileRecords, record)
		return nil
	case met.failed != nil:
		missing(met.failed, met.why)
		return nil
	case !met.held:
		// A dry run fetches nothing, so there are no bytes to share, and it counts
		// the record as a real run writes it.
		e.fileRecords = append(e.fileRecords, record)
		return nil
	}

	stored, err := w.ShareFile(met.stored, record.SourceID, record.Filename)
	if err != nil {
		return err
	}
	record.Holds(stored)
	e.fileRecords = append(e.fileRecords, record)
	return nil
}

// bringing is what fetches the bytes of a file, by the route the file comes by,
// or the refusal of this run's own that no file gets past. Only a file named at an
// address can be refused here: the file an upload shows is asked for by its
// identifiers, which need no address.
//
// **Whatever it refuses, it refuses as its own**, marked as such: what became of
// a download turns on the difference between a word of this run and an answer of
// the source, and that difference travels with the failure rather than with
// whoever reads it.
func (e *Extractor) bringing(ctx context.Context, file sourceFile) (func(io.Writer) error, error) {
	if file.anEarlierFile() {
		// The size is the one proof that what came is the file of the version,
		// so a version that gives none is not asked for.
		if file.byteSize == nil {
			return nil, &ourOwnRefusal{&Error{Kind: API, Message: "Basecamp gives the earlier file no size, so nothing could tell it from another"}}
		}
		address, err := e.versionAddress(file)
		if err != nil {
			return nil, &ourOwnRefusal{err}
		}
		return func(into io.Writer) error { return e.Files.DownloadVersion(ctx, address, into) }, nil
	}
	if file.byTheUpload() {
		return func(into io.Writer) error {
			return e.Files.DownloadUpload(ctx, file.bucket, file.upload, into)
		}, nil
	}
	address, err := e.storageAddress(file)
	if err != nil {
		return nil, &ourOwnRefusal{err}
	}
	return func(into io.Writer) error { return e.Files.Download(ctx, address, into) }, nil
}

// ourOwnRefusal is a failure this run raised **before it asked anybody**: a key
// it will build no address from, an address it will not hand to the Basecamp
// CLI. Such a failure says nothing about the file, whatever class it carries, so
// nothing standing on the file is written off over it and the run says out loud
// that it did not understand. It is carried by the failure because a guard that
// one day spoke in a class of the source would otherwise be taken for the source.
type ourOwnRefusal struct{ err error }

func (r *ourOwnRefusal) Error() string { return r.err.Error() }
func (r *ourOwnRefusal) Unwrap() error { return r.err }

// howADownloadEnded is what became of a download: nothing of it where the file
// came, the end of the run, a file the source no longer has, or one that is
// missing with the word whose the failure was.
type howADownloadEnded struct {
	endsTheRun bool
	gone       bool
	failed     *Error
	why        WhyAFileIsMissing
}

// howTheDownloadEnded reads a failure of a download, or the want of one. It is
// the one place where both halves of the answer are known — the class, and
// whether this run raised the failure itself — and it takes a class for the
// source's word only where the source gave it: a "not found" of our own would
// otherwise write the file off as one Basecamp no longer has, and a refusal of
// our own as one it hands to nobody.
//
// A sign-in that is gone and an interruption end the run: no further file would
// come either way.
func howTheDownloadEnded(err error) howADownloadEnded {
	if err == nil {
		return howADownloadEnded{}
	}
	var failed *Error
	if !errors.As(err, &failed) || failed.Kind == Interrupted || failed.Kind == AuthRequired {
		return howADownloadEnded{endsTheRun: true}
	}
	var ours *ourOwnRefusal
	why := whoseWordAMissingFileIs(errors.As(err, &ours), failed)
	if why == RefusedBySource && failed.Kind == NotFound {
		return howADownloadEnded{gone: true}
	}
	return howADownloadEnded{failed: failed, why: why}
}

// theFileDidNotCome notes a download that brought no file, names it among the
// missing with the reason and whose word it was, and says whether any run will
// ever bring it.
//
// **Only a refusal of the source decides anything about the file**, and the
// three answers must not be run together: a failure that may pass leaves a hole
// this run reports and the next one fills, a refusal leaves a hole for ever, and
// one nobody could place says nothing about the file at all — the run fails over
// it, and the records that stood on it keep waiting rather than being thrown
// away over an answer this program did not understand.
func (e *Extractor) theFileDidNotCome(file sourceFile, met *fileOfTheSource, failed *Error, why WhyAFileIsMissing, missing func(*Error, WhyAFileIsMissing)) {
	met.failed, met.why = failed, why
	if why.NoRunWillBring() {
		e.filesThatNeverArrive[file.ref] = true
	}
	missing(failed, why)
}

// theFileNeverArrives tells that no run will ever bring the bytes of a file to
// Minka: the source no longer has it, it weighs more than a run takes there, or
// Basecamp itself refused the download. A file this run merely
// missed is not one of them, and neither is one whose failure this run could not
// place: nothing about such a file is decided at all — it leaves no record at all, the run ends
// non-zero, and the run after it brings the file — and neither is one the walk
// has not met yet.
//
// Only a record whose body is nothing else, or whose file is all it stands for,
// asks: for every other body a file that stays outside leaves a hole and nothing
// more. Where such a file left a record behind — one the source no longer has,
// one that weighs too much — that record stays in the bundle as the trace of
// what became of it, whatever became of the record it belonged to.
func (e *Extractor) theFileNeverArrives(ref string) bool {
	return e.filesThatNeverArrive[ref]
}

// sizeDiffers is the failure of a download that ended well and brought
// another number of bytes than Basecamp says the file has. Fewer of them are
// a transfer that broke off, which may pass.
func sizeDiffers(got, want int64) *Error {
	failed := &Error{Kind: API, Message: fmt.Sprintf("the download brought %d bytes of a file of %d", got, want)}
	if got < want {
		failed.Kind = Network
	}
	return failed
}
