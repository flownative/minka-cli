package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// line is a record that says which one it is.
func line(n int) Message {
	return Message{Record: Record{SourceID: fmt.Sprint(n)}}
}

// idsIn reads the source IDs of a file of a bundle, in its order.
func idsIn(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, row := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var record struct {
			SourceID string `json:"sourceId"`
		}
		if err := json.Unmarshal([]byte(row), &record); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		ids = append(ids, record.SourceID)
	}
	return ids
}

func manifestOf(t *testing.T, target string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(target, manifestFile))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// writeParts adds records in groups, each of which ends its part, and commits.
// A part may end before any record.
func writeParts(t *testing.T, w *Writer, size int, groups ...int) {
	t.Helper()
	if err := w.InParts(Messages, size); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, group := range groups {
		for range group {
			n++
			if err := w.MayCut(Messages); err != nil {
				t.Fatal(err)
			}
			mustAdd(t, w, Messages, line(n))
		}
		if err := w.EndPart(Messages); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
}

func TestAKindInPartsIsCutAtTheSizeAndWhereAPartIsEnded(t *testing.T) {
	for name, c := range map[string]struct {
		size   int
		groups []int
		want   [][]string
	}{
		"one more than a part holds": {3, []int{4}, [][]string{{"1", "2", "3"}, {"4"}}},
		"exactly two parts":          {3, []int{6}, [][]string{{"1", "2", "3"}, {"4", "5", "6"}}},
		"two groups under the size":  {10, []int{2, 1}, [][]string{{"1", "2"}, {"3"}}},
		"a full group and another":   {3, []int{3, 1}, [][]string{{"1", "2", "3"}, {"4"}}},
		"a group of nothing between": {3, []int{1, 0, 0, 2}, [][]string{{"1"}, {"2", "3"}}},
		"a long group and a short":   {2, []int{5, 1}, [][]string{{"1", "2"}, {"3", "4"}, {"5"}, {"6"}}},
	} {
		target := filepath.Join(t.TempDir(), "bundle")
		w := mustCreate(t, target)
		writeParts(t, w, c.size, c.groups...)

		var names []any
		for i, want := range c.want {
			file := fmt.Sprintf("messages-%04d.jsonl", i+1)
			names = append(names, file)
			if got := idsIn(t, filepath.Join(target, file)); !slices.Equal(got, want) {
				t.Errorf("%s: %s holds %v, want %v", name, file, got, want)
			}
		}
		manifest := manifestOf(t, target)
		if want := map[string]any{"messages": names}; !reflect.DeepEqual(manifest["parts"], want) {
			t.Errorf("%s: the manifest names the parts %v, want %v", name, manifest["parts"], want)
		}
		if total := manifest["counts"].(map[string]any)["messages"]; total != float64(len(slices.Concat(c.want...))) {
			t.Errorf("%s: the manifest counts %v messages", name, total)
		}
		// Neither a part too many nor the one file beside the parts.
		if got, want := entriesOf(t, target), append([]string{manifestFile}, toStrings(names)...); !slices.Equal(got, want) {
			t.Errorf("%s: the bundle holds %v, want %v", name, got, want)
		}
		if !reflect.DeepEqual(w.Parts(), map[Kind][]string{Messages: toStrings(names)}) {
			t.Errorf("%s: the writer names the parts %v", name, w.Parts())
		}
	}
}

func toStrings(values []any) []string {
	names := make([]string, len(values))
	for i, value := range values {
		names[i] = value.(string)
	}
	return names
}

func TestAKindThatCameToOnePartIsTheOneFileOfItsKind(t *testing.T) {
	for name, groups := range map[string][]int{
		"fewer than a part holds":   {2},
		"exactly what a part holds": {3},
		"beside groups of nothing":  {0, 3, 0},
	} {
		target := filepath.Join(t.TempDir(), "bundle")
		w := mustCreate(t, target)
		writeParts(t, w, 3, groups...)

		if got, want := entriesOf(t, target), []string{manifestFile, "messages.jsonl"}; !slices.Equal(got, want) {
			t.Errorf("%s: the bundle holds %v, want %v", name, got, want)
		}
		if _, has := manifestOf(t, target)["parts"]; has || w.Parts() != nil {
			t.Errorf("%s: the manifest names parts of a kind that has one file", name)
		}
	}
}

func TestAKindWithoutRecordsHasNeitherAFileNorParts(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)
	writeParts(t, w, 3, 0, 0)

	if got := entriesOf(t, target); !slices.Equal(got, []string{manifestFile}) {
		t.Errorf("the bundle holds %v", got)
	}
	if manifest := manifestOf(t, target); manifest["parts"] != nil {
		t.Errorf("the manifest names the parts %v", manifest["parts"])
	}
}

