package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/flownative/minka-cli/internal/fakebasecamp"
)

// buildOf has the CLI be a build of a version for the length of a test.
func buildOf(t *testing.T, v string) {
	t.Helper()
	before := version
	version = v
	t.Cleanup(func() { version = before })
}

// failedRunThatKeeps is a run with --keep-files that is stopped in the middle
// of the files of project 7005, the way Ctrl-C stops one.
func failedRunThatKeeps(t *testing.T, e *importEnv, out string) envelope {
	t.Helper()
	stopped := fakebasecamp.FileKey(5102)
	e.account.FailFileOf[stopped] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}
	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	expectCode(t, r, ExitNetwork)
	delete(e.account.FailFileOf, stopped)
	return env
}

func TestARunWithKeepFilesThatFailsKeepsWhatItBroughtForTheNextOne(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	kept := filepath.Join(parent, ".bundle.kept")

	env := failedRunThatKeeps(t, e, out)
	if data := decode(t, env.Data); data["keptFiles"] != kept {
		t.Errorf("data %v does not say where the files are kept", data)
	}
	if !strings.Contains(env.Notice, kept) || !strings.Contains(env.Notice, "personal data") {
		t.Errorf("the notice does not say where the files lie and what they are: %q", env.Notice)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{".bundle.kept"}) {
		t.Fatalf("beside the bundle: %v", got)
	}
	if info, err := os.Stat(kept); err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("the kept files: %v, %v", info.Mode().Perm(), err)
	}
	brought := len(e.account.Downloads)

	// The next run fetches what the first did not bring, and nothing it did.
	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	reused, fetched := data["filesReused"].(float64), data["filesFetched"].(float64)
	// The file that is gone at Basecamp is asked for on every run.
	if reused < 1 || reused+fetched != 12 || len(e.account.Downloads)-brought != int(fetched)+1 {
		t.Errorf("reused %v, fetched %v, %d downloads", reused, fetched, len(e.account.Downloads)-brought)
	}
	if _, says := data["keptFiles"]; says {
		t.Errorf("data says files are kept at %v once the bundle is in place", data["keptFiles"])
	}
	if !strings.Contains(env.Notice, "the files kept for a run that fails are removed") || !strings.Contains(env.Notice, out) {
		t.Errorf("the notice does not say the bundle holds the files now: %q", env.Notice)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("beside the bundle: %v", got)
	}

	// A bundle the files were kept for is the bundle a run without the flag
	// writes.
	again := filepath.Join(t.TempDir(), "bundle")
	expectCode(t, e.run("import", "basecamp", "--project", "7005", "--out", again, "--json"), ExitOK)
	withKept, without := filesUnder(t, out), filesUnder(t, again)
	if !reflect.DeepEqual(withKept, without) {
		t.Error("the bundle of a run that took kept files over differs from one that fetched them")
	}
}

func TestKeptFilesAreNamedInTheTextOfAFailure(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	out := filepath.Join(t.TempDir(), "bundle")
	e.account.FailFileOf[fakebasecamp.FileKey(5102)] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}

	r := e.run("import", "basecamp", "--project", "7005", "--out", out, "--keep-files", "--styled")
	expectCode(t, r, ExitNetwork)
	kept := filepath.Join(filepath.Dir(out), ".bundle.kept")
	if !strings.Contains(r.stdout, "Kept files: "+kept) || !strings.Contains(r.stderr, "personal data") {
		t.Errorf("the text does not name the kept files:\n%s\n%s", r.stdout, r.stderr)
	}
}

// Without --keep-files a run is what it always was. Files a run with the flag
// kept are neither taken over nor removed, and the run says where they lie.
func TestARunWithoutKeepFilesLeavesKeptFilesAloneAndSaysSo(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	kept := filepath.Join(parent, ".bundle.kept")
	failedRunThatKeeps(t, e, out)
	before := filesUnder(t, kept)

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitOK)
	data := decode(t, env.Data)
	if data["filesReused"] != float64(0) || data["filesFetched"] != float64(12) || data["keptFiles"] != kept {
		t.Errorf("reused %v, fetched %v, kept files at %v", data["filesReused"], data["filesFetched"], data["keptFiles"])
	}
	if !strings.Contains(env.Notice, "neither took them over nor removed them") || !strings.Contains(env.Notice, kept) {
		t.Errorf("the notice does not name the kept files: %q", env.Notice)
	}
	if after := filesUnder(t, kept); !reflect.DeepEqual(after, before) {
		t.Error("a run without --keep-files changed the kept files")
	}
	r = e.run("import", "basecamp", "--project", "7005", "--out", out, "--styled")
	if !strings.Contains(r.stdout, "Kept files: "+kept) {
		t.Errorf("the text does not name the kept files:\n%s", r.stdout)
	}

	// A failure says it too.
	e.account.Fail["projects/7005.json"] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitNetwork)
	if decode(t, env.Data)["keptFiles"] != kept || !strings.Contains(env.Notice, kept) {
		t.Errorf("data %s, notice %q", env.Data, env.Notice)
	}
	if after := filesUnder(t, kept); !reflect.DeepEqual(after, before) {
		t.Error("a failed run without --keep-files changed the kept files")
	}
}

