package basecamp

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

const maskedAddress = "a\u2022\u2022\u2022@\u2022\u2022\u2022\u2022\u2022\u2022\u2022.\u2022\u2022\u2022\u2022"

// someTime is when the people of these tests were created and updated: a
// person without their times does not come into a bundle.
const someTime = "2020-01-01T08:00:00.000Z"

// timed gives the people a registry has listed their times.
func timed(r *registry) {
	for id, p := range r.account {
		p.CreatedAt, p.UpdatedAt = someTime, someTime
		r.account[id] = p
	}
}

func mustRefer(t *testing.T, r *registry, p person, present bool) string {
	t.Helper()
	p.CreatedAt, p.UpdatedAt = someTime, someTime
	reference, err := r.refer(&p, present)
	if err != nil || reference == nil {
		t.Fatalf("referring to person %d: %v, %v", p.ID, reference, err)
	}
	return *reference
}

func TestOnlyWhoIsReferredToComesIntoTheBundle(t *testing.T) {
	r := newRegistry()
	err := r.listAccount([]person{
		{ID: 3, Name: "Linus Placeholder"},
		{ID: 2, Name: "Grace Sample", EmailAddress: "grace@example.test"},
		{ID: 4, Name: "Edsger Invented", EmailAddress: "edsger@example.test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	timed(r)
	if len(r.sorted()) != 0 || r.leftOut() != 3 {
		t.Fatalf("the list of the account brought %d people in and left %d out", len(r.sorted()), r.leftOut())
	}

	if got := mustRefer(t, r, person{ID: 3}, true); got != "3" {
		t.Errorf("reference %q", got)
	}
	// What the account says about somebody wins over what a record says.
	mustRefer(t, r, person{ID: 2, Name: "An older name", EmailAddress: "older@example.test"}, false)
	// Somebody the account does not know has left, unless a record proves
	// that they are there.
	mustRefer(t, r, person{ID: 1, Name: "Former Colleague"}, false)
	mustRefer(t, r, person{ID: 9, Name: "A Guest"}, true)
	mustRefer(t, r, person{ID: 8, Name: "Later A Member"}, false)
	mustRefer(t, r, person{ID: 8, Name: "Later A Member"}, true)
	mustRefer(t, r, person{ID: 3}, false)

	if reference, err := r.refer(nil, false); reference != nil || err != nil {
		t.Errorf("nobody gives the reference %v (%v)", reference, err)
	}
	if reference, err := r.refer(&person{}, false); reference != nil || err != nil {
		t.Errorf("a person without an ID gives the reference %v (%v)", reference, err)
	}

	type row struct {
		id     int64
		name   string
		email  string
		listed bool
	}
	var got []row
	for _, seen := range r.sorted() {
		got = append(got, row{seen.ID, seen.Name, seen.EmailAddress, seen.listed})
	}
	want := []row{
		{1, "Former Colleague", "", false},
		{2, "Grace Sample", "grace@example.test", true},
		{3, "Linus Placeholder", "", true},
		{8, "Later A Member", "", true},
		{9, "A Guest", "", true},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the people of the bundle:\n got %v\nwant %v", got, want)
	}
	if r.leftOut() != 1 {
		t.Errorf("%d of the account left out, want 1", r.leftOut())
	}
}

func TestAMaskAnywhereInTheAccountEndsTheRun(t *testing.T) {
	r := newRegistry()
	err := r.listAccount([]person{
		{ID: 1, EmailAddress: "ada@example.test"}, {ID: 2}, {ID: 3, EmailAddress: "grace@example.test"},
		{ID: 4, EmailAddress: "linus@example.test"}, {ID: 5, EmailAddress: maskedAddress}, {ID: 6, EmailAddress: "no address"},
	})
	expectKind(t, err, Forbidden)
	if !strings.Contains(err.Error(), "1 of 5") || strings.Contains(err.Error(), "\u2022") {
		t.Errorf("the message: %s", err)
	}
	if len(r.account) != 0 {
		t.Errorf("%d people were taken from a list with a mask in it", len(r.account))
	}

	_, err = r.refer(&person{ID: 7, EmailAddress: maskedAddress}, false)
	expectKind(t, err, Forbidden)
	if len(r.sorted()) != 0 {
		t.Error("somebody with a masked address came into the bundle")
	}
}

func TestAnAddressThatIsNoneIsLeftOutAndNotRepaired(t *testing.T) {
	r := newRegistry()
	err := r.listAccount([]person{
		{ID: 30, EmailAddress: `"quoted@example.test"`}, {ID: 4, EmailAddress: "not referred to"},
		{ID: 5, EmailAddress: "ada@example.test"}, {ID: 6, EmailAddress: "grace@example.test"}, {ID: 7, EmailAddress: "linus@example.test"},
	})
	if err != nil {
		t.Fatalf("two addresses of five that are none ended the run: %v", err)
	}
	timed(r)
	mustRefer(t, r, person{ID: 30}, true)
	mustRefer(t, r, person{ID: 5}, true)
	mustRefer(t, r, person{ID: 200, EmailAddress: "a***@***.***"}, false)
	mustRefer(t, r, person{ID: 9, EmailAddress: "former colleague"}, false)

	addresses := map[int64]string{}
	for _, seen := range r.sorted() {
		addresses[seen.ID] = seen.EmailAddress
	}
	if want := map[int64]string{5: "ada@example.test", 9: "", 30: "", 200: ""}; !reflect.DeepEqual(addresses, want) {
		t.Errorf("addresses %v, want %v", addresses, want)
	}
	// By number, not by text, and nobody the bundle does not hold.
	if got := r.invalidAddresses(); !slices.Equal(got, []string{"9", "30", "200"}) {
		t.Errorf("invalid addresses %v", got)
	}
}

func TestMostAddressesBeingNoneIsNoFlawInTheData(t *testing.T) {
	other := "a\u25cf\u25cf\u25cf@\u25cf\u25cf\u25cf.\u25cf\u25cf"
	r := newRegistry()
	err := r.listAccount([]person{{ID: 1, EmailAddress: "ada@example.test"}, {ID: 2, EmailAddress: other}, {ID: 3, EmailAddress: other}, {ID: 4}})
	expectKind(t, err, API)
	if !strings.Contains(err.Error(), "2 of 3") || strings.Contains(err.Error(), "\u25cf") {
		t.Errorf("the message: %s", err)
	}
	if len(r.account) != 0 {
		t.Errorf("%d people were taken", len(r.account))
	}

	// Half is not most.
	if err := newRegistry().listAccount([]person{{ID: 1, EmailAddress: "ada@example.test"}, {ID: 2, EmailAddress: other}}); err != nil {
		t.Errorf("one of two ended the run: %v", err)
	}
}

func TestWhitespaceAroundAnAddressIsTakenAway(t *testing.T) {
	r := newRegistry()
	if err := r.listAccount([]person{{ID: 1, EmailAddress: " ada@example.test\n"}, {ID: 2, EmailAddress: "  \t"}}); err != nil {
		t.Fatal(err)
	}
	timed(r)
	mustRefer(t, r, person{ID: 1}, true)
	mustRefer(t, r, person{ID: 2}, true)
	mustRefer(t, r, person{ID: 3, EmailAddress: "\tformer@example.test "}, false)

	want := map[int64]string{1: "ada@example.test", 2: "", 3: "former@example.test"}
	for _, seen := range r.sorted() {
		if seen.EmailAddress != want[seen.ID] {
			t.Errorf("person %d carries the address %q, want %q", seen.ID, seen.EmailAddress, want[seen.ID])
		}
	}
	if got := r.invalidAddresses(); len(got) != 0 {
		t.Errorf("whitespace made an address count as none: %v", got)
	}
}

// The applier stops at a person without their times, which is after a run of
// hours: the run says so where somebody first comes in.
func TestSomebodyWithoutTheirTimesEndsTheRunWhereTheyFirstComeIn(t *testing.T) {
	r := newRegistry()
	if err := r.listAccount([]person{{ID: 1, Name: "Ada Example", CreatedAt: someTime}}); err != nil {
		t.Fatal(err)
	}
	for what, c := range map[string]struct {
		named   person
		message string
	}{
		"somebody the account lists": {person{ID: 1}, "Basecamp does not say when person 1 was updated"},
		"a bot without either":       {person{ID: 801, Name: "Build Robot"}, "Basecamp does not say when person 801 was created and updated"},
		"somebody deleted":           {person{ID: 802, Name: "Deleted Somebody", UpdatedAt: someTime}, "Basecamp does not say when person 802 was created"},
	} {
		reference, err := r.refer(&c.named, false)
		var failed *Error
		if !errors.As(err, &failed) || failed.Kind != API || failed.Message != c.message || reference != nil {
			t.Errorf("%s: got %v and %v, want the error %q", what, reference, err, c.message)
		}
	}
	if len(r.sorted()) != 0 {
		t.Errorf("%d people without their times came into the bundle", len(r.sorted()))
	}
}

// The kind of person says who is retired, wherever somebody comes from: the
// list of an account holds every kind for an admin and users alone otherwise.
func TestTheKindOfPersonSaysWhoIsRetired(t *testing.T) {
	r := newRegistry()
	err := r.listAccount([]person{
		{ID: 1, PersonableType: "User"}, {ID: 2, PersonableType: "Integration"}, {ID: 3, PersonableType: "Tombstone"},
		{ID: 4, PersonableType: "DummyUser"}, {ID: 5, PersonableType: "Outsider", EmailAddress: "outsider@example.test"},
		{ID: 6, PersonableType: "Client"}, {ID: 7, PersonableType: "SomethingNew"}, {ID: 8}, {ID: 9},
	})
	if err != nil {
		t.Fatal(err)
	}
	timed(r)
	want := map[int64]bool{}
	refer := func(p person, present, retired bool) {
		mustRefer(t, r, p, present)
		want[p.ID] = retired
	}
	// From the list, where a record says less or something else about them.
	for id := int64(1); id <= 9; id++ {
		refer(person{ID: id}, id == 9, id != 1 && id != 6 && id != 8 && id != 9)
	}
	// From the members of a project.
	refer(person{ID: 11, PersonableType: "User"}, true, false)
	refer(person{ID: 12, PersonableType: "Integration"}, true, true)
	refer(person{ID: 13}, true, false)
	// From a record alone.
	refer(person{ID: 21, PersonableType: "User"}, false, false)
	refer(person{ID: 22, PersonableType: "Client"}, false, false)
	refer(person{ID: 23, PersonableType: "Integration"}, false, true)
	refer(person{ID: 24, PersonableType: "Tombstone", EmailAddress: "gone@example.test"}, false, true)
	refer(person{ID: 25, PersonableType: "Outsider"}, false, true)
	refer(person{ID: 26, PersonableType: "user"}, false, true)
	refer(person{ID: 27}, false, true)

	for _, seen := range r.sorted() {
		if seen.retired() != want[seen.ID] {
			t.Errorf("person %d of the kind %q comes with retired %v", seen.ID, seen.PersonableType, seen.retired())
		}
	}
	// Whoever keeps an address keeps it, retired or not.
	for _, seen := range r.sorted() {
		if (seen.ID == 5 || seen.ID == 24) && seen.EmailAddress == "" {
			t.Errorf("person %d lost their address", seen.ID)
		}
	}
	// Somebody whose kind the list leaves out and a record says.
	late := newRegistry()
	if err := late.listAccount([]person{{ID: 1}}); err != nil {
		t.Fatal(err)
	}
	timed(late)
	mustRefer(t, late, person{ID: 1, PersonableType: "Integration"}, false)
	if !late.sorted()[0].retired() {
		t.Error("a bot whose kind only a record says is not retired")
	}
}
