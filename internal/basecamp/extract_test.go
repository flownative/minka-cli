package basecamp_test

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// extract runs an extraction into a bundle in a temporary directory and
// returns where it stands.
func extract(t *testing.T, account *fakebasecamp.Account, projects ...string) (string, basecamp.Result) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	writer, err := bundle.Create(dir, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()

	extractor := basecamp.Extractor{Source: account, Avatars: account}
	result, err := extractor.Extract(t.Context(), projects, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Commit(bundle.Manifest{SourceInstance: result.SourceInstance}); err != nil {
		t.Fatal(err)
	}
	return dir, result
}

func records(t *testing.T, dir string, kind bundle.Kind) []map[string]any {
	t.Helper()
	file, err := os.Open(filepath.Join(dir, string(kind)+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var all []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("%s: a line is no JSON object: %v: %s", kind, err, scanner.Text())
		}
		all = append(all, record)
	}
	return all
}

func TestPeopleOfTheAccountComeWithEmailAvatarAndWhetherTheyAreStillThere(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), "7001")

	want := []map[string]any{
		{"sourceId": "99", "createdAt": "2019-01-01T08:00:00.000Z", "updatedAt": "2021-01-01T08:00:00.000Z", "createdBy": nil,
			"name": "Former Colleague", "email": "former@example.test", "avatar": "avatar-99", "retired": true},
		{"sourceId": "101", "createdAt": "2020-01-01T08:00:00.000Z", "updatedAt": "2024-01-01T08:00:00.000Z", "createdBy": nil,
			"name": "Ada Example", "email": "ada@example.test", "avatar": "avatar-101", "retired": false},
		{"sourceId": "102", "createdAt": "2020-02-01T08:00:00.000Z", "updatedAt": "2024-02-01T08:00:00.000Z", "createdBy": nil,
			"name": "Grace Sample", "email": "grace@example.test", "avatar": "avatar-102", "retired": false},
		{"sourceId": "103", "createdAt": "2020-03-01T08:00:00.000Z", "updatedAt": "2024-03-01T08:00:00.000Z", "createdBy": nil,
			"name": "Linus Placeholder", "email": nil, "avatar": "avatar-103", "retired": false},
		{"sourceId": "104", "createdAt": "2020-04-01T08:00:00.000Z", "updatedAt": "2024-04-01T08:00:00.000Z", "createdBy": nil,
			"name": "Edsger Invented", "email": "edsger@example.test", "avatar": nil, "retired": false},
	}
	if got := records(t, dir, bundle.People); !reflect.DeepEqual(got, want) {
		t.Errorf("people:\n got %v\nwant %v", got, want)
	}
	if result.Avatars != 4 {
		t.Errorf("%d avatars, want 4", result.Avatars)
	}
}

func TestAnAvatarIsAnAttachmentOfItsPersonWithItsFile(t *testing.T) {
	dir, _ := extract(t, fakebasecamp.New(), "7001")

	attachments := records(t, dir, bundle.Attachments)
	if len(attachments) != 4 {
		t.Fatalf("%d attachments, want 4", len(attachments))
	}
	content := fakebasecamp.AvatarOf(101)
	sum := sha256.Sum256(content)
	want := map[string]any{
		"sourceId": "avatar-101", "createdAt": "2020-01-01T08:00:00.000Z", "updatedAt": "2024-01-01T08:00:00.000Z", "createdBy": "101",
		"filename": "avatar.webp", "contentType": "image/webp", "bytes": float64(len(content)),
		"sha256": hex.EncodeToString(sum[:]), "file": "avatar-101/avatar.webp",
		"ownerKind": "person", "ownerSourceId": "101",
	}
	got := attachments[1]
	if !reflect.DeepEqual(got, want) {
		t.Errorf("attachment:\n got %v\nwant %v", got, want)
	}

	stored, err := os.ReadFile(filepath.Join(dir, "files", got["file"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, content) {
		t.Errorf("the file holds %q", stored)
	}
}

func TestAProjectComesWithItsMembersAndItsDescriptionAsHTML(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), "7002", "7001", "7001")

	want := []map[string]any{
		{"sourceId": "7001", "createdAt": "2022-05-01T09:00:00.000Z", "updatedAt": "2025-06-01T09:00:00.000Z", "createdBy": nil,
			"name": "Example Project", "description": "<p>First line<br>second &lt;line&gt;</p><p>Last &amp; least</p>",
			"status": "active", "members": []any{"101", "102", "103", "104"}},
		{"sourceId": "7002", "createdAt": "2023-01-01T09:00:00.000Z", "updatedAt": "2023-06-01T09:00:00.000Z", "createdBy": nil,
			"name": "Second Project", "description": "", "status": "archived", "members": []any{"101"}},
	}
	if got := records(t, dir, bundle.Projects); !reflect.DeepEqual(got, want) {
		t.Errorf("projects:\n got %v\nwant %v", got, want)
	}
	if result.SourceInstance != "4242" {
		t.Errorf("source instance %q, want the account 4242", result.SourceInstance)
	}
}

