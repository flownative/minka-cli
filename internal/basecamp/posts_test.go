package basecamp_test

import (
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// The message boards and the chats of the fake are in its fourth project: a
// board that holds every case, one that is switched off, one with a category
// of its own, a chat with every kind of line, one without any, a small one.
const talkProject = "7004"

func news(a *fakebasecamp.Account) *fakebasecamp.Tool    { return &a.Projects[3].Dock[0] }
func pitches(a *fakebasecamp.Account) *fakebasecamp.Tool { return &a.Projects[3].Dock[2] }

// busyPost is the post that has a category, a mention, a file, comments and
// boosts.
func busyPost(a *fakebasecamp.Account) *fakebasecamp.Post { return &news(a).Posts[0] }

func stranger() *fakebasecamp.Person {
	return &fakebasecamp.Person{ID: 555, Name: "A Stranger", Email: "stranger@example.test", CreatedAt: "2021-01-01T08:00:00.000Z", UpdatedAt: "2021-01-01T08:00:00.000Z"}
}

func TestTheKindsOfPostOfAProjectComeOnceInTheOrderOfTheirNames(t *testing.T) {
	account := fakebasecamp.New()
	dir, _ := extract(t, account, talkProject)

	// The last but one is of the second board, which no list holds: it is as old
	// as the oldest post that carries it. No case of a letter decides the order.
	want := []map[string]any{
		{"sourceId": "9601", "createdAt": "2023-04-01T08:00:00.000Z", "updatedAt": "2023-04-01T08:00:00.000Z", "createdBy": nil,
			"projectSourceId": "7004", "emoji": "📢", "name": "Announcement", "position": float64(1)},
		{"sourceId": "9602", "createdAt": "2023-04-01T08:00:00.000Z", "updatedAt": "2023-04-02T08:00:00.000Z", "createdBy": nil,
			"projectSourceId": "7004", "emoji": "❓", "name": "faq", "position": float64(2)},
		{"sourceId": "9603", "createdAt": "2023-04-01T08:00:00.000Z", "updatedAt": "2023-04-01T08:00:00.000Z", "createdBy": nil,
			"projectSourceId": "7004", "emoji": "💓", "name": "Heartbeat", "position": float64(3)},
		{"sourceId": "9650", "createdAt": "2023-04-11T09:00:00.000Z", "updatedAt": "2023-04-11T09:00:00.000Z", "createdBy": nil,
			"projectSourceId": "7004", "emoji": "💡", "name": "Pitch", "position": float64(4)},
	}
	if got := records(t, dir, bundle.Categories); !reflect.DeepEqual(got, want) {
		t.Errorf("categories:\n got %v\nwant %v", got, want)
	}
}

func TestKindsOfPostThatAreCalledTheSameComeInTheOrderOfTheirNumbers(t *testing.T) {
	account := fakebasecamp.New()
	categories := account.Projects[3].Categories
	categories[0].Name, categories[1].Name, categories[1].ID = "FAQ", "faq", 10000
	dir, _ := extract(t, account, talkProject)

	if got, want := sourceIDs(records(t, dir, bundle.Categories)), []string{"9601", "9602", "10000", "9650"}; !slices.Equal(got, want) {
		t.Errorf("categories %v, want %v", got, want)
	}
}

func TestAKindOfPostOnlyAPostThatIsWrittenShowsComesOnceAndNoOther(t *testing.T) {
	account := fakebasecamp.New()
	// Of a board that is not extracted, and so of no post that is written.
	account.Projects[3].Dock[1].Posts[0].Category = &fakebasecamp.Category{ID: 9660, Name: "Attic finds", Icon: "🕸"}
	// A post may show a kind the list holds as well: the list says more of it.
	busyPost(account).Category = &fakebasecamp.Category{ID: 9603, Name: "Heartbeat as a post shows it", Icon: "💓"}
	dir, _ := extract(t, account, talkProject)

	categories := records(t, dir, bundle.Categories)
	if got, want := sourceIDs(categories), []string{"9601", "9602", "9603", "9650"}; !slices.Equal(got, want) {
		t.Errorf("categories %v, want %v", got, want)
	}
	if name := recordWithID(t, categories, "9603")["name"]; name != "Heartbeat" {
		t.Errorf("the kind a list holds is called %q, want what the list says", name)
	}
	for _, post := range records(t, dir, bundle.Posts) {
		if id, named := post["categorySourceId"].(string); named && !slices.Contains(sourceIDs(categories), id) {
			t.Errorf("post %v names the kind %s, which is not in the bundle", post["sourceId"], id)
		}
	}
}

// The list of categories has no pages and answers every page with all of it,
// which a walk through pages takes for a list that never ends.
func TestTheKindsOfPostAreReadOnceAndOnlyWhereAMessageBoardIsExtracted(t *testing.T) {
	account := fakebasecamp.New()
	extract(t, account, talkProject)
	if got, want := asked(account, "categories.json"), []string{"buckets/7004/categories.json"}; !slices.Equal(got, want) {
		t.Errorf("the categories were asked for with %v, want %v", got, want)
	}

	account = fakebasecamp.New()
	dir, _ := extract(t, account, boardProject)
	if got := asked(account, "categories.json"); len(got) != 0 {
		t.Errorf("a project without a message board was asked for %v", got)
	}
	if got := recordsOrNone(t, dir, bundle.Categories); got != nil {
		t.Errorf("a project without a message board has the kinds %v", got)
	}

	// A board without a message still offers the kinds of its project.
	account = fakebasecamp.New()
	for i := range account.Projects[3].Dock {
		account.Projects[3].Dock[i].Posts = nil
	}
	dir, _ = extract(t, account, talkProject)
	if got, want := sourceIDs(records(t, dir, bundle.Categories)), []string{"9601", "9602", "9603"}; !slices.Equal(got, want) {
		t.Errorf("categories %v, want %v", got, want)
	}
}

func TestAPostComesWithItsBoardItsKindItsSubjectAndItsText(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), talkProject)
	posts := records(t, dir, bundle.Posts)

	// The title is the subject: what Basecamp calls the title begins with the
	// emoji of the category.
	want := map[string]any{
		"sourceId": "9701", "createdAt": "2023-04-10T08:00:00.000Z", "updatedAt": "2023-04-10T12:00:00.000Z", "createdBy": "101",
		"projectSourceId": "7004", "toolSourceId": "9030", "categorySourceId": "9601", "title": "Office move",
		"body": `<div>We move in May, <import-mention ref="102">Grace Sample</import-mention> packs.</div><h1>Plan</h1>` +
			`<table><tr><td>Boxes &amp; tape</td></tr></table><import-attachment ref="attachment-5011"></import-attachment>`,
		"pinnedAt": nil, "status": "active",
	}
	if got := recordWithID(t, posts, "9701"); !reflect.DeepEqual(got, want) {
		t.Errorf("post:\n got %v\nwant %v", got, want)
	}

	// Basecamp leaves the category of a message without one out altogether.
	want = map[string]any{
		"sourceId": "9702", "createdAt": "2023-04-11T08:00:00.000Z", "updatedAt": "2023-04-11T08:00:00.000Z", "createdBy": "102",
		"projectSourceId": "7004", "toolSourceId": "9030", "categorySourceId": nil, "title": "Reminder without words",
		"body": "", "pinnedAt": nil, "status": "active",
	}
	if got := recordWithID(t, posts, "9702"); !reflect.DeepEqual(got, want) {
		t.Errorf("post:\n got %v\nwant %v", got, want)
	}
}

