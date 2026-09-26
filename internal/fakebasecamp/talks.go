package fakebasecamp

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strconv"
)

// Category is a kind of message. A message shows of it only the ID, the name
// and the icon.
type Category struct {
	ID        int64
	Name      string
	Icon      string
	CreatedAt string
	UpdatedAt string
}

// Post is a message of a message board.
type Post struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the message
	// says about it, wherever it is listed.
	Status      string
	ShownStatus string
	Subject     string
	Content     string
	Category    *Category
	Creator     *Person
	CreatedAt   string
	UpdatedAt   string
	Comments    []Comment
	Boosts      []Boost
	Files       []File
}

const (
	TextLine        = "Chat::Lines::Text"
	RichTextLine    = "Chat::Lines::RichText"
	UploadLine      = "Chat::Lines::Upload"
	IntegrationLine = "Chat::Lines::Integration"
	// UnknownLine stands for the kinds of line Basecamp grows that a bundle
	// has no word for. What it is called says what it is, because what the
	// next one will be called nobody knows.
	UnknownLine = "Chat::Lines::Unknown"
)

// Line is a line of a chat. One that is an upload has no content.
type Line struct {
	ID   int64
	Type string
	// Status is "active" when empty.
	Status    string
	Content   string
	Creator   *Person
	CreatedAt string
	UpdatedAt string
	Uploads   []Upload
	Boosts    []Boost
}

// Upload is a file of a line. Its addresses are made from the key; URL and
// DownloadURL replace them.
type Upload struct {
	Key         string
	Filename    string
	ContentType string
	Bytes       int64
	URL         string
	DownloadURL string
	// Gone is a file the account lists and no longer has.
	Gone bool
	// URLName replaces the name in the address the file is downloaded from.
	URLName string
}

// ForeignPicture is a picture that lies at an address of somebody else, as
// Basecamp puts it into a text: an embed without a signed ID.
func ForeignPicture(address, caption string) string {
	return fmt.Sprintf(`<bc-attachment content-type="image" url="%s" width="640" height="480" caption="%s"><figure><img src="%s"><figcaption>%s</figcaption></figure></bc-attachment>`,
		html.EscapeString(address), html.EscapeString(caption), html.EscapeString(address), html.EscapeString(caption))
}

// LinkPreview is the preview of a link as Basecamp puts it into a text: an
// embed without a signed ID and with nothing in it.
func LinkPreview(address, title string) string {
	return fmt.Sprintf(`<bc-attachment content-type="application/vnd.basecamp.opengraph-embed" url="%s" href="%s" title="%s" caption="example.test"></bc-attachment>`,
		html.EscapeString(address), html.EscapeString(address), html.EscapeString(title))
}

