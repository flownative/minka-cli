package bundle

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const keepingExtractor = "minka v1.2.3"

func mustKeep(t *testing.T, target, extractor string) *Writer {
	t.Helper()
	w := mustCreate(t, target)
	if err := w.Keep(extractor); err != nil {
		t.Fatal(err)
	}
	if err := w.ReadsFrom("4711"); err != nil {
		t.Fatal(err)
	}
	return w
}

// bring is a file a run fetched and keeps.
func bring(t *testing.T, w *Writer, sourceID, filename string, content []byte) StoredFile {
	t.Helper()
	stored, err := w.StoreFile(sourceID, filename, func(into io.Writer) error {
		_, err := into.Write(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := w.KeepFile(sourceID, filename, stored); err != nil {
		t.Fatal(err)
	}
	return stored
}

// kill leaves a run the way a process ends that no handler ran in: the kernel
// closes its files and drops its lock, and nothing else happens.
func kill(w *Writer) {
	w.kept.closeIndex()
	for _, file := range w.files {
		_ = file.Close()
	}
	_ = w.lock.file.Close()
	w.lock.file = nil
}

func expectTakenOver(t *testing.T, w *Writer, sourceID, filename string, want StoredFile) {
	t.Helper()
	stored, reused, err := w.ReuseFile(sourceID, filename)
	if err != nil || !reused || stored != want {
		t.Fatalf("taken over %v as %+v (%v), want %+v", reused, stored, err, want)
	}
	content, err := os.ReadFile(filepath.Join(w.staging, "files", sourceID, filename))
	if err != nil || int64(len(content)) != want.Bytes {
		t.Errorf("the file taken over holds %d bytes (%v)", len(content), err)
	}
}

func expectNotTakenOver(t *testing.T, w *Writer, sourceID, filename string) {
	t.Helper()
	if stored, reused, err := w.ReuseFile(sourceID, filename); err != nil || reused {
		t.Fatalf("taken over %v as %+v (%v)", reused, stored, err)
	}
}

func TestTheFilesOfARunThatIsGivenUpAreKeptForTheNextOne(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	w := mustKeep(t, target, keepingExtractor)
	plan := bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	// The file is kept by a second name, which costs no space twice.
	brought, _ := os.Stat(filepath.Join(w.staging, "files", "attachment-1", "plan.png"))
	keptFile, _ := os.Stat(filepath.Join(keptDir(target), "files", "attachment-1", "plan.png"))
	if brought == nil || keptFile == nil || !os.SameFile(brought, keptFile) {
		t.Errorf("the kept file is not the one brought")
	}
	w.Abort()

	kept := filepath.Join(parent, ".bundle.kept")
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundle.kept"}) {
		t.Errorf("beside the bundle: %v", got)
	}
	if w.Kept() != kept {
		t.Errorf("the writer says the files are kept at %q", w.Kept())
	}

	for path, want := range map[string]os.FileMode{
		kept: 0o700, filepath.Join(kept, "files"): 0o700, filepath.Join(kept, "files", "attachment-1"): 0o700,
		filepath.Join(kept, "files", "attachment-1", "plan.png"): 0o600,
		filepath.Join(kept, "kept.json"):                         0o600, filepath.Join(kept, "files.jsonl"): 0o600,
	} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
			t.Errorf("%s: %v, %v", path, info.Mode().Perm(), err)
		}
	}

	// Another run cannot touch them while one is at work.
	next := mustKeep(t, target, keepingExtractor)
	if _, err := Create(target, "basecamp"); !errors.Is(err, ErrLocked) {
		t.Errorf("a second run was let in: %v", err)
	}
	expectTakenOver(t, next, "attachment-1", "plan.png", plan)
	if err := next.KeepFile("attachment-1", "plan.png", plan); err != nil {
		t.Fatal(err)
	}
	if _, err := next.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}

	// A bundle in place holds what the run used, and the next run takes it over
	// from there.
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("beside the bundle once it is in place: %v", got)
	}
	if next.Kept() != "" {
		t.Errorf("the writer says files are kept at %q", next.Kept())
	}
	if content, _ := os.ReadFile(filepath.Join(target, "files", "attachment-1", "plan.png")); string(content) != "a plan" {
		t.Errorf("the bundle holds %q", content)
	}
}