func TestAPostWithoutATextIsWrittenAsItIsAndCounted(t *testing.T) {
	account := fakebasecamp.New()
	pitches(account).Posts[0].Content = " \n"
	dir, result := extract(t, account, talkProject)

	if result.PostsWithEmptyBody != 2 {
		t.Errorf("%d posts without a text, want 2", result.PostsWithEmptyBody)
	}
	if body := recordWithID(t, records(t, dir, bundle.Posts), "9711")["body"]; body != " \n" {
		t.Errorf("the body %q was changed", body)
	}
}

func TestThePostsOfABoardAreWrittenOldestFirstAndBoardsInTheOrderOfTheTools(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), talkProject)
	if got, want := sourceIDs(records(t, dir, bundle.Posts)), []string{"9703", "9701", "9702", "9704", "9712", "9711"}; !slices.Equal(got, want) {
		t.Errorf("posts %v, want %v", got, want)
	}

	// The moment counts, not the letters of a time, and the number of an ID
	// where two were made in the same moment, not its letters.
	account := fakebasecamp.New()
	posts := pitches(account).Posts
	posts[0].CreatedAt = "2023-04-11T10:30:00.000+02:00"
	posts = append(posts, fakebasecamp.Post{ID: 10000, Subject: "As old as the shelf", Creator: posts[1].Creator, CreatedAt: posts[1].CreatedAt, UpdatedAt: posts[1].UpdatedAt})
	slices.Reverse(posts)
	pitches(account).Posts = posts
	dir, _ = extract(t, account, talkProject)
	if got, want := sourceIDs(records(t, dir, bundle.Posts))[4:], []string{"9711", "9712", "10000"}; !slices.Equal(got, want) {
		t.Errorf("posts %v, want %v", got, want)
	}
	// The kind only these posts show is as old as the oldest of them.
	if at := recordWithID(t, records(t, dir, bundle.Categories), "9650")["createdAt"]; at != "2023-04-11T10:30:00.000+02:00" {
		t.Errorf("the kind was created at %v, want the time of the oldest post that carries it", at)
	}
}