// talkProject is a project with every case of a message board and a chat: two
// boards, the second with a category no list holds, a board that is switched
// off, messages of every status, one without a category, one without a text,
// comments and boosts; a chat with every kind of line a bundle knows and one of
// a kind it does not, a bot and somebody deleted among the writers, uploads with
// and without a key, previews of links and an embedded video, a chat without a
// line, and a small one.
func talkProject(ada, grace, linus, edsger, former, bot, deleted Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2023-04-%02dT%02d:00:00.000Z", day, hour) }
	minute := func(n int) string { return fmt.Sprintf("2023-04-20T09:%02d:00.000Z", n) }
	// Only the first tool was made together with the project.
	made := tool
	tool = func(id int64, name, kind, title string, enabled bool, position int, creator *Person) Tool {
		t := made(id, name, kind, title, enabled, position, creator)
		t.CreatedAt, t.UpdatedAt = at(2, int(id-9030)), at(3, int(id-9030))
		return t
	}

	announcement := Category{ID: 9601, Name: "Announcement", Icon: "📢", CreatedAt: at(1, 8), UpdatedAt: at(1, 8)}
	question := Category{ID: 9602, Name: "faq", Icon: "❓", CreatedAt: at(1, 8), UpdatedAt: at(2, 8)}
	heartbeat := Category{ID: 9603, Name: "Heartbeat", Icon: "💓", CreatedAt: at(1, 8), UpdatedAt: at(1, 8)}
	// Of the second board, and so in no list.
	pitch := Category{ID: 9650, Name: "Pitch", Icon: "💡"}

	floor := File{ID: 5011, SGID: SGID("gid://bc3/Attachment/5011?expires_in"), Key: FileKey(5011), Filename: "floor.png", ContentType: "image/png", Bytes: 1024}

	news := tool(9030, "message_board", "message_boards", "News", true, 1, &ada)
	news.CreatedAt = "2023-04-01T07:00:00.200Z"
	news.Posts = []Post{
		{ID: 9701, Subject: "Office move", Category: &announcement, Creator: &ada, CreatedAt: at(10, 8), UpdatedAt: at(10, 12), Files: []File{floor},
			Content: `<div>We move in May, ` + Mention(SGID("gid://bc3/Person/102?expires_in"), "", 102, "Grace Sample") + ` packs.</div><h1>Plan</h1><table><tr><td>Boxes &amp; tape</td></tr></table>` + EmbeddedFile(floor.SGID, "floor.png"),
			Comments: []Comment{
				{ID: 9752, Creator: &former, CreatedAt: at(10, 10), UpdatedAt: at(10, 10), Content: `<div>Who carries the plants? ` + ForeignPicture("https://pictures.example.test/fern.png", "A fern") + `</div>`},
				{ID: 9751, Creator: &grace, CreatedAt: at(10, 9), UpdatedAt: at(10, 9), Content: `<div>Count me in</div>`,
					Boosts: []Boost{{ID: 9791, Content: "👍", CreatedAt: at(10, 11), Booster: &linus}}},
			},
			Boosts: []Boost{{ID: 9792, Content: "🎉", CreatedAt: at(10, 13), Booster: &edsger}},
		},
		{ID: 9702, Subject: "Reminder without words", Creator: &grace, CreatedAt: at(11, 8), UpdatedAt: at(11, 8)},
		{ID: 9703, Status: "archived", Subject: "Old house rules", Category: &question, Creator: &ada, CreatedAt: at(9, 8), UpdatedAt: at(13, 8),
			Content: `<div>Does anybody still read these?</div>`},
		{ID: 9704, Status: "trashed", Subject: "Posted on the wrong board", Creator: &ada, CreatedAt: at(12, 8), UpdatedAt: at(12, 9),
			Content:  `<div>Never mind.</div>`,
			Comments: []Comment{{ID: 9753, Creator: &ada, CreatedAt: at(12, 9), UpdatedAt: at(12, 9), Content: `<div>Moving it.</div>`}}},
	}

	attic := tool(9031, "message_board", "message_boards", "Attic", false, 0, &ada)
	attic.Posts = []Post{
		{ID: 9721, Status: "archived", Subject: "Forgotten in the attic", Creator: &ada, CreatedAt: at(3, 8), UpdatedAt: at(4, 8), Content: `<div>Dust.</div>`},
	}

	pitches := tool(9032, "message_board", "message_boards", "Pitches", true, 2, &grace)
	pitches.Posts = []Post{
		{ID: 9711, Subject: "A tea club", Category: &pitch, Creator: &grace, CreatedAt: at(12, 9), UpdatedAt: at(12, 9), Content: `<div>Every Thursday.</div>`},
		{ID: 9712, Subject: "A book shelf", Category: &pitch, Creator: &ada, CreatedAt: at(11, 9), UpdatedAt: at(11, 9),
			Content: `<div>Like ` + LinkPreview("https://example.test/shelves", "Shelves of the world") + `</div>`},
	}

	campfire := tool(9033, "chat", "chats", "Campfire", true, 3, &ada)
	campfire.Lines = []Line{
		{ID: 9801, Type: TextLine, Creator: &ada, CreatedAt: minute(1), UpdatedAt: minute(1),
			Content: `Good morning<br>all, see <a href="https://example.test/agenda">https://example.test/agenda</a>`},
		{ID: 9802, Type: RichTextLine, Creator: &grace, CreatedAt: minute(2), UpdatedAt: minute(3),
			Content: `<div>` + Mention(EnvelopedSGID("gid://bc3/Person/101?expires_in"), "", 101, "Ada Example") + ` look: ` +
				LinkPreview("https://example.test/news?a=1&b=2", "News & weather") + `</div><figure><iframe src="https://video.example.test/embed/42"></iframe></figure>`},
		{ID: 9803, Type: UploadLine, Creator: &grace, CreatedAt: minute(4), UpdatedAt: minute(4), Uploads: []Upload{
			{Key: "0a1b2c3d-0001-4000-8000-00000000000a", Filename: "minutes.pdf", ContentType: "application/pdf", Bytes: 2048},
			{Key: "0a1b2c3d-0002-4000-8000-00000000000b", Filename: "whiteboard.png", ContentType: "image/png", Bytes: 4096},
		}},
		{ID: 9804, Type: IntegrationLine, Creator: &bot, CreatedAt: minute(5), UpdatedAt: minute(5), Content: `<b>Build 42</b> passed`},
		{ID: 9805, Type: TextLine, Creator: &deleted, CreatedAt: minute(6), UpdatedAt: minute(6), Content: `Goodbye, everyone`},
		{ID: 9806, Type: RichTextLine, Creator: &ada, CreatedAt: minute(7), UpdatedAt: minute(7),
			Content: `<div>` + LinkPreview("https://example.test/plain", "") + ` or ` + LinkPreview("javascript:alert(1)", "Click <me>") + `</div>`,
			Boosts: []Boost{
				{ID: 9892, Content: "👏", CreatedAt: minute(9), Booster: &linus},
				{ID: 9891, Content: "Ha", CreatedAt: minute(8), Booster: &grace},
			}},
		// The second file is named at no address a key stands in; the first
		// only where it is previewed.
		{ID: 9807, Type: UploadLine, Creator: &ada, CreatedAt: minute(10), UpdatedAt: minute(10), Uploads: []Upload{
			{Key: "0a1b2c3d-0003-4000-8000-00000000000c", Filename: "route.png", ContentType: "image/png", Bytes: 512, DownloadURL: "https://storage.basecamp.test/elsewhere/route.png"},
			{Filename: "lost route.png", ContentType: "image/png", Bytes: 512, URL: "https://preview.basecamp.test/4242/blobs/not a key/previews/full", DownloadURL: "https://storage.basecamp.test/4242/blobs/not a key/download/lost route.png"},
		}},
		{ID: 9808, Type: UploadLine, Creator: &grace, CreatedAt: minute(11), UpdatedAt: minute(11), Uploads: []Upload{
			{Filename: "nowhere.png", ContentType: "image/png", Bytes: 512, URL: "https://preview.basecamp.test/nowhere", DownloadURL: "https://storage.basecamp.test/nowhere"},
		}},
		// A kind of line a bundle has no word for, with a text and a boost that
		// a bundle carries none of either.
		{ID: 9809, Type: UnknownLine, Creator: &ada, CreatedAt: minute(12), UpdatedAt: minute(12), Content: `Whatever this is`,
			Boosts: []Boost{{ID: 9893, Content: "🎲", CreatedAt: minute(13), Booster: &grace}}},
		// A second of that kind, thrown away and without the time it was last
		// touched: its kind is what it is left out for, and two lines of one kind
		// are two lines.
		{ID: 9810, Type: UnknownLine, Status: "trashed", Creator: &ada, CreatedAt: minute(14), Content: `Whatever that was`},
	}

	quiet := tool(9034, "chat", "chats", "Quiet room", true, 4, &grace)

	// Two lines of the same moment, the one with the higher ID listed last.
	side := tool(9035, "chat", "chats", "Side room", true, 5, &grace)
	side.Lines = []Line{
		{ID: 9812, Type: TextLine, Creator: &ada, CreatedAt: minute(30), UpdatedAt: minute(30), Content: `Pong`},
		{ID: 9811, Type: TextLine, Creator: &grace, CreatedAt: minute(30), UpdatedAt: minute(30), Content: `Ping`},
	}

	return Project{
		ID: 7004, Name: "Talk Project", Status: "active",
		CreatedAt: "2023-04-01T07:00:00.000Z", UpdatedAt: "2023-04-20T10:00:00.000Z",
		Members:    []int64{101, 102},
		Dock:       []Tool{news, attic, pitches, campfire, quiet, side},
		Categories: []Category{question, heartbeat, announcement},
	}
}

