package fakeapi

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// What the server keeps of a decision.
const (
	maxDecisionBodyLength   = 100000
	maxDecisionReason       = 500
	maxDecisionVotes        = 100
	maxDeciderName          = 100
	mostDecidersOfADecision = 50
	maxLinkDescription      = 200
	maxDecisionsAtOnce      = 100 // written down by one request
)

// decisionRoles are the roles a status of a Decisions tool goes by, in the
// order a new tool lists them, with the English name each is shown by.
var decisionRoles = []struct{ role, name string }{
	{"draft", "Draft"}, {"proposed", "Proposed"}, {"holds", "In force"}, {"done", "Done"},
	{"superseded", "Superseded"}, {"revoked", "Revoked"}, {"rejected", "Rejected"},
}

// dayForm is a day as a decision takes one, checked against the calendar
// afterwards.
var dayForm = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// DecisionsTool is a Decisions tool of a project with its statuses in their
// order and its decisions in no particular order: the list orders them.
type DecisionsTool struct {
	ID        string
	ProjectID string
	Title     string

	// Archived is a tool no longer in use: the project names it among its
	// tools no more, and at its own address it is read and written as before.
	Archived bool

	// ShownToClients lets the clients of the project into the tool at all.
	ShownToClients bool

	// Prefix numbers the decisions of the tool, "DEC" as in DEC-14; a prefix
	// replaced since stays a former one, and a decision goes by it as well.
	Prefix         string
	FormerPrefixes []string
	NextNumber     int

	Statuses  []*DecisionStatus
	Decisions []*Decision
}

// DecisionStatus is one status of a tool. Name is empty for a status that
// goes by the name of its role; Role is empty for one the team added.
type DecisionStatus struct {
	ID   string
	Name string
	Role string
}

// Decider is one of who decided: a member by ID, or a name given freely.
type Decider struct {
	PersonID string
	Name     string
}

// DecisionVersion is what a write left a decision saying.
type DecisionVersion struct {
	ID        string
	Title     string
	Reason    string
	Body      string
	DecidedOn string
	DecidedBy []Decider
	EventID   string
	Votes     string
	AuthorID  string
	WrittenAt time.Time
}

// DecisionEntry is one entry of the history or the consequences of a
// decision: a record of the instance, or a link with its description.
type DecisionEntry struct {
	ID       string
	Kind     string // card, todo, post or link
	TargetID string // empty for a link
	URL      string // a link's address
	Title    string // a link's description
}

// Decision is one decision of a tool. StatusID is empty for a decision from
// before its tool had statuses, which holds.
type Decision struct {
	ID         string
	Title      string
	Reason     string // empty for none
	Body       string // the HTML as stored, empty for none
	StatusID   string
	DecidedOn  string // YYYY-MM-DD, empty while the day is not known
	DecidedBy  []Decider
	EventID    string // the event it was taken in, empty for none
	Votes      string
	ValidUntil string
	ReviewOn   string

	// EarlierID is the decision this one supersedes or revokes, by Relation.
	EarlierID string
	Relation  string // "supersedes" or "revokes"

	// AppliesToKind is "event" or "card", empty for nothing.
	AppliesToKind string
	AppliesToID   string

	ShownToClients bool
	Archived       bool
	Trashed        bool

	// ShortID is such as "DEC-14", empty for a decision its tool has not
	// numbered yet.
	ShortID string

	// AgendaNumber is its place among the decisions written down with it,
	// counted from 1; 0 for a decision written down alone.
	AgendaNumber int

	// StatusSetBy is who set the current status, nil where nobody is known.
	StatusSetBy *DecisionStatusSetBy

	// Announcements are the lines a status of role proposed or holds sent
	// out, oldest first.
	Announcements []DecisionAnnouncement

	CreatedAt time.Time
	UpdatedAt time.Time
	CreatorID string
	Comments  []*Comment

	History      []*DecisionEntry
	Consequences []*DecisionEntry
	Versions     []*DecisionVersion
}

// DecisionStatusSetBy is who set the status of a decision and when. With
// Minka true, Minka moved it, and PersonID is the one its line names; an
// empty PersonID is a person purged since.
type DecisionStatusSetBy struct {
	PersonID string
	Minka    bool
	At       time.Time
}

// DecisionAnnouncement is whom a decision told of the status it came into:
// "everybody", "chosen" with NotifyIDs, or "nobody".
type DecisionAnnouncement struct {
	Role      string
	Audience  string
	NotifyIDs []string
}

// decisionAudiences are the audiences a decision proposed or in force
// reaches; everybody when none is named.
var decisionAudiences = []string{"everybody", "chosen", "nobody"}

// NewDecisionsTool is a tool with the statuses a new tool gets, one per role,
// numbering its decisions under DEC.
func NewDecisionsTool(projectID, title string) *DecisionsTool {
	tool := &DecisionsTool{ID: newID(), ProjectID: projectID, Title: title, ShownToClients: true, Prefix: "DEC", NextNumber: 1}
	for _, role := range decisionRoles {
		tool.Statuses = append(tool.Statuses, &DecisionStatus{ID: newID(), Role: role.role})
	}
	return tool
}

// StatusOfRole is the status of the tool that goes by a role.
func (t *DecisionsTool) StatusOfRole(role string) *DecisionStatus {
	for _, status := range t.Statuses {
		if status.Role == role {
			return status
		}
	}
	return nil
}

func (s *Server) registerDecisionsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectId}/decisions", s.authenticated(s.decisionsTools))
	mux.HandleFunc("GET /api/v1/decisions-tools/{toolId}", s.authenticated(s.decisionsTool))
	mux.HandleFunc("GET /api/v1/decisions-tools/{toolId}/statuses", s.authenticated(s.decisionStatuses))
	mux.HandleFunc("GET /api/v1/decisions-tools/{toolId}/decisions", s.authenticated(s.decisionsList))
	mux.HandleFunc("POST /api/v1/decisions-tools/{toolId}/decisions", s.authenticated(s.addDecision))
	mux.HandleFunc("POST /api/v1/decisions-tools/{toolId}/decisions/batch", s.authenticated(s.addDecisions))
	mux.HandleFunc("GET /api/v1/decisions/{decisionId}", s.authenticated(s.decision))
	// A pattern of its own for "by-short-id" would clash with the comments
	// under a decision, so it is told apart here.
	mux.HandleFunc("GET /api/v1/decisions/{first}/{second}", s.authenticated(s.decisionByShortIdentifier))
	mux.HandleFunc("PATCH /api/v1/decisions/{decisionId}", s.authenticated(s.updateDecision))
	mux.HandleFunc("DELETE /api/v1/decisions/{decisionId}", s.authenticated(s.removeDecision))
	mux.HandleFunc("GET /api/v1/decisions/{decisionId}/versions", s.authenticated(s.decisionVersions))
	mux.HandleFunc("GET /api/v1/decisions/{decisionId}/versions/{versionId}", s.authenticated(s.decisionVersion))
	mux.HandleFunc("POST /api/v1/decisions/{decisionId}/history", s.authenticated(s.addDecisionHistoryEntry))
	mux.HandleFunc("POST /api/v1/decisions/{decisionId}/consequences", s.authenticated(s.addDecisionConsequence))
	mux.HandleFunc("POST /api/v1/decisions/{decisionId}/entries/{entryId}/moves", s.authenticated(s.moveDecisionEntry))
	mux.HandleFunc("DELETE /api/v1/decisions/{decisionId}/entries/{entryId}", s.authenticated(s.removeDecisionEntry))

	s.registerCommentRoutes(mux, "decisions/{decisionId}")
}

func (s *Server) decisionsTools(w http.ResponseWriter, r *http.Request, person *Person) {
	project := s.visibleProject(r.PathValue("projectId"), person)
	if project == nil {
		notFound(w)
		return
	}
	tools := []any{}
	for _, tool := range s.DecisionsTools {
		if tool.ProjectID == project.ID && !tool.Archived && (tool.ShownToClients || !s.isAClient(person.ID)) {
			tools = append(tools, map[string]any{"id": tool.ID, "title": tool.Title, "appUrl": s.decisionsToolURL(tool)})
		}
	}
	writeJSON(w, http.StatusOK, tools)
}

func (s *Server) decisionsTool(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleDecisionsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.toolResource(tool.ID, tool.ProjectID, tool.Title, s.decisionsToolURL(tool)))
}

