package fakebasecamp

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strconv"
)

// List is a list of a card table: one of the three every table has, or a
// column somebody made, which alone has a position.
type List struct {
	ID        int64
	Type      string
	Title     string
	Position  int
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	// Cards holds the archived and the trashed cards of the list as well; the
	// account lists them where Basecamp does.
	Cards  []Card
	OnHold *OnHold
}

// OnHold is the part of a column its cards can be set aside in.
type OnHold struct {
	ID    int64
	Cards []Card
}

type Card struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the card says
	// about it, wherever it is listed.
	Status      string
	ShownStatus string
	Title       string
	// Content is the note as stored, Description the same note rendered.
	Content     string
	Description string
	// Position is left out of the answer when 0.
	Position   int
	DueOn      string
	Creator    *Person
	Assignees  []Person
	CreatedAt  string
	UpdatedAt  string
	Completion *Completion
	Steps      []Step
	Comments   []Comment
	Boosts     []Boost
	Files      []File
}

type Completion struct {
	At string
	By *Person
}

type Step struct {
	ID         int64
	Title      string
	Position   int
	Creator    *Person
	CreatedAt  string
	UpdatedAt  string
	Completion *Completion
}

type Comment struct {
	ID int64
	// Status is "active" when empty.
	Status    string
	Content   string
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	Files     []File
	Boosts    []Boost
}

type Boost struct {
	ID        int64
	Content   string
	CreatedAt string
	Booster   *Person
}

// File is a file Basecamp lists next to the text it is embedded in.
type File struct {
	ID          int64
	SGID        string
	Key         string
	Filename    string
	ContentType string
	Bytes       int64
}

const (
	Triage = "Kanban::Triage"
	NotNow = "Kanban::NotNowColumn"
	Column = "Kanban::Column"
	Done   = "Kanban::DoneColumn"
)

// SGID is a signed global ID the way Basecamp writes them today: base64 of an
// envelope that Ruby serialised, and a signature.
func SGID(globalID string) string {
	envelope := "\x04\x08{\x06I\"\x0b_rails\x06:\x06ET{\x07I\"\tdata\x06;\x00T" + rubyString(globalID) + "\x06;\x00TI\"\x08pur\x06;\x00TI\"\x0fattachable\x06;\x00T"
	return base64.StdEncoding.EncodeToString([]byte(envelope)) + "--0123456789abcdef0123456789abcdef01234567"
}

// EnvelopedSGID is a signed global ID the way older texts hold them: an
// envelope of JSON, in which what it names stands serialised by Ruby and as
// base64 that is safe for an address, without padding.
func EnvelopedSGID(globalID string) string {
	message := base64.RawURLEncoding.EncodeToString([]byte("\x04\x08" + rubyString(globalID) + "\x06:\x06ET"))
	return base64.RawStdEncoding.EncodeToString([]byte(`{"_rails":{"message":"`+message+`","exp":null,"pur":"attachable"}}`)) + "--fedcba9876543210fedcba9876543210fedcba98"
}

// rubyString is a short string as Ruby serialises it: a byte for its length.
func rubyString(text string) string {
	return "I\"" + string([]byte{byte(len(text) + 5)}) + text
}

// Mention is a mention as Basecamp renders it into a comment. An empty content
// leaves the attribute of newer mentions out.
func Mention(sgid, content string, personID int64, name string) string {
	attribute := ""
	if content != "" {
		attribute = fmt.Sprintf(` content='%s'`, content)
	}
	return fmt.Sprintf(`<bc-attachment sgid="%s" content-type="application/vnd.basecamp.mention"%s><figure><img data-avatar-for-person-id="%d" alt="%s" src="https://avatars.basecamp.test/4242/people/%d/avatar"><figcaption>%s</figcaption></figure></bc-attachment>`,
		sgid, attribute, personID, name, personID, name)
}

// EmbeddedFile is a file as Basecamp renders it into a text. The key names the
// stored file in the address of the embed.
func EmbeddedFile(sgid, filename string) string {
	return EmbeddedFileWithKey(sgid, "", filename)
}

func EmbeddedFileWithKey(sgid, key, filename string) string {
	address := ""
	if key != "" {
		address = fmt.Sprintf(` url="https://storage.basecamp.test/4242/blobs/%s/download/%s" href="https://storage.basecamp.test/4242/blobs/%s/download/%s"`, key, filename, key, filename)
	}
	return fmt.Sprintf(`<bc-attachment sgid="%s" content-type="image/png"`+address+` filename="%s"><figure><img src="https://preview.basecamp.test/%s"><figcaption>%s</figcaption></figure></bc-attachment>`,
		sgid, filename, filename, filename)
}

