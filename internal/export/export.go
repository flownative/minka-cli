// Package export writes a folder of Docs & Files, a book above all, into a
// directory: one directory per folder, one file per document and per upload,
// the files hanging in a document beside it, and an index per directory in
// Minka's order. Links between the documents point at the files, and the
// headings carry the anchors Minka's page gives them.
//
// A run into the same directory brings it up to date. A manifest says what an
// export wrote, and nothing it does not name is ever written over or removed.
package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/localfile"
)

// Options say what is exported where.
type Options struct {
	FolderID string
	ToolID   string // walks a folder in no book; without it such a folder is ErrNoBook
	Out      string
	Format   string   // FormatMarkdown or FormatHTML
	Host     *url.URL // the host signed in to, whose addresses name entries of Minka
	// Force exports into a directory that holds something without a manifest,
	// and writes over or removes a file of the export that was changed since.
	// A file the export did not write is never touched.
	Force bool
	// Parallel is how many documents are read, and files downloaded, at a time.
	Parallel int
	// LockDir holds the locks that keep two runs out of one directory, one
	// per directory, outside it: minka's own place.
	LockDir string
	Now     func() time.Time
}

// Report says what a run did.
type Report struct {
	Out       string               `json:"out"`
	FolderID  string               `json:"folderId"`
	Title     string               `json:"title"`
	IsBook    bool                 `json:"isBook"`
	Book      *api.FolderReference `json:"book"`
	Format    string               `json:"format"`
	Folders   int                  `json:"folders"`   // the root included
	Documents int                  `json:"documents"` // written or unchanged
	Uploads   int                  `json:"uploads"`
	Assets    int                  `json:"assets"`    // the files hanging in documents
	Written   []string             `json:"written"`   // new or changed, relative to Out
	Unchanged int                  `json:"unchanged"` // left as they were
	Removed   []string             `json:"removed"`   // written by an earlier run, gone from Minka since
	Gone      []GoneDocument       `json:"gone"`      // in the tree, but at no address any more when read
	Manifest  string               `json:"manifest"`
}

// GoneDocument is a document the tree named that was gone when its body was
// read, as one put away meanwhile is.
type GoneDocument struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// RefusalError says why a run changed nothing in the directory.
type RefusalError struct {
	Message string
	Hint    string
	Paths   []string
}

func (e *RefusalError) Error() string { return e.Message }

// WriteError is a failure of the file system, not of Minka.
type WriteError struct{ Err error }

func (e *WriteError) Error() string { return e.Err.Error() }
func (e *WriteError) Unwrap() error { return e.Err }

