package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
	"github.com/flownative/minka-cli/internal/localfile"
)

// commentBySomebodyElse puts a comment on the card the way another person
// would, after everything on it so far.
func commentBySomebodyElse(e *testEnv, card *fakeapi.Card, id, body string) *fakeapi.Comment {
	e.t.Helper()
	at := card.CreatedAt
	for _, comment := range card.Comments {
		if comment.CreatedAt.After(at) {
			at = comment.CreatedAt
		}
	}
	comment := &fakeapi.Comment{ID: id, AuthorID: e.fixture.Anke.ID, Body: "<p>" + body + "</p>", CreatedAt: at.Add(time.Minute)}
	card.Comments = append(card.Comments, comment)
	return comment
}

// commentIDsIn reads the IDs out of the comments a command handed over.
func commentIDsIn(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var comments []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &comments); err != nil {
		t.Fatalf("data is not a list of comments: %v: %s", err, raw)
	}
	ids := []string{}
	for _, comment := range comments {
		ids = append(ids, comment.ID)
	}
	return ids
}

func expectComments(t *testing.T, raw json.RawMessage, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	if got := commentIDsIn(t, raw); !slices.Equal(got, want) {
		t.Errorf("comments %v, want %v", got, want)
	}
}

const (
	answerID      = "0199e7b0-0000-4000-8000-0000000000a1"
	laterAnswerID = "0199e7b0-0000-4000-8000-0000000000a2"
)

// The safeguard: a comment guarded by the newest comment its writer read is
// not written where somebody answered since. What came in after it is shown
// instead, with an exit code of its own.
func TestACommentGuardedByAnOlderCommentIsNotWritten(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	read := card.Comments[0].ID
	commentBySomebodyElse(e, card, answerID, "Already answered")
	commentBySomebodyElse(e, card, laterAnswerID, "And again")

	env, r := e.runJSON("", "comments", "create", card.ID, "Question open", "--if-latest", read)
	expectCode(t, r, ExitUnread)
	if env.Code != CodeUnread || env.OK {
		t.Errorf("code %q, ok %v, want %q", env.Code, env.OK, CodeUnread)
	}
	expectComments(t, env.Data, answerID, laterAnswerID)
	if writes := e.server.WritingRequests(); len(writes) != 0 {
		t.Errorf("%d writing requests went out, want none", len(writes))
	}
	if !strings.Contains(env.Hint, "--if-latest "+laterAnswerID) {
		t.Errorf("the hint %q does not name the newest comment", env.Hint)
	}

	// The text shows who wrote what, so that somebody at a terminal reads it.
	r = e.run("", "comments", "create", card.ID, "Question open", "--if-latest", read, "--styled")
	expectCode(t, r, ExitUnread)
	if !strings.Contains(r.stdout, "Already answered") || !strings.Contains(r.stdout, e.fixture.Anke.Name) {
		t.Errorf("the text does not show the newer comments:\n%s", r.stdout)
	}
}

func TestACommentGuardedByTheNewestCommentIsWritten(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	commentBySomebodyElse(e, card, answerID, "Already answered")

	env, r := e.runJSON("", "comments", "create", card.ID, "Thanks", "--if-latest", answerID)
	expectCode(t, r, ExitOK)
	if len(card.Comments) != 3 {
		t.Fatalf("the card has %d comments, want 3", len(card.Comments))
	}
	if env.Notice != "" {
		t.Errorf("notice %q, want none", env.Notice)
	}
}

// none is the guard of a card without comments: it holds only while there are
// none, and a card that has some shows them all.
func TestIfLatestNoneHoldsOnlyForACardWithoutComments(t *testing.T) {
	e := newTestEnv(t)
	empty := e.fixture.CardInDoing
	_, r := e.runJSON("", "comments", "create", empty.ID, "First", "--if-latest", "none")
	expectCode(t, r, ExitOK)

	card := e.fixture.CardInTriage
	env, r := e.runJSON("", "comments", "create", card.ID, "First", "--if-latest", "none")
	expectCode(t, r, ExitUnread)
	expectComments(t, env.Data, card.Comments[0].ID)
}