// boardProject is a project with every case of a board: all four roles, a
// column with cards set aside, positions with a gap, completed cards, archived
// and trashed ones, a card of a board that is switched off, steps, comments
// over two pages, boosts on a card and on a comment, and texts with mentions
// and files of every kind.
func boardProject(ada, grace, linus, edsger, former Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2023-03-%02dT%02d:00:00.000Z", day, hour) }

	graceMention := SGID("an opaque reference")
	niklausMention := EnvelopedSGID("gid://bc3/Person/105?expires_in")
	// An embed of an earlier day: its signed ID is another than the list has.
	plan := File{ID: 5003, SGID: SGID("gid://bc3/Attachment/5003?expires_in"), Key: "k5003f00dcafe", Filename: "plan.png", ContentType: "image/png", Bytes: 4096}
	goneMention := SGID("another opaque reference")
	sketch := File{ID: 5001, SGID: SGID("gid://bc3/Attachment/5001?expires_in"), Key: "k5001f00dcafe", Filename: "sketch.png", ContentType: "image/png", Bytes: 2048}
	lost := SGID("gid://bc3/Attachment/5002?expires_in")

	roadmap := tool(9020, "kanban_board", "card_tables", "Roadmap", true, 1, &ada)
	roadmap.Lists = []List{
		{ID: 9103, Type: Triage, Title: "Inbox", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 9), Cards: []Card{
			{ID: 9204, Title: "Write the announcement", Position: 1, Creator: &former, CreatedAt: at(2, 8), UpdatedAt: at(2, 9)},
		}},
		{ID: 9105, Type: NotNow, Title: "Not now", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 9)},
		{ID: 9104, Type: Column, Title: "Doing", Position: 1, Creator: &grace, CreatedAt: at(1, 10), UpdatedAt: at(1, 11),
			Cards: []Card{
				{
					ID: 9201, Title: "Draw the map", Position: 1, DueOn: "2023-04-01", Creator: &ada, Assignees: []Person{grace, ada},
					CreatedAt: at(5, 8), UpdatedAt: at(6, 8),
					Content: `<div>Ask <bc-attachment sgid="` + graceMention + `" content-type="application/vnd.basecamp.mention" content='&lt;bc-mention gid="gid://bc3/Person/102"&gt;&lt;/bc-mention&gt;'></bc-attachment> about tea &amp; biscuits.</div>` +
						`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"></bc-attachment>` +
						`<div><bc-attachment sgid="` + sketch.SGID + `" content-type="image/png" filename="sketch.png"></bc-attachment></div>`,
					Description: `<div>Ask ` + Mention(graceMention, `&lt;bc-mention gid="gid://bc3/Person/102"&gt;&lt;/bc-mention&gt;`, 102, "Grace Sample") + ` about tea &amp; biscuits.</div>` +
						`<bc-attachment content-type="application/vnd.basecamp.horizontal-rule.html"><hr></bc-attachment>` +
						`<div>` + EmbeddedFile(sketch.SGID, "sketch.png") + `</div>`,
					Files: []File{sketch},
					Steps: []Step{
						{ID: 9402, Title: "Colour the sea", Position: 2, Creator: &ada, CreatedAt: at(5, 10), UpdatedAt: at(5, 10)},
						{ID: 9401, Title: "Outline the coast", Position: 1, Creator: &ada, CreatedAt: at(5, 9), UpdatedAt: at(7, 9), Completion: &Completion{At: at(7, 9), By: &grace}},
						{ID: 9403, Title: "Name the towns", Position: 3, Creator: &grace, CreatedAt: at(5, 11), UpdatedAt: at(5, 11)},
					},
					Comments: []Comment{
						{ID: 9301, Creator: &grace, CreatedAt: at(5, 12), UpdatedAt: at(5, 13),
							Content: `<div>Fish &amp; chips for ` + Mention(niklausMention, "", 105, "Niklaus Nowhere") + `?</div>`,
							Boosts: []Boost{
								{ID: 9502, Content: "Well done, all of you", CreatedAt: at(5, 15), Booster: &ada},
								{ID: 9501, Content: "🎉", CreatedAt: at(5, 14), Booster: &grace},
							}},
						{ID: 9302, Creator: &former, CreatedAt: at(5, 14), UpdatedAt: at(5, 14),
							Content: `<div>` + Mention(goneMention, "", 777, "Gone Person") + ` had one: ` + EmbeddedFile(lost, "lost.png") + `</div>`},
						{ID: 9303, Creator: &ada, CreatedAt: at(5, 16), UpdatedAt: at(5, 16), Files: []File{plan},
							Content: `<div>Settled.` + EmbeddedFileWithKey(EnvelopedSGID("gid://bc3/Attachment/777777?expires_in"), plan.Key, "plan.png") + `</div>`},
					},
					Boosts: []Boost{{ID: 9503, Content: "👍", CreatedAt: at(6, 8), Booster: &linus}},
				},
				{ID: 9202, Title: "Print the map", Position: 3, Creator: &grace, CreatedAt: at(3, 8), UpdatedAt: at(3, 8)},
				{ID: 9207, Status: "archived", Title: "Fold the map", Creator: &ada, CreatedAt: at(2, 10), UpdatedAt: at(8, 8)},
			},
			OnHold: &OnHold{ID: 9113, Cards: []Card{
				{ID: 9203, Title: "Frame the map", Position: 1, Creator: &ada, CreatedAt: at(4, 8), UpdatedAt: at(4, 8)},
				{ID: 9208, Status: "trashed", Title: "Burn the map", Creator: &ada, CreatedAt: at(4, 9), UpdatedAt: at(9, 8)},
			}},
		},
		{ID: 9101, Type: Column, Title: "Review", Position: 2, Creator: &grace, CreatedAt: at(1, 12), UpdatedAt: at(1, 12)},
		{ID: 9102, Type: Done, Title: "Done", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 9), Cards: []Card{
			{ID: 9206, Title: "Buy paper", Creator: &ada, CreatedAt: at(1, 14), UpdatedAt: at(3, 14), Completion: &Completion{At: at(3, 14), By: &edsger}},
			{ID: 9205, Title: "Buy pencils", Creator: &ada, CreatedAt: at(1, 14), UpdatedAt: at(2, 14), Completion: &Completion{At: at(2, 14), By: &grace}},
		}},
	}

	retired := tool(9021, "kanban_board", "card_tables", "Retired board", false, 0, &ada)
	retired.Lists = []List{
		{ID: 9121, Type: Triage, Title: "Triage", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8), Cards: []Card{
			{ID: 9209, Status: "archived", Title: "Of a board nobody sees", Creator: &ada, CreatedAt: at(2, 11), UpdatedAt: at(8, 9)},
		}},
	}

	second := tool(9022, "kanban_board", "card_tables", "Second board", true, 2, &grace)
	second.Lists = []List{
		{ID: 9131, Type: Triage, Title: "Triage", Creator: &grace, CreatedAt: at(1, 8), UpdatedAt: at(1, 8), Cards: []Card{
			{ID: 9211, Title: "Sharpen pencils", Position: 1, Creator: &grace, CreatedAt: at(6, 9), UpdatedAt: at(6, 9)},
			{ID: 9210, Status: "archived", Title: "Count pencils", Creator: &grace, CreatedAt: at(6, 8), UpdatedAt: at(8, 10)},
		}},
		{ID: 9132, Type: NotNow, Title: "Not now", Creator: &grace, CreatedAt: at(1, 8), UpdatedAt: at(1, 8)},
		{ID: 9133, Type: Done, Title: "Done", Creator: &grace, CreatedAt: at(1, 8), UpdatedAt: at(1, 8)},
	}

	return Project{
		ID: 7003, Name: "Board Project", Status: "active",
		CreatedAt: "2023-03-01T07:00:00.000Z", UpdatedAt: "2023-03-09T09:00:00.000Z",
		Members: []int64{101, 102},
		Dock:    []Tool{roadmap, retired, second},
	}
}

