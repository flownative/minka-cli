package basecamp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/flownative/minka-cli/internal/bundle"
)

// PeopleLeftOff counts, per reason, the people a list of the source names that
// a bundle cannot name in Minka: whom a question is asked of, whom an item is
// followed by. One reason each, in the order of the fields.
type PeopleLeftOff struct {
	NotInTheProject  int `json:"notInTheProject"`
	Clients          int `json:"clients"`
	Retired          int `json:"retired"`
	WithoutAnAddress int `json:"withoutAnAddress"`
}

// SubscriberCounts says what became of who follows a post, a card, a to-do, a
// document or an upload at the source.
type SubscriberCounts struct {
	// Lists counts the records that carry `subscribers`.
	Lists int `json:"lists"`
	// LeftOff counts the people those lists would have named and Minka cannot
	// put on an item, over all records.
	LeftOff PeopleLeftOff `json:"leftOff"`
	// Unread names the records whose list the source would not hand out. They
	// carry no `subscribers`, so Minka follows them the way it always did.
	Unread []UnreadSubscribers `json:"unread"`
}

// UnreadSubscribers is a record whose followers the source would not hand out,
// with what it answered: notFound or forbidden.
type UnreadSubscribers struct {
	Record          string `json:"record"`
	SourceID        string `json:"sourceId"`
	ProjectSourceID string `json:"projectSourceId"`
	Why             string `json:"why"`
}

// Why a list of followers stayed unread: what Basecamp answered.
const (
	SubscribersNotFound  = "notFound"
	SubscribersForbidden = "forbidden"
)

// subscription is who a recording is handed to. The list is kept raw, so that an
// answer without one is told apart from a list that names nobody. Count is how
// many people the source says the list holds.
type subscription struct {
	Count       *int            `json:"count"`
	Subscribers json.RawMessage `json:"subscribers"`
}

// handedTo reads whom Basecamp hands a recording to. Every list read from it is
// taken as the whole truth somewhere, so an answer that is not plainly that ends
// the run: one without a list, a list that holds fewer or more people than the
// answer counts, and an entry that names nobody by an ID. Read as nobody, each of
// them would take everybody off the item.
func handedTo(ctx context.Context, source Source, bucket, recording int64) ([]person, error) {
	path := fmt.Sprintf("buckets/%d/recordings/%d/subscription.json", bucket, recording)
	answer, err := getOne[subscription](ctx, source, path)
	if err != nil {
		return nil, err
	}
	if len(answer.Subscribers) == 0 || string(answer.Subscribers) == "null" {
		return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s without saying who it is handed to", path)}
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(answer.Subscribers, &entries); err != nil {
		return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with something that cannot be read: %s", path, err)}
	}
	if answer.Count != nil && *answer.Count != len(entries) {
		return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with %d people and counts %d, so the list is not all of them", path, len(entries), *answer.Count)}
	}
	people := make([]person, 0, len(entries))
	for i, entry := range entries {
		// A null entry reads as a person without an ID.
		var one person
		if err := json.Unmarshal(entry, &one); err != nil {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with something that cannot be read: %s", path, err)}
		}
		if one.ID <= 0 {
			return nil, &Error{Kind: API, Message: fmt.Sprintf("Basecamp answered %s with entry %d naming nobody by an ID", path, i+1)}
		}
		people = append(people, one)
	}
	return people, nil
}

// whomMinkaTakes are those of the people named who Minka can name on a record
// of the project, in the order of their IDs and each once, so that two runs over
// an unchanged account write the same bytes. The second value is how many fell
// out, each counted in leftOff under its reason.
//
// Only a member of the project is somebody the bundle carries in it, and every
// member came in where the project was written. Of them, a client is in no
// project in Minka, and somebody who has left or comes without an address is
// retired there, which makes them no member either. An agent comes without an
// address and is given one there, so it stays.
func (e *Extractor) whomMinkaTakes(bucket int64, named []person, leftOff *PeopleLeftOff) ([]string, int) {
	members := e.members[bucket]
	taken := map[int64]bool{}
	fell := 0
	var ids []int64
	for _, one := range named {
		if taken[one.ID] {
			continue
		}
		taken[one.ID] = true
		known, _ := e.people.known(one.ID)
		switch {
		case !members[one.ID]:
			leftOff.NotInTheProject++
			fell++
		case known.aClient():
			leftOff.Clients++
			fell++
		case known.retired():
			leftOff.Retired++
			fell++
		case known.EmailAddress == "" && !e.people.isAnAgent(one.ID): // the agents'
			leftOff.WithoutAnAddress++
			fell++
		default:
			ids = append(ids, one.ID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	taking := make([]string, 0, len(ids))
	for _, id := range ids {
		taking = append(taking, sourceID(id))
	}
	return taking, fell
}

// subscribersOf is whom a record is followed by at the source, for its
// `subscribers`. Where the field is there, Minka takes it as the whole truth and
// takes everybody else off the item, so nil, and no field, stands for every list
// this run does not have:
//
//   - A record in the trash is not asked about. Minka changes nothing on one.
//   - A list the source answers as not there or forbidden is named in the report
//     and the run goes on. Without the field Minka follows the item as it always
//     did, and a later run brings the list once the source hands it out.
//
// Every other failure ends the run as it does anywhere. An empty list is a list:
// nobody follows the item.
func (e *Extractor) subscribersOf(ctx context.Context, what string, bucket, id int64, status string) (*[]string, error) {
	if status == bundle.StatusTrashed {
		return nil, nil
	}
	named, err := handedTo(ctx, e.source, bucket, id)
	var failed *Error
	if errors.As(err, &failed) && (failed.Kind == NotFound || failed.Kind == Forbidden) {
		why := SubscribersNotFound
		if failed.Kind == Forbidden {
			why = SubscribersForbidden
		}
		e.subscribers.Unread = append(e.subscribers.Unread, UnreadSubscribers{Record: what, SourceID: sourceID(id), ProjectSourceID: sourceID(bucket), Why: why})
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	following, _ := e.whomMinkaTakes(bucket, named, &e.subscribers.LeftOff)
	e.subscribers.Lists++
	return &following, nil
}
