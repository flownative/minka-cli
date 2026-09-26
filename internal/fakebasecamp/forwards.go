package fakebasecamp

import (
	"fmt"
	"regexp"
	"strconv"
)

// Forward is a mail somebody forwarded into a project. The account says
// neither how many comments nor how many boosts it has, as Basecamp does not,
// and names the sender by their address alone.
type Forward struct {
	ID int64
	// Status is "active" when empty. ShownStatus replaces what the forward says
	// about it, and SaysNothingOfItsStatus leaves the field out of the answer
	// altogether, which is a Basecamp nobody has met.
	Status                 string
	ShownStatus            string
	SaysNothingOfItsStatus bool
	// SaysNothingOfItsReplyCount leaves the count of the answers out of the
	// answer, which is another Basecamp nobody has met.
	SaysNothingOfItsReplyCount bool
	Subject                    string
	From                       string
	Content                    string
	Creator                    *Person
	CreatedAt                  string
	UpdatedAt                  string
	Files                      []File
	Replies                    []Reply
	Comments                   []Comment
	Boosts                     []Boost
	// VisibleToClients is what the forward says about clients, which a bundle has
	// no word for: an Email forwards tool is shown to them as a whole.
	VisibleToClients *bool
}

// Reply is an answer by mail to a forward. Its writer is somebody of the
// account or somebody from outside it, whom Basecamp gives the kind
// "Outsider" and their address as their name.
type Reply struct {
	ID int64
	// Status is "active" when empty, as a forward's is, and
	// SaysNothingOfItsStatus leaves the field out of the answer.
	Status                 string
	SaysNothingOfItsStatus bool
	Content                string
	Creator                *Person
	CreatedAt              string
	UpdatedAt              string
	Files                  []File
	Boosts                 []Boost
	// VisibleToClients is what the reply says about clients, which follows its
	// tool as a forward's does.
	VisibleToClients *bool
}

// Outsider is somebody who wrote to a project from their own mailbox: no
// member of the account, and nobody it can be asked about. Basecamp gives such
// a writer their address as their name, and the field for an address with it.
func Outsider(id int64, address string) Person {
	return Person{ID: id, Kind: "Outsider", Name: address, Email: address,
		CreatedAt: "2025-02-01T08:00:00.000Z", UpdatedAt: "2025-02-01T08:00:00.000Z"}
}

// OutsiderByNameAlone is one whose address stands in their name and nowhere
// else, which is all Basecamp is known to promise of somebody outside.
func OutsiderByNameAlone(id int64, address string) Person {
	writer := Outsider(id, address)
	writer.Email = ""
	return writer
}

