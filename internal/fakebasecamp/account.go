// Package fakebasecamp is an in-memory Basecamp account for tests. It answers
// the paths an extraction asks for the way the Basecamp CLI does: one page per
// call, an empty list behind the last page, a not-found error for what is not
// there. Everybody in it is invented.
package fakebasecamp

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/flownative/minka-cli/internal/basecamp"
)

const apiHost = "https://3.basecampapi.test"

type Person struct {
	ID        int64
	Name      string
	Email     string
	NoAvatar  bool
	CreatedAt string
	UpdatedAt string
}

type Project struct {
	ID          int64
	Name        string
	Description string
	Status      string
	CreatedAt   string
	UpdatedAt   string
	Members     []int64
	Dock        []Tool
	// URL replaces the address the account would give the project.
	URL string
}

// Tool is an entry of a project's dock. Type is the segment of its address,
// such as "chats"; URL replaces the address the account would give it.
type Tool struct {
	ID        int64
	Name      string
	Type      string
	Title     string
	Enabled   bool
	Position  int
	CreatedAt string
	UpdatedAt string
	Creator   *Person
	URL       string
	// Lists are the lists of a card table.
	Lists []List
}

// Account is the fake. It is a basecamp.Source and a basecamp.AvatarFetcher,
// and records what it was asked for.
type Account struct {
	ID       string
	PageSize int
	People   []Person
	Projects []Project

	// Former is somebody who left the account and is only found as a creator.
	Former Person

	// MemberEmail replaces the address of a person in the list of a project.
	MemberEmail map[int64]string

	// OnHoldInColumn lists the cards that are set aside among the cards of
	// their column as well.
	OnHoldInColumn bool
	// OnHoldListEmpty leaves the list of the cards that are set aside empty.
	OnHoldListEmpty bool
	// InlineSteps is how many of its steps a card shows itself; all when 0.
	InlineSteps int
	// SubtasksURL replaces the address a card names for the list of its steps.
	SubtasksURL map[int64]string

	// Fail makes the request of a path, without its page, end in an error.
	Fail map[string]error
	// FailAvatarOf makes the download of one person's avatar end in an error.
	FailAvatarOf map[int64]error

	Requests       []string
	AvatarRequests []string
}

