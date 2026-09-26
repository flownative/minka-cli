package basecamp_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The forwarded mail of the fake stands in its tenth project: the first inbox
// of the dock holds every case, the second holds one mail, the third is
// switched off.
const forwardProject = "7010"

func inbox(a *fakebasecamp.Account) *fakebasecamp.Tool { return &a.Projects[9].Dock[0] }

// theMail finds a mail of the first inbox by its ID, for a test that changes it.
func theMail(t *testing.T, a *fakebasecamp.Account, id int64) *fakebasecamp.Forward {
	t.Helper()
	for i := range inbox(a).Forwards {
		if inbox(a).Forwards[i].ID == id {
			return &inbox(a).Forwards[i]
		}
	}
	t.Fatalf("the first inbox holds no mail %d", id)
	return nil
}

// A forward carries the subject as its title, the sender as an address and the
// mail as the source holds it: the line breaks stay, because Minka turns those
// into paragraphs, and the files, mentions and references become the tags of the
// format. The mails of an inbox stand in the bundle oldest first, whatever order
// the inbox lists them in, and every inbox that is switched on is a tool.
func TestTheForwardedMailOfAnInboxComesWithItsSenderItsSubjectAndItsBody(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), forwardProject)

	tools := records(t, dir, bundle.Tools)
	for _, id := range []string{"9290", "9292"} {
		if kind := recordWithID(t, tools, id)["kind"]; kind != bundle.ToolForwards {
			t.Errorf("inbox %s comes as a tool of the kind %v", id, kind)
		}
	}

	forwards := records(t, dir, bundle.Forwards)
	want := map[string]any{
		"sourceId": "9301", "createdAt": "2025-02-02T08:00:00.000Z", "updatedAt": "2025-02-02T08:00:00.000Z", "createdBy": "101",
		"projectSourceId": forwardProject, "toolSourceId": "9290",
		"title": "Invoice for March", "fromAddress": "billing@supplier.test", "fromName": nil, "status": bundle.StatusActive,
		"body": "Dear all,\n\nthe invoice is attached, see also\n" +
			`<import-reference kind="cards" ref="9201" href="https://3.basecamp.com/4242/buckets/7003/card_tables/cards/9201">the card</import-reference>` +
			"\n\nKind regards\n" + `<import-attachment ref="attachment-5301"></import-attachment>`,
	}
	if got := recordWithID(t, forwards, "9301"); !reflect.DeepEqual(got, want) {
		t.Errorf("the mail with everything on it:\n got %v\nwant %v", got, want)
	}

	// The inbox lists its mail newest first, and the bundle holds it the other
	// way round.
	if got := sourceIDs(forwards); !reflect.DeepEqual(got, []string{"9301", "9302", "9303", "9304", "9305", "9308", "9309", "9310", "9311", "9321"}) {
		t.Errorf("the mails stand in the order %v", got)
	}
	if got := result.Forwards.ByStatus; !reflect.DeepEqual(got, map[string]int{bundle.StatusActive: 8, bundle.StatusTrashed: 2}) {
		t.Errorf("the mails by status are %v", got)
	}
}

// What the source keeps on its shelf comes into the trash, which is the one
// place a forward can be brought back from here, and the run says how many. What
// hangs on such a mail comes with it.
func TestAMailOnTheShelfOfTheSourceComesIntoTheTrashAndIsCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), forwardProject)

	shelved := recordWithID(t, records(t, dir, bundle.Forwards), "9304")
	if shelved["status"] != bundle.StatusTrashed {
		t.Errorf("the mail on the shelf comes as %v", shelved["status"])
	}
	if result.Forwards.FromTheShelf != 1 {
		t.Errorf("%d mails from the shelf, want the one", result.Forwards.FromTheShelf)
	}
	// Its comment comes with it: a mail in the trash keeps what hangs on it.
	comment := recordWithID(t, records(t, dir, bundle.Comments), "9503")
	if comment["parentKind"] != bundle.ParentForward || comment["parentSourceId"] != "9304" {
		t.Errorf("the comment of the shelved mail hangs on %v %v", comment["parentKind"], comment["parentSourceId"])
	}
}

