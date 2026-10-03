package fakebasecamp

import (
	"fmt"
	"html"
)

// Link is a link as Basecamp keeps it in a text.
func Link(address, words string) string {
	return fmt.Sprintf(`<a href="%s">%s</a>`, html.EscapeString(address), words)
}

// referenceProject is a project whose texts link to cards, messages, chats
// and entries of a calendar, to a line of a chat and a day of an entry, of
// this project and of others, at both hosts Basecamp shows its pages at, with
// an anchor and below a page; and to what is none of them: a to-do, a
// document, the project, a card table, another account, another host, a
// stored file, and what stands below an entry that is no day of it. Every
// kind of text holds such links: the note of a card, a comment, a message, a
// line and the preview of a link in a line.
func referenceProject(ada, grace Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2023-06-%02dT%02d:00:00.000Z", day, hour) }
	page := func(path string) string { return "https://3.basecamp.com/4242/" + path }

	board := tool(9060, "kanban_board", "card_tables", "Linked board", true, 1, &ada)
	board.CreatedAt, board.UpdatedAt = "2023-06-01T07:00:00.100Z", at(1, 8)
	note := `<div>Next: ` + Link(page("buckets/7006/card_tables/cards/9962"), "the second card") +
		`, as said in <a href="https://app.basecamp.com/4242/buckets/7003/card_tables/cards/9201#__recording_9301" target="_blank">a comment elsewhere</a>.</div>` +
		`<div>Not ours: ` + Link(page("buckets/7006/todos/1234"), "a to-do") + `, ` +
		Link(page("buckets/7006/card_tables/9060"), "the table") + `, ` +
		Link("https://3.basecamp.com/9999/buckets/7006/card_tables/cards/9962", "another account") + `, ` +
		Link("https://example.test/4242/buckets/7006/card_tables/cards/9962", "another host") + `.</div>`
	board.Lists = []List{
		{ID: 9161, Type: Triage, Title: "Triage", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8), Cards: []Card{
			{
				ID: 9961, Title: "Links", Position: 1, Creator: &ada, CreatedAt: at(2, 8), UpdatedAt: at(2, 9),
				Content: note, Description: note,
				Comments: []Comment{
					{ID: 9971, Creator: &grace, CreatedAt: at(2, 10), UpdatedAt: at(2, 10),
						Content: `<div>See ` + Link(page("buckets/7006/messages/9982"), "<strong>the plan</strong>") + ` and ` +
							Link("http://app.basecamp.com/4242/buckets/7004/messages/9701#__recording_9751", "what was said") + `.</div>`},
				},
			},
			{ID: 9962, Title: "Linked to", Position: 2, Creator: &grace, CreatedAt: at(3, 8), UpdatedAt: at(3, 8)},
		}},
		{ID: 9162, Type: NotNow, Title: "Not now", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8)},
		{ID: 9163, Type: Done, Title: "Done", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8)},
	}

	news := tool(9061, "message_board", "message_boards", "Linked news", true, 2, &ada)
	news.CreatedAt, news.UpdatedAt = at(1, 9), at(1, 9)
	news.Posts = []Post{
		{ID: 9981, Subject: "Where it was said", Creator: &ada, CreatedAt: at(4, 8), UpdatedAt: at(4, 8),
			Content: `<div>In the chat: ` + Link(page("buckets/7006/chats/9062"), "the room") + `, ` +
				Link(page("buckets/7006/chats/9062@9993"), "a line") + `, ` +
				Link(page("buckets/7004/chats/9033/lines/9801/options"), "a line elsewhere") + `.</div>` +
				`<div>Not ours: ` + Link(page("projects/7006"), "the project") + `, ` +
				Link(page("buckets/7006/documents/77"), "a document") + `, ` +
				Link(page("buckets/7006/message_boards/9061"), "the board") + `, ` +
				Link("https://storage.3.basecamp.com/4242/blobs/"+FileKey(1)+"/download/x.png", "a stored file") + `, ` +
				Link("https://3.basecamp.com.example.test/4242/buckets/7006/messages/9982", "a lookalike") + `, ` +
				Link(page("buckets/7006/messages/9982@1"), "a message with a line") + `.</div>`},
		{ID: 9982, Subject: "The plan", Creator: &grace, CreatedAt: at(4, 9), UpdatedAt: at(4, 9), Content: `<div>Plan.</div>`},
		{ID: 9983, Subject: "When", Creator: &ada, CreatedAt: at(4, 10), UpdatedAt: at(4, 10),
			Content: `<div>Meet at ` + Link(page("buckets/7006/schedule_entries/9441"), "the kick-off") + `, ` +
				Link(page("buckets/7006/schedule_entries/9442/occurrences/20230512"), "one Friday") + `, ` +
				Link("https://app.basecamp.com/4242/buckets/7004/schedule_entries/9443/occurrences/20240229#__recording_9444", "a leap day elsewhere") + `.</div>` +
				`<div>Not ours: ` + Link("https://3.basecamp.com/9999/buckets/7006/schedule_entries/9441", "another account") + `, ` +
				Link(page("buckets/7006/schedule_entries/9441x"), "letters after the ID") + `, ` +
				Link(page("buckets/7006/schedule_entries/9441.json"), "the API") + `, ` +
				Link(page("buckets/7006/schedule_entries/9441/edit"), "a page below the entry") + `, ` +
				Link(page("buckets/7006/schedule_entries/9441@1"), "an entry with a line") + `, ` +
				Link(page("buckets/7006/schedule_entries/9442/occurrences/20230230"), "the thirtieth of February") + `, ` +
				Link(page("buckets/7006/schedule_entries/9442/occurrences/2023051"), "seven digits") + `, ` +
				Link(page("buckets/7006/schedule_entries/9442/occurrences/20230512/subscription"), "below a day") + `, ` +
				Link(page("buckets/7006/schedule_entries/9442/occurrences"), "no day") + `.</div>`},
	}

	chat := tool(9062, "chat", "chats", "Linked chat", true, 3, &ada)
	chat.CreatedAt, chat.UpdatedAt = at(1, 10), at(1, 10)
	chat.Lines = []Line{
		{ID: 9991, Type: RichTextLine, Creator: &ada, CreatedAt: at(5, 8), UpdatedAt: at(5, 8),
			Content: `<div>` + LinkPreview(page("buckets/7006/card_tables/cards/9961"), "Links") + ` and <a name="top">an anchor</a></div>`},
		{ID: 9992, Type: TextLine, Creator: &grace, CreatedAt: at(5, 9), UpdatedAt: at(5, 9),
			Content: `see ` + Link("https://APP.basecamp.com/4242/buckets/7006/card_tables/cards/9962", "https://APP.basecamp.com/4242/buckets/7006/card_tables/cards/9962")},
		{ID: 9993, Type: TextLine, Creator: &ada, CreatedAt: at(5, 10), UpdatedAt: at(5, 10), Content: `Here`},
	}

	return Project{
		ID: 7006, Name: "Reference Project", Status: "active",
		CreatedAt: "2023-06-01T07:00:00.000Z", UpdatedAt: "2023-06-05T10:00:00.000Z",
		Members: []int64{101, 102},
		Dock:    []Tool{board, news, chat},
	}
}
