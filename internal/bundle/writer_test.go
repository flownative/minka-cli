package bundle

import (
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
)

func mustCreate(t *testing.T, target string) *Writer {
	t.Helper()
	writer, err := Create(target, "basecamp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(writer.Abort)
	return writer
}

func mustAdd(t *testing.T, w *Writer, kind Kind, record any) {
	t.Helper()
	if err := w.Add(kind, record); err != nil {
		t.Fatal(err)
	}
}

// entriesOf lists a directory, so that a test sees what a run left behind.
func entriesOf(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestABundleHoldsALinePerRecordAndAManifestWithTheCounts(t *testing.T) {
	target := filepath.Join(t.TempDir(), "deeper", "bundle")
	w := mustCreate(t, target)

	ada := "101"
	mustAdd(t, w, People, Person{Record: Record{SourceID: "101", CreatedAt: "2020-01-01T08:00:00Z", UpdatedAt: "2024-01-01T08:00:00Z"}, Name: "Ada Example"})
	mustAdd(t, w, People, Person{Record: Record{SourceID: "102"}, Name: "Grace Sample", Avatar: &ada})
	mustAdd(t, w, Projects, Project{Record: Record{SourceID: "7001", CreatedBy: &ada}, Description: "<p>a &amp; b</p>", Members: []string{}})

	if _, err := w.Commit(Manifest{Source: "overruled", SourceInstance: "4242", ExtractedAt: "2026-01-01T00:00:00Z", Extractor: "minka test"}); err != nil {
		t.Fatal(err)
	}

	people, err := os.ReadFile(filepath.Join(target, "people.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"sourceId":"101","createdAt":"2020-01-01T08:00:00Z","updatedAt":"2024-01-01T08:00:00Z","createdBy":null,"name":"Ada Example","email":null,"avatar":null,"retired":false}` + "\n" +
		`{"sourceId":"102","createdAt":"","updatedAt":"","createdBy":null,"name":"Grace Sample","email":null,"avatar":"101","retired":false}` + "\n"
	if string(people) != want {
		t.Errorf("people.jsonl:\n%s\nwant:\n%s", people, want)
	}

	projects, _ := os.ReadFile(filepath.Join(target, "projects.jsonl"))
	if !strings.Contains(string(projects), `"description":"<p>a &amp; b</p>"`) || !strings.Contains(string(projects), `"members":[]`) {
		t.Errorf("projects.jsonl: %s", projects)
	}

	var manifest map[string]any
	content, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	wantManifest := map[string]any{
		"format": float64(1), "source": "basecamp", "sourceInstance": "4242", "extractedAt": "2026-01-01T00:00:00Z",
		"extractor": "minka test", "counts": map[string]any{"people": float64(2), "projects": float64(1)},
	}
	if !reflect.DeepEqual(manifest, wantManifest) {
		t.Errorf("manifest %v, want %v", manifest, wantManifest)
	}
}

func TestAFileIsStoredUnderItsAttachmentAndDescribed(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)

	content := []byte("a picture")
	stored, err := w.AddFile("avatar-101", "avatar.webp", content)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	if want := (StoredFile{File: "avatar-101/avatar.webp", Bytes: 9, SHA256: hex.EncodeToString(sum[:])}); stored != want {
		t.Errorf("got %+v, want %+v", stored, want)
	}
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(target, "files", "avatar-101", "avatar.webp"))
	if err != nil || string(got) != "a picture" {
		t.Errorf("the file holds %q, %v", got, err)
	}
}

func TestABundleIsOnlyForItsUserToRead(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{})
	if _, err := w.AddFile("avatar-1", "avatar.png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]os.FileMode{
		"": 0o700, "files": 0o700, "files/avatar-1": 0o700,
		"manifest.json": 0o600, "people.jsonl": 0o600, "files/avatar-1/avatar.png": 0o600,
	} {
		info, err := os.Stat(filepath.Join(target, path))
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%q has the permissions %04o, want %04o", path, got, want)
		}
	}
}

func TestAFileCannotBeStoredOutsideItsPlace(t *testing.T) {
	parent := t.TempDir()
	w := mustCreate(t, filepath.Join(parent, "bundle"))

	for _, name := range [][2]string{
		{"..", "avatar.png"}, {"avatar-1", "../../escaped.png"}, {"a/b", "avatar.png"}, {"avatar-1", `..\escaped.png`},
		{"", "avatar.png"}, {"avatar-1", ""}, {".", "avatar.png"}, {"avatar-1", ".."},
	} {
		if _, err := w.AddFile(name[0], name[1], []byte("x")); err == nil {
			t.Errorf("%q/%q was stored", name[0], name[1])
		}
	}
	w.Abort()
	if got := entriesOf(t, parent); len(got) != 0 {
		t.Errorf("left behind: %v", got)
	}
}

