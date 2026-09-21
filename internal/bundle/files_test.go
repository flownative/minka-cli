package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repeated is content of any size that is never held as a whole.
func repeated(size int64) io.Reader {
	return io.LimitReader(&cycle{}, size)
}

type cycle struct{ at byte }

func (c *cycle) Read(into []byte) (int, error) {
	for i := range into {
		into[i] = c.at
		c.at++
	}
	return len(into), nil
}

func digest(t *testing.T, content io.Reader) string {
	t.Helper()
	sum := sha256.New()
	if _, err := io.Copy(sum, content); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// bundleWithFile writes and commits a bundle that holds one file of a text,
// with the record an index carries for it.
func bundleWithFile(t *testing.T, target, sourceID, filename string, content []byte) StoredFile {
	t.Helper()
	w := mustCreate(t, target)
	stored, err := w.AddFile(sourceID, filename, content)
	if err != nil {
		t.Fatal(err)
	}
	record := Attachment{Record: Record{SourceID: sourceID}, Filename: filename, OwnerKind: OwnerCard, OwnerSourceID: "1"}
	record.Holds(stored)
	mustAdd(t, w, Attachments, record)
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	return stored
}

func TestAFileIsCountedAndDigestedWhileItIsWritten(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	w := mustCreate(t, target)

	const size = 5<<20 + 17
	stored, err := w.StoreFile("attachment-1", "big.bin", func(into io.Writer) error {
		_, err := io.Copy(into, repeated(size))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := (StoredFile{File: "attachment-1/big.bin", Bytes: size, SHA256: digest(t, repeated(size))}); stored != want {
		t.Errorf("got %+v, want %+v", stored, want)
	}
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filepath.Join(target, "files", "attachment-1", "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if got := digest(t, file); got != stored.SHA256 {
		t.Errorf("the file has the digest %s, its record says %s", got, stored.SHA256)
	}
	for path, want := range map[string]os.FileMode{"files": 0o700, "files/attachment-1": 0o700, "files/attachment-1/big.bin": 0o600} {
		info, err := os.Stat(filepath.Join(target, path))
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s has the permissions %v, %v; want %04o", path, info.Mode().Perm(), err, want)
		}
	}
}

func TestAFileThatBreaksOffLeavesNothingBehind(t *testing.T) {
	w := mustCreate(t, filepath.Join(t.TempDir(), "bundle"))
	broke := errors.New("the source broke off")

	_, err := w.StoreFile("attachment-1", "half.bin", func(into io.Writer) error {
		if _, err := into.Write([]byte("half of a file")); err != nil {
			return err
		}
		return broke
	})
	if err != broke {
		t.Errorf("got %v, want the failure as it is", err)
	}
	if _, err := os.Stat(filepath.Join(w.staging, "files", "attachment-1")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the half file or its directory stayed: %v", err)
	}

	// The place is free for the same attachment afterwards.
	if _, err := w.AddFile("attachment-1", "half.bin", []byte("whole")); err != nil {
		t.Errorf("a later file of the attachment: %v", err)
	}
	if err := w.DropFile("attachment-1"); err != nil {
		t.Fatal(err)
	}
	if got := entriesOf(t, filepath.Join(w.staging, "files")); len(got) != 0 {
		t.Errorf("a dropped file left %v", got)
	}
}

func TestOnlyAPlainNameNamesAFileOfABundle(t *testing.T) {
	w := mustCreate(t, filepath.Join(t.TempDir(), "bundle"))
	longest := strings.Repeat("ä", MaxNameLength)

	for _, name := range []string{"a/b.png", `a\b.png`, "a\x00b.png", ".", "..", "", longest + "ä", "bad\xffutf8"} {
		if PlainName(name) {
			t.Errorf("%q counts as a plain name", name)
		}
		_, err := w.AddFile("attachment-1", name, []byte("x"))
		if err == nil {
			t.Fatalf("%q was stored", name)
		}
		// What somebody called a file is theirs, and an error ends up in a log.
		if name != "" && name != "." && name != ".." && strings.Contains(err.Error(), name) {
			t.Errorf("the refusal quotes the name: %s", err)
		}
		if !strings.Contains(err.Error(), "attachment-1") {
			t.Errorf("the refusal does not name the attachment: %s", err)
		}
	}
	for _, name := range []string{longest, "plain name.png", ".hidden", "a..b", "ünï", "line\nbreak"} {
		if !PlainName(name) {
			t.Errorf("%q counts as no plain name", name)
		}
	}
	if _, err := w.AddFile("attachment-2", strings.Repeat("n", MaxNameLength), []byte("x")); err != nil {
		t.Errorf("a name of %d characters: %v", MaxNameLength, err)
	}
	// Where the file system refuses a name, the failure does not repeat it.
	// A name of as many characters as a bundle takes, each of four bytes, is
	// more than a file system stores.
	heavy := strings.Repeat("\U0001F4CE", MaxNameLength)
	if _, err := w.AddFile("attachment-3", heavy, []byte("x")); err == nil {
		t.Log("this file system stores a name of 1020 bytes, so its refusal is not seen")
	} else if strings.Contains(err.Error(), "\U0001F4CE") || !strings.Contains(err.Error(), "attachment-3") {
		t.Errorf("the failure of the file system quotes the name, or does not name the attachment: %s", err)
	}
}

// A file of the new bundle may be a link to the content of the old one, and
// writing into it would change the old bundle. So a file goes into a directory
// made anew for it, and one that stands already takes nothing.
func TestNothingIsWrittenIntoTheDirectoryOfAFileThatStands(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	content := []byte("what the run before brought")
	bundleWithFile(t, target, "attachment-1", "plan.png", content)

	w := mustCreate(t, target)
	if _, reused, err := w.ReuseFile("attachment-1", "plan.png"); err != nil || !reused {
		t.Fatalf("reused %v, %v", reused, err)
	}
	for name, store := range map[string]func() error{
		"a download": func() error {
			_, err := w.AddFile("attachment-1", "plan.png", []byte("x"))
			return err
		},
		"a second take-over": func() error {
			_, _, err := w.ReuseFile("attachment-1", "plan.png")
			return err
		},
	} {
		if err := store(); err == nil {
			t.Errorf("%s went into the place of a file that stands", name)
		}
	}
	for _, path := range []string{filepath.Join(target, "files", "attachment-1", "plan.png"), filepath.Join(w.staging, "files", "attachment-1", "plan.png")} {
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, content) {
			t.Errorf("%s holds %q, %v", path, got, err)
		}
	}
}

func TestAFileIsTakenOverFromTheBundleBeingReplaced(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	content := []byte("what the run before brought")
	before := bundleWithFile(t, target, "attachment-1", "plan.png", content)
	old := filepath.Join(target, "files", "attachment-1", "plan.png")
	oldInfo, err := os.Stat(old)
	if err != nil {
		t.Fatal(err)
	}

	w := mustCreate(t, target)
	stored, reused, err := w.ReuseFile("attachment-1", "plan.png")
	if err != nil || !reused || stored != before {
		t.Fatalf("got %+v, %v, %v; want %+v taken over", stored, reused, err, before)
	}
	linked, err := os.Stat(filepath.Join(w.staging, "files", "attachment-1", "plan.png"))
	if err != nil || !os.SameFile(oldInfo, linked) {
		t.Errorf("the file was not linked: %v", err)
	}

	// Removing the old bundle leaves the file of the new one whole.
	if _, err := w.Commit(Manifest{}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(old)
	if err != nil || !bytes.Equal(got, content) {
		t.Errorf("after the old bundle went, the file holds %q, %v", got, err)
	}
	if entries := entriesOf(t, filepath.Dir(target)); len(entries) != 1 {
		t.Errorf("next to the bundle: %v", entries)
	}
}

func TestOnlyAFileThatIsWhatItsRecordSaysIsTakenOver(t *testing.T) {
	content := []byte("what the run before brought")
	sum := sha256.Sum256(content)
	right := fmt.Sprintf(`{"sourceId":"attachment-1","file":"attachment-1/plan.png","bytes":%d,"sha256":"%s"}`, len(content), hex.EncodeToString(sum[:]))

	cases := map[string]struct {
		index  string
		tamper func(t *testing.T, file string)
		id     string
		name   string
	}{
		"other bytes of the same size": {index: right, tamper: func(t *testing.T, file string) {
			if err := os.WriteFile(file, bytes.ToUpper(content), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		"another size": {index: right, tamper: func(t *testing.T, file string) {
			if err := os.WriteFile(file, content[:5], 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		"no file": {index: right, tamper: func(t *testing.T, file string) {
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
		}},
		"a link instead of the file": {index: right, tamper: func(t *testing.T, file string) {
			elsewhere := filepath.Join(t.TempDir(), "elsewhere.png")
			if err := os.WriteFile(elsewhere, content, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, file); err != nil {
				t.Fatal(err)
			}
		}},
		"a link instead of its directory": {index: right, tamper: func(t *testing.T, file string) {
			elsewhere := filepath.Join(t.TempDir(), "elsewhere")
			if err := os.Rename(filepath.Dir(file), elsewhere); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(elsewhere, filepath.Dir(file)); err != nil {
				t.Fatal(err)
			}
		}},
		"a record that calls the file gone": {index: strings.Replace(right, "{", `{"gone":true,`, 1)},
		"a record without a digest":         {index: strings.Replace(right, `"sha256":"`+hex.EncodeToString(sum[:])+`"`, `"sha256":null`, 1)},
		"a record of another digest":        {index: strings.Replace(right, hex.EncodeToString(sum[:]), strings.Repeat("0", 64), 1)},
		"a record of another size":          {index: strings.Replace(right, fmt.Sprintf(`"bytes":%d`, len(content)), `"bytes":5`, 1)},
		"a record of a file elsewhere":      {index: strings.Replace(right, "attachment-1/plan.png", "../attachment-1/plan.png", 1)},
		"no record":                         {index: `{"sourceId":"attachment-2"}`},
		"an index nobody can read":          {index: "not a record"},
		"another name than the record has":  {index: right, name: "plan.jpg"},
		"another attachment":                {index: right, id: "attachment-2"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "bundle")
			bundleWithFile(t, target, "attachment-1", "plan.png", content)
			if err := os.WriteFile(filepath.Join(target, attachmentsFile), []byte(c.index+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if c.tamper != nil {
				c.tamper(t, filepath.Join(target, "files", "attachment-1", "plan.png"))
			}
			id, filename := "attachment-1", "plan.png"
			if c.id != "" {
				id = c.id
			}
			if c.name != "" {
				filename = c.name
			}

			w := mustCreate(t, target)
			stored, reused, err := w.ReuseFile(id, filename)
			if err != nil || reused || stored != (StoredFile{}) {
				t.Errorf("got %+v, %v, %v; want nothing taken over", stored, reused, err)
			}
			// The place stays free for the download that follows.
			if _, err := w.AddFile(id, filename, content); err != nil {
				t.Errorf("the download that follows: %v", err)
			}
		})
	}

	// The control: the same bundle, untouched, hands its file over.
	target := filepath.Join(t.TempDir(), "bundle")
	bundleWithFile(t, target, "attachment-1", "plan.png", content)
	if err := os.WriteFile(filepath.Join(target, attachmentsFile), []byte(right+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, reused, err := mustCreate(t, target).ReuseFile("attachment-1", "plan.png"); err != nil || !reused {
		t.Errorf("an untouched file: reused %v, %v", reused, err)
	}
}

// A link in the place of a file could lead anywhere, and linking it would put
// a link into the new bundle, even where what it leads to is the right file.
func TestALinkInThePlaceOfAFileIsNeverTakenOver(t *testing.T) {
	elsewhere := filepath.Join(t.TempDir(), "elsewhere.png")
	// As long as the path of the link, so that the size of the link is right.
	content := []byte(strings.Repeat("x", len(elsewhere)))
	if err := os.WriteFile(elsewhere, content, 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "bundle")
	bundleWithFile(t, target, "attachment-1", "plan.png", content)
	old := filepath.Join(target, "files", "attachment-1", "plan.png")
	if err := os.Remove(old); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, old); err != nil {
		t.Fatal(err)
	}

	if _, reused, err := mustCreate(t, target).ReuseFile("attachment-1", "plan.png"); err != nil || reused {
		t.Errorf("a link was taken over: %v, %v", reused, err)
	}
}

// The file is read once for its digest and once for the copy, and only what
// the copy holds counts.
func TestACopyThatIsNotWhatTheRecordSaysIsNoFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "bundle")
	content := []byte("what the run before brought")
	bundleWithFile(t, target, "attachment-1", "plan.png", content)
	linkFile = func(old, _ string) error {
		if err := os.WriteFile(old, bytes.ToUpper(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return errors.New("links are not supported")
	}
	t.Cleanup(func() { linkFile = os.Link })

	w := mustCreate(t, target)
	if _, reused, err := w.ReuseFile("attachment-1", "plan.png"); err != nil || reused {
		t.Errorf("a copy of other bytes was taken over: %v, %v", reused, err)
	}
	if _, err := w.AddFile("attachment-1", "plan.png", content); err != nil {
		t.Errorf("the download that follows: %v", err)
	}
}

func TestAFileIsCopiedWhereItCannotBeLinked(t *testing.T) {
	content := []byte("what the run before brought")
	for name, prepare := range map[string]func(t *testing.T, old string){
		"a file system without links": func(t *testing.T, _ string) {
			linkFile = func(string, string) error { return errors.New("links are not supported") }
			t.Cleanup(func() { linkFile = os.Link })
		},
		"a file others may read": func(t *testing.T, old string) {
			if err := os.Chmod(old, 0o644); err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			target := filepath.Join(t.TempDir(), "bundle")
			before := bundleWithFile(t, target, "attachment-1", "plan.png", content)
			old := filepath.Join(target, "files", "attachment-1", "plan.png")
			prepare(t, old)
			oldInfo, err := os.Stat(old)
			if err != nil {
				t.Fatal(err)
			}

			w := mustCreate(t, target)
			stored, reused, err := w.ReuseFile("attachment-1", "plan.png")
			if err != nil || !reused || stored != before {
				t.Fatalf("got %+v, %v, %v", stored, reused, err)
			}
			copied := filepath.Join(w.staging, "files", "attachment-1", "plan.png")
			info, err := os.Stat(copied)
			if err != nil || os.SameFile(oldInfo, info) || info.Mode().Perm() != 0o600 {
				t.Errorf("the copy: %v, the same file %v, permissions %v", err, os.SameFile(oldInfo, info), info.Mode().Perm())
			}
			if got, _ := os.ReadFile(copied); !bytes.Equal(got, content) {
				t.Errorf("the copy holds %q", got)
			}
		})
	}
}

// A file taken over shares its content with the old bundle. Whatever ends the
// run, the old bundle stays whole, and what a killed run left is tidied up
// without touching it.
func TestAFileTakenOverLeavesTheOldBundleWholeHoweverTheRunEnds(t *testing.T) {
	content := []byte("what the run before brought")
	whole := func(t *testing.T, target string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(target, "files", "attachment-1", "plan.png"))
		if err != nil || !bytes.Equal(got, content) {
			t.Errorf("the old bundle holds %q, %v", got, err)
		}
		if _, err := os.Stat(filepath.Join(target, manifestFile)); err != nil {
			t.Errorf("the old bundle lost its manifest: %v", err)
		}
		if got := entriesOf(t, filepath.Dir(target)); len(got) != 1 {
			t.Errorf("next to the bundle: %v", got)
		}
	}
	taken := func(t *testing.T, target string) *Writer {
		t.Helper()
		w, err := Create(target, "basecamp")
		if err != nil {
			t.Fatal(err)
		}
		if _, reused, err := w.ReuseFile("attachment-1", "plan.png"); err != nil || !reused {
			t.Fatalf("reused %v, %v", reused, err)
		}
		return w
	}
	// killed forgets a run the way the kernel does: the lock goes, the rest stays.
	killed := func(w *Writer) {
		w.lock.release()
		w.staging, w.created = "", ""
	}

	t.Run("given up", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "bundle")
		bundleWithFile(t, target, "attachment-1", "plan.png", content)
		taken(t, target).Abort()
		whole(t, target)
	})
	t.Run("killed while it was building", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "bundle")
		bundleWithFile(t, target, "attachment-1", "plan.png", content)
		killed(taken(t, target))

		next := taken(t, target)
		next.Abort()
		whole(t, target)
	})
	t.Run("killed after the old bundle was moved aside", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "bundle")
		bundleWithFile(t, target, "attachment-1", "plan.png", content)
		w := taken(t, target)
		if err := os.Rename(target, w.staging+previousSuffix); err != nil {
			t.Fatal(err)
		}
		killed(w)

		next := taken(t, target)
		next.Abort()
		whole(t, target)
	})
}

func TestADiscardingWriterTakesNothingOver(t *testing.T) {
	w := Discard()
	if stored, reused, err := w.ReuseFile("attachment-1", "plan.png"); err != nil || reused || stored != (StoredFile{}) {
		t.Errorf("got %+v, %v, %v", stored, reused, err)
	}
	if err := w.DropFile("attachment-1"); err != nil {
		t.Error(err)
	}
}
