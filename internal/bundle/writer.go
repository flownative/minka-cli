package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	manifestFile   = "manifest.json"
	filesDir       = "files"
	stagingMarker  = ".staging-"
	previousSuffix = ".previous"
	// maxParts is how many parts the name of a part can number.
	maxParts = 9999
)

var (
	// ErrNotABundle is the refusal to replace a directory that holds
	// something other than a bundle of the same source.
	ErrNotABundle = errors.New("the directory is not empty and holds no bundle of this source")
	// ErrSymlink is the refusal to put a bundle where a symbolic link stands.
	ErrSymlink = errors.New("is a symbolic link")
	// ErrLocked tells that another run is writing the same bundle.
	ErrLocked = errors.New("another run is writing this bundle")
)

// Replaced in tests, to see what happens when the file system gives way in the
// middle of putting a bundle in place.
var (
	renameDir = os.Rename
	removeAll = os.RemoveAll
	mkdirTemp = os.MkdirTemp
)

// Writer collects the records of one run.
//
// A bundle holds personal data and replaces the one of the run before, so it
// is built in a directory next to the target that only the user can read, and
// takes the target's place in Commit. A run that fails leaves the target as it
// was. One run at a time works on a target: a second one would take the
// directory of the first for what a crash left behind.
//
// A Writer made by Discard writes nothing and only counts, for a run that
// shows what it would extract.
//
// A kind may be written in parts, where one file would be too big for anybody
// to handle. The writer counts and the caller says where a part may end,
// because only the caller knows which records have to stay in one file. A
// bundle carries either the one file of a kind or its parts, never both: a
// kind that came to a single part stands in its one file.
type Writer struct {
	discard bool
	source  string
	target  string
	staging string
	// created is the topmost directory above the target that this run had to
	// make, so that a run that is given up leaves none of them.
	created string
	lock    *lock
	files   map[Kind]*os.File
	counts  map[Kind]int
	parts   map[Kind]*parted
}

// parted is a kind that is written in parts.
type parted struct {
	// size is how many records a part holds before it ends where it may.
	size  int
	names []string
	// open tells that the last part still takes records, held how many it has.
	open bool
	held int
}

// StoredFile is what an attachment record says about its file.
type StoredFile struct {
	File   string
	Bytes  int64
	SHA256 string
}

// Create prepares a bundle of a source at target. The target may be missing,
// empty, or a bundle of an earlier run over the same source. Until Commit or
// Abort, no other run can work on the same target.
func Create(target, source string) (*Writer, error) {
	target, err := filepath.Abs(target)
	if err != nil {
		return nil, err
	}
	if err := Check(target, source); err != nil {
		return nil, err
	}

	parent := filepath.Dir(target)
	w := &Writer{source: source, target: target, created: firstMissing(parent), files: map[Kind]*os.File{}, counts: map[Kind]int{}, parts: map[Kind]*parted{}}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		w.Abort()
		return nil, err
	}
	if w.lock, err = acquireLock(filepath.Join(parent, "."+filepath.Base(target)+".lock")); err != nil {
		w.Abort()
		return nil, err
	}
	// Only the run that holds the lock knows that what lies around is left
	// over, and not the work of a run that is still going on.
	if err := recoverFromCrash(target); err != nil {
		w.Abort()
		return nil, err
	}
	if w.staging, err = mkdirTemp(parent, "."+filepath.Base(target)+stagingMarker+"*"); err != nil {
		w.Abort()
		return nil, err
	}
	return w, nil
}

// Discard makes a Writer that counts what it is given and writes nothing.
func Discard() *Writer {
	return &Writer{discard: true, counts: map[Kind]int{}, parts: map[Kind]*parted{}}
}

// Check tells whether a run may put its bundle at target. A run replaces
// everything there, so what is there has to be nothing, or a bundle this
// format and this source wrote: a manifest.json alone proves nothing, a web
// application has one too. A symbolic link is refused, because the bundle
// would take the place of the link and the one behind it would stay, with its
// personal data, where nobody looks for it.
func Check(target, source string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s %w", target, ErrSymlink)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", target)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	var manifest struct {
		Format any    `json:"format"`
		Source string `json:"source"`
	}
	content, err := os.ReadFile(filepath.Join(target, manifestFile))
	if err != nil || json.Unmarshal(content, &manifest) != nil ||
		manifest.Format != float64(Format) || manifest.Source != source {
		return fmt.Errorf("%s: %w; it holds %s", target, ErrNotABundle, someNames(entries))
	}
	return nil
}

