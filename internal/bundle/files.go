package bundle

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	attachmentsFile = "attachments.jsonl"
	// MaxNameLength is how long the name of a file may be, in characters.
	MaxNameLength = 255
	// maxIndexLine is the longest record of the bundle being replaced that is
	// read; a longer one is no record a run wrote.
	maxIndexLine = 1 << 20
)

// linkFile is replaced in tests, to see a file taken over where the file
// system makes no links.
var linkFile = os.Link

// StoredFile is what an attachment record says about its file.
type StoredFile struct {
	File   string
	Bytes  int64
	SHA256 string
}

// replacedFile is what the bundle being replaced says about one of its files.
type replacedFile struct {
	file   string
	bytes  int64
	sha256 string
}

// PlainName tells whether a name can stand for one file or one directory
// under files/: a single segment of a path that leads nowhere else, and no
// longer than the applier takes.
func PlainName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		!strings.ContainsAny(name, "/\\\x00") &&
		utf8.ValidString(name) && utf8.RuneCountInString(name) <= MaxNameLength
}

// fileDir is the directory of the one file of an attachment. What the name of
// a file says is somebody's own, so a refusal names the attachment only.
func fileDir(root, sourceID, filename string) (string, error) {
	if !PlainName(sourceID) || !PlainName(filename) {
		return "", fmt.Errorf("the file of attachment %q cannot be stored: its ID or its name cannot be part of a path in the bundle", sourceID)
	}
	return filepath.Join(root, filesDir, sourceID), nil
}

// AddFile stores the content of an attachment under files/ and returns what
// the attachment record says about it. A discarding Writer stores nothing and
// still answers, so that a record can be counted.
func (w *Writer) AddFile(sourceID, filename string, content []byte) (StoredFile, error) {
	return w.StoreFile(sourceID, filename, func(into io.Writer) error {
		_, err := into.Write(content)
		return err
	})
}

// StoreFile stores what fill writes as the file of an attachment, and counts
// and digests it on the way: a file is never held in memory, whatever its
// size. When fill fails, nothing of the file stays and its error comes back
// as it is. A failure of the file system comes without the path it happened
// at, which ends in the name somebody gave their file.
//
// An attachment has one file in a directory of its own, which is made anew
// for it and never taken as it stands: a file that stands there may share its
// content with the bundle being replaced, which a run only reads.
func (w *Writer) StoreFile(sourceID, filename string, fill func(io.Writer) error) (StoredFile, error) {
	dir, err := fileDir(w.staging, sourceID, filename)
	if err != nil {
		return StoredFile{}, err
	}
	digest, size := sha256.New(), &counter{}
	stored := func() StoredFile {
		return StoredFile{File: sourceID + "/" + filename, Bytes: size.count, SHA256: hex.EncodeToString(digest.Sum(nil))}
	}
	if w.Discards() {
		if err := fill(io.MultiWriter(digest, size)); err != nil {
			return StoredFile{}, err
		}
		return stored(), nil
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return StoredFile{}, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return StoredFile{}, err
	}
	file, err := os.OpenFile(filepath.Join(dir, filename), os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		_ = os.RemoveAll(dir)
		return StoredFile{}, fmt.Errorf("the file of attachment %q cannot be stored: %w", sourceID, withoutPath(err))
	}
	err = fill(io.MultiWriter(pathless{file}, digest, size))
	if closeErr := file.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("the file of attachment %q cannot be stored: %w", sourceID, withoutPath(closeErr))
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return StoredFile{}, err
	}
	return stored(), nil
}

// withoutPath is the failure of the file system without the path it happened
// at.
func withoutPath(err error) error {
	var failed *fs.PathError
	if errors.As(err, &failed) {
		return failed.Err
	}
	return err
}

// pathless is a file whose failures to write come without its path. Whoever
// fills a file passes such a failure on as it is, and it ends up in an output.
type pathless struct {
	file *os.File
}

func (p pathless) Write(content []byte) (int, error) {
	written, err := p.file.Write(content)
	return written, withoutPath(err)
}

type counter struct {
	count int64
}

func (c *counter) Write(content []byte) (int, error) {
	c.count += int64(len(content))
	return len(content), nil
}

