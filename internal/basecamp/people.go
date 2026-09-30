package basecamp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// registry decides who comes into a bundle: the people some record of the
// bundle refers to, and nobody else. An account holds many more people than
// the projects of a run have to do with, and they have no place where only
// those projects move to. The people list of the account is what somebody is
// looked up in; refer is the one door into the bundle, so that every
// reference a record carries leads to a person who is written.
//
// Nobody comes in with an e-mail address that is none. Basecamp masks the
// addresses of other people for an identity that may not see them, and a
// bundle with masks in it would make people nobody can sign in as: that ends
// the run. An address that is broken at the source is another matter: only
// its owner can repair it there, and it must not cost the import of an
// account, so the person comes without an address and is noted.
type registry struct {
	account  map[int64]person
	referred map[int64]*seenPerson
	invalid  map[int64]bool
	unquoted map[int64]bool
}

type seenPerson struct {
	person
	// listed tells that the list of the account or the members of a project
	// name the person.
	listed bool
	// company is what the bundle carries for a client, read where they come
	// in. Nobody else has one: Minka reads a company on a client alone.
	company *bundle.CompanyOrNone
	// named counts the records that name the person, and integrationLines
	// those of them that are lines an integration posted.
	named            int
	integrationLines int
}

// integration is the kind of person Basecamp gives a chatbot: one person per
// chatbot of the account, whichever chats it writes into.
const integration = "Integration"

// bot tells whether somebody is a chatbot or an integration of the source,
// which a bundle carries as a person of kind bot. Basecamp says so of a
// chatbot it still has. Of one that was deleted it keeps what it keeps of a
// deleted user, a Tombstone, and only the lines it wrote tell the two apart:
// whoever is neither a user nor a client and wrote a line of an integration
// was one. A user or a client stays a person whatever line names them, and so
// does somebody whose kind Basecamp does not say.
func (s seenPerson) bot() bool {
	return s.PersonableType == integration || (s.botByItsLines() && s.integrationLines > 0)
}

// botByItsLines tells that only the lines somebody wrote can say whether they
// are a bot: their kind is none that decides it by itself. Such a bot is one
// Basecamp deleted.
func (s seenPerson) botByItsLines() bool {
	switch s.PersonableType {
	case integration, "User", "Client", "":
		return false
	}
	return true
}

// alsoNamedElsewhere tells of a bot its lines made one that a record other
// than a line of an integration names as well. That may be a deleted user who
// once wrote as an integration, and Minka never turns a bot back into a human.
func (s seenPerson) alsoNamedElsewhere() bool {
	return s.botByItsLines() && s.bot() && s.named > s.integrationLines
}

// retired tells whether somebody comes as an author and nothing more. The
// kind of person decides: a user is somebody to work with, and every other
// kind is not, be it a bot, what is left of somebody who was deleted, or a
// kind nobody knows yet. The list of an account says nothing here: what an
// admin sees holds every kind, what anybody else sees holds users alone.
// Only of somebody whose kind Basecamp does not say does the list decide:
// whom only a record names has left.
func (s seenPerson) retired() bool {
	switch s.PersonableType {
	case "User", "Client":
		return false
	case "":
		return !s.listed
	}
	return true
}

// aClient tells whether a bundle says somebody is a client of the source. The
// legacy kind of person counts as one whatever the flag says of them: only the
// word client keeps somebody the import finds here by their address out of the
// projects, and a kind of person Basecamp made before the flag existed is no
// reason to put a client among the people of a project.
//
// A bot is never a client: Minka reads nothing of the word on a bot.
func (s seenPerson) aClient() bool {
	return !s.bot() && (s.Client || s.PersonableType == "Client")
}

func newRegistry() *registry {
	return &registry{account: map[int64]person{}, referred: map[int64]*seenPerson{}, invalid: map[int64]bool{}, unquoted: map[int64]bool{}}
}

// sourceProblemShare is the share of addresses that are none above which the
// people list of an account is not taken for data with flaws in it any more:
// a source that masks addresses with another character than Basecamp does
// today would otherwise come through as an account of people without logins.
const sourceProblemShare = 0.5

// listAccount takes the people list of the account. It brings nobody into the
// bundle; it is the one list that shows whether the identity sees addresses
// at all, so it is checked as a whole, whoever of it a record refers to later.
func (r *registry) listAccount(people []person) error {
	people = tidied(people)
	if err := refuseMasks(people); err != nil {
		return err
	}

	given, malformed := 0, 0
	for _, p := range people {
		if p.EmailAddress == "" {
			continue
		}
		given++
		if _, _, anAddress := anAddressMinkaTakes(p.EmailAddress); !anAddress {
			malformed++
		}
	}
	if float64(malformed) > sourceProblemShare*float64(given) {
		return &Error{
			Kind:    API,
			Message: fmt.Sprintf("Basecamp gave e-mail addresses that are none: %d of %d", malformed, given),
			Hint:    "That many are no flaw in the data. Either the identity the Basecamp CLI is signed in as hides the addresses of others, which basecamp auth status says, or the account writes its addresses in a shape Minka takes none of: outside ASCII, or with a domain of one label.",
		}
	}

	for _, p := range people {
		r.account[p.ID] = p
	}
	return nil
}