// New is an account with five people, one of whom the first two projects have
// nothing to do with. Those two have a dock that holds every case: several
// tools of a kind, tools Minka has nothing for, tools that are switched off,
// and a tool whose creator has left; their one board is empty. The third
// project holds the boards.
func New() *Account {
	ada := Person{ID: 101, Name: "Ada Example", Email: "ada@example.test", CreatedAt: "2020-01-01T08:00:00.000Z", UpdatedAt: "2024-01-01T08:00:00.000Z"}
	grace := Person{ID: 102, Name: "Grace Sample", Email: "grace@example.test", CreatedAt: "2020-02-01T08:00:00.000Z", UpdatedAt: "2024-02-01T08:00:00.000Z"}
	linus := Person{ID: 103, Name: "Linus Placeholder", CreatedAt: "2020-03-01T08:00:00.000Z", UpdatedAt: "2024-03-01T08:00:00.000Z"}
	edsger := Person{ID: 104, Name: "Edsger Invented", Email: "edsger@example.test", NoAvatar: true, CreatedAt: "2020-04-01T08:00:00.000Z", UpdatedAt: "2024-04-01T08:00:00.000Z"}
	niklaus := Person{ID: 105, Name: "Niklaus Nowhere", Email: "niklaus@example.test", CreatedAt: "2020-05-01T08:00:00.000Z", UpdatedAt: "2024-05-01T08:00:00.000Z"}
	former := Person{ID: 99, Name: "Former Colleague", Email: "former@example.test", CreatedAt: "2019-01-01T08:00:00.000Z", UpdatedAt: "2021-01-01T08:00:00.000Z"}

	tool := func(id int64, name, kind, title string, enabled bool, position int, creator *Person) Tool {
		return Tool{ID: id, Name: name, Type: kind, Title: title, Enabled: enabled, Position: position, Creator: creator,
			CreatedAt: fmt.Sprintf("2022-05-%02dT10:00:00.000Z", id-9000), UpdatedAt: fmt.Sprintf("2025-05-%02dT10:00:00.000Z", id-9000)}
	}

	bugs := tool(9003, "kanban_board", "card_tables", "Bugs", true, 3, &former)
	for i, kind := range []string{Triage, NotNow, Done} {
		bugs.Lists = append(bugs.Lists, List{ID: 9091 + int64(i), Type: kind, Title: "A column every table has", Creator: &former,
			CreatedAt: bugs.CreatedAt, UpdatedAt: bugs.UpdatedAt})
	}

	return &Account{
		ID:       "4242",
		PageSize: 2,
		People:   []Person{grace, ada, edsger, linus, niklaus},
		Former:   former,
		Projects: []Project{
			{
				ID: 7001, Name: "Example Project", Status: "active",
				Description: "First line\nsecond <line>\n\nLast & least",
				CreatedAt:   "2022-05-01T09:00:00.000Z", UpdatedAt: "2025-06-01T09:00:00.000Z",
				Members: []int64{103, 101, 104, 102},
				Dock: []Tool{
					tool(9001, "chat", "chats", "Campfire", true, 1, &ada),
					tool(9002, "message_board", "message_boards", "Announcements", true, 2, &ada),
					bugs,
					tool(9004, "chat", "chats", "Second chat", true, 4, &grace),
					tool(9005, "todoset", "todosets", "To-dos", true, 5, &ada),
					tool(9006, "schedule", "schedules", "Schedule", false, 0, &ada),
					tool(9007, "kanban_board", "card_tables", "Old board", false, 0, &ada),
					tool(9008, "vault", "vaults", "Docs", true, 6, &ada),
					tool(9009, "todoset", "todosets", "More to-dos", true, 7, &ada),
				},
			},
			{
				ID: 7002, Name: "Second Project", Status: "archived",
				CreatedAt: "2023-01-01T09:00:00.000Z", UpdatedAt: "2023-06-01T09:00:00.000Z",
				Members: []int64{101},
				Dock:    []Tool{tool(9010, "chat", "chats", "Campfire", true, 1, nil)},
			},
			boardProject(ada, grace, linus, edsger, former, tool),
		},
		SubtasksURL:  map[int64]string{},
		Fail:         map[string]error{},
		FailAvatarOf: map[int64]error{},
	}
}

func (a *Account) root() string {
	return apiHost + "/" + a.ID + "/"
}

func (a *Account) avatarURL(p Person) string {
	if p.NoAvatar {
		return ""
	}
	return fmt.Sprintf("https://avatars.basecamp.test/%s/people/%d/avatar", a.ID, p.ID)
}

// AvatarOf is what the download of a person's avatar answers.
func AvatarOf(id int64) []byte {
	return []byte(fmt.Sprintf("the picture of person %d", id))
}

func (a *Account) person(p Person) map[string]any {
	return map[string]any{
		"id": p.ID, "name": p.Name, "email_address": p.Email, "avatar_url": a.avatarURL(p),
		"created_at": p.CreatedAt, "updated_at": p.UpdatedAt, "personable_type": "User",
	}
}

var (
	pagePattern    = regexp.MustCompile(`^(.*)[?&]page=([0-9]+)$`)
	projectPattern = regexp.MustCompile(`^projects/([0-9]+)\.json$`)
	membersPattern = regexp.MustCompile(`^projects/([0-9]+)/people\.json$`)
	toolPattern    = regexp.MustCompile(`^buckets/([0-9]+)/([a-z_]+)/([0-9]+)\.json$`)
	avatarPattern  = regexp.MustCompile(`/people/([0-9]+)/avatar$`)
)

