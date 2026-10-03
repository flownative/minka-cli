package fakebasecamp

import (
	"regexp"
	"strconv"
)

var subscriptionPattern = regexp.MustCompile(`^buckets/([0-9]+)/recordings/([0-9]+)/subscription\.json$`)

// subscription answers whom a recording is handed to, for every kind of record
// that has followers: a question, a card, a message, a to-do, a document and an
// upload. The second value tells whether the path named one of them; every other
// recording is not found, as a path the account does not know.
func (a *Account) subscription(path string) ([]byte, bool) {
	match := subscriptionPattern.FindStringSubmatch(path)
	if match == nil {
		return nil, false
	}
	p := a.project(match[1])
	if p == nil {
		return nil, false
	}
	handedTo, found := a.subscribersOf(p, match[2])
	if !found {
		return nil, false
	}
	subscribers := []map[string]any{}
	for _, subscriber := range handedTo {
		subscribers = append(subscribers, a.person(subscriber))
	}
	if a.SubscriptionAnswer != nil {
		return encode(a.SubscriptionAnswer), true
	}
	// The subscription of a recording is no list and has no pages.
	return encode(map[string]any{
		"subscribed": true, "count": len(handedTo), "url": a.root() + path, "subscribers": subscribers,
	}), true
}

func (a *Account) subscribersOf(p *Project, id string) ([]Person, bool) {
	var subscribers []Person
	found := false
	is := func(candidate int64, of []Person) {
		if strconv.FormatInt(candidate, 10) == id {
			subscribers, found = of, true
		}
	}
	a.eachCard(p, func(_ *List, _ *OnHold, card *Card) { is(card.ID, card.Subscribers) })
	for t := range p.Dock {
		for _, post := range p.Dock[t].Posts {
			is(post.ID, post.Subscribers)
		}
		for _, question := range p.Dock[t].Questions {
			is(question.ID, question.Subscribers)
		}
	}
	a.eachTodo(p, func(one *Todo) { is(one.ID, one.Subscribers) })
	a.eachEntry(p, func(entry *Entry) { is(entry.ID, entry.Subscribers) })
	a.eachVault(p, func(in vault) {
		for _, document := range in.documents {
			is(document.ID, document.Subscribers)
		}
		for _, upload := range in.uploads {
			is(upload.ID, upload.Subscribers)
		}
	})
	return subscribers, found
}
