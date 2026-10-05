package cli

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

const aMention = `<action-text-attachment sgid="x" content-type="application/vnd.minka.person" content="@Anke"></action-text-attachment>`

func TestThePostsToolsInUseAndTheKindsOfAProjectAreListed(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "tools", f.Project.ID)
	tools := listOfObjects(t, env.Data)
	if len(tools) != 1 || tools[0]["id"] != f.Posts.ID {
		t.Fatalf("tools %v: a tool no longer in use is listed, or the one in use is missing", tools)
	}

	env = runOK(t, e, "posts", "categories", f.Project.ID)
	kinds := listOfObjects(t, env.Data)
	if len(kinds) != 2 || kinds[0]["id"] != f.Announcement.ID || kinds[1]["id"] != f.Heartbeat.ID {
		t.Fatalf("kinds %v, want the two of the project in their order", kinds)
	}
	// Minka keeps the deprecated `emoji` beside the name, always empty, and the
	// data hands it on as it came.
	if emoji, given := kinds[0]["emoji"]; !given || emoji != "" || kinds[0]["name"] != "📢 Announcement" {
		t.Fatalf("a kind %v: want the emoji in the name and an empty `emoji` beside it", kinds[0])
	}

	for _, args := range [][]string{
		{"posts", "tools", f.OtherProject.ID},
		{"posts", "categories", f.OtherProject.ID},
		{"posts", "list", f.OtherPosts.ID},
	} {
		_, r := e.runJSON("", args...)
		expectCode(t, r, ExitNotFound)
	}
}

// An emoji is part of a kind's name, and the `emoji` Minka still sends beside
// it is empty: a kind is shown by its name alone, with nothing put in front.
func TestAKindOfPostIsShownByItsNameAlone(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	text := e.run("", "posts", "categories", f.Project.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{f.Announcement.ID + "  📢 Announcement\n", f.Heartbeat.ID + "  ❤️ Heartbeat\n"} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text lacks %q:\n%s", want, text.stdout)
		}
	}
}

func TestTheListHasThePinnedPostsFirstThenTheNewestAndNothingElse(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "list", f.Posts.ID)
	page := decode(t, env.Data)
	if ids := idsOf(t, page, "posts"); !slices.Equal(ids, []string{f.PinnedPost.ID, f.Post.ID, f.OlderPost.ID}) {
		t.Fatalf("posts %v: want the pinned one, then the newest; no draft, none of the archive or the trash", ids)
	}
	if page["next"] != nil {
		t.Fatalf("next %v on the only page", page["next"])
	}
	first := decode(t, rawAt(t, page, "posts", 1))
	if first["category"] != "📢 Announcement" || first["lead"] != "It comes on Friday." {
		t.Fatalf("a line of the list: %v", first)
	}
	if slices.ContainsFunc(env.Breadcrumbs, func(crumb Breadcrumb) bool { return crumb.Action == "next" }) {
		t.Fatalf("a next page is named on the last one: %+v", env.Breadcrumbs)
	}
	if !strings.Contains(e.lastRequestQuery(), "limit=50") {
		t.Fatalf("query %q does not send the size of the page", e.lastRequestQuery())
	}

	// The tool out of use is read at its own address as before.
	runOK(t, e, "posts", "list", f.PostsOutOfUse.ID)
}

func TestTheListComesAPageAtATimeAndSaysWhereTheNextStarts(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "list", f.Posts.ID, "--limit", "2")
	page := decode(t, env.Data)
	next, _ := page["next"].(string)
	if ids := idsOf(t, page, "posts"); !slices.Equal(ids, []string{f.PinnedPost.ID, f.Post.ID}) || next == "" {
		t.Fatalf("first page %v, next %q", ids, next)
	}
	if !strings.Contains(crumbFor(t, env.Breadcrumbs, "next"), "--before "+next) || !strings.Contains(env.Summary, "next page") {
		t.Fatalf("the page does not hand over where the next one starts: %q, %+v", env.Summary, env.Breadcrumbs)
	}

	// Pinning a post between two pages moves nothing: the next page starts
	// behind the values of the last post, not behind the post.
	runOK(t, e, "posts", "pin", f.OlderPost.ID)
	env = runOK(t, e, "posts", "list", f.Posts.ID, "--limit", "2", "--before", next)
	page = decode(t, env.Data)
	if ids := idsOf(t, page, "posts"); len(ids) != 0 || page["next"] != nil {
		t.Fatalf("second page %v, next %v: the post pinned since stands above the place", ids, page["next"])
	}

	text := e.run("", "posts", "list", f.Posts.ID, "--limit", "1", "--styled")
	expectCode(t, text, ExitOK)
	if !strings.Contains(text.stdout, "Next page: minka posts list "+f.Posts.ID+" --before ") {
		t.Fatalf("the text does not name the next page:\n%s", text.stdout)
	}
}