// A comment named as the newest that the card no longer has leaves nothing to
// tell older from newer, so every comment is shown and nothing is written.
func TestACommentNamedThatIsGoneShowsEveryComment(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	commentBySomebodyElse(e, card, answerID, "Answer")
	first := card.Comments[0].ID

	env, r := e.runJSON("", "comments", "create", card.ID, "Reply", "--if-latest", laterAnswerID)
	expectCode(t, r, ExitUnread)
	expectComments(t, env.Data, first, answerID)
	if len(e.server.WritingRequests()) != 0 {
		t.Error("a comment was written")
	}
}

// A card that lost the comment named and has none left still refuses, and
// says that the comment is gone rather than that something came after it.
func TestACommentNamedThatIsGoneFromACardWithoutCommentsIsSaidToBeGone(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	gone := card.Comments[0].ID
	card.Comments = nil

	env, r := e.runJSON("", "comments", "create", card.ID, "Reply", "--if-latest", gone)
	expectCode(t, r, ExitUnread)
	expectComments(t, env.Data)
	if !strings.Contains(env.Error, gone+" is gone") {
		t.Errorf("error %q does not say that the comment is gone", env.Error)
	}
	if !strings.Contains(env.Hint, "--if-latest none") {
		t.Errorf("hint %q does not name the guard of a card without comments", env.Hint)
	}
	if len(e.server.WritingRequests()) != 0 {
		t.Error("a comment was written")
	}
}

func TestIfLatestTakesACommentIDOrNoneAndNotForce(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	for _, args := range [][]string{
		{"--if-latest", "newest"},
		{"--if-latest", ""},
		{"--if-latest", card.Comments[0].ID, "--force"},
	} {
		_, r := e.runJSON("", append([]string{"comments", "create", card.ID, "Reply"}, args...)...)
		expectCode(t, r, ExitUsage)
	}
	if len(e.server.Requests()) != 0 {
		t.Error("a request went out for a command line that was refused")
	}
}

// The safeguard: comments unread shows what this machine has not shown, and
// every command that shows the comments of a card in full counts as having
// shown them.
func TestUnreadShowsOnlyWhatWasNotShownHere(t *testing.T) {
	for _, reading := range [][]string{{"cards", "show"}, {"comments", "list"}, {"comments", "unread"}} {
		t.Run(strings.Join(reading, " "), func(t *testing.T) {
			e := newTestEnv(t)
			card := e.fixture.CardInTriage
			first := card.Comments[0].ID

			env, r := e.runJSON("", "comments", "unread", card.ID)
			expectCode(t, r, ExitOK)
			expectComments(t, env.Data, first)

			commentBySomebodyElse(e, card, answerID, "New")
			_, r = e.runJSON("", append(reading, card.ID)...)
			expectCode(t, r, ExitOK)

			env, r = e.runJSON("", "comments", "unread", card.ID)
			expectCode(t, r, ExitOK)
			expectComments(t, env.Data)
		})
	}
}

// A comment that comes in with an earlier moment than one already read, as an
// import writes them, is still unread: the mark is the set of comments shown,
// not the newest of them.
func TestACommentThatComesInBeforeOneReadIsStillUnread(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	_, r := e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)

	card.Comments = append(card.Comments, &fakeapi.Comment{ID: answerID, AuthorID: e.fixture.Anke.ID, Body: "Imported", CreatedAt: card.Comments[0].CreatedAt.Add(-time.Hour)})
	env, r := e.runJSON("", "comments", "unread", card.ID)
	expectCode(t, r, ExitOK)
	expectComments(t, env.Data, answerID)
}

