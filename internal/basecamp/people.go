package basecamp

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
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
}

type seenPerson struct {
	person
	// listed tells that the source still has the person.
	listed bool
}

func newRegistry() *registry {
	return &registry{account: map[int64]person{}, referred: map[int64]*seenPerson{}, invalid: map[int64]bool{}}
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
		if !looksLikeAnAddress(p.EmailAddress) {
			malformed++
		}
	}
	if float64(malformed) > sourceProblemShare*float64(given) {
		return &Error{
			Kind:    API,
			Message: fmt.Sprintf("Basecamp gave e-mail addresses that are none: %d of %d", malformed, given),
			Hint:    "That many are no flaw in the data. Look at what the identity the Basecamp CLI is signed in as may see: basecamp auth status",
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
		return &id, nil
	}

	p, listed := r.account[named.ID]
	if !listed {
		p = tidied([]person{*named})[0]
		if err := refuseMasks([]person{p}); err != nil {
			return nil, err
		}
	}
	// An address that is none is left out, not repaired.
	if p.EmailAddress != "" && !looksLikeAnAddress(p.EmailAddress) {
		p.EmailAddress = ""
		r.invalid[p.ID] = true
	}
	r.referred[p.ID] = &seenPerson{person: p, listed: listed || present}
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
// the bundle without an address because theirs was none.
func (r *registry) invalidAddresses() []string {
	ids := make([]int64, 0, len(r.invalid))
	for id := range r.invalid {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = sourceID(id)
	}
	return names
}

// mask is the character Basecamp puts in the place of what an identity may
// not see.
const mask = '\u2022'

// looksLikeAnAddress is a plain check of the shape, not of what a mail server
// would take: something without whitespace, one @, and a domain of at least
// two labels made of letters, digits and hyphens. Letters of any script count,
// so that a domain in somebody's own language passes; the glyphs a source may
// mask an address with do not.
func looksLikeAnAddress(address string) bool {
	local, domain, found := strings.Cut(address, "@")
	if !found || local == "" || strings.ContainsFunc(address, unicode.IsSpace) {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" || strings.ContainsFunc(label, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
		}) {
			return false
		}
	}
	return true
}
