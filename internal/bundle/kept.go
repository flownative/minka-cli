package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	keptSuffix = ".kept"
	keptRecord = "kept.json"
	keptIndex  = "files.jsonl"
)

// ErrKeptThroughALink is the refusal to read or write kept files through a
// symbolic link, which could lead anywhere.
var ErrKeptThroughALink = errors.New("is a symbolic link where kept files lie")

// ErrKeptForAnotherRun is the refusal to take files over that were kept under
// another source, account or program: nothing could then say what they are.
var ErrKeptForAnotherRun = errors.New("the files kept beside this bundle were brought by another kind of run")

// keptFiles is where a run that keeps its files puts every file it brought, for
// a run after it that ends before its bundle is in place: a bundle takes its
// place only at the end, and a run that fails late would lose hours of
// downloads with it.
//
// The place lies beside the target under a name of its own, made before the
// first file comes, so that no tidying up after a killed run takes it for what
// was being built. It holds the files under the same paths as a bundle, an
// index with a line per file that is written once the file is whole, and the
// record of what the files were brought under.
type keptFiles struct {
	dir    string
	record keptUnder
	// files is what the index says, the last line of a file counting.
	files map[string]replacedFile
	// index is nil for a discarding Writer, which only checks.
	index *os.File
	// taken are the files this run took over from the place, which therefore
	// stand there already.
	taken map[string]bool
	// removed tells that the place went with a run that put its bundle in place.
	removed bool
	// refused tells that the files are of another account, which nothing takes
	// over and nothing adds to.
	refused bool
}

// keptUnder is what kept files were brought under. A file is taken over by its
// identifier, its name, its size and its digest, which say what it is only
// within one account of one source, and only as long as the program that
// names the files names them the same way.
type keptUnder struct {
	Source         string `json:"source"`
	SourceInstance string `json:"sourceInstance"`
	Extractor      string `json:"extractor"`
}

// KeptBeside names the place where a run that keeps its files puts them for a
// bundle at target, when one is there.
func KeptBeside(target string) string {
	dir := keptDir(target)
	if info, err := os.Lstat(dir); err == nil && info.IsDir() {
		return dir
	}
	return ""
}

func keptDir(target string) string {
	return filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+keptSuffix)
}

// Keep has the writer keep every file it is given with KeepFile beside the
// target, and take files over from there that an earlier run kept. extractor
// names the program and its version.
//
// Files kept by a run of another source or another extractor are refused, and
// so are those of another account, once ReadsFrom names it. A discarding Writer
// made for a target checks the same and touches nothing.
func (w *Writer) Keep(extractor string) error {
	if w.target == "" {
		return nil
	}
	dir := keptDir(w.target)
	kept := &keptFiles{dir: dir, files: map[string]replacedFile{}, taken: map[string]bool{}}

	info, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if w.discard {
			return nil
		}
		if err := os.Mkdir(dir, 0o700); err != nil {
			return err
		}
	case err != nil:
		return err
	case info.Mode()&fs.ModeSymlink != 0:
		return fmt.Errorf("%s %w", dir, ErrKeptThroughALink)
	case !info.IsDir():
		return fmt.Errorf("%s is not a directory", dir)
	}

	// What a run reads and writes in the place has to lie in it: a link could
	// lead anywhere, as the place itself could.
	for _, name := range []string{keptRecord, keptIndex, filesDir} {
		if entry, err := os.Lstat(filepath.Join(dir, name)); err == nil && entry.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%s %w", filepath.Join(dir, name), ErrKeptThroughALink)
		}
	}
	record, err := readKeptRecord(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist) && !holdsFiles(dir):
		// The record comes before the first file, so a place without either is
		// one a run made and ended in before it wrote the record: there is
		// nothing in it to vouch for.
		if w.discard {
			return nil
		}
		kept.record = keptUnder{Source: w.source, Extractor: extractor}
		if err := writeKeptRecord(dir, kept.record); err != nil {
			return err
		}
	case err != nil:
		return fmt.Errorf("%s: %w: it says nothing a run can read about what they were brought under", dir, ErrKeptForAnotherRun)
	case record.Source != w.source:
		return fmt.Errorf("%s: %w: they came from %q, this run reads %q", dir, ErrKeptForAnotherRun, record.Source, w.source)
	case record.Extractor != extractor:
		return fmt.Errorf("%s: %w: %q brought them, this run is %q", dir, ErrKeptForAnotherRun, record.Extractor, extractor)
	default:
		kept.record = record
		if w.discard {
			w.kept = kept
			return nil
		}
		kept.files = readReplacedFiles(filepath.Join(dir, keptIndex))
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}

	if kept.index, err = openIndex(filepath.Join(dir, keptIndex)); err != nil {
		return err
	}
	w.kept = kept
	return nil
}

