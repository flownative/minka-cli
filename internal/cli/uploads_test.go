package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flownative/minka-cli/internal/fakeapi"
)

// largePNG is a picture whose longer side is past what Minka shows a picture
// at, which is what gives a file a second rendering. Which of the two sides
// is the long one decides nothing.
func largePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// aPictureWithATallSide and aPictureWithAWideSide are past the edge on one
// side each; aSmallPicture is below it on both.
func aPictureWithAWideSide(t *testing.T) []byte { return largePNG(t, fakeapi.DisplayEdge+1, 40) }
func aPictureWithATallSide(t *testing.T) []byte { return largePNG(t, 40, fakeapi.DisplayEdge+1) }
func aSmallPicture(t *testing.T) []byte         { return largePNG(t, fakeapi.DisplayEdge, fakeapi.DisplayEdge) }

// uploaded puts a file into the project of the fixture and answers with what
// the upload said about it.
func (e *testEnv) uploaded(content []byte, name string) map[string]any {
	e.t.Helper()
	path := filepath.Join(e.t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		e.t.Fatal(err)
	}
	env, r := e.runJSON("", "uploads", "create", path, "--project", e.fixture.Project.ID)
	expectCode(e.t, r, ExitOK)
	return decode(e.t, env.Data)
}

func blobID(t *testing.T, upload map[string]any) string {
	t.Helper()
	id, ok := upload["id"].(string)
	if !ok {
		t.Fatalf("the upload has no id: %v", upload)
	}
	return id
}

// The file lands under the name it has in Minka, with the bytes that were put
// there, and the answer says where it went.
func TestAFileIsFetchedUnderTheNameItHasInMinka(t *testing.T) {
	e := newTestEnv(t)
	content := onePixelPNG()
	upload := e.uploaded(content, "plan.png")
	here := t.TempDir()
	t.Chdir(here)

	env, r := e.runJSON("", "uploads", "download", blobID(t, upload))
	expectCode(t, r, ExitOK)

	data := decode(t, env.Data)
	if data["variant"] != "original" || data["byteSize"] != float64(len(content)) {
		t.Errorf("data %s", env.Data)
	}
	if got, want := data["path"], filepath.Join(here, "plan.png"); got != want {
		t.Errorf("path %v, want %s", got, want)
	}
	// What the API says about the file goes through untouched, so a field it
	// gains reaches a caller without a change here.
	file, _ := json.Marshal(data["file"])
	for _, want := range []string{`"filename":"plan.png"`, `"isImage":true`, `"variants"`} {
		if !strings.Contains(string(file), want) {
			t.Errorf("the answer does not carry %s: %s", want, file)
		}
	}

	written, err := os.ReadFile(filepath.Join(here, "plan.png"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, content) {
		t.Errorf("%d bytes written, want the %d that were uploaded", len(written), len(content))
	}
}

// Only the renderings a file really has can be asked for. A small picture and
// anything that is no picture have the original alone, and a name the file
// does not carry is refused before a byte travels.
func TestOnlyARenderingTheFileHasIsFetched(t *testing.T) {
	e := newTestEnv(t)
	wide := e.uploaded(aPictureWithAWideSide(t), "zelt.png")
	small := e.uploaded(aSmallPicture(t), "punkt.png")
	t.Chdir(t.TempDir())

	env, r := e.runJSON("", "uploads", "download", blobID(t, wide), "--variant", "display", "-o", "thumbnail.png")
	expectCode(t, r, ExitOK)
	if decode(t, env.Data)["variant"] != "display" {
		t.Errorf("data %s", env.Data)
	}
	thumbnail, err := os.Stat("thumbnail.png")
	if err != nil {
		t.Fatal(err)
	}
	if thumbnail.Size() >= int64(len(aPictureWithAWideSide(t))) {
		t.Errorf("the rendering to show weighs %d, which is not less than the picture it was made from", thumbnail.Size())
	}

	requestsBefore := len(e.server.Requests())
	env, r = e.runJSON("", "uploads", "download", blobID(t, small), "--variant", "display")
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "original") || strings.Contains(env.Hint, "display") {
		t.Errorf("hint %q does not say which renderings there are", env.Hint)
	}
	if n := len(e.server.Requests()) - requestsBefore; n != 1 {
		t.Errorf("%d requests, want only the one that asked what the file is", n)
	}

	_, r = e.runJSON("", "uploads", "download", blobID(t, small), "--variant", "riesig")
	expectCode(t, r, ExitUsage)
}

