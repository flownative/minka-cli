package fakebasecamp

import (
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/flownative/minka-cli/internal/basecamp"
)

// storedFile is a file the account can serve, whether it is embedded in a
// text or was dropped into a chat.
type storedFile struct {
	Key string
	// URLName is the name the addresses of the file carry.
	URLName string
	Bytes   int64
	NoSize  bool
	Gone    bool
}

func (u Upload) urlName() string {
	if u.URLName != "" {
		return u.URLName
	}
	return u.Filename
}

func (f File) urlName() string {
	if f.URLName != "" {
		return f.URLName
	}
	return f.Filename
}

// sizeWithoutSaying is how large a file is that Basecamp gives no size of.
const sizeWithoutSaying = 64

var storagePattern = regexp.MustCompile(`^https://storage\.3\.basecamp\.com/([0-9]+)/blobs/([^/]+)/download/([^/]+)$`)

// FileContent is what the download of a stored file answers: bytes made from
// its key, as many as asked for, and never held as a whole.
func FileContent(key string, size int64) io.Reader {
	seed := sha256.Sum256([]byte(key))
	return io.LimitReader(&pattern{block: seed[:]}, size)
}

type pattern struct {
	block []byte
	at    int
}

func (p *pattern) Read(into []byte) (int, error) {
	for i := range into {
		into[i] = p.block[p.at]
		p.at = (p.at + 1) % len(p.block)
	}
	return len(into), nil
}

// Download streams a stored file the way the Basecamp CLI does. It answers
// only the address a run has to build: the storage host, the account, the key
// and the name as the account escapes it in its own addresses. A file that is
// made to fail brings half of itself first.
func (a *Account) Download(ctx context.Context, address string, into io.Writer) error {
	a.Downloads = append(a.Downloads, address)
	notFound := &basecamp.Error{Kind: basecamp.NotFound, Message: "Basecamp: resource not found"}

	match := storagePattern.FindStringSubmatch(address)
	if match == nil || match[1] != a.ID {
		return notFound
	}
	file, known := a.storedFile(match[2])
	if !known || file.Gone || match[3] != url.PathEscape(file.URLName) {
		return notFound
	}
	if ctx.Err() != nil {
		return &basecamp.Error{Kind: basecamp.Interrupted, Message: "interrupted"}
	}
	if a.BeforeDownload != nil {
		a.BeforeDownload(file.Key)
	}

	size := file.Bytes
	if file.NoSize {
		size = sizeWithoutSaying
	}
	if served, replaced := a.ServeBytes[file.Key]; replaced {
		size = served
	}
	if err := a.FailFileOf[file.Key]; err == basecamp.ErrLooksLikeAFailure {
		// All of it comes, and the doubt with it.
		if _, copyErr := io.Copy(into, FileContent(file.Key, size)); copyErr != nil {
			return copyErr
		}
		return err
	} else if err != nil {
		if _, copyErr := io.Copy(into, FileContent(file.Key, size/2)); copyErr != nil {
			return copyErr
		}
		return err
	}
	_, err := io.Copy(into, FileContent(file.Key, size))
	return err
}

// storedFile finds a file by its key, wherever in the account it is.
func (a *Account) storedFile(key string) (storedFile, bool) {
	var found *storedFile
	listed := func(files []File) {
		for _, file := range files {
			if file.Key == key && found == nil {
				found = &storedFile{Key: file.Key, URLName: file.urlName(), Bytes: file.Bytes, NoSize: file.NoSize, Gone: file.Gone}
			}
		}
	}
	commented := func(comments []Comment) {
		for _, comment := range comments {
			listed(comment.Files)
		}
	}
	for p := range a.Projects {
		a.eachCard(&a.Projects[p], func(_ *List, _ *OnHold, card *Card) {
			listed(card.Files)
			commented(card.Comments)
		})
		for _, tool := range a.Projects[p].Dock {
			for _, post := range tool.Posts {
				listed(post.Files)
				commented(post.Comments)
			}
			for _, line := range tool.Lines {
				for _, upload := range line.Uploads {
					if upload.Key == key && found == nil {
						found = &storedFile{Key: upload.Key, URLName: upload.urlName(), Bytes: upload.Bytes, Gone: upload.Gone}
					}
				}
			}
			for _, question := range tool.Questions {
				for _, answer := range question.Answers {
					listed(answer.Files)
					commented(answer.Comments)
				}
			}
		}
		a.eachTodoListAndGroup(&a.Projects[p], func(list *TodoList) {
			listed(list.Files)
			commented(list.Comments)
		})
		a.eachTodo(&a.Projects[p], func(one *Todo) {
			listed(one.Files)
			commented(one.Comments)
		})
		a.eachForward(&a.Projects[p], func(forward *Forward) {
			listed(forward.Files)
			commented(forward.Comments)
			for _, reply := range forward.Replies {
				listed(reply.Files)
			}
		})
		a.eachVault(&a.Projects[p], func(in vault) {
			for _, document := range in.documents {
				listed(document.Files)
				commented(document.Comments)
			}
			for _, upload := range in.uploads {
				listed(upload.Files)
				commented(upload.Comments)
				if !upload.InTheCloud && found == nil && StoredKeyOfUpload(upload.ID) == key {
					found = &storedFile{Key: key, URLName: upload.urlName(), Bytes: upload.Bytes, NoSize: upload.NoSize, Gone: upload.Gone}
				}
			}
		})
	}
	if found == nil {
		return storedFile{}, false
	}
	return *found, true
}

