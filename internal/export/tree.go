package export

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/flownative/minka-cli/internal/api"
)

// Source is what an export reads from Minka: a folder, the tree below it in
// one answer, a document with its body and the bytes of a file; and from a
// Minka without the tree of a folder, the tree of a book and the entries of a
// folder one folder deep.
type Source interface {
	FilesFolder(ctx context.Context, folderID string) (api.Result[api.FilesEntries], error)
	TreeOf(ctx context.Context, folderID string) (api.Result[api.Book], error)
	BookOf(ctx context.Context, folderID string) (api.Result[api.Book], error)
	FilesEntriesOf(ctx context.Context, toolID, folderID string) (api.Result[api.FilesEntries], error)
	Document(ctx context.Context, documentID string) (api.Result[api.Document], error)
	FileBytes(ctx context.Context, blobID, variant string) (*api.FileBytes, error)
}

// ErrNoBook says the folder stands in no book of a Minka that reads no tree
// of a folder, and no tool was named to walk it through. The answer of Minka
// is wrapped in it.
var ErrNoBook = errors.New("the folder stands in no book")

// ErrNotInTheBook says Minka answered with the book, but the folder is none of
// the chapters in use, as an archived chapter is.
var ErrNotInTheBook = errors.New("the folder is no chapter of its book in use")

const (
	kindFolder   = "folder"
	kindDocument = "document"
	kindUpload   = "upload"
)

// node is one entry of the tree being exported, in Minka's order.
type node struct {
	kind         string
	id           string
	title        string
	appURL       string
	order        string // a folder's: "automatic" or "manual", empty where Minka does not say
	inNavigation bool   // a document's
	file         api.Attachment
	children     []*node
	parent       *node

	// A document's newest version and when it changed, as the tree names
	// them: what tells a page read before from one changed since. Without a
	// version the page is read again.
	updatedAt string
	version   *int

	stem      string        // the name within the directory of the parent, without extension
	document  *api.Document // the document as read, nil until then and for one gone since
	fromCache bool          // the document is the one an earlier run read
}

// tree is what is exported: the folder named and everything below it.
type tree struct {
	root   *node
	isBook bool
	book   *api.FolderReference // the book the root stands in, nil outside one
	toolID string
	// readFrom says how the folder was read: from its tree, as a book or
	// walked through its tool.
	readFrom string
}

// loadTree reads the folder and the tree below it, two requests however deep
// it goes: the folder says whether it stands in a book, the tree holds its
// chapters, pages and uploads. A Minka without the tree of a folder answers
// the folder with a page of its web; there the folder is read as a book, or
// walked through its tool where it stands in no book. A book's answer carries
// no uploads, so such a book is exported as its readers read it.
//
// An export read from a tree before keeps its tree: where keepTheTree is set,
// such an answer refuses the run instead.
func loadTree(ctx context.Context, source Source, folderID, toolID string, keepTheTree bool) (*tree, error) {
	folder, err := source.FilesFolder(ctx, folderID)
	if err == nil {
		read, err := source.TreeOf(ctx, folderID)
		if err != nil {
			return nil, err
		}
		return treeOfFolder(folder.Value, read.Value), nil
	}
	if !answeredByAnOlderMinka(err) {
		return nil, err
	}
	if keepTheTree {
		return nil, &RefusalError{
			Message: "the host did not answer as a Minka that reads the tree of a folder, though this export was read from one",
			Hint:    "Something in front of Minka may have answered: try again. Where the host really is an older Minka now, --force exports the way it allows, a book without its uploads.",
		}
	}

	book, err := source.BookOf(ctx, folderID)
	if err == nil {
		return treeOfBook(book.Value, folderID)
	}
	if !isNotFound(err) {
		return nil, err
	}
	if toolID == "" {
		return nil, errors.Join(ErrNoBook, err)
	}
	root, err := walk(ctx, source, toolID, folderID)
	if err != nil {
		return nil, err
	}
	return &tree{root: root, toolID: toolID, readFrom: readFromWalk}, nil
}