func TestOfADockTheToolsMinkaKnowsComeOverAndTheRestIsCounted(t *testing.T) {
	dir, result := extract(t, fakebasecamp.New(), "7001", "7002")

	type tool struct{ id, project, kind, name, createdBy string }
	var got []tool
	for _, record := range records(t, dir, bundle.Tools) {
		createdBy, _ := record["createdBy"].(string)
		got = append(got, tool{record["sourceId"].(string), record["projectSourceId"].(string), record["kind"].(string), record["name"].(string), createdBy})
	}
	want := []tool{
		{"9001", "7001", "chat", "Campfire", "101"},
		{"9002", "7001", "posts", "Announcements", "101"},
		{"9003", "7001", "board", "Bugs", "99"},
		{"9004", "7001", "chat", "Second chat", "102"},
		{"9010", "7002", "chat", "Campfire", ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("tools:\n got %v\nwant %v", got, want)
	}

	first := records(t, dir, bundle.Tools)[0]
	if first["position"] != float64(1) || first["createdAt"] != "2022-05-01T10:00:00.000Z" || first["updatedAt"] != "2025-05-01T10:00:00.000Z" {
		t.Errorf("the first tool: %v", first)
	}

	skipped := []basecamp.SkippedTools{
		{Kind: "kanban_board", Enabled: false, Count: 1},
		{Kind: "schedule", Enabled: false, Count: 1},
		{Kind: "todoset", Enabled: true, Count: 2},
		{Kind: "vault", Enabled: true, Count: 1},
	}
	if !slices.Equal(result.Skipped, skipped) {
		t.Errorf("skipped:\n got %v\nwant %v", result.Skipped, skipped)
	}
}

func TestEveryReferenceInABundleLeadsToARecordOfTheBundle(t *testing.T) {
	// The creator of the first project is somebody nothing else names.
	account := fakebasecamp.New()
	stranger := fakebasecamp.Person{ID: 555, Name: "A Stranger", CreatedAt: "2021-01-01T08:00:00.000Z", UpdatedAt: "2021-01-01T08:00:00.000Z"}
	account.Projects[0].Dock[4].CreatedAt, account.Projects[0].Dock[4].Creator = "2022-05-01T09:00:00.150Z", &stranger
	dir, _ := extract(t, account, "7001", "7002", "7003")

	people := map[string]bool{}
	for _, record := range records(t, dir, bundle.People) {
		people[record["sourceId"].(string)] = true
	}
	projects := map[string]bool{}
	for _, record := range records(t, dir, bundle.Projects) {
		projects[record["sourceId"].(string)] = true
		if creator, ok := record["createdBy"].(string); ok && !people[creator] {
			t.Errorf("project %v was created by %s, who is no person of the bundle", record["sourceId"], creator)
		}
		if id := record["sourceId"]; id != "7002" && record["createdBy"] == nil {
			t.Errorf("project %v names no creator", id)
		}
		for _, member := range record["members"].([]any) {
			if !people[member.(string)] {
				t.Errorf("member %v of project %v is no person of the bundle", member, record["sourceId"])
			}
		}
	}
	for _, record := range records(t, dir, bundle.Attachments) {
		if !people[record["ownerSourceId"].(string)] || !people[record["createdBy"].(string)] {
			t.Errorf("attachment %v belongs to somebody who is no person of the bundle", record["sourceId"])
		}
	}
	for _, record := range records(t, dir, bundle.Tools) {
		if !projects[record["projectSourceId"].(string)] {
			t.Errorf("tool %v names a project that is not in the bundle", record["sourceId"])
		}
		if creator, ok := record["createdBy"].(string); ok && !people[creator] {
			t.Errorf("tool %v was created by %s, who is no person of the bundle", record["sourceId"], creator)
		}
	}
}

func TestTwoRunsOverTheSameAccountWriteTheSameBytes(t *testing.T) {
	first, _ := extract(t, fakebasecamp.New(), "7001", "7002")

	// The order a source lists things in is not what a bundle depends on.
	shuffled := fakebasecamp.New()
	slices.Reverse(shuffled.People)
	slices.Reverse(shuffled.Projects[0].Dock)
	slices.Reverse(shuffled.Projects[0].Members)
	second, _ := extract(t, shuffled, "7002", "7001")

	for _, name := range []string{"people.jsonl", "projects.jsonl", "tools.jsonl", "columns.jsonl", "attachments.jsonl", "manifest.json"} {
		a, err := os.ReadFile(filepath.Join(first, name))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(second, name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Errorf("%s differs between two runs:\n%s\n%s", name, a, b)
		}
	}
}

func TestADiscardedRunWalksTheSameWayAndDownloadsNoAvatar(t *testing.T) {
	written := fakebasecamp.New()
	extract(t, written, "7001")

	dry := fakebasecamp.New()
	writer := bundle.Discard()
	extractor := basecamp.Extractor{Source: dry, Avatars: dry}
	result, err := extractor.Extract(t.Context(), []string{"7001"}, writer)
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(dry.Requests, written.Requests) {
		t.Errorf("a dry run asked for\n%v\na real one for\n%v", dry.Requests, written.Requests)
	}
	if len(dry.AvatarRequests) != 0 {
		t.Errorf("a dry run downloaded %v", dry.AvatarRequests)
	}
	want := map[bundle.Kind]int{bundle.People: 5, bundle.Projects: 1, bundle.Tools: 4, bundle.Columns: 3, bundle.Attachments: 4}
	if got := writer.Counts(); !reflect.DeepEqual(got, want) {
		t.Errorf("counts %v, want %v", got, want)
	}
	if result.Avatars != 4 {
		t.Errorf("%d avatars, want 4", result.Avatars)
	}
}

func TestAToolAtAnAddressOutsideTheAccountIsNotFetched(t *testing.T) {
	for _, address := range []string{
		"https://elsewhere.example.test/4242/buckets/7001/chats/9001.json",
		"https://3.basecampapi.test/1/buckets/7001/chats/9001.json",
		"https://3.basecampapi.test/4242/../1/buckets/7001/chats/9001.json",
		"https://3.basecampapi.test/4242//elsewhere.example.test/chats/9001.json",
		"https://3.basecampapi.test/4242/buckets/7001/chats/9001.json?next=https://elsewhere.example.test",
		"buckets/7001/chats/9001.json",
	} {
		account := fakebasecamp.New()
		account.Projects[0].Dock[0].URL = address

		extractor := basecamp.Extractor{Source: account, Avatars: account}
		_, err := extractor.Extract(t.Context(), []string{"7001"}, bundle.Discard())
		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API {
			t.Errorf("%q: got %v, want an API error", address, err)
		}
		for _, request := range account.Requests {
			if strings.Contains(request, "://") || strings.Contains(request, "..") || strings.Contains(request, "//") {
				t.Errorf("%q: asked the Basecamp CLI for %q", address, request)
			}
		}
	}
}

func TestAProjectThatIsNotThereIsNamedInTheError(t *testing.T) {
	account := fakebasecamp.New()
	extractor := basecamp.Extractor{Source: account, Avatars: account}
	_, err := extractor.Extract(t.Context(), []string{"7001", "8888"}, bundle.Discard())

	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.NotFound {
		t.Fatalf("got %v, want not found", err)
	}
	if !strings.Contains(failed.Message, "8888") {
		t.Errorf("the message %q does not name the project", failed.Message)
	}
}

func TestWhatIsNoProjectIDNeverBecomesAPath(t *testing.T) {
	for _, id := range []string{"", "abc", "-1", "7001/../people", "7001.json", "99999999999999999999999"} {
		account := fakebasecamp.New()
		extractor := basecamp.Extractor{Source: account, Avatars: account}
		if _, err := extractor.Extract(t.Context(), []string{id}, bundle.Discard()); err == nil {
			t.Errorf("%q was taken as the ID of a project", id)
		}
		if len(account.Requests) != 0 {
			t.Errorf("%q: asked for %v", id, account.Requests)
		}
	}
}

func expectAPIError(t *testing.T, account *fakebasecamp.Account, what string, projects ...string) {
	t.Helper()
	extractor := basecamp.Extractor{Source: account, Avatars: account}
	_, err := extractor.Extract(t.Context(), projects, bundle.Discard())
	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.API {
		t.Errorf("%s: got %v, want an API error", what, err)
	}
}

// expectRefusedAtOnce holds a project to be refused by what it says itself,
// before anything else of it is asked for.
func expectRefusedAtOnce(t *testing.T, account *fakebasecamp.Account, what, project string, projects ...string) {
	t.Helper()
	expectAPIError(t, account, what, projects...)
	if slices.Contains(account.Requests, "projects/"+project+"/people.json?page=1") {
		t.Errorf("%s: the run went on with the project: %v", what, account.Requests)
	}
}

func TestTheAddressOfAProjectHasToNameItsAccount(t *testing.T) {
	for _, address := range []string{
		"http://3.basecampapi.test/4242/projects/7001.json",
		"https://3.basecampapi.test/projects/7001.json",
		"https://3.basecampapi.test/an-account/projects/7001.json",
		"https://3.basecampapi.test/4242/projects/7002.json",
		"https://3.basecampapi.test/4242/projects/7001.json?to=elsewhere",
		"https://3.basecampapi.test/4242/people/7001.json",
		"4242/projects/7001.json",
	} {
		account := fakebasecamp.New()
		account.Projects[0].URL = address
		expectRefusedAtOnce(t, account, address, "7001", "7001")
	}
}

func TestProjectsOfTwoAccountsDoNotShareABundle(t *testing.T) {
	account := fakebasecamp.New()
	account.Projects[1].URL = "https://3.basecampapi.test/5555/projects/7002.json"
	expectRefusedAtOnce(t, account, "a project of another account", "7002", "7001", "7002")
}

func TestWhatABundleCannotSayEndsTheRun(t *testing.T) {
	for what, change := range map[string]func(*fakebasecamp.Account){
		"a status the format does not know": func(a *fakebasecamp.Account) { a.Projects[0].Status = "paused" },
		"a project without a status":        func(a *fakebasecamp.Account) { a.Projects[0].Status = "" },
		"a project without createdAt":       func(a *fakebasecamp.Account) { a.Projects[0].CreatedAt = "" },
		"a project without updatedAt":       func(a *fakebasecamp.Account) { a.Projects[0].UpdatedAt = "" },
		"a tool without createdAt":          func(a *fakebasecamp.Account) { a.Projects[0].Dock[0].CreatedAt = "" },
		"a tool without updatedAt":          func(a *fakebasecamp.Account) { a.Projects[0].Dock[0].UpdatedAt = "" },
	} {
		account := fakebasecamp.New()
		change(account)
		expectAPIError(t, account, what, "7001")
	}
	for _, status := range []string{"active", "archived", "trashed"} {
		account := fakebasecamp.New()
		account.Projects[0].Status = status
		dir, _ := extract(t, account, "7001")
		if got := records(t, dir, bundle.Projects)[0]["status"]; got != status {
			t.Errorf("status %v, want %s", got, status)
		}
	}
}

func TestAFileThatCannotBeFetchedIsLeftOutAndNamed(t *testing.T) {
	account := fakebasecamp.New()
	account.FailAvatarOf[102] = &basecamp.Error{Kind: basecamp.Network, Message: "downloading an avatar: connection reset"}
	account.FailAvatarOf[103] = &basecamp.Error{Kind: basecamp.API, Message: "downloading an avatar: HTTP 404"}
	dir, result := extract(t, account, "7001")

	want := []basecamp.MissingFile{
		{AttachmentSourceID: "avatar-102", OwnerKind: "person", OwnerSourceID: "102", Reason: "downloading an avatar: connection reset", Retryable: true},
		{AttachmentSourceID: "avatar-103", OwnerKind: "person", OwnerSourceID: "103", Reason: "downloading an avatar: HTTP 404", Retryable: false},
	}
	if !slices.Equal(result.Missing, want) {
		t.Errorf("missing:\n got %v\nwant %v", result.Missing, want)
	}
	if result.Avatars != 2 {
		t.Errorf("%d avatars, want the 2 that came", result.Avatars)
	}

	avatars := map[string]any{}
	for _, person := range records(t, dir, bundle.People) {
		avatars[person["sourceId"].(string)] = person["avatar"]
	}
	wantAvatars := map[string]any{"99": "avatar-99", "101": "avatar-101", "102": nil, "103": nil, "104": nil}
	if !reflect.DeepEqual(avatars, wantAvatars) {
		t.Errorf("avatars of the people: %v, want %v", avatars, wantAvatars)
	}
	var attached []string
	for _, attachment := range records(t, dir, bundle.Attachments) {
		attached = append(attached, attachment["sourceId"].(string))
	}
	if !slices.Equal(attached, []string{"avatar-99", "avatar-101"}) {
		t.Errorf("attachments %v", attached)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "files"))
	if len(entries) != 2 {
		t.Errorf("files/ holds %v", entries)
	}

	var manifest bundle.Manifest
	content, _ := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err := json.Unmarshal(content, &manifest); err != nil || manifest.Counts[bundle.Attachments] != 2 || manifest.Counts[bundle.People] != 5 {
		t.Errorf("manifest %s (%v)", content, err)
	}
}