// Minka cuts a page above 100 and says so in next, so the size goes out as it
// is given; one that is no size and a place that is none are Minka's to refuse.
func TestTheSizeAndThePlaceOfAPageGoOutAsGiven(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	runOK(t, e, "posts", "list", f.Posts.ID, "--limit", "500")
	if !strings.Contains(e.lastRequestQuery(), "limit=500") {
		t.Fatalf("query %q", e.lastRequestQuery())
	}

	for flags, want := range map[string]string{
		"--limit=0":         apiCodePostsLimitNotANumber,
		"--before=nonsense": apiCodePostsBeforeUnknown,
	} {
		env, r := e.runJSON("", "posts", "list", f.Posts.ID, flags)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != want || env.Hint == "" {
			t.Errorf("%s: apiCode %v, hint %q", flags, env.Meta["apiCode"], env.Hint)
		}
	}

	env, r := e.runJSON("", "posts", "list", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "--limit", "0")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "posts tools") {
		t.Fatalf("hint %q, want the tool before the size and the way to the tools", env.Hint)
	}
}

func TestTheArchiveOfAToolIsListedTheOneArchivedLastFirst(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	runOK(t, e, "posts", "archive", f.OlderPost.ID)

	env := runOK(t, e, "posts", "list", f.Posts.ID, "--archived")
	archive := listOfObjects(t, env.Data)
	if len(archive) != 2 || archive[0]["id"] != f.OlderPost.ID || archive[1]["id"] != f.ArchivedPost.ID {
		t.Fatalf("archive %v, want the one archived now first", archive)
	}
	if by := archive[1]["archivedBy"].(map[string]any); by["id"] != f.Anke.ID || archive[1]["category"] != "❤️ Heartbeat" {
		t.Fatalf("a line of the archive: %v", archive[1])
	}

	for _, flag := range []string{"--limit=2", "--before=x"} {
		_, r := e.runJSON("", "posts", "list", f.Posts.ID, "--archived", flag)
		expectCode(t, r, ExitUsage)
	}
}

func TestAPostIsReadWithItsBodyItsKindAndItsComments(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "show", f.Post.ID)
	post := decode(t, env.Data)
	if post["body"] != "<div>It comes on <strong>Friday</strong>.</div>" || post["categoryId"] != f.Announcement.ID || post["editedAt"] != nil {
		t.Fatalf("post %v", post)
	}
	if comments, _ := post["comments"].([]any); len(comments) != 1 {
		t.Fatalf("comments %v", post["comments"])
	}
	if crumbFor(t, env.Breadcrumbs, "pin") == "" {
		t.Fatalf("a post that is not pinned offers no pin: %+v", env.Breadcrumbs)
	}

	text := e.run("", "posts", "show", f.Post.ID, "--styled")
	expectCode(t, text, ExitOK)
	for _, want := range []string{"It comes on **Friday**.", "Kind: 📢 Announcement", "Great"} {
		if !strings.Contains(text.stdout, want) {
			t.Fatalf("the text lacks %q:\n%s", want, text.stdout)
		}
	}

	archived := runOK(t, e, "posts", "show", f.ArchivedPost.ID)
	if decode(t, archived.Data)["archived"] != true || !strings.Contains(archived.Summary, "archive") {
		t.Fatalf("a post of the archive does not say so: %q", archived.Summary)
	}
	if crumbFor(t, archived.Breadcrumbs, "unarchive") == "" {
		t.Fatalf("a post of the archive offers no way back: %+v", archived.Breadcrumbs)
	}
}

func TestADraftAPostInTheTrashAndAForeignPostAreAtNoAddress(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, id := range []string{f.DraftPost.ID, f.TrashedPost.ID, f.OtherPost.ID} {
		env, r := e.runJSON("", "posts", "show", id)
		expectCode(t, r, ExitNotFound)
		if !strings.Contains(env.Hint, "draft") || !strings.Contains(env.Hint, "trash") {
			t.Fatalf("hint %q names neither the draft nor the trash", env.Hint)
		}
	}
}

// newPost is the command line of a post in the tool of the fixture, with
// whatever the case adds to it.
func newPost(e *testEnv, rest ...string) []string {
	return append([]string{"posts", "create", "--tool", e.fixture.Posts.ID, "--title", "Setting up", "--body", "Bring **gloves**."}, rest...)
}

func TestAPostIsSentOutWithMarkdownAsMarkupAndTellsWhomItsAudienceNames(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, newPost(e, "--audience", "everybody", "--category", f.Heartbeat.ID)...)
	body := e.lastWriteBody()
	if body["body"] != "<p>Bring <strong>gloves</strong>.</p>" || body["categoryId"] != f.Heartbeat.ID || body["audience"] != "everybody" {
		t.Fatalf("sent %v", body)
	}
	if _, named := body["notifyIds"]; named {
		t.Fatalf("an audience without people sends a list: %v", body)
	}
	post := decode(t, env.Data)
	written := postWithID(f.Posts, post["id"].(string))
	if !slices.Equal(written.SubscriberIDs, f.Project.MemberIDs) {
		t.Fatalf("told %v, want every member", written.SubscriberIDs)
	}

	runOK(t, e, newPost(e, "--audience", "chosen", "--notify", "me", "--notify", f.Anke.ID)...)
	if notify := e.lastWriteBody()["notifyIds"]; !slices.Equal(anyStrings(notify), []string{f.Bruno.ID, f.Anke.ID}) {
		t.Fatalf("notifyIds %v", notify)
	}

	html, r := e.runJSON("", newPost(e, "--audience", "nobody", "--body", "# Heading", "--body-format", "html")...)
	expectCode(t, r, ExitOK)
	if !strings.Contains(html.Notice, "characters it consists of") {
		t.Fatalf("notice %q does not say that the body is kept as characters", html.Notice)
	}
}