// Only the caller knows which records have to stay in one file: a part ends
// where it says so, and nowhere else, however many records that makes.
func TestAPartEndsOnlyWhereItMayOnceItHoldsItsSize(t *testing.T) {
	for name, c := range map[string]struct {
		// mayCut are the records a part may begin with.
		mayCut []int
		want   [][]string
	}{
		"nowhere":                       {nil, [][]string{{"1", "2", "3", "4", "5", "6", "7", "8"}}},
		"before it holds its size":      {[]int{2, 3}, [][]string{{"1", "2", "3", "4", "5", "6", "7", "8"}}},
		"at the first place after it":   {[]int{2, 6, 7}, [][]string{{"1", "2", "3", "4", "5"}, {"6", "7", "8"}}},
		"exactly where it is full":      {[]int{4, 5}, [][]string{{"1", "2", "3"}, {"4", "5", "6", "7", "8"}}},
		"every part counts from itself": {[]int{5, 6, 7, 8}, [][]string{{"1", "2", "3", "4"}, {"5", "6", "7"}, {"8"}}},
	} {
		target := filepath.Join(t.TempDir(), "bundle")
		w := mustCreate(t, target)
		if err := w.InParts(Messages, 3); err != nil {
			t.Fatal(err)
		}
		for n := 1; n <= 8; n++ {
			if slices.Contains(c.mayCut, n) {
				if err := w.MayCut(Messages); err != nil {
					t.Fatal(err)
				}
			}
			mustAdd(t, w, Messages, line(n))
		}
		if _, err := w.Commit(Manifest{}); err != nil {
			t.Fatal(err)
		}

		if len(c.want) == 1 {
			if got := idsIn(t, filepath.Join(target, "messages.jsonl")); !slices.Equal(got, c.want[0]) || w.Parts() != nil {
				t.Errorf("%s: the one file holds %v and the parts are %v", name, got, w.Parts())
			}
			continue
		}
		if len(w.Parts()[Messages]) != len(c.want) {
			t.Errorf("%s: the parts are %v, want %d of them", name, w.Parts(), len(c.want))
			continue
		}
		for i, part := range w.Parts()[Messages] {
			if got := idsIn(t, filepath.Join(target, part)); !slices.Equal(got, c.want[i]) {
				t.Errorf("%s: %s holds %v, want %v", name, part, got, c.want[i])
			}
		}
	}
}

func TestPartsAreForTheirOwnerOnlyAndAreBuiltWhereTheBundleIs(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	w := mustCreate(t, target)
	if err := w.InParts(Messages, 1); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, w, Messages, line(1))
	if err := w.MayCut(Messages); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, w, Messages, line(2))

	// Until the bundle is in place, nothing of it stands at the target.
	if _, err := os.Stat(target); err == nil {
		t.Error("a part was written to the target before the bundle was put in place")
	}
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"messages-0001.jsonl", "messages-0002.jsonl"} {
		info, err := os.Stat(filepath.Join(target, name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s has the mode %v, want 0600", name, info.Mode().Perm())
		}
	}
}

func TestARunInPartsThatIsGivenUpLeavesNothing(t *testing.T) {
	parent := t.TempDir()
	w := mustCreate(t, filepath.Join(parent, "bundle"))
	if err := w.InParts(Messages, 1); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, w, Messages, line(1))
	if err := w.MayCut(Messages); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, w, Messages, line(2))
	w.Abort()

	if got := entriesOf(t, parent); len(got) != 0 {
		t.Errorf("a run that was given up left %v", got)
	}
}

func TestABundleInPartsReplacesOneInOneFileAndTheOtherWayRound(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	writeParts(t, mustCreate(t, target), 3, 2)
	writeParts(t, mustCreate(t, target), 3, 2, 2)
	if got, want := entriesOf(t, target), []string{manifestFile, "messages-0001.jsonl", "messages-0002.jsonl"}; !slices.Equal(got, want) {
		t.Errorf("after the run in parts the bundle holds %v, want %v", got, want)
	}
	writeParts(t, mustCreate(t, target), 3, 2)
	if got, want := entriesOf(t, target), []string{manifestFile, "messages.jsonl"}; !slices.Equal(got, want) {
		t.Errorf("after the run in one file the bundle holds %v, want %v", got, want)
	}
}

func TestADiscardingWriterNamesTheSamePartsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	for _, groups := range [][]int{{4, 0, 1}, {3}, {0}} {
		written := mustCreate(t, filepath.Join(t.TempDir(), "bundle"))
		writeParts(t, written, 3, groups...)
		dry := Discard()
		writeParts(t, dry, 3, groups...)

		if !reflect.DeepEqual(dry.Parts(), written.Parts()) || !reflect.DeepEqual(dry.Counts(), written.Counts()) {
			t.Errorf("%v: a discarding writer names %v and counts %v, a writing one %v and %v", groups, dry.Parts(), dry.Counts(), written.Parts(), written.Counts())
		}
	}
	if got := entriesOf(t, dir); len(got) != 0 {
		t.Errorf("wrote %v", got)
	}
}

func TestPartsAreSaidBeforeTheFirstRecordAndHoldSomething(t *testing.T) {
	w := Discard()
	mustAdd(t, w, Messages, line(1))
	if err := w.InParts(Messages, 3); err == nil {
		t.Error("a kind that holds records already was put into parts")
	}
	if err := Discard().InParts(Messages, 0); err == nil {
		t.Error("parts of no record were taken")
	}
}

func TestMorePartsThanANameCanNumberEndTheRun(t *testing.T) {
	w := Discard()
	if err := w.InParts(Messages, 1); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= maxParts; n++ {
		mustAdd(t, w, Messages, line(n))
		if err := w.MayCut(Messages); err != nil {
			t.Fatal(err)
		}
	}
	if got := w.Parts()[Messages]; got[len(got)-1] != "messages-9999.jsonl" {
		t.Fatalf("the last part is %s", got[len(got)-1])
	}
	if err := w.Add(Messages, line(maxParts+1)); err == nil {
		t.Error("a part was begun whose number has five digits")
	}
	if w.Counts()[Messages] != maxParts {
		t.Errorf("%d messages are counted, want the %d that were taken", w.Counts()[Messages], maxParts)
	}
}
