package basecamp

import (
	"context"
	"fmt"
	"html"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
	nethtml "golang.org/x/net/html"
)

// ForwardCounts says what became of the forwarded mail of a run.
type ForwardCounts struct {
	// ByStatus counts the forwards of the bundle: active, trashed.
	ByStatus map[string]int `json:"byStatus"`
	// FromTheShelf counts the forwards the source keeps on its shelf, which
	// come as trashed: Minka gives a forward no shelf, and its trash is the one
	// place such a mail can be brought back from.
	FromTheShelf int `json:"fromTheShelf"`
	// WithoutASubject counts the mails that came with none and carry the title
	// a run gives them; SubjectsChanged counts those whose subject had to be
	// folded into one line or cut short to make a title of it.
	WithoutASubject int `json:"withoutASubject"`
	SubjectsChanged int `json:"subjectsChanged"`
	// GivenALineOfTheRun counts the forwards and the replies a run wrote a line
	// of its own into, because what the mail says would reach Minka as nothing.
	GivenALineOfTheRun int `json:"givenALineOfTheRun"`
	// WithoutAForwarder counts the forwards Basecamp names nobody for, which
	// Minka gives to the owner of the organization without a word.
	WithoutAForwarder int `json:"withoutAForwarder"`
	// WithoutAnAddress are the forwards left out because what the source calls
	// the sender is no address Minka takes, by sourceId.
	WithoutAnAddress []string `json:"withoutAnAddress"`
	// RepliesFromOutside counts the replies whose sender stays an address,
	// because somebody from outside the account never becomes a person.
	RepliesFromOutside int `json:"repliesFromOutside"`
	// RepliesWithoutASender are the replies left out because the source names
	// neither somebody nor an address, by sourceId.
	RepliesWithoutASender []string `json:"repliesWithoutASender"`
	// ReactionsOnRepliesLeftOut counts the boosts on a reply: Minka passes such
	// a reaction over, so a bundle carries none.
	ReactionsOnRepliesLeftOut int `json:"reactionsOnRepliesLeftOut"`
	// LongerThanMinkaTakes counts the mails and the answers whose body is longer
	// than Minka holds as they are written here. Minka passes such a record over
	// on every run and everything hanging on it waits with it, so the run names
	// how many there are; the body is not cut short.
	LongerThanMinkaTakes int `json:"longerThanMinkaTakes"`
}

// forward is a mail somebody forwarded into a project. Basecamp names the
// sender by their address alone, holds the mail as text with line breaks and a
// few tags in it, and says neither how many comments nor how many boosts it
// has.
type forward struct {
	recording
	ID      int64      `json:"id"`
	Status  string     `json:"status"`
	Subject string     `json:"subject"`
	From    string     `json:"from"`
	Content string     `json:"content"`
	Files   []bodyFile `json:"content_attachments"`
	// RepliesCount is how many answers by mail the forward has, and nil where
	// the source says nothing: then the list is asked for, as the comments and
	// the boosts are, because a mail with no answers costs less than every
	// answer of an account.
	RepliesCount *int `json:"replies_count"`
}

// forwardReply is an answer by mail to a forward. Its writer is somebody of
// the account or somebody from outside it, and the mail says nothing about
// where it came from beyond that. It says what became of it as a forward does,
// and how many boosts it has; it has no comments and so no count of them.
type forwardReply struct {
	recording
	ID          int64      `json:"id"`
	Status      string     `json:"status"`
	Content     string     `json:"content"`
	Files       []bodyFile `json:"content_attachments"`
	BoostsCount int        `json:"boosts_count"`
}

// A record of Basecamp that says nothing of what became of it is in use.
func (f forward) status() string {
	if f.Status == "" {
		return bundle.StatusActive
	}
	return f.Status
}

func (r forwardReply) status() string {
	if r.Status == "" {
		return bundle.StatusActive
	}
	return r.Status
}

// replyStatuses are the words a bundle has for what became of a reply, which
// are none: the record has no such field, so only one the source says is in
// use comes, and whatever else it says of one stays at the source.
var replyStatuses = map[string]bool{bundle.StatusActive: true}