func TestWhatIsNoAudienceNoIDAndNoKindStopsBeforeARequest(t *testing.T) {
	f := newTestEnv(t).fixture
	for name, args := range map[string][]string{
		"no audience":              {"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words"},
		"an audience that is none": {"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "some"},
		"a kind that is no ID":     {"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "nobody", "--category", "Announcement"},
		"two kinds on an edit":     {"posts", "edit", f.Post.ID, "--category", f.Heartbeat.ID, "--no-category"},
		"nothing to change":        {"posts", "edit", f.Post.ID},
		"a kind that is no ID too": {"posts", "edit", f.Post.ID, "--category", "Heartbeat"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			_, r := e.runJSON("", args...)
			expectCode(t, r, ExitUsage)
			if len(e.server.Requests()) != 0 {
				t.Fatal("a request went out all the same")
			}
		})
	}
}

// Of a new post Minka reads the tool first, then the request, then what the
// write meets, and last the people, one at a time.
func TestTheRefusalsOfANewPostComeInMinkasOrder(t *testing.T) {
	unknown := "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	for name, one := range map[string]struct {
		args     func(f *fakeapi.Fixture) []string
		archived bool
		code     string
		exit     int
	}{
		"the tool before the title": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", unknown, "--title", "", "--audience", "nobody"}
		}, false, "", ExitNotFound},
		"the title": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", " ", "--audience", "chosen"}
		}, false, apiCodePostTitleMissing, ExitValidation},
		"a body that says nothing": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--audience", "chosen"}
		}, false, apiCodePostBodyMissing, ExitValidation},
		"chosen naming nobody": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "chosen", "--category", unknown}
		}, true, apiCodeNotifyIDsMissing, ExitValidation},
		"everybody naming somebody": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "everybody", "--notify", "me"}
		}, true, apiCodeNotifyIDsWithoutChosenAudience, ExitValidation},
		"the project before the kind": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "nobody", "--category", unknown}
		}, true, apiCodeProjectArchived, ExitValidation},
		"a kind of another project": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "nobody", "--category", f.OtherCategory.ID}
		}, false, apiCodePostCategoryUnknown, ExitValidation},
		"the project before the people": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "chosen", "--notify", f.Stranger.ID}
		}, true, apiCodeProjectArchived, ExitValidation},
		"a stranger": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "create", "--tool", f.Posts.ID, "--title", "A post", "--body", "Words", "--audience", "chosen", "--notify", f.Stranger.ID}
		}, false, apiCodePersonNotAProjectMember, ExitValidation},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.fixture.Project.Archived = one.archived
			before := len(e.fixture.Posts.Posts)
			env, r := e.runJSON("", one.args(e.fixture)...)
			expectCode(t, r, one.exit)
			if one.code != "" && env.Meta["apiCode"] != one.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], one.code)
			}
			if env.Hint == "" {
				t.Error("a refusal without a hint")
			}
			if len(e.fixture.Posts.Posts) != before {
				t.Fatal("a post was written all the same")
			}
		})
	}
}

