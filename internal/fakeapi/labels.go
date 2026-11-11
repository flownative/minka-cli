package fakeapi

import (
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// labelColors are the colours a label takes, in the order Minka lists them and
// breaks a tie of its suggestion by.
var labelColors = []string{"red", "orange", "yellow", "lime", "green", "teal", "blue", "indigo", "purple", "pink", "brown", "gray"}

// labelColorPlane places every colour by hue in degrees and chroma, where the
// suggestion measures how far one colour lies from another.
var labelColorPlane = map[string][2]float64{
	"red": {25, 0.14}, "orange": {50, 0.13}, "yellow": {90, 0.10}, "lime": {125, 0.12},
	"green": {150, 0.11}, "teal": {190, 0.09}, "blue": {250, 0.12}, "indigo": {280, 0.13},
	"purple": {310, 0.13}, "pink": {350, 0.13}, "brown": {55, 0.06}, "gray": {60, 0.0},
}

const maxLabelNameLength = 50

// lucideIconNames is a sample of the Lucide icons Minka accepts, aliases
// among them ("check-circle" beside "circle-check", "home" beside "house"),
// the names Minka's tests use. Minka itself takes every name of the Lucide set
// it bundles.
var lucideIconNames = map[string]bool{
	"check": true, "bug": true, "flag": true, "star": true, "rocket": true,
	"circle-check": true, "check-circle": true, "house": true, "home": true,
	"square-function": true, "function-square": true,
}

// labelNamePart is what each side of a scoped name, or a plain name, may hold.
var labelNamePart = regexp.MustCompile(`^[\p{L}\p{M}\p{Nd} +*\-]+$`)

var labelScopeSeparator = regexp.MustCompile(` ?:: ?`)

// Label is a label of a project, or of one of its boards or Files tools when
// ToolID is set. Its place in the list of its place is its place among the
// labels of the same place in Server.Labels.
type Label struct {
	ID        string
	ProjectID string
	ToolID    string // empty for a label of the project
	Name      string // tidied, in the case it was written in
	Color     string
	Icon      string // a Lucide name, empty for none; one the fake does not know reads as none
}

func (s *Server) registerLabelRoutes(mux *http.ServeMux) {
	for _, place := range []string{"projects/{projectId}", "boards/{boardId}", "files-tools/{toolId}"} {
		mux.HandleFunc("GET /api/v1/"+place+"/labels", s.authenticated(s.labels))
		mux.HandleFunc("POST /api/v1/"+place+"/labels", s.authenticated(s.addLabel))
		mux.HandleFunc("POST /api/v1/"+place+"/labels/scope-renames", s.authenticated(s.renameLabelScope))
		mux.HandleFunc("PATCH /api/v1/"+place+"/labels/{labelId}", s.authenticated(s.updateLabel))
		mux.HandleFunc("POST /api/v1/"+place+"/labels/{labelId}/moves", s.authenticated(s.moveLabel))
		mux.HandleFunc("DELETE /api/v1/"+place+"/labels/{labelId}", s.authenticated(s.removeLabel))
	}
	for _, item := range []string{"cards/{cardId}", "documents/{documentId}", "files-uploads/{uploadId}", "decisions/{decisionId}"} {
		mux.HandleFunc("POST /api/v1/"+item+"/labels", s.authenticated(s.assignLabel))
		mux.HandleFunc("DELETE /api/v1/"+item+"/labels", s.authenticated(s.unassignLabel))
		mux.HandleFunc("DELETE /api/v1/"+item+"/labels/{labelId}", s.authenticated(s.unassignLabel))
	}
}

// AddLabel puts a label at the end of its place, for a test to start from: a
// project's where toolID is empty, else a board's or a Files tool's.
func (s *Server) AddLabel(projectID, toolID, name, color string) *Label {
	label := &Label{ID: newID(), ProjectID: projectID, ToolID: toolID, Name: tidiedLabelName(name), Color: color}
	s.Labels = append(s.Labels, label)
	return label
}

// PutLabelOn makes an item carry a label, without the rule of the scope.
func (s *Server) PutLabelOn(itemID string, label *Label) {
	s.ItemLabels[itemID] = append(s.ItemLabels[itemID], label.ID)
}

// LabelsOn are the labels an item carries, the tool's before the project's.
func (s *Server) LabelsOn(itemID string) []*Label {
	var tools, projects []*Label
	for _, label := range s.Labels {
		if !slices.Contains(s.ItemLabels[itemID], label.ID) {
			continue
		}
		if label.ToolID != "" {
			tools = append(tools, label)
		} else {
			projects = append(projects, label)
		}
	}
	return append(tools, projects...)
}

// labels lists what the items of a place can take: a tool's own labels, then
// the project's, each counted from 1.
func (s *Server) labels(w http.ResponseWriter, r *http.Request, person *Person) {
	project, toolID, ok := s.labelPlace(w, r, person)
	if !ok {
		return
	}
	list := s.labelResources(s.labelsOf(project.ID, toolID))
	if toolID != "" {
		list = append(list, s.labelResources(s.labelsOf(project.ID, ""))...)
	}
	writeJSON(w, http.StatusOK, list)
}

// addLabel puts a label at the end of its place. A name is taken by any label
// of the project or of one of its tools; the colour a request leaves out is
// the one Minka suggests.
func (s *Server) addLabel(w http.ResponseWriter, r *http.Request, person *Person) {
	project, toolID, ok := s.labelPlace(w, r, person)
	if !ok {
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	name, ok := stringField(w, fields, "name")
	if !ok || !validLabelName(w, deref(name)) {
		return
	}
	color, ok := stringField(w, fields, "color")
	if !ok {
		return
	}
	chosen := ""
	if color == nil {
		taken := s.labelsOf(project.ID, toolID)
		if toolID != "" {
			taken = append(taken, s.labelsOf(project.ID, "")...)
		}
		chosen = suggestedLabelColor(taken)
	} else if chosen = *color; !slices.Contains(labelColors, chosen) {
		labelColorUnknown(w)
		return
	}
	icon, ok := stringField(w, fields, "icon")
	if !ok || icon != nil && !validLabelIcon(w, *icon) {
		return
	}
	tidied := tidiedLabelName(*name)
	if s.labelNameTaken(project.ID, tidied, nil) {
		labelNameTaken(w)
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}

	label := s.AddLabel(project.ID, toolID, tidied, chosen)
	label.Icon = deref(icon)
	writeJSON(w, http.StatusCreated, s.labelResource(label))
}

// updateLabel renames a label of exactly this place, recolours it and sets or
// takes off its icon; what the body leaves out stays, and an icon of null
// takes it off. A rename into a scope takes nothing off the items.
func (s *Server) updateLabel(w http.ResponseWriter, r *http.Request, person *Person) {
	project, label, ok := s.labelOfPlace(w, r, person)
	if !ok {
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	name, ok := stringField(w, fields, "name")
	if !ok {
		return
	}
	color, ok := stringField(w, fields, "color")
	if !ok {
		return
	}
	icon, ok := stringField(w, fields, "icon")
	if !ok {
		return
	}
	_, iconGiven := fields["icon"]
	if name == nil && color == nil && !iconGiven {
		unprocessable(w, "nothingToChange", `Name "name", "color" or "icon" to change a label.`)
		return
	}
	newName, newColor, newIcon := label.Name, label.Color, knownLabelIcon(label.Icon)
	if name != nil {
		if !validLabelName(w, *name) {
			return
		}
		newName = tidiedLabelName(*name)
	}
	if color != nil {
		if !slices.Contains(labelColors, *color) {
			labelColorUnknown(w)
			return
		}
		newColor = *color
	}
	if iconGiven {
		if icon != nil && !validLabelIcon(w, *icon) {
			return
		}
		newIcon = deref(icon)
	}
	if s.labelNameTaken(project.ID, newName, label) {
		labelNameTaken(w)
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}

	label.Name, label.Color, label.Icon = newName, newColor, newIcon
	writeJSON(w, http.StatusOK, s.labelResource(label))
}

// renameLabelScope gives every label of one scope at exactly this place the
// new scope, or none of them: a resulting name another label of the project
// bears refuses the whole rename. A scope already in use is joined, and the
// items keep whatever they carry, two labels of one scope included. The
// answer is the place's own labels, without the project's under a board or
// a tool.
func (s *Server) renameLabelScope(w http.ResponseWriter, r *http.Request, person *Person) {
	project, toolID, ok := s.labelPlace(w, r, person)
	if !ok {
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	scope, ok := stringField(w, fields, "scope")
	if !ok {
		return
	}
	newScope, ok := stringField(w, fields, "newScope")
	if !ok {
		return
	}
	if !validLabelScope(w, deref(scope)) || !validLabelScope(w, deref(newScope)) {
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}
	scopeKey, tidiedScope := labelNameKey(*scope), tidiedLabelName(*newScope)

	// Each label is judged in the order of its place against the names
	// before the rename; the server renames one after the other and stops
	// at the first refusal.
	renames := map[*Label]string{}
	for _, label := range s.labelsOf(project.ID, toolID) {
		if labelScopeKey(label.Name) != scopeKey {
			continue
		}
		_, value, _ := strings.Cut(label.Name, "::")
		renamed := tidiedScope + "::" + value
		if utf8.RuneCountInString(renamed) > maxLabelNameLength {
			unprocessable(w, "labelNameTooLong", "With this scope a label name would have more than 50 characters.")
			return
		}
		if s.labelNameTaken(project.ID, renamed, label) {
			labelNameTaken(w)
			return
		}
		renames[label] = renamed
	}
	if len(renames) == 0 {
		notFound(w)
		return
	}

	for label, renamed := range renames {
		label.Name = renamed
	}
	writeJSON(w, http.StatusOK, s.labelResources(s.labelsOf(project.ID, toolID)))
}

// labelsForANewCard are the labels a new card is to carry, each named by the
// exact ID of a label of its board or its project. A client of the project
// hears of no label, but only where labels are sent.
func (s *Server) labelsForANewCard(w http.ResponseWriter, board *Board, person *Person, labelIDs []string) ([]*Label, bool) {
	if len(labelIDs) == 0 {
		return nil, true
	}
	if s.isAClient(person.ID) {
		notFound(w)
		return nil, false
	}
	choices := append(s.labelsOf(board.ProjectID, board.ID), s.labelsOf(board.ProjectID, "")...)
	var labels []*Label
	for _, id := range labelIDs {
		at := slices.IndexFunc(choices, func(l *Label) bool { return l.ID == id })
		if at < 0 {
			unprocessable(w, "labelUnknown", "Neither this board nor its project has such a label.")
			return nil, false
		}
		labels = append(labels, choices[at])
	}
	return labels, true
}

// labelsShareAScope says whether two different labels of one scope are
// among those named; one label named twice counts once.
func labelsShareAScope(labels []*Label) bool {
	seen := map[string]*Label{}
	for _, label := range labels {
		scope := labelScopeKey(label.Name)
		if scope == "" {
			continue
		}
		if other, taken := seen[scope]; taken && other != label {
			return true
		}
		seen[scope] = label
	}
	return false
}

// moveLabel puts a label right behind previousLabelId, which alone decides
// where both are given, else right in front of nextLabelId, else at the front.
// Either has to be another label of the same place. An archived project
// refuses a move that would change nothing as well.
func (s *Server) moveLabel(w http.ResponseWriter, r *http.Request, person *Person) {
	project, label, ok := s.labelOfPlace(w, r, person)
	if !ok {
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	previousID, ok := stringField(w, fields, "previousLabelId")
	if !ok {
		return
	}
	nextID, ok := stringField(w, fields, "nextLabelId")
	if !ok {
		return
	}
	siblings := s.labelsOf(project.ID, label.ToolID)
	var neighbours [2]*Label
	for i, id := range []*string{previousID, nextID} {
		if id == nil || *id == "" {
			continue
		}
		at := slices.IndexFunc(siblings, func(l *Label) bool { return l.ID == *id })
		if at < 0 || siblings[at] == label {
			unprocessable(w, "labelPlaceUnknown", "A neighbour of a move is another label of the same place.")
			return
		}
		neighbours[i] = siblings[at]
	}
	if s.refusedAsArchived(w, project) {
		return
	}

	others := slices.DeleteFunc(slices.Clone(siblings), func(l *Label) bool { return l == label })
	to := 0
	if neighbours[0] != nil {
		to = slices.Index(others, neighbours[0]) + 1
	} else if neighbours[1] != nil {
		to = slices.Index(others, neighbours[1])
	}
	ordered := slices.Insert(others, to, label)
	// The labels of other places keep their slots; this place's slots take
	// its labels in their new order.
	next := 0
	for i, l := range s.Labels {
		if l.ProjectID == project.ID && l.ToolID == label.ToolID {
			s.Labels[i] = ordered[next]
			next++
		}
	}
	writeJSON(w, http.StatusOK, s.labelResource(label))
}

// removeLabel deletes a label and takes it off every item; there is no trash
// for labels.
func (s *Server) removeLabel(w http.ResponseWriter, r *http.Request, person *Person) {
	project, label, ok := s.labelOfPlace(w, r, person)
	if !ok {
		return
	}
	if s.refusedAsArchived(w, project) {
		return
	}

	s.Labels = slices.DeleteFunc(s.Labels, func(l *Label) bool { return l == label })
	for itemID, ids := range s.ItemLabels {
		s.ItemLabels[itemID] = slices.DeleteFunc(ids, func(id string) bool { return id == label.ID })
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// assignLabel puts a label of the item's tool or project on the item, by its
// ID or by its name. A scoped label takes any other label of the same scope
// off. The answer is every label the item carries: 201 where the label is new
// on it, 200 where it carried it already.
func (s *Server) assignLabel(w http.ResponseWriter, r *http.Request, person *Person) {
	item, ok := s.labelledItem(w, r, person)
	if !ok {
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	labelID, ok := stringField(w, fields, "labelId")
	if !ok {
		return
	}
	name, ok := stringField(w, fields, "name")
	if !ok {
		return
	}
	if labelID != nil && name != nil {
		labelAmbiguous(w)
		return
	}
	label, ok := s.labelForItem(w, item, labelID, name)
	if !ok {
		return
	}
	if s.refusedAsAClosedItem(w, item) {
		return
	}

	status := http.StatusOK
	if !slices.Contains(s.ItemLabels[item.id], label.ID) {
		status = http.StatusCreated
		if scope := labelScopeKey(label.Name); scope != "" {
			kept := []string{}
			for _, carried := range s.LabelsOn(item.id) {
				if carried == label || labelScopeKey(carried.Name) != scope {
					kept = append(kept, carried.ID)
				}
			}
			s.ItemLabels[item.id] = kept
		}
		s.PutLabelOn(item.id, label)
	}
	writeJSON(w, status, s.labelResources(s.LabelsOn(item.id)))
}

// unassignLabel takes a label off an item, by the ID in the address or by
// ?name=; a label the item does not carry is no refusal. No body is read.
func (s *Server) unassignLabel(w http.ResponseWriter, r *http.Request, person *Person) {
	item, ok := s.labelledItem(w, r, person)
	if !ok {
		return
	}
	labelID := r.PathValue("labelId")
	name := r.URL.Query().Get("name")
	if labelID != "" && name != "" {
		labelAmbiguous(w)
		return
	}
	if labelID != "" && !uuidPattern.MatchString(labelID) {
		notFound(w)
		return
	}
	var byID, byName *string
	if labelID != "" {
		byID = &labelID
	}
	if name != "" {
		byName = &name
	}
	label, ok := s.labelForItem(w, item, byID, byName)
	if !ok {
		return
	}
	if s.refusedAsAClosedItem(w, item) {
		return
	}

	s.ItemLabels[item.id] = slices.DeleteFunc(s.ItemLabels[item.id], func(id string) bool { return id == label.ID })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// labelPlace finds the project or the tool an address names. A client of the
// project hears of no label, the same as somebody outside it.
func (s *Server) labelPlace(w http.ResponseWriter, r *http.Request, person *Person) (*Project, string, bool) {
	var project *Project
	toolID := ""
	switch {
	case r.PathValue("boardId") != "":
		if board := s.visibleBoard(r.PathValue("boardId"), person); board != nil {
			project, toolID = s.projectOf(board), board.ID
		}
	case r.PathValue("toolId") != "":
		if tool := s.visibleFilesTool(r.PathValue("toolId"), person); tool != nil {
			project, toolID = s.projectWithID(tool.ProjectID), tool.ID
		}
	default:
		project = s.visibleProject(r.PathValue("projectId"), person)
	}
	if project == nil || s.isAClient(person.ID) {
		notFound(w)
		return nil, "", false
	}
	return project, toolID, true
}

// labelOfPlace finds a label of exactly the place the address names: a tool's
// label is at no address of its project, and the other way round.
func (s *Server) labelOfPlace(w http.ResponseWriter, r *http.Request, person *Person) (*Project, *Label, bool) {
	project, toolID, ok := s.labelPlace(w, r, person)
	if !ok {
		return nil, nil, false
	}
	labelID := r.PathValue("labelId")
	for _, label := range s.labelsOf(project.ID, toolID) {
		if label.ID == labelID {
			return project, label, true
		}
	}
	notFound(w)
	return nil, nil, false
}

// labelledItem is a card, a document, an upload or a decision whose labels are changed,
// with what refuses a change on it.
type labelledItem struct {
	id           string
	project      *Project
	toolID       string
	archived     bool
	archivedCode string
}

// labelledItem finds the item an address names. A card in the archive of its
// board is found, a draft is not, and a client of the project finds nothing.
func (s *Server) labelledItem(w http.ResponseWriter, r *http.Request, person *Person) (labelledItem, bool) {
	var item labelledItem
	switch {
	case r.PathValue("cardId") != "":
		if board, _, card, archived := s.cardToWrite(r.PathValue("cardId"), person); card != nil {
			item = labelledItem{id: card.ID, project: s.projectOf(board), toolID: board.ID, archived: archived, archivedCode: "cardArchived"}
		}
	case r.PathValue("documentId") != "":
		if tool, document := s.visibleDocument(r.PathValue("documentId"), person); document != nil {
			item = labelledItem{id: document.ID, project: s.projectWithID(tool.ProjectID), toolID: tool.ID,
				archived: document.Archived || folderIsOutOfUse(tool, document.FolderID), archivedCode: "documentArchived"}
		}
	case r.PathValue("decisionId") != "":
		if tool, decision := s.visibleDecision(r.PathValue("decisionId"), person); decision != nil {
			item = labelledItem{id: decision.ID, project: s.projectWithID(tool.ProjectID), toolID: tool.ID,
				archived: decision.Archived, archivedCode: "decisionArchived"}
		}
	default:
		if tool, upload := s.visibleFilesUpload(r.PathValue("uploadId"), person); upload != nil {
			item = labelledItem{id: upload.ID, project: s.projectWithID(tool.ProjectID), toolID: tool.ID,
				archived: upload.Archived || folderIsOutOfUse(tool, upload.FolderID), archivedCode: "uploadArchived"}
		}
	}
	if item.project == nil || s.isAClient(person.ID) {
		notFound(w)
		return item, false
	}
	return item, true
}

// labelForItem is the label of the item's tool or project an ID names
// exactly, or a name names the way names are compared. A name Minka would not
// take names no label.
func (s *Server) labelForItem(w http.ResponseWriter, item labelledItem, labelID, name *string) (*Label, bool) {
	if labelID == nil && name == nil {
		unprocessable(w, "labelMissing", `Name the label by "labelId" or by "name".`)
		return nil, false
	}
	key := ""
	if name != nil && labelNameRefusal(*name) == "" {
		key = labelNameKey(*name)
	}
	choices := append(s.labelsOf(item.project.ID, item.toolID), s.labelsOf(item.project.ID, "")...)
	for _, choice := range choices {
		if labelID != nil && choice.ID == *labelID || labelID == nil && key != "" && labelNameKey(choice.Name) == key {
			return choice, true
		}
	}
	unprocessable(w, "labelUnknown", "Neither this item's tool nor its project has such a label.")
	return nil, false
}

// refusedAsAClosedItem says why the labels of an item stay as they are: its
// project is archived, or the item is.
func (s *Server) refusedAsAClosedItem(w http.ResponseWriter, item labelledItem) bool {
	if s.refusedAsArchived(w, item.project) {
		return true
	}
	if item.archived {
		unprocessable(w, item.archivedCode, "This item is archived. Its labels stay as they are until it is brought back.")
		return true
	}
	return false
}

// labelsOf are the labels of one place in their order.
func (s *Server) labelsOf(projectID, toolID string) []*Label {
	var labels []*Label
	for _, label := range s.Labels {
		if label.ProjectID == projectID && label.ToolID == toolID {
			labels = append(labels, label)
		}
	}
	return labels
}

// labelNameTaken says whether a label of the project or of any of its tools
// other than the one renamed bears the name, compared case-folded.
func (s *Server) labelNameTaken(projectID, name string, renamed *Label) bool {
	for _, label := range s.Labels {
		if label.ProjectID == projectID && label != renamed && labelNameKey(label.Name) == labelNameKey(name) {
			return true
		}
	}
	return false
}

func (s *Server) labelResources(labels []*Label) []any {
	list := []any{}
	for _, label := range labels {
		list = append(list, s.labelResource(label))
	}
	return list
}

// labelResource is a label as every answer shows it. Its position counts
// within its own place.
func (s *Server) labelResource(label *Label) map[string]any {
	var scope any
	value := label.Name
	if before, after, scoped := strings.Cut(label.Name, "::"); scoped {
		scope, value = before, after
	}
	position := slices.Index(s.labelsOf(label.ProjectID, label.ToolID), label) + 1
	return map[string]any{
		"id": label.ID, "name": label.Name, "scope": scope, "value": value, "color": label.Color,
		"icon": nullable(knownLabelIcon(label.Icon)), "projectId": label.ProjectID, "toolId": nullable(label.ToolID), "position": position,
	}
}

// tidiedLabelName is a name as Minka judges and keeps it: composed, every run
// of white space one space, no space around "::", trimmed.
func tidiedLabelName(name string) string {
	composed := norm.NFC.String(name)
	collapsed := strings.Join(strings.FieldsFunc(composed, unicode.IsSpace), " ")
	return phpTrim(labelScopeSeparator.ReplaceAllString(collapsed, "::"))
}

// labelNameKey is what two names are compared by.
func labelNameKey(name string) string {
	return cases.Fold().String(tidiedLabelName(name))
}

// labelScopeKey is the scope of a name as scopes are compared, empty for a
// plain label.
func labelScopeKey(name string) string {
	scope, _, scoped := strings.Cut(name, "::")
	if !scoped {
		return ""
	}
	return cases.Fold().String(scope)
}

// labelNameRefusal is the code Minka refuses a name with, empty for none, in
// its order: nothing, too long, then the shape.
func labelNameRefusal(name string) string {
	name = tidiedLabelName(name)
	if name == "" {
		return "labelNameMissing"
	}
	if utf8.RuneCountInString(name) > maxLabelNameLength {
		return "labelNameTooLong"
	}
	parts := strings.Split(name, "::")
	if len(parts) > 2 {
		return "labelNameMalformed"
	}
	for _, part := range parts {
		if phpTrim(part) == "" || !labelNamePart.MatchString(part) {
			return "labelNameMalformed"
		}
	}
	return ""
}

var labelNameRefusals = map[string]string{
	"labelNameMissing":   "A label needs a name.",
	"labelNameTooLong":   "A label name has at most 50 characters.",
	"labelNameMalformed": `A label name holds letters, digits, spaces, "-", "+" and "*", with at most one "::" between scope and value.`,
}

func validLabelName(w http.ResponseWriter, name string) bool {
	if code := labelNameRefusal(name); code != "" {
		unprocessable(w, code, labelNameRefusals[code])
		return false
	}
	return true
}

// validLabelScope refuses what makes no scope: a scope follows the rules of
// a name and holds no "::".
func validLabelScope(w http.ResponseWriter, scope string) bool {
	switch labelNameRefusal(scope) {
	case "":
		if !strings.Contains(tidiedLabelName(scope), "::") {
			return true
		}
	case "labelNameMissing":
		unprocessable(w, "labelScopeMissing", `Name the scope in "scope" and its new name in "newScope".`)
		return false
	case "labelNameTooLong":
		unprocessable(w, "labelNameTooLong", "A label scope has at most 50 characters.")
		return false
	}
	unprocessable(w, "labelScopeMalformed", `A label scope holds letters, digits, spaces, "-", "+" and "*", and no "::".`)
	return false
}

// validLabelIcon refuses a name that is no Lucide icon, the empty one included.
func validLabelIcon(w http.ResponseWriter, icon string) bool {
	if !lucideIconNames[icon] {
		unprocessable(w, "labelIconUnknown", `A label icon is the name of a Lucide icon, such as "check".`)
		return false
	}
	return true
}

// knownLabelIcon is a stored icon as Minka reads it: a name Lucide no longer
// has is none, so the label stays readable and editable.
func knownLabelIcon(icon string) string {
	if lucideIconNames[icon] {
		return icon
	}
	return ""
}

func labelColorUnknown(w http.ResponseWriter) {
	unprocessable(w, "labelColorUnknown", "A label color is one of "+strings.Join(labelColors, ", ")+".")
}

func labelNameTaken(w http.ResponseWriter) {
	unprocessable(w, "labelNameTaken", "Another label of this project or of one of its tools has that name.")
}

func labelAmbiguous(w http.ResponseWriter) {
	unprocessable(w, "labelAmbiguous", `Name the label by "labelId" or by "name", not both.`)
}

// suggestedLabelColor is the colour Minka suggests beside the labels taken:
// among the colours used least, the one whose nearest used colour lies
// farthest away, the earlier one on a tie; blue where nothing is taken.
func suggestedLabelColor(taken []*Label) string {
	if len(taken) == 0 {
		return "blue"
	}
	uses := map[string]int{}
	for _, label := range taken {
		uses[label.Color]++
	}
	fewest := math.MaxInt
	for _, color := range labelColors {
		fewest = min(fewest, uses[color])
	}
	chosen, chosenDistance := "blue", -1.0
	for _, candidate := range labelColors {
		if uses[candidate] != fewest {
			continue
		}
		distance := math.Inf(1)
		for _, label := range taken {
			distance = min(distance, labelColorDistance(candidate, label.Color))
		}
		if distance > chosenDistance {
			chosen, chosenDistance = candidate, distance
		}
	}
	return chosen
}

func labelColorDistance(one, other string) float64 {
	a, b := onTheColorPlane(one)
	otherA, otherB := onTheColorPlane(other)
	return math.Hypot(a-otherA, b-otherB)
}

func onTheColorPlane(color string) (float64, float64) {
	place := labelColorPlane[color]
	hue := place[0] * math.Pi / 180
	return place[1] * math.Cos(hue), place[1] * math.Sin(hue)
}