// A download must not destroy what somebody has under that name.
func TestAFileThatIsThereAlreadyIsNotWrittenThrough(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded(onePixelPNG(), "plan.png")
	here := t.TempDir()
	t.Chdir(here)
	mine := []byte("something of my own")
	if err := os.WriteFile(filepath.Join(here, "plan.png"), mine, 0o600); err != nil {
		t.Fatal(err)
	}

	env, r := e.runJSON("", "uploads", "download", blobID(t, upload))
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "--force") {
		t.Errorf("hint %q does not say how to replace it", env.Hint)
	}
	kept, _ := os.ReadFile(filepath.Join(here, "plan.png"))
	if !bytes.Equal(kept, mine) {
		t.Fatalf("the file was written through: %q", kept)
	}

	_, r = e.runJSON("", "uploads", "download", blobID(t, upload), "--force")
	expectCode(t, r, ExitOK)
	replaced, _ := os.ReadFile(filepath.Join(here, "plan.png"))
	if !bytes.Equal(replaced, onePixelPNG()) {
		t.Error("--force did not replace the file")
	}
}

// A picture is measured by its longer side, whichever of the two that is, and
// one below the edge on both has the original alone.
func TestTheLongerSideDecidesWhetherAPictureHasASecondRendering(t *testing.T) {
	e := newTestEnv(t)
	t.Chdir(t.TempDir())

	for name, content := range map[string][]byte{
		"wide.png": aPictureWithAWideSide(t),
		"tall.png": aPictureWithATallSide(t),
	} {
		env, r := e.runJSON("", "uploads", "download", blobID(t, e.uploaded(content, name)), "-o", name+".out")
		expectCode(t, r, ExitOK)
		if got := variantNames(t, decode(t, env.Data)); strings.Join(got, ",") != "original,display" {
			t.Errorf("%s has the renderings %v", name, got)
		}
	}

	env, r := e.runJSON("", "uploads", "download", blobID(t, e.uploaded(aSmallPicture(t), "small.png")), "-o", "small.out")
	expectCode(t, r, ExitOK)
	if got := variantNames(t, decode(t, env.Data)); strings.Join(got, ",") != "original" {
		t.Errorf("a picture below the edge on both sides has the renderings %v", got)
	}
}

// The file comes out of a project only its members see, and it is written so
// that it stays that way — on the way over an older file as much as on the
// first one.
func TestAFetchedFileIsReadableByItsOwnerAlone(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded(onePixelPNG(), "plan.png")
	here := t.TempDir()
	t.Chdir(here)

	expectCode(t, e.run("", "uploads", "download", blobID(t, upload), "--json"), ExitOK)
	expectMode(t, filepath.Join(here, "plan.png"), 0o600)

	// A file that is already there is replaced by one of this command's own
	// making, so the mode of the old one decides nothing.
	loose := filepath.Join(here, "loose.png")
	if err := os.WriteFile(loose, []byte("anybody may read this"), 0o666); err != nil {
		t.Fatal(err)
	}
	expectCode(t, e.run("", "uploads", "download", blobID(t, upload), "-o", loose, "--force", "--json"), ExitOK)
	expectMode(t, loose, 0o600)
}