// A post written through the API is shown to no client, so a client named is
// no mistake in the request and cannot be mended by repeating it.
func TestAClientIsNeverToldAboutANewPost(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	agent := &fakeapi.Person{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e503", Name: "Claude (Nele)", Email: "claude-nele@agents.invalid", PrincipalID: f.Client.ID}
	e.server.People[agent.ID] = agent
	f.Project.MemberIDs = append(f.Project.MemberIDs, agent.ID)
	before := len(f.Posts.Posts)

	for _, who := range []string{f.Client.ID, agent.ID} {
		env, r := e.runJSON("", newPost(e, "--audience", "chosen", "--notify", who)...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePersonCannotSeeIt {
			t.Fatalf("apiCode %v", env.Meta["apiCode"])
		}
		if !strings.Contains(env.Hint, "refused again") || !strings.Contains(env.Hint, "Name somebody else") || strings.Contains(env.Hint, "folder") {
			t.Errorf("hint %q", env.Hint)
		}
		if env.Retryable {
			t.Error("the refusal reads as one a repeat could get past")
		}
	}
	if len(f.Posts.Posts) != before {
		t.Fatal("a post was written all the same")
	}

	// Of one person, membership is answered first, and of a list, the first
	// name that is refused decides.
	for want, names := range map[string][]string{
		apiCodePersonNotAProjectMember: {f.ForeignClient.ID},
		apiCodePersonCannotSeeIt:       {f.Client.ID, f.Stranger.ID},
	} {
		args := newPost(e, "--audience", "chosen")
		for _, name := range names {
			args = append(args, "--notify", name)
		}
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != want {
			t.Errorf("apiCode %v, want %s", env.Meta["apiCode"], want)
		}
	}
	env, r := e.runJSON("", newPost(e, "--audience", "chosen", "--notify", f.Stranger.ID, "--notify", f.Client.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonNotAProjectMember {
		t.Errorf("apiCode %v for the stranger named first", env.Meta["apiCode"])
	}

	// Everybody refuses nobody, the client among them.
	runOK(t, e, newPost(e, "--audience", "everybody")...)
}

// A member named twice is told once, and the repetition goes out as given.
// Somebody outside the project is refused however often they are named.
func TestSomebodyNamedTwiceOnAPostIsToldOnce(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, newPost(e, "--audience", "chosen", "--notify", f.Anke.ID, "--notify", "me", "--notify", f.Anke.ID, "--notify", "me")...)
	if notify := anyStrings(e.lastWriteBody()["notifyIds"]); !slices.Equal(notify, []string{f.Anke.ID, f.Bruno.ID, f.Anke.ID, f.Bruno.ID}) {
		t.Fatalf("notifyIds %v", notify)
	}
	written := postWithID(f.Posts, decode(t, env.Data)["id"].(string))
	if !slices.Equal(written.SubscriberIDs, []string{f.Bruno.ID, f.Anke.ID}) {
		t.Fatalf("told %v, want the writer and Anke once each", written.SubscriberIDs)
	}

	env, r := e.runJSON("", newPost(e, "--audience", "chosen", "--notify", f.Anke.ID, "--notify", f.Anke.ID, "--notify", f.Stranger.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePersonNotAProjectMember || strings.Contains(env.Hint, "twice") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
}

func TestAnEditSendsOnlyWhatIsGivenAndTakesAKindOff(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "edit", f.Post.ID, "--title", "The tent comes on Saturday")
	if body := e.lastWriteBody(); len(body) != 1 || body["title"] != "The tent comes on Saturday" {
		t.Fatalf("sent %v", body)
	}
	if decode(t, env.Data)["editedAt"] == nil {
		t.Fatal("a post written over does not say when")
	}

	env = runOK(t, e, "posts", "edit", f.Post.ID, "--no-category")
	if body := e.lastWriteBody(); len(body) != 1 || body["categoryId"] != "" {
		t.Fatalf("sent %v", body)
	}
	if decode(t, env.Data)["categoryId"] != nil {
		t.Fatal("the kind was not taken off")
	}

	runOK(t, e, "posts", "edit", f.Post.ID, "--category", f.Heartbeat.ID)
	if f.Post.CategoryID != f.Heartbeat.ID {
		t.Fatalf("kind %q", f.Post.CategoryID)
	}
}

// Of an edit Minka reads the post first, then the request, and then asks why
// the write found nothing: the project, the post, the kind.
func TestTheRefusalsOfAnEditComeInMinkasOrder(t *testing.T) {
	unknown := "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	for name, one := range map[string]struct {
		args     func(f *fakeapi.Fixture) []string
		archived bool
		code     string
		exit     int
	}{
		"the post before the title": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", unknown, "--title", ""}
		}, false, "", ExitNotFound},
		"the post before the body": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", unknown, "--body", ""}
		}, false, "", ExitNotFound},
		"a title emptied": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", f.Post.ID, "--title", ""}
		}, true, apiCodePostTitleMissing, ExitValidation},
		"a body emptied": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", f.Post.ID, "--body", ""}
		}, true, apiCodePostBodyMissing, ExitValidation},
		"the project before the kind": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", f.Post.ID, "--category", unknown}
		}, true, apiCodeProjectArchived, ExitValidation},
		"the post before the kind": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", f.ArchivedPost.ID, "--category", unknown}
		}, false, apiCodePostArchived, ExitValidation},
		"the kind": {func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", f.Post.ID, "--category", f.OtherCategory.ID}
		}, false, apiCodePostCategoryUnknown, ExitValidation},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.fixture.Project.Archived = one.archived
			env, r := e.runJSON("", one.args(e.fixture)...)
			expectCode(t, r, one.exit)
			if one.code != "" && env.Meta["apiCode"] != one.code {
				t.Fatalf("apiCode %v, want %s", env.Meta["apiCode"], one.code)
			}
			if env.Hint == "" {
				t.Error("a refusal without a hint")
			}
		})
	}
}

