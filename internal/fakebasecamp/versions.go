package fakebasecamp

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"

	"github.com/flownative/minka-cli/internal/basecamp"
)

// EarlierFile is a file an upload showed before it was given another: a version
// of the upload, as Basecamp lists it among the versions.
type EarlierFile struct {
	// ID is the identifier of the version, which Basecamp writes into the
	// address the file is downloaded from.
	ID          int64
	Filename    string
	ContentType string
	Bytes       int64
	Creator     *Person
	CreatedAt   string
	// Gone is a file the account lists and no longer has, and NoFile a version
	// the account lists without its file, as it does for a file removed from it.
	Gone   bool
	NoFile bool
}

// StoredKeyOfVersion is what the account serves the earlier file of a version
// under. Nothing of a run ever sees it: it is how a test says which earlier file
// is to fail or to come out at another size.
func StoredKeyOfVersion(id int64) string { return FileKey(id) }

// currentVersion is the identifier of the version of the file an upload shows
// now, made from the upload's own where the fixture names none.
func (u *UploadedFile) currentVersion() int64 {
	if u.VersionID != 0 {
		return u.VersionID
	}
	return u.ID * 100
}

var (
	uploadVersionsPattern  = regexp.MustCompile(`^buckets/([0-9]+)/uploads/([0-9]+)/versions\.json$`)
	versionDownloadPattern = regexp.MustCompile(`^https://3\.basecampapi\.test/([0-9]+)/buckets/([0-9]+)/uploads/([0-9]+)/versions/([0-9]+)/download/([^/]+)$`)
)

// versions answers the versions of an upload the way Basecamp does: the whole
// list newest first, the file the upload shows now marked as current, and the
// same list again for whatever page is asked for.
func (a *Account) versions(path string) ([]byte, bool) {
	match := uploadVersionsPattern.FindStringSubmatch(path)
	if match == nil {
		return nil, false
	}
	project, _ := strconv.ParseInt(match[1], 10, 64)
	id, _ := strconv.ParseInt(match[2], 10, 64)
	p := a.project(match[1])
	upload, known := a.uploadedFile(project, id)
	if p == nil || !known {
		return nil, false
	}

	version := func(versionID int64, action, createdAt string, creator *Person, file map[string]any) map[string]any {
		entry := map[string]any{"id": versionID, "recording_id": id, "action": action, "details": map[string]any{}, "created_at": createdAt}
		if creator != nil {
			entry["creator"] = a.person(*creator)
		}
		if file != nil {
			entry["upload"] = file
		}
		return entry
	}
	file := func(versionID int64, name, contentType string, size int64, current bool) map[string]any {
		return map[string]any{
			"filename": name, "content_type": contentType, "byte_size": size, "current": current,
			"download_url": fmt.Sprintf("%sbuckets/%d/uploads/%d/versions/%d/download/%s", a.root(), p.ID, id, versionID, url.PathEscape(name)),
			"app_download_url": fmt.Sprintf("https://storage.app.basecamp.test/%s/buckets/%d/uploads/%d/versions/%d/download/%s",
				a.ID, p.ID, id, versionID, url.PathEscape(name)),
		}
	}

	shownAction := "active"
	if len(upload.EarlierFiles) > 0 {
		shownAction = "blob_changed"
	}
	shownAt := upload.VersionCreatedAt
	if shownAt == "" {
		shownAt = upload.CreatedAt
	}
	listed := []map[string]any{version(upload.currentVersion(), shownAction, shownAt, upload.Creator,
		file(upload.currentVersion(), upload.urlName(), upload.ContentType, upload.Bytes, true))}
	for i := len(upload.EarlierFiles) - 1; i >= 0; i-- {
		earlier := upload.EarlierFiles[i]
		action := "blob_changed"
		if i == 0 {
			action = "active"
		}
		var held map[string]any
		if !earlier.NoFile {
			held = file(earlier.ID, earlier.Filename, earlier.ContentType, earlier.Bytes, false)
		}
		listed = append(listed, version(earlier.ID, action, earlier.CreatedAt, earlier.Creator, held))
	}
	if a.ReshapeVersions != nil {
		listed = a.ReshapeVersions(id, listed)
	}
	return encode(listed), true
}

// DownloadVersion streams the file of one version of an upload, asked for at the
// address the account gives that version. The file the upload shows now is a
// version as well, so its address brings the bytes the upload's own download
// brings; every other version brings the earlier file it held.
func (a *Account) DownloadVersion(ctx context.Context, address string, into io.Writer) error {
	a.VersionDownloads = append(a.VersionDownloads, address)
	notFound := &basecamp.Error{Kind: basecamp.NotFound, Message: "Basecamp no longer has the earlier file"}

	match := versionDownloadPattern.FindStringSubmatch(address)
	if match == nil || match[1] != a.ID {
		return notFound
	}
	project, _ := strconv.ParseInt(match[2], 10, 64)
	id, _ := strconv.ParseInt(match[3], 10, 64)
	versionID, _ := strconv.ParseInt(match[4], 10, 64)
	upload, known := a.uploadedFile(project, id)
	if !known {
		return notFound
	}
	if ctx.Err() != nil {
		return &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}
	}

	key, size, name, gone := "", int64(0), "", true
	if versionID == upload.currentVersion() {
		key, size, name, gone = StoredKeyOfUpload(id), upload.Bytes, upload.urlName(), upload.Gone
	}
	for _, earlier := range upload.EarlierFiles {
		if earlier.ID == versionID && !earlier.NoFile {
			key, size, name, gone = StoredKeyOfVersion(versionID), earlier.Bytes, earlier.Filename, earlier.Gone
		}
	}
	if gone || match[5] != url.PathEscape(name) {
		return notFound
	}
	if a.BeforeDownload != nil {
		a.BeforeDownload(key)
	}
	if served, replaced := a.ServeBytes[key]; replaced {
		size = served
	}
	if err := a.FailFileOf[key]; err != nil {
		if _, copyErr := io.Copy(into, FileContent(key, size/2)); copyErr != nil {
			return copyErr
		}
		return err
	}
	_, err := io.Copy(into, FileContent(key, size))
	return err
}