var (
	cardsPattern      = regexp.MustCompile(`^buckets/([0-9]+)/card_tables/lists/([0-9]+)/cards\.json$`)
	recordingsPattern = regexp.MustCompile(`^projects/recordings\.json\?type=Kanban::Card&bucket=([0-9]+)&status=(archived|trashed)$`)
	commentsPattern   = regexp.MustCompile(`^buckets/([0-9]+)/recordings/([0-9]+)/comments\.json$`)
	boostsPattern     = regexp.MustCompile(`^buckets/([0-9]+)/recordings/([0-9]+)/boosts\.json$`)
	subtasksPattern   = regexp.MustCompile(`^recordings/([0-9]+)/subtasks\.json$`)
)

// boards answers the paths of card tables. The second value tells whether the
// path was one of them.
func (a *Account) boards(path string, page int) ([]byte, bool) {
	switch {
	case cardsPattern.MatchString(path):
		match := cardsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			listed := false
			a.eachCard(p, func(list *List, onHold *OnHold, card *Card) {
				id := list.ID
				if onHold != nil {
					id = onHold.ID
				}
				asked := strconv.FormatInt(id, 10) == match[2] && !(onHold != nil && a.OnHoldListEmpty) ||
					a.OnHoldInColumn && strconv.FormatInt(list.ID, 10) == match[2]
				if asked && card.status() == "active" {
					found = append(found, a.cardJSON(p, list, onHold, card))
				}
			})
			for _, tool := range p.Dock {
				for _, list := range tool.Lists {
					listed = listed || strconv.FormatInt(list.ID, 10) == match[2] ||
						list.OnHold != nil && strconv.FormatInt(list.OnHold.ID, 10) == match[2]
				}
			}
			if listed {
				return a.pageOf(found, page), true
			}
		}
	case recordingsPattern.MatchString(path):
		match := recordingsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			a.eachCard(p, func(list *List, onHold *OnHold, card *Card) {
				if card.status() == match[2] {
					found = append(found, a.cardJSON(p, list, onHold, card))
				}
			})
			// One list across all card tables, whatever table a card is of.
			sort.SliceStable(found, func(i, j int) bool { return found[i]["id"].(int64) > found[j]["id"].(int64) })
			return a.pageOf(found, page), true
		}
	case commentsPattern.MatchString(path):
		match := commentsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var answer []byte
			a.eachCard(p, func(_ *List, _ *OnHold, card *Card) {
				if strconv.FormatInt(card.ID, 10) == match[2] {
					comments := []map[string]any{}
					for _, comment := range card.Comments {
						comments = append(comments, a.commentJSON(p, comment))
					}
					answer = a.pageOf(comments, page)
				}
			})
			return answer, answer != nil
		}
	case boostsPattern.MatchString(path):
		match := boostsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var answer []byte
			a.eachCard(p, func(_ *List, _ *OnHold, card *Card) {
				if strconv.FormatInt(card.ID, 10) == match[2] {
					answer = a.pageOf(a.boostsJSON(card.Boosts), page)
				}
				for _, comment := range card.Comments {
					if strconv.FormatInt(comment.ID, 10) == match[2] {
						answer = a.pageOf(a.boostsJSON(comment.Boosts), page)
					}
				}
			})
			return answer, answer != nil
		}
	case subtasksPattern.MatchString(path):
		match := subtasksPattern.FindStringSubmatch(path)
		for i := range a.Projects {
			var answer []byte
			a.eachCard(&a.Projects[i], func(_ *List, _ *OnHold, card *Card) {
				if strconv.FormatInt(card.ID, 10) == match[1] {
					answer = a.pageOf(a.stepsJSON(card.Steps), page)
				}
			})
			if answer != nil {
				return answer, true
			}
		}
	}
	return nil, false
}