// refer brings the person a record names into the bundle and returns the
// reference the record carries: nil when the source names nobody.
//
// What the account's list says about somebody wins over what a record says.
// Somebody the list does not know comes as the record shows them; present
// tells whether the record proves that they are still there, as a list of
// members does, while the creator of something may have left long ago.
func (r *registry) refer(named *person, present bool) (*string, error) {
	if named == nil || named.ID == 0 {
		return nil, nil
	}
	id := sourceID(named.ID)
	if seen, known := r.referred[named.ID]; known {
		seen.listed = seen.listed || present
		seen.named++
		return &id, nil
	}

	p, listed := r.account[named.ID]
	if !listed {
		p = tidied([]person{*named})[0]
		if err := refuseMasks([]person{p}); err != nil {
			return nil, err
		}
	}
	if p.PersonableType == "" {
		p.PersonableType = named.PersonableType
	}
	// A person is a record like any other and carries its times: said here,
	// where somebody first comes in, and not at the end of a run of hours.
	if err := requireTimestamps(fmt.Sprintf("person %d", p.ID), p.CreatedAt, p.UpdatedAt); err != nil {
		return nil, err
	}
	// An address that is none is left out, not repaired, and the one repair
	// there is — a pair of quotation marks around the whole of it — is counted.
	if p.EmailAddress != "" {
		address, unquoted, anAddress := anAddressMinkaTakes(p.EmailAddress)
		p.EmailAddress = address
		switch {
		case !anAddress:
			r.invalid[p.ID] = true
		case unquoted:
			r.unquoted[p.ID] = true
		}
	}
	seen := &seenPerson{person: p, listed: listed || present, named: 1}
	// A company a bundle cannot carry ends the run: said here, where the client
	// comes in, and not at the end of a run of hours. Of somebody whose lines
	// may yet make them a bot, who is then no client, it is said where the
	// people are written.
	if seen.aClient() && !seen.botByItsLines() {
		company, err := companyOf("person", p.ID, p.Company)
		if err != nil {
			return nil, err
		}
		seen.company = company
	}
	r.referred[p.ID] = seen
	return &id, nil
}

// referByID brings somebody into the bundle of whom a record names the ID
// alone, as a mention does, and returns their name with the reference. Only
// the list of the account knows such a person: the reference is nil for
// anybody else, because there is nothing to write a person from.
func (r *registry) referByID(id int64) (*string, string, error) {
	p, listed := r.account[id]
	if !listed {
		return nil, "", nil
	}
	reference, err := r.refer(&p, false)
	return reference, p.Name, err
}

// wroteForAnIntegration notes that a line of an integration names somebody the
// bundle carries as its author, which is what tells a deleted chatbot from a
// deleted user.
func (r *registry) wroteForAnIntegration(id int64) {
	if seen, referred := r.referred[id]; referred {
		seen.integrationLines++
	}
}

// botsAlsoNamedElsewhere names, by ID and in the order of the IDs, the bots
// their lines made ones that other records name as well.
func (r *registry) botsAlsoNamedElsewhere() []string {
	names := []string{}
	for _, seen := range r.sorted() {
		if seen.alsoNamedElsewhere() {
			names = append(names, sourceID(seen.ID))
		}
	}
	return names
}

// known is what the bundle holds about somebody a record has already brought
// in: the address and the kind of person as they will be written, which is what
// decides whether anybody else may refer to them again.
func (r *registry) known(id int64) (seenPerson, bool) {
	seen, referred := r.referred[id]
	if !referred {
		return seenPerson{}, false
	}
	return *seen, true
}

// nameOf is the name the list of the account has for somebody, without
// bringing them into the bundle.
func (r *registry) nameOf(id int64) (string, bool) {
	p, listed := r.account[id]
	return p.Name, listed
}

// refuseMasks ends a run over masked addresses. The error says how many, and
// never which: neither an address nor whose it is belongs into a message that
// ends up in a log.
func refuseMasks(people []person) error {
	given, masked := 0, 0
	for _, p := range people {
		if p.EmailAddress != "" {
			given++
		}
		if strings.ContainsRune(p.EmailAddress, mask) {
			masked++
		}
	}
	if masked == 0 {
		return nil
	}
	return &Error{
		Kind:    Forbidden,
		Message: fmt.Sprintf("Basecamp masks e-mail addresses for the identity that is signed in: %d of %d", masked, given),
		Hint:    "The identity the Basecamp CLI is signed in as may not see the e-mail addresses of other people. Run the import as somebody who may: a person of the account's own company, or an admin. Check with: basecamp auth status",
	}
}

// tidied takes the whitespace around an address away: a stray space is no
// reason for somebody to come without an address.
func tidied(people []person) []person {
	tidy := make([]person, len(people))
	for i, p := range people {
		p.EmailAddress = strings.TrimSpace(p.EmailAddress)
		tidy[i] = p
	}
	return tidy
}

// sorted are the people of the bundle, in the order of their IDs.
func (r *registry) sorted() []seenPerson {
	all := make([]seenPerson, 0, len(r.referred))
	for _, seen := range r.referred {
		all = append(all, *seen)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

// leftOut counts the people of the account no record refers to.
func (r *registry) leftOut() int {
	count := 0
	for id := range r.account {
		if _, referred := r.referred[id]; !referred {
			count++
		}
	}
	return count
}

// invalidAddresses names, by ID and in the order of the IDs, who came into
// the bundle without an address because theirs was none. A bot is none of
// them: it comes without an address whatever the source keeps.
func (r *registry) invalidAddresses() []string {
	ids := make([]int64, 0, len(r.invalid))
	for id := range r.invalid {
		if !r.referred[id].bot() {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = sourceID(id)
	}
	return names
}

// unquotedAddresses counts the people whose address came inside a pair of
// quotation marks that the run took off, one for each of them. A bot's address
// is not written, so nothing came off it.
func (r *registry) unquotedAddresses() int {
	count := 0
	for id := range r.unquoted {
		if !r.referred[id].bot() {
			count++
		}
	}
	return count
}

// mask is the character Basecamp puts in the place of what an identity may
// not see.
const mask = '\u2022'