// The name of the file comes out of an answer, so the server has a say in
// which entry of the directory is written. A link under that name is taken
// away, never followed: with --force it would otherwise write wherever the
// link points, and outside the directory the command runs in.
func TestForceReplacesALinkInsteadOfWritingThroughIt(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded(onePixelPNG(), "plan.png")
	elsewhere := t.TempDir()
	theirs := filepath.Join(elsewhere, "somebody-elses.txt")
	mine := []byte("something of my own")
	if err := os.WriteFile(theirs, mine, 0o600); err != nil {
		t.Fatal(err)
	}
	here := t.TempDir()
	t.Chdir(here)
	if err := os.Symlink(theirs, filepath.Join(here, "plan.png")); err != nil {
		t.Fatal(err)
	}

	expectCode(t, e.run("", "uploads", "download", blobID(t, upload), "--force", "--json"), ExitOK)

	kept, err := os.ReadFile(theirs)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(kept, mine) {
		t.Fatalf("the file the link pointed at was written through: %q", kept)
	}
	written, err := os.Lstat(filepath.Join(here, "plan.png"))
	if err != nil {
		t.Fatal(err)
	}
	if written.Mode()&os.ModeSymlink != 0 {
		t.Fatal("the link is still a link, so the bytes went through it")
	}
	expectMode(t, filepath.Join(here, "plan.png"), 0o600)
}

// A directory is no file to write. Taking it away to make room would be the
// very destroying this command does not do, whatever --force says.
func TestADirectoryUnderTheNameIsRefusedAndNotRemoved(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded(onePixelPNG(), "plan.png")
	here := t.TempDir()
	t.Chdir(here)
	inTheWay := filepath.Join(here, "plan.png")
	if err := os.Mkdir(inTheWay, 0o700); err != nil {
		t.Fatal(err)
	}

	for _, force := range []bool{false, true} {
		args := []string{"uploads", "download", blobID(t, upload)}
		if force {
			args = append(args, "--force")
		}

		env, r := e.runJSON("", args...)
		expectCode(t, r, ExitUsage)
		if !strings.Contains(env.Error, "directory") {
			t.Errorf("--force %v: error %q does not say what stands in the way", force, env.Error)
		}
		entry, err := os.Lstat(inTheWay)
		if err != nil || !entry.IsDir() {
			t.Fatalf("--force %v: the directory is gone: %v", force, err)
		}
	}
}

func expectMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s is %04o, want %04o", path, got, want)
	}
}

// With -o - the bytes are the answer, so standard output carries them alone
// and the word about them goes the other way.
func TestTheBytesGoToStandardOutputWhenTheyAreAskedForThere(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded(onePixelPNG(), "plan.png")
	t.Chdir(t.TempDir())

	r := e.run("", "uploads", "download", blobID(t, upload), "-o", "-")
	expectCode(t, r, ExitOK)
	if r.stdout != string(onePixelPNG()) {
		t.Errorf("%d bytes on standard output, want the %d of the file", len(r.stdout), len(onePixelPNG()))
	}
	if !strings.Contains(r.stderr, "standard output") {
		t.Errorf("stderr %q says nothing about what happened", r.stderr)
	}

	env, r := e.runJSON("", "uploads", "download", blobID(t, upload), "-o", "-")
	expectCode(t, r, ExitUsage)
	if env.Code != CodeUsage {
		t.Errorf("code %q", env.Code)
	}
}

// A failure must not follow half a file into the same stream. Standard output
// is not a terminal here, which is where an envelope would otherwise go.
func TestAFailureBesideTheBytesGoesToStandardErrorAsWell(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded([]byte("a few words of text"), "notes.txt")
	t.Chdir(t.TempDir())
	e.server.SendsAFileShort = true

	r := e.run("", "uploads", "download", blobID(t, upload), "-o", "-")
	expectCode(t, r, ExitAPI)
	if strings.Contains(r.stdout, `"ok"`) || strings.Contains(r.stdout, "error") {
		t.Errorf("an envelope followed the bytes into standard output: %q", r.stdout)
	}
	if !strings.Contains(r.stderr, "Error:") {
		t.Errorf("stderr %q does not say that it went wrong", r.stderr)
	}
	if !strings.Contains(r.stderr, "standard output already") {
		t.Errorf("hint %q claims nothing was kept, but the bytes are out", r.stderr)
	}
}