// decisionStatuses lists the statuses of a tool in their order; a client is
// offered no draft.
func (s *Server) decisionStatuses(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleDecisionsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	statuses := []any{}
	for _, status := range s.statusesOfferedTo(tool, person) {
		statuses = append(statuses, statusResource(status))
	}
	writeJSON(w, http.StatusOK, statuses)
}

// decisionsList lists the decisions of a tool out of its archive, the day
// decided on last first and one without a day by the day it was written, in
// every status unless one is named; "label" and "appliesTo" narrow it by an
// identifier each, "setBy" "agent" to the decisions whose status an agent set
// by hand.
func (s *Server) decisionsList(w http.ResponseWriter, r *http.Request, person *Person) {
	tool := s.visibleDecisionsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	query := r.URL.Query()
	label := query.Get("label")
	if label != "" && !uuidPattern.MatchString(label) {
		unprocessable(w, "decisionLabelUnknown", "The label is named by its identifier.")
		return
	}
	appliesTo := query.Get("appliesTo")
	if appliesTo != "" && !uuidPattern.MatchString(appliesTo) {
		unprocessable(w, "decisionAppliesToUnknown", "The event or card is named by its identifier.")
		return
	}
	statusID := ""
	if value := query.Get("status"); value != "" && value != "all" {
		status := s.statusNamed(tool, person, value)
		if status == nil {
			decisionStatusUnknown(w)
			return
		}
		statusID = status.ID
	}
	setBy := query.Get("setBy")
	if setBy != "" && setBy != "agent" {
		unprocessable(w, "decisionSetByUnknown", `Only "agent" narrows by who set the status.`)
		return
	}

	var listed []*Decision
	for _, decision := range tool.Decisions {
		if decision.Archived || !s.seesDecision(tool, decision, person) {
			continue
		}
		if statusID != "" && s.statusIDOf(tool, decision) != statusID {
			continue
		}
		if label != "" && !slices.Contains(s.ItemLabels[decision.ID], strings.ToLower(label)) {
			continue
		}
		if appliesTo != "" && decision.AppliesToID != strings.ToLower(appliesTo) {
			continue
		}
		if setBy != "" && !s.setByAnAgent(decision) {
			continue
		}
		listed = append(listed, decision)
	}
	slices.SortFunc(listed, func(a, b *Decision) int {
		if c := cmp.Compare(dayOfTheList(b), dayOfTheList(a)); c != 0 {
			return c
		}
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return compareAgendaNumbers(a, b)
	})
	list := []any{}
	for _, decision := range listed {
		list = append(list, s.decisionListItemResource(tool, decision))
	}
	writeJSON(w, http.StatusOK, list)
}

// compareAgendaNumbers puts decisions written down together in the order of
// their agenda, and one written down alone after them.
func compareAgendaNumbers(a, b *Decision) int {
	switch {
	case a.AgendaNumber == b.AgendaNumber:
		return 0
	case a.AgendaNumber == 0:
		return 1
	case b.AgendaNumber == 0:
		return -1
	}
	return cmp.Compare(a.AgendaNumber, b.AgendaNumber)
}

// dayOfTheList is the day a decision stands at in the list: the one decided
// on, else the one it was written on.
func dayOfTheList(decision *Decision) string {
	if decision.DecidedOn != "" {
		return decision.DecidedOn
	}
	return decision.CreatedAt.UTC().Format(time.DateOnly)
}

func (s *Server) decision(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, decision := s.visibleDecision(r.PathValue("decisionId"), person)
	if decision == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.decisionResource(tool, decision, person))
}

// decisionByShortIdentifier finds a decision by its short identifier in any
// case, also under a former prefix of its tool, under the conditions of its
// page. A draft for a client, an unknown name and a name of the wrong shape are
// the same 404.
func (s *Server) decisionByShortIdentifier(w http.ResponseWriter, r *http.Request, person *Person) {
	if r.PathValue("first") != "by-short-id" {
		notFound(w)
		return
	}
	tool, decision := s.decisionGoingBy(r.PathValue("second"), person)
	if decision == nil {
		notFound(w)
		return
	}
	resource := s.decisionResource(tool, decision, person)
	if s.AnswersDecisionsWithoutID {
		resource["id"] = ""
	}
	writeJSON(w, http.StatusOK, resource)
}

// decisionGoingBy finds the decision a person is shown under a short
// identifier in any case, its tool's prefix or a former one.
func (s *Server) decisionGoingBy(identifier string, person *Person) (*DecisionsTool, *Decision) {
	prefix, number, found := strings.Cut(strings.ToUpper(strings.TrimSpace(identifier)), "-")
	if !found || prefix == "" || number == "" {
		return nil, nil
	}
	for _, tool := range s.DecisionsTools {
		if !strings.EqualFold(tool.Prefix, prefix) && !slices.ContainsFunc(tool.FormerPrefixes, func(former string) bool { return strings.EqualFold(former, prefix) }) {
			continue
		}
		for _, decision := range tool.Decisions {
			if decision.ShortID == "" || !strings.HasSuffix(decision.ShortID, "-"+number) {
				continue
			}
			if visibleTool, visible := s.visibleDecision(decision.ID, person); visible != nil {
				return visibleTool, visible
			}
		}
	}
	return nil, nil
}

// setByAnAgent says whether an agent set the status of a decision by hand,
// not Minka on its behalf.
func (s *Server) setByAnAgent(decision *Decision) bool {
	setBy := decision.StatusSetBy
	if setBy == nil || setBy.Minka || s.People[setBy.PersonID] == nil {
		return false
	}
	return kindOf(s.People[setBy.PersonID]) == "agent"
}

// decisionFields is what a write of a decision says, read and checked.
type decisionFields struct {
	title, reason, body, decidedOn *string
	statusID                       *string
	decidedBy                      *[]Decider
	eventID                        *string
	votes                          *string
	earlier                        *struct{ id, relation string }
	appliesTo                      *struct{ kind, id string }
	validUntil, reviewOn           *string

	// audience is whom a status of role proposed or holds reaches, named or
	// not; notifyIDs the people of "chosen".
	audience  string
	notifyIDs []string
}

// addDecision writes a decision down, refusing in the order the controller
// refuses: the tool, then each field, then the project, and last what the
// write meets of the subject and the earlier decision.
func (s *Server) addDecision(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool := s.visibleDecisionsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	read, ok := s.readDecisionFields(w, fields, tool, person, nil)
	if !ok {
		return
	}
	visible, ok := booleanField(w, fields, "visibleToClients")
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	decision := &Decision{ID: newID(), CreatedAt: s.now, UpdatedAt: s.now, CreatorID: person.ID,
		ShownToClients: visible != nil && *visible || s.isAClient(person.ID), StatusID: tool.StatusOfRole("holds").ID}
	if !s.subjectAndEarlierKnown(w, tool, decision, read, person, false) {
		return
	}
	if read.statusID != nil {
		decision.StatusID = *read.statusID
	}
	announcement, ok := s.announced(w, tool, decision.ShownToClients, decision.StatusID, read, person)
	if !ok {
		return
	}
	read.applyTo(decision)
	if read.decidedOn == nil && decision.DecidedOn == "" && decision.EventID != "" {
		decision.DecidedOn = s.dayAnEventBrings(decision.EventID, person)
	}
	decision.StatusSetBy = &DecisionStatusSetBy{PersonID: person.ID, At: s.now}
	if announcement != nil {
		decision.Announcements = append(decision.Announcements, *announcement)
	}
	s.numberAndKeep(tool, decision, person)
	if s.statusRole(tool, decision.StatusID) == "holds" {
		s.moveTheEarlier(tool, decision, person)
	}
	writeJSON(w, http.StatusCreated, s.decisionResource(tool, decision, person))
}

// numberAndKeep gives a new decision the next short identifier of its tool,
// where the tool numbers them, and its first version.
func (s *Server) numberAndKeep(tool *DecisionsTool, decision *Decision, person *Person) {
	if tool.Prefix != "" {
		number := max(tool.NextNumber, 1)
		decision.ShortID = tool.Prefix + "-" + strconv.Itoa(number)
		tool.NextNumber = number + 1
	}
	decision.Versions = append(decision.Versions, decision.snapshot(person.ID, s.now))
	tool.Decisions = append(tool.Decisions, decision)
}