// someNames says what is in the way, which is not always visible: a file
// manager leaves hidden files in a directory that looks empty.
func someNames(entries []fs.DirEntry) string {
	const shown = 3
	var names []string
	for i, entry := range entries {
		if i == shown {
			names = append(names, fmt.Sprintf("and %d more", len(entries)-shown))
			break
		}
		names = append(names, entry.Name())
	}
	return strings.Join(names, ", ")
}

func firstMissing(dir string) string {
	missing := ""
	for {
		if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
			return missing
		}
		missing = dir
		dir = filepath.Dir(dir)
	}
}

// recoverFromCrash tidies up what a run left next to the target when it was
// killed. What it was building is personal data nobody will use, so it goes.
// A bundle that had already been moved aside comes back when the new one never
// took its place.
//
// Only real directories with exactly the name a run gives them are touched:
// the name of the target is somebody's input and must not work as a pattern,
// and a link could lead anywhere.
func recoverFromCrash(target string) error {
	parent := filepath.Dir(target)
	ours := regexp.MustCompile(`^` + regexp.QuoteMeta("."+filepath.Base(target)+stagingMarker) + `[0-9]+(` + regexp.QuoteMeta(previousSuffix) + `)?$`)

	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	var leftovers []string
	for _, entry := range entries {
		if entry.IsDir() && ours.MatchString(entry.Name()) {
			leftovers = append(leftovers, filepath.Join(parent, entry.Name()))
		}
	}

	for _, leftover := range leftovers {
		if !strings.HasSuffix(leftover, previousSuffix) {
			continue
		}
		if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			if err := renameDir(leftover, target); err != nil {
				return err
			}
		}
	}
	for _, leftover := range leftovers {
		if err := removeAll(leftover); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) Discards() bool {
	return w.discard
}

// Path is where the bundle stands after Commit.
func (w *Writer) Path() string {
	return w.target
}

// Counts are the records added so far, per kind.
func (w *Writer) Counts() map[Kind]int {
	counts := make(map[Kind]int, len(w.counts))
	for kind, count := range w.counts {
		counts[kind] = count
	}
	return counts
}

// InParts has a kind written in parts, named <kind>-0001.jsonl and on in the
// order they are written. A part ends at the first place the caller allows
// once it holds size records, so it may hold more than that. It is said
// before the first record of the kind is added.
func (w *Writer) InParts(kind Kind, size int) error {
	if size < 1 || w.counts[kind] > 0 {
		return fmt.Errorf("%s cannot be written in parts of %d once it holds %d records", kind, size, w.counts[kind])
	}
	w.parts[kind] = &parted{size: size}
	return nil
}

// MayCut says that the next record of a kind may begin a part: the part that
// is being written ends here when it holds its size or more.
func (w *Writer) MayCut(kind Kind) error {
	part := w.parts[kind]
	if part == nil || !part.open || part.held < part.size {
		return nil
	}
	return w.EndPart(kind)
}

// EndPart closes the part of a kind that is being written, so that the next
// record begins a new one: where a part has to end with what it holds, whatever
// its size. Without a part that is open it does nothing, so no part is empty
// and the numbers have no gap.
func (w *Writer) EndPart(kind Kind) error {
	part := w.parts[kind]
	if part == nil || !part.open {
		return nil
	}
	part.open, part.held = false, 0
	file, ok := w.files[kind]
	if !ok {
		return nil
	}
	delete(w.files, kind)
	return file.Close()
}

// Parts names the files of every kind that came to more than one part, in the
// order they were written. A discarding Writer answers the same as one that
// writes.
func (w *Writer) Parts() map[Kind][]string {
	var parts map[Kind][]string
	for kind, part := range w.parts {
		if len(part.names) > 1 {
			if parts == nil {
				parts = map[Kind][]string{}
			}
			parts[kind] = append([]string(nil), part.names...)
		}
	}
	return parts
}

// fileFor names the file the next record of a kind goes into. For a kind in
// parts that is a new part when none is open.
func (w *Writer) fileFor(kind Kind) (string, error) {
	part := w.parts[kind]
	if part == nil {
		return string(kind) + ".jsonl", nil
	}
	if !part.open {
		if len(part.names) == maxParts {
			return "", fmt.Errorf("%s came to more than the %d parts a name can number", kind, maxParts)
		}
		part.names = append(part.names, fmt.Sprintf("%s-%04d.jsonl", kind, len(part.names)+1))
		part.open = true
	}
	part.held++
	return part.names[len(part.names)-1], nil
}