func TestAFileOfSomebodyElseReadsLikeAnAddressThatNamesNothing(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded(onePixelPNG(), "plan.png")
	t.Chdir(t.TempDir())
	e.vars["MINKA_TOKEN"] = e.fixture.StrangerToken

	env, r := e.runJSON("", "uploads", "download", blobID(t, upload))
	expectCode(t, r, ExitNotFound)
	if env.Meta["apiCode"] != "notFound" {
		t.Errorf("meta %v", env.Meta)
	}
	if _, err := os.Stat("plan.png"); err == nil {
		t.Error("something was written all the same")
	}
}

// The name of a file comes out of an answer, and a name that would lead
// somewhere else is refused instead of being cut down to something that fits.
func TestANameThatIsNoNameHereIsRefused(t *testing.T) {
	e := newTestEnv(t)
	id := "0199f3a4-5b6c-4d7e-8f90-a1b2c3d4e5f6"
	e.server.Uploads = append(e.server.Uploads, &fakeapi.Upload{
		ID: id, Circle: e.fixture.Project.ID, Filename: "../elsewhere.png",
		ContentType: "image/png", Size: 3, Content: []byte("abc"),
	})
	here := t.TempDir()
	t.Chdir(here)

	env, r := e.runJSON("", "uploads", "download", id)
	if _, err := os.Stat(filepath.Join(filepath.Dir(here), "elsewhere.png")); err == nil {
		t.Fatal("a file was written outside the directory the command ran in")
	}
	expectCode(t, r, ExitUsage)
	if !strings.Contains(env.Hint, "-o") {
		t.Errorf("hint %q does not say how to name a target", env.Hint)
	}

	// Named a target of its own, the file is fetched all the same.
	_, r = e.runJSON("", "uploads", "download", id, "-o", "picture.png")
	expectCode(t, r, ExitOK)
}

// What the file says it weighs is what has to arrive; anything else is no
// file, and no stump of one is left behind under its name.
func TestBytesThatAreNotTheOnesAnnouncedLeaveNothingBehind(t *testing.T) {
	e := newTestEnv(t)
	upload := e.uploaded([]byte("a few words of text"), "notes.txt")
	here := t.TempDir()
	t.Chdir(here)
	e.server.SendsAFileShort = true

	env, r := e.runJSON("", "uploads", "download", blobID(t, upload))
	expectCode(t, r, ExitAPI)
	if env.Code != CodeAPI {
		t.Errorf("code %q", env.Code)
	}
	if _, err := os.Stat(filepath.Join(here, "notes.txt")); err == nil {
		t.Fatal("a file that is not what it says it is was kept")
	}
}