// decisionsSharedFields are the fields the decisions written down at once
// share, each read as for one decision; the batch reads no other.
var decisionsSharedFields = []string{
	"decidedOn", "decidedBy", "decidedInEventId", "status", "appliesTo", "audience", "notifyIds", "visibleToClients", "labelIds",
}

// addDecisions writes several decisions down at once, all or none, refusing
// in the order the controller refuses: the tool, the sentences, the shared
// fields, the labels, then what the first decision meets: its subject, the
// scopes of its labels and the people chosen. They share one moment and
// are numbered and listed in their order.
func (s *Server) addDecisions(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool := s.visibleDecisionsTool(r.PathValue("toolId"), person)
	if tool == nil {
		notFound(w)
		return
	}
	titles, ok := decisionLines(w, fields["titles"])
	if !ok {
		return
	}
	// A sentence that passed stands in for the title the fields of one
	// decision need.
	shared := map[string]any{"title": titles[0]}
	for _, name := range decisionsSharedFields {
		if value, given := fields[name]; given {
			shared[name] = value
		}
	}
	read, ok := s.readDecisionFields(w, shared, tool, person, nil)
	if !ok {
		return
	}
	labels, ok := s.labelsForNewDecisions(w, tool, person, shared)
	if !ok {
		return
	}
	visible, ok := booleanField(w, shared, "visibleToClients")
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	statusID := tool.StatusOfRole("holds").ID
	if read.statusID != nil {
		statusID = *read.statusID
	}
	shownToClients := visible != nil && *visible || s.isAClient(person.ID)
	if !s.subjectKnown(w, tool, read, person) {
		return
	}
	if labelsShareAScope(labels) {
		unprocessable(w, "labelsShareAScope", "Decisions take only one label of each scope.")
		return
	}
	announcement, ok := s.announced(w, tool, shownToClients, statusID, read, person)
	if !ok {
		return
	}

	written := []any{}
	for i, title := range titles {
		decision := &Decision{ID: newID(), CreatedAt: s.now, UpdatedAt: s.now, CreatorID: person.ID,
			ShownToClients: shownToClients, AgendaNumber: i + 1}
		read.applyTo(decision)
		decision.Title, decision.StatusID = title, statusID
		if read.decidedOn == nil && decision.DecidedOn == "" && decision.EventID != "" {
			decision.DecidedOn = s.dayAnEventBrings(decision.EventID, person)
		}
		decision.StatusSetBy = &DecisionStatusSetBy{PersonID: person.ID, At: s.now}
		if announcement != nil {
			decision.Announcements = append(decision.Announcements, *announcement)
		}
		for _, label := range labels {
			if !slices.Contains(s.ItemLabels[decision.ID], label.ID) {
				s.PutLabelOn(decision.ID, label)
			}
		}
		s.numberAndKeep(tool, decision, person)
		written = append(written, s.decisionResource(tool, decision, person))
	}
	writeJSON(w, http.StatusCreated, map[string]any{"decisions": written})
}

// decisionLines reads the sentences of several decisions, trimmed: a list
// of strings, at least one and at most maxDecisionsAtOnce, each refused at
// its place, counted from 0, before anything is written.
func decisionLines(w http.ResponseWriter, value any) ([]string, bool) {
	list, isList := value.([]any)
	titles := []string{}
	for _, entry := range list {
		text, isText := entry.(string)
		if !isText {
			isList = false
			break
		}
		titles = append(titles, phpTrim(text))
	}
	if !isList {
		malformed(w, `The field "titles" has to be a list of strings.`)
		return nil, false
	}
	switch {
	case len(titles) == 0:
		unprocessable(w, "decisionsMissing", `Several decisions need at least one sentence in "titles".`)
		return nil, false
	case len(titles) > maxDecisionsAtOnce:
		unprocessable(w, "decisionsTooMany", fmt.Sprintf("At most %d decisions are written down at once.", maxDecisionsAtOnce))
		return nil, false
	}
	messages := map[string]string{
		"TooLong":              "The sentence of a decision is too long.",
		"HasControlCharacters": "The sentence of a decision must not contain control characters.",
		"Missing":              "A decision needs the sentence that holds.",
	}
	refused := []any{}
	for i, title := range titles {
		if refusal := titleRefusal(title); refusal != "" {
			refused = append(refused, map[string]any{"index": i, "code": "decisionTitle" + refusal, "message": messages[refusal]})
		}
	}
	if len(refused) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": map[string]any{
			"code": "decisionLinesRefused", "message": "Some sentences are refused; nothing was written.", "lines": refused,
		}})
		return nil, false
	}
	return titles, true
}

// labelsForNewDecisions are the labels of the project that "labelIds" names
// by their identifier as Minka keeps it; a client puts on none and meets
// 404, as on one decision.
func (s *Server) labelsForNewDecisions(w http.ResponseWriter, tool *DecisionsTool, person *Person, fields map[string]any) ([]*Label, bool) {
	ids, ok := stringListField(w, fields, "labelIds")
	if !ok || len(ids) == 0 {
		return nil, ok
	}
	if s.isAClient(person.ID) {
		notFound(w)
		return nil, false
	}
	var labels []*Label
	for _, id := range ids {
		var named *Label
		for _, label := range s.labelsOf(tool.ProjectID, "") {
			if label.ID == id {
				named = label
			}
		}
		if named == nil {
			unprocessable(w, "labelUnknown", "The project has no such label.")
			return nil, false
		}
		labels = append(labels, named)
	}
	return labels, true
}

