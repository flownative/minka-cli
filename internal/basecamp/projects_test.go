package basecamp_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// extractEvery runs an extraction of every project the account lists into a
// bundle in a temporary directory and returns where it stands.
func extractEvery(t *testing.T, account *fakebasecamp.Account, activeOnly bool) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	writer, err := bundle.Create(dir, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Abort()

	extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account, Now: func() time.Time { return runDay }}
	result, err := extractor.ExtractEveryProject(t.Context(), activeOnly, writer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Commit(bundle.Manifest{SourceInstance: result.SourceInstance}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// projectsIn are the projects of a bundle as "<sourceId> <status>", in the order
// of the file.
func projectsIn(t *testing.T, dir string) []string {
	t.Helper()
	var projects []string
	for _, record := range records(t, dir, bundle.Projects) {
		projects = append(projects, record["sourceId"].(string)+" "+record["status"].(string))
	}
	return projects
}

// accountListedBackwards is the fake account with three archived projects, one
// in the trash, and its projects listed from the highest ID down, so that the
// order of a bundle can be told from the order of the listings.
func accountListedBackwards() *fakebasecamp.Account {
	account := fakebasecamp.New()
	for i := range account.Projects {
		switch account.Projects[i].ID {
		case 7003, 7009:
			account.Projects[i].Status = "archived"
		case 7010:
			account.Projects[i].Status = "trashed"
		}
	}
	slices.Reverse(account.Projects)
	return account
}

func TestWithoutProjectsNamedEveryActiveAndArchivedProjectComesInTheOrderOfTheIDs(t *testing.T) {
	account := accountListedBackwards()
	dir := extractEvery(t, account, false)

	want := []string{"7001 active", "7002 archived", "7003 archived", "7004 active", "7005 active", "7006 active",
		"7007 active", "7008 active", "7009 archived", "7011 active"}
	if got := projectsIn(t, dir); !reflect.DeepEqual(got, want) {
		t.Errorf("projects:\n got %v\nwant %v", got, want)
	}
	// Four pages of active projects and two of archived ones, each list asked
	// until a page comes back empty; the trash is never asked for.
	for _, page := range []string{"projects.json?page=5", "projects.json?status=archived&page=2", "projects.json?status=archived&page=3"} {
		if !slices.Contains(account.Requests, page) {
			t.Errorf("%s was not asked for: %v", page, account.Requests)
		}
	}
	for _, request := range account.Requests {
		if strings.HasPrefix(request, "projects.json?status=trashed") || request == "projects/7010.json" {
			t.Errorf("the trash was asked for: %s", request)
		}
	}
}

func TestTheProjectsAreListedOnlyOnceThePeopleOfTheAccountAreRead(t *testing.T) {
	account := fakebasecamp.New()
	extractEvery(t, account, false)

	people := slices.Index(account.Requests, "people.json?page=1")
	projects := slices.Index(account.Requests, "projects.json?page=1")
	if people < 0 || projects < people {
		t.Errorf("the people at request %d, the projects at %d: %v", people, projects, account.Requests)
	}
}

func TestActiveOnlyLeavesTheArchivedProjectsOutAndDoesNotAskForThem(t *testing.T) {
	account := accountListedBackwards()
	dir := extractEvery(t, account, true)

	want := []string{"7001 active", "7004 active", "7005 active", "7006 active", "7007 active", "7008 active", "7011 active"}
	if got := projectsIn(t, dir); !reflect.DeepEqual(got, want) {
		t.Errorf("projects:\n got %v\nwant %v", got, want)
	}
	for _, request := range account.Requests {
		if strings.HasPrefix(request, "projects.json?status=") {
			t.Errorf("a list of projects of another status was asked for: %s", request)
		}
	}
}

func TestAProjectInBothListsIsWalkedOnce(t *testing.T) {
	account := fakebasecamp.New()
	account.AlsoListedActive = []int64{7002}
	dir := extractEvery(t, account, false)

	got := projectsIn(t, dir)
	if first := slices.Index(got, "7002 archived"); first < 0 || slices.Contains(got[first+1:], "7002 archived") {
		t.Errorf("the project did not come once: %v", got)
	}
}

func TestAnAccountThatListsNoProjectEndsTheRun(t *testing.T) {
	for name, c := range map[string]struct {
		statuses   []string
		activeOnly bool
		message    string
	}{
		"no project at all":          {nil, false, "no project"},
		"only archived, active only": {[]string{"archived"}, true, "no active project"},
		"only trashed, all of them":  {[]string{"trashed"}, false, "no project"},
		"only trashed, active only":  {[]string{"trashed"}, true, "no active project"},
	} {
		t.Run(name, func(t *testing.T) {
			account := fakebasecamp.New()
			if c.statuses == nil {
				account.Projects = nil
			}
			for i := range account.Projects {
				account.Projects[i].Status = c.statuses[0]
			}
			extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
			_, err := extractor.ExtractEveryProject(t.Context(), c.activeOnly, bundle.Discard())

			var failed *basecamp.Error
			if !errors.As(err, &failed) || failed.Kind != basecamp.NotFound {
				t.Fatalf("got %v, want not found", err)
			}
			if !strings.Contains(failed.Message, "lists "+c.message+" ") {
				t.Errorf("the message %q does not say %q", failed.Message, c.message)
			}
		})
	}
}

// An ID of 0 is also what an entry without an ID reads as.
func TestAListedProjectWhoseIDIsNoneNeverBecomesAPath(t *testing.T) {
	for _, id := range []int64{-7002, 0} {
		account := fakebasecamp.New()
		account.Projects[1].ID = id
		extractor := basecamp.Extractor{Source: account, Avatars: account, Files: account}
		_, err := extractor.ExtractEveryProject(t.Context(), false, bundle.Discard())

		var failed *basecamp.Error
		if !errors.As(err, &failed) || failed.Kind != basecamp.API {
			t.Fatalf("%d: got %v, want an API error", id, err)
		}
		for _, request := range account.Requests {
			if strings.HasPrefix(request, "projects/") {
				t.Errorf("%d: a project was asked for: %s", id, request)
			}
		}
	}
}