// A title is one line of at most 200 characters without a character nobody can
// see, and a subject is whatever a mail server let through. Nobody can repair a
// subject at the source, so it is made into a title rather than costing the mail
// or the run.
func TestASubjectBecomesATitleMinkaTakesWhateverTheMailCalledItself(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), forwardProject)

	forwards := records(t, dir, bundle.Forwards)
	if got := recordWithID(t, forwards, "9302")["title"]; got != "(no subject)" {
		t.Errorf("the mail without a subject is called %q", got)
	}
	title, _ := recordWithID(t, forwards, "9303")["title"].(string)
	switch {
	case utf8.RuneCountInString(title) > 200 || utf8.RuneCountInString(title) < 190:
		t.Errorf("the long subject became a title of %d characters", utf8.RuneCountInString(title))
	case !strings.HasSuffix(title, "…"):
		t.Errorf("the long subject was cut without saying so: %q", title)
	case !strings.HasPrefix(title, "Re: Fwd: A subject"):
		t.Errorf("the long subject was folded into %q", title)
	case strings.ContainsFunc(title, unicode.IsControl):
		t.Errorf("the title holds a character nobody can see")
	case strings.TrimSpace(title) != title:
		t.Errorf("the title is not trimmed: %q", title)
	}
	if result.Forwards.WithoutASubject != 1 || result.Forwards.SubjectsChanged != 1 {
		t.Errorf("%d mails without a subject and %d subjects changed, want one of each",
			result.Forwards.WithoutASubject, result.Forwards.SubjectsChanged)
	}
}

// A subject Minka would take stays exactly as it is: a run that folded every
// subject would change the one thing a mail is found by.
func TestASubjectATitleCanCarryIsLeftAsItIs(t *testing.T) {
	account := fakebasecamp.New()
	theMail(t, account, 9301).Subject = "  Invoice  for   March  "
	dir, result := extract(t, account, forwardProject)

	if got := recordWithID(t, records(t, dir, bundle.Forwards), "9301")["title"]; got != "Invoice  for   March" {
		t.Errorf("the subject became %q, want it as it stood without the whitespace around it", got)
	}
	if result.Forwards.SubjectsChanged != 1 {
		t.Errorf("%d subjects changed, want the long one alone", result.Forwards.SubjectsChanged)
	}
}

// A mail whose sender is no address Minka takes is left out and named by ID:
// fromAddress takes nothing else and a run that wrote it as it stands would end
// where the applier reads it. Nothing of such a mail is asked for.
func TestAMailWhoseSenderIsNoAddressIsLeftOutAndNamed(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, forwardProject)

	for _, record := range records(t, dir, bundle.Forwards) {
		if record["sourceId"] == "9307" {
			t.Errorf("the mail whose sender is no address is in the bundle: %v", record)
		}
	}
	if got := result.Forwards.WithoutAnAddress; !reflect.DeepEqual(got, []string{"9307"}) {
		t.Errorf("mails without an address: %v, want the one", got)
	}
	// Never the address itself: a bundle carries one, a message never does.
	for _, request := range append(asked(account, "9307"), asked(account, "localhost")...) {
		t.Errorf("the run asked for something of the mail it left out: %s", request)
	}
}

// A mail whose body would reach Minka as nothing at all carries a line of the
// run's own: written empty there, it would be remembered as finished or passed
// over on every run, and the subject, the sender and everything hanging on it
// would be lost with it.
func TestAMailThatWouldArriveEmptyCarriesALineOfTheRun(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), forwardProject)

	forwards := records(t, dir, bundle.Forwards)
	// Nothing but a file the source no longer has: the tag stays, because it
	// says where the file stood, and Minka drops it.
	gone, _ := recordWithID(t, forwards, "9308")["body"].(string)
	if !strings.Contains(gone, `<import-attachment ref="attachment-5303">`) ||
		!strings.Contains(gone, "the file it held could not be brought") {
		t.Errorf("the mail that was a file nobody has any more: %q", gone)
	}
	if got := recordWithID(t, forwards, "9309")["body"]; !strings.Contains(got.(string), "This mail came without words.") {
		t.Errorf("the mail that says nothing: %q", got)
	}
	// An embed is Minka's own rule about an address, so a mail that hangs on one
	// alone gets the address as a link as well.
	player, _ := recordWithID(t, forwards, "9310")["body"].(string)
	if !strings.Contains(player, `<import-embed url="https://video.example.test/embed/7">`) ||
		!strings.Contains(player, `<a href="https://video.example.test/embed/7">`) {
		t.Errorf("the mail that was a player alone: %q", player)
	}
	if result.Forwards.GivenALineOfTheRun != 3 {
		t.Errorf("%d mails carry a line of the run, want the three", result.Forwards.GivenALineOfTheRun)
	}
}