// updateDecision changes what the request names; what it leaves out stays,
// and null or an empty value clears it.
func (s *Server) updateDecision(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool, decision := s.visibleDecision(r.PathValue("decisionId"), person)
	if decision == nil {
		notFound(w)
		return
	}
	read, ok := s.readDecisionFields(w, fields, tool, person, decision)
	if !ok {
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	// The handler finds no decision to change in the archive, which the
	// controller names once the project is known to take writes.
	if decision.Archived {
		unprocessable(w, "decisionArchived", "An archived decision takes no change.")
		return
	}
	// Only a status newly set announces itself; a save that keeps it ignores
	// the audience, which was checked all the same. The handler enrols the
	// people chosen right after the text, before it relates the earlier
	// decision and the subject.
	statusChanged := read.statusID != nil && *read.statusID != decision.StatusID
	var announcement *DecisionAnnouncement
	if statusChanged {
		if announcement, ok = s.announced(w, tool, decision.ShownToClients, *read.statusID, read, person); !ok {
			return
		}
	}
	if !s.subjectAndEarlierKnown(w, tool, decision, read, person, true) {
		return
	}
	before := decision.snapshot("", time.Time{})
	earlierBefore, relationBefore, statusBefore := decision.EarlierID, decision.Relation, decision.StatusID
	read.applyTo(decision)
	if read.decidedOn == nil && decision.DecidedOn == "" && decision.EventID != "" {
		decision.DecidedOn = s.dayAnEventBrings(decision.EventID, person)
	}
	if statusChanged {
		decision.StatusSetBy = &DecisionStatusSetBy{PersonID: person.ID, At: s.now}
	}
	if announcement != nil {
		decision.Announcements = append(decision.Announcements, *announcement)
	}
	after := decision.snapshot(person.ID, s.aMomentAfter(decision.UpdatedAt))
	if !before.sameText(after) {
		decision.Versions = append(decision.Versions, after)
		decision.UpdatedAt = after.WrittenAt
	}
	// Naming the same decision in the other relation relates it anew.
	newlyRelated := decision.EarlierID != "" && (decision.EarlierID != earlierBefore || decision.Relation != relationBefore)
	cameIntoForce := decision.StatusID != statusBefore && s.statusRole(tool, decision.StatusID) == "holds"
	if (newlyRelated || cameIntoForce) && s.statusRole(tool, decision.StatusID) == "holds" {
		s.moveTheEarlier(tool, decision, person)
	}
	writeJSON(w, http.StatusOK, s.decisionResource(tool, decision, person))
}

// removeDecision puts a decision into the trash; a client only one of their
// own.
func (s *Server) removeDecision(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, decision := s.visibleDecision(r.PathValue("decisionId"), person)
	if decision == nil {
		notFound(w)
		return
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return
	}
	if s.isAClient(person.ID) && decision.CreatorID != person.ID {
		notFound(w)
		return
	}
	decision.Trashed = true
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// readDecisionFields reads the fields of a write in the order the controller
// checks them. current is nil for a new decision; for a change, a field left
// out stays nil.
func (s *Server) readDecisionFields(w http.ResponseWriter, fields map[string]any, tool *DecisionsTool, person *Person, current *Decision) (decisionFields, bool) {
	var read decisionFields
	given := func(name string) bool {
		_, present := fields[name]
		return present
	}

	title, ok := stringField(w, fields, "title")
	if !ok {
		return read, false
	}
	if title != nil || current == nil {
		if !validTitle(w, "decision", "a decision", deref(title)) {
			return read, false
		}
		trimmed := phpTrim(deref(title))
		read.title = &trimmed
	}
	if current == nil || given("reason") {
		reason, ok := stringField(w, fields, "reason")
		if !ok {
			return read, false
		}
		value := phpTrim(deref(reason))
		switch {
		case utf8.RuneCountInString(value) > maxDecisionReason:
			unprocessable(w, "decisionReasonTooLong", "The reason of a decision is one sentence; this one is too long.")
			return read, false
		case strings.IndexFunc(value, unicode.IsControl) >= 0:
			unprocessable(w, "decisionReasonHasControlCharacters", "The reason of a decision must not contain control characters.")
			return read, false
		}
		read.reason = &value
	}
	if current == nil || given("body") {
		body, ok := stringField(w, fields, "body")
		if !ok {
			return read, false
		}
		stored, ok := storedRichText(w, "decisionBodyTooLong", "The text of a decision is too long.", maxDecisionBodyLength, deref(body))
		if !ok {
			return read, false
		}
		read.body = &stored
	}
	if current == nil || given("decidedOn") {
		day, ok := s.dayField(w, fields, "decidedOn", "decisionDecidedOnNotADay")
		if !ok {
			return read, false
		}
		if current != nil || day != "" {
			read.decidedOn = &day
		}
	}
	status, ok := stringField(w, fields, "status")
	if !ok {
		return read, false
	}
	if status != nil {
		chosen := s.statusNamed(tool, person, *status)
		if chosen == nil {
			decisionStatusUnknown(w)
			return read, false
		}
		read.statusID = &chosen.ID
	}
	if current == nil || given("decidedBy") {
		deciders, ok := s.decidersField(w, fields, tool, current)
		if !ok {
			return read, false
		}
		read.decidedBy = &deciders
	}
	if current == nil || given("decidedInEventId") {
		eventID, ok := stringField(w, fields, "decidedInEventId")
		if !ok {
			return read, false
		}
		value := strings.ToLower(deref(eventID))
		if value != "" {
			eventTool, event := s.visibleEvent(value, person)
			if event == nil || eventTool.ProjectID != tool.ProjectID {
				unprocessable(w, "decisionEventUnknown", "The event a decision was taken in is one of its project's calendars.")
				return read, false
			}
		}
		read.eventID = &value
	}
	if current == nil || given("votes") {
		votes, ok := stringField(w, fields, "votes")
		if !ok {
			return read, false
		}
		value := phpTrim(deref(votes))
		switch {
		case utf8.RuneCountInString(value) > maxDecisionVotes:
			unprocessable(w, "decisionVotesTooLong", "The votes of a decision are one line of at most 100 characters.")
			return read, false
		case strings.IndexFunc(value, unicode.IsControl) >= 0:
			unprocessable(w, "decisionVotesHasControlCharacters", "The votes of a decision are one line of at most 100 characters.")
			return read, false
		}
		read.votes = &value
	}
	if current == nil || given("supersedes") || given("revokes") {
		supersedes, ok := stringField(w, fields, "supersedes")
		if !ok {
			return read, false
		}
		revokes, ok := stringField(w, fields, "revokes")
		if !ok {
			return read, false
		}
		if supersedes != nil && revokes != nil {
			unprocessable(w, "decisionEarlierTwice", "A decision supersedes or revokes one earlier decision, not both.")
			return read, false
		}
		earlier := &struct{ id, relation string }{}
		switch {
		case deref(supersedes) != "":
			earlier.id, earlier.relation = *supersedes, "supersedes"
		case deref(revokes) != "":
			earlier.id, earlier.relation = *revokes, "revokes"
		}
		if earlier.id != "" && !uuidPattern.MatchString(earlier.id) {
			decisionEarlierUnknown(w)
			return read, false
		}
		earlier.id = strings.ToLower(earlier.id)
		read.earlier = earlier
	}
	if current == nil || given("appliesTo") {
		subject := &struct{ kind, id string }{}
		switch value := fields["appliesTo"].(type) {
		case nil:
		case string:
			if value != "" {
				malformed(w, `The field "appliesTo" has to be {"kind": "event" or "card", "id"} or null.`)
				return read, false
			}
		case map[string]any:
			kind, _ := value["kind"].(string)
			id, _ := value["id"].(string)
			if kind != "event" && kind != "card" || !uuidPattern.MatchString(id) {
				malformed(w, `The field "appliesTo" has to be {"kind": "event" or "card", "id"} or null.`)
				return read, false
			}
			subject.kind, subject.id = kind, strings.ToLower(id)
		default:
			malformed(w, `The field "appliesTo" has to be {"kind": "event" or "card", "id"} or null.`)
			return read, false
		}
		read.appliesTo = subject
	}
	for _, day := range []struct {
		name, code string
		into       **string
	}{{"validUntil", "decisionValidUntilNotADay", &read.validUntil}, {"reviewOn", "decisionReviewOnNotADay", &read.reviewOn}} {
		if current != nil && !given(day.name) {
			continue
		}
		value, ok := s.dayField(w, fields, day.name, day.code)
		if !ok {
			return read, false
		}
		*day.into = &value
	}
	audience, ok := stringField(w, fields, "audience")
	if !ok {
		return read, false
	}
	read.audience = "everybody"
	if audience != nil {
		if !slices.Contains(decisionAudiences, *audience) {
			unprocessable(w, "decisionAudienceUnknown", `A decision reaches "everybody", "chosen" with "notifyIds", or "nobody".`)
			return read, false
		}
		read.audience = *audience
	}
	notifyIDs, ok := stringListField(w, fields, "notifyIds")
	if !ok {
		return read, false
	}
	for _, personID := range notifyIDs {
		if !uuidPattern.MatchString(personID) {
			unprocessable(w, "personNotAProjectMember", "Only a member of the project can be told about a decision.")
			return read, false
		}
		read.notifyIDs = append(read.notifyIDs, strings.ToLower(personID))
	}
	if read.audience == "chosen" && len(read.notifyIDs) == 0 {
		unprocessable(w, "notifyIdsMissing", `An audience of "chosen" names the people in "notifyIds".`)
		return read, false
	}
	if read.audience != "chosen" && len(read.notifyIDs) > 0 {
		unprocessable(w, "notifyIdsWithoutChosenAudience", `Only an audience of "chosen" takes "notifyIds".`)
		return read, false
	}
	return read, true
}

// announced is the line a decision coming into a status sends, nil where the
// status tells nobody: only one of role proposed or holds is announced. The
// people chosen are met in the order given, the writer and anybody met before
// passed over, and the first who cannot be told decides: a client the
// decision is not shown to, else somebody outside the project.
func (s *Server) announced(w http.ResponseWriter, tool *DecisionsTool, shownToClients bool, statusID string, read decisionFields, writer *Person) (*DecisionAnnouncement, bool) {
	role := s.statusRole(tool, statusID)
	if role != "proposed" && role != "holds" {
		return nil, true
	}
	project := s.projectWithID(tool.ProjectID)
	met := []string{writer.ID}
	for _, personID := range read.notifyIDs {
		if slices.Contains(met, personID) {
			continue
		}
		met = append(met, personID)
		if !slices.Contains(project.MemberIDs, personID) {
			unprocessable(w, "personNotAProjectMember", "Only a member of the project can be told about a decision.")
			return nil, false
		}
		if s.isAClient(personID) && !(tool.ShownToClients && shownToClients) {
			unprocessable(w, "personCannotSeeIt", "Somebody chosen is a client of the project, and this decision is not shown to clients.")
			return nil, false
		}
	}
	return &DecisionAnnouncement{Role: role, Audience: read.audience, NotifyIDs: slices.Clone(read.notifyIDs)}, true
}

// dayField reads a day: empty for none, whether left out, null or empty.
func (s *Server) dayField(w http.ResponseWriter, fields map[string]any, name, code string) (string, bool) {
	value, ok := stringField(w, fields, name)
	if !ok {
		return "", false
	}
	if deref(value) == "" {
		return "", true
	}
	if !dayForm.MatchString(*value) {
		unprocessable(w, code, "The field \""+name+"\" is a day such as 2026-11-10.")
		return "", false
	}
	if _, err := time.Parse(time.DateOnly, *value); err != nil {
		unprocessable(w, code, "The field \""+name+"\" is a day such as 2026-11-10.")
		return "", false
	}
	return *value, true
}

// decidersField reads who decided: a list of {"personId"} and {"name"}, a
// person a member of the project or named on the decision before.
func (s *Server) decidersField(w http.ResponseWriter, fields map[string]any, tool *DecisionsTool, current *Decision) ([]Decider, bool) {
	value, present := fields["decidedBy"]
	if !present || value == nil {
		return []Decider{}, true
	}
	list, isList := value.([]any)
	if !isList {
		malformed(w, `The field "decidedBy" has to be a list of {"personId"} and {"name"}.`)
		return nil, false
	}
	known := slices.Clone(s.projectWithID(tool.ProjectID).MemberIDs)
	if current != nil {
		for _, decider := range current.DecidedBy {
			known = append(known, decider.PersonID)
		}
	}
	deciders := []Decider{}
	for _, entry := range list {
		object, _ := entry.(map[string]any)
		personID, hasPerson := object["personId"].(string)
		name, hasName := object["name"].(string)
		if hasPerson == hasName {
			malformed(w, `An entry of "decidedBy" names either a "personId" or a "name".`)
			return nil, false
		}
		if hasPerson {
			personID = strings.ToLower(personID)
			if !uuidPattern.MatchString(personID) || !slices.Contains(known, personID) {
				unprocessable(w, "personNotAProjectMember", "Only a member of the project can be named as having decided.")
				return nil, false
			}
			deciders = appendDistinct(deciders, Decider{PersonID: personID})
			continue
		}
		name = phpTrim(name)
		switch {
		case name == "":
			unprocessable(w, "decisionDeciderNameMissing", "A name of who decided is one line of at most 100 characters.")
			return nil, false
		case utf8.RuneCountInString(name) > maxDeciderName:
			unprocessable(w, "decisionDeciderNameTooLong", "A name of who decided is one line of at most 100 characters.")
			return nil, false
		case strings.IndexFunc(name, unicode.IsControl) >= 0:
			unprocessable(w, "decisionDeciderNameHasControlCharacters", "A name of who decided is one line of at most 100 characters.")
			return nil, false
		}
		deciders = appendDistinct(deciders, Decider{Name: name})
	}
	if len(deciders) > mostDecidersOfADecision {
		unprocessable(w, "decisionDecidedByTooLong", "A decision names at most 50 of who decided.")
		return nil, false
	}
	return deciders, true
}

// appendDistinct adds one of who decided unless the list names them already:
// a person by ID, a name in any case. Minka counts the limit of the list
// after that.
func appendDistinct(deciders []Decider, decider Decider) []Decider {
	for _, named := range deciders {
		if decider.PersonID != "" && named.PersonID == decider.PersonID ||
			decider.PersonID == "" && named.PersonID == "" && strings.ToLower(named.Name) == strings.ToLower(decider.Name) {
			return deciders
		}
	}
	return append(deciders, decider)
}

// subjectAndEarlierKnown checks what the write meets: the subject, an event
// of the project or a card the writer sees, and the earlier decision, an
// active one of the same tool that does not come after this one. A new
// decision meets the subject first, a change the earlier decision, as their
// handlers ask.
func (s *Server) subjectAndEarlierKnown(w http.ResponseWriter, tool *DecisionsTool, decision *Decision, read decisionFields, person *Person, earlierFirst bool) bool {
	if earlierFirst {
		return s.earlierKnown(w, tool, decision, read, person) && s.subjectKnown(w, tool, read, person)
	}
	return s.subjectKnown(w, tool, read, person) && s.earlierKnown(w, tool, decision, read, person)
}

func (s *Server) subjectKnown(w http.ResponseWriter, tool *DecisionsTool, read decisionFields, person *Person) bool {
	if read.appliesTo != nil && read.appliesTo.id != "" {
		known := false
		if read.appliesTo.kind == "event" {
			eventTool, event := s.visibleEvent(read.appliesTo.id, person)
			known = event != nil && eventTool.ProjectID == tool.ProjectID
		} else {
			_, _, card := s.visibleCard(read.appliesTo.id, person)
			known = card != nil
		}
		if !known {
			unprocessable(w, "decisionAppliesToUnknown", "A decision applies to an event of its project or a card the writer sees.")
			return false
		}
	}
	return true
}

func (s *Server) earlierKnown(w http.ResponseWriter, tool *DecisionsTool, decision *Decision, read decisionFields, person *Person) bool {
	if read.earlier != nil && read.earlier.id != "" {
		earlier := s.decisionOfTool(tool, read.earlier.id)
		if earlier == nil || earlier == decision || earlier.Archived || !s.seesDecision(tool, earlier, person) || s.comesAfter(tool, earlier, decision) {
			decisionEarlierUnknown(w)
			return false
		}
	}
	return true
}

// comesAfter says whether the chain of earlier decisions of a leads to b.
func (s *Server) comesAfter(tool *DecisionsTool, a, b *Decision) bool {
	for seen := 0; a != nil && seen <= len(tool.Decisions); seen++ {
		if a.EarlierID == "" {
			return false
		}
		if a.EarlierID == b.ID {
			return true
		}
		a = s.decisionOfTool(tool, a.EarlierID)
	}
	return false
}

func (read decisionFields) applyTo(decision *Decision) {
	set := func(into *string, value *string) {
		if value != nil {
			*into = *value
		}
	}
	set(&decision.Title, read.title)
	set(&decision.Reason, read.reason)
	set(&decision.Body, read.body)
	set(&decision.DecidedOn, read.decidedOn)
	set(&decision.StatusID, read.statusID)
	set(&decision.EventID, read.eventID)
	set(&decision.Votes, read.votes)
	set(&decision.ValidUntil, read.validUntil)
	set(&decision.ReviewOn, read.reviewOn)
	if read.decidedBy != nil {
		decision.DecidedBy = *read.decidedBy
	}
	if read.earlier != nil {
		decision.EarlierID, decision.Relation = read.earlier.id, read.earlier.relation
	}
	if read.appliesTo != nil {
		decision.AppliesToKind, decision.AppliesToID = read.appliesTo.kind, read.appliesTo.id
	}
}

// moveTheEarlier puts the earlier decision a decision in force names into
// the status of the relation's role, unless it went into the archive since;
// the writer of the later one is who set it.
func (s *Server) moveTheEarlier(tool *DecisionsTool, decision *Decision, writer *Person) {
	// Only a status of role holds moves it: a decision from before its tool
	// had statuses holds without one, and moves nothing.
	if decision.StatusID == "" {
		return
	}
	earlier := s.decisionOfTool(tool, decision.EarlierID)
	if earlier == nil || earlier.Archived {
		return
	}
	role := map[string]string{"supersedes": "superseded", "revokes": "revoked"}[decision.Relation]
	if status := tool.StatusOfRole(role); status != nil && earlier.StatusID != status.ID {
		earlier.StatusID = status.ID
		earlier.StatusSetBy = &DecisionStatusSetBy{PersonID: writer.ID, At: s.now}
	}
}

func (decision *Decision) snapshot(authorID string, at time.Time) *DecisionVersion {
	return &DecisionVersion{ID: newID(), Title: decision.Title, Reason: decision.Reason, Body: decision.Body,
		DecidedOn: decision.DecidedOn, DecidedBy: slices.Clone(decision.DecidedBy), EventID: decision.EventID,
		Votes: decision.Votes, AuthorID: authorID, WrittenAt: at}
}

// sameText says whether two versions say the same; a version is kept only
// of what differs.
func (v *DecisionVersion) sameText(other *DecisionVersion) bool {
	return v.Title == other.Title && v.Reason == other.Reason && v.Body == other.Body && v.DecidedOn == other.DecidedOn &&
		slices.Equal(v.DecidedBy, other.DecidedBy) && v.EventID == other.EventID && v.Votes == other.Votes
}

// dayAnEventBrings is the day of an event that does not repeat, in the zone
// of the reader; a series brings none.
func (s *Server) dayAnEventBrings(eventID string, person *Person) string {
	_, event := s.visibleEvent(eventID, person)
	if event == nil || event.Repetition != nil {
		return ""
	}
	return s.firstDayOf(event, person)
}

func (s *Server) firstDayOf(event *CalendarEvent, person *Person) string {
	if event.FirstDay != "" {
		return event.FirstDay
	}
	_, zone := s.zoneOf(person)
	return event.StartsAt.In(zone).Format(time.DateOnly)
}

func (s *Server) decisionVersions(w http.ResponseWriter, r *http.Request, person *Person) {
	_, decision := s.visibleDecision(r.PathValue("decisionId"), person)
	if decision == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	lines := []any{}
	for number, version := range decision.Versions {
		lines = append(lines, map[string]any{
			"id": version.ID, "decisionId": decision.ID, "number": number + 1, "current": number == len(decision.Versions)-1,
			"author": s.personReferenceOrNil(version.AuthorID), "writtenAt": timestamp(version.WrittenAt),
		})
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Server) decisionVersion(w http.ResponseWriter, r *http.Request, person *Person) {
	tool, decision := s.visibleDecision(r.PathValue("decisionId"), person)
	if decision == nil || s.isAClient(person.ID) {
		notFound(w)
		return
	}
	for number, version := range decision.Versions {
		if version.ID != r.PathValue("versionId") {
			continue
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id": version.ID, "decisionId": decision.ID, "number": number + 1, "current": number == len(decision.Versions)-1,
			"title": version.Title, "reason": nullable(version.Reason), "body": version.Body,
			"attachments": s.attachmentsIn(tool.ProjectID, version.Body), "decidedOn": nullable(version.DecidedOn),
			"author": s.personReferenceOrNil(version.AuthorID), "writtenAt": timestamp(version.WrittenAt),
			"appUrl":    s.decisionURL(tool, decision) + "/versions/" + version.ID,
			"decidedBy": s.deciderResources(version.DecidedBy), "decidedIn": s.eventReferenceOf(tool, version.EventID, person),
			"votes": nullable(version.Votes),
		})
		return
	}
	notFound(w)
}

func (s *Server) addDecisionHistoryEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	s.addDecisionEntry(w, r, person, false)
}

func (s *Server) addDecisionConsequence(w http.ResponseWriter, r *http.Request, person *Person) {
	s.addDecisionEntry(w, r, person, true)
}

// cardAddress, todoAddress, postAddress and decisionAddress are the pages of
// the fake an entry may name, ending in the identifier of what they show.
var (
	decisionAddress = regexp.MustCompile(`^/projects/[0-9a-f-]{36}/decisions/[0-9a-f-]{36}/([0-9a-f-]{36})/?$`)
	cardAddress     = regexp.MustCompile(`^/projects/[0-9a-f-]{36}/boards/[0-9a-f-]{36}/cards/([0-9a-f-]{36})$`)
	todoAddress     = regexp.MustCompile(`^/projects/[0-9a-f-]{36}/todos/[0-9a-f-]{36}/todos/([0-9a-f-]{36})$`)
	postAddress     = regexp.MustCompile(`^/projects/[0-9a-f-]{36}/posts/[0-9a-f-]{36}/([0-9a-f-]{36})$`)
)

// addDecisionEntry appends what an address names to the history or the
// consequences. The fake reads the pages of a card, a to-do and a post, a
// "#" card name and, for the history, a decision by its "#" name or its short
// address and a web address with its description; naming the same again
// answers with the entry there.
func (s *Server) addDecisionEntry(w http.ResponseWriter, r *http.Request, person *Person, consequence bool) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	_, decision, ok := s.decisionToChange(w, r, person)
	if !ok {
		return
	}
	address, ok := stringField(w, fields, "address")
	if !ok {
		return
	}
	description, ok := stringField(w, fields, "description")
	if !ok {
		return
	}
	entry, ok := s.entryNamed(w, phpTrim(deref(address)), phpTrim(deref(description)), consequence, person)
	if !ok {
		return
	}
	part := &decision.History
	if consequence {
		part = &decision.Consequences
	}
	for _, there := range *part {
		if there.Kind == entry.Kind && there.TargetID == entry.TargetID && there.URL == entry.URL {
			writeJSON(w, http.StatusCreated, s.decisionEntryResource(there, consequence, person))
			return
		}
	}
	*part = append(*part, entry)
	writeJSON(w, http.StatusCreated, s.decisionEntryResource(entry, consequence, person))
}