// A changed comment keeps its ID, and a comment written here was read by its
// writer: neither turns up as unread.
func TestNeitherAChangedNorAnOwnCommentIsUnread(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	_, r := e.runJSON("", "comments", "list", card.ID)
	expectCode(t, r, ExitOK)

	edited := card.Comments[0].CreatedAt.Add(time.Hour)
	card.Comments[0].EditedAt = &edited
	card.Comments[0].Body = "<p>Changed</p>"
	_, r = e.runJSON("", "comments", "create", card.ID, "Mine")
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", "comments", "unread", card.ID)
	expectCode(t, r, ExitOK)
	expectComments(t, env.Data)
}

// What one token read says nothing about another: two people at one machine
// read for themselves.
func TestTheMarksBelongToTheTokenThatRead(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	_, r := e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)

	e.vars["MINKA_TOKEN"] = e.fixture.AgentToken
	env, r := e.runJSON("", "comments", "unread", card.ID)
	expectCode(t, r, ExitOK)
	expectComments(t, env.Data, card.Comments[0].ID)
}

// The safeguard: a mark that cannot be written fails no read. The comments
// were shown all the same, and the notice says what it costs.
func TestAMarkThatCannotBeWrittenFailsNoRead(t *testing.T) {
	e := newTestEnv(t)
	marks := filepath.Join(filepath.Dir(e.vars["MINKA_CONFIG"]), "comments-read.json")
	if err := os.MkdirAll(marks, 0o700); err != nil {
		t.Fatal(err)
	}
	card := e.fixture.CardInTriage
	for _, reading := range [][]string{{"cards", "show"}, {"comments", "list"}, {"comments", "unread"}} {
		env, r := e.runJSON("", append(reading, card.ID)...)
		expectCode(t, r, ExitOK)
		if !strings.Contains(env.Notice, "count as unread") {
			t.Errorf("%s: notice %q does not say that the comments count as unread", reading, env.Notice)
		}
	}
	_, r := e.runJSON("", "comments", "create", card.ID, "Still written")
	expectCode(t, r, ExitOK)
}

// The safeguard: a lock somebody else does not give up holds a read up for a
// bounded wait at most, and then the read ends as usual with the notice.
func TestAHeldLockOnTheMarksFailsNoRead(t *testing.T) {
	e := newTestEnv(t)
	dir := filepath.Dir(e.vars["MINKA_CONFIG"])
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	held, err := localfile.Acquire(t.Context(), filepath.Join(dir, "comments-read.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	r := e.runWith(func(a *app) { a.markWait = 0 }, "", "cards", "show", e.fixture.CardInTriage.ID, "--json")
	expectCode(t, r, ExitOK)
	var env envelope
	if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.Notice, "count as unread") {
		t.Errorf("notice %q does not say that the comments count as unread", env.Notice)
	}
}

// The safeguard: a token from the environment needs no configuration file, so
// one that cannot be read leaves the setting off, says so, and stops nothing.
func TestAnUnreadableConfigLeavesRequireReadOffForAnEnvironmentToken(t *testing.T) {
	for name, content := range map[string]struct {
		text string
		mode os.FileMode
	}{
		"insecure":   {`{"comments":{"requireRead":true}}`, 0o644},
		"unreadable": {`{not json`, 0o600},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			path := e.vars["MINKA_CONFIG"]
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content.text), content.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, content.mode); err != nil {
				t.Fatal(err)
			}

			env, r := e.runJSON("", "comments", "create", e.fixture.CardInTriage.ID, "Plain")
			expectCode(t, r, ExitOK)
			if !strings.Contains(env.Notice, "comments.requireRead is taken as off") {
				t.Errorf("notice %q does not say that the setting is taken as off", env.Notice)
			}
		})
	}
}

func switchOnRequireRead(e *testEnv) {
	e.t.Helper()
	_, r := e.runJSON("", "config", "set", "comments.requireRead", "true")
	expectCode(e.t, r, ExitOK)
}