// Where nothing was kept, a run without the flag reads exactly as before.
func TestARunWithoutKeepFilesSaysNothingOfKeptFiles(t *testing.T) {
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitOK)
	if _, says := decode(t, env.Data)["keptFiles"]; says || strings.Contains(env.Notice, "kept") {
		t.Errorf("data %s, notice %q", env.Data, env.Notice)
	}
	e.account.Fail["projects/7005.json"] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitNetwork)
	if env.Data != nil || strings.Contains(env.Notice, "kept") {
		t.Errorf("data %s, notice %q", env.Data, env.Notice)
	}
	if got := entriesOf(t, parent); !reflect.DeepEqual(got, []string{"bundle"}) {
		t.Errorf("beside the bundle: %v", got)
	}
}

func TestKeepFilesNeedsABuildThatNamesItsCommit(t *testing.T) {
	for _, build := range []string{"dev", "v0.32.0-2-g4f14378-dirty", ""} {
		t.Run(build, func(t *testing.T) {
			buildOf(t, build)
			e := newImportEnv(t)
			parent := t.TempDir()

			env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", filepath.Join(parent, "bundle"), "--keep-files")
			expectCode(t, r, ExitUsage)
			if env.Code != CodeUsage || !strings.Contains(env.Error, "--keep-files needs a build") {
				t.Errorf("code %q, error %q", env.Code, env.Error)
			}
			if len(e.account.Requests) != 0 || len(entriesOf(t, parent)) != 0 {
				t.Errorf("Basecamp was asked for %v, and %v written", e.account.Requests, entriesOf(t, parent))
			}
		})
	}
	for _, build := range []string{"v0.32.0", "0.32.0", "v0.32.0-2-g4f14378", "4f14378"} {
		if !buildNamesItsCode(build) {
			t.Errorf("%q is refused", build)
		}
	}
}