func TestAnInterruptionEndsTheRunInsteadOfLeavingFilesOut(t *testing.T) {
	account := fakebasecamp.New()
	account.FailAvatarOf[102] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}

	writer, err := bundle.Create(filepath.Join(t.TempDir(), "bundle"), basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()
	extractor := basecamp.Extractor{Source: account, Avatars: account}
	_, err = extractor.Extract(t.Context(), []string{"7001"}, writer)

	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.Interrupted {
		t.Fatalf("got %v, want the interruption", err)
	}
	if len(account.AvatarRequests) != 3 {
		t.Errorf("went on downloading after the interruption: %v", account.AvatarRequests)
	}
}

func TestAnErrorOfTheSourceIsNotChangedForTheNextRun(t *testing.T) {
	account := fakebasecamp.New()
	shared := &basecamp.Error{Kind: basecamp.NotFound, Message: "as the source said it"}
	account.Fail["projects/7001.json"] = shared

	extractor := basecamp.Extractor{Source: account, Avatars: account}
	_, err := extractor.Extract(t.Context(), []string{"7001"}, bundle.Discard())
	if err == nil || !strings.Contains(err.Error(), "7001") {
		t.Errorf("got %v, want the project named", err)
	}
	if shared.Message != "as the source said it" {
		t.Errorf("the error of the source now reads %q", shared.Message)
	}
}