// A mail whose body is a file that does come says nothing of its own: the run
// writes a line where the file will not arrive and nowhere else.
func TestAMailThatIsAFileWhichArrivesGetsNoLineOfTheRun(t *testing.T) {
	account := fakebasecamp.New()
	theMail(t, account, 9308).Files[0].Gone = false
	dir, result := extract(t, account, forwardProject)

	body, _ := recordWithID(t, records(t, dir, bundle.Forwards), "9308")["body"].(string)
	if strings.Contains(body, "could not be brought") || strings.Contains(body, "came without words") {
		t.Errorf("the mail whose file arrives carries a line of the run: %q", body)
	}
	if result.Forwards.GivenALineOfTheRun != 2 {
		t.Errorf("%d mails carry a line of the run, want the two that are left", result.Forwards.GivenALineOfTheRun)
	}
}

// An answer by mail is a record of its own and no comment. It belongs to
// somebody of the account or to the address it came from, never to both, because
// Minka reads the person and drops the address without a word. Somebody from
// outside never becomes a person: they would be somebody of the organization.
func TestAnAnswerByMailBelongsToAPersonOrToAnAddressAndNeverToBoth(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), forwardProject)

	replies := records(t, dir, bundle.ForwardReplies)
	want := []map[string]any{
		{"sourceId": "9401", "createdAt": "2025-02-03T09:00:00.000Z", "updatedAt": "2025-02-03T09:00:00.000Z", "createdBy": "102",
			"projectSourceId": forwardProject, "toolSourceId": "9290", "forwardSourceId": "9301",
			"fromAddress": nil, "body": "<div>Thank you, it is booked.</div>"},
		{"sourceId": "9402", "createdAt": "2025-02-03T10:00:00.000Z", "updatedAt": "2025-02-03T10:00:00.000Z", "createdBy": nil,
			"projectSourceId": forwardProject, "toolSourceId": "9290", "forwardSourceId": "9301",
			"fromAddress": "anna@supplier.test",
			"body":        `<div>Here is the signed copy.</div><import-attachment ref="attachment-5302"></import-attachment>`},
	}
	if !reflect.DeepEqual(replies, want) {
		t.Errorf("the answers by mail:\n got %v\nwant %v", replies, want)
	}
	for _, person := range records(t, dir, bundle.People) {
		if person["sourceId"] == "901" || person["sourceId"] == "902" {
			t.Errorf("somebody from outside the account came into the bundle: %v", person)
		}
	}
	if result.Forwards.RepliesFromOutside != 1 {
		t.Errorf("%d answers from outside, want the one", result.Forwards.RepliesFromOutside)
	}
}

// A boost on an answer is counted and never written: Minka passes such a
// reaction over, so a bundle carrying it would say more than arrives. Nor is the
// list of them asked for.
func TestABoostOnAnAnswerByMailIsCountedAndNeitherWrittenNorAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, forwardProject)

	for _, reaction := range records(t, dir, bundle.Reactions) {
		if reaction["targetSourceId"] == "9402" || reaction["sourceId"] == "9591" {
			t.Errorf("a boost on an answer by mail is in the bundle: %v", reaction)
		}
	}
	if result.Forwards.ReactionsOnRepliesLeftOut != 1 {
		t.Errorf("%d boosts on an answer left out, want the one", result.Forwards.ReactionsOnRepliesLeftOut)
	}
	for _, request := range asked(account, "recordings/9402/boosts") {
		t.Errorf("the run asked for the boosts of an answer: %s", request)
	}
}