// The safeguard: with comments.requireRead on, a comment waits until the
// comments of its card were shown here, and the refusal shows exactly those.
func TestRequireReadRefusesWhileCommentsAreUnread(t *testing.T) {
	e := newTestEnv(t)
	switchOnRequireRead(e)
	card := e.fixture.CardInTriage
	_, r := e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)
	commentBySomebodyElse(e, card, answerID, "Answered")

	env, r := e.runJSON("", "comments", "create", card.ID, "Question open")
	expectCode(t, r, ExitUnread)
	expectComments(t, env.Data, answerID)
	if len(e.server.WritingRequests()) != 0 {
		t.Fatal("a comment was written")
	}

	// The refusal showed them, so they are read now.
	_, r = e.runJSON("", "comments", "create", card.ID, "Answer read")
	expectCode(t, r, ExitOK)
}

func TestRequireReadGivesWayToForceAndToIfLatest(t *testing.T) {
	e := newTestEnv(t)
	switchOnRequireRead(e)
	card := e.fixture.CardInTriage

	_, r := e.runJSON("", "comments", "create", card.ID, "Anyway", "--force")
	expectCode(t, r, ExitOK)
	commentBySomebodyElse(e, card, answerID, "Read elsewhere")
	_, r = e.runJSON("", "comments", "create", card.ID, "Seen elsewhere", "--if-latest", answerID)
	expectCode(t, r, ExitOK)
}

// Off, as it is by default, nothing is read before a comment is written.
func TestWithoutRequireReadACommentReadsNothingFirst(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	commentBySomebodyElse(e, card, answerID, "Unread")

	_, r := e.runJSON("", "comments", "create", card.ID, "Plain")
	expectCode(t, r, ExitOK)
	for _, request := range e.server.Requests() {
		if request.Method == "GET" {
			t.Errorf("read %s before writing", request.Path)
		}
	}
}

func TestConfigSetsAndShowsRequireRead(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "config", "get", "comments.requireRead")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["value"] != false {
		t.Errorf("default %s, want false", env.Data)
	}

	switchOnRequireRead(e)
	env, r = e.runJSON("", "config", "get", "comments.requireRead")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["value"] != true {
		t.Errorf("value %s, want true", env.Data)
	}
	content, err := os.ReadFile(e.vars["MINKA_CONFIG"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"requireRead": true`) {
		t.Errorf("the configuration does not keep it:\n%s", content)
	}

	for _, args := range [][]string{
		{"config", "set", "comments.requireRead", "yes"},
		{"config", "set", "comments.requireread", "true"},
		{"config", "get", "nothing"},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
	}
}

// What is read shows how to answer it: the comment command of the
// breadcrumbs is guarded by the newest comment.
func TestAReadOffersTheGuardedComment(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	env, r := e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)
	for _, crumb := range env.Breadcrumbs {
		if crumb.Action == "comment" && strings.HasSuffix(crumb.Cmd, "--if-latest "+card.Comments[0].ID) {
			return
		}
	}
	t.Errorf("no breadcrumb comments guarded by the newest comment: %v", env.Breadcrumbs)
}

// The safeguard: a guarded comment that went through marks the comment it
// named and its own, nothing else. A comment that came in with an earlier
// moment after the card was read was never shown, and stays unread.
func TestAGuardedCommentMarksOnlyWhatItNamed(t *testing.T) {
	e := newTestEnv(t)
	card := e.fixture.CardInTriage
	newest := card.Comments[0].ID
	_, r := e.runJSON("", "cards", "show", card.ID)
	expectCode(t, r, ExitOK)

	card.Comments = append(card.Comments, &fakeapi.Comment{ID: answerID, AuthorID: e.fixture.Anke.ID, Body: "Imported", CreatedAt: card.Comments[0].CreatedAt.Add(-time.Hour)})
	_, r = e.runJSON("", "comments", "create", card.ID, "Reply", "--if-latest", newest)
	expectCode(t, r, ExitOK)

	env, r := e.runJSON("", "comments", "unread", card.ID)
	expectCode(t, r, ExitOK)
	expectComments(t, env.Data, answerID)
}