var (
	messagesPattern   = regexp.MustCompile(`^buckets/([0-9]+)/message_boards/([0-9]+)/messages\.json$`)
	postsPattern      = regexp.MustCompile(`^projects/recordings\.json\?type=Message&bucket=([0-9]+)&status=(archived|trashed)$`)
	categoriesPattern = regexp.MustCompile(`^buckets/([0-9]+)/categories\.json$`)
	linesPattern      = regexp.MustCompile(`^buckets/([0-9]+)/chats/([0-9]+)/lines\.json$`)
)

// talks answers the paths of message boards and chats. The second value tells
// whether the path was one of them.
func (a *Account) talks(path string, page int) ([]byte, bool) {
	switch {
	case messagesPattern.MatchString(path):
		match := messagesPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for t := range p.Dock {
				tool := &p.Dock[t]
				if tool.Type != "message_boards" || strconv.FormatInt(tool.ID, 10) != match[2] {
					continue
				}
				var found []map[string]any
				for i := range tool.Posts {
					if tool.Posts[i].status() == "active" {
						found = append(found, a.postJSON(p, tool, &tool.Posts[i]))
					}
				}
				return a.gearedPageOf(found, page), true
			}
		}
	case postsPattern.MatchString(path):
		match := postsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			var found []map[string]any
			for t := range p.Dock {
				for i := range p.Dock[t].Posts {
					if p.Dock[t].Posts[i].status() == match[2] {
						found = append(found, a.postJSON(p, &p.Dock[t], &p.Dock[t].Posts[i]))
					}
				}
			}
			// One list across all message boards, whatever board a message is of.
			sort.SliceStable(found, func(i, j int) bool { return found[i]["id"].(int64) > found[j]["id"].(int64) })
			return a.gearedPageOf(found, page), true
		}
	case categoriesPattern.MatchString(path):
		// The list has no pages: every page is all of it.
		if p := a.project(categoriesPattern.FindStringSubmatch(path)[1]); p != nil {
			categories := []map[string]any{}
			for _, category := range p.Categories {
				categories = append(categories, map[string]any{
					"id": category.ID, "name": category.Name, "icon": category.Icon,
					"created_at": category.CreatedAt, "updated_at": category.UpdatedAt,
				})
			}
			return encode(categories), true
		}
	case linesPattern.MatchString(path):
		match := linesPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for t := range p.Dock {
				tool := &p.Dock[t]
				if tool.Type != "chats" || strconv.FormatInt(tool.ID, 10) != match[2] {
					continue
				}
				// Newest first, and nothing turns the list round.
				var found []map[string]any
				for i := len(tool.Lines) - 1; i >= 0; i-- {
					found = append(found, a.lineJSON(p, tool, tool.Lines[i]))
				}
				return a.gearedPageOf(found, page), true
			}
		}
	}
	return nil, false
}

