package basecamp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// VersionCounts says what became of the earlier files of the uploads a run
// wrote: the files an upload showed before it was given the one it shows now.
type VersionCounts struct {
	// UploadsWithEarlierFiles counts the uploads whose versions the bundle
	// carries, and EarlierFiles the entries of those versions before the file
	// each upload shows now, whatever became of their bytes.
	UploadsWithEarlierFiles int `json:"uploadsWithEarlierFiles"`
	EarlierFiles            int `json:"earlierFiles"`
	// WithoutTheirFile names the versions Basecamp lists without the file they
	// held, which it does for a file removed from the account: there is nothing
	// to bring, so the entry is left out of the upload's versions.
	WithoutTheirFile []VersionWithoutAFile `json:"withoutTheirFile"`
	// HistoriesUnread names the uploads whose versions this run did not read,
	// with why: Basecamp would not hand them out, or handed them out in a shape
	// this run cannot place. Such an upload comes with the file it shows now and
	// without the earlier ones.
	HistoriesUnread []UnreadHistory `json:"historiesUnread"`
}

// VersionWithoutAFile is a version of an upload Basecamp lists without its file.
type VersionWithoutAFile struct {
	UploadSourceID  string `json:"uploadSourceId"`
	VersionSourceID string `json:"versionSourceId"`
}

// WhyAHistoryIsUnread says why the versions of an upload were not read.
type WhyAHistoryIsUnread string

const (
	HistoryNotFound  WhyAHistoryIsUnread = "notFound"
	HistoryForbidden WhyAHistoryIsUnread = "forbidden"
	// HistoryNotUnderstood is a list that does not name the file the upload
	// shows now first and once, or names a version without an identifier: what
	// it says about the earlier files cannot be told apart from what it says
	// about the current one.
	HistoryNotUnderstood WhyAHistoryIsUnread = "notUnderstood"
)

// UnreadHistory is an upload whose versions this run did not read.
type UnreadHistory struct {
	UploadSourceID  string              `json:"uploadSourceId"`
	ProjectSourceID string              `json:"projectSourceId"`
	Why             WhyAHistoryIsUnread `json:"why"`
}

// uploadVersion is one entry of what Basecamp lists of the versions of an
// upload, newest first: the file the upload was given, when and by whom. File is
// nil where Basecamp no longer has the file of that version.
type uploadVersion struct {
	ID        int64        `json:"id"`
	CreatedAt string       `json:"created_at"`
	Creator   *person      `json:"creator"`
	File      *versionFile `json:"upload"`
}

type versionFile struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ByteSize    *int64 `json:"byte_size"`
	DownloadURL string `json:"download_url"`
	Current     bool   `json:"current"`
}