func (c *Card) status() string {
	if c.Status == "" {
		return "active"
	}
	return c.Status
}

// eachCard visits the cards of every card table of a project, those of tables
// that are switched off included.
func (a *Account) eachCard(p *Project, visit func(*List, *OnHold, *Card)) {
	for t := range p.Dock {
		for l := range p.Dock[t].Lists {
			list := &p.Dock[t].Lists[l]
			for c := range list.Cards {
				visit(list, nil, &list.Cards[c])
			}
			if list.OnHold != nil {
				for c := range list.OnHold.Cards {
					visit(list, list.OnHold, &list.OnHold.Cards[c])
				}
			}
		}
	}
}

func (a *Account) listsJSON(p *Project, lists []List) []map[string]any {
	answer := []map[string]any{}
	for _, list := range lists {
		cardsURL := func(id int64) string {
			return fmt.Sprintf("%sbuckets/%d/card_tables/lists/%d/cards.json", a.root(), p.ID, id)
		}
		entry := map[string]any{
			"id": list.ID, "type": list.Type, "title": list.Title, "color": nil, "cards_url": cardsURL(list.ID),
			"created_at": list.CreatedAt, "updated_at": list.UpdatedAt,
		}
		if list.Creator != nil {
			entry["creator"] = a.person(*list.Creator)
		}
		if list.Position != 0 {
			entry["position"] = list.Position
		}
		if list.OnHold != nil {
			entry["on_hold"] = map[string]any{"id": list.OnHold.ID, "title": "On hold", "cards_url": cardsURL(list.OnHold.ID)}
		}
		answer = append(answer, entry)
	}
	return answer
}

