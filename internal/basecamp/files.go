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

// Files hands out the files Basecamp stores for an account.
type Files interface {
	// Download writes the file at a storage address into a writer, as it comes.
	// A file Basecamp no longer has is a NotFound. What was written before a
	// failure is no file, with one exception: ErrLooksLikeAFailure comes with
	// everything that was to come, for whoever knows better what the file is.
	Download(ctx context.Context, address string, into io.Writer) error
}

// ErrLooksLikeAFailure is a download that ended well and brought nothing but
// what the Basecamp CLI prints when it fails. That may be the file, a small
// one of JSON that somebody uploaded, so all of it was written; only the size
// Basecamp gives the file can tell.
var ErrLooksLikeAFailure = &Error{Kind: API, Message: "the Basecamp CLI printed a failure in place of the file and ended as if it had brought it"}

// FileCounts says what became of the files the texts and the chats of a run
// refer to. A file counts once, however many references name it.
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
	// SecondOwners counts the times a file turned up under another owner than
	// its first, who keeps it.
	SecondOwners int
}

// sourceFile is a file a text or a line refers to, as Basecamp lists it.
type sourceFile struct {
	// ref is the ID of the attachment record, which the reference carries.
	ref         string
	key         string
	filename    string
	contentType string
	// byteSize is nil where Basecamp does not say how large the file is.
	byteSize    *int64
	downloadURL string
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
	// largestFileMinkaTakes is the ceiling one file meets on the way in, in
	// bytes. Above it the file stays outside there however whole it is here, so
	// a record whose body is nothing but that file arrives empty. An instance
	// may be told another number; this is the one the format names.
	largestFileMinkaTakes = 1 << 30
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

// attachFiles brings the files a text refers to, in the order of their
// references, as attachments of whatever carries the text.
func (e *Extractor) attachFiles(ctx context.Context, w *bundle.Writer, ownerKind string, owner bundle.Record, files []sourceFile) error {
	for _, file := range files {
		if err := e.attachFile(ctx, w, ownerKind, owner, file); err != nil {
			return err
		}
	}
	return nil
}

// attachFile brings one file: taken over from the bundle the run replaces, or
// else downloaded, straight into the bundle. Its record waits for the end of
// the run, because the index begins with the avatars, which come last.
//
// A file has one record and is fetched once, however many references name it;
// the first owner keeps it. It takes the times of its owner, because Basecamp
// gives a file none, and names nobody: whoever wrote the text brought it.
//
// Every file is fetched, whatever its size and its type: what a Minka takes
// is the applier's to decide. A file Basecamp no longer has gets a record that
// says so, since no later run will find it. One that fails for any other
// reason leaves no record and is noted as missing, and the run goes on. What
// ends it is an interruption, a bundle that cannot be written, and a Basecamp
// CLI that is not signed in, which no further file would survive either.
//
// A discarding Writer fetches nothing and counts the record.
func (e *Extractor) attachFile(ctx context.Context, w *bundle.Writer, ownerKind string, owner bundle.Record, file sourceFile) error {
	if first, met := e.fileOwners[file.ref]; met {
		if first != ownerKind+" "+owner.SourceID {
			e.files.SecondOwners++
		}
		return nil
	}
	e.fileOwners[file.ref] = ownerKind + " " + owner.SourceID
	e.files.Expected++
	if file.byteSize != nil {
		e.files.BytesExpected += *file.byteSize
	}

	name, changed := plainName(file.filename)
	if changed {
		e.files.NamesChanged++
	}
	record := bundle.Attachment{
		Record:        bundle.Record{SourceID: file.ref, CreatedAt: owner.CreatedAt, UpdatedAt: owner.UpdatedAt},
		Filename:      name,
		OwnerKind:     ownerKind,
		OwnerSourceID: owner.SourceID,
	}
	if file.contentType != "" {
		record.ContentType = &file.contentType
	}
	if w.Discards() {
		// All a dry run knows of a file is the size the source gives it.
		if file.byteSize == nil || *file.byteSize <= largestFileMinkaTakes {
			e.filesThatArrive[file.ref] = true
		}
		e.fileRecords = append(e.fileRecords, record)
		return nil
	}
	missing := func(failed *Error) {
		e.missing = append(e.missing, MissingFile{
			AttachmentSourceID: file.ref,
			OwnerKind:          ownerKind,
			OwnerSourceID:      owner.SourceID,
			Reason:             failed.Message,
			Retryable:          failed.Transient(),
		})
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
		address, err := e.storageAddress(file)
		if err == nil {
			stored, err = w.StoreFile(file.ref, name, func(into io.Writer) error {
				err := e.Files.Download(ctx, address, into)
				// A file of the size Basecamp gives it is the file, whatever it
				// looks like: the comparison below decides, and nothing else.
				if errors.Is(err, ErrLooksLikeAFailure) && file.byteSize != nil {
					return nil
				}
				return err
			})
		}
		var failed *Error
		switch {
		case err == nil:
		case ctx.Err() != nil:
			return &Error{Kind: Interrupted, Message: "interrupted"}
		case !errors.As(err, &failed) || failed.Kind == Interrupted || failed.Kind == AuthRequired:
			return err
		case failed.Kind == NotFound:
			record.Gone = true
			e.files.Gone++
			e.fileRecords = append(e.fileRecords, record)
			return nil
		default:
			missing(failed)
			return nil
		}
		if file.byteSize != nil && stored.Bytes != *file.byteSize {
			if err := w.DropFile(file.ref); err != nil {
				return err
			}
			missing(sizeDiffers(stored.Bytes, *file.byteSize))
			return nil
		}
		e.files.Fetched++
	}

	e.files.Bytes += stored.Bytes
	if stored.Bytes <= largestFileMinkaTakes {
		e.filesThatArrive[file.ref] = true
	}
	record.Holds(stored)
	e.fileRecords = append(e.fileRecords, record)
	return nil
}

// theFileArrives tells whether the bytes of a file reach Minka: whether the
// bundle carries them and whether Minka takes a file of that size. Only what
// arrives is written down, so a file the walk never met, one it could not
// fetch, one the source no longer has and one too large all answer the same.
// Only a record whose body is nothing else asks: for every other body a file
// that stays outside leaves a hole and nothing more.
func (e *Extractor) theFileArrives(ref string) bool {
	return e.filesThatArrive[ref]
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