// A card names the files hanging in its description and every comment its
// own, so that reading a card is enough to fetch its picture.
func TestATextNamesTheFilesHangingInIt(t *testing.T) {
	e := newTestEnv(t)
	picture := e.uploaded(onePixelPNG(), "plan.png")
	document := e.uploaded([]byte("a few words"), "notes.txt")
	card := e.fixture.CardInTriage.ID

	expectCode(t, e.run(source(t, picture), "cards", "edit", card, "--description", "-", "--description-format", "html", "--json"), ExitOK)
	expectCode(t, e.run(source(t, document), "comments", "create", card, "-", "--format", "html", "--json"), ExitOK)

	env, r := e.runJSON("", "cards", "show", card)
	expectCode(t, r, ExitOK)
	shown := decode(t, env.Data)
	files := rows(t, shown["attachments"])
	if len(files) != 1 || files[0]["id"] != picture["id"] || files[0]["filename"] != "plan.png" || files[0]["isImage"] != true {
		t.Fatalf("the files of the card: %v", files)
	}
	if files[0]["byteSize"] != float64(len(onePixelPNG())) {
		t.Errorf("the file hanging in the text does not weigh what was uploaded: %v", files[0])
	}
	comments := rows(t, shown["comments"])
	inTheComment := rows(t, comments[len(comments)-1]["attachments"])
	if len(inTheComment) != 1 || inTheComment[0]["id"] != document["id"] {
		t.Fatalf("the files of the comment: %v", inTheComment)
	}

	r = e.run("", "cards", "show", card, "--styled")
	expectCode(t, r, ExitOK)
	for _, want := range []string{"## Files (1)", blobID(t, picture) + "  plan.png (image/png,", "Files: notes.txt (text/plain"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

// Which files hang in a text is asked of the files themselves, never of what
// the text carries: a handle issued for another circle names nothing.
func TestAHandleOfAnotherCircleNamesNoFile(t *testing.T) {
	e := newTestEnv(t)
	path := filepath.Join(t.TempDir(), "plan.png")
	if err := os.WriteFile(path, onePixelPNG(), 0o600); err != nil {
		t.Fatal(err)
	}
	env, r := e.runJSON("", "uploads", "create", path, "--conversation", e.fixture.Conversation.ID)
	expectCode(t, r, ExitOK)
	ofTheConversation := decode(t, env.Data)

	card := e.fixture.CardInTriage.ID
	expectCode(t, e.run(source(t, ofTheConversation), "cards", "edit", card, "--description", "-", "--description-format", "html", "--json"), ExitOK)

	env, r = e.runJSON("", "cards", "show", card)
	expectCode(t, r, ExitOK)
	if files := rows(t, decode(t, env.Data)["attachments"]); len(files) != 0 {
		t.Errorf("a handle of a conversation named a file of the card: %v", files)
	}
}

// A message names its files the same way, which is what a chat needs for the
// pictures somebody drops into it.
func TestAMessageNamesTheFilesHangingInIt(t *testing.T) {
	e := newTestEnv(t)
	picture := e.uploaded(onePixelPNG(), "plan.png")

	expectCode(t, e.run(source(t, picture), "messages", "create", e.fixture.Chat.ID, "-", "--format", "html", "--json"), ExitOK)

	env, r := e.runJSON("", "messages", "list", e.fixture.Chat.ID)
	expectCode(t, r, ExitOK)
	var messages []map[string]any
	if err := json.Unmarshal(env.Data, &messages); err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1]
	if files := rows(t, last["attachments"]); len(files) != 1 || files[0]["id"] != picture["id"] {
		t.Fatalf("the files of the message: %v", last["attachments"])
	}

	r = e.run("", "messages", "list", e.fixture.Chat.ID, "--styled")
	expectCode(t, r, ExitOK)
	if !strings.Contains(r.stdout, "Files: plan.png (image/png,") {
		t.Errorf("the message says nothing about its file:\n%s", r.stdout)
	}
}

// source is the element that hangs an uploaded file in a text.
func source(t *testing.T, upload map[string]any) string {
	t.Helper()
	markup, ok := upload["attachmentSource"].(string)
	if !ok {
		t.Fatalf("the upload carries no attachment: %v", upload)
	}
	return "<p>See this</p>" + markup
}

// variantNames are the renderings the answer of a download names on the file.
func variantNames(t *testing.T, data map[string]any) []string {
	t.Helper()
	file, ok := data["file"].(map[string]any)
	if !ok {
		t.Fatalf("the answer carries no file: %v", data)
	}
	var names []string
	for _, variant := range rows(t, file["variants"]) {
		name, _ := variant["name"].(string)
		names = append(names, name)
	}
	return names
}

func rows(t *testing.T, value any) []map[string]any {
	t.Helper()
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("not a list: %v", value)
	}
	var rows []map[string]any
	for _, one := range list {
		row, ok := one.(map[string]any)
		if !ok {
			t.Fatalf("not an object: %v", one)
		}
		rows = append(rows, row)
	}
	return rows
}