func TestAPostWithFilesIsNotReplacedByOneWithoutUnlessItTakesNoChange(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Post.Body = "<div>See " + aMention + "</div>"
	f.ArchivedPost.Body = f.Post.Body

	env, r := e.runJSON("", "posts", "edit", f.Post.ID, "--body", "Nothing attached")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") || len(e.server.WritingRequests()) != 0 {
		t.Fatalf("hint %q, %d writes", env.Hint, len(e.server.WritingRequests()))
	}
	dropped := runOK(t, e, "posts", "edit", f.Post.ID, "--body", "Nothing attached", "--drop-attachments")
	if !strings.Contains(dropped.Notice, "dropped") {
		t.Fatalf("notice %q", dropped.Notice)
	}

	// A post of the archive takes no body at all, so Minka's answer comes and
	// nothing could have been lost.
	env, r = e.runJSON("", "posts", "edit", f.ArchivedPost.ID, "--body", "Nothing attached")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePostArchived {
		t.Fatalf("apiCode %v, want the archive before the guard", env.Meta["apiCode"])
	}
}

func TestAPinnedPostStandsOnTopAndPinningTwiceKeepsItsPlace(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "pin", f.OlderPost.ID)
	if decode(t, env.Data)["pinned"] != true || crumbFor(t, env.Breadcrumbs, "unpin") == "" {
		t.Fatalf("a pinned post: %s, %+v", env.Data, env.Breadcrumbs)
	}
	top := []string{f.OlderPost.ID, f.PinnedPost.ID, f.Post.ID}
	if ids := idsOf(t, decode(t, runOK(t, e, "posts", "list", f.Posts.ID).Data), "posts"); !slices.Equal(ids, top) {
		t.Fatalf("posts %v, want the one pinned last on top", ids)
	}

	runOK(t, e, "posts", "pin", f.PinnedPost.ID)
	if ids := idsOf(t, decode(t, runOK(t, e, "posts", "list", f.Posts.ID).Data), "posts"); !slices.Equal(ids, top) {
		t.Fatalf("posts %v: pinning a pinned post moved it", ids)
	}

	runOK(t, e, "posts", "unpin", f.OlderPost.ID)
	env = runOK(t, e, "posts", "unpin", f.OlderPost.ID)
	if decode(t, env.Data)["pinned"] != false {
		t.Fatal("unpinning twice pinned it")
	}

	for _, write := range []string{"pin", "unpin"} {
		env, r := e.runJSON("", "posts", write, f.ArchivedPost.ID)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePostArchived || !strings.Contains(env.Hint, "posts unarchive") {
			t.Fatalf("%s: apiCode %v, hint %q", write, env.Meta["apiCode"], env.Hint)
		}
	}

	// A pinned post of an archived project is refused as the project, not
	// answered as a pin that changed nothing.
	f.Project.Archived = true
	env, r := e.runJSON("", "posts", "pin", f.PinnedPost.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodeProjectArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

func TestAPostGoesIntoTheArchiveAndComesBackPinnedWhereItWasPinned(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "archive", f.Post.ID)
	if decode(t, env.Data)["archived"] != true {
		t.Fatal("the answer does not say archived")
	}
	if ids := idsOf(t, decode(t, runOK(t, e, "posts", "list", f.Posts.ID).Data), "posts"); slices.Contains(ids, f.Post.ID) {
		t.Fatalf("an archived post is listed: %v", ids)
	}

	for write, want := range map[string]string{"archive": apiCodePostArchived, "edit": apiCodePostArchived} {
		args := []string{"posts", write, f.Post.ID}
		if write == "edit" {
			args = append(args, "--title", "Again")
		}
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != want {
			t.Errorf("%s: apiCode %v", write, env.Meta["apiCode"])
		}
	}

	env = runOK(t, e, "posts", "unarchive", f.ArchivedPost.ID)
	if decode(t, env.Data)["pinned"] != true {
		t.Fatal("a pinned post lost its pin in the archive")
	}
	if ids := idsOf(t, decode(t, runOK(t, e, "posts", "list", f.Posts.ID).Data), "posts"); ids[0] != f.ArchivedPost.ID {
		t.Fatalf("posts %v, want the post pinned last back on top", ids)
	}

	env, r := e.runJSON("", "posts", "unarchive", f.OlderPost.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePostNotArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}
}