// forwardProject is a project with every case of forwarded mail: two inboxes
// that are switched on and one that is switched off, a mail with words, a file,
// comments and boosts, one without a subject, one whose subject is longer than a
// title holds and runs over two lines, one on the shelf and one in the trash, one
// of a status nobody knows, one whose sender is no address a bundle can carry,
// one that is nothing but a file the source no longer has, one that says nothing
// at all, one that is nothing but an embedded player, one nobody is named for,
// replies from inside and outside the account, one of those named by their name
// alone and one whose address Minka would refuse, a reply in the trash, and a
// boost on a reply that comes and one on a reply that does not, and an answer
// that is nothing but a file the source no longer has.
func forwardProject(ada, grace, former Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2025-02-%02dT%02d:00:00.000Z", day, hour) }
	made := tool
	tool = func(id int64, name, kind, title string, enabled bool, position int, creator *Person) Tool {
		t := made(id, name, kind, title, enabled, position, creator)
		t.CreatedAt, t.UpdatedAt = at(1, 7), at(20, 12)
		return t
	}

	anna := Outsider(901, "anna@supplier.test")
	// Somebody from outside whose address a mail server would take and Minka
	// would not: no domain of its own.
	daemon := Outsider(902, "postmaster@localhost")
	// And one the account names by their name alone.
	otis := OutsiderByNameAlone(903, "otis@partner.test")

	invoice := File{ID: 5301, SGID: SGID("gid://bc3/Attachment/5301?expires_in"), Key: FileKey(5301),
		Filename: "invoice.pdf", ContentType: "application/pdf", Bytes: 3072}
	signed := File{ID: 5302, SGID: SGID("gid://bc3/Attachment/5302?expires_in"), Key: FileKey(5302),
		Filename: "signed.pdf", ContentType: "application/pdf", Bytes: 1536}
	lost := File{ID: 5303, SGID: SGID("gid://bc3/Attachment/5303?expires_in"), Key: FileKey(5303),
		Filename: "contract.pdf", ContentType: "application/pdf", Bytes: 2048, Gone: true}
	lostToo := File{ID: 5304, SGID: SGID("gid://bc3/Attachment/5304?expires_in"), Key: FileKey(5304),
		Filename: "amendment.pdf", ContentType: "application/pdf", Bytes: 1024, Gone: true}

	// A mail as Basecamp holds one: text with line breaks, an auto-linked
	// address and a figure for the file.
	enquiry := Forward{
		ID: 9301, Subject: "Invoice for March", From: "billing@supplier.test", Creator: &ada,
		CreatedAt: at(2, 8), UpdatedAt: at(2, 8), Files: []File{invoice},
		Content: "Dear all,\n\nthe invoice is attached, see also\n" +
			`<a href="https://3.basecamp.com/4242/buckets/7003/card_tables/cards/9201">the card</a>` +
			"\n\nKind regards\n" + Embed{File: invoice}.HTML(),
		Replies: []Reply{
			{ID: 9401, Creator: &grace, CreatedAt: at(3, 9), UpdatedAt: at(3, 9),
				Content: `<div>Thank you, it is booked.</div>`},
			{ID: 9402, Creator: &anna, CreatedAt: at(3, 10), UpdatedAt: at(3, 10),
				Content: `<div>Here is the signed copy.</div>` + Embed{File: signed}.HTML(), Files: []File{signed},
				Boosts: []Boost{{ID: 9591, Content: "👍", CreatedAt: at(3, 11), Booster: &grace}}},
			{ID: 9403, Status: "trashed", Creator: &grace, CreatedAt: at(3, 12), UpdatedAt: at(3, 13),
				Content: `<div>Sent to the wrong mail.</div>`,
				Boosts:  []Boost{{ID: 9594, Content: "\U0001f44d", CreatedAt: at(3, 13), Booster: &ada}}},
		},
		Comments: []Comment{
			{ID: 9501, Creator: &grace, CreatedAt: at(4, 9), UpdatedAt: at(4, 9), Content: `<div>Paid.</div>`,
				Boosts: []Boost{{ID: 9592, Content: "🎉", CreatedAt: at(4, 10), Booster: &ada}}},
			{ID: 9502, Creator: &former, CreatedAt: at(4, 11), UpdatedAt: at(4, 11), Content: `<div>Filed away.</div>`},
		},
		Boosts: []Boost{{ID: 9593, Content: "👏", CreatedAt: at(4, 12), Booster: &grace}},
	}

	inbox := tool(9290, "inbox", "inboxes", "Email Forwards", true, 1, &ada)
	inbox.CreatedAt = "2025-02-01T07:00:00.100Z"
	inbox.Forwards = []Forward{
		enquiry,
		// A mail nobody gave a subject, held as the plain text it is.
		{ID: 9302, From: "noreply@notifications.test", Creator: &ada, CreatedAt: at(5, 8), UpdatedAt: at(5, 8),
			Content: "Build 42 failed.\n\nSee the log.",
			Replies: []Reply{
				{ID: 9404, Creator: &daemon, CreatedAt: at(5, 9), UpdatedAt: at(5, 9),
					Content: `<div>Delivery failed.</div>`},
				{ID: 9405, Creator: &otis, CreatedAt: at(5, 10), UpdatedAt: at(5, 10),
					Content: `<div>What is build 42?</div>`},
			}},
		// A subject over two lines and longer than a title holds.
		{ID: 9303, Subject: "Re:\tFwd: " + LongSubject, From: "office@supplier.test", Creator: &grace,
			CreatedAt: at(6, 8), UpdatedAt: at(6, 8), Content: "See below."},
		// On the shelf at the source, which is the trash here.
		{ID: 9304, Status: "archived", Subject: "Old enquiry", From: "hello@supplier.test", Creator: &ada,
			CreatedAt: at(7, 8), UpdatedAt: at(9, 8), Content: "Is this still open?",
			// An answer that is nothing but a file the source no longer has.
			Replies: []Reply{{ID: 9406, Creator: &grace, CreatedAt: at(8, 9), UpdatedAt: at(8, 9),
				Content: Embed{File: lostToo}.HTML(), Files: []File{lostToo}}},
			Comments: []Comment{{ID: 9503, Creator: &ada, CreatedAt: at(8, 8), UpdatedAt: at(8, 8), Content: `<div>Closed.</div>`}}},
		{ID: 9305, Status: "trashed", Subject: "Advertising", From: "sales@supplier.test", Creator: &ada,
			CreatedAt: at(10, 8), UpdatedAt: at(11, 8), Content: "Special offer."},
		// A word a bundle has none for, on a mail that is in use.
		{ID: 9306, ShownStatus: "quarantined", Subject: "Held back", From: "unknown@supplier.test", Creator: &ada,
			CreatedAt: at(12, 8), UpdatedAt: at(12, 8), Content: "Nobody knows."},
		// An address a mail server took and Minka does not.
		{ID: 9307, Subject: "Returned mail", From: "root@localhost", Creator: &ada,
			CreatedAt: at(13, 8), UpdatedAt: at(13, 8), Content: "Undeliverable."},
		// Nothing but a file the source no longer has.
		{ID: 9308, Subject: "The contract", From: "legal@supplier.test", Creator: &ada,
			CreatedAt: at(14, 8), UpdatedAt: at(14, 8), Files: []File{lost},
			Content: Embed{File: lost}.HTML()},
		// A mail that says nothing at all.
		{ID: 9309, Subject: "No words", From: "quiet@supplier.test", Creator: &ada,
			CreatedAt: at(15, 8), UpdatedAt: at(15, 8), Content: "  \n  "},
		// Nothing but a player, which Minka takes by a rule of its own.
		{ID: 9310, Subject: "A recording", From: "studio@supplier.test", Creator: &ada,
			CreatedAt: at(16, 8), UpdatedAt: at(16, 8),
			Content: `<figure><iframe src="https://video.example.test/embed/7"></iframe></figure>`},
		// A mail Basecamp names nobody for.
		{ID: 9311, Subject: "From nobody in particular", From: "anon@supplier.test",
			CreatedAt: at(17, 8), UpdatedAt: at(17, 8), Content: "Anonymous."},
	}

	second := tool(9292, "inbox", "inboxes", "Second inbox", true, 2, &grace)
	second.Forwards = []Forward{
		{ID: 9321, Subject: "Another mailbox", From: "post@supplier.test", Creator: &grace,
			CreatedAt: at(18, 8), UpdatedAt: at(18, 8), Content: "Over here."},
	}

	// Switched off, and so never asked for: what it holds must reach no bundle.
	closed := tool(9291, "inbox", "inboxes", "Old forwards", false, 0, &ada)
	closed.Forwards = []Forward{
		{ID: 9331, Subject: "Behind a switch", From: "old@supplier.test", Creator: &ada,
			CreatedAt: at(19, 8), UpdatedAt: at(19, 8), Content: "Nobody reads this."},
	}

	return Project{
		ID: 7010, Name: "Forward Project", Status: "active",
		CreatedAt: "2025-02-01T07:00:00.000Z", UpdatedAt: "2025-02-20T12:00:00.000Z",
		Members: []int64{101, 102},
		Dock:    []Tool{inbox, second, closed},
	}
}