// outsider is the kind of person Basecamp gives somebody who wrote to a
// project from outside the account.
const outsider = "Outsider"

// extractForwards adds the forwarded mail of every Email forwards tool, with
// what hangs on a forward: its replies, its comments and its boosts.
//
// Basecamp lists the forwards of an inbox itself and hands out no list of the
// archived and the trashed ones of a project — it refuses to be asked for one
// — so a bundle holds what an inbox shows. The forwards of an inbox are
// written oldest first, by created_at and then by ID, the inboxes in the order
// of the tools. A walk is no snapshot: a forward met twice is written once.
//
// It comes last of all the kinds, which is a place and not a precondition:
// Minka reads a bundle whole and applies the kinds in an order of its own, so
// nothing a mail points at gains by being written first. What the place decides
// is one thing, and it is why it is the last: a file met in two records belongs
// to the first that named it, so a file that hangs in a card and in a mail
// alike stays the card's.
func (e *Extractor) extractForwards(ctx context.Context, w *bundle.Writer) error {
	for _, t := range e.talks {
		for _, inbox := range t.inboxes {
			listed, err := getList[forward](ctx, e.Source, fmt.Sprintf("buckets/%d/inboxes/%d/forwards.json", t.bucket, inbox))
			if err != nil {
				return err
			}
			// A walk of pages can hand one mail out twice, where the list moved
			// under it between two requests.
			carried := make([]forward, 0, len(listed))
			met := map[int64]bool{}
			for _, found := range listed {
				if met[found.ID] {
					continue
				}
				met[found.ID] = true
				taken, err := e.metAmong(recordStatuses, "forward", found.ID, found.status(), found.recording, "project", sourceID(t.bucket))
				if err != nil {
					return err
				}
				if taken {
					carried = append(carried, found)
				}
			}
			if err := oldestFirst(carried, "forward", func(f forward) (int64, string) { return f.ID, f.CreatedAt }); err != nil {
				return err
			}
			for _, found := range carried {
				if err := e.writeForward(ctx, w, t.bucket, sourceID(inbox), found); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeForward adds one mail with its files, its replies, its comments and its
// boosts.
//
// What the source keeps on its shelf comes as trashed, and the run says how
// many: Minka has no shelf for a forward and would stop at the word, while its
// trash keeps the mail and hands it back. A forward whose sender is no address
// Minka takes is left out and named: `fromAddress` is the one thing a forward
// is about, the field takes nothing else, and a run that wrote it as it stands
// would stop where the applier reads it.
func (e *Extractor) writeForward(ctx context.Context, w *bundle.Writer, bucket int64, toolID string, found forward) error {
	projectID := sourceID(bucket)
	address, unquoted, anAddress := anAddressMinkaTakes(found.From)
	if !anAddress {
		e.forwards.WithoutAnAddress = append(e.forwards.WithoutAnAddress, sourceID(found.ID))
		return nil
	}
	if unquoted {
		e.addressesUnquoted++
	}
	origin, err := e.record("forward", found.ID, found.recording)
	if err != nil {
		return err
	}
	if origin.CreatedBy == nil {
		e.forwards.WithoutAForwarder++
	}
	title, withoutASubject, changed := aSubjectAsATitle(found.Subject)
	if withoutASubject {
		e.forwards.WithoutASubject++
	}
	if changed {
		e.forwards.SubjectsChanged++
	}

	body, files, err := e.rewriteBody(found.Content, "", found.Files)
	if err != nil {
		return err
	}
	// Before the body is settled: only what became of the files says whether
	// the mail reaches Minka with anything in it.
	if err := e.attachFiles(ctx, w, bundle.OwnerForward, origin, files); err != nil {
		return err
	}
	body = e.aBodyThatSaysSomething(body)

	status := bundle.StatusActive
	switch found.status() {
	case bundle.StatusArchived:
		status = bundle.StatusTrashed
		e.forwards.FromTheShelf++
	case bundle.StatusTrashed:
		status = bundle.StatusTrashed
	}
	// Basecamp gives the sender as an address and never as a name.
	err = w.Add(bundle.Forwards, bundle.Forward{
		Record:          origin,
		ProjectSourceID: projectID,
		ToolSourceID:    toolID,
		Title:           title,
		FromAddress:     address,
		Body:            body,
		Status:          status,
	})
	if err != nil {
		return err
	}
	e.measure(bundle.Forwards, body, LongestBodyRecord{Record: "forward", SourceID: origin.SourceID, In: "project", InSourceID: projectID})
	e.countIfItIsLongerThanMinkaTakes(body)
	e.forwards.ByStatus[status]++

	if err := e.writeForwardReplies(ctx, w, bucket, toolID, origin.SourceID, found); err != nil {
		return err
	}
	// A forward says neither how many comments nor how many boosts it has, so
	// both lists are asked for: a count of zero here is the field missing and
	// not a mail nobody answered.
	comments, err := e.askForComments(ctx, w, bucket, bundle.ParentForward, found.ID)
	if err != nil {
		return err
	}
	if err := e.writeBoosts(ctx, w, bucket, bundle.TargetForward, found.ID); err != nil {
		return err
	}
	for _, commented := range comments {
		if commented.BoostsCount > 0 {
			if err := e.writeBoosts(ctx, w, bucket, bundle.TargetComment, commented.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeForwardReplies adds the answers by mail to one forward, oldest first:
// Minka reads them back by their moment, and the order of the file decides
// between two of one second. The list is asked for only where the mail says it
// has answers, and where it says nothing about them at all. A reply that is not
// in use is left out and counted, because a bundle has no word for what became
// of one.
func (e *Extractor) writeForwardReplies(ctx context.Context, w *bundle.Writer, bucket int64, toolID, forwardID string, found forward) error {
	if found.RepliesCount != nil && *found.RepliesCount == 0 {
		return nil
	}
	listed, err := getList[forwardReply](ctx, e.Source, fmt.Sprintf("buckets/%d/inbox_forwards/%d/replies.json", bucket, found.ID))
	if err != nil {
		return err
	}
	carried := make([]forwardReply, 0, len(listed))
	met := map[int64]bool{}
	for _, reply := range listed {
		if met[reply.ID] {
			continue
		}
		met[reply.ID] = true
		// Counted of every answer the source shows, carried or not: a bundle
		// holds none of them either way.
		e.forwards.ReactionsOnRepliesLeftOut += reply.BoostsCount
		taken, err := e.metAmong(replyStatuses, "reply", reply.ID, reply.status(), reply.recording, "project", sourceID(bucket))
		if err != nil {
			return err
		}
		if taken {
			carried = append(carried, reply)
		}
	}
	if err := oldestFirst(carried, "reply", func(r forwardReply) (int64, string) { return r.ID, r.CreatedAt }); err != nil {
		return err
	}
	for _, reply := range carried {
		if err := e.writeForwardReply(ctx, w, bucket, toolID, forwardID, reply); err != nil {
			return err
		}
	}
	return nil
}

// writeForwardReply adds one answer by mail.
//
// Somebody from outside the account stays an address and becomes nobody: a
// person of a bundle is somebody of the organization, and whoever wrote to a
// project from their own mailbox is not. The two are never written together,
// because Minka reads the person and drops the address without a word. A reply
// with neither is left out and named: it would stop the applier.
//
// The boosts of a reply are counted and not written. Minka passes a reaction
// on one over, so a bundle carrying them would say more than arrives.
func (e *Extractor) writeForwardReply(ctx context.Context, w *bundle.Writer, bucket int64, toolID, forwardID string, reply forwardReply) error {
	projectID := sourceID(bucket)
	address, unquoted, fromOutside := theAddressOfAnOutsider(reply.Creator)
	if unquoted {
		e.addressesUnquoted++
	}
	if fromOutside {
		// Asked before the record is made: referring to them would write them
		// into the bundle as somebody of the organization.
		reply.Creator = nil
	}
	origin, err := e.record("reply", reply.ID, reply.recording)
	if err != nil {
		return err
	}
	var fromAddress *string
	if fromOutside && address != "" {
		fromAddress = &address
	}
	if origin.CreatedBy == nil && fromAddress == nil {
		e.forwards.RepliesWithoutASender = append(e.forwards.RepliesWithoutASender, sourceID(reply.ID))
		return nil
	}
	if fromAddress != nil {
		e.forwards.RepliesFromOutside++
	}

	body, files, err := e.rewriteBody(reply.Content, "", reply.Files)
	if err != nil {
		return err
	}
	if err := e.attachFiles(ctx, w, bundle.OwnerForwardReply, origin, files); err != nil {
		return err
	}
	body = e.aBodyThatSaysSomething(body)

	err = w.Add(bundle.ForwardReplies, bundle.ForwardReply{
		Record:          origin,
		ProjectSourceID: projectID,
		ToolSourceID:    toolID,
		ForwardSourceID: forwardID,
		FromAddress:     fromAddress,
		Body:            body,
	})
	if err != nil {
		return err
	}
	e.measure(bundle.ForwardReplies, body, LongestBodyRecord{Record: "reply", SourceID: origin.SourceID, In: "project", InSourceID: projectID})
	e.countIfItIsLongerThanMinkaTakes(body)
	return nil
}

// countIfItIsLongerThanMinkaTakes notes a mail Minka will pass over for its
// length. The body comes as it is — the source has no such limit, and a run that
// shortened it would decide in the one place that cannot be repeated — but a run
// that said nothing would leave a mail, its answers and its comments out of
// Minka with the number nobody looked at. Which record it was stands in
// longestBodyRecords.
func (e *Extractor) countIfItIsLongerThanMinkaTakes(body string) {
	if utf8.RuneCountInString(body) > longestBodyMinkaTakes {
		e.forwards.LongerThanMinkaTakes++
	}
}

// theAddressOfAnOutsider is what a reply from outside the account says about
// whoever wrote it, whether a pair of quotation marks had to come off it, and
// whether the writer is an outsider at all. Basecamp gives such a writer their
// address as their name as well, so the name stands in where the field for an
// address is empty: a writer who is nobody here is worth reading twice.
func theAddressOfAnOutsider(writer *person) (address string, unquoted, fromOutside bool) {
	if writer == nil || writer.PersonableType != outsider {
		return "", false, false
	}
	for _, said := range []string{writer.EmailAddress, writer.Name} {
		if taken, marksCameOff, anAddress := anAddressMinkaTakes(said); anAddress {
			return taken, marksCameOff, true
		}
	}
	return "", false, true
}

const (
	// noSubject is the title a mail gets that came without one. A title cannot
	// be empty here, and a mail without a subject is a mail all the same.
	noSubject = "(no subject)"
	// maxTitleLength is how long a title Minka takes, in characters, and
	// ellipsis what says that the rest of a subject is not there.
	maxTitleLength = 200
	ellipsis       = "…"
)

// aSubjectAsATitle makes the subject of a mail into a title Minka takes, and
// says whether the mail came without one and whether the subject had to be
// changed.
//
// A title is one line, not empty, of at most 200 characters and free of
// characters nobody can see, and a subject is none of that: it is whatever a
// mail server let through. The applier stops at a title it cannot take, and
// nobody can repair a subject at the source, so it is made into a title here
// rather than costing the mail or the run: every character nobody can see
// becomes a blank, what is left is trimmed, and what is longer than a title
// holds is cut before the last character and ends in an ellipsis.
func aSubjectAsATitle(subject string) (title string, withoutASubject, changed bool) {
	// A subject that is not UTF-8 is a title Minka reads as one long control
	// character, whatever else is in it.
	folded := strings.ToValidUTF8(subject, " ")
	folded = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, folded)
	changed = folded != subject
	folded = strings.TrimSpace(folded)
	if folded == "" {
		return noSubject, true, false
	}
	if utf8.RuneCountInString(folded) <= maxTitleLength {
		return folded, false, changed
	}
	cut := []rune(folded)[:maxTitleLength-utf8.RuneCountInString(ellipsis)]
	return strings.TrimRightFunc(string(cut), unicode.IsSpace) + ellipsis, false, true
}

// longestBodyMinkaTakes is how long the body of a mail or of an answer may be
// for Minka to hold it, in characters. What is measured here is the body the run
// writes; Minka counts what it stores, which is that body with the markup of its
// line breaks and of its files in it, so a body below the number can still be
// one too many.
const longestBodyMinkaTakes = 1000000

// The lines a run writes into a mail that would arrive with nothing in it. One
// wording for a mail that held one file and for a mail that held several: a
// mail that would be lost is no place to count them.
const (
	withoutWords         = "<p>This mail came without words.</p>"
	withoutWordsAndAFile = "<p>This mail came without words, and what it held could not be brought.</p>"
)

// aBodyThatSaysSomething is the body of a mail with a line of the run's own
// behind it, where what the mail says would reach Minka as nothing at all.
//
// Such a body is written empty there and the mail is remembered as finished,
// or passed over before it is remembered and passed over again by every run
// after this one — with its subject, its sender, its replies and its comments.
// Neither can be mended from that side, so the run says here what became of the
// mail. A file that will never come keeps its tag beside the line: the tag says
// where the file stood, and Minka drops it.
//
// An embed counts for nothing here although Minka usually takes one: whether it
// does is its own reading of an address, and a mail that hangs on one alone is
// lost wherever the two readings differ. What it gets is its address as a link,
// which every reading keeps, and no sentence: the mail did point there, and a
// run that said it came without words would put that in writing wherever Minka
// took the embed after all.
//
// A body PHP reads as nothing gets the line too. It takes a NUL off the edges
// of a text where Go does not, so a body of nothing else is missing on that
// side, and a field the format needs and cannot find stops the run.
func (e *Extractor) aBodyThatSaysSomething(body string) string {
	filesThatStayOut, embedded, empty := e.whatWouldArriveOf(body)
	if !empty && strings.Trim(body, paddingPHPTakesOff) != "" {
		return body
	}
	e.forwards.GivenALineOfTheRun++
	line := ""
	switch {
	case filesThatStayOut > 0:
		line = withoutWordsAndAFile
	case len(embedded) == 0:
		line = withoutWords
	}
	for _, address := range embedded {
		line += `<p><a href="` + html.EscapeString(address) + `">` + html.EscapeString(address) + `</a></p>`
	}
	return body + line
}

// paddingPHPTakesOff is the whitespace Minka trims off a field of a bundle
// before it asks whether the field says anything.
const paddingPHPTakesOff = " \t\n\r\x00\v"

// whatWouldArriveOf reads a body the run wrote and says what of it reaches a
// page: how many of the files it names will never come, the addresses of the
// embeds in it, and whether what is left says nothing. It asks what saysNothing
// asks of every other body and two questions more, so what is changed in the
// one belongs in the other: a file that will never come counts for nothing
// here, and so does an embed.
//
// A mention and a reference count for something, although Minka leaves either
// as the words it was given where it finds no target — and a run writes words
// into both, so a body of one of them alone says something there too.
func (e *Extractor) whatWouldArriveOf(body string) (filesThatStayOut int, embedded []string, empty bool) {
	reader := newBodyReader(body)
	for {
		token, more := reader.next()
		switch {
		case !more:
			return filesThatStayOut, embedded, true
		case token.kind == nethtml.TextToken:
			if !blank(token.raw) {
				return filesThatStayOut, embedded, false
			}
		case token.kind == nethtml.CommentToken || token.kind == nethtml.DoctypeToken:
		case token.name == "import-attachment":
			if token.kind == nethtml.EndTagToken {
				continue
			}
			if !e.theFileNeverArrives(token.attribute("ref")) {
				return filesThatStayOut, embedded, false
			}
			filesThatStayOut++
		case token.name == "import-embed":
			if token.kind != nethtml.EndTagToken {
				embedded = append(embedded, token.attribute("url"))
			}
		case token.name != "br" && token.name != "div" && token.name != "p":
			return filesThatStayOut, embedded, false
		}
	}
}