func TestADirectoryThatHoldsSomethingElseIsNotReplaced(t *testing.T) {
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Create(target, "basecamp")
	if !errors.Is(err, ErrNotABundle) {
		t.Fatalf("got %v, want the refusal", err)
	}
	if got := entriesOf(t, target); !reflect.DeepEqual(got, []string{"notes.txt"}) {
		t.Errorf("the directory now holds %v", got)
	}
	if got := entriesOf(t, filepath.Dir(target)); len(got) != 1 {
		t.Errorf("next to the directory: %v", got)
	}

	file := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(file, "basecamp"); err == nil {
		t.Error("a file was taken as the directory of a bundle")
	}
}

func TestSomethingPutThereDuringTheRunIsNotReplaced(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{})

	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Commit(Manifest{}); !errors.Is(err, ErrNotABundle) {
		t.Fatalf("got %v, want the refusal", err)
	}
	if got := entriesOf(t, target); !reflect.DeepEqual(got, []string{"notes.txt"}) {
		t.Errorf("the directory now holds %v", got)
	}
}

func TestAnEarlierBundleIsReplacedAsAWhole(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")

	first := mustCreate(t, target)
	mustAdd(t, first, People, Person{Name: "Ada Example"})
	mustAdd(t, first, Tools, Tool{Name: "Campfire"})
	if _, err := first.AddFile("avatar-1", "avatar.png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := first.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}

	second := mustCreate(t, target)
	mustAdd(t, second, People, Person{Name: "Grace Sample"})
	if _, err := second.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}

	if got := entriesOf(t, target); !reflect.DeepEqual(got, []string{"manifest.json", "people.jsonl"}) {
		t.Errorf("the bundle holds %v, want nothing of the run before", got)
	}
	people, _ := os.ReadFile(filepath.Join(target, "people.jsonl"))
	if strings.Contains(string(people), "Ada Example") || !strings.Contains(string(people), "Grace Sample") {
		t.Errorf("people.jsonl: %s", people)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("next to the bundle: %v", got)
	}
}

func TestAnEmptyDirectoryIsTaken(t *testing.T) {
	target := t.TempDir()
	w := mustCreate(t, target)
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	if got := entriesOf(t, target); !reflect.DeepEqual(got, []string{"manifest.json"}) {
		t.Errorf("the bundle holds %v", got)
	}
}

func TestARunThatIsGivenUpLeavesWhatWasThere(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")

	// Without an earlier bundle, nothing is there afterwards.
	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{Name: "Ada Example"})
	if got := entriesOf(t, parent); len(got) != 2 || got[0] != ".bundle.lock" || !strings.HasPrefix(got[1], ".bundle.staging-") {
		t.Fatalf("while a run goes on, the parent holds %v, want the lock and where the bundle is built", got)
	}
	w.Abort()
	if got := entriesOf(t, parent); len(got) != 0 {
		t.Errorf("after giving up: %v", got)
	}

	// An earlier bundle stays as it was.
	earlier := mustCreate(t, target)
	mustAdd(t, earlier, People, Person{Name: "Ada Example"})
	if _, err := earlier.Commit(Manifest{ExtractedAt: "earlier"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(target, "manifest.json"))

	failing := mustCreate(t, target)
	mustAdd(t, failing, People, Person{Name: "Grace Sample"})
	mustAdd(t, failing, Projects, Project{})
	failing.Abort()

	after, _ := os.ReadFile(filepath.Join(target, "manifest.json"))
	if string(before) != string(after) || len(before) == 0 {
		t.Errorf("the manifest changed:\n%s\n%s", before, after)
	}
	if got := entriesOf(t, target); !reflect.DeepEqual(got, []string{"manifest.json", "people.jsonl"}) {
		t.Errorf("the earlier bundle now holds %v", got)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("next to the bundle: %v", got)
	}
}

func TestADiscardingWriterCountsAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	w := Discard()
	mustAdd(t, w, People, Person{})
	mustAdd(t, w, People, Person{})
	mustAdd(t, w, Tools, Tool{})
	if _, err := w.AddFile("avatar-1", "avatar.png", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	w.Abort()

	if want := map[Kind]int{People: 2, Tools: 1}; !reflect.DeepEqual(w.Counts(), want) {
		t.Errorf("counts %v, want %v", w.Counts(), want)
	}
	if got := entriesOf(t, dir); len(got) != 0 {
		t.Errorf("wrote %v", got)
	}
}