func (p *Post) status() string {
	if p.Status == "" {
		return "active"
	}
	return p.Status
}

// commentsOf finds the comments of a card or a message by its ID.
func (a *Account) commentsOf(p *Project, id string) ([]Comment, bool) {
	var comments []Comment
	found := false
	a.eachCard(p, func(_ *List, _ *OnHold, card *Card) {
		if strconv.FormatInt(card.ID, 10) == id {
			comments, found = card.Comments, true
		}
	})
	for t := range p.Dock {
		for _, post := range p.Dock[t].Posts {
			if strconv.FormatInt(post.ID, 10) == id {
				comments, found = post.Comments, true
			}
		}
		for _, question := range p.Dock[t].Questions {
			for _, answer := range question.Answers {
				if strconv.FormatInt(answer.ID, 10) == id {
					comments, found = answer.Comments, true
				}
			}
		}
	}
	a.eachTodoListAndGroup(p, func(list *TodoList) {
		if strconv.FormatInt(list.ID, 10) == id {
			comments, found = list.Comments, true
		}
	})
	a.eachTodo(p, func(one *Todo) {
		if strconv.FormatInt(one.ID, 10) == id {
			comments, found = one.Comments, true
		}
	})
	return comments, found
}

// boostsOf finds the boosts of a card, a message, a comment or a line by its
// ID.
func (a *Account) boostsOf(p *Project, id string) ([]Boost, bool) {
	var boosts []Boost
	found := false
	is := func(candidate int64, of []Boost) {
		if strconv.FormatInt(candidate, 10) == id {
			boosts, found = of, true
		}
	}
	a.eachCard(p, func(_ *List, _ *OnHold, card *Card) {
		is(card.ID, card.Boosts)
		for _, comment := range card.Comments {
			is(comment.ID, comment.Boosts)
		}
	})
	for t := range p.Dock {
		for _, post := range p.Dock[t].Posts {
			is(post.ID, post.Boosts)
			for _, comment := range post.Comments {
				is(comment.ID, comment.Boosts)
			}
		}
		for _, line := range p.Dock[t].Lines {
			is(line.ID, line.Boosts)
		}
		for _, question := range p.Dock[t].Questions {
			for _, answer := range question.Answers {
				is(answer.ID, answer.Boosts)
				for _, comment := range answer.Comments {
					is(comment.ID, comment.Boosts)
				}
			}
		}
	}
	a.eachTodoListAndGroup(p, func(list *TodoList) {
		is(list.ID, list.Boosts)
		for _, comment := range list.Comments {
			is(comment.ID, comment.Boosts)
		}
	})
	a.eachTodo(p, func(one *Todo) {
		is(one.ID, one.Boosts)
		for _, comment := range one.Comments {
			is(comment.ID, comment.Boosts)
		}
	})
	return boosts, found
}

