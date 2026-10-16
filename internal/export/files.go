package export

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/flownative/minka-cli/internal/api"
)

// ManifestName is the file an export keeps in its directory: what it wrote,
// so that the next run knows what is its own to change and to remove.
const ManifestName = ".minka-export.json"

// tempPrefix starts the name of a file being written; one left by a run that
// was killed is taken away by the next.
const tempPrefix = ".minka-export-"

const manifestVersion = 1

type manifest struct {
	Version     int            `json:"version"`
	Host        string         `json:"host"`
	FolderID    string         `json:"folderId"`
	ToolID      string         `json:"toolId,omitempty"`
	Format      string         `json:"format"`
	ExportedAt  string         `json:"exportedAt"`
	Files       []manifestFile `json:"files"`
	Directories []string       `json:"directories"`
	// ReadFrom says how the folder was read: "tree", "book" or "walk".
	ReadFrom string `json:"readFrom,omitempty"`
	// Pages are the bodies of the pages as read, by their ID, for the next
	// run to take where Minka names the same version. A run takes them only
	// in the format PagesFormat names, which this minka writes.
	PagesFormat int                   `json:"pagesFormat,omitempty"`
	Pages       map[string]cachedPage `json:"pages,omitempty"`
}

// pagesFormat is the shape of the pages a manifest keeps. Another shape is
// read as none: the pages are read again, the manifest holds as ever.
const pagesFormat = 1

// How a folder was read.
const (
	readFromTree = "tree"
	readFromBook = "book"
	readFromWalk = "walk"
)