func TestOnlyABundleOfTheSameSourceAndFormatIsReplaced(t *testing.T) {
	for name, manifest := range map[string]string{
		"the manifest of a web application": `{"name":"An app","start_url":"/","display":"standalone"}`,
		"a bundle of another source":        `{"format":1,"source":"jira","counts":{}}`,
		"a bundle of another format":        `{"format":2,"source":"basecamp","counts":{}}`,
		"a format that is no number":        `{"format":"1","source":"basecamp"}`,
		"no JSON":                           `format: 1`,
		"nothing":                           ``,
	} {
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			for file, content := range map[string]string{"manifest.json": manifest, "index.html": "<html>"} {
				if err := os.WriteFile(filepath.Join(target, file), []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := Create(target, "basecamp"); !errors.Is(err, ErrNotABundle) {
				t.Fatalf("got %v, want the refusal", err)
			}
			if err := Check(target, "basecamp"); !errors.Is(err, ErrNotABundle) {
				t.Fatalf("Check: got %v, want the refusal", err)
			}
			if got := entriesOf(t, target); !reflect.DeepEqual(got, []string{"index.html", "manifest.json"}) {
				t.Errorf("the directory now holds %v", got)
			}
		})
	}
}

// failRename makes the renames with these numbers fail, counted from now, and
// puts the real one back when the test is over.
func failRename(t *testing.T, failing ...int) {
	t.Helper()
	calls := 0
	renameDir = func(oldPath, newPath string) error {
		calls++
		if slices.Contains(failing, calls) {
			return errors.New("the file system gave way")
		}
		return os.Rename(oldPath, newPath)
	}
	t.Cleanup(func() { renameDir = os.Rename })
}

func earlierBundle(t *testing.T, target string) {
	t.Helper()
	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{Name: "Ada Example"})
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
}

func TestTheBundleOfTheRunBeforeComesBackWhenTheNewOneCannotTakeItsPlace(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	earlierBundle(t, target)

	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{Name: "Grace Sample"})
	failRename(t, 2)
	if _, err := w.Commit(Manifest{}); err == nil {
		t.Fatal("the run did not fail")
	}
	renameDir = os.Rename
	w.Abort()

	people, _ := os.ReadFile(filepath.Join(target, "people.jsonl"))
	if !strings.Contains(string(people), "Ada Example") {
		t.Errorf("the bundle of the run before is not back: %q", people)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("next to the bundle: %v", got)
	}
}

func TestAnOldBundleThatCannotBePutBackIsNamedInTheError(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	earlierBundle(t, target)

	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{Name: "Grace Sample"})
	failRename(t, 2, 3)
	_, err := w.Commit(Manifest{})
	if err == nil {
		t.Fatal("the run did not fail")
	}

	var aside string
	for _, name := range entriesOf(t, parent) {
		if strings.HasSuffix(name, ".previous") {
			aside = filepath.Join(parent, name)
		}
	}
	if aside == "" || !strings.Contains(err.Error(), aside) {
		t.Errorf("the error %q does not name where the old bundle lies (%q)", err, aside)
	}
	people, _ := os.ReadFile(filepath.Join(aside, "people.jsonl"))
	if !strings.Contains(string(people), "Ada Example") {
		t.Errorf("the old bundle is not there: %q", people)
	}
}

func TestAnOldBundleThatCannotBeRemovedDoesNotFailTheRun(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	earlierBundle(t, target)

	w := mustCreate(t, target)
	mustAdd(t, w, People, Person{Name: "Grace Sample"})
	removeAll = func(string) error { return errors.New("the file system gave way") }
	t.Cleanup(func() { removeAll = os.RemoveAll })

	leftover, err := w.Commit(Manifest{})
	if err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if info, statErr := os.Stat(leftover); leftover == "" || statErr != nil || !info.IsDir() {
		t.Errorf("leftover %q does not name where the old bundle lies", leftover)
	}
	people, _ := os.ReadFile(filepath.Join(target, "people.jsonl"))
	if !strings.Contains(string(people), "Grace Sample") {
		t.Errorf("the new bundle is not in place: %q", people)
	}
}