// ShareFile gives an attachment the file another attachment of this bundle
// already holds, so that one file of the source is fetched once however many
// records own it. The file it comes from is named by what its own record says
// about it, and the copy carries the same size and digest: the bytes are this
// run's own, counted and digested where they arrived, so nothing is read again
// to say what they are.
//
// The file is linked where that can be done, which costs no space, and copied
// otherwise, and a copy is held against the digest it should have. A link shares
// its content, which is safe here for the same reason it is safe for a file taken
// over from the bundle being replaced: every attachment has a directory of its
// own, made anew for it, and nothing of a bundle is ever written into twice.
//
// A discarding Writer is never asked: it holds no bytes for anything to share.
func (w *Writer) ShareFile(held StoredFile, sourceID, filename string) (StoredFile, error) {
	dir, err := fileDir(w.staging, sourceID, filename)
	if err != nil {
		return StoredFile{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return StoredFile{}, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return StoredFile{}, err
	}
	from := filepath.Join(w.staging, filesDir, held.File)
	to := filepath.Join(dir, filename)
	if linkFile(from, to) != nil && !copyFile(from, to, held.SHA256) {
		_ = os.RemoveAll(dir)
		return StoredFile{}, fmt.Errorf("the file of attachment %q cannot be stored beside the one it shares its bytes with", sourceID)
	}
	return StoredFile{File: sourceID + "/" + filename, Bytes: held.Bytes, SHA256: held.SHA256}, nil
}

// DropFile removes the file of an attachment that turned out to be none the
// bundle should carry.
func (w *Writer) DropFile(sourceID string) error {
	if w.Discards() {
		return nil
	}
	dir, err := fileDir(w.staging, sourceID, "file")
	if err != nil {
		return err
	}
	if w.kept != nil {
		delete(w.kept.taken, sourceID)
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("the file of attachment %q cannot be removed: %w", sourceID, withoutPath(err))
	}
	return nil
}

// ReuseFile takes the file of an attachment over from the files an earlier run
// kept, or else from the bundle this run replaces, so that a run does not fetch
// again what a run before brought. It answers false when neither has such a
// file, or not the one its record describes: the file has to lie where the new
// record puts it, with the size and the digest the old record gives it. The
// digest is computed anew, because a record only claims it.
//
// Where the files come from is only read. The file is linked where that can
// be done, which costs no space, and copied otherwise. A link shares its
// content with the old file, so nothing may ever write into a file of the new
// bundle, which the directory made anew for every file sees to; removing
// either name leaves the other whole.
func (w *Writer) ReuseFile(sourceID, filename string) (StoredFile, bool, error) {
	if w.Discards() {
		return StoredFile{}, false, nil
	}
	dir, err := fileDir(w.staging, sourceID, filename)
	if err != nil {
		return StoredFile{}, false, err
	}
	if w.kept != nil && !w.kept.refused {
		stored, taken, err := takeOver(w.kept.dir, w.kept.files, sourceID, filename, dir)
		if err != nil || taken {
			w.kept.taken[sourceID] = taken
			return stored, taken, err
		}
	}
	if w.replaced == nil {
		w.replaced = readReplacedFiles(filepath.Join(w.target, attachmentsFile))
	}
	return takeOver(w.target, w.replaced, sourceID, filename, dir)
}

// takeOver puts the file of an attachment into dir, made anew for it, from the
// files under root that index describes.
func takeOver(root string, index map[string]replacedFile, sourceID, filename, dir string) (StoredFile, bool, error) {
	old, known := index[sourceID]
	if !known || old.file != sourceID+"/"+filename {
		return StoredFile{}, false, nil
	}

	oldDir := filepath.Join(root, filesDir, sourceID)
	oldPath := filepath.Join(oldDir, filename)
	// A link could lead anywhere, so only a real file in a real directory
	// counts.
	if info, err := os.Lstat(oldDir); err != nil || !info.IsDir() {
		return StoredFile{}, false, nil
	}
	info, err := os.Lstat(oldPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() != old.bytes {
		return StoredFile{}, false, nil
	}
	if digest, err := digestOf(oldPath); err != nil || digest != old.sha256 {
		return StoredFile{}, false, nil
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return StoredFile{}, false, err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return StoredFile{}, false, err
	}
	newPath := filepath.Join(dir, filename)
	// A link carries the mode of the old file, so only a file that is its
	// owner's alone is linked.
	if info.Mode().Perm() != 0o600 || linkFile(oldPath, newPath) != nil {
		if !copyFile(oldPath, newPath, old.sha256) {
			_ = os.RemoveAll(dir)
			return StoredFile{}, false, nil
		}
	}
	return StoredFile{File: old.file, Bytes: old.bytes, SHA256: old.sha256}, true, nil
}

// readReplacedFiles reads the index of the bundle at the target: the files it
// says it holds. A record of a file that is gone is left out, because the
// source is asked again for such a file. A bundle without an index, or with
// lines nobody can read, has nothing to take over.
func readReplacedFiles(index string) map[string]replacedFile {
	files := map[string]replacedFile{}
	file, err := os.Open(index)
	if err != nil {
		return files
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), maxIndexLine)
	for scanner.Scan() {
		var record struct {
			SourceID string  `json:"sourceId"`
			File     *string `json:"file"`
			Bytes    *int64  `json:"bytes"`
			SHA256   *string `json:"sha256"`
			Gone     bool    `json:"gone"`
		}
		if json.Unmarshal(scanner.Bytes(), &record) != nil || record.Gone ||
			record.File == nil || record.Bytes == nil || record.SHA256 == nil {
			continue
		}
		files[record.SourceID] = replacedFile{file: *record.File, bytes: *record.Bytes, sha256: *record.SHA256}
	}
	return files
}

func digestOf(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// copyFile copies a file of the bundle being replaced and tells whether the
// copy has the digest it should have: the file is read a second time here,
// and only what was written counts. A copy that fails is none; where the
// bundle itself cannot be written, the download that follows says so.
func copyFile(from, to, sha string) bool {
	source, err := os.Open(from)
	if err != nil {
		return false
	}
	defer source.Close()

	target, err := os.OpenFile(to, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	digest := sha256.New()
	_, err = io.Copy(io.MultiWriter(target, digest), source)
	if closeErr := target.Close(); err == nil {
		err = closeErr
	}
	return err == nil && hex.EncodeToString(digest.Sum(nil)) == sha
}