func TestArchivedAndTrashedPostsComeWithTheirBoardAndThoseOfNoExtractedBoardAreCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), talkProject)
	posts := records(t, dir, bundle.Posts)

	for id, want := range map[string][2]string{"9703": {"archived", "9030"}, "9704": {"trashed", "9030"}} {
		post := recordWithID(t, posts, id)
		if post["status"] != want[0] || post["toolSourceId"] != want[1] {
			t.Errorf("post %s has the status %v on board %v, want %v", id, post["status"], post["toolSourceId"], want)
		}
	}
	if slices.Contains(sourceIDs(posts), "9721") {
		t.Error("a post of a board that is switched off was written")
	}
	if result.PostsLeftOut != 1 {
		t.Errorf("%d posts left out, want 1", result.PostsLeftOut)
	}
	if want := map[string]int{"active": 4, "archived": 1, "trashed": 1}; !reflect.DeepEqual(result.PostsByStatus, want) {
		t.Errorf("posts by status %v, want %v", result.PostsByStatus, want)
	}
}

func TestBasecampTellsOfNoPinSoNoPostCarriesOne(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), talkProject)
	for _, post := range records(t, dir, bundle.Posts) {
		if pinned, has := post["pinnedAt"]; !has || pinned != nil {
			t.Errorf("post %v is pinned at %v, want the key with null", post["sourceId"], pinned)
		}
	}
}

// A post is held to what a bundle can carry where the walk first meets it:
// before a category or a post is written and before a comment is asked for.
func TestAPostThatCannotBeCarriedEndsTheRunWhereItIsFirstMet(t *testing.T) {
	for what, c := range map[string]struct {
		change  func(*fakebasecamp.Account)
		message string
	}{
		"a draft":                    {func(a *fakebasecamp.Account) { news(a).Posts[1].ShownStatus = "draft" }, `post 9702 has the status "draft", which a bundle does not know`},
		"an archived post, drafted":  {func(a *fakebasecamp.Account) { news(a).Posts[2].ShownStatus = "drafted" }, `post 9703 has the status "drafted", which a bundle does not know`},
		"a post without createdAt":   {func(a *fakebasecamp.Account) { pitches(a).Posts[0].CreatedAt = "" }, "when post 9711 was created"},
		"a post without updatedAt":   {func(a *fakebasecamp.Account) { pitches(a).Posts[0].UpdatedAt = "" }, "when post 9711 was updated"},
		"a post created at no time":  {func(a *fakebasecamp.Account) { pitches(a).Posts[0].CreatedAt = "on a Thursday" }, "post 9711 was created at something that is no time"},
		"a category without a time":  {func(a *fakebasecamp.Account) { a.Projects[3].Categories[0].UpdatedAt = "" }, "when category 9602 was updated"},
		"a category without a birth": {func(a *fakebasecamp.Account) { a.Projects[3].Categories[0].CreatedAt = "" }, "when category 9602 was created"},
	} {
		account := fakebasecamp.New()
		c.change(account)
		writer := bundle.Discard()
		extractor := basecamp.Extractor{Source: account, Avatars: account}
		_, err := extractor.Extract(t.Context(), []string{talkProject}, writer)
		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API || !strings.HasSuffix(failed.Message, c.message) {
			t.Errorf("%s: got %v, want an error of the API that ends in %q", what, err, c.message)
		}
		counts := writer.Counts()
		if counts[bundle.Categories] != 0 || counts[bundle.Posts] != 0 || counts[bundle.Comments] != 0 || len(asked(account, "comments.json")) != 0 {
			t.Errorf("%s: %v was written and %v asked for", what, counts, asked(account, "comments.json"))
		}
	}
}

