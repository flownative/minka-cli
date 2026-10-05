package basecamp

import (
	"fmt"

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
//
// A folder, a document and an upload of Docs & Files carry the word at every
// depth, and not only at the top of their tool. Minka reads it below the top as
// well, and it is the one thing there that can hide a folder at the top the
// source shows: where something below is hidden from clients, the import hides
// the top folder with everything in it and names it. An entry that left the word
// out would be imported in the open under a folder clients see, and what a client
// has seen cannot be taken back.
var visibilityOfItsOwn = map[string]bool{"tool": true, "post": true, "to-do list": true, "question": true,
	"folder": true, "document": true, "upload": true, "event": true}

// clientsOf is what a bundle says of the clients of a project: whether it works
// with them, and which company. Both stand or neither does, because Minka reads
// a project that names a client company and no access as one whose clients have
// none; a source that says nothing of clients at all leaves both out.
func (e *Extractor) clientsOf(p project) (*bool, *bundle.CompanyOrNone, error) {
	if p.ClientsEnabled == nil {
		return nil, nil, nil
	}
	company, madeToFit, err := companyOf("project", p.ID, p.ClientCompany)
	if err != nil {
		return nil, nil, err
	}
	if p.ClientCompany != nil {
		e.noteMadeToFit("company", p.ClientCompany.ID, madeToFit)
	}
	return p.ClientsEnabled, company, nil
}

// companyOf is the company a record names, as a bundle carries it: the company,
// or the word that the source names none. Its name is made into one Minka takes,
// as every name is, and madeToFit says what that took, for whoever writes the
// record to name it.
//
// A company named without an identifier ends the run instead of being left out
// of the record. Minka reads a company that is missing and one that is null the
// same way and drops the company it holds, so a record written without the
// company the source names says of somebody that they belong to nobody, and
// takes away on the next run what the run before wrote.
func companyOf(what string, id int64, named *companyAtTheSource) (company *bundle.CompanyOrNone, madeToFit []string, err error) {
	if named == nil {
		return bundle.NoCompany(), nil, nil
	}
	if named.ID == 0 {
		return nil, nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp names the company of %s %d without an identifier", what, id)}
	}
	name, madeToFit := aTitleOfAtMost(named.Name, noName, maxName, foldedAsANestedName)
	return bundle.TheCompany(sourceID(named.ID), name), madeToFit, nil
}