// downloadURL is where the account says a stored file is downloaded: on the
// host of the API, which the Basecamp CLI takes no download from.
func (a *Account) downloadURL(key, filename string) string {
	return fmt.Sprintf("%sblobs/%s/download/%s", a.root(), key, url.PathEscape(filename))
}

// Embed is a file as Basecamp renders it into a text, with what somebody said
// about it. Host is the storage host its address names; Basecamp writes one of
// two.
type Embed struct {
	File         File
	Caption      string
	Presentation string
	Host         string
}

// StoredHTML is the embed as a stored text holds it: without a figure.
func (e Embed) StoredHTML() string {
	return e.open() + `</bc-attachment>`
}

// HTML is the embed as Basecamp renders it. The caption of the figure is made
// from the name and the size where nobody wrote one.
func (e Embed) HTML() string {
	caption := e.Caption
	if caption == "" {
		caption = fmt.Sprintf("%s %d KB", e.File.Filename, e.File.Bytes/1024)
	}
	return e.open() + `<figure><img src="https://preview.basecamp.test/` + html.EscapeString(e.File.Key) + `"><figcaption>` +
		html.EscapeString(caption) + `</figcaption></figure></bc-attachment>`
}

func (e Embed) open() string {
	host := e.Host
	if host == "" {
		host = "storage.3.basecamp.com"
	}
	address := fmt.Sprintf("https://%s/4242/blobs/%s/download/%s", host, e.File.Key, url.PathEscape(e.File.urlName()))
	var attributes strings.Builder
	fmt.Fprintf(&attributes, ` sgid="%s" content-type="%s" url="%s" href="%s" filename="%s" filesize="%d"`,
		e.File.SGID, html.EscapeString(e.File.ContentType), address, address, html.EscapeString(e.File.Filename), e.File.Bytes)
	if e.Caption != "" {
		fmt.Fprintf(&attributes, ` caption="%s"`, html.EscapeString(e.Caption))
	}
	if e.Presentation != "" {
		fmt.Fprintf(&attributes, ` presentation="%s"`, html.EscapeString(e.Presentation))
	}
	return `<bc-attachment` + attributes.String() + `>`
}

// FileKey is the key of a stored file the way Basecamp makes them: a UUID.
func FileKey(number int64) string {
	return fmt.Sprintf("5a1b2c3d-%04d-4000-8000-%012d", number%10000, number)
}

// LongCaption is a caption of more characters than a reference may carry.
var LongCaption = strings.Repeat("A caption that goes on. ", 50)

// LongName is a name of more characters than a file of a bundle may have.
var LongName = strings.Repeat("ä long name ", 30) + ".jpeg"