// An answer the source names neither somebody nor a usable address for is left
// out and named: Minka stops the run at one that names neither.
func TestAnAnswerNamingNeitherAWriterNorAnAddressIsLeftOutAndNamed(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), forwardProject)

	for _, reply := range records(t, dir, bundle.ForwardReplies) {
		if reply["sourceId"] == "9404" {
			t.Errorf("the answer without a sender is in the bundle: %v", reply)
		}
	}
	if got := result.Forwards.RepliesWithoutASender; !reflect.DeepEqual(got, []string{"9404"}) {
		t.Errorf("answers without a sender: %v, want the one", got)
	}
}

// A word the bundle has none for leaves the record out and is counted with the
// word and the place it stood in: a mail of a status nobody knows, and an answer
// the source has thrown away, which a bundle cannot say.
func TestAMailOrAnAnswerOfAStatusNobodyKnowsIsLeftOutAndCounted(t *testing.T) {
	accountOfTheRun := fakebasecamp.New()
	dir, result := extract(t, accountOfTheRun, forwardProject)

	for _, record := range records(t, dir, bundle.Forwards) {
		if record["sourceId"] == "9306" {
			t.Errorf("the mail of a status nobody knows is in the bundle: %v", record)
		}
	}
	for _, reply := range records(t, dir, bundle.ForwardReplies) {
		if reply["sourceId"] == "9403" {
			t.Errorf("the answer in the trash is in the bundle: %v", reply)
		}
	}
	// Nothing of a record that is left out is asked for: nobody comes into the
	// bundle for it and no request is spent on it.
	for _, part := range []string{"recordings/9306/", "inbox_forwards/9306/"} {
		for _, request := range asked(accountOfTheRun, part) {
			t.Errorf("the run asked for something of the mail it left out: %s", request)
		}
	}
	left := map[string]int{}
	for _, one := range result.RecordsLeftOut {
		if one.Record == "forward" || one.Record == "reply" {
			left[one.Record+" "+one.Field+" "+one.Value+" in "+one.In+" "+one.InSourceID] = one.LeftOut
		}
	}
	want := map[string]int{
		"forward status quarantined in project " + forwardProject: 1,
		"reply status trashed in project " + forwardProject:       1,
	}
	if !reflect.DeepEqual(left, want) {
		t.Errorf("records left out:\n got %v\nwant %v", left, want)
	}
}

// An inbox the source has switched off is skipped and counted as every tool
// that is switched off is, and nothing in it is asked for: a run does not read
// what the source has put away.
func TestAnInboxThatIsSwitchedOffIsSkippedAndNothingInItIsAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, forwardProject)

	for _, tool := range records(t, dir, bundle.Tools) {
		if tool["sourceId"] == "9291" {
			t.Errorf("the inbox that is switched off came as a tool: %v", tool)
		}
	}
	for _, record := range records(t, dir, bundle.Forwards) {
		if record["sourceId"] == "9331" {
			t.Errorf("a mail of the inbox that is switched off is in the bundle: %v", record)
		}
	}
	for _, request := range asked(account, "inboxes/9291/") {
		t.Errorf("the run read an inbox that is switched off: %s", request)
	}
	skipped := map[string]int{}
	for _, one := range result.Skipped {
		if !one.Enabled {
			skipped[one.Kind] = one.Count
		}
	}
	if skipped["inbox"] != 1 {
		t.Errorf("%d inboxes counted as skipped, want the one that is switched off: %v", skipped["inbox"], result.Skipped)
	}
}

// A forward says neither how many comments nor how many boosts it has, so both
// lists are asked for of every one of them: a count of zero is the field missing
// and not a mail nobody answered.
func TestTheCommentsAndTheBoostsOfAMailAreAlwaysAskedFor(t *testing.T) {
	account := fakebasecamp.New()
	extract(t, account, forwardProject)

	// The mail nobody commented on and nobody boosted.
	for _, part := range []string{"recordings/9302/comments", "recordings/9302/boosts"} {
		if len(asked(account, part)) == 0 {
			t.Errorf("the run never asked for %s", part)
		}
	}
}