// Run exports the folder. Everything is read and checked against the
// directory before the first file is written; what was written before a
// failure is in the manifest all the same.
func Run(ctx context.Context, source Source, options Options) (*Report, error) {
	if options.Format == "" {
		options.Format = FormatMarkdown
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	out, created, err := openDirectory(options.Out)
	if err != nil {
		return nil, err
	}
	lockPath, err := LockPath(options.LockDir, out)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(lockPath), 0o700)
	}
	var lock *localfile.Lock
	if err == nil {
		lock, err = localfile.AcquireWithin(lockPath, 0)
	}
	if err != nil {
		if created {
			_ = os.Remove(out)
		}
		if errors.Is(err, localfile.ErrBusy) {
			return nil, &RefusalError{Message: "another export is writing into " + out, Hint: "Wait for it to end, then run again."}
		}
		return nil, &WriteError{Err: err}
	}
	// A run that wrote nothing leaves no directory it made.
	wrote := false
	defer func() {
		lock.Release()
		if !wrote && created {
			_ = os.Remove(out)
		}
	}()

	earlier, err := prepare(out, options)
	if err != nil {
		return nil, err
	}

	t, err := loadTree(ctx, source, options.FolderID, options.ToolID)
	if err != nil {
		return nil, err
	}
	gone, err := readDocuments(ctx, source, t.root, options.Parallel)
	if err != nil {
		return nil, err
	}
	nameTree(t.root, earlierNames(earlier))

	l := newLayout(options.Format, options.Host, t.root)
	plan, dirs, report := l.plan(t)
	report.Out, report.Format, report.Manifest = out, options.Format, ManifestName
	report.Gone = []GoneDocument{}
	for _, n := range gone {
		report.Gone = append(report.Gone, GoneDocument{ID: n.id, Title: n.title})
	}

	w := newWriter(out, earlier, options.Force)
	if err := w.check(plan, dirs); err != nil {
		return nil, err
	}
	m := manifest{Host: hostName(options.Host), FolderID: options.FolderID, ToolID: t.toolID, Format: options.Format, ExportedAt: options.Now().UTC().Format(time.RFC3339)}

	// The manifest names every file before the first is written, so that a
	// run that dies halfway leaves nothing the next one would take as foreign.
	wrote = true
	w.markPending(plan)
	if err := w.saveManifest(m, dirs); err != nil {
		return nil, err
	}
	if earlier != nil {
		w.removeStrayTemporaryFiles(dirs)
	}
	failed := w.clear()
	if failed == nil {
		failed = w.carryOut(ctx, source, plan, dirs, max(1, options.Parallel))
	}
	if failed == nil {
		failed = w.removeStale(plan, dirs)
	}
	if err := w.saveManifest(m, dirs); err != nil && failed == nil {
		failed = err
	}
	if failed != nil {
		return nil, failed
	}

	sort.Strings(w.written)
	report.Written, report.Unchanged, report.Removed = append([]string{}, w.written...), w.same, append([]string{}, w.removed...)
	return report, nil
}

// LockPath is the file a run into a directory holds locked: named after the
// directory as it really is, links resolved, so that two names of one
// directory share one lock. It lies outside the directory, which keeps
// nothing of a run that wrote nothing in it, and it is never taken away,
// which would let two runs hold a lock each.
func LockPath(lockDir, out string) (string, error) {
	if lockDir == "" {
		return "", errors.New("no place for the lock of an export")
	}
	real, err := filepath.EvalSymlinks(out)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(real))
	return filepath.Join(lockDir, hex.EncodeToString(sum[:16])+".lock"), nil
}

// openDirectory finds the directory, made where it is not there yet, and
// says whether this run made it.
func openDirectory(name string) (string, bool, error) {
	out, err := filepath.Abs(name)
	if err != nil {
		return "", false, &WriteError{Err: err}
	}
	info, err := os.Stat(out)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(out, 0o777); err != nil {
			return "", false, &WriteError{Err: err}
		}
		return out, true, nil
	case err != nil:
		return "", false, &WriteError{Err: err}
	case !info.IsDir():
		return "", false, &RefusalError{Message: out + " is no directory", Hint: "Name a directory, or one that does not exist yet."}
	}
	return out, false, nil
}

// prepare reads the manifest of the directory, and refuses one that holds the
// export of another folder or of another host, or that holds something
// without a manifest unless forced.
func prepare(out string, options Options) (*manifest, error) {
	earlier, err := readManifest(out)
	if err != nil {
		return nil, err
	}
	if earlier == nil {
		entries, err := os.ReadDir(out)
		if err != nil {
			return nil, &WriteError{Err: err}
		}
		if len(entries) > 0 && !options.Force {
			return nil, &RefusalError{
				Message: fmt.Sprintf("%s holds files and no export", out),
				Hint:    "Export into an empty or new directory, or give --force to export beside what is there; a file the export did not write is never written over.",
			}
		}
		return nil, nil
	}
	if earlier.FolderID != options.FolderID {
		return nil, &RefusalError{
			Message: fmt.Sprintf("%s holds the export of the folder %s", out, earlier.FolderID),
			Hint:    "Export each folder into a directory of its own: a run removes what its folder no longer holds.",
		}
	}
	if host := hostName(options.Host); earlier.Host != "" && host != "" && earlier.Host != host {
		return nil, &RefusalError{
			Message: fmt.Sprintf("%s holds an export from %s", out, earlier.Host),
			Hint:    "Export from each host into a directory of its own.",
		}
	}
	return earlier, nil
}

