package fakeapi

import (
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// What a search serves: a page of a fixed size, a term cut to a length, and
// an excerpt of about this many characters.
const (
	searchPageSize      = 25
	searchTermMaxLength = 200
	searchExcerptLength = 160
)

// searchOffsetForm is an offset Minka takes: at most six digits, or none.
var searchOffsetForm = regexp.MustCompile(`^\d{0,6}$`)

// headingForm finds the headings of a body in the order they stand.
var headingForm = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)

// spelledOut are the letters an anchor writes out before it drops accents.
var spelledOut = strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss")

// withoutAccents stands in for decomposing a letter and dropping its marks: a
// letter of the Latin alphabets with a canonical decomposition, and its base.
var withoutAccents = func() map[rune]rune {
	bases := map[rune]string{
		'a': "àáâãåāăąǎ", 'c': "çćĉċč", 'd': "ď", 'e': "èéêëēĕėęě", 'g': "ĝğġģ",
		'h': "ĥ", 'i': "ìíîïĩīĭįǐ", 'j': "ĵ", 'k': "ķ", 'l': "ĺļľ", 'n': "ñńņňǹ",
		'o': "òóôõōŏőǒ", 'r': "ŕŗř", 's': "śŝşš", 't': "ţť", 'u': "ùúûũūŭůűųǔ",
		'w': "ŵ", 'y': "ýÿŷ", 'z': "źżž",
	}
	table := map[rune]rune{}
	for base, accented := range bases {
		for _, letter := range accented {
			table[letter] = base
		}
	}
	return table
}()

func (s *Server) registerSearchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/search", s.authenticated(s.search))
}

// searchSection is the part of a document from one heading to the next, or
// the part before the first heading, which names no heading.
type searchSection struct {
	heading string
	anchor  string
	text    string // what the term is looked for in, the heading included
	excerpt string // what the excerpt is made of: the heading stands beside it
}

type searchHit struct {
	resource  map[string]any
	id        string
	inTitle   bool
	updatedAt string
}