func (s *Server) entryNamed(w http.ResponseWriter, address, description string, consequence bool, person *Person) (*DecisionEntry, bool) {
	targetUnknown := func() (*DecisionEntry, bool) {
		unprocessable(w, "entryTargetUnknown", "The address names nothing this person sees, or nothing this part of the decision takes: a consequence is a to-do or a card.")
		return nil, false
	}
	if strings.HasPrefix(address, "#") {
		for _, board := range s.Boards {
			for _, column := range board.Columns {
				for _, card := range column.Cards {
					if card.ShortID != "" && strings.EqualFold(card.ShortID, address[1:]) && s.visibleProject(board.ProjectID, person) != nil {
						return &DecisionEntry{ID: newID(), Kind: "card", TargetID: card.ID}, true
					}
				}
			}
		}
		if _, decision := s.decisionGoingBy(address[1:], person); decision != nil && !consequence {
			return &DecisionEntry{ID: newID(), Kind: "decision", TargetID: decision.ID}, true
		}
		return targetUnknown()
	}
	parsed, err := url.Parse(address)
	if err != nil || address == "" || parsed.Scheme == "" && !strings.HasPrefix(address, "/") {
		unprocessable(w, "entryAddressMalformed", `An entry is an address of this instance, a "#" card name or a web address.`)
		return nil, false
	}
	own, _ := url.Parse(s.URL)
	if parsed.Scheme != "" && parsed.Host != own.Host {
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			unprocessable(w, "entryAddressMalformed", `An entry is an address of this instance, a "#" card name or a web address.`)
			return nil, false
		}
		if consequence {
			return targetUnknown()
		}
		switch {
		case description == "":
			unprocessable(w, "linkDescriptionMissing", "A link outside Minka needs a few words that describe it.")
			return nil, false
		case utf8.RuneCountInString(description) > maxLinkDescription || strings.IndexFunc(description, unicode.IsControl) >= 0:
			unprocessable(w, "linkDescriptionTooLong", "The description of a link is at most 200 characters, without control characters.")
			return nil, false
		}
		return &DecisionEntry{ID: newID(), Kind: "link", URL: address, Title: description}, true
	}
	if identifier, isShort := strings.CutPrefix(strings.TrimSuffix(parsed.Path, "/"), "/go/decision/"); isShort && !consequence {
		if _, decision := s.decisionGoingBy(identifier, person); decision != nil {
			return &DecisionEntry{ID: newID(), Kind: "decision", TargetID: decision.ID}, true
		}
	}
	path := strings.ToLower(parsed.Path)
	if match := cardAddress.FindStringSubmatch(path); match != nil {
		if _, _, card := s.visibleCard(match[1], person); card != nil {
			return &DecisionEntry{ID: newID(), Kind: "card", TargetID: card.ID}, true
		}
	}
	if match := todoAddress.FindStringSubmatch(path); match != nil {
		if _, _, todo := s.visibleTodo(match[1], person); todo != nil {
			return &DecisionEntry{ID: newID(), Kind: "todo", TargetID: todo.ID}, true
		}
	}
	if match := decisionAddress.FindStringSubmatch(path); match != nil && !consequence {
		if _, decision := s.visibleDecision(match[1], person); decision != nil {
			return &DecisionEntry{ID: newID(), Kind: "decision", TargetID: decision.ID}, true
		}
	}
	if match := postAddress.FindStringSubmatch(path); match != nil && !consequence {
		if _, post := s.visiblePost(match[1], person); post != nil {
			return &DecisionEntry{ID: newID(), Kind: "post", TargetID: post.ID}, true
		}
	}
	return targetUnknown()
}