// answeredByAnOlderMinka says whether a 404 came without Minka's error body:
// a Minka that does not know the address answers with a page of its web.
func answeredByAnOlderMinka(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound && apiErr.Body == nil
}

func isNotFound(err error) bool {
	var apiErr *api.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func treeOfFolder(folder api.FilesEntries, read api.Book) *tree {
	root := &node{kind: kindFolder, id: read.ID, title: read.Title, appURL: read.AppURL, order: read.Order}
	root.children = nodesOfBook(read.Entries, root)
	t := &tree{root: root, toolID: read.Tool.ID, readFrom: readFromTree}
	switch {
	case folder.Book != nil:
		t.isBook, t.book = true, folder.Book
	case folder.IsBook:
		t.isBook, t.book = true, &api.FolderReference{ID: read.ID, Title: read.Title}
	}
	return t
}

func treeOfBook(book api.Book, folderID string) (*tree, error) {
	root := &node{kind: kindFolder, id: book.ID, title: book.Title, appURL: book.AppURL, order: book.Order}
	root.children = nodesOfBook(book.Entries, root)
	found := root.find(kindFolder, folderID)
	if found == nil {
		return nil, ErrNotInTheBook
	}
	found.parent = nil
	return &tree{root: found, isBook: true, book: &api.FolderReference{ID: book.ID, Title: book.Title}, toolID: book.Tool.ID, readFrom: readFromBook}, nil
}

func nodesOfBook(entries []api.BookEntry, parent *node) []*node {
	nodes := make([]*node, 0, len(entries))
	for _, entry := range entries {
		n := &node{id: entry.ID, title: entry.Title, appURL: entry.AppURL, parent: parent, updatedAt: entry.UpdatedAt}
		switch {
		case entry.Kind == "chapter":
			n.kind, n.order = kindFolder, entry.Order
			n.children = nodesOfBook(entry.Entries, n)
		case entry.Kind == "page":
			n.kind, n.inNavigation, n.version = kindDocument, entry.InNavigation, entry.Version
		case entry.Kind == "upload" && entry.File != nil:
			n.kind, n.file = kindUpload, *entry.File
		default:
			// A kind of entry a later Minka knows and this export does not.
			continue
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// walk reads a folder and every folder below it, one request each, and keeps
// the order Minka shows: the mixed one of a place arranged by hand, else
// folders, documents and uploads.
func walk(ctx context.Context, source Source, toolID, folderID string) (*node, error) {
	entries, err := source.FilesEntriesOf(ctx, toolID, folderID)
	if err != nil {
		return nil, err
	}
	value := entries.Value
	root := &node{kind: kindFolder, id: folderID, appURL: value.AppURL, order: value.Order}
	if value.Folder != nil {
		root.title = value.Folder.Title
	}
	if err := walkInto(ctx, source, toolID, root, value); err != nil {
		return nil, err
	}
	return root, nil
}

func walkInto(ctx context.Context, source Source, toolID string, parent *node, value api.FilesEntries) error {
	byID := map[string]*node{}
	var shown []*node
	add := func(n *node) {
		n.parent = parent
		byID[n.kind+"/"+n.id] = n
		shown = append(shown, n)
	}
	for _, folder := range value.Folders {
		add(&node{kind: kindFolder, id: folder.ID, title: folder.Title, appURL: folder.AppURL})
	}
	for _, document := range value.Documents {
		add(&node{kind: kindDocument, id: document.ID, title: document.Title, appURL: document.AppURL, inNavigation: document.InNavigation == nil || *document.InNavigation})
	}
	for _, upload := range value.Uploads {
		add(&node{kind: kindUpload, id: upload.ID, title: upload.Title, appURL: upload.AppURL, file: upload.File})
	}

	// An entry the order does not name still stands behind the rest.
	if len(value.Entries) > 0 {
		var ordered []*node
		for _, entry := range value.Entries {
			if n := byID[entry.Kind+"/"+entry.ID]; n != nil {
				ordered = append(ordered, n)
				delete(byID, entry.Kind+"/"+entry.ID)
			}
		}
		for _, n := range shown {
			if byID[n.kind+"/"+n.id] != nil {
				ordered = append(ordered, n)
			}
		}
		shown = ordered
	}
	parent.children = shown

	for _, n := range shown {
		if n.kind != kindFolder {
			continue
		}
		entries, err := source.FilesEntriesOf(ctx, toolID, n.id)
		if err != nil {
			return err
		}
		n.order = entries.Value.Order
		if entries.Value.AppURL != "" {
			n.appURL = entries.Value.AppURL
		}
		if err := walkInto(ctx, source, toolID, n, entries.Value); err != nil {
			return err
		}
	}
	return nil
}

func (n *node) find(kind, id string) *node {
	if n.kind == kind && n.id == id {
		return n
	}
	for _, child := range n.children {
		if found := child.find(kind, id); found != nil {
			return found
		}
	}
	return nil
}

// each visits the node and everything below it, in Minka's order.
func (n *node) each(visit func(*node)) {
	visit(n)
	for _, child := range n.children {
		child.each(visit)
	}
}

// keyOf names an entry of the export as a reference or an address names it.
func keyOf(kind, id string) string {
	return kind + "/" + strings.ToLower(id)
}

// entriesBelow are the entries of the export by their key.
func entriesBelow(root *node) map[string]*node {
	entries := map[string]*node{}
	root.each(func(n *node) { entries[keyOf(n.kind, n.id)] = n })
	return entries
}

// readDocuments reads the body of every document, at most `parallel` at a
// time, and takes it from the cache where an earlier run read the same
// version. A document gone since the tree was read is left out and named;
// a page taken from the cache that refers to it is weighed again, since its
// reference no longer leads into the export.
func readDocuments(ctx context.Context, source Source, root *node, parallel int, cache map[string]cachedPage) (gone []*node, read int, err error) {
	var pending []*node
	root.each(func(n *node) {
		if n.kind == kindDocument {
			pending = append(pending, n)
		}
	})
	for len(pending) > 0 {
		inside := entriesBelow(root)
		var toRead []*node
		for _, n := range pending {
			if page, usable := cachedPageOf(cache, n, inside); usable {
				n.document, n.fromCache = page, true
				continue
			}
			n.document, n.fromCache = nil, false
			toRead = append(toRead, n)
		}
		vanished, err := fetchDocuments(ctx, source, toRead, parallel)
		read += len(toRead)
		if err != nil {
			return nil, read, err
		}
		pending = nil
		if len(vanished) == 0 {
			break
		}
		for _, n := range vanished {
			gone = append(gone, n)
			n.parent.children = removed(n.parent.children, n)
		}
		root.each(func(n *node) {
			if n.kind == kindDocument && n.fromCache {
				pending = append(pending, n)
			}
		})
	}
	return gone, read, nil
}

// fetchDocuments reads the documents, at most `parallel` at a time, and says
// which of them are at no address any more.
func fetchDocuments(ctx context.Context, source Source, documents []*node, parallel int) ([]*node, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		firstErr  error
		slots     = make(chan struct{}, max(1, parallel))
		vanishing = map[*node]bool{}
	)
	for _, n := range documents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			read, err := source.Document(ctx, n.id)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				document := read.Value
				n.document = &document
			case isNotFound(err):
				vanishing[n] = true
			case firstErr == nil:
				firstErr = err
				cancel()
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var vanished []*node
	for _, n := range documents {
		if vanishing[n] {
			vanished = append(vanished, n)
		}
	}
	return vanished, nil
}

func removed(nodes []*node, gone *node) []*node {
	kept := nodes[:0:0]
	for _, n := range nodes {
		if n != gone {
			kept = append(kept, n)
		}
	}
	return kept
}