// search finds the documents and uploads of a tool, a folder and what lies
// beneath it, or both, refusing in the order the controller does: the scope
// before the term, the term before the offset.
func (s *Server) search(w http.ResponseWriter, r *http.Request, person *Person) {
	query := r.URL.Query()
	toolID := strings.ToLower(query.Get("toolId"))
	folderID := strings.ToLower(query.Get("folderId"))
	if toolID == "" && folderID == "" {
		unprocessable(w, "searchScopeMissing", `A search names "toolId", "folderId" or both.`)
		return
	}
	tool, ok := s.searchScope(toolID, folderID, person)
	if !ok {
		notFound(w)
		return
	}
	asked := []rune(phpTrim(query.Get("q")))
	term := phpTrim(string(asked[:min(searchTermMaxLength, len(asked))]))
	if term == "" {
		unprocessable(w, "searchTermMissing", `A search names what it looks for in "q".`)
		return
	}
	offsetText := query.Get("offset")
	if !searchOffsetForm.MatchString(offsetText) {
		unprocessable(w, "searchOffsetNotANumber", `The "offset" of a search is the "next" of the page before.`)
		return
	}
	offset, _ := strconv.Atoi(offsetText)

	hits := s.searchHits(tool, folderID, term, person)
	// The identifier settles a tie, as it does in Minka's statement.
	recent := query.Get("sort") == "recent"
	slices.SortFunc(hits, func(a, b searchHit) int {
		if !recent && a.inTitle != b.inTitle {
			if a.inTitle {
				return -1
			}
			return 1
		}
		if order := strings.Compare(b.updatedAt, a.updatedAt); order != 0 {
			return order
		}
		return strings.Compare(a.id, b.id)
	})

	page := []any{}
	for i := offset; i < len(hits) && i < offset+searchPageSize; i++ {
		page = append(page, hits[i].resource)
	}
	var next any
	if len(hits) > offset+searchPageSize {
		next = strconv.Itoa(offset + searchPageSize)
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": page, "next": next})
}

// searchScope is the tool the search stays in. What is malformed, not there,
// not to be read, or a folder of another tool than the one named, is no scope.
func (s *Server) searchScope(toolID, folderID string, person *Person) (*FilesTool, bool) {
	var tool *FilesTool
	if toolID != "" {
		if !uuidPattern.MatchString(toolID) {
			return nil, false
		}
		if tool = s.visibleFilesTool(toolID, person); tool == nil {
			return nil, false
		}
	}
	if folderID == "" {
		return tool, true
	}
	if !uuidPattern.MatchString(folderID) {
		return nil, false
	}
	var toolOfFolder *FilesTool
	for _, candidate := range s.FilesTools {
		if folderIn(candidate, folderID) != nil {
			toolOfFolder = s.visibleFilesTool(candidate.ID, person)
		}
	}
	if toolOfFolder == nil || tool != nil && tool != toolOfFolder {
		return nil, false
	}
	if folderInUse(toolOfFolder, folderID) == nil || s.isAClient(person.ID) && !shownToClients(toolOfFolder, folderID) {
		return nil, false
	}
	return toolOfFolder, true
}

// searchHits are the published entries of the scope that hold the term in
// their title or their text, in any case. An archived project has none.
func (s *Server) searchHits(tool *FilesTool, folderID, term string, person *Person) []searchHit {
	if project := s.projectWithID(tool.ProjectID); project == nil || project.Archived {
		return []searchHit{}
	}
	inScope := func(entryFolderID string) bool {
		if folderIsInTheTrash(tool, entryFolderID) || s.isAClient(person.ID) && !shownToClients(tool, entryFolderID) {
			return false
		}
		return folderID == "" || folderChainHolds(tool, entryFolderID, func(folder *Folder) bool { return folder.ID == folderID })
	}
	needle := strings.ToLower(term)
	var hits []searchHit
	for _, document := range tool.Documents {
		if document.Draft || document.Trashed || !inScope(document.FolderID) {
			continue
		}
		inTitle := strings.Contains(strings.ToLower(document.Title), needle)
		sections := sectionsOf(document.Body)
		found := slices.IndexFunc(sections, func(section searchSection) bool {
			return strings.Contains(strings.ToLower(section.text), needle)
		})
		if !inTitle && found < 0 {
			continue
		}
		var section any
		excerptOf, url := plainText(document.Body), s.documentURL(tool, document)
		if found >= 0 && sections[found].anchor != "" {
			section = map[string]any{"heading": sections[found].heading, "anchor": sections[found].anchor}
			excerptOf, url = sections[found].excerpt, url+"#"+sections[found].anchor
		}
		hits = append(hits, searchHit{id: document.ID, inTitle: inTitle, updatedAt: timestamp(document.UpdatedAt), resource: s.searchHitResource(
			"document", document.ID, document.Title, excerpt(excerptOf, needle), section, tool, timestamp(document.UpdatedAt),
			document.Archived || folderIsOutOfUse(tool, document.FolderID), url)})
	}
	for _, upload := range tool.Uploads {
		if upload.Trashed || !inScope(upload.FolderID) {
			continue
		}
		filename := ""
		if file := s.storedFileOf(tool.ProjectID, upload.BlobID); file != nil {
			filename = file.Filename
		}
		text := plainText(upload.Description)
		inTitle := strings.Contains(strings.ToLower(upload.Title), needle)
		if !inTitle && !strings.Contains(strings.ToLower(text+" "+filename), needle) {
			continue
		}
		// An upload without a description shows the name of its file.
		if text == "" {
			text = filename
		}
		hits = append(hits, searchHit{id: upload.ID, inTitle: inTitle, updatedAt: timestamp(upload.UpdatedAt), resource: s.searchHitResource(
			"upload", upload.ID, upload.Title, excerpt(text, needle), nil, tool, timestamp(upload.UpdatedAt),
			upload.Archived || folderIsOutOfUse(tool, upload.FolderID), s.filesUploadURL(tool, upload))})
	}
	return hits
}

// searchHitResource is a hit as Minka describes it. archived marks an entry
// in the archive itself or below an archived folder.
func (s *Server) searchHitResource(kind, id, title, excerpt string, section any, tool *FilesTool, updatedAt string, archived bool, appURL string) map[string]any {
	project := s.projectWithID(tool.ProjectID)
	return map[string]any{
		"kind": kind, "id": id, "title": title, "excerpt": excerpt, "section": section,
		"projectId": tool.ProjectID, "toolId": tool.ID, "project": project.Name,
		"updatedAt": updatedAt, "archived": archived, "appUrl": appURL,
	}
}

// sectionsOf splits a body at its headings: the text before the first one,
// then each heading with what follows it up to the next.
func sectionsOf(body string) []searchSection {
	matches := headingForm.FindAllStringSubmatchIndex(body, -1)
	headings := make([]string, len(matches))
	for i, match := range matches {
		headings[i] = plainText(body[match[2]:match[3]])
	}
	anchors := headingAnchors(headings)

	sections := []searchSection{}
	start := len(body)
	if len(matches) > 0 {
		start = matches[0][0]
	}
	if before := plainText(body[:start]); before != "" || len(matches) == 0 {
		sections = append(sections, searchSection{text: before, excerpt: before})
	}
	for i, match := range matches {
		end := len(body)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		text := plainText(body[match[0]:end])
		sections = append(sections, searchSection{heading: headings[i], anchor: anchors[i], text: text,
			excerpt: strings.TrimLeft(strings.TrimPrefix(text, headings[i]), "\n ")})
	}
	return sections
}

// headingAnchors are the anchors of the headings of one text, in their order:
// "h-" and the heading as a slug, a repeated one counted on from "-2".
func headingAnchors(headings []string) []string {
	taken := map[string]bool{}
	anchors := make([]string, len(headings))
	for i, heading := range headings {
		base := "h-" + slugOf(heading)
		anchor := base
		for count := 2; taken[anchor]; count++ {
			anchor = base + "-" + strconv.Itoa(count)
		}
		taken[anchor] = true
		anchors[i] = anchor
	}
	return anchors
}

// slugOf is a heading in lower case, umlauts spelled out, accents dropped and
// every run of other characters one dash; letters of other scripts stay.
func slugOf(heading string) string {
	var slug strings.Builder
	dash := false
	for _, r := range spelledOut.Replace(strings.ToLower(phpTrim(heading))) {
		if base, ok := withoutAccents[r]; ok {
			r = base
		}
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			if dash && slug.Len() > 0 {
				slug.WriteByte('-')
			}
			dash = false
			slug.WriteRune(r)
		default:
			dash = true
		}
	}
	if slug.Len() == 0 {
		return "section"
	}
	return slug.String()
}

// plainText is a piece of HTML as words, without markup.
func plainText(markup string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagForm.ReplaceAllString(markup, " "))), " ")
}

// excerpt is the text around the first place the term stands, or its start
// where the term stands only in the title.
func excerpt(text, needle string) string {
	runes := []rune(text)
	start := 0
	if at := strings.Index(strings.ToLower(text), needle); at > 0 {
		start = min(len(runes), max(0, len([]rune(strings.ToLower(text)[:at]))-searchExcerptLength/4))
	}
	end := min(len(runes), start+searchExcerptLength)
	return string(runes[start:end])
}