// Add appends a record to the file of its kind. The applier reads a file from
// top to bottom, so a parent has to be added before its children.
func (w *Writer) Add(kind Kind, record any) error {
	name, err := w.fileFor(kind)
	if err != nil {
		return err
	}
	w.counts[kind]++
	if w.Discards() {
		return nil
	}

	file, ok := w.files[kind]
	if !ok {
		file, err = os.OpenFile(filepath.Join(w.staging, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		w.files[kind] = file
	}

	line, err := encode(record, "")
	if err != nil {
		return err
	}
	_, err = file.Write(line)
	return err
}

// AddFile stores the content of an attachment under files/ and returns what
// the attachment record says about it. A discarding Writer stores nothing and
// still answers, so that a record can be counted.
func (w *Writer) AddFile(sourceID, filename string, content []byte) (StoredFile, error) {
	for _, segment := range []string{sourceID, filename} {
		if segment == "" || segment == "." || segment == ".." || strings.ContainsAny(segment, `/\`) {
			return StoredFile{}, fmt.Errorf("%q cannot be part of the path of a file in the bundle", segment)
		}
	}

	sum := sha256.Sum256(content)
	stored := StoredFile{
		File:   sourceID + "/" + filename,
		Bytes:  int64(len(content)),
		SHA256: hex.EncodeToString(sum[:]),
	}
	if w.Discards() {
		return stored, nil
	}

	dir := filepath.Join(w.staging, filesDir, sourceID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return StoredFile{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, filename), content, 0o600); err != nil {
		return StoredFile{}, err
	}
	return stored, nil
}

// Commit writes the manifest with the counts and the parts, and puts the
// bundle in the place of the target.
//
// The bundle of the run before is moved aside first and removed last. When it
// cannot be removed, the new bundle is in place all the same: leftover then
// names where the old one still lies, and the run has not failed.
func (w *Writer) Commit(manifest Manifest) (leftover string, err error) {
	manifest.Format = Format
	manifest.Source = w.source
	manifest.Counts = w.Counts()
	manifest.Parts = w.Parts()
	if w.Discards() {
		return "", nil
	}

	for kind, file := range w.files {
		delete(w.files, kind)
		if err := file.Close(); err != nil {
			return "", err
		}
	}
	// A kind that came to one part is the one file of its kind.
	for kind, part := range w.parts {
		if len(part.names) == 1 {
			if err := os.Rename(filepath.Join(w.staging, part.names[0]), filepath.Join(w.staging, string(kind)+".jsonl")); err != nil {
				return "", err
			}
		}
	}
	content, err := encode(manifest, "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(w.staging, manifestFile), content, 0o600); err != nil {
		return "", err
	}

	// Somebody may have put something there while the run went on.
	if err := Check(w.target, w.source); err != nil {
		return "", err
	}
	previous := w.staging + previousSuffix
	err = renameDir(w.target, previous)
	hadPrevious := err == nil
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := renameDir(w.staging, w.target); err != nil {
		if hadPrevious {
			if restoreErr := renameDir(previous, w.target); restoreErr != nil {
				return "", fmt.Errorf("%w; the bundle of the run before could not be put back either and now lies at %s", err, previous)
			}
		}
		return "", err
	}
	w.staging, w.created = "", ""
	if hadPrevious && removeAll(previous) != nil {
		leftover = previous
	}
	w.lock.release()
	return leftover, nil
}

// Abort throws away what was written. After Commit it does nothing, so it can
// be deferred.
func (w *Writer) Abort() {
	for kind, file := range w.files {
		delete(w.files, kind)
		_ = file.Close()
	}
	if w.staging != "" {
		_ = os.RemoveAll(w.staging)
		w.staging = ""
	}
	w.lock.release()
	if w.created != "" {
		// Remove takes only an empty directory, so nothing of anybody else goes.
		for dir := filepath.Dir(w.target); strings.HasPrefix(dir, w.created); dir = filepath.Dir(dir) {
			if os.Remove(dir) != nil {
				break
			}
		}
		w.created = ""
	}
}

// encode keeps HTML readable: a description stands in the bundle the way the
// applier gets it.
func encode(value any, indent string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