// openIndex opens the index of kept files for the lines of this run. A run
// that ended in the middle of a line left it without its end, and the first
// line of this run begins on a line of its own, so that it is not lost with
// the half before it.
func openIndex(path string) (*os.File, error) {
	index, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	last := make([]byte, 1)
	if info, err := index.Stat(); err == nil && info.Size() > 0 {
		if _, err := index.ReadAt(last, info.Size()-1); err == nil && last[0] != '\n' {
			_, err = index.Write([]byte("\n"))
		}
		if err != nil {
			_ = index.Close()
			return nil, err
		}
	}
	return index, nil
}

// ReadsFrom tells the writer which instance of the source the run reads, as
// soon as it is known and before any file is asked for. Files kept from
// another instance are refused; a place that names none yet takes this one.
func (w *Writer) ReadsFrom(instance string) error {
	kept := w.kept
	if kept == nil || kept.record.SourceInstance == instance {
		return nil
	}
	if kept.record.SourceInstance != "" {
		kept.refused = true
		return fmt.Errorf("%s: %w: they came from %s %s, this run reads %s %s", kept.dir, ErrKeptForAnotherRun,
			kept.record.Source, kept.record.SourceInstance, w.source, instance)
	}
	if w.discard {
		return nil
	}
	kept.record.SourceInstance = instance
	return writeKeptRecord(kept.dir, kept.record)
}

// KeepFile puts the file of an attachment that this run holds into the place of
// kept files, unless it was taken over from there. What stood there under the
// same identifier goes first, by its name. The file is linked, which costs no
// space, or copied where it cannot be, and its line in the index comes only
// once it is whole: a run killed in between leaves a file the index does not
// vouch for, which nobody takes over.
func (w *Writer) KeepFile(sourceID, filename string, stored StoredFile) error {
	kept := w.kept
	if w.discard || kept == nil || kept.refused || kept.taken[sourceID] {
		return nil
	}
	dir, err := fileDir(kept.dir, sourceID, filename)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("the file of attachment %q cannot be kept: %w", sourceID, withoutPath(err))
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	from := filepath.Join(w.staging, filesDir, sourceID, filename)
	to := filepath.Join(dir, filename)
	if linkFile(from, to) != nil && !copyFile(from, to, stored.SHA256) {
		_ = os.RemoveAll(dir)
		return fmt.Errorf("the file of attachment %q cannot be kept", sourceID)
	}

	line, err := encode(struct {
		SourceID string `json:"sourceId"`
		File     string `json:"file"`
		Bytes    int64  `json:"bytes"`
		SHA256   string `json:"sha256"`
	}{sourceID, stored.File, stored.Bytes, stored.SHA256}, "")
	if err != nil {
		return err
	}
	if _, err := kept.index.Write(line); err != nil {
		return fmt.Errorf("the file of attachment %q cannot be kept: %w", sourceID, withoutPath(err))
	}
	kept.files[sourceID] = replacedFile{file: stored.File, bytes: stored.Bytes, sha256: stored.SHA256}
	return nil
}

// Kept names the place of the kept files while they are there: during a run
// that keeps them, after one that was given up, and after one that put its
// bundle in place and could not remove them.
func (w *Writer) Kept() string {
	if w.discard || w.kept == nil || w.kept.removed {
		return ""
	}
	return w.kept.dir
}

// closeIndex ends the writing of the index of kept files.
func (k *keptFiles) closeIndex() {
	if k != nil && k.index != nil {
		_ = k.index.Close()
		k.index = nil
	}
}

// holdsFiles tells whether a place holds anything beside its record.
func holdsFiles(dir string) bool {
	for _, name := range []string{keptIndex, filesDir} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, fs.ErrNotExist) {
			return true
		}
	}
	return false
}

func readKeptRecord(dir string) (keptUnder, error) {
	var record keptUnder
	content, err := os.ReadFile(filepath.Join(dir, keptRecord))
	if err != nil {
		return record, err
	}
	err = json.Unmarshal(content, &record)
	return record, err
}

// writeKeptRecord replaces the record with a rename, so that a run killed while
// writing it leaves the one before. What stands at the name it is written
// under first is what a run before left there, which goes by its name, so
// that the record is never written through a link.
func writeKeptRecord(dir string, record keptUnder) error {
	content, err := encode(record, "  ")
	if err != nil {
		return err
	}
	written := filepath.Join(dir, keptRecord+".new")
	if err := os.Remove(written); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	file, err := os.OpenFile(written, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(content)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(written, filepath.Join(dir, keptRecord))
}