// A run that is killed runs no handler. The kept files carry a name no tidying
// up takes for what a run was building, from before the first file came.
func TestTheFilesOfARunThatIsKilledAreKeptForTheNextOne(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	earlierBundle(t, target)
	w := mustKeep(t, target, keepingExtractor)
	plan := bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	staging := filepath.Base(w.staging)
	kill(w)
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundle.kept", ".bundle.lock", staging, "bundle"}) {
		t.Fatalf("a killed run left %v", got)
	}

	next := mustKeep(t, target, keepingExtractor)
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundle.kept", ".bundle.lock", filepath.Base(next.staging), "bundle"}) {
		t.Errorf("after tidying up: %v", got)
	}
	expectTakenOver(t, next, "attachment-1", "plan.png", plan)

	// A file taken over from there stands there already.
	if err := next.KeepFile("attachment-1", "plan.png", plan); err != nil {
		t.Fatal(err)
	}
	if index, _ := os.ReadFile(filepath.Join(keptDir(target), "files.jsonl")); strings.Count(string(index), "\n") != 1 {
		t.Errorf("the index holds %q", index)
	}
}

// The index vouches for a file once it is whole. A file it says nothing of, or
// one that is not what it says, is fetched again, and what the run then brings
// takes its place.
func TestOnlyAKeptFileThatIsWhatItsIndexSaysIsTakenOver(t *testing.T) {
	for name, spoil := range map[string]func(t *testing.T, kept string){
		"killed before its line was written": func(t *testing.T, kept string) {
			if err := os.WriteFile(filepath.Join(kept, "files.jsonl"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"cut short": func(t *testing.T, kept string) {
			if err := os.Truncate(filepath.Join(kept, "files", "attachment-1", "plan.png"), 3); err != nil {
				t.Fatal(err)
			}
		},
		"spoilt at the same size": func(t *testing.T, kept string) {
			if err := os.WriteFile(filepath.Join(kept, "files", "attachment-1", "plan.png"), []byte("a plot"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"half a line": func(t *testing.T, kept string) {
			index := filepath.Join(kept, "files.jsonl")
			line, _ := os.ReadFile(index)
			if err := os.WriteFile(index, line[:len(line)/2], 0o600); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "bundle")
			w := mustKeep(t, target, keepingExtractor)
			bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
			w.Abort()
			spoil(t, keptDir(target))

			w = mustKeep(t, target, keepingExtractor)
			expectNotTakenOver(t, w, "attachment-1", "plan.png")
			fetched := bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
			w.Abort()

			expectTakenOver(t, mustKeep(t, target, keepingExtractor), "attachment-1", "plan.png", fetched)
		})
	}
}

// A file taken over and then found to be of another size than the source gives
// is dropped and fetched anew, and the new one is what is kept.
func TestAKeptFileThatIsDroppedIsReplacedByTheOneFetchedInstead(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustKeep(t, target, keepingExtractor)
	bring(t, w, "upload-1", "plan.png", []byte("the plan before"))
	w.Abort()

	w = mustKeep(t, target, keepingExtractor)
	if _, reused, err := w.ReuseFile("upload-1", "plan.png"); err != nil || !reused {
		t.Fatalf("not taken over: %v", err)
	}
	if err := w.DropFile("upload-1"); err != nil {
		t.Fatal(err)
	}
	now := bring(t, w, "upload-1", "plan.png", []byte("the plan now"))
	w.Abort()

	expectTakenOver(t, mustKeep(t, target, keepingExtractor), "upload-1", "plan.png", now)
}

func TestAKeptFileIsCopiedWhereItCannotBeLinked(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	linkFile = func(string, string) error { return errors.New("links are not supported") }
	t.Cleanup(func() { linkFile = os.Link })

	w := mustKeep(t, target, keepingExtractor)
	plan := bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	w.Abort()
	expectTakenOver(t, mustKeep(t, target, keepingExtractor), "attachment-1", "plan.png", plan)
}

func TestKeptFilesOfAnotherKindOfRunAreRefused(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustKeep(t, target, keepingExtractor)
	bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	w.Abort()
	kept := keptDir(target)
	record, _ := os.ReadFile(filepath.Join(kept, "kept.json"))

	refused := func(t *testing.T, err error, says string) {
		t.Helper()
		if !errors.Is(err, ErrKeptForAnotherRun) || !strings.Contains(err.Error(), kept) || !strings.Contains(err.Error(), says) {
			t.Errorf("got %v, want the refusal saying %q", err, says)
		}
		if after, _ := os.ReadFile(filepath.Join(kept, "kept.json")); !bytes.Equal(after, record) {
			t.Errorf("the record was changed: %s", after)
		}
	}

	t.Run("another build", func(t *testing.T) {
		refused(t, mustCreate(t, target).Keep("minka v1.2.4"), `"minka v1.2.3" brought them`)
	})
	t.Run("another account", func(t *testing.T) {
		w := mustCreate(t, target)
		if err := w.Keep(keepingExtractor); err != nil {
			t.Fatal(err)
		}
		refused(t, w.ReadsFrom("4712"), "they came from basecamp 4711, this run reads basecamp 4712")
		expectNotTakenOver(t, w, "attachment-1", "plan.png")
	})
	t.Run("another source", func(t *testing.T) {
		w, err := Create(target, "trello")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(w.Abort)
		refused(t, w.Keep(keepingExtractor), `they came from "basecamp"`)
	})
	for name, spoil := range map[string]func(dir string) error{
		"a record nobody can read": func(dir string) error {
			return os.WriteFile(filepath.Join(dir, "kept.json"), []byte("{"), 0o600)
		},
		"files without a record": func(dir string) error { return os.Remove(filepath.Join(dir, "kept.json")) },
		"files without a record or an index": func(dir string) error {
			if err := os.Remove(filepath.Join(dir, "files.jsonl")); err != nil {
				return err
			}
			return os.Remove(filepath.Join(dir, "kept.json"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			other := filepath.Join(t.TempDir(), "bundle")
			w := mustKeep(t, other, keepingExtractor)
			bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
			w.Abort()
			if err := spoil(keptDir(other)); err != nil {
				t.Fatal(err)
			}
			if err := mustCreate(t, other).Keep(keepingExtractor); !errors.Is(err, ErrKeptForAnotherRun) {
				t.Errorf("got %v", err)
			}
		})
	}
	t.Run("a link", func(t *testing.T) {
		other := filepath.Join(t.TempDir(), "bundle")
		if err := os.Symlink(kept, keptDir(other)); err != nil {
			t.Fatal(err)
		}
		if err := mustCreate(t, other).Keep(keepingExtractor); !errors.Is(err, ErrKeptThroughALink) {
			t.Errorf("got %v", err)
		}
	})
}

// The account is known only once the first project is read. Files kept before
// it was known are none, and the place takes the account of the run that comes.
func TestAPlaceThatNamesNoAccountYetTakesTheOneOfTheRun(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)
	if err := w.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	w.Abort()

	if record, _ := readKeptRecord(keptDir(target)); record.SourceInstance != "" {
		t.Errorf("the place names account %q before any was known", record.SourceInstance)
	}

	mustKeep(t, target, keepingExtractor).Abort()
	w = mustCreate(t, target)
	if err := w.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	if err := w.ReadsFrom("4712"); !errors.Is(err, ErrKeptForAnotherRun) {
		t.Errorf("the account the place took was not held against the next run: %v", err)
	}
}

// Without Keep a run is the run it always was: kept files are neither taken
// over nor removed, however it ends.
func TestARunThatDoesNotKeepLeavesKeptFilesAlone(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	w := mustKeep(t, target, keepingExtractor)
	bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	w.Abort()
	before := filesUnder(t, keptDir(target))

	w = mustCreate(t, target)
	expectNotTakenOver(t, w, "attachment-1", "plan.png")
	if w.Kept() != "" {
		t.Errorf("a run that keeps nothing says files are kept at %q", w.Kept())
	}
	w.Abort()
	w = mustCreate(t, target)
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	if after := filesUnder(t, keptDir(target)); !reflect.DeepEqual(after, before) {
		t.Errorf("the kept files changed:\n%v\n%v", before, after)
	}
	if KeptBeside(target) != keptDir(target) {
		t.Errorf("KeptBeside says %q", KeptBeside(target))
	}
}

// A dry run refuses what the real run would refuse, and touches nothing.
func TestADiscardingWriterChecksKeptFilesAndKeepsNothing(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "bundle")
	dry, err := DiscardFor(target, "basecamp")
	if err != nil {
		t.Fatal(err)
	}
	if err := dry.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	if err := dry.ReadsFrom("4711"); err != nil {
		t.Fatal(err)
	}
	if got := entriesOf(t, parent); len(got) != 0 {
		t.Errorf("a dry run wrote %v", got)
	}

	w := mustKeep(t, target, keepingExtractor)
	bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	w.Abort()
	before := filesUnder(t, keptDir(target))

	dry, _ = DiscardFor(target, "basecamp")
	if err := dry.Keep("minka v1.2.4"); !errors.Is(err, ErrKeptForAnotherRun) {
		t.Errorf("another build: %v", err)
	}
	dry, _ = DiscardFor(target, "basecamp")
	if err := dry.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	if err := dry.ReadsFrom("4712"); !errors.Is(err, ErrKeptForAnotherRun) {
		t.Errorf("another account: %v", err)
	}
	if dry.Kept() != "" {
		t.Errorf("a dry run says it keeps files at %q", dry.Kept())
	}
	if after := filesUnder(t, keptDir(target)); !reflect.DeepEqual(after, before) {
		t.Errorf("a dry run changed the kept files")
	}
}

func TestKeptFilesThatCannotBeRemovedAreNamed(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustKeep(t, target, keepingExtractor)
	bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	removeAll = func(string) error { return errors.New("the file system gave way") }
	t.Cleanup(func() { removeAll = os.RemoveAll })

	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatalf("the run failed: %v", err)
	}
	if w.Kept() != keptDir(target) {
		t.Errorf("the writer says files are kept at %q", w.Kept())
	}
}

// filesUnder reads every file under a directory, by its path in it.
func filesUnder(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := os.ReadFile(path)
		relative, _ := filepath.Rel(dir, path)
		files[filepath.ToSlash(relative)] = string(content)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// A run that ends between making the place and writing its record leaves a
// place with nothing in it to vouch for, which the next run takes as new.
func TestAPlaceWithNeitherARecordNorAFileIsTakenAsNew(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	if err := os.Mkdir(keptDir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keptDir(target), "kept.json.new"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	w := mustKeep(t, target, keepingExtractor)
	plan := bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
	w.Abort()
	expectTakenOver(t, mustKeep(t, target, keepingExtractor), "attachment-1", "plan.png", plan)
}

// What a run reads and writes in the place has to lie in it.
func TestALinkInsideThePlaceOfKeptFilesIsRefused(t *testing.T) {
	for _, name := range []string{"kept.json", "files.jsonl", "files"} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "bundle")
			w := mustKeep(t, target, keepingExtractor)
			bring(t, w, "attachment-1", "plan.png", []byte("a plan"))
			w.Abort()

			path := filepath.Join(keptDir(target), name)
			elsewhere := filepath.Join(t.TempDir(), name)
			if err := os.Rename(path, elsewhere); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, path); err != nil {
				t.Fatal(err)
			}
			if err := mustCreate(t, target).Keep(keepingExtractor); !errors.Is(err, ErrKeptThroughALink) || !strings.Contains(err.Error(), path) {
				t.Errorf("got %v", err)
			}
			dry, _ := DiscardFor(target, "basecamp")
			if err := dry.Keep(keepingExtractor); !errors.Is(err, ErrKeptThroughALink) {
				t.Errorf("a dry run: %v", err)
			}
		})
	}
	target := filepath.Join(t.TempDir(), "bundle")
	if err := os.Symlink(t.TempDir(), keptDir(target)); err != nil {
		t.Fatal(err)
	}
	if KeptBeside(target) != "" {
		t.Errorf("a link is taken for kept files")
	}
}

