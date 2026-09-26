package basecamp

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/bundle"
)

// visibleToClientsOf is what the source says of this very record. It guards
// nothing: the rule lives at the places that call it, each of which has a record
// of its own and the tool it stands in within reach.
//
// The rule is that a record's word is its own. Two tools of one kind in one
// project differ, and of the records of a tool that is shown to clients some are
// kept from them, so a record that took the word of its tool would show a client
// what the source hides. Where the source says nothing, nothing is written and
// Minka leaves what it holds alone: in doubt, private.
func visibleToClientsOf(origin recording) *bool {
	return origin.VisibleToClients
}

// visibilityOfItsOwn are the kinds of record a bundle has a word of its own for
// whether clients see them. Everything else follows what it hangs on: a to-do
// its list, a card its board, an answer its question, a comment what it was
// written under. Where such a record carries a word all the same, it is counted
// and left out, because a second truth about one record is one truth too many.
// A forward and a reply follow their tool like the rest: an Email forwards
// tool is shown to clients as a whole or not at all.
var visibilityOfItsOwn = map[string]bool{"tool": true, "post": true, "to-do list": true, "question": true}

// toolKindsARunLeaves are the kinds of Basecamp tool Minka has a counterpart for
// that a run does not extract. Neither their word about clients nor anything
// standing in them reaches a bundle.
var toolKindsARunLeaves = map[string]bool{"vault": true}

// SaysNothingAboutClients tells of a kind of dock entry that a bundle carries no
// word about clients for such a tool, because a run does not extract that kind
// at all. What a run extracts is asked first, so a kind that comes to be
// extracted leaves this by itself.
func SaysNothingAboutClients(kind string) bool {
	_, extracted := toolKinds[kind]
	return !extracted && toolKindsARunLeaves[kind]
}

// clientsOf is what a bundle says of the clients of a project: whether it works
// with them, and which company. Both stand or neither does, because Minka reads
// a project that names a client company and no access as one whose clients have
// none; a source that says nothing of clients at all leaves both out.
func (e *Extractor) clientsOf(p project) (*bool, *bundle.CompanyOrNone, error) {
	if p.ClientsEnabled == nil {
		return nil, nil, nil
	}
	company, err := companyOf("project", p.ID, p.ClientCompany)
	if err != nil {
		return nil, nil, err
	}
	return p.ClientsEnabled, company, nil
}

// companyOf is the company a record names, as a bundle carries it: the company,
// or the word that the source names none. The name comes without the whitespace
// around it that Minka takes off anyway.
//
// A company a bundle cannot carry ends the run instead of being left out of the
// record. Minka reads a company that is missing and one that is null the same
// way and drops the company it holds, so a record written without the company
// the source names says of somebody that they belong to nobody, and takes away
// on the next run what the run before wrote.
//
// No message repeats the name: a field that holds no name holds something, and
// what it holds is nobody's to print. The identifier says which company it is.
func companyOf(what string, id int64, named *companyAtTheSource) (*bundle.CompanyOrNone, error) {
	if named == nil {
		return bundle.NoCompany(), nil
	}
	if named.ID == 0 {
		return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp names the company of %s %d without an identifier", what, id)}
	}
	name := strings.TrimSpace(named.Name)
	company := fmt.Sprintf("company %d, named by %s %d", named.ID, what, id)
	switch {
	case name == "":
		return nil, &Error{Kind: API, Message: company + ", has no name"}
	case utf8.RuneCountInString(name) > maxName:
		return nil, &Error{Kind: API, Message: fmt.Sprintf("%s, has a name longer than the %d characters a name may have", company, maxName)}
	case strings.ContainsFunc(name, unicode.IsControl):
		return nil, &Error{Kind: API, Message: company + ", has a name that holds a character nobody can see"}
	}
	return bundle.TheCompany(sourceID(named.ID), name), nil
}