const maskedAddress = "g\u2022\u2022\u2022\u2022@\u2022\u2022\u2022\u2022\u2022\u2022\u2022.\u2022\u2022\u2022\u2022"

func TestAnAddressComesIntoTheBundleWithoutTheWhitespaceAroundIt(t *testing.T) {
	account := fakebasecamp.New()
	account.People[1].Email = " ada@example.test\n"
	account.Projects[0].Dock[2].Creator.Email = "\tformer@example.test "
	dir, _ := extract(t, account, "7001")

	addresses := map[string]any{}
	for _, person := range records(t, dir, bundle.People) {
		addresses[person["sourceId"].(string)] = person["email"]
	}
	if addresses["101"] != "ada@example.test" || addresses["99"] != "former@example.test" {
		t.Errorf("addresses in the bundle: %q and %q", addresses["101"], addresses["99"])
	}
}

// endsBeforeAnythingElse runs an extraction that is to end at the people list
// of the account, for a real bundle and for a dry run.
func endsBeforeAnythingElse(t *testing.T, change func(*fakebasecamp.Account), want basecamp.Kind, counts string) {
	t.Helper()
	for _, dry := range []bool{false, true} {
		account := fakebasecamp.New()
		change(account)

		parent := t.TempDir()
		writer := bundle.Discard()
		if !dry {
			var err error
			if writer, err = bundle.Create(filepath.Join(parent, "bundle"), basecamp.SourceName); err != nil {
				t.Fatal(err)
			}
		}
		extractor := basecamp.Extractor{Source: account, Avatars: account}
		_, err := extractor.Extract(t.Context(), []string{"7002"}, writer)
		writer.Abort()

		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != want || failed.Hint == "" {
			t.Fatalf("dry %v: got %v, want %s with a hint", dry, err, want)
		}
		if !strings.Contains(failed.Message, counts) || strings.ContainsAny(failed.Message, "@\u2022\u25cf") {
			t.Errorf("dry %v: the message: %s", dry, failed.Message)
		}
		for _, request := range account.Requests {
			if !strings.HasPrefix(request, "people.json") {
				t.Errorf("dry %v: went on to ask for %s", dry, request)
			}
		}
		if len(account.AvatarRequests) != 0 || len(writer.Counts()) != 0 {
			t.Errorf("dry %v: downloaded %v, counted %v", dry, account.AvatarRequests, writer.Counts())
		}
		if entries, _ := os.ReadDir(parent); len(entries) != 0 {
			t.Errorf("dry %v: left %v", dry, entries)
		}
	}
}