// earlierFilesOf brings the files an upload showed before the one it shows now
// and returns its versions, oldest first and that file last, or nothing where
// it was never given another.
//
// The list is read **as one answer and never by pages**: Basecamp answers every
// page of it with the whole list, which a walk by pages takes for a list that
// moved under it. Nothing on the upload says whether it has earlier files, so
// the list is asked for every upload a bundle carries.
//
// Every entry the list names with a file is written, whatever became of its
// bytes: one Basecamp no longer has stands in the bundle as gone and one this
// run could not fetch is named among the missing files, and Minka leaves either
// out of the versions with a warning. An entry without a file has nothing to
// stand for and is left out and named.
func (e *Extractor) earlierFilesOf(ctx context.Context, w *bundle.Writer, bucket int64, found vaultUpload, mine fileOwner, upload bundle.Record, shownFile string) ([]bundle.UploadVersion, error) {
	listed, err := getOne[[]uploadVersion](ctx, e.source, fmt.Sprintf("buckets/%d/uploads/%d/versions.json", bucket, found.ID))
	var failed *Error
	if errors.As(err, &failed) && (failed.Kind == NotFound || failed.Kind == Forbidden) {
		why := HistoryNotFound
		if failed.Kind == Forbidden {
			why = HistoryForbidden
		}
		e.historyUnread(bucket, found.ID, why)
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !theCurrentFileComesFirst(listed) {
		e.historyUnread(bucket, found.ID, HistoryNotUnderstood)
		return nil, nil
	}

	var earlier []uploadVersion
	for i := len(listed) - 1; i > 0; i-- {
		if listed[i].File == nil {
			e.docs.Versions.WithoutTheirFile = append(e.docs.Versions.WithoutTheirFile,
				VersionWithoutAFile{UploadSourceID: sourceID(found.ID), VersionSourceID: sourceID(listed[i].ID)})
			continue
		}
		earlier = append(earlier, listed[i])
	}
	if len(earlier) == 0 {
		return nil, nil
	}

	versions := make([]bundle.UploadVersion, 0, len(earlier)+1)
	for _, version := range earlier {
		entry, err := e.versionEntry(found.ID, version, fmt.Sprintf("upload-%d-version-%d", found.ID, version.ID), upload)
		if err != nil {
			return nil, err
		}
		file, _ := e.claimFile(sourceFile{
			ref: entry.File, upload: found.ID, bucket: bucket, version: version.ID,
			filename: version.File.Filename, contentType: version.File.ContentType, byteSize: version.File.ByteSize,
			downloadURL: version.File.DownloadURL,
		}, nil, mine)
		// Basecamp gives the file of a version the moment it was given and no
		// other, so that moment is both of its times.
		if err := e.attachFile(ctx, w, mine, bundle.Record{CreatedAt: version.CreatedAt, UpdatedAt: version.CreatedAt}, file); err != nil {
			return nil, err
		}
		versions = append(versions, entry)
	}
	shown, err := e.versionEntry(found.ID, listed[0], shownFile, upload)
	if err != nil {
		return nil, err
	}
	e.docs.Versions.UploadsWithEarlierFiles++
	e.docs.Versions.EarlierFiles += len(versions)
	return append(versions, shown), nil
}

// theCurrentFileComesFirst tells a list of versions this run can read: newest
// first, the file the upload shows now named once and at the top, and every
// version with an identifier of its own. Anything else leaves open which file is
// the current one, and a version written for the wrong one would put the file
// the upload shows among the earlier ones.
func theCurrentFileComesFirst(listed []uploadVersion) bool {
	if len(listed) == 0 || listed[0].File == nil || !listed[0].File.Current {
		return false
	}
	seen := map[int64]bool{}
	for i, version := range listed {
		if version.ID <= 0 || seen[version.ID] || (i > 0 && version.File != nil && version.File.Current) {
			return false
		}
		seen[version.ID] = true
	}
	return true
}

// versionEntry is one entry of the versions of an upload: the record of its
// file, the moment the upload was given it and who gave it, nobody named where
// that was whoever made the upload.
func (e *Extractor) versionEntry(upload int64, version uploadVersion, file string, owner bundle.Record) (bundle.UploadVersion, error) {
	what := fmt.Sprintf("version %d of upload %d", version.ID, upload)
	if err := requireTimestamps(what, version.CreatedAt, version.CreatedAt); err != nil {
		return bundle.UploadVersion{}, err
	}
	by, err := e.people.refer(version.Creator, false)
	if err != nil {
		return bundle.UploadVersion{}, err
	}
	if by != nil && owner.CreatedBy != nil && *by == *owner.CreatedBy {
		by = nil
	}
	return bundle.UploadVersion{File: file, CreatedAt: version.CreatedAt, CreatedBy: by}, nil
}

func (e *Extractor) historyUnread(bucket, upload int64, why WhyAHistoryIsUnread) {
	e.docs.Versions.HistoriesUnread = append(e.docs.Versions.HistoriesUnread,
		UnreadHistory{UploadSourceID: sourceID(upload), ProjectSourceID: sourceID(bucket), Why: why})
}

// nameSegmentCharacters make one segment of a path that names a file, as
// Basecamp escapes it.
const nameSegmentCharacters = `[A-Za-z0-9._~!$&'()*+,;=:@%-]+`

var nameSegment = regexp.MustCompile(`^` + nameSegmentCharacters + `$`)

// versionAddress is the address an earlier file is fetched at: the version's
// own on the API of the account, which Basecamp answers with the way to that
// file and no other. It is taken from the address Basecamp gives the version
// only where that names this account, the project, the upload and the version
// and nothing but a name behind them; the host and the path of an answer are
// otherwise never passed on.
func (e *Extractor) versionAddress(file sourceFile) (string, error) {
	prefix := fmt.Sprintf("%sbuckets/%d/uploads/%d/versions/%d/download/", e.apiRoot, file.bucket, file.upload, file.version)
	name, found := strings.CutPrefix(file.downloadURL, prefix)
	if e.apiRoot == "" || !found || !nameSegment.MatchString(name) || !aSingleSegment(name) {
		return "", &Error{Kind: API, Message: "the address Basecamp gives the earlier file cannot be asked for safely"}
	}
	return prefix + name, nil
}