func TestKeptFilesOfAnotherBuildOrAccountAreRefused(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	kept := filepath.Join(parent, ".bundle.kept")
	failedRunThatKeeps(t, e, out)
	before := filesUnder(t, kept)
	asked := len(e.account.Requests)

	buildOf(t, "v1.2.4")
	for _, args := range [][]string{{}, {"--dry-run"}} {
		env, r := e.runJSON(append([]string{"import", "basecamp", "--project", "7005", "--out", out, "--keep-files"}, args...)...)
		expectCode(t, r, ExitUsage)
		if env.Code != CodeUsage || !strings.Contains(env.Error, kept) || !strings.Contains(env.Error, `"minka v1.2.3" brought them`) || env.Hint == "" {
			t.Errorf("%v: code %q, error %q, hint %q", args, env.Code, env.Error, env.Hint)
		}
	}
	if len(e.account.Requests) != asked {
		t.Errorf("Basecamp was asked for %v", e.account.Requests[asked:])
	}

	// The account is known once the first project is read, before any file.
	buildOf(t, "v1.2.3")
	record := filepath.Join(kept, "kept.json")
	content, _ := os.ReadFile(record)
	if err := os.WriteFile(record, []byte(strings.Replace(string(content), `"sourceInstance": "`, `"sourceInstance": "9`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	before["kept.json"] = func() string { c, _ := os.ReadFile(record); return string(c) }()
	downloaded := len(e.account.Downloads)
	for _, args := range [][]string{{}, {"--dry-run"}} {
		env, r := e.runJSON(append([]string{"import", "basecamp", "--project", "7005", "--out", out, "--keep-files"}, args...)...)
		expectCode(t, r, ExitUsage)
		if env.Code != CodeUsage || !strings.Contains(env.Error, "this run reads basecamp") {
			t.Errorf("%v: code %q, error %q", args, env.Code, env.Error)
		}
	}
	if len(e.account.Downloads) != downloaded {
		t.Errorf("files were fetched: %v", e.account.Downloads[downloaded:])
	}
	if after := filesUnder(t, kept); !reflect.DeepEqual(after, before) {
		t.Error("a refused run changed the kept files")
	}
}

func TestADryRunWithKeepFilesKeepsNothing(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()

	out := filepath.Join(parent, "bundle")
	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files", "--dry-run")
	expectCode(t, r, ExitOK)
	if len(entriesOf(t, parent)) != 0 || strings.Contains(env.Notice, "kept") {
		t.Errorf("a dry run wrote %v, notice %q", entriesOf(t, parent), env.Notice)
	}

	// Of files kept before, it says where they lie, and asks for no flag it has.
	failedRunThatKeeps(t, e, out)
	kept := filepath.Join(parent, ".bundle.kept")
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files", "--dry-run")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["keptFiles"] != kept || !strings.Contains(env.Notice, "which a dry run neither takes over nor removes") ||
		strings.Contains(env.Notice, "run with --keep-files to take them over") {
		t.Errorf("data %v, notice %q", decode(t, env.Data)["keptFiles"], env.Notice)
	}
}

// entriesOf names what lies in a directory.
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

// A run that is refused before it could take kept files over says where they
// lie as well, and a failure whose data is the report keeps the report.
func TestEveryFailureNamesTheKeptFiles(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	kept := filepath.Join(parent, ".bundle.kept")
	failedRunThatKeeps(t, e, out)

	buildOf(t, "v1.2.4")
	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	expectCode(t, r, ExitUsage)
	if decode(t, env.Data)["keptFiles"] != kept || !strings.Contains(env.Notice, kept) {
		t.Errorf("a refusal of the kept files: data %s, notice %q", env.Data, env.Notice)
	}
	buildOf(t, "dev")
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	expectCode(t, r, ExitUsage)
	if decode(t, env.Data)["keptFiles"] != kept {
		t.Errorf("a refusal of the build: data %s", env.Data)
	}

	buildOf(t, "v1.2.3")
	other, err := bundle.Create(out, basecamp.SourceName)
	if err != nil {
		t.Fatal(err)
	}
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	other.Abort()
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig || decode(t, env.Data)["keptFiles"] != kept {
		t.Errorf("a refusal of the lock: code %q, data %s", env.Code, env.Data)
	}
	// The run that holds the lock may be using them, so nothing says to remove
	// them now.
	if !strings.Contains(env.Notice, "the run that holds the lock may be using") || strings.Contains(env.Notice, "or remove the directory") ||
		strings.Contains(env.Notice, "remove the directory once no run is to take them over") {
		t.Errorf("a refusal of the lock: notice %q", env.Notice)
	}

	e.account.FailFileOf[fakebasecamp.FileKey(4711)] = &basecamp.Error{Kind: basecamp.Network, Message: "the network failed during the download"}
	env, r = e.runJSON("import", "basecamp", "--project", "7005", "--out", out)
	expectCode(t, r, ExitNetwork)
	data := decode(t, env.Data)
	if data["keptFiles"] != kept || len(data["missingFiles"].([]any)) != 1 {
		t.Errorf("the report of a run that lost a file: kept files %v, missing %v", data["keptFiles"], data["missingFiles"])
	}
}

// The file an upload shows and the files it showed before are kept as every
// other file is.
func TestTheFilesOfUploadsAreKept(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	stopped := fakebasecamp.StoredKeyOfVersion(9294)
	e.account.FailFileOf[stopped] = &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}

	_, r := e.runJSON("import", "basecamp", "--project", "7011", "--out", out, "--upload-versions", "--keep-files")
	expectCode(t, r, ExitNetwork)
	index, _ := os.ReadFile(filepath.Join(parent, ".bundle.kept", "files.jsonl"))
	for _, want := range []string{`"sourceId":"upload-9277"`, `"sourceId":"upload-9277-version-9293"`} {
		if !strings.Contains(string(index), want) {
			t.Errorf("the index lacks %s:\n%s", want, index)
		}
	}
}

// Kept files that cannot be removed once the bundle is in place are named.
func TestKeptFilesThatStayAfterTheBundleIsInPlaceAreNamed(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root removes what nobody else can")
	}
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	kept := filepath.Join(parent, ".bundle.kept")
	stuck := filepath.Join(kept, "stuck")
	if err := os.MkdirAll(stuck, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stuck, "file"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "kept.json"), []byte(`{"source":"basecamp","sourceInstance":"","extractor":"minka v1.2.3"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(stuck, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(stuck, 0o700) })

	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["keptFiles"] != kept || !strings.Contains(env.Notice, "could not be removed and still lie at "+kept) {
		t.Errorf("data %v, notice %q", decode(t, env.Data)["keptFiles"], env.Notice)
	}
}

func TestALinkWhereKeptFilesLieHasAHintOfItsOwn(t *testing.T) {
	buildOf(t, "v1.2.3")
	e := newImportEnv(t)
	parent := t.TempDir()
	out := filepath.Join(parent, "bundle")
	if err := os.Symlink(t.TempDir(), filepath.Join(parent, ".bundle.kept")); err != nil {
		t.Fatal(err)
	}
	env, r := e.runJSON("import", "basecamp", "--project", "7005", "--out", out, "--keep-files")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeConfig || !strings.Contains(env.Hint, "Kept files are neither read nor written through a link") || strings.Contains(env.Hint, "--out") {
		t.Errorf("code %q, hint %q", env.Code, env.Hint)
	}
}