// Project 7002 has to do with Ada alone. The mask is on somebody the bundle
// would not even hold: it still says that the identity sees no addresses.
func TestOneMaskedAddressEndsTheRunBeforeAnythingElseIsAskedFor(t *testing.T) {
	endsBeforeAnythingElse(t, func(a *fakebasecamp.Account) { a.People[4].Email = maskedAddress }, basecamp.Forbidden, "1 of 4")
}

func TestMostAddressesBeingNoneEndsTheRunBeforeAnythingElseIsAskedFor(t *testing.T) {
	endsBeforeAnythingElse(t, func(a *fakebasecamp.Account) {
		for _, i := range []int{0, 2, 4} {
			a.People[i].Email = "a\u25cf\u25cf@\u25cf\u25cf\u25cf.\u25cf\u25cf"
		}
	}, basecamp.API, "3 of 4")
}

func TestSomebodyWithAnAddressThatIsNoneComesWithoutOneAndIsNamedByID(t *testing.T) {
	account := fakebasecamp.New()
	account.People[3].Email = `"linus@example.test"`               // 103, a member
	account.People[4].Email = "nobody refers to this one"          // 105, left out
	account.Projects[0].Dock[2].Creator.Email = "former colleague" // 99, a creator who left
	dir, result := extract(t, account, "7001")

	if !slices.Equal(result.InvalidAddresses, []string{"99", "103"}) {
		t.Errorf("invalid addresses %v, want 99 and 103 in the order of the numbers", result.InvalidAddresses)
	}
	addresses := map[string]any{}
	for _, person := range records(t, dir, bundle.People) {
		addresses[person["sourceId"].(string)] = person["email"]
	}
	want := map[string]any{"99": nil, "101": "ada@example.test", "102": "grace@example.test", "103": nil, "104": "edsger@example.test"}
	if !reflect.DeepEqual(addresses, want) {
		t.Errorf("addresses %v, want %v", addresses, want)
	}
}