// moveDecisionEntry puts an entry between two others of its part: behind
// previousEntryId, else in front of nextEntryId, else on top.
func (s *Server) moveDecisionEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	tool, decision, ok := s.decisionToChange(w, r, person)
	if !ok {
		return
	}
	part, moved := decision.entryWithID(r.PathValue("entryId"))
	if moved == nil {
		notFound(w)
		return
	}
	neighbours := map[string]*DecisionEntry{}
	for _, name := range []string{"previousEntryId", "nextEntryId"} {
		value, ok := stringField(w, fields, name)
		if !ok {
			return
		}
		if deref(value) == "" {
			continue
		}
		_, neighbour := decision.entryWithID(strings.ToLower(*value))
		if neighbour == nil || neighbour == moved {
			unprocessable(w, "entryPlaceUnknown", "The neighbours named are no other entries of this decision.")
			return
		}
		neighbours[name] = neighbour
	}
	rest := slices.DeleteFunc(slices.Clone(*part), func(entry *DecisionEntry) bool { return entry == moved })
	at := 0
	if previous := neighbours["previousEntryId"]; previous != nil {
		at = slices.Index(rest, previous) + 1
	} else if next := neighbours["nextEntryId"]; next != nil {
		at = max(slices.Index(rest, next), 0)
	}
	*part = slices.Insert(rest, at, moved)
	writeJSON(w, http.StatusOK, s.decisionResource(tool, decision, person))
}