func TestTheCommentsOfPostsStandBehindThoseOfCardsEachParentOldestFirst(t *testing.T) {
	// A post of the project with the lowest number: its comment comes behind
	// those of the cards of a later project all the same.
	account := fakebasecamp.New()
	ada := &account.People[1]
	account.Projects[0].Dock[1].Posts = []fakebasecamp.Post{{
		ID: 9790, Subject: "An early word", Creator: ada, CreatedAt: "2022-06-01T08:00:00.000Z", UpdatedAt: "2022-06-01T08:00:00.000Z", Content: "<div>Hello</div>",
		Comments: []fakebasecamp.Comment{{ID: 9799, Creator: ada, CreatedAt: "2022-06-01T09:00:00.000Z", UpdatedAt: "2022-06-01T09:00:00.000Z", Content: "<div>Hello yourself</div>"}},
	}}
	// Two comments of one moment: the number of the ID decides, not its letters.
	comments := busyPost(account).Comments
	comments[0].ID, comments[0].CreatedAt = 10001, comments[1].CreatedAt
	dir, _ := extract(t, account, "7001", boardProject, talkProject)

	type comment struct{ id, parentKind, parent string }
	var got []comment
	for _, record := range records(t, dir, bundle.Comments) {
		got = append(got, comment{record["sourceId"].(string), record["parentKind"].(string), record["parentSourceId"].(string)})
	}
	want := []comment{
		{"9301", "card", "9201"}, {"9302", "card", "9201"}, {"9303", "card", "9201"},
		{"9799", "post", "9790"}, {"9751", "post", "9701"}, {"10001", "post", "9701"}, {"9753", "post", "9704"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("comments:\n got %v\nwant %v", got, want)
	}
	if project := records(t, dir, bundle.Comments)[4]["projectSourceId"]; project != "7004" {
		t.Errorf("the comment of a post names the project %v", project)
	}
}

func TestBoostsOfPostsTheirCommentsAndOfLinesBecomeReactionsInTheOrderOfWhatTheyHangOn(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), boardProject, talkProject)

	type reaction struct{ id, kind, target, by, content string }
	var got []reaction
	for _, record := range records(t, dir, bundle.Reactions) {
		got = append(got, reaction{record["sourceId"].(string), record["targetKind"].(string), record["targetSourceId"].(string), record["createdBy"].(string), record["content"].(string)})
	}
	// Those of cards, then those of posts, then those of lines; the boosts of a
	// post before those of its comments, and the boosts of one target oldest
	// first.
	want := []reaction{
		{"9503", "card", "9201", "103", "👍"}, {"9501", "comment", "9301", "102", "🎉"}, {"9502", "comment", "9301", "101", "Well done, all of you"},
		{"9792", "post", "9701", "104", "🎉"}, {"9791", "comment", "9751", "103", "👍"},
		{"9891", "message", "9806", "102", "Ha"}, {"9892", "message", "9806", "103", "👏"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("reactions:\n got %v\nwant %v", got, want)
	}
	boost := recordWithID(t, records(t, dir, bundle.Reactions), "9792")
	if boost["createdAt"] != "2023-04-10T13:00:00.000Z" || boost["updatedAt"] != boost["createdAt"] {
		t.Errorf("the boost of a post is of %v and %v", boost["createdAt"], boost["updatedAt"])
	}
}

var singleRecord = regexp.MustCompile(`/(messages|lines|comments|boosts)/[0-9]+\.json|/messages/[0-9]+/`)

