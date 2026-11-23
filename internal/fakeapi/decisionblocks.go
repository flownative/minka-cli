package fakeapi

import (
	"cmp"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The decisions blocks of a document or a post: a stored query whose rows are
// read with every answer, and which a write checks against the project.
const (
	decisionsElement          = "minka-decisions"
	maxDecisionBlocksPerText  = 20
	decisionBlockSummaryTitle = "List of decisions"
	decisionBlockHidden       = "not shown to you"
)

// decisionBlockAttributes are the attributes Minka keeps on the element, the
// tool first.
var decisionBlockAttributes = []string{"tool", "label", "status", "decided-in", "applies-to"}

var (
	decisionsElementForm   = regexp.MustCompile(`(?is)<minka-decisions\b([^>]*)>(.*?)</minka-decisions\s*>`)
	decisionsAttributeForm = regexp.MustCompile(`([A-Za-z][A-Za-z0-9-]*)\s*=\s*"([^"]*)"`)
)

// DocumentLock is a locked document: its title and body take no write, and
// its decisions blocks show the rows copied at the lock, by query.
type DocumentLock struct {
	Rows map[string][]FrozenDecision
}

// FrozenDecision is a decision as a locked document copied it.
type FrozenDecision struct {
	DecisionID   string
	ToolID       string
	ShortID      string
	Title        string
	StatusID     string
	StatusRole   string
	VotesFor     *int
	VotesAgainst *int
	Abstentions  *int
	VotesNote    string
}

// decisionBlockQuery is what one element asks for; an empty filter is none.
type decisionBlockQuery struct {
	tool, label, status, decidedIn, appliesTo string
}

func (q decisionBlockQuery) key() string {
	return strings.Join([]string{q.tool, q.label, q.status, q.decidedIn, q.appliesTo}, "/")
}

// decisionsElements are the elements of a text in their order, each with
// its attributes as written.
func decisionsElements(text string) []map[string]string {
	if !strings.Contains(strings.ToLower(text), decisionsElement) {
		return nil
	}
	context := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(text), context)
	if err != nil {
		return nil
	}
	var found []map[string]string
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == decisionsElement {
			attributes := map[string]string{}
			for _, attr := range n.Attr {
				attributes[attr.Key] = attr.Val
			}
			found = append(found, attributes)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	for _, n := range nodes {
		visit(n)
	}
	return found
}

// queryOf is the query a stored element asks: nil without a readable tool, a
// filter that is no identifier left out.
func queryOf(attributes map[string]string) *decisionBlockQuery {
	value := func(name string) string {
		if uuidPattern.MatchString(attributes[name]) {
			return attributes[name]
		}
		return ""
	}
	query := &decisionBlockQuery{tool: value("tool"), label: value("label"), status: value("status"), decidedIn: value("decided-in"), appliesTo: value("applies-to")}
	if query.tool == "" {
		return nil
	}
	return query
}

// keptDecisionBlocks is a text as Minka keeps its decisions blocks: the five
// attributes with an identifier each, nothing inside, and an element without
// a tool unwrapped to its words.
func keptDecisionBlocks(text string) string {
	if !strings.Contains(strings.ToLower(text), decisionsElement) {
		return text
	}
	return decisionsElementForm.ReplaceAllStringFunc(text, func(element string) string {
		match := decisionsElementForm.FindStringSubmatch(element)
		attributes := map[string]string{}
		var order []string
		for _, attribute := range decisionsAttributeForm.FindAllStringSubmatch(match[1], -1) {
			name := strings.ToLower(attribute[1])
			if _, seen := attributes[name]; seen || !slices.Contains(decisionBlockAttributes, name) {
				continue
			}
			attributes[name] = html.UnescapeString(attribute[2])
			order = append(order, name)
		}
		if !uuidPattern.MatchString(attributes["tool"]) {
			return match[2]
		}
		kept := "<" + decisionsElement
		for _, name := range order {
			if uuidPattern.MatchString(attributes[name]) {
				kept += " " + name + `="` + attributes[name] + `"`
			}
		}
		return kept + "></" + decisionsElement + ">"
	})
}

// decisionBlockRefusal is why a sent text's blocks cannot be stored, or
// nothing. A block the stored text holds already passes unchecked.
func (s *Server) decisionBlockRefusal(projectID, sent, stored string, person *Person) string {
	elements := decisionsElements(sent)
	if len(elements) == 0 {
		return ""
	}
	if len(elements) > maxDecisionBlocksPerText {
		return fmt.Sprintf("A text holds at most %d decisions blocks.", maxDecisionBlocksPerText)
	}
	passed := map[string]bool{}
	for _, attributes := range decisionsElements(stored) {
		if query := queryOf(attributes); query != nil {
			passed[query.key()] = true
		}
	}
	for _, attributes := range elements {
		for _, name := range decisionBlockAttributes {
			if value := strings.TrimSpace(attributes[name]); value != "" && !uuidPattern.MatchString(value) {
				return fmt.Sprintf("The %q of a decisions block has to be an identifier.", name)
			}
		}
		if strings.TrimSpace(attributes["tool"]) == "" {
			return `A decisions block names its Decisions tool in "tool".`
		}
		query := queryOf(attributes)
		if query == nil || passed[query.key()] {
			continue
		}
		if refusal := s.decisionQueryRefusal(projectID, *query, person); refusal != "" {
			return refusal
		}
		passed[query.key()] = true
	}
	return ""
}

func (s *Server) decisionQueryRefusal(projectID string, query decisionBlockQuery, person *Person) string {
	tool := s.visibleDecisionsTool(query.tool, person)
	if tool == nil || tool.ProjectID != projectID {
		return `The "tool" of a decisions block has to be a Decisions tool of this project.`
	}
	if query.label != "" && s.labelOfBlock(projectID, tool.ID, query.label) == nil {
		return `The "label" of a decisions block has to be a label of this project or of its tool.`
	}
	if query.status != "" && !slices.ContainsFunc(tool.Statuses, func(status *DecisionStatus) bool { return status.ID == query.status }) {
		return `The "status" of a decisions block has to be a status of its tool.`
	}
	if query.decidedIn != "" && s.eventOfBlock(projectID, query.decidedIn, person) == nil {
		return `The "decided-in" of a decisions block has to be an event of this project.`
	}
	if query.appliesTo != "" && s.subjectTitleOfBlock(projectID, query.appliesTo, person) == "" {
		return `The "applies-to" of a decisions block has to be an event or a card of this project.`
	}
	return ""
}

func (s *Server) labelOfBlock(projectID, toolID, labelID string) *Label {
	for _, label := range s.Labels {
		if label.ID == labelID && label.ProjectID == projectID && (label.ToolID == "" || label.ToolID == toolID) {
			return label
		}
	}
	return nil
}

func (s *Server) eventOfBlock(projectID, eventID string, person *Person) *CalendarEvent {
	tool, event := s.visibleEvent(eventID, person)
	if event == nil || tool.ProjectID != projectID {
		return nil
	}
	return event
}

// subjectTitleOfBlock is the title of the event or card a block's decisions
// apply to, empty for one that is no such thing of the project.
func (s *Server) subjectTitleOfBlock(projectID, itemID string, person *Person) string {
	if event := s.eventOfBlock(projectID, itemID, person); event != nil {
		return event.Title
	}
	if board, _, card := s.visibleCard(itemID, person); card != nil && board.ProjectID == projectID {
		return card.Title
	}
	return ""
}

// decisionBlockResources are the blocks of a text as its reader sees them,
// one per element in its order; position counts every element.
func (s *Server) decisionBlockResources(projectID, text string, lock *DocumentLock, person *Person) []any {
	list := []any{}
	for position, attributes := range decisionsElements(text) {
		query := queryOf(attributes)
		if query == nil {
			continue
		}
		tool := s.visibleDecisionsTool(query.tool, person)
		if tool != nil && tool.ProjectID != projectID {
			tool = nil
		}
		var toolReference any
		decisions := []any{}
		if tool != nil {
			toolReference = map[string]any{"id": tool.ID, "title": tool.Title}
			if lock != nil {
				decisions = s.frozenDecisionResources(tool, lock.Rows[query.key()], person)
			} else {
				for _, decision := range s.decisionsOfBlock(tool, *query, person) {
					decisions = append(decisions, s.decisionOfBlockResource(tool, decision))
				}
			}
		}
		summary := s.decisionBlockSummary(projectID, tool, *query, person)
		list = append(list, map[string]any{
			"position": position, "toolId": query.tool,
			"labelId": nullable(query.label), "statusId": nullable(query.status),
			"decidedInId": nullable(query.decidedIn), "appliesToId": nullable(query.appliesTo),
			"tool": toolReference, "summary": summary,
			"frozen": lock != nil, "decisions": decisions,
			"markdown": decisionBlockMarkdown(summary, lock != nil, decisions),
		})
	}
	return list
}

var (
	markdownPunctuation   = regexp.MustCompile("[\\\\`*_{}\\[\\]()<>#!|&~@]")
	markdownBareAddress   = regexp.MustCompile(`(?i)(://|\bwww\.)`)
	markdownLineStart     = regexp.MustCompile(`^[-+=]`)
	markdownNumberedStart = regexp.MustCompile(`^(\d{1,9})([.)])`)
)

// decisionBlockMarkdown is a block as Minka's Markdown list writes it: the
// summary in bold, then a line per decision, every word escaped so that
// none becomes markup, nor a bare address a link.
func decisionBlockMarkdown(summary string, frozen bool, decisions []any) string {
	head := escapedForMarkdown(summary)
	if frozen {
		head += " " + escapedForMarkdown("(as locked)")
	}
	lines := []string{"**" + head + "**", ""}
	for _, entry := range decisions {
		lines = append(lines, "- "+escapedForMarkdown(decisionBlockLine(entry.(map[string]any))))
	}
	if len(decisions) == 0 {
		lines = append(lines, "- "+escapedForMarkdown("(no decisions)"))
	}
	return strings.Join(lines, "\n")
}

func decisionBlockLine(entry map[string]any) string {
	first := entry["title"].(string) + " — " + entry["status"].(map[string]any)["name"].(string)
	if shortID, ok := entry["shortId"].(string); ok {
		first = shortID + " " + first
	}
	parts := []string{first}
	votesFor, _ := entry["votesFor"].(*int)
	votesAgainst, _ := entry["votesAgainst"].(*int)
	abstentions, _ := entry["abstentions"].(*int)
	if votesFor != nil || votesAgainst != nil || abstentions != nil {
		parts = append(parts, fmt.Sprintf("%d for, %d against, %d abstaining", countOf(votesFor), countOf(votesAgainst), countOf(abstentions)))
	}
	if note, ok := entry["votesNote"].(string); ok && note != "" {
		parts = append(parts, note)
	}
	if today, ok := entry["today"].(map[string]any); ok {
		if later, ok := today["supersededBy"].(map[string]any); ok {
			name, hasShortID := later["shortId"].(string)
			if !hasShortID {
				name = later["title"].(string)
			}
			parts = append(parts, "today: superseded by "+name)
		} else if status, ok := today["status"].(map[string]any); ok {
			parts = append(parts, "today: "+status["name"].(string))
		}
		if today["reworded"] == true {
			parts = append(parts, "today: reworded")
		}
	}
	return strings.Join(parts, " · ")
}

func countOf(votes *int) int {
	if votes == nil {
		return 0
	}
	return *votes
}

func escapedForMarkdown(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	escaped := markdownPunctuation.ReplaceAllString(line, `\$0`)
	escaped = markdownBareAddress.ReplaceAllStringFunc(escaped, func(match string) string {
		if match == "://" {
			return `\://`
		}
		return match[:3] + `\.`
	})
	escaped = markdownLineStart.ReplaceAllString(escaped, `\$0`)
	return markdownNumberedStart.ReplaceAllString(escaped, `$1\$2`)
}

// decisionsOfBlock are the decisions a live block lists: those of the tool
// in and out of the archive that the reader sees and every filter lets
// through, in the order of their numbers.
func (s *Server) decisionsOfBlock(tool *DecisionsTool, query decisionBlockQuery, person *Person) []*Decision {
	var listed []*Decision
	for _, decision := range tool.Decisions {
		switch {
		case !s.seesDecision(tool, decision, person):
		case query.status != "" && s.statusIDOf(tool, decision) != query.status:
		case query.label != "" && !slices.Contains(s.ItemLabels[decision.ID], query.label):
		case query.decidedIn != "" && decision.EventID != query.decidedIn:
		case query.appliesTo != "" && decision.AppliesToID != query.appliesTo:
		default:
			listed = append(listed, decision)
		}
	}
	slices.SortStableFunc(listed, func(a, b *Decision) int {
		numberA, numberB := numberOf(a.ShortID), numberOf(b.ShortID)
		switch {
		case numberA == 0 && numberB != 0:
			return 1
		case numberB == 0 && numberA != 0:
			return -1
		}
		if c := cmp.Compare(numberA, numberB); c != 0 {
			return c
		}
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return listed
}

// numberOf is the number of a short identifier such as DEC-14, 0 for none.
func numberOf(shortID string) int {
	_, digits, found := strings.Cut(shortID, "-")
	if !found {
		return 0
	}
	number, _ := strconv.Atoi(digits)
	return number
}

func (s *Server) decisionOfBlockResource(tool *DecisionsTool, decision *Decision) map[string]any {
	return map[string]any{
		"id": decision.ID, "shortId": nullable(decision.ShortID), "title": decision.Title,
		"status":   s.statusResourceOf(tool, s.statusIDOf(tool, decision)),
		"votesFor": decision.VotesFor, "votesAgainst": decision.VotesAgainst, "abstentions": decision.Abstentions,
		"votesNote": nullable(decision.Votes), "today": nil, "appUrl": s.decisionURL(tool, decision),
	}
}

// frozenDecisionResources are the copies of a lock that the reader sees
// today, each with what changed since; a copied draft is never shown to a
// client.
func (s *Server) frozenDecisionResources(tool *DecisionsTool, rows []FrozenDecision, person *Person) []any {
	list := []any{}
	for _, row := range rows {
		current := s.decisionOfTool(tool, row.DecisionID)
		if current == nil || !s.seesDecision(tool, current, person) || row.StatusRole == "draft" && s.isAClient(person.ID) {
			continue
		}
		today := map[string]any{"status": nil, "supersededBy": nil, "reworded": current.Title != row.Title}
		if statusID := s.statusIDOf(tool, current); statusID != row.StatusID {
			today["status"] = s.statusResourceOf(tool, statusID)
		}
		if later := s.supersedingDecision(tool, current, person); later != nil {
			today["supersededBy"] = map[string]any{"id": later.ID, "shortId": nullable(later.ShortID), "title": later.Title, "appUrl": s.decisionURL(tool, later)}
		}
		list = append(list, map[string]any{
			"id": row.DecisionID, "shortId": nullable(row.ShortID), "title": row.Title,
			"status":   s.statusResourceOf(tool, row.StatusID),
			"votesFor": row.VotesFor, "votesAgainst": row.VotesAgainst, "abstentions": row.Abstentions,
			"votesNote": nullable(row.VotesNote), "today": today, "appUrl": s.decisionURL(tool, current),
		})
	}
	return list
}

// supersedingDecision is the newest decision the reader sees that
// supersedes one, or nil.
func (s *Server) supersedingDecision(tool *DecisionsTool, decision *Decision, person *Person) *Decision {
	var newest *Decision
	for _, candidate := range tool.Decisions {
		if candidate.EarlierID == decision.ID && candidate.Relation == "supersedes" && s.seesDecision(tool, candidate, person) {
			if newest == nil || candidate.CreatedAt.After(newest.CreatedAt) {
				newest = candidate
			}
		}
	}
	return newest
}

// decisionBlockSummary names the tool and every filter, in English, as the
// editor's card shows them.
func (s *Server) decisionBlockSummary(projectID string, tool *DecisionsTool, query decisionBlockQuery, person *Person) string {
	if tool == nil {
		return decisionBlockSummaryTitle
	}
	parts := []string{tool.Title}
	orHidden := func(words string) string {
		if words == "" {
			return decisionBlockHidden
		}
		return words
	}
	if query.decidedIn != "" {
		title := ""
		if event := s.eventOfBlock(projectID, query.decidedIn, person); event != nil {
			title = event.Title
		}
		parts = append(parts, "decided in "+orHidden(title))
	}
	if query.appliesTo != "" {
		parts = append(parts, "applies to "+orHidden(s.subjectTitleOfBlock(projectID, query.appliesTo, person)))
	}
	if query.label != "" {
		name := ""
		if label := s.labelOfBlock(projectID, "", query.label); label != nil {
			name = label.Name
		}
		parts = append(parts, "label "+orHidden(name))
	}
	if query.status != "" {
		name := ""
		for _, status := range tool.Statuses {
			if status.ID == query.status {
				name, _ = statusResource(status)["name"].(string)
			}
		}
		parts = append(parts, "status "+orHidden(name))
	}
	return strings.Join(parts, " · ")
}

// LockDocument locks a document as the team does on its page: its blocks
// copy the decisions they list now.
func (s *Server) LockDocument(documentID string) {
	for _, tool := range s.FilesTools {
		for _, document := range tool.Documents {
			if document.ID != documentID {
				continue
			}
			lock := &DocumentLock{Rows: map[string][]FrozenDecision{}}
			for _, attributes := range decisionsElements(document.Body) {
				query := queryOf(attributes)
				if query == nil {
					continue
				}
				for _, decisionsTool := range s.DecisionsTools {
					if decisionsTool.ID != query.tool || decisionsTool.ProjectID != tool.ProjectID {
						continue
					}
					var rows []FrozenDecision
					for _, decision := range s.decisionsOfBlock(decisionsTool, *query, &Person{}) {
						statusID := s.statusIDOf(decisionsTool, decision)
						rows = append(rows, FrozenDecision{
							DecisionID: decision.ID, ToolID: decisionsTool.ID, ShortID: decision.ShortID, Title: decision.Title,
							StatusID: statusID, StatusRole: s.statusRole(decisionsTool, statusID),
							VotesFor: decision.VotesFor, VotesAgainst: decision.VotesAgainst, Abstentions: decision.Abstentions,
							VotesNote: decision.Votes,
						})
					}
					lock.Rows[query.key()] = rows
				}
			}
			document.Lock = lock
			return
		}
	}
}