// LongSubject is a subject of more characters than a title holds.
var LongSubject = "A subject that will not stop going on and on about the very thing it is about, " +
	"written by somebody who never met a full stop they liked, and carrying on well past the point " +
	"at which a title here has to give up and end in an ellipsis instead."

var (
	forwardsPattern = regexp.MustCompile(`^buckets/([0-9]+)/inboxes/([0-9]+)/forwards\.json$`)
	repliesPattern  = regexp.MustCompile(`^buckets/([0-9]+)/inbox_forwards/([0-9]+)/replies\.json$`)
)

// forwards answers the paths of Email forwards tools. The second value tells
// whether the path was one of them.
func (a *Account) forwards(path string, page int) ([]byte, bool) {
	switch {
	case forwardsPattern.MatchString(path):
		match := forwardsPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for t := range p.Dock {
				tool := &p.Dock[t]
				if tool.Type != "inboxes" || strconv.FormatInt(tool.ID, 10) != match[2] {
					continue
				}
				// Newest first, as an inbox shows its mail.
				found := []map[string]any{}
				for i := len(tool.Forwards) - 1; i >= 0; i-- {
					found = append(found, a.forwardJSON(p, &tool.Forwards[i]))
				}
				return a.pageOf(a.shiftedWhereTheListMoves(found), page), true
			}
		}
	case repliesPattern.MatchString(path):
		match := repliesPattern.FindStringSubmatch(path)
		if p := a.project(match[1]); p != nil {
			for t := range p.Dock {
				for f := range p.Dock[t].Forwards {
					forward := &p.Dock[t].Forwards[f]
					if strconv.FormatInt(forward.ID, 10) != match[2] {
						continue
					}
					// Newest first, as the answers of a mail are shown.
					found := []map[string]any{}
					for i := len(forward.Replies) - 1; i >= 0; i-- {
						found = append(found, a.replyJSON(forward.Replies[i]))
					}
					return a.pageOf(a.shiftedWhereTheListMoves(found), page), true
				}
			}
		}
	}
	return nil, false
}

