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
	handle.File.Account("https://minka.example.com").List(NoSession).Start(Card{ID: "a-card", StartedAt: time.Now()})
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
	handle.File.Account(aHostOfTheStore).List(NoSession).Start(Card{ID: "the-one-card", StartedAt: time.Now()})
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
	handle.File.Account(aHostOfTheStore).List(NoSession).Start(Card{ID: "another-card", StartedAt: time.Now()})
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
	entry := &List{}
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
	host := &Account{}
	entry := host.List("mine")
	entry.Start(Card{ID: "a-card", StartedAt: first})
	host.StillAtIt("mine", first.Add(time.Hour))

	again := first.Add(2 * time.Hour)
	entry.Start(Card{ID: "a-card", StartedAt: again})
	if seen := entry.Cards[0].SeenAt; !seen.IsZero() {
		t.Errorf("the card taken up anew carries the sign of life %s of the work before", seen)
	}
	if got := lastSeen(entry.Cards[0]); !got.Equal(again) {
		t.Errorf("the card was last seen at %s, want the moment it was taken up anew", got)
	}
}

// A session that dies says nothing more about its cards, and nobody else knows
// they are over: what nothing refreshes runs out by itself.
func TestACardNobodyStartedAgainRunsOut(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &List{Cards: []Card{
		{ID: "old", StartedAt: began},
		{ID: "fresh", StartedAt: began.Add(ForgottenAfter - time.Minute)},
	}}

	if forgotten, _ := entry.ForgetWhatRanOut(began.Add(ForgottenAfter)); forgotten != 1 {
		t.Fatalf("%d cards were forgotten, want 1", forgotten)
	}
	if cards := entry.CardIDs(); len(cards) != 1 || cards[0] != "fresh" {
		t.Errorf("%v is left", cards)
	}
}

// The sign of life belongs to the session that took a card up: one session at
// work keeps nothing of a session that died alive, nor a card of nobody.
func TestOnlyTheSessionThatTookACardUpKeepsItAlive(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	host := &Account{}
	host.List("busy").Start(Card{ID: "of the busy session", StartedAt: began})
	host.List("gone").Start(Card{ID: "of the session that died", StartedAt: began})
	host.List(NoSession).Start(Card{ID: "of a person at a terminal", StartedAt: began})

	// The one session says it is still at it, every hour of a working day.
	for hour := 1; hour <= 9; hour++ {
		host.StillAtIt("busy", began.Add(time.Duration(hour)*time.Hour))
	}

	host.ForgetWhatRanOut("busy", began.Add(ForgottenAfter+time.Minute))
	if len(host.Sessions) != 1 {
		t.Fatalf("%d lists are left, want the one of the session that kept working", len(host.Sessions))
	}
	if cards := host.List("busy").CardIDs(); len(cards) != 1 || cards[0] != "of the busy session" {
		t.Errorf("%v is left, want the card of the session that kept working", cards)
	}
}

// A list is dropped once it has nothing left to hold, to say or to take back,
// and not a moment sooner: a list whose last card was stopped still has an
// empty set to report, until the server has forgotten what it was sent.
func TestAListGoesOnceThereIsNothingLeftToTakeBack(t *testing.T) {
	sent := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	host := &Account{}
	list := host.List("done")
	list.SentAt, list.SentCards = sent, []string{"a-card"}

	host.ForgetWhatRanOut(NoSession, sent.Add(HeldFor-time.Second))
	if _, kept := host.Sessions["done"]; !kept {
		t.Fatal("the list went while the server still counted what it was sent")
	}
	host.ForgetWhatRanOut(NoSession, sent.Add(HeldFor))
	if _, kept := host.Sessions["done"]; kept {
		t.Error("the list stayed after the server forgot what it was sent")
	}

	host.List("taken back").SentAt = sent
	if _, changed := host.ForgetWhatRanOut(NoSession, sent); !changed {
		t.Error("dropping a list that sent an empty set is not worth writing down")
	}
	if len(host.Sessions) != 0 {
		t.Errorf("%v is left", host.Sessions)
	}
}

// Saying it again costs a write, and the moment is read against a window of
// hours, so it is written down once a minute and not once a tool call.
func TestSayingItAgainIsWrittenDownOnceAMinute(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Account{}
	entry.List("mine").Start(Card{ID: "one", StartedAt: began})

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
	if entry.StillAtIt("somebody else", began.Add(time.Hour)) || entry.StillAtIt(NoSession, began.Add(time.Hour)) {
		t.Error("a card was refreshed by a session that does not hold it")
	}
}

// The mark of a beat is read against a window of many minutes, so it is written
// down once a minute like the sign of life and not once a tool call.
func TestTheMarkOfABeatIsWrittenDownOnceAMinute(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &List{Cards: []Card{{ID: "one", StartedAt: began}}}

	if !entry.Beat(began) {
		t.Fatal("the first beat was not written down")
	}
	if !entry.BeatAt.Equal(began) {
		t.Errorf("the mark stands at %s, want %s", entry.BeatAt, began)
	}
	if entry.Beat(began.Add(BeatEvery / 2)) {
		t.Error("a second beat within the minute was written down")
	}
	if !entry.Beat(began.Add(BeatEvery + time.Second)) {
		t.Error("the beat after the minute was not written down")
	}
	if !entry.BeatAt.Equal(began.Add(BeatEvery + time.Second)) {
		t.Errorf("the mark stands at %s after the second beat", entry.BeatAt)
	}
}

// The safeguard: a card that was taken up where nothing said who was asking
// belongs to nobody, so no sign of life reaches it — neither one that names a
// session nor one that names none, which is what a heartbeat outside a session
// brings. Such a card goes by the moment it was started.
func TestACardWithoutASessionIsKeptAliveByNobody(t *testing.T) {
	began := time.Date(2026, 9, 23, 9, 0, 0, 0, time.UTC)
	entry := &Account{}
	entry.List(NoSession).Start(Card{ID: "of a person at a terminal", StartedAt: began})

	for hour := 1; hour <= 9; hour++ {
		at := began.Add(time.Duration(hour) * time.Hour)
		if entry.StillAtIt("a session at work", at) {
			t.Fatalf("a session that holds no card refreshed it after %d hours", hour)
		}
		if entry.StillAtIt(NoSession, at) {
			t.Fatalf("a sign of life that names no session refreshed the card after %d hours", hour)
		}
	}

	if forgotten, _ := entry.ForgetWhatRanOut(NoSession, began.Add(ForgottenAfter+time.Minute)); forgotten != 1 {
		t.Fatalf("%d cards were forgotten, want the one nobody said anything about", forgotten)
	}
	if len(entry.Sessions) != 0 {
		t.Errorf("%v is left, want nothing", entry.Sessions)
	}
}
