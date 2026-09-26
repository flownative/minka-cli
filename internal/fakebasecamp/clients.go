package fakebasecamp

import "fmt"

// northwind is the company the client project works with, and the company of
// its clients: a person and a project that name one company name the same
// identifier.
var northwind = Company{ID: 9281, Name: "Northwind Example"}

// nameless is a company whose name is no name. No bundle carries one, because
// Minka would end its run over it.
var nameless = Company{ID: 9282, Name: "  "}

// clientPeople are the people the client project works with: a client of a
// company, somebody of the kind Basecamp had for a client before the flag, a
// client whose company the source does not name, and one whose company has no
// name.
func clientPeople() (iris, otto, nora, cora Person) {
	iris = Person{ID: 106, Name: "Iris Outside", Email: "iris@northwind.test", Client: true, Company: &northwind,
		CreatedAt: "2021-01-01T08:00:00.000Z", UpdatedAt: "2024-06-01T08:00:00.000Z"}
	otto = Person{ID: 107, Name: "Otto Older", Kind: "Client", Email: "otto@northwind.test",
		CreatedAt: "2021-02-01T08:00:00.000Z", UpdatedAt: "2024-06-02T08:00:00.000Z"}
	nora = Person{ID: 108, Name: "Nora Nowhere", Email: "nora@example.test", Client: true,
		CreatedAt: "2021-03-01T08:00:00.000Z", UpdatedAt: "2024-06-03T08:00:00.000Z"}
	cora = Person{ID: 109, Name: "Cora Unnamed", Email: "cora@example.test", Client: true, Company: &nameless,
		CreatedAt: "2021-04-01T08:00:00.000Z", UpdatedAt: "2024-06-04T08:00:00.000Z"}
	return iris, otto, nora, cora
}

// clientProject is a project that works with clients, with every case of what
// the source says about what they see: two tools of one kind whose word differs,
// a tool shown to clients holding a post and a list that are kept from them, a
// tool kept from them holding a post and a question that are shown, and a group
// and a to-do whose word falls to what they hang on. Its Docs & Files and
// Forwards are the tools a run does not extract, whose word no bundle carries.
//
// Its members are an employee and the four kinds of client, so that a run says
// how many memberships Minka will leave out.
func clientProject(ada, iris, otto, nora, cora Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2025-09-%02dT%02d:00:00.000Z", day, hour) }
	made := tool
	tool = func(id int64, name, kind, title string, enabled bool, position int, creator *Person) Tool {
		t := made(id, name, kind, title, enabled, position, creator)
		t.CreatedAt, t.UpdatedAt = at(1, 7), at(10, 12)
		return t
	}
	shown, hidden := true, false

	// A board the clients see, holding the one post the source keeps from them.
	news := tool(9220, "message_board", "message_boards", "Client news", true, 1, &ada)
	news.VisibleToClients = &shown
	news.Posts = []Post{
		{ID: 9231, Subject: "Only for us", Creator: &ada, CreatedAt: at(2, 8), UpdatedAt: at(2, 8),
			Content: `<div>Not for the client.</div>`, VisibleToClients: &hidden},
		{ID: 9232, Subject: "For everybody", Creator: &ada, CreatedAt: at(2, 9), UpdatedAt: at(2, 9),
			Content: `<div>Welcome.</div>`, VisibleToClients: &shown},
	}

	// The other way round: a board they do not see, holding a post they do.
	notes := tool(9221, "message_board", "message_boards", "Internal notes", true, 2, &ada)
	notes.VisibleToClients = &hidden
	notes.Posts = []Post{
		{ID: 9233, Subject: "Shared all the same", Creator: &ada, CreatedAt: at(2, 10), UpdatedAt: at(2, 10),
			Content: `<div>Handed over.</div>`, VisibleToClients: &shown},
	}

	// Two chats of one project whose word differs, as the source has them.
	chat := tool(9222, "chat", "chats", "Chat", true, 3, &ada)
	chat.VisibleToClients = &hidden
	clientChat := tool(9223, "chat", "chats", "Client Chat", true, 4, &ada)
	clientChat.VisibleToClients = &shown

	// A tool the clients see, holding a list they do not, and in that list a
	// group and a to-do whose word falls to the list.
	todos := tool(9224, "todoset", "todosets", "Client to-dos", true, 5, &ada)
	todos.VisibleToClients = &shown
	todos.TodoLists = []TodoList{
		{ID: 9241, Title: "Ours alone", Creator: &ada, CreatedAt: at(3, 8), UpdatedAt: at(3, 8), VisibleToClients: &hidden,
			Groups: []TodoList{{ID: 9251, Title: "This week", Creator: &ada, CreatedAt: at(3, 9), UpdatedAt: at(3, 9), VisibleToClients: &hidden}},
			Todos: []Todo{{ID: 9261, Title: "Write the offer", Creator: &ada, CreatedAt: at(3, 10), UpdatedAt: at(3, 10),
				VisibleToClients: &shown}}},
		{ID: 9242, Title: "Together", Creator: &ada, CreatedAt: at(3, 11), UpdatedAt: at(3, 11), VisibleToClients: &shown},
	}

	// A tool they do not see, holding a question they do.
	checkins := tool(9225, "questionnaire", "questionnaires", "Check-ins", true, 6, &ada)
	checkins.VisibleToClients = &hidden
	checkins.Questions = []Question{
		{ID: 9271, Title: "How is the handover?", Creator: &ada, CreatedAt: at(4, 8), UpdatedAt: at(4, 8),
			Schedule:    Schedule{Frequency: "every_week", Days: []int{1}, Hour: 9, StartDate: "2025-09-01"},
			Subscribers: []Person{ada}, VisibleToClients: &shown},
	}

	return Project{
		ID: 7009, Name: "Client Project", Status: "active",
		CreatedAt: at(1, 7), UpdatedAt: at(10, 12),
		ClientsEnabled: true, ClientCompany: &northwind,
		Members: []int64{ada.ID, iris.ID, otto.ID, nora.ID, cora.ID},
		Dock: []Tool{news, notes, chat, clientChat, todos, checkins,
			tool(9226, "vault", "vaults", "Docs & Files", true, 7, &ada),
			tool(9227, "inbox", "inboxes", "Forwards", true, 8, &ada)},
	}
}