// The files of a mail and of an answer come with the record that shows them,
// each under its own kind of owner: Minka binds a reply's files to the forward,
// and the bundle says which record named them.
func TestTheFilesOfAMailAndOfAnAnswerNameTheirOwnRecord(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), forwardProject)

	attachments := records(t, dir, bundle.Attachments)
	for id, want := range map[string][]string{
		"attachment-5301": {bundle.OwnerForward, "9301"},
		"attachment-5302": {bundle.OwnerForwardReply, "9402"},
		"attachment-5303": {bundle.OwnerForward, "9308"},
	} {
		record := recordWithID(t, attachments, id)
		if record["ownerKind"] != want[0] || record["ownerSourceId"] != want[1] {
			t.Errorf("%s belongs to %v %v, want %v %v", id, record["ownerKind"], record["ownerSourceId"], want[0], want[1])
		}
	}
	// The file the source no longer has says so, so that no run waits for it.
	if gone := recordWithID(t, attachments, "attachment-5303")["gone"]; gone != true {
		t.Errorf("the file nobody has any more says %v about being gone", gone)
	}
}

// The forwarded mail of a run is read after everything it can point at. No
// second look rewrites a reference of a forward, unlike one of a card or a post,
// so a reference that found nothing when the mail was written stays a link for
// ever.
func TestForwardedMailIsReadAfterEverythingItCanPointAt(t *testing.T) {
	account := fakebasecamp.New()
	extract(t, account, boardProject, talkProject, checkinProject, todoProject, forwardProject)

	first := len(account.Requests)
	for i, request := range account.Requests {
		if strings.Contains(request, "/forwards.json") || strings.Contains(request, "/inbox_forwards/") {
			first = i
			break
		}
	}
	for _, part := range []string{"/cards.json", "/messages.json", "/lines.json", "/questions.json", "/todolists.json"} {
		for i, request := range account.Requests {
			if strings.Contains(request, part) && i > first {
				t.Errorf("%s was read after the forwarded mail: %s", part, request)
			}
		}
	}
	if first == len(account.Requests) {
		t.Fatal("the run never read an inbox")
	}
}

// The words of the format are the contract: a kind, a file name, a kind of owner
// and a kind of parent are what Minka reads them as and not what a constant here
// happens to say. Nobody on that side checks a word it does not know, so a typo
// would be silent.
func TestTheWordsAForwardIsWrittenUnderAreTheOnesMinkaReads(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), forwardProject)

	if got := recordsIn(t, filepath.Join(dir, "forwards.jsonl")); len(got) == 0 {
		t.Error("the mails stand in no forwards.jsonl")
	}
	if got := recordsIn(t, filepath.Join(dir, "forwardreplies.jsonl")); len(got) == 0 {
		t.Error("the answers stand in no forwardreplies.jsonl")
	}
	if got := recordWithID(t, records(t, dir, bundle.Tools), "9290")["kind"]; got != "forwards" {
		t.Errorf("an inbox becomes a tool of kind %v, want forwards", got)
	}
	if got := recordWithID(t, records(t, dir, bundle.Comments), "9501")["parentKind"]; got != "forward" {
		t.Errorf("a comment on a mail says parentKind %v, want forward", got)
	}
	if got := recordWithID(t, records(t, dir, bundle.Reactions), "9593")["targetKind"]; got != "forward" {
		t.Errorf("a boost on a mail says targetKind %v, want forward", got)
	}
	attachments := records(t, dir, bundle.Attachments)
	if got := recordWithID(t, attachments, "attachment-5301")["ownerKind"]; got != "forward" {
		t.Errorf("the file of a mail says ownerKind %v, want forward", got)
	}
	if got := recordWithID(t, attachments, "attachment-5302")["ownerKind"]; got != "forwardreply" {
		t.Errorf("the file of an answer says ownerKind %v, want forwardreply", got)
	}
}

// A mail Minka will pass over for its length is counted: it is lost there on
// every run, and its answers and comments wait with it.
func TestAMailLongerThanMinkaHoldsIsCounted(t *testing.T) {
	account := fakebasecamp.New()
	theMail(t, account, 9302).Content = strings.Repeat("a", 1000001)
	_, result := extract(t, account, forwardProject)

	if result.Forwards.LongerThanMinkaTakes != 1 {
		t.Errorf("%d mails longer than Minka holds, want the one", result.Forwards.LongerThanMinkaTakes)
	}
	// It comes as it is: a run that shortened it would decide in the one place
	// nobody can repeat.
	if got := result.LongestBody[bundle.Forwards]; got != 1000001 {
		t.Errorf("the longest mail is %d characters", got)
	}
}
