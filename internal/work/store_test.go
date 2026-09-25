package work

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const aHostOfTheStore = "https://minka.example.com"

func TestTheStoreIsWrittenSoThatOnlyItsOwnerReadsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minka", "work.json")

	handle, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	handle.File.Host("https://minka.example.com").Start(Card{ID: "a-card", StartedAt: time.Now()})
	if err := handle.Save(); err != nil {
		t.Fatal(err)
	}
	handle.Close()

	if Untouched(path) {
		t.Fatal("the store was not written")
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("the directory has %04o, want 0700", perm)
	}
}

// The safeguard: the store is saved by renaming a file that is complete, so a
// save that cannot be carried out leaves what stands there whole instead of
// emptying it first.
func TestAStoreThatCannotBeWrittenKeepsWhatItHeld(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "work.json")

	handle, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	handle.File.Host(aHostOfTheStore).Start(Card{ID: "the-one-card", StartedAt: time.Now()})
	if err := handle.Save(); err != nil {
		t.Fatal(err)
	}
	handle.Close()
	held, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Nothing can be created in the directory any more, so no complete file
	// can be put in place of the one that is there.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	handle, err = Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()
	handle.File.Host(aHostOfTheStore).Start(Card{ID: "another-card", StartedAt: time.Now()})
	if err := handle.Save(); err == nil {
		t.Fatal("a save that cannot be carried out says it went well")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(held) {
		t.Errorf("the store was changed by a save that failed:\n%s", after)
	}
}

func TestStartingACardAgainSaysItIsStillTheTurnOfThatCard(t *testing.T) {
	entry := &Host{}
	first := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)

	if !entry.Start(Card{ID: "a-card", StartedAt: first}) {
		t.Error("the first start is not a new card")
	}
	if entry.Start(Card{ID: "a-card", StartedAt: first.Add(time.Hour)}) {
		t.Error("the second start is a new card")
	}
	if len(entry.Cards) != 1 {
		t.Fatalf("%d cards, want 1", len(entry.Cards))
	}
	if !entry.Cards[0].StartedAt.Equal(first.Add(time.Hour)) {
		t.Error("starting again did not say that the card is still the one")
	}
}

// A card that is taken up anew goes by that moment. It is taken up whole, so
// the sign of life of the work before says nothing about the work that starts
// now — and it is on that that the window of a card rests.
func TestACardTakenUpAnewGoesByTheNewMoment(t *testing.T) {
	first := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Host{}
	entry.Start(Card{ID: "a-card", StartedAt: first, Session: "mine"})
	entry.StillAtIt("mine", first.Add(time.Hour))

	again := first.Add(2 * time.Hour)
	entry.Start(Card{ID: "a-card", StartedAt: again, Session: "mine"})
	if got := lastSeen(entry.Cards[0]); !got.Equal(again) {
		t.Errorf("the card was last seen at %s, want the moment it was taken up anew", got)
	}
}

// A session that dies says nothing more about its cards, and nobody else knows
// they are over: what nothing refreshes runs out by itself.
func TestACardNobodyStartedAgainRunsOut(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Host{Cards: []Card{
		{ID: "old", StartedAt: began},
		{ID: "fresh", StartedAt: began.Add(ForgottenAfter - time.Minute)},
	}}

	if forgotten := entry.ForgetWhatRanOut(began.Add(ForgottenAfter)); forgotten != 1 {
		t.Fatalf("%d cards were forgotten, want 1", forgotten)
	}
	if cards := entry.CardIDs(); len(cards) != 1 || cards[0] != "fresh" {
		t.Errorf("%v is left", cards)
	}
}

// The list belongs to the machine and a signal carries all of it, so the sign
// of life has to belong to the session that took a card up: otherwise one
// session at work would keep the cards of a session that died alive.
func TestOnlyTheSessionThatTookACardUpKeepsItAlive(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Host{Cards: []Card{
		{ID: "of the busy session", StartedAt: began, Session: "busy"},
		{ID: "of the session that died", StartedAt: began, Session: "gone"},
		{ID: "of a person at a terminal", StartedAt: began},
	}}

	// The one session says it is still at it, every hour of a working day.
	for hour := 1; hour <= 9; hour++ {
		entry.StillAtIt("busy", began.Add(time.Duration(hour)*time.Hour))
	}

	if forgotten := entry.ForgetWhatRanOut(began.Add(ForgottenAfter + time.Minute)); forgotten != 2 {
		t.Fatalf("%d cards were forgotten, want the two nobody said anything about", forgotten)
	}
	if cards := entry.CardIDs(); len(cards) != 1 || cards[0] != "of the busy session" {
		t.Errorf("%v is left, want the card of the session that kept working", cards)
	}
}

// Saying it again costs a write, and the moment is read against a window of
// hours, so it is written down once a minute and not once a tool call.
func TestSayingItAgainIsWrittenDownOnceAMinute(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Host{Cards: []Card{{ID: "one", StartedAt: began, Session: "mine"}}}

	if !entry.StillAtIt("mine", began.Add(SeenEvery)) {
		t.Fatal("the first sign of life was not written down")
	}
	if entry.StillAtIt("mine", began.Add(SeenEvery+SeenEvery/2)) {
		t.Error("a second sign of life within the minute was written down")
	}
	if !entry.StillAtIt("mine", began.Add(2*SeenEvery+time.Second)) {
		t.Error("the sign of life after the minute was not written down")
	}
	// A session that is not this one, and a session with no name at all.
	if entry.StillAtIt("somebody else", began.Add(time.Hour)) || entry.StillAtIt("", began.Add(time.Hour)) {
		t.Error("a card was refreshed by a session that does not hold it")
	}
}

// The safeguard: a card that was taken up where nothing said who was asking
// belongs to nobody, so no sign of life reaches it — neither one that names a
// session nor one that names none, which is what a heartbeat outside a session
// brings. Such a card goes by the moment it was started.
func TestACardWithoutASessionIsKeptAliveByNobody(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Host{Cards: []Card{{ID: "of a person at a terminal", StartedAt: began}}}

	for hour := 1; hour <= 9; hour++ {
		at := began.Add(time.Duration(hour) * time.Hour)
		if entry.StillAtIt("a session at work", at) {
			t.Fatalf("a session that holds no card refreshed it after %d hours", hour)
		}
		if entry.StillAtIt("", at) {
			t.Fatalf("a sign of life that names no session refreshed the card after %d hours", hour)
		}
	}

	if forgotten := entry.ForgetWhatRanOut(began.Add(ForgottenAfter + time.Minute)); forgotten != 1 {
		t.Fatalf("%d cards were forgotten, want the one nobody said anything about", forgotten)
	}
	if cards := entry.CardIDs(); len(cards) != 0 {
		t.Errorf("%v is left, want none", cards)
	}
}