func (a *Account) Get(_ context.Context, path string) ([]byte, error) {
	a.Requests = append(a.Requests, path)

	page := 0
	if match := pagePattern.FindStringSubmatch(path); match != nil {
		path = match[1]
		page, _ = strconv.Atoi(match[2])
	}
	if err := a.Fail[path]; err != nil {
		return nil, err
	}

	switch {
	case path == "people.json":
		return a.page(a.People, page), nil
	case projectPattern.MatchString(path):
		if p := a.project(projectPattern.FindStringSubmatch(path)[1]); p != nil {
			return encode(a.projectJSON(*p)), nil
		}
	case membersPattern.MatchString(path):
		if p := a.project(membersPattern.FindStringSubmatch(path)[1]); p != nil {
			var members []Person
			for _, id := range p.Members {
				for _, candidate := range a.People {
					if candidate.ID == id {
						if address, replaced := a.MemberEmail[id]; replaced {
							candidate.Email = address
						}
						members = append(members, candidate)
					}
				}
			}
			return a.page(members, page), nil
		}
	case toolPattern.MatchString(path):
		match := toolPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for _, tool := range p.Dock {
				if strconv.FormatInt(tool.ID, 10) == match[3] && tool.Type == match[2] {
					answer := map[string]any{"id": tool.ID, "title": tool.Title, "created_at": tool.CreatedAt, "updated_at": tool.UpdatedAt}
					if tool.Creator != nil {
						answer["creator"] = a.person(*tool.Creator)
					}
					if tool.Type == "card_tables" {
						answer["lists"] = a.listsJSON(p, tool.Lists)
					}
					return encode(answer), nil
				}
			}
		}
	default:
		if answer, known := a.boards(path, page); known {
			return answer, nil
		}
	}
	return nil, &basecamp.Error{Kind: basecamp.NotFound, Message: "Basecamp: Resource not found: " + a.root() + path}
}

func (a *Account) page(people []Person, page int) []byte {
	start := (page - 1) * a.PageSize
	if page == 0 {
		start = 0
	}
	items := []map[string]any{}
	for i := start; i < len(people) && i < start+a.PageSize; i++ {
		items = append(items, a.person(people[i]))
	}
	return encode(items)
}

func (a *Account) project(id string) *Project {
	for i := range a.Projects {
		if strconv.FormatInt(a.Projects[i].ID, 10) == id {
			return &a.Projects[i]
		}
	}
	return nil
}

func (a *Account) projectJSON(p Project) map[string]any {
	dock := []map[string]any{}
	for _, tool := range p.Dock {
		address := tool.URL
		if address == "" {
			address = fmt.Sprintf("%sbuckets/%d/%s/%d.json", a.root(), p.ID, tool.Type, tool.ID)
		}
		entry := map[string]any{"id": tool.ID, "name": tool.Name, "title": tool.Title, "enabled": tool.Enabled, "url": address}
		if tool.Enabled {
			entry["position"] = tool.Position
		} else {
			entry["position"] = nil
		}
		dock = append(dock, entry)
	}
	address := p.URL
	if address == "" {
		address = fmt.Sprintf("%sprojects/%d.json", a.root(), p.ID)
	}
	return map[string]any{
		"id": p.ID, "name": p.Name, "description": p.Description, "status": p.Status,
		"created_at": p.CreatedAt, "updated_at": p.UpdatedAt, "url": address, "dock": dock,
	}
}

func (a *Account) Fetch(_ context.Context, address string) (basecamp.Avatar, error) {
	a.AvatarRequests = append(a.AvatarRequests, address)
	match := avatarPattern.FindStringSubmatch(address)
	if match == nil || !strings.HasPrefix(address, "https://") {
		return basecamp.Avatar{}, &basecamp.Error{Kind: basecamp.API, Message: "no avatar at " + address}
	}
	id, _ := strconv.ParseInt(match[1], 10, 64)
	if err := a.FailAvatarOf[id]; err != nil {
		return basecamp.Avatar{}, err
	}
	return basecamp.Avatar{Content: AvatarOf(id), ContentType: "image/webp"}, nil
}

func encode(value any) []byte {
	content, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return content
}