func TestAPostAndALineCostNoRequestOfTheirOwn(t *testing.T) {
	account := fakebasecamp.New()
	extract(t, account, talkProject)

	for _, request := range account.Requests {
		if singleRecord.MatchString(request) {
			t.Errorf("%s was asked for, though a list shows its records in full", request)
		}
	}
	// Comments only where something says it has some, boosts likewise.
	if got, want := asked(account, "comments.json"), []string{
		"buckets/7004/recordings/9701/comments.json?page=1", "buckets/7004/recordings/9701/comments.json?page=2",
		"buckets/7004/recordings/9704/comments.json?page=1", "buckets/7004/recordings/9704/comments.json?page=2",
	}; !slices.Equal(got, want) {
		t.Errorf("comments were asked for with %v, want %v", got, want)
	}
	var boosted []string
	for _, request := range asked(account, "boosts.json?page=1") {
		boosted = append(boosted, strings.Split(request, "/")[3])
	}
	if want := []string{"9701", "9751", "9806"}; !slices.Equal(boosted, want) {
		t.Errorf("boosts were asked for of %v, want %v", boosted, want)
	}
}

// The boards are walked one after the other, so the first post that shows a
// kind is not the oldest one where two boards share it.
func TestAKindOnlyPostsShowIsAsOldAsTheOldestOfThemOnAnyBoard(t *testing.T) {
	account := fakebasecamp.New()
	shared := pitches(account).Posts[0].Category
	news(account).Posts = append(news(account).Posts, fakebasecamp.Post{ID: 9705, Subject: "A pitch on the wrong board", Category: shared,
		Creator: busyPost(account).Creator, CreatedAt: "2023-04-14T08:00:00.000Z", UpdatedAt: "2023-04-14T08:00:00.000Z", Content: "<div>Sorry</div>"})
	dir, _ := extract(t, account, talkProject)

	kind := recordWithID(t, records(t, dir, bundle.Categories), "9650")
	if kind["createdAt"] != "2023-04-11T09:00:00.000Z" || kind["updatedAt"] != kind["createdAt"] {
		t.Errorf("the kind is of %v and %v, want the time the oldest post that carries it was created", kind["createdAt"], kind["updatedAt"])
	}
}

func TestAMessageMetTwiceIsWrittenOnce(t *testing.T) {
	account := fakebasecamp.New()
	pitches(account).Posts = append(pitches(account).Posts, pitches(account).Posts[0])
	dir, result := extract(t, account, talkProject)

	if got, want := sourceIDs(records(t, dir, bundle.Posts))[4:], []string{"9712", "9711"}; !slices.Equal(got, want) {
		t.Errorf("posts %v, want %v", got, want)
	}
	if result.PostsByStatus["active"] != 4 {
		t.Errorf("%d active posts are counted, want 4", result.PostsByStatus["active"])
	}
}

func TestAKindOfPostWithoutANameEndsTheRunAndOneWithoutAnIconComes(t *testing.T) {
	for what, c := range map[string]struct {
		change  func(*fakebasecamp.Account)
		message string
	}{
		"in the list":     {func(a *fakebasecamp.Account) { a.Projects[3].Categories[1].Name = " " }, "category 9603 has no name"},
		"shown by a post": {func(a *fakebasecamp.Account) { pitches(a).Posts[0].Category.Name = "" }, "category 9650 has no name"},
		"of no post we see": {func(a *fakebasecamp.Account) {
			a.Projects[3].Dock[1].Posts[0].Category = &fakebasecamp.Category{ID: 9660}
		}, ""},
	} {
		account := fakebasecamp.New()
		c.change(account)
		writer := bundle.Discard()
		extractor := basecamp.Extractor{Source: account, Avatars: account}
		_, err := extractor.Extract(t.Context(), []string{talkProject}, writer)
		if c.message == "" {
			if err != nil {
				t.Errorf("%s: %v", what, err)
			}
			continue
		}
		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API || failed.Message != c.message {
			t.Errorf("%s: got %v, want the error %q", what, err, c.message)
		}
		if counts := writer.Counts(); counts[bundle.Categories] != 0 || counts[bundle.Posts] != 0 {
			t.Errorf("%s: %v was written", what, counts)
		}
	}

	account := fakebasecamp.New()
	account.Projects[3].Categories[1].Icon = ""
	dir, _ := extract(t, account, talkProject)
	if emoji, has := recordWithID(t, records(t, dir, bundle.Categories), "9603")["emoji"]; !has || emoji != "" {
		t.Errorf("a category without an icon comes with the emoji %v", emoji)
	}
}
