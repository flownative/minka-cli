package export

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/flownative/minka-cli/internal/api"
)

// Source is what an export reads from Minka: the tree of a book in one
// answer, the entries of a folder one folder deep, a document with its body,
// and the bytes of a file.
type Source interface {
	BookOf(ctx context.Context, folderID string) (api.Result[api.Book], error)
	FilesEntriesOf(ctx context.Context, toolID, folderID string) (api.Result[api.FilesEntries], error)
	Document(ctx context.Context, documentID string) (api.Result[api.Document], error)
	FileBytes(ctx context.Context, blobID, variant string) (*api.FileBytes, error)
}

// ErrNoBook says the folder stands in no book and no tool was named to walk
// it through. The answer of Minka is wrapped in it.
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

	stem     string        // the name within the directory of the parent, without extension
	document *api.Document // the document as read, nil until then and for one gone since
}

// tree is what is exported: the folder named and everything below it.
type tree struct {
	root   *node
	isBook bool
	book   *api.FolderReference // the book the root stands in, nil outside one
	toolID string
}

// loadTree reads the folder as a book, in one answer, or walks it through its
// tool where it stands in no book. A book's answer carries no uploads, so a
// book is exported as its readers read it: chapters and pages.
func loadTree(ctx context.Context, source Source, folderID, toolID string) (*tree, error) {
	book, err := source.BookOf(ctx, folderID)
	if err == nil {
		return treeOfBook(book.Value, folderID)
	}
	var apiErr *api.Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		return nil, err
	}
	if toolID == "" {
		return nil, errors.Join(ErrNoBook, err)
	}
	root, err := walk(ctx, source, toolID, folderID)
	if err != nil {
		return nil, err
	}
	return &tree{root: root, toolID: toolID}, nil
}

func treeOfBook(book api.Book, folderID string) (*tree, error) {
	root := &node{kind: kindFolder, id: book.ID, title: book.Title, appURL: book.AppURL, order: book.Order}
	root.children = nodesOfBook(book.Entries, root)
	found := root.find(kindFolder, folderID)
	if found == nil {
		return nil, ErrNotInTheBook
	}
	found.parent = nil
	return &tree{root: found, isBook: true, book: &api.FolderReference{ID: book.ID, Title: book.Title}, toolID: book.Tool.ID}, nil
}

func nodesOfBook(entries []api.BookEntry, parent *node) []*node {
	nodes := make([]*node, 0, len(entries))
	for _, entry := range entries {
		n := &node{id: entry.ID, title: entry.Title, appURL: entry.AppURL, parent: parent}
		switch entry.Kind {
		case "chapter":
			n.kind, n.order = kindFolder, entry.Order
			n.children = nodesOfBook(entry.Entries, n)
		case "page":
			n.kind, n.inNavigation = kindDocument, entry.InNavigation
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

// readDocuments reads the body of every document, at most `parallel` at a
// time. A document gone since the tree was read is left out and named.
func readDocuments(ctx context.Context, source Source, root *node, parallel int) (gone []*node, err error) {
	var documents []*node
	root.each(func(n *node) {
		if n.kind == kindDocument {
			documents = append(documents, n)
		}
	})

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
			var apiErr *api.Error
			switch {
			case err == nil:
				document := read.Value
				n.document = &document
			case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
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

	for _, n := range documents {
		if vanishing[n] {
			gone = append(gone, n)
			n.parent.children = removed(n.parent.children, n)
		}
	}
	return gone, nil
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