func (s *Server) removeDecisionEntry(w http.ResponseWriter, r *http.Request, person *Person) {
	_, decision, ok := s.decisionToChange(w, r, person)
	if !ok {
		return
	}
	part, removed := decision.entryWithID(r.PathValue("entryId"))
	if removed == nil {
		notFound(w)
		return
	}
	*part = slices.DeleteFunc(*part, func(entry *DecisionEntry) bool { return entry == removed })
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusNoContent)
}

// decisionToChange finds a decision whose entries change: neither it nor
// its project in the archive.
func (s *Server) decisionToChange(w http.ResponseWriter, r *http.Request, person *Person) (*DecisionsTool, *Decision, bool) {
	tool, decision := s.visibleDecision(r.PathValue("decisionId"), person)
	if decision == nil {
		notFound(w)
		return nil, nil, false
	}
	if s.refusedAsArchived(w, s.projectWithID(tool.ProjectID)) {
		return nil, nil, false
	}
	if decision.Archived {
		unprocessable(w, "decisionArchived", "An archived decision takes no change.")
		return nil, nil, false
	}
	return tool, decision, true
}

func (decision *Decision) entryWithID(id string) (*[]*DecisionEntry, *DecisionEntry) {
	for _, part := range []*[]*DecisionEntry{&decision.History, &decision.Consequences} {
		for _, entry := range *part {
			if entry.ID == id {
				return part, entry
			}
		}
	}
	return nil, nil
}

// decisionEntryResource is an entry as the API shows it, with what it names now.
func (s *Server) decisionEntryResource(entry *DecisionEntry, consequence bool, person *Person) map[string]any {
	resource := map[string]any{"id": entry.ID, "kind": entry.Kind, "title": entry.Title, "targetId": nullable(entry.TargetID),
		"commentId": nil, "projectId": nil, "appUrl": entry.URL, "done": nil, "shortId": nil}
	switch entry.Kind {
	case "card":
		if board, column, card := s.visibleCard(entry.TargetID, person); card != nil {
			resource["title"], resource["projectId"], resource["appUrl"] = card.Title, board.ProjectID, s.cardURL(board, card)
			resource["shortId"] = nullable(card.ShortID)
			if consequence {
				resource["done"] = column.Type == "done"
			}
		}
	case "decision":
		if tool, decision := s.visibleDecision(entry.TargetID, person); decision != nil {
			resource["title"], resource["projectId"], resource["appUrl"] = decision.Title, tool.ProjectID, s.decisionURL(tool, decision)
			resource["shortId"] = nullable(decision.ShortID)
		}
	case "todo":
		if tool, _, todo := s.visibleTodo(entry.TargetID, person); todo != nil {
			resource["title"], resource["projectId"], resource["appUrl"] = todo.Title, tool.ProjectID, s.todoURL(tool, todo)
			if consequence {
				resource["done"] = todo.CompletedAt != nil
			}
		}
	case "post":
		if tool, post := s.visiblePost(entry.TargetID, person); post != nil {
			resource["title"], resource["projectId"], resource["appUrl"] = post.Title, tool.ProjectID, s.postURL(tool, post)
		}
	}
	return resource
}

func (s *Server) decisionEntryResources(entries []*DecisionEntry, consequence bool, person *Person) ([]any, int) {
	list := []any{}
	done := 0
	for _, entry := range entries {
		resource := s.decisionEntryResource(entry, consequence, person)
		if resource["projectId"] == nil && entry.Kind != "link" {
			continue
		}
		if resource["done"] == true {
			done++
		}
		list = append(list, resource)
	}
	return list, done
}

// statusesOfferedTo are the statuses of a tool a person may name: a client
// is offered no draft.
func (s *Server) statusesOfferedTo(tool *DecisionsTool, person *Person) []*DecisionStatus {
	var offered []*DecisionStatus
	for _, status := range tool.Statuses {
		if status.Role == "draft" && s.isAClient(person.ID) {
			continue
		}
		offered = append(offered, status)
	}
	return offered
}

// statusNamed finds a status by its identifier or its role, either in any
// case.
func (s *Server) statusNamed(tool *DecisionsTool, person *Person, value string) *DecisionStatus {
	named := strings.ToLower(value)
	for _, status := range s.statusesOfferedTo(tool, person) {
		if status.ID == named || status.Role != "" && status.Role == named {
			return status
		}
	}
	return nil
}

// statusIDOf is the status a decision is filtered by: one from before its
// tool had statuses counts as the one of role holds.
func (s *Server) statusIDOf(tool *DecisionsTool, decision *Decision) string {
	if decision.StatusID == "" {
		if holds := tool.StatusOfRole("holds"); holds != nil {
			return holds.ID
		}
	}
	return decision.StatusID
}

func (s *Server) statusRole(tool *DecisionsTool, statusID string) string {
	if statusID == "" {
		return "holds"
	}
	for _, status := range tool.Statuses {
		if status.ID == statusID {
			return status.Role
		}
	}
	return ""
}

// statusResourceOf is the status a decision stands in; a decision without
// one holds.
func (s *Server) statusResourceOf(tool *DecisionsTool, statusID string) map[string]any {
	for _, status := range tool.Statuses {
		if status.ID == statusID {
			return statusResource(status)
		}
	}
	return map[string]any{"id": nil, "name": "In force", "customName": nil, "role": "holds"}
}

func statusResource(status *DecisionStatus) map[string]any {
	name := status.Name
	if name == "" {
		for _, role := range decisionRoles {
			if role.role == status.Role {
				name = role.name
			}
		}
	}
	return map[string]any{"id": status.ID, "name": name, "customName": nullable(status.Name), "role": nullable(status.Role)}
}

func decisionStatusUnknown(w http.ResponseWriter) {
	unprocessable(w, "decisionStatusUnknown", `The status is one of the tool's, named by its identifier or by its role, such as "holds".`)
}

func decisionEarlierUnknown(w http.ResponseWriter) {
	unprocessable(w, "decisionEarlierUnknown", "The earlier decision is an active decision of the same tool, and none that comes after this one.")
}

// visibleDecisionsTool finds a tool in a project the person is a member of,
// and for a client only a tool the clients are let into.
func (s *Server) visibleDecisionsTool(toolID string, person *Person) *DecisionsTool {
	for _, tool := range s.DecisionsTools {
		if tool.ID == toolID && s.visibleProject(tool.ProjectID, person) != nil && (tool.ShownToClients || !s.isAClient(person.ID)) {
			return tool
		}
	}
	return nil
}

// seesDecision says whether a person is shown a decision out of the trash:
// a client only one shown to clients and never a draft.
func (s *Server) seesDecision(tool *DecisionsTool, decision *Decision, person *Person) bool {
	if decision.Trashed {
		return false
	}
	return !s.isAClient(person.ID) || decision.ShownToClients && s.statusRole(tool, decision.StatusID) != "draft"
}