// shiftedWhereTheListMoves hands the last record of the first page out again as
// the first of the second, which is what a list does that something was added
// to between two requests.
func (a *Account) shiftedWhereTheListMoves(found []map[string]any) []map[string]any {
	if !a.MailListMoves || len(found) <= a.PageSize {
		return found
	}
	shifted := append([]map[string]any{}, found[:a.PageSize]...)
	return append(shifted, found[a.PageSize-1:]...)
}

func (f *Forward) status() string {
	if f.Status == "" {
		return "active"
	}
	return f.Status
}

func (r Reply) status() string {
	if r.Status == "" {
		return "active"
	}
	return r.Status
}

// forwardJSON is a forward as Basecamp answers one: no count of its comments
// and none of its boosts, because it has neither.
func (a *Account) forwardJSON(p *Project, forward *Forward) map[string]any {
	answer := map[string]any{
		"id": forward.ID, "status": forward.status(), "subject": forward.Subject, "title": forward.Subject,
		"from": forward.From, "content": forward.Content, "content_attachments": a.filesJSON(forward.Files),
		"replies_count": len(forward.Replies),
		// No count of its comments and none of its boosts: Basecamp gives a
		// forward neither.
		"replies_url": fmt.Sprintf("%sbuckets/%d/inbox_forwards/%d/replies.json", a.root(), p.ID, forward.ID),
		"created_at":  forward.CreatedAt, "updated_at": forward.UpdatedAt,
	}
	if forward.SaysNothingOfItsStatus {
		delete(answer, "status")
	}
	if forward.SaysNothingOfItsReplyCount {
		delete(answer, "replies_count")
	}
	if forward.VisibleToClients != nil {
		answer["visible_to_clients"] = *forward.VisibleToClients
	}
	if forward.ShownStatus != "" {
		answer["status"] = forward.ShownStatus
	}
	if forward.Creator != nil {
		answer["creator"] = a.person(*forward.Creator)
	}
	return answer
}

// replyJSON is a reply as Basecamp answers one: with what became of it, as a
// forward comes, and with no count of comments, which a reply cannot have.
func (a *Account) replyJSON(reply Reply) map[string]any {
	answer := map[string]any{
		"id": reply.ID, "status": reply.status(), "content": reply.Content, "content_attachments": a.filesJSON(reply.Files),
		"boosts_count": len(reply.Boosts), "created_at": reply.CreatedAt, "updated_at": reply.UpdatedAt,
	}
	if reply.SaysNothingOfItsStatus {
		delete(answer, "status")
	}
	if reply.VisibleToClients != nil {
		answer["visible_to_clients"] = *reply.VisibleToClients
	}
	if reply.Creator != nil {
		answer["creator"] = a.person(*reply.Creator)
	}
	return answer
}

// eachForward visits the forwards of every inbox of a project, those of
// inboxes that are switched off included.
func (a *Account) eachForward(p *Project, visit func(*Forward)) {
	for t := range p.Dock {
		for f := range p.Dock[t].Forwards {
			visit(&p.Dock[t].Forwards[f])
		}
	}
}