func hostName(host *url.URL) string {
	if host == nil {
		return ""
	}
	return host.Scheme + "://" + host.Host
}

// earlierNames are the names an earlier run gave the entries, read from the
// paths its manifest holds.
func earlierNames(m *manifest) map[string]earlierName {
	names := map[string]earlierName{}
	if m == nil {
		return names
	}
	parent := func(p string) string {
		if dir := path.Dir(p); dir != "." {
			return dir
		}
		return ""
	}
	for _, f := range m.Files {
		switch f.Kind {
		case fileDocument, fileUpload:
			base := path.Base(f.Path)
			stem, _, _ := strings.Cut(base, ".")
			names[f.Kind+"/"+f.ID] = earlierName{dir: parent(f.Path), stem: stem}
		case fileIndex:
			if dir := parent(f.Path); dir != "" {
				names[kindFolder+"/"+f.ID] = earlierName{dir: parent(dir), stem: path.Base(dir)}
			}
		}
	}
	return names
}

// plan lays out every file of the export and the directories they stand in.
func (l *layout) plan(t *tree) ([]planned, []string, *Report) {
	report := &Report{FolderID: t.root.id, Title: t.root.title, IsBook: t.isBook, Book: t.book}
	var plan []planned
	dirSet := map[string]bool{}
	addDir := func(d string) {
		for ; d != "" && d != "."; d = path.Dir(d) {
			dirSet[d] = true
		}
	}
	var book *bookOfIndex
	if t.book != nil {
		book = &bookOfIndex{id: t.book.ID, title: t.book.Title}
	}

	t.root.each(func(n *node) {
		p := l.pathOf(n)
		addDir(path.Dir(p))
		switch n.kind {
		case kindFolder:
			report.Folders++
			kind := "folder"
			switch {
			case t.isBook && t.book.ID == n.id:
				kind = "book"
			case t.isBook:
				kind = "chapter"
			}
			plan = append(plan, planned{path: p, kind: fileIndex, id: n.id, content: l.renderIndex(n, kind, book)})
		case kindDocument:
			report.Documents++
			page := l.renderDocument(n)
			plan = append(plan, planned{path: p, kind: fileDocument, id: n.id, content: page.content})
			for _, a := range page.assets {
				report.Assets++
				addDir(path.Dir(a.path))
				plan = append(plan, planned{path: a.path, kind: fileAsset, id: n.id, blobID: a.blobID})
			}
		case kindUpload:
			report.Uploads++
			plan = append(plan, planned{path: p, kind: fileUpload, id: n.id, blobID: n.file.ID})
		}
	})

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	// Those above first, so that each is made inside its parent.
	sort.Slice(dirs, func(i, j int) bool {
		return len(dirs[i]) < len(dirs[j]) || (len(dirs[i]) == len(dirs[j]) && dirs[i] < dirs[j])
	})
	return plan, dirs, report
}

// carryOut makes the directories, writes what the run made and downloads the
// files, at most `parallel` at a time.
func (w *writer) carryOut(ctx context.Context, source Source, plan []planned, dirs []string, parallel int) error {
	if err := w.makeDirectories(dirs); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu       sync.Mutex
		wg       sync.WaitGroup
		firstErr error
		slots    = make(chan struct{}, parallel)
	)
	fail := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
	}
	for _, p := range plan {
		if ctx.Err() != nil {
			break
		}
		if p.blobID == "" {
			if err := w.writeContent(p); err != nil {
				fail(err)
				break
			}
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			if err := w.download(ctx, source, p); err != nil {
				fail(err)
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}