// visibleDecision finds a decision by its identifier alone, in the list or
// in the archive.
func (s *Server) visibleDecision(decisionID string, person *Person) (*DecisionsTool, *Decision) {
	if !uuidPattern.MatchString(decisionID) {
		return nil, nil
	}
	for _, tool := range s.DecisionsTools {
		for _, decision := range tool.Decisions {
			if decision.ID == decisionID {
				if s.visibleDecisionsTool(tool.ID, person) == nil || !s.seesDecision(tool, decision, person) {
					return nil, nil
				}
				return tool, decision
			}
		}
	}
	return nil, nil
}

func (s *Server) decisionOfTool(tool *DecisionsTool, decisionID string) *Decision {
	for _, decision := range tool.Decisions {
		if decision.ID == decisionID && !decision.Trashed {
			return decision
		}
	}
	return nil
}

func (s *Server) deciderResources(deciders []Decider) []any {
	list := []any{}
	for _, decider := range deciders {
		if decider.PersonID != "" {
			list = append(list, map[string]any{"person": s.personReference(decider.PersonID), "name": nil})
		} else {
			list = append(list, map[string]any{"person": nil, "name": decider.Name})
		}
	}
	return list
}

// eventReferenceOf is the event a decision was taken in, and null also where
// the reader does not see it.
func (s *Server) eventReferenceOf(tool *DecisionsTool, eventID string, person *Person) any {
	if eventID == "" {
		return nil
	}
	calendar, event := s.visibleEvent(eventID, person)
	if event == nil {
		return nil
	}
	return map[string]any{"id": event.ID, "calendarId": calendar.ID, "title": event.Title,
		"startsOn": s.firstDayOf(event, person), "appUrl": s.eventURL(calendar, event)}
}

func (s *Server) subjectReferenceOf(decision *Decision, person *Person) any {
	switch decision.AppliesToKind {
	case "event":
		if calendar, event := s.visibleEvent(decision.AppliesToID, person); event != nil {
			return map[string]any{"kind": "event", "id": event.ID, "projectId": calendar.ProjectID, "toolId": calendar.ID,
				"title": event.Title, "inTrash": false, "appUrl": s.eventURL(calendar, event)}
		}
	case "card":
		if board, _, card := s.visibleCard(decision.AppliesToID, person); card != nil {
			return map[string]any{"kind": "card", "id": card.ID, "projectId": board.ProjectID, "toolId": board.ID,
				"title": card.Title, "inTrash": false, "appUrl": s.cardURL(board, card)}
		}
	}
	return nil
}

// linkResource is another decision of the same tool as a decision names it.
func (s *Server) linkResource(tool *DecisionsTool, decision *Decision) map[string]any {
	return map[string]any{"id": decision.ID, "title": decision.Title, "status": s.statusResourceOf(tool, decision.StatusID),
		"archived": decision.Archived, "appUrl": s.decisionURL(tool, decision), "shortId": nullable(decision.ShortID)}
}

// statusSetByResource is who set the status of a decision, null where
// nobody is known; "agent" is an agent that set it by hand.
func (s *Server) statusSetByResource(decision *Decision) any {
	setBy := decision.StatusSetBy
	if setBy == nil {
		return nil
	}
	var person any
	if s.People[setBy.PersonID] != nil {
		person = s.personReference(setBy.PersonID)
	}
	return map[string]any{"person": person, "agent": s.setByAnAgent(decision), "minka": setBy.Minka, "at": timestamp(setBy.At)}
}

// shortDecisionURL is the go/decision address of a decision, null before its
// tool numbered it.
func (s *Server) shortDecisionURL(decision *Decision) any {
	if decision.ShortID == "" {
		return nil
	}
	return s.URL + "/go/decision/" + decision.ShortID
}

func (s *Server) decisionListItemResource(tool *DecisionsTool, decision *Decision) map[string]any {
	return map[string]any{
		"id": decision.ID, "projectId": tool.ProjectID, "title": decision.Title, "reason": nullable(decision.Reason),
		"status": s.statusResourceOf(tool, decision.StatusID), "decidedOn": nullable(decision.DecidedOn),
		"labels": s.labelResources(s.LabelsOn(decision.ID)), "commentCount": len(decision.Comments),
		"createdAt": timestamp(decision.CreatedAt), "creator": s.personReference(decision.CreatorID),
		"appUrl": s.decisionURL(tool, decision), "decidedBy": s.deciderResources(decision.DecidedBy),
		"validUntil": nullable(decision.ValidUntil), "reviewOn": nullable(decision.ReviewOn),
		"statusSetBy": s.statusSetByResource(decision), "shortId": nullable(decision.ShortID), "shortUrl": s.shortDecisionURL(decision),
		"agendaNumber": agendaNumberOf(decision),
	}
}

func (s *Server) decisionResource(tool *DecisionsTool, decision *Decision, person *Person) map[string]any {
	var supersedes, revokes any
	if earlier := s.decisionOfTool(tool, decision.EarlierID); earlier != nil && s.seesDecision(tool, earlier, person) {
		if decision.Relation == "supersedes" {
			supersedes = s.linkResource(tool, earlier)
		} else {
			revokes = s.linkResource(tool, earlier)
		}
	}
	supersededBy, revokedBy := []any{}, []any{}
	later := slices.Clone(tool.Decisions)
	slices.SortFunc(later, func(a, b *Decision) int { return b.CreatedAt.Compare(a.CreatedAt) })
	for _, other := range later {
		if other.EarlierID != decision.ID || !s.seesDecision(tool, other, person) {
			continue
		}
		if other.Relation == "supersedes" {
			supersededBy = append(supersededBy, s.linkResource(tool, other))
		} else {
			revokedBy = append(revokedBy, s.linkResource(tool, other))
		}
	}
	history, _ := s.decisionEntryResources(decision.History, false, person)
	consequences, done := s.decisionEntryResources(decision.Consequences, true, person)
	return map[string]any{
		"id": decision.ID, "projectId": tool.ProjectID, "toolId": tool.ID,
		"project": s.projectReference(tool.ProjectID), "tool": map[string]any{"id": tool.ID, "title": tool.Title},
		"title": decision.Title, "reason": nullable(decision.Reason), "status": s.statusResourceOf(tool, decision.StatusID),
		"body": decision.Body, "bodySource": decision.Body, "attachments": s.attachmentsIn(tool.ProjectID, decision.Body),
		"decidedOn": nullable(decision.DecidedOn), "labels": s.labelResources(s.LabelsOn(decision.ID)),
		"commentCount": len(decision.Comments), "createdAt": timestamp(decision.CreatedAt),
		"editedAt": editedAt(decision.CreatedAt, decision.UpdatedAt), "creator": s.personReference(decision.CreatorID),
		"comments": s.commentResources(tool.ProjectID, s.decisionURL(tool, decision), decision.Comments),
		"appUrl":   s.decisionURL(tool, decision), "archived": decision.Archived,
		"decidedBy": s.deciderResources(decision.DecidedBy), "decidedIn": s.eventReferenceOf(tool, decision.EventID, person),
		"votes": nullable(decision.Votes), "supersedes": supersedes, "revokes": revokes,
		"supersededBy": supersededBy, "revokedBy": revokedBy, "history": history, "consequences": consequences,
		"consequencesDone": done, "appliesTo": s.subjectReferenceOf(decision, person),
		"validUntil": nullable(decision.ValidUntil), "reviewOn": nullable(decision.ReviewOn),
		"statusSetBy": s.statusSetByResource(decision), "shortId": nullable(decision.ShortID), "shortUrl": s.shortDecisionURL(decision),
		"agendaNumber": agendaNumberOf(decision),
	}
}

func (s *Server) decisionsToolURL(tool *DecisionsTool) string {
	return s.URL + "/projects/" + tool.ProjectID + "/decisions/" + tool.ID
}

func (s *Server) decisionURL(tool *DecisionsTool, decision *Decision) string {
	return s.decisionsToolURL(tool) + "/" + decision.ID
}

// agendaNumberOf is the place among the decisions written down with it, null
// for a decision written down alone.
func agendaNumberOf(decision *Decision) any {
	if decision.AgendaNumber == 0 {
		return nil
	}
	return decision.AgendaNumber
}