// The trash is not the archive: archiving a post there names the trash, even
// in an archived project, and bringing one back from the archive does not find
// it at all.
func TestAPostInTheTrashIsNamedAsSuchOnlyWhenItIsArchived(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env, r := e.runJSON("", "posts", "archive", f.TrashedPost.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePostInTrash || !strings.Contains(env.Hint, "web") {
		t.Fatalf("apiCode %v, hint %q", env.Meta["apiCode"], env.Hint)
	}
	f.Project.Archived = true
	env, r = e.runJSON("", "posts", "archive", f.TrashedPost.ID)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePostInTrash {
		t.Fatalf("apiCode %v, want the trash before the project", env.Meta["apiCode"])
	}
	f.Project.Archived = false

	_, r = e.runJSON("", "posts", "unarchive", f.TrashedPost.ID)
	expectCode(t, r, ExitNotFound)
	_, r = e.runJSON("", "posts", "archive", f.DraftPost.ID)
	expectCode(t, r, ExitNotFound)
}

func TestAPostGoesIntoTheTrashFromTheListAndFromTheArchive(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	for _, post := range []*fakeapi.Post{f.Post, f.ArchivedPost} {
		env := runOK(t, e, "posts", "delete", post.ID)
		if decode(t, env.Data)["deleted"] != true || !post.Trashed || post.Archived {
			t.Fatalf("post %s: trashed %v, archived %v", post.Title, post.Trashed, post.Archived)
		}
		_, r := e.runJSON("", "posts", "show", post.ID)
		expectCode(t, r, ExitNotFound)
		env, r = e.runJSON("", "posts", "archive", post.ID)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePostInTrash {
			t.Fatalf("apiCode %v", env.Meta["apiCode"])
		}
	}
	_, r := e.runJSON("", "posts", "delete", f.Post.ID)
	expectCode(t, r, ExitNotFound)
}

func TestACommentOnAPostIsReadWrittenChangedReactedToAndTakenBack(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	env := runOK(t, e, "posts", "comments", "create", f.Post.ID, "Who **helps**?")
	if e.lastWriteBody()["body"] != "<p>Who <strong>helps</strong>?</p>" {
		t.Fatalf("sent %v", e.lastWriteBody())
	}
	id := decode(t, env.Data)["id"].(string)

	if comments := listOfObjects(t, runOK(t, e, "posts", "comments", "list", f.Post.ID).Data); len(comments) != 2 || comments[1]["id"] != id {
		t.Fatalf("comments %v, want the new one last", comments)
	}
	runOK(t, e, "posts", "comments", "edit", f.Post.ID, id, "Who helps on Saturday?")

	react := runOK(t, e, "posts", "comments", "react", f.Post.ID, id, "👍")
	again := runOK(t, e, "posts", "comments", "react", f.Post.ID, id, "👍")
	if react.Summary == again.Summary || !strings.Contains(again.Summary, "already") {
		t.Fatalf("summaries %q and %q", react.Summary, again.Summary)
	}
	runOK(t, e, "posts", "comments", "unreact", f.Post.ID, id, "👍")
	runOK(t, e, "posts", "comments", "delete", f.Post.ID, id)
	_, r := e.runJSON("", "posts", "comments", "delete", f.Post.ID, id)
	expectCode(t, r, ExitNotFound)

	// Somebody else's comment is not there to change.
	env, r = e.runJSON("", "posts", "comments", "edit", f.Post.ID, f.Post.Comments[0].ID, "Mine now")
	expectCode(t, r, ExitNotFound)
	if !strings.Contains(env.Hint, "only the author") {
		t.Fatalf("hint %q", env.Hint)
	}
	// A comment of another post is not under this one.
	_, r = e.runJSON("", "posts", "comments", "react", f.Post.ID, f.ArchivedPost.Comments[0].ID, "👍")
	expectCode(t, r, ExitNotFound)
}

// Under a post of the archive nothing is written, and Minka says so before it
// says that a comment is somebody else's.
func TestUnderAPostOfTheArchiveEveryCommentWriteIsRefusedAsTheArchive(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	comment := f.ArchivedPost.Comments[0]
	comment.AuthorID = f.Anke.ID

	for name, args := range map[string][]string{
		"create":  {"posts", "comments", "create", f.ArchivedPost.ID, "More"},
		"edit":    {"posts", "comments", "edit", f.ArchivedPost.ID, comment.ID, "Mine now"},
		"delete":  {"posts", "comments", "delete", f.ArchivedPost.ID, comment.ID},
		"react":   {"posts", "comments", "react", f.ArchivedPost.ID, comment.ID, "👍"},
		"unreact": {"posts", "comments", "unreact", f.ArchivedPost.ID, comment.ID, "👍"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodePostArchived {
			t.Errorf("%s: apiCode %v", name, env.Meta["apiCode"])
		}
	}
	if comments := listOfObjects(t, runOK(t, e, "posts", "comments", "list", f.ArchivedPost.ID).Data); len(comments) != 1 {
		t.Fatalf("comments %v", comments)
	}
}

// The guard over files keeps a comment of one's own from losing them. A
// comment of somebody else and one under a post of the archive take no text
// at all, so Minka's answer comes and not the guard.
func TestACommentWithFilesIsGuardedOnlyWhereItCouldBeChanged(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	mine := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e511", AuthorID: f.Bruno.ID, Body: "<div>" + aMention + "</div>", CreatedAt: time.Now()}
	f.Post.Comments = append(f.Post.Comments, mine)
	f.Post.Comments[0].Body = "<div>" + aMention + "</div>"
	f.ArchivedPost.Comments[0].Body = "<div>" + aMention + "</div>"

	env, r := e.runJSON("", "posts", "comments", "edit", f.Post.ID, mine.ID, "Plain")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--drop-attachments") {
		t.Fatalf("hint %q", env.Hint)
	}

	_, r = e.runJSON("", "posts", "comments", "edit", f.Post.ID, f.Post.Comments[0].ID, "Plain")
	expectCode(t, r, ExitNotFound)

	env, r = e.runJSON("", "posts", "comments", "edit", f.ArchivedPost.ID, f.ArchivedPost.Comments[0].ID, "Plain")
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePostArchived {
		t.Fatalf("apiCode %v", env.Meta["apiCode"])
	}

	runOK(t, e, "posts", "comments", "edit", f.Post.ID, mine.ID, "Plain", "--drop-attachments")
}

func TestTheCatalogListsThePostsCommands(t *testing.T) {
	e := newTestEnv(t)
	env := runOK(t, e, "commands")
	for _, want := range []string{"minka posts list", "minka posts create", "minka posts archive", "minka posts comments react"} {
		if !strings.Contains(string(env.Data), `"`+want+`"`) {
			t.Fatalf("the catalog lacks %q", want)
		}
	}
}

func postWithID(tool *fakeapi.PostsTool, id string) *fakeapi.Post {
	for _, post := range tool.Posts {
		if post.ID == id {
			return post
		}
	}
	return nil
}

func anyStrings(value any) []string {
	list, _ := value.([]any)
	var texts []string
	for _, entry := range list {
		text, _ := entry.(string)
		texts = append(texts, text)
	}
	return texts
}

// lastRequestQuery is the query string of the last request, unescaped.
func (e *testEnv) lastRequestQuery() string {
	requests := e.server.Requests()
	query, _ := url.QueryUnescape(requests[len(requests)-1].Query)
	return query
}

// Markdown that converts to nothing would send a body that says nothing, which
// is this side's conversion losing the text and not a refusal of Minka.
func TestABodyTheConversionLosesWhollyStopsBeforeARequest(t *testing.T) {
	e := newTestEnv(t)
	for _, args := range [][]string{
		newPost(e, "--audience", "nobody", "--body", "[a]: http://example.com"),
		{"posts", "edit", e.fixture.Post.ID, "--body", "[a]: http://example.com"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "nothing Minka keeps") || len(e.server.Requests()) != 0 {
			t.Fatalf("%v: error %q, %d requests", args[1], env.Error, len(e.server.Requests()))
		}
	}
}

// Only the place a page ends at is stable: the last post of a page pinned
// between two pages is not handed out again, because the next page starts
// behind the values it had and not behind the post.
func TestTheNextPageStartsBehindThePlaceAndNotBehindThePost(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture

	page := decode(t, runOK(t, e, "posts", "list", f.Posts.ID, "--limit", "2").Data)
	next := page["next"].(string)
	runOK(t, e, "posts", "pin", f.Post.ID)
	second := decode(t, runOK(t, e, "posts", "list", f.Posts.ID, "--limit", "2", "--before", next).Data)
	if ids := idsOf(t, second, "posts"); !slices.Equal(ids, []string{f.OlderPost.ID}) {
		t.Fatalf("second page %v, want the older post alone", ids)
	}
}

func TestANewPostInAToolThatIsNotThereNamesTheTools(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "posts", "create", "--tool", "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "--title", "A post", "--body", "Words", "--audience", "nobody")
	expectCode(t, r, ExitNotFound)
	if env.Hint != postsToolNotFoundHint {
		t.Fatalf("hint %q, want the one of a tool", env.Hint)
	}
}

// Where the project is archived Minka refuses whatever is sent, so the guard
// over files stands aside and projectArchived is the answer.
func TestInAnArchivedProjectTheGuardOverFilesStandsAside(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Post.Body = "<div>See " + aMention + "</div>"
	mine := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e512", AuthorID: f.Bruno.ID, Body: "<div>" + aMention + "</div>", CreatedAt: time.Now()}
	f.Post.Comments = append(f.Post.Comments, mine)
	f.Project.Archived = true

	for _, args := range [][]string{
		{"posts", "edit", f.Post.ID, "--body", "Plain"},
		{"posts", "comments", "edit", f.Post.ID, mine.ID, "Plain"},
	} {
		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitValidation)
		if env.Meta["apiCode"] != apiCodeProjectArchived {
			t.Fatalf("%v: apiCode %v, want the project before the guard", args[1:3], env.Meta["apiCode"])
		}
	}
}