func TestThePlaceOfKeptFilesIsMadeTheUsersAloneAgain(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	mustKeep(t, target, keepingExtractor).Abort()
	if err := os.Chmod(keptDir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	mustKeep(t, target, keepingExtractor).Abort()
	if info, err := os.Stat(keptDir(target)); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the place is %v (%v)", info.Mode().Perm(), err)
	}
}

// Every run adds to the index, so a file kept by a run before stays vouched for.
func TestTheIndexKeepsTheLinesOfEveryRun(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustKeep(t, target, keepingExtractor)
	plan := bring(t, w, "attachment-1", "plan.png", []byte("a plan of the first run"))
	w.Abort()
	w = mustKeep(t, target, keepingExtractor)
	sketch := bring(t, w, "attachment-2", "sketch.png", []byte("a sketch"))
	w.Abort()

	w = mustKeep(t, target, keepingExtractor)
	expectTakenOver(t, w, "attachment-1", "plan.png", plan)
	expectTakenOver(t, w, "attachment-2", "sketch.png", sketch)
}

// A dry run reads the place and changes nothing of it: not its mode, not its
// record, and it makes no index.
func TestADryRunChangesNothingOfThePlace(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)
	if err := w.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	w.Abort()
	dir := keptDir(target)
	if err := os.Remove(filepath.Join(dir, "files.jsonl")); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	before := filesUnder(t, dir)

	dry, err := DiscardFor(target, "basecamp")
	if err != nil {
		t.Fatal(err)
	}
	if err := dry.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	if err := dry.ReadsFrom("4711"); err != nil {
		t.Fatal(err)
	}
	if after := filesUnder(t, dir); !reflect.DeepEqual(after, before) {
		t.Errorf("a dry run changed the place:\n%v\n%v", before, after)
	}
	if info, _ := os.Stat(dir); info.Mode().Perm() != 0o750 {
		t.Errorf("a dry run made the place %v", info.Mode().Perm())
	}

	// Nor does it make a record in a place that has none.
	if err := os.Remove(filepath.Join(dir, "kept.json")); err != nil {
		t.Fatal(err)
	}
	dry, _ = DiscardFor(target, "basecamp")
	if err := dry.Keep(keepingExtractor); err != nil {
		t.Fatal(err)
	}
	if got := entriesOf(t, dir); len(got) != 0 {
		t.Errorf("a dry run wrote %v", got)
	}
}

// The record is never written through a link that a run before, or somebody
// else, left at the name it is written under first.
func TestTheRecordIsNotWrittenThroughALink(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	if err := os.Mkdir(keptDir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	elsewhere := filepath.Join(t.TempDir(), "somebody's file")
	if err := os.WriteFile(elsewhere, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(keptDir(target), "kept.json.new")); err != nil {
		t.Fatal(err)
	}
	mustKeep(t, target, keepingExtractor).Abort()
	if content, _ := os.ReadFile(elsewhere); string(content) != "untouched" {
		t.Errorf("the record was written through the link: %q", content)
	}
	record, err := readKeptRecord(keptDir(target))
	if err != nil || record.Extractor != keepingExtractor {
		t.Errorf("the record is %+v (%v)", record, err)
	}
	if info, err := os.Lstat(filepath.Join(keptDir(target), "kept.json")); err != nil || !info.Mode().IsRegular() {
		t.Errorf("kept.json: %v", err)
	}
}