func (a *Account) postJSON(p *Project, board *Tool, post *Post) map[string]any {
	answer := map[string]any{
		"id": post.ID, "status": post.status(), "subject": post.Subject, "title": post.Subject,
		"content": post.Content, "content_attachments": a.filesJSON(post.Files),
		"parent":         map[string]any{"id": board.ID, "type": "Message::Board", "title": board.Title},
		"comments_count": len(post.Comments), "comments_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/comments.json", a.root(), p.ID, post.ID),
		"boosts_count": len(post.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, post.ID),
		"created_at": post.CreatedAt, "updated_at": post.UpdatedAt,
	}
	if post.ShownStatus != "" {
		answer["status"] = post.ShownStatus
	}
	// A message without a category has no such key, and its title no emoji.
	if post.Category != nil {
		answer["category"] = map[string]any{"id": post.Category.ID, "name": post.Category.Name, "icon": post.Category.Icon}
		answer["title"] = post.Category.Icon + " " + post.Subject
	}
	if post.Creator != nil {
		answer["creator"] = a.person(*post.Creator)
	}
	return answer
}

func (a *Account) lineJSON(p *Project, chat *Tool, line Line) map[string]any {
	status := line.Status
	if status == "" {
		status = "active"
	}
	answer := map[string]any{
		"id": line.ID, "status": status, "type": line.Type, "title": "A line",
		"parent":       map[string]any{"id": chat.ID, "type": "Chat::Transcript", "title": chat.Title},
		"boosts_count": len(line.Boosts), "boosts_url": fmt.Sprintf("%sbuckets/%d/recordings/%d/boosts.json", a.root(), p.ID, line.ID),
		"created_at": line.CreatedAt, "updated_at": line.UpdatedAt,
	}
	if line.Creator != nil {
		answer["creator"] = a.person(*line.Creator)
	}
	if line.Type != UploadLine {
		answer["content"] = line.Content
		return answer
	}
	uploads := []map[string]any{}
	for _, upload := range line.Uploads {
		address, download := upload.URL, upload.DownloadURL
		if address == "" {
			address = fmt.Sprintf("https://preview.basecamp.test/%s/blobs/%s/previews/full", a.ID, upload.Key)
		}
		if download == "" {
			download = a.downloadURL(upload.Key, upload.urlName())
		}
		uploads = append(uploads, map[string]any{
			"title": upload.Filename, "filename": upload.Filename, "content_type": upload.ContentType, "byte_size": upload.Bytes,
			"url": address, "download_url": download,
		})
	}
	answer["attachments"] = uploads
	return answer
}

// gearedPageOf is a page of a list whose pages grow, as those of Basecamp do:
// the second holds twice and the third three times what the first holds, and
// every further one four times.
func (a *Account) gearedPageOf(items []map[string]any, page int) []byte {
	if items == nil {
		items = []map[string]any{}
	}
	start := 0
	for n := 1; n < max(page, 1); n++ {
		start += a.PageSize * min(n, 4)
	}
	start = min(start, len(items))
	end := min(start+a.PageSize*min(max(page, 1), 4), len(items))
	return encode(items[start:end])
}