// An archived post in an archived project is refused as the project.
func TestTheProjectComesBeforeThePostOfTheArchive(t *testing.T) {
	for name, args := range map[string]func(f *fakeapi.Fixture) []string{
		"edit": func(f *fakeapi.Fixture) []string {
			return []string{"posts", "edit", f.ArchivedPost.ID, "--title", "Again"}
		},
		"pin":     func(f *fakeapi.Fixture) []string { return []string{"posts", "pin", f.ArchivedPost.ID} },
		"unpin":   func(f *fakeapi.Fixture) []string { return []string{"posts", "unpin", f.ArchivedPost.ID} },
		"archive": func(f *fakeapi.Fixture) []string { return []string{"posts", "archive", f.ArchivedPost.ID} },
		"comment": func(f *fakeapi.Fixture) []string {
			return []string{"posts", "comments", "create", f.ArchivedPost.ID, "More"}
		},
		"comment edit": func(f *fakeapi.Fixture) []string {
			return []string{"posts", "comments", "edit", f.ArchivedPost.ID, f.ArchivedPost.Comments[0].ID, "Other"}
		},
		"comment delete": func(f *fakeapi.Fixture) []string {
			return []string{"posts", "comments", "delete", f.ArchivedPost.ID, f.ArchivedPost.Comments[0].ID}
		},
		"react": func(f *fakeapi.Fixture) []string {
			return []string{"posts", "comments", "react", f.ArchivedPost.ID, f.ArchivedPost.Comments[0].ID, "👍"}
		},
		"unreact": func(f *fakeapi.Fixture) []string {
			return []string{"posts", "comments", "unreact", f.ArchivedPost.ID, f.ArchivedPost.Comments[0].ID, "👍"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := newTestEnv(t)
			e.fixture.Project.Archived = true
			env, r := e.runJSON("", args(e.fixture)...)
			expectCode(t, r, ExitValidation)
			if env.Meta["apiCode"] != apiCodeProjectArchived {
				t.Fatalf("apiCode %v, want the project before the post", env.Meta["apiCode"])
			}
		})
	}
}

