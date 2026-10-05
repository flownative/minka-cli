package basecamp_test

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// A comment and a line of a chat holding a control character Minka refuses
// come without it, named in the report, and the run goes on.
func TestACommentAndALineLoseTheControlCharactersMinkaRefuses(t *testing.T) {
	account := fakebasecamp.New()
	comment := &roadmap(account).Lists[2].Cards[0].Comments[0]
	comment.Content = "<div>Fish\x07 &amp; chips</div>"
	campfire(account).Lines[0].Content = "Good\x00 morning"

	dir, result := extract(t, account, boardProject, talkProject)
	if got := recordWithID(t, records(t, dir, bundle.Comments), "9301")["body"]; got != "<div>Fish &amp; chips</div>" {
		t.Errorf("the comment comes as %q", got)
	}
	if got := recordWithID(t, messages(t, dir), "9801")["body"].(string); strings.ContainsRune(got, 0) || !strings.Contains(got, "Good morning") {
		t.Errorf("the line comes as %q", got)
	}
	want := []basecamp.TextCleaned{{Record: "comment", SourceID: "9301"}, {Record: "line", SourceID: "9801"}}
	if !reflect.DeepEqual(result.TextsCleaned, want) {
		t.Errorf("the report names %+v, want %+v", result.TextsCleaned, want)
	}

	_, result = extract(t, fakebasecamp.New(), boardProject, talkProject)
	if len(result.TextsCleaned) != 0 {
		t.Errorf("texts without control characters are named: %+v", result.TextsCleaned)
	}
}

// A description of a project longer than Minka holds is cut, named in the
// report, and the project comes.
func TestADescriptionOfAProjectTooLongIsCut(t *testing.T) {
	account := fakebasecamp.New()
	account.Projects[0].Description = strings.Repeat("Plan the work. ", 200)
	dir, result := extract(t, account, "7001")
	description := recordWithID(t, records(t, dir, bundle.Projects), "7001")["description"].(string)
	if !strings.HasPrefix(description, "<p>Plan the work.") || !strings.HasSuffix(description, "work. Plan…</p>") {
		t.Errorf("the description comes as …%q", description[len(description)-40:])
	}
	if words := strings.TrimSuffix(strings.TrimPrefix(description, "<p>"), "</p>"); utf8.RuneCountInString(words) > 2000 {
		t.Errorf("the description holds %d characters", utf8.RuneCountInString(words))
	}
	if !reflect.DeepEqual(result.DescriptionsCut, []string{"7001"}) {
		t.Errorf("the report names %v", result.DescriptionsCut)
	}

	dir, result = extract(t, fakebasecamp.New(), "7001")
	if got := recordWithID(t, records(t, dir, bundle.Projects), "7001")["description"]; got != "<p>First line<br>second &lt;line&gt;</p><p>Last &amp; least</p>" {
		t.Errorf("a description that fits comes as %q", got)
	}
	if len(result.DescriptionsCut) != 0 {
		t.Errorf("the report names %v", result.DescriptionsCut)
	}
}