func TestACreatorWithAMaskedAddressEndsTheRun(t *testing.T) {
	account := fakebasecamp.New()
	account.Projects[0].Dock[2].Creator.Email = maskedAddress
	extractor := basecamp.Extractor{Source: account, Avatars: account}
	_, err := extractor.Extract(t.Context(), []string{"7001"}, bundle.Discard())
	var failed *basecamp.Error
	if !errors.As(err, &failed) || failed.Kind != basecamp.Forbidden {
		t.Fatalf("got %v, want forbidden", err)
	}
}

func TestOnlyPeopleARecordRefersToComeIntoTheBundle(t *testing.T) {
	account := fakebasecamp.New()
	dir, result := extract(t, account, "7002")

	var written []string
	for _, person := range records(t, dir, bundle.People) {
		written = append(written, person["sourceId"].(string))
	}
	if !slices.Equal(written, []string{"101"}) || result.PeopleLeftOut != 4 {
		t.Errorf("wrote %v and left %d out, want the one member and 4 left out", written, result.PeopleLeftOut)
	}
	if len(account.AvatarRequests) != 1 || !strings.Contains(account.AvatarRequests[0], "/people/101/") {
		t.Errorf("downloaded %v, want the avatar of the one person only", account.AvatarRequests)
	}
	if attachments := records(t, dir, bundle.Attachments); len(attachments) != 1 || result.Avatars != 1 {
		t.Errorf("%d attachments, %d avatars", len(attachments), result.Avatars)
	}

	_, result = extract(t, fakebasecamp.New(), "7001", "7002")
	if result.PeopleLeftOut != 1 {
		t.Errorf("%d left out, want the one nobody refers to", result.PeopleLeftOut)
	}
}