// Of a new post, a kind the project does not offer is answered before any
// person named is looked at.
func TestTheKindOfANewPostComesBeforeThePeople(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	env, r := e.runJSON("", newPost(e, "--audience", "chosen", "--notify", f.Stranger.ID, "--category", f.OtherCategory.ID)...)
	expectCode(t, r, ExitValidation)
	if env.Meta["apiCode"] != apiCodePostCategoryUnknown {
		t.Fatalf("apiCode %v, want the kind before the stranger", env.Meta["apiCode"])
	}
}

// Minka cuts a page above 100 without a word, and next says that there is more.
func TestAPageAbove100IsCutAndSaysThereIsMore(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	for i := range 105 {
		moment := time.Date(2026, 8, 1, 0, i, 0, 0, time.UTC)
		f.Posts.Posts = append(f.Posts.Posts, &fakeapi.Post{
			ID: fmt.Sprintf("0192f3a4-5b6c-4d7e-8f90-%012d", i), Title: "Filler", Body: "<div>More</div>",
			CreatedAt: moment, UpdatedAt: moment, CreatorID: f.Bruno.ID,
		})
	}
	page := decode(t, runOK(t, e, "posts", "list", f.Posts.ID, "--limit", "500").Data)
	if ids := idsOf(t, page, "posts"); len(ids) != 100 || page["next"] == nil {
		t.Fatalf("%d posts, next %v: want 100 and a next page", len(ids), page["next"])
	}
}

// Minka writes a comment and its moment again whatever the text, so the same
// text again says that it was edited.
func TestTheSameCommentAgainIsAnEdit(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	id := decode(t, runOK(t, e, "posts", "comments", "create", f.Post.ID, "Same").Data)["id"].(string)
	env := runOK(t, e, "posts", "comments", "edit", f.Post.ID, id, "Same")
	if decode(t, env.Data)["editedAt"] == nil {
		t.Fatal("the same text again leaves no moment of an edit")
	}
}

func TestAReactionOnACommentThatIsNotThereNamesNoAuthor(t *testing.T) {
	e := newTestEnv(t)
	env, r := e.runJSON("", "posts", "comments", "react", e.fixture.Post.ID, "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6", "👍")
	expectCode(t, r, ExitNotFound)
	if strings.Contains(env.Hint, "somebody else") || !strings.Contains(env.Hint, "posts comments list") {
		t.Fatalf("hint %q", env.Hint)
	}
}

// The archived projects are read only where the guard over files is about to
// refuse, so a guarded edit that goes through costs no read of them.
func TestAGuardedEditThatGoesThroughReadsNoProjects(t *testing.T) {
	e := newTestEnv(t)
	f := e.fixture
	f.Post.Body = "<div>See " + aMention + "</div>"
	mine := &fakeapi.Comment{ID: "0192f3a4-5b6c-4d7e-8f90-a1b2c3d4e513", AuthorID: f.Bruno.ID, Body: "<div>" + aMention + "</div>", CreatedAt: time.Now()}
	f.Post.Comments = append(f.Post.Comments, mine)
	kept := "<div>Still " + aMention + "</div>"

	runOK(t, e, "posts", "edit", f.Post.ID, "--body", kept, "--body-format", "html")
	runOK(t, e, "posts", "comments", "edit", f.Post.ID, mine.ID, kept, "--format", "html")
	for _, request := range e.server.Requests() {
		if request.Path == "/api/v1/projects" {
			t.Fatalf("the projects were read although the guard let the text through: %+v", request)
		}
	}
}
