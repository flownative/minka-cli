package basecamp

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

const maskedAddress = "a\u2022\u2022\u2022@\u2022\u2022\u2022\u2022\u2022\u2022\u2022.\u2022\u2022\u2022\u2022"

func mustRefer(t *testing.T, r *registry, p person, present bool) string {
	t.Helper()
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
