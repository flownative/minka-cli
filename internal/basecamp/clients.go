package basecamp

import (
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// visibleToClientsOf is what a bundle says of a record whose word about clients
// is its own: what the source says of that very record, and never what it says
// of the tool the record stands in.
//
// Two tools of one kind in one project differ, and of the records of a tool that
// is shown to clients some are kept from them. A record that took the word of
// its tool would therefore show a client what the source hides, and nothing
// afterwards tells that from the truth. Where the source says nothing, nothing
// is written and Minka leaves what it holds alone: in doubt, private.
func visibleToClientsOf(origin recording) *bool {
	return origin.VisibleToClients
}

// visibilityOfItsOwn are the kinds of record a bundle has a word of its own for
// whether clients see them. Everything else follows what it hangs on: a to-do
// its list, a card its board, an answer its question, a comment what it was
// written under. Where such a record carries a word all the same, it is counted
// and left out, because a second truth about one record is one truth too many.
var visibilityOfItsOwn = map[string]bool{"tool": true, "post": true, "to-do list": true, "question": true}

// clientsOf is what a bundle says of the clients of a project: whether it works
// with them, and which company. Both stand or neither does, because Minka reads
// a project that names a client company and no access as one whose clients have
// none; a source that says nothing of clients at all leaves both out.
func (e *Extractor) clientsOf(p project) (*bool, *bundle.CompanyOrNone) {
	if p.ClientsEnabled == nil {
		return nil, nil
	}
	return p.ClientsEnabled, e.companyOf(p.ClientCompany)
}

// companyOf is the company a record names, as a bundle says it. A company
// without an identifier or without a name is left out and counted: Minka ends a
// run over such a one, and the record comes saying that the source names none.
func (e *Extractor) companyOf(named *companyAtTheSource) *bundle.CompanyOrNone {
	if named == nil {
		return bundle.NoCompany()
	}
	if named.ID == 0 || strings.TrimSpace(named.Name) == "" {
		e.companiesLeftOut++
		return bundle.NoCompany()
	}
	return bundle.TheCompany(sourceID(named.ID), named.Name)
}