func TestWhatAKilledRunLeftBehindIsTidiedUp(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	write := func(dir, name string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "people.jsonl"), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Killed between moving the old bundle aside and putting the new one in
	// place: the old one comes back, what was being built goes.
	write(filepath.Join(parent, ".bundle.staging-111"), "half a run")
	write(filepath.Join(parent, ".bundle.staging-111.previous"), "the old bundle")
	write(filepath.Join(parent, ".bundles.staging-111"), "of another target")
	if err := os.WriteFile(filepath.Join(parent, ".bundle.staging-111.previous", "manifest.json"), []byte(`{"format":1,"source":"basecamp"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	w := mustCreate(t, target)
	people, _ := os.ReadFile(filepath.Join(target, "people.jsonl"))
	if string(people) != "the old bundle" {
		t.Errorf("the old bundle did not come back: %q", people)
	}
	w.Abort()
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundles.staging-111", "bundle"}) {
		t.Errorf("next to the bundle: %v", got)
	}

	// With a bundle in place, one that was moved aside is of no use any more.
	write(filepath.Join(parent, ".bundle.staging-222.previous"), "an older bundle")
	mustCreate(t, target).Abort()
	people, _ = os.ReadFile(filepath.Join(target, "people.jsonl"))
	if string(people) != "the old bundle" {
		t.Errorf("the bundle in place was replaced: %q", people)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundles.staging-111", "bundle"}) {
		t.Errorf("next to the bundle: %v", got)
	}
}

func TestARunThatIsGivenUpLeavesNoDirectoriesItMade(t *testing.T) {
	root := t.TempDir()
	kept := filepath.Join(root, "kept")
	if err := os.MkdirAll(kept, 0o755); err != nil {
		t.Fatal(err)
	}

	w := mustCreate(t, filepath.Join(kept, "made", "deeper", "bundle"))
	mustAdd(t, w, People, Person{})
	w.Abort()
	if got := entriesOf(t, kept); len(got) != 0 {
		t.Errorf("left behind: %v", got)
	}
	if got := entriesOf(t, root); !reflect.DeepEqual(got, []string{"kept"}) {
		t.Errorf("a directory that was there before is gone: %v", got)
	}

	// A directory somebody else put something into stays.
	w = mustCreate(t, filepath.Join(kept, "made", "deeper", "bundle"))
	if err := os.WriteFile(filepath.Join(kept, "made", "notes.txt"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	w.Abort()
	if got := entriesOf(t, filepath.Join(kept, "made")); !reflect.DeepEqual(got, []string{"notes.txt"}) {
		t.Errorf("in the directory somebody else used: %v", got)
	}
}

// leftover puts something next to a target that looks like what a killed run
// leaves behind.
func leftover(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "people.jsonl"), []byte("of somebody else"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestTheNameOfATargetIsNoPattern(t *testing.T) {
	root := t.TempDir()
	foreign := []string{
		leftover(t, filepath.Join(root, "a", ".bundle.staging-111")),
		leftover(t, filepath.Join(root, "a", ".bundle.staging-111.previous")),
		leftover(t, filepath.Join(root, ".bundle-prod.staging-222")),
		leftover(t, filepath.Join(root, ".bundle-prod.staging-222.previous")),
		leftover(t, filepath.Join(root, ".b.staging-333")),
		// Of the target "bundle.staging-7", whose name begins like a leftover of "bundle".
		leftover(t, filepath.Join(root, ".bundle.staging-7.staging-444")),
		leftover(t, filepath.Join(root, ".bundle.staging-")),
		leftover(t, filepath.Join(root, ".bundle.staging-x1")),
		leftover(t, filepath.Join(root, "my.bundle.staging-555")),
	}

	for _, target := range []string{
		filepath.Join(root, "[abc]", "bundle"),
		filepath.Join(root, "bundle*"),
		filepath.Join(root, "[a-z]"),
		filepath.Join(root, "bundle"),
		filepath.Join(root, "["),
		filepath.Join(root, "?", "bundle"),
	} {
		w, err := Create(target, "basecamp")
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		mustAdd(t, w, People, Person{Name: "Ada Example"})
		if _, err := w.Commit(Manifest{}); err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		people, _ := os.ReadFile(filepath.Join(target, "people.jsonl"))
		if !strings.Contains(string(people), "Ada Example") {
			t.Errorf("%s holds %q, want the bundle that was written", target, people)
		}
	}
	for _, dir := range foreign {
		if content, _ := os.ReadFile(filepath.Join(dir, "people.jsonl")); string(content) != "of somebody else" {
			t.Errorf("%s was touched", dir)
		}
	}
}

func TestOnlyRealDirectoriesAreTakenForLeftovers(t *testing.T) {
	parent := t.TempDir()
	elsewhere := leftover(t, filepath.Join(t.TempDir(), "kept"))
	if err := os.Symlink(elsewhere, filepath.Join(parent, ".bundle.staging-111")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(parent, ".bundle.staging-222.previous")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".bundle.staging-333"), []byte("a file"), 0o600); err != nil {
		t.Fatal(err)
	}

	mustCreate(t, filepath.Join(parent, "bundle")).Abort()

	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundle.staging-111", ".bundle.staging-222.previous", ".bundle.staging-333"}) {
		t.Errorf("next to the target: %v", got)
	}
	if content, _ := os.ReadFile(filepath.Join(elsewhere, "people.jsonl")); string(content) != "of somebody else" {
		t.Error("what a link leads to was touched")
	}
}

func TestOneRunAtATimeWorksOnATarget(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	earlierBundle(t, target)

	first := mustCreate(t, target)
	mustAdd(t, first, People, Person{Name: "Grace Sample"})

	_, err := Create(target, "basecamp")
	if !errors.Is(err, ErrLocked) || !strings.Contains(err.Error(), filepath.Join(parent, ".bundle.lock")) {
		t.Fatalf("got %v, want the refusal with the path of the lock", err)
	}
	if got := entriesOf(t, parent); len(got) != 3 {
		t.Fatalf("the second run touched what the first is building: %v", got)
	}
	// Another target in the same directory is free.
	mustCreate(t, filepath.Join(parent, "another")).Abort()

	if _, err := first.Commit(Manifest{}); err != nil {
		t.Fatalf("the first run failed after the second was refused: %v", err)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("after the run: %v", got)
	}

	// The lock is free again after a run that ended, was given up, or was
	// killed and left its file behind.
	mustCreate(t, target).Abort()
	if err := os.WriteFile(filepath.Join(parent, ".bundle.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	mustCreate(t, target).Abort()
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("after the runs: %v", got)
	}
}

func TestABundleIsNotPutWhereALinkStands(t *testing.T) {
	parent := t.TempDir()
	behind := filepath.Join(parent, "behind")
	earlierBundle(t, behind)
	link := filepath.Join(parent, "bundle")
	if err := os.Symlink(behind, link); err != nil {
		t.Fatal(err)
	}
	dangling := filepath.Join(parent, "dangling")
	if err := os.Symlink(filepath.Join(parent, "nowhere"), dangling); err != nil {
		t.Fatal(err)
	}

	for _, target := range []string{link, dangling} {
		if _, err := Create(target, "basecamp"); !errors.Is(err, ErrSymlink) {
			t.Errorf("%s: got %v, want the refusal", target, err)
		}
		if info, err := os.Lstat(target); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s is no link any more", target)
		}
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"behind", "bundle", "dangling"}) {
		t.Errorf("next to the link: %v", got)
	}
}

func TestTheRefusalNamesWhatIsInTheWay(t *testing.T) {
	target := t.TempDir()
	for _, name := range []string{".DS_Store", "a", "b", "c", "d"} {
		if err := os.WriteFile(filepath.Join(target, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	err := Check(target, "basecamp")
	if !errors.Is(err, ErrNotABundle) || !strings.Contains(err.Error(), ".DS_Store, a, b, and 2 more") {
		t.Errorf("got %v, want it to name the first entries", err)
	}
}

func TestARunThatCannotBeginLeavesNoDirectoriesItMade(t *testing.T) {
	root := t.TempDir()
	// A name of this length can be a directory, but not with what the lock
	// adds to it, so the run fails after it has made the way to the target.
	target := filepath.Join(root, "made", "deeper", strings.Repeat("n", 250))
	if _, err := Create(target, "basecamp"); err == nil {
		t.Skip("this file system takes longer names")
	}
	if got := entriesOf(t, root); len(got) != 0 {
		t.Errorf("left behind: %v", got)
	}
}

func TestARunThatCannotTidyUpOrBeginHoldsNoLockAfterwards(t *testing.T) {
	// What a killed run left cannot be removed.
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	leftover(t, filepath.Join(parent, ".bundle.staging-111"))
	removeAll = func(string) error { return errors.New("the file system gave way") }
	_, err := Create(target, "basecamp")
	removeAll = os.RemoveAll
	if err == nil {
		t.Fatal("the run began")
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundle.staging-111"}) {
		t.Errorf("after the run that could not tidy up: %v", got)
	}
	mustCreate(t, target).Abort()

	// The directory the bundle is built in cannot be made.
	root := t.TempDir()
	target = filepath.Join(root, "made", "deeper", "bundle")
	mkdirTemp = func(string, string) (string, error) { return "", errors.New("the file system gave way") }
	_, err = Create(target, "basecamp")
	mkdirTemp = os.MkdirTemp
	if err == nil {
		t.Fatal("the run began")
	}
	if got := entriesOf(t, root); len(got) != 0 {
		t.Errorf("after the run that could not begin: %v", got)
	}
	mustCreate(t, target).Abort()
}
