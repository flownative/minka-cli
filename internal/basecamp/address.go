package basecamp

import (
	"regexp"
	"strings"
)

// The one rule for every e-mail address a bundle carries: the address of a
// person, the sender of a forwarded mail and the sender of an answer by mail.
// Minka reads all three through the same value object, and every one of them
// stops the applier where it refuses one, after a walk of hours that cannot be
// repeated cheaply. So the rule here is Minka's rule: what passes here passes
// there.
//
// It is a reading of PHP's `filter_var` with the plain local part Minka asks
// for on top of it, measured against that reader rather than guessed. Two
// shapes Minka takes are left out all the same, because the domain is written
// as a bracketed IP literal and no mailbox of a person is written that way:
// matching PHP's reading of one exactly is a risk in the direction that stops a
// run, and the direction that merely loses an address is the cheaper one.
const (
	// maxAddressLength is the whole of an address, maxLocalPartLength what
	// stands before the "@" and maxLabelLength one label of the domain, in
	// bytes, as the standards of mail draw them and PHP's reader counts them.
	maxAddressLength    = 254
	maxLocalPartLength  = 64
	maxLabelLength      = 63
	minDomainLabelCount = 2
)

var (
	// plainLocalPart is what stands before the last "@": the ordinary
	// characters in groups separated by single dots, and never the quoted
	// form, which another reader could take for the end of the address.
	plainLocalPart = regexp.MustCompile("^[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+(?:\\.[A-Za-z0-9!#$%&'*+/=?^_`{|}~-]+)*$")
	// domainLabel is one label of the domain: letters, digits and hyphens,
	// beginning and ending with neither a hyphen nor nothing.
	domainLabel = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)
	// topDomainLabel is the last label, which names no host of its own and
	// begins with a letter.
	topDomainLabel = regexp.MustCompile(`^[A-Za-z](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)
)

// anAddressMinkaTakes is the address to write, whether a pair of quotation
// marks had to come off it first, and whether what is left is an address at
// all. The whitespace around it goes, as it does at the far end.
//
// The quotation marks are the one place a run repairs what the source says
// instead of passing it on. A mail header holds whole addresses written inside
// a pair of them often enough to be worth the exception, none of those is
// anybody's to repair where it stands, and Minka refuses every one of them. It
// is that pair and nothing else: no other character is trimmed, unfolded or
// mended, and the stripping comes before the reading, because a tighter reading
// alone would turn away the addresses the stripping saves.
//
// A pair around the whole of an address is never a loss. An address Minka takes
// ends in a letter or a digit, so one that ends in a quotation mark is refused
// whatever is done with it, and the quoted local part — `"ada b"@example.test`
// — keeps its closing mark before the "@" and is left as it stands.
func anAddressMinkaTakes(value string) (address string, unquoted, anAddress bool) {
	address, unquoted = withoutAPairOfQuotationMarks(strings.TrimSpace(value))
	if !minkaTakesTheAddress(address) {
		return "", false, false
	}
	return address, unquoted, true
}

// withoutAPairOfQuotationMarks takes one mark off each end where both are
// there, and says whether it did.
func withoutAPairOfQuotationMarks(value string) (string, bool) {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value[1 : len(value)-1], true
	}
	return value, false
}

// minkaTakesTheAddress is Minka's own reading of an address, as strictly and no
// more strictly. What it takes is ASCII throughout, because every part of it is
// asked of characters of the first 128 and of no others.
func minkaTakesTheAddress(address string) bool {
	at := strings.LastIndex(address, "@")
	if at < 0 || len(address) > maxAddressLength {
		return false
	}
	local, domain := address[:at], address[at+1:]
	if len(local) > maxLocalPartLength || !plainLocalPart.MatchString(local) {
		return false
	}
	labels := strings.Split(domain, ".")
	if len(labels) < minDomainLabelCount {
		return false
	}
	last := len(labels) - 1
	for i, label := range labels {
		shape := domainLabel
		if i == last {
			shape = topDomainLabel
		}
		if len(label) > maxLabelLength || !shape.MatchString(label) {
			return false
		}
	}
	return true
}