// fileProject is a project with every case of a file: captions and galleries,
// a file embedded twice in one text through both storage hosts, names a path
// cannot carry, a file without a type and one without a size, a file Basecamp
// no longer has, a file of two owners, and files dropped into a chat.
func fileProject(ada, grace Person, tool func(int64, string, string, string, bool, int, *Person) Tool) Project {
	at := func(day, hour int) string { return fmt.Sprintf("2023-05-%02dT%02d:00:00.000Z", day, hour) }
	file := func(id int64, name, contentType string, size int64) File {
		return File{ID: id, SGID: SGID(fmt.Sprintf("gid://bc3/Attachment/%d?expires_in", id)), Key: FileKey(id), Filename: name, ContentType: contentType, Bytes: size}
	}

	sunset := file(5101, "holiday photo.png", "image/png", 3000)
	beach := file(5102, "beach.png", "image/png", 2000)
	nested := file(5103, `reports/2023\q1.pdf`, "application/pdf", 1500)
	nested.URLName = "reports-2023-q1.pdf"
	broken := file(5104, "line\nbreak\x00.txt", "text/plain", 100)
	broken.URLName = "line\nbreak.txt"
	long := file(5105, LongName, "image/jpeg", 700)
	dots := file(5106, "..", "", 300)
	dots.URLName = "dots"
	wordy := file(5107, "wordy.png", "image/png", 400)
	gone := file(5108, "shredded.pdf", "application/pdf", 900)
	gone.Gone = true
	unsized := file(5109, "unsized.bin", "application/octet-stream", 0)
	unsized.NoSize = true
	waiting := file(4711, "waiting.png", "image/png", 600)
	minutes := file(5110, "minutes.pdf", "application/pdf", 1200)

	board := tool(9040, "kanban_board", "card_tables", "Files board", true, 1, &ada)
	board.CreatedAt, board.UpdatedAt = "2023-05-01T07:00:00.100Z", at(1, 8)
	board.Lists = []List{
		{ID: 9141, Type: Triage, Title: "Triage", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8), Cards: []Card{
			{
				ID: 9901, Title: "Pictures", Position: 1, Creator: &ada, CreatedAt: at(2, 8), UpdatedAt: at(2, 9),
				Files: []File{sunset, beach},
				// The stored note says how the first file is laid out; only the
				// rendered one carries the caption of the second.
				Content: `<div>` + Embed{File: sunset, Caption: `Sunset & "sea" <3`, Presentation: "gallery"}.StoredHTML() +
					Embed{File: beach, Presentation: "fullwidth"}.StoredHTML() +
					Embed{File: sunset, Host: "storage.app.basecamp.com"}.StoredHTML() + `</div>`,
				Description: `<div>` + Embed{File: sunset, Caption: `Sunset & "sea" <3`, Presentation: "gallery"}.HTML() +
					Embed{File: beach, Caption: "The beach", Presentation: "fullwidth"}.HTML() +
					Embed{File: sunset, Host: "storage.app.basecamp.com"}.HTML() + `</div>`,
				Comments: []Comment{
					{ID: 9911, Creator: &grace, CreatedAt: at(2, 10), UpdatedAt: at(2, 11), Files: []File{nested, broken},
						Content: `<div>Two more: ` + Embed{File: nested}.HTML() + Embed{File: broken}.HTML() + `</div>`},
				},
			},
			{
				ID: 9902, Title: "Odd files", Position: 2, Creator: &grace, CreatedAt: at(3, 8), UpdatedAt: at(3, 8),
				Files: []File{long, dots, wordy, gone, unsized, waiting},
				Content: `<div>` + Embed{File: long}.StoredHTML() + Embed{File: dots}.StoredHTML() +
					Embed{File: wordy, Caption: LongCaption}.StoredHTML() + Embed{File: gone}.StoredHTML() +
					Embed{File: unsized}.StoredHTML() + Embed{File: waiting}.StoredHTML() + `</div>`,
			},
		}},
		{ID: 9142, Type: NotNow, Title: "Not now", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8)},
		{ID: 9143, Type: Done, Title: "Done", Creator: &ada, CreatedAt: at(1, 8), UpdatedAt: at(1, 8)},
	}

	news := tool(9041, "message_board", "message_boards", "File news", true, 2, &ada)
	news.CreatedAt, news.UpdatedAt = at(1, 9), at(1, 9)
	news.Posts = []Post{
		{ID: 9921, Subject: "Minutes", Creator: &grace, CreatedAt: at(4, 8), UpdatedAt: at(4, 9), Files: []File{minutes, sunset},
			Content: `<div>Attached: ` + Embed{File: minutes, Presentation: "gallery"}.HTML() + ` and again ` + Embed{File: sunset}.HTML() + `</div>`},
	}

	chat := tool(9042, "chat", "chats", "File chat", true, 3, &ada)
	chat.CreatedAt, chat.UpdatedAt = at(1, 10), at(1, 10)
	chat.Lines = []Line{
		{ID: 9931, Type: UploadLine, Creator: &ada, CreatedAt: at(5, 8), UpdatedAt: at(5, 8), Uploads: []Upload{
			{Key: FileKey(6001), Filename: "scan 1.pdf", ContentType: "application/pdf", Bytes: 5000},
			{Key: FileKey(6002), Filename: "a/b.png", URLName: "a-b.png", ContentType: "image/png", Bytes: 250},
		}},
		{ID: 9932, Type: TextLine, Creator: &grace, CreatedAt: at(5, 9), UpdatedAt: at(5, 9), Content: `Thanks`},
	}

	return Project{
		ID: 7005, Name: "File Project", Status: "active",
		CreatedAt: "2023-05-01T07:00:00.000Z", UpdatedAt: "2023-05-05T10:00:00.000Z",
		Members: []int64{101, 102},
		Dock:    []Tool{board, news, chat},
	}
}
