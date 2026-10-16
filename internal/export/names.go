package export

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/flownative/minka-cli/internal/richtext"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// maxStemBytes bounds a name well below what any file system takes, with
// room for a counter and an extension.
const maxStemBytes = 80

// untitled names what has no letter or digit in its title.
const untitled = "untitled"

// reservedStems are taken in every directory before any entry is named: the
// index of a directory in either format, and the names Windows keeps for its
// devices whatever follows them.
var reservedStems = func() map[string]bool {
	names := map[string]bool{"readme": true, "index": true, "con": true, "prn": true, "aux": true, "nul": true}
	for i := 1; i <= 9; i++ {
		names["com"+strconv.Itoa(i)] = true
		names["lpt"+strconv.Itoa(i)] = true
	}
	return names
}()

// extensionForm is what of a file's name is kept as its extension.
var extensionForm = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

var folding = cases.Fold()

// stemOf is a title as a name: the slug of a heading's anchor, which a reader
// can guess from the title and which holds only letters, digits and dashes,
// so no name is a path, starts with a dot or differs from another in case
// alone; composed again, so that a file system stores it as it is written.
func stemOf(title string) string {
	if !strings.ContainsFunc(title, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
		return untitled
	}
	stem := norm.NFC.String(richtext.Slug(title))
	if len(stem) > maxStemBytes {
		cut := maxStemBytes
		for cut > 0 && !utfStart(stem[cut]) {
			cut--
		}
		stem = strings.TrimRight(stem[:cut], "-")
	}
	if stem == "" {
		return untitled
	}
	return stem
}

func utfStart(b byte) bool {
	return b&0xC0 != 0x80
}

// extensionOf is the extension of a file's name in lower case, or none where
// it holds more than letters and digits.
func extensionOf(filename string) string {
	extension := strings.ToLower(strings.TrimPrefix(path.Ext(filename), "."))
	if !extensionForm.MatchString(extension) {
		return ""
	}
	return extension
}

// withExtension joins a stem and an extension.
func withExtension(stem, extension string) string {
	if extension == "" {
		return stem
	}
	return stem + "." + extension
}

// stemOfUpload names an upload after its title, without the extension its
// file already brings.
func stemOfUpload(title, filename string) string {
	if extension := extensionOf(filename); extension != "" {
		if strings.EqualFold(path.Ext(title), "."+extension) {
			title = strings.TrimSuffix(title, path.Ext(title))
		}
	}
	return stemOf(title)
}

// sameName is how a file system that ignores case tells two names apart.
func sameName(name string) string {
	return folding.String(norm.NFC.String(name))
}

// counted says whether a stem is the base itself or the base counted on.
func counted(stem, base string) bool {
	if stem == base {
		return true
	}
	number, found := strings.CutPrefix(stem, base+"-")
	if !found {
		return false
	}
	n, err := strconv.Atoi(number)
	return err == nil && n >= 2 && strconv.Itoa(n) == number
}

// earlierName is where an entry stood in the export before: the directory
// and the stem it had there.
type earlierName struct {
	dir, stem string
}

// nameTree gives every entry below the root its stem. Within a directory the
// documents, folders and uploads share one set of stems, so that a page, its
// directory of files and a chapter of the same title never meet. An entry
// keeps the name an earlier run gave it while that name is still its title's
// and free, so names do not move with the order; the others take the first
// free one of the title, the title, then the title and "-2", "-3" in Minka's
// order.
func nameTree(root *node, earlier map[string]earlierName) {
	nameChildren(root, "", earlier)
}

func nameChildren(parent *node, dir string, earlier map[string]earlierName) {
	taken := map[string]bool{}
	for stem := range reservedStems {
		taken[sameName(stem)] = true
	}
	base := func(n *node) string {
		if n.kind == kindUpload {
			return stemOfUpload(n.title, n.file.Filename)
		}
		return stemOf(n.title)
	}

	for _, n := range parent.children {
		before, known := earlier[n.kind+"/"+n.id]
		if known && before.dir == dir && counted(before.stem, base(n)) && !taken[sameName(before.stem)] {
			n.stem = before.stem
			taken[sameName(n.stem)] = true
		}
	}
	for _, n := range parent.children {
		if n.stem != "" {
			continue
		}
		stem := base(n)
		for count := 2; taken[sameName(stem)]; count++ {
			stem = base(n) + "-" + strconv.Itoa(count)
		}
		n.stem = stem
		taken[sameName(stem)] = true
	}

	for _, n := range parent.children {
		if n.kind == kindFolder {
			nameChildren(n, path.Join(dir, n.stem), earlier)
		}
	}
}

// dirOf is the directory a folder is exported to, relative to the root of the
// export; the root itself is "".
func (n *node) dirOf() string {
	if n.parent == nil {
		return ""
	}
	return path.Join(n.parent.dirOf(), n.stem)
}

// placeOf is the directory an entry stands in.
func (n *node) placeOf() string {
	if n.parent == nil {
		return ""
	}
	return n.parent.dirOf()
}

// relative is the link from a file in one directory to a path of the export,
// both relative to its root.
func relative(fromDir, to string) string {
	from := splitPath(fromDir)
	target := splitPath(to)
	common := 0
	for common < len(from) && common < len(target)-1 && from[common] == target[common] {
		common++
	}
	parts := make([]string, 0, len(from)-common+len(target)-common)
	for range from[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, target[common:]...)
	return strings.Join(parts, "/")
}

func splitPath(p string) []string {
	if p == "" || p == "." {
		return nil
	}
	return strings.Split(p, "/")
}