// manifestFile is one file the export wrote, with the bytes it wrote as
// their hash: a file whose bytes differ has been changed since by somebody
// else. A file the run was about to write has no hash yet: the manifest names
// it before it is written, so that a run that dies halfway leaves nothing
// behind it does not know, and the next run writes it over whatever it holds.
type manifestFile struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`             // index, document, asset or upload
	ID     string `json:"id"`               // the folder, document or upload; an asset's document
	BlobID string `json:"blobId,omitempty"` // the file of Minka an asset or an upload holds
	SHA256 string `json:"sha256"`
}

// planned is one file this run writes: its bytes, or the file of Minka they
// are downloaded from.
type planned struct {
	path    string
	kind    string
	id      string
	content []byte
	blobID  string
}

// isLocal says whether a path of the export stays inside its directory: a
// clean relative path with forward slashes that names neither the manifest
// nor anything above the directory.
func isLocal(p string) bool {
	return p != "" && p != ManifestName && !strings.Contains(p, `\`) && path.Clean(p) == p && filepath.IsLocal(filepath.FromSlash(p))
}

// readManifest reads the manifest of a directory, nil where there is none. A
// manifest that names a path outside the directory is refused whole: it was
// not written by an export, and nothing it names is touched.
func readManifest(dir string) (*manifest, error) {
	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, &WriteError{Err: err}
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil || m.Version != manifestVersion {
		return nil, &RefusalError{
			Message: fmt.Sprintf("%s in %s is no manifest this minka can read", ManifestName, dir),
			Hint:    "Export into another directory, or take the file away if the directory holds nothing of yours.",
		}
	}
	for _, f := range m.Files {
		if !isLocal(f.Path) {
			return nil, unsafeManifest(dir, f.Path)
		}
	}
	for _, d := range m.Directories {
		if !isLocal(d) {
			return nil, unsafeManifest(dir, d)
		}
	}
	return &m, nil
}

func unsafeManifest(dir, p string) *RefusalError {
	return &RefusalError{
		Message: fmt.Sprintf("the manifest in %s names %q, which lies outside it", dir, p),
		Hint:    "An export writes no such path; the manifest was changed by hand. Nothing was written or removed.",
		Paths:   []string{p},
	}
}

// writer carries out a plan inside one directory, and only what the old
// manifest names is its own besides what it writes now.
type writer struct {
	out    string
	force  bool
	old    map[string]manifestFile
	oldDir map[string]bool

	// cleared are directories of the export that the plan writes a file in
	// place of, holding nothing but files it removes, each with what the
	// check found in it.
	cleared []clearing

	mu      sync.Mutex
	current map[string]manifestFile
	hashes  map[string]string // of the files on disk the check read
	written []string
	same    int
	removed []string
}

func newWriter(out string, old *manifest, force bool) *writer {
	w := &writer{out: out, force: force, old: map[string]manifestFile{}, oldDir: map[string]bool{}, current: map[string]manifestFile{}, hashes: map[string]string{}}
	if old != nil {
		for _, f := range old.Files {
			w.old[f.Path] = f
			w.current[f.Path] = f
		}
		for _, d := range old.Directories {
			w.oldDir[d] = true
		}
	}
	return w
}

func (w *writer) onDisk(p string) string {
	return filepath.Join(w.out, filepath.FromSlash(p))
}

// check looks at every path the plan writes and every file it removes before
// anything is changed: a path in the way that the export did not write is
// refused, and so is one of its own files that was changed since, unless
// forced. Nothing is written or removed where anything is refused.
func (w *writer) check(plan []planned, dirs []string) error {
	// unsure are files the manifest names without a hash: a run that ended
	// may have written them, or a person may have put them there since.
	var foreign, changed, unsure []string
	for _, d := range dirs {
		if !w.isDirectoryOrMissing(d) {
			foreign = append(foreign, d)
		}
	}
	for _, p := range plan {
		if !isLocal(p.path) {
			return &RefusalError{Message: fmt.Sprintf("the export would write %q, which lies outside %s", p.path, w.out), Paths: []string{p.path}}
		}
		if !w.ancestorsAreDirectories(p.path) {
			continue // the directory above is refused already
		}
		info, err := os.Lstat(w.onDisk(p.path))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return &WriteError{Err: err}
		}
		if info.IsDir() {
			if going, holds := w.holdsOnlyWhatGoes(p.path, plan); holds {
				w.cleared = append(w.cleared, going)
				continue
			}
		}
		ours, known := w.old[p.path]
		if !info.Mode().IsRegular() || !known {
			foreign = append(foreign, p.path)
			continue
		}
		hash, err := hashFile(w.onDisk(p.path))
		if err != nil {
			return &WriteError{Err: err}
		}
		w.hashes[p.path] = hash
		// What the plan writes there already is no change of anybody's: a
		// run that died right after writing it left it so.
		alreadyWritten := p.blobID == "" && hash == hashOf(p.content)
		switch {
		case alreadyWritten:
		case ours.SHA256 == "":
			if !w.force {
				unsure = append(unsure, p.path)
			}
		case hash != ours.SHA256 && !w.force:
			changed = append(changed, p.path)
		}
	}
	for _, p := range w.removals(plan) {
		hash, regular, err := w.ownFile(p)
		if err != nil {
			return err
		}
		switch {
		case !regular || w.force:
		case w.old[p].SHA256 == "":
			unsure = append(unsure, p)
		case hash != w.old[p].SHA256:
			changed = append(changed, p)
		}
	}

	switch {
	case len(foreign) > 0:
		return &RefusalError{
			Message: fmt.Sprintf("%s in %s %s in the way: the export did not write %s and leaves %s alone", plural(len(foreign), "path"), w.out, isOrAre(len(foreign)), itOrThem(len(foreign)), itOrThem(len(foreign))),
			Hint:    "Move them away, or export into another directory.",
			Paths:   foreign,
		}
	case len(unsure) > 0:
		paths := append(changed, unsure...)
		sort.Strings(paths)
		return &RefusalError{
			Message: fmt.Sprintf("%s of the export in %s may have been changed since it was written: a run ended before it could note what it wrote", plural(len(paths), "file"), w.out),
			Hint:    "Keep what is yours elsewhere and run again with --force, which writes over them and removes what Minka no longer holds.",
			Paths:   paths,
		}
	case len(changed) > 0:
		return &RefusalError{
			Message: fmt.Sprintf("%s the export wrote in %s %s been changed since", plural(len(changed), "file"), w.out, hasOrHave(len(changed))),
			Hint:    "Keep the changes elsewhere and run again, or give --force to write over them and remove what Minka no longer holds.",
			Paths:   changed,
		}
	}
	return nil
}

// clearing is a directory of the export that a file of the plan takes the
// place of: the files and directories in it, as the check found them.
type clearing struct {
	files, dirs []string
}

// holdsOnlyWhatGoes says whether a directory in the place of a file of the
// plan is one the export made and holds nothing but files it removes, as a
// chapter does whose place an upload of the same name takes; with what it
// holds.
func (w *writer) holdsOnlyWhatGoes(d string, plan []planned) (clearing, bool) {
	var found clearing
	if !w.oldDir[d] || !w.ancestorsAreDirectories(d) {
		return found, false
	}
	// A file named without a hash is never removed unasked, so it keeps the
	// directory where it is.
	going := map[string]bool{}
	for _, p := range w.removals(plan) {
		going[p] = w.old[p].SHA256 != ""
	}
	only := true
	_ = filepath.WalkDir(w.onDisk(d), func(name string, entry fs.DirEntry, err error) error {
		relative, _ := filepath.Rel(w.out, name)
		relative = filepath.ToSlash(relative)
		// A link is neither a directory nor a regular file, so it is never
		// the export's.
		switch {
		case err != nil:
			only = false
		case entry.IsDir():
			only = only && w.oldDir[relative]
			found.dirs = append(found.dirs, relative)
		default:
			only = only && going[relative] && entry.Type().IsRegular()
			found.files = append(found.files, relative)
		}
		if !only {
			return fs.SkipAll
		}
		return nil
	})
	return found, only
}

// clear removes the directories of the export a file of the plan takes the
// place of, and what the check found in them, before anything is written.
func (w *writer) clear() error {
	for _, c := range w.cleared {
		files, dirs := c.files, slices.Clone(c.dirs)
		for _, f := range files {
			if err := os.Remove(w.onDisk(f)); err != nil {
				return &WriteError{Err: err}
			}
			delete(w.current, f)
			w.removed = append(w.removed, f)
		}
		slices.Reverse(dirs)
		for _, sub := range dirs {
			if err := os.Remove(w.onDisk(sub)); err != nil {
				return &WriteError{Err: err}
			}
			delete(w.oldDir, sub)
		}
	}
	return nil
}

// removeStrayTemporaryFiles takes away what a run that was killed left while
// it wrote, in the directories of an export that has a manifest.
func (w *writer) removeStrayTemporaryFiles(dirs []string) {
	places := append([]string{""}, dirs...)
	for d := range w.oldDir {
		places = append(places, d)
	}
	for _, d := range places {
		if d != "" && (!w.ancestorsAreDirectories(d) || !w.isDirectoryOrMissing(d)) {
			continue
		}
		entries, err := os.ReadDir(w.onDisk(d))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), tempPrefix) && entry.Type().IsRegular() {
				_ = os.Remove(filepath.Join(w.onDisk(d), entry.Name()))
			}
		}
	}
}

// markPending names every new file of the plan in the manifest before it is
// written, without a hash. A file the manifest knows keeps the hash it was
// written with, so that a person's change to it is still seen after a run
// that died.
func (w *writer) markPending(plan []planned) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range plan {
		if _, known := w.current[p.path]; !known {
			w.current[p.path] = manifestFile{Path: p.path, Kind: p.kind, ID: p.id, BlobID: p.blobID}
		}
	}
}

// isDirectoryOrMissing says whether a directory of the plan can be made or
// written into: a link is never followed, so nothing is written outside.
func (w *writer) isDirectoryOrMissing(d string) bool {
	info, err := os.Lstat(w.onDisk(d))
	return errors.Is(err, fs.ErrNotExist) || (err == nil && info.IsDir())
}

func (w *writer) ancestorsAreDirectories(p string) bool {
	for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
		if !w.isDirectoryOrMissing(dir) {
			return false
		}
	}
	return true
}

// ownFile reads a file the old manifest names: its hash, and whether it is
// still a file the export may remove. One behind a link, or no longer a
// file, is no longer the export's.
func (w *writer) ownFile(p string) (hash string, regular bool, err error) {
	if !w.ancestorsAreDirectories(p) {
		return "", false, nil
	}
	info, err := os.Lstat(w.onDisk(p))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, &WriteError{Err: err}
	}
	if !info.Mode().IsRegular() {
		return "", false, nil
	}
	hash, err = hashFile(w.onDisk(p))
	if err != nil {
		return "", false, &WriteError{Err: err}
	}
	w.hashes[p] = hash
	return hash, true, nil
}

// removals are the files of the old manifest the plan no longer writes.
func (w *writer) removals(plan []planned) []string {
	kept := map[string]bool{}
	for _, p := range plan {
		kept[p.path] = true
	}
	var gone []string
	for p := range w.old {
		if !kept[p] {
			gone = append(gone, p)
		}
	}
	sort.Strings(gone)
	return gone
}

func (w *writer) makeDirectories(dirs []string) error {
	for _, d := range dirs {
		if err := os.MkdirAll(w.onDisk(d), 0o777); err != nil {
			return &WriteError{Err: err}
		}
	}
	return nil
}

// writeContent writes bytes the run made where they differ from what lies
// there.
func (w *writer) writeContent(p planned) error {
	hash := hashOf(p.content)
	w.mu.Lock()
	unchanged := w.hashes[p.path] == hash
	w.mu.Unlock()
	if !unchanged {
		if err := writeAtomically(w.onDisk(p.path), func(f io.Writer) error {
			_, err := f.Write(p.content)
			return err
		}); err != nil {
			return &WriteError{Err: err}
		}
	}
	w.note(p, hash, unchanged)
	return nil
}

// download fetches a file of Minka unless the same file lies there as the
// export wrote it.
func (w *writer) download(ctx context.Context, source Source, p planned) error {
	w.mu.Lock()
	before, known := w.old[p.path]
	unchanged := known && before.BlobID == p.blobID && before.SHA256 != "" && w.hashes[p.path] == before.SHA256
	w.mu.Unlock()
	if unchanged {
		w.note(p, before.SHA256, true)
		return nil
	}

	bytes, err := source.FileBytes(ctx, p.blobID, "original")
	if err != nil {
		return err
	}
	defer bytes.Body.Close()
	digest := sha256.New()
	if err := writeAtomically(w.onDisk(p.path), func(f io.Writer) error {
		_, err := io.Copy(io.MultiWriter(f, digest), readerOf{bytes.Body})
		return err
	}); err != nil {
		// The bytes that did not come are the network's, not the disk's.
		var failed *readFailure
		if errors.As(err, &failed) {
			return &api.NetworkError{Method: http.MethodGet, Err: failed.err}
		}
		return &WriteError{Err: err}
	}
	w.note(p, hex.EncodeToString(digest.Sum(nil)), false)
	return nil
}

// readFailure is a download that broke off, told apart from a failure to
// write what came.
type readFailure struct{ err error }

func (e *readFailure) Error() string { return e.err.Error() }

type readerOf struct{ io.Reader }

func (r readerOf) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && err != io.EOF {
		err = &readFailure{err}
	}
	return n, err
}

func (w *writer) note(p planned, hash string, unchanged bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.current[p.path] = manifestFile{Path: p.path, Kind: p.kind, ID: p.id, BlobID: p.blobID, SHA256: hash}
	if unchanged {
		w.same++
	} else {
		w.written = append(w.written, p.path)
	}
}

// removeStale removes the files of the old manifest the plan no longer
// writes, and then the directories of the export that are left empty. One
// changed since was refused by the check unless forced; one behind a link or
// no longer a file is given up and left where it is.
func (w *writer) removeStale(plan []planned, dirs []string) error {
	for _, p := range w.removals(plan) {
		_, regular, err := w.ownFile(p)
		if err != nil {
			return err
		}
		delete(w.current, p)
		if !regular {
			continue
		}
		if err := os.Remove(w.onDisk(p)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return &WriteError{Err: err}
		}
		w.removed = append(w.removed, p)
	}

	planned := map[string]bool{}
	for _, d := range dirs {
		planned[d] = true
	}
	var stale []string
	for d := range w.oldDir {
		if !planned[d] {
			stale = append(stale, d)
		}
	}
	// The deepest first, so that a directory emptied by another goes too.
	sort.Slice(stale, func(i, j int) bool { return strings.Count(stale[i], "/") > strings.Count(stale[j], "/") })
	for _, d := range stale {
		if !w.ancestorsAreDirectories(d) || !w.isDirectoryOrMissing(d) {
			delete(w.oldDir, d)
			continue
		}
		// Only an empty one goes: what somebody else put there stays.
		if err := os.Remove(w.onDisk(d)); err == nil || errors.Is(err, fs.ErrNotExist) {
			delete(w.oldDir, d)
		}
	}
	return nil
}

// saveManifest writes down what the export holds now: every file it wrote and
// still holds, and its directories, an earlier run's left that are not empty
// yet included.
func (w *writer) saveManifest(m manifest, dirs []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	m.Version = manifestVersion
	m.Files = make([]manifestFile, 0, len(w.current))
	for _, f := range w.current {
		m.Files = append(m.Files, f)
	}
	slices.SortFunc(m.Files, func(a, b manifestFile) int { return strings.Compare(a.Path, b.Path) })
	all := map[string]bool{}
	for _, d := range dirs {
		all[d] = true
	}
	for d := range w.oldDir {
		all[d] = true
	}
	m.Directories = make([]string, 0, len(all))
	for d := range all {
		m.Directories = append(m.Directories, d)
	}
	sort.Strings(m.Directories)

	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return &WriteError{Err: err}
	}
	if err := writeAtomically(filepath.Join(w.out, ManifestName), func(f io.Writer) error {
		_, err := f.Write(append(raw, '\n'))
		return err
	}); err != nil {
		return &WriteError{Err: err}
	}
	return nil
}

// writeAtomically writes a file next to its place and renames it there, so
// an interrupted run leaves no half file, and a link in its place is
// replaced rather than followed. A new file gets the mode any program gives
// one, with the umask taking its part; a file written over keeps its own.
func writeAtomically(target string, write func(io.Writer) error) error {
	mode, keep := os.FileMode(0o666), false
	if info, err := os.Lstat(target); err == nil && info.Mode().IsRegular() {
		mode, keep = info.Mode().Perm(), true
	}
	temp, err := createTemporary(filepath.Dir(target), mode)
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := write(temp); err != nil {
		temp.Close()
		return err
	}
	if keep {
		if err := temp.Chmod(mode); err != nil {
			temp.Close()
			return err
		}
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), target)
}

// createTemporary makes a new file of a name nobody holds, with the mode
// given and the umask applied, unlike os.CreateTemp, which makes it 0600.
func createTemporary(dir string, mode os.FileMode) (*os.File, error) {
	for {
		random := make([]byte, 8)
		if _, err := rand.Read(random); err != nil {
			return nil, err
		}
		file, err := os.OpenFile(filepath.Join(dir, tempPrefix+hex.EncodeToString(random)), os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if !errors.Is(err, fs.ErrExist) {
			return file, err
		}
	}
}

func hashFile(name string) (string, error) {
	f, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer f.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func hashOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func isOrAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func hasOrHave(n int) string {
	if n == 1 {
		return "has"
	}
	return "have"
}

func itOrThem(n int) string {
	if n == 1 {
		return "it"
	}
	return "them"
}