func (a *Account) cardJSON(p *Project, list *List, onHold *OnHold, card *Card) map[string]any {
	parent := map[string]any{"id": list.ID, "type": list.Type, "title": list.Title}
	if onHold != nil {
		parent = map[string]any{"id": onHold.ID, "type": "Kanban::OnHold", "title": "On hold"}
	}
	steps := a.stepsJSON(card.Steps)
	if a.InlineSteps > 0 && len(steps) > a.InlineSteps {
		steps = steps[:a.InlineSteps]
	}
	assignees := []map[string]any{}
	for _, assignee := range card.Assignees {
		assignees = append(assignees, a.person(assignee))
	}
	var dueOn any
	if card.DueOn != "" {
		dueOn = card.DueOn
	}

	answer := map[string]any{
		"id": card.ID, "status": card.status(), "title": card.Title, "parent": parent,
		"content": card.Content, "description": card.Description, "description_attachments": a.filesJSON(card.Files),
		"due_on": dueOn, "assignees": assignees, "completed": card.Completion != nil,
		"steps": steps, "subtasks_count": len(card.Steps), "subtasks_url": fmt.Sprintf("%srecordings/%d/subtasks.json", a.root(), card.ID),
		"comments_count": len(card.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, card.ID),
		"boosts_count": len(card.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, card.ID),
		"created_at": card.CreatedAt, "updated_at": card.UpdatedAt,
	}
	if card.ShownStatus != "" {
		answer["status"] = card.ShownStatus
	}
	if url, replaced := a.SubtasksURL[card.ID]; replaced {
		answer["subtasks_url"] = url
	}
	if card.Creator != nil {
		answer["creator"] = a.person(*card.Creator)
	}
	if card.Position != 0 {
		answer["position"] = card.Position
	}
	if card.Completion != nil {
		answer["completion"] = a.completionJSON(*card.Completion)
	}
	return answer
}

func (a *Account) completionJSON(c Completion) map[string]any {
	answer := map[string]any{"created_at": c.At}
	if c.By != nil {
		answer["creator"] = a.person(*c.By)
	}
	return answer
}

func (a *Account) stepsJSON(steps []Step) []map[string]any {
	answer := []map[string]any{}
	for _, step := range steps {
		entry := map[string]any{
			"id": step.ID, "title": step.Title, "position": step.Position, "completed": step.Completion != nil,
			"created_at": step.CreatedAt, "updated_at": step.UpdatedAt, "assignees": []any{}, "due_on": nil,
		}
		if step.Creator != nil {
			entry["creator"] = a.person(*step.Creator)
		}
		if step.Completion != nil {
			entry["completion"] = a.completionJSON(*step.Completion)
		}
		answer = append(answer, entry)
	}
	return answer
}

func (a *Account) commentJSON(p *Project, comment Comment) map[string]any {
	status := comment.Status
	if status == "" {
		status = "active"
	}
	answer := map[string]any{
		"id": comment.ID, "status": status, "content": comment.Content, "content_attachments": a.filesJSON(comment.Files),
		"created_at": comment.CreatedAt, "updated_at": comment.UpdatedAt,
		"boosts_count": len(comment.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, comment.ID),
	}
	if comment.Creator != nil {
		answer["creator"] = a.person(*comment.Creator)
	}
	return answer
}

func (a *Account) boostsJSON(boosts []Boost) []map[string]any {
	answer := []map[string]any{}
	for _, boost := range boosts {
		entry := map[string]any{"id": boost.ID, "content": boost.Content, "created_at": boost.CreatedAt}
		if boost.Booster != nil {
			entry["booster"] = a.person(*boost.Booster)
		}
		answer = append(answer, entry)
	}
	return answer
}

func (a *Account) filesJSON(files []File) []map[string]any {
	answer := []map[string]any{}
	for _, file := range files {
		answer = append(answer, map[string]any{
			"id": file.ID, "sgid": file.SGID, "attachable_sgid": file.SGID, "key": file.Key, "filename": file.Filename,
			"content_type": file.ContentType, "byte_size": file.Bytes,
		})
	}
	return answer
}

func (a *Account) pageOf(items []map[string]any, page int) []byte {
	start := max(page-1, 0) * a.PageSize
	if start > len(items) {
		start = len(items)
	}
	end := min(start+a.PageSize, len(items))
	if items == nil {
		items = []map[string]any{}
	}
	return encode(items[start:end])
}
