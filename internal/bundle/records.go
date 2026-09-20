// Package bundle writes an import bundle: the directory of JSONL files and
// attached files that Minka's applier reads foreign history from.
package bundle

// Format is the version of the bundle format a manifest announces.
const Format = 1

// Kind names one sort of record. It is the name of the JSONL file and the key
// of the counts in the manifest.
type Kind string

const (
	People      Kind = "people"
	Projects    Kind = "projects"
	Tools       Kind = "tools"
	Categories  Kind = "categories"
	Columns     Kind = "columns"
	Cards       Kind = "cards"
	Steps       Kind = "steps"
	Posts       Kind = "posts"
	Messages    Kind = "messages"
	Comments    Kind = "comments"
	Reactions   Kind = "reactions"
	Attachments Kind = "attachments"
)

// Kinds lists every kind in the order of the format, which is the order a
// report shows them in.
var Kinds = []Kind{People, Projects, Tools, Categories, Columns, Cards, Steps, Posts, Messages, Comments, Reactions, Attachments}

const (
	ToolBoard = "board"
	ToolChat  = "chat"
	ToolPosts = "posts"

	OwnerPerson = "person"
)

// Record is what every record carries. CreatedBy is nil when the source does
// not say who created it.
type Record struct {
	SourceID  string  `json:"sourceId"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
	CreatedBy *string `json:"createdBy"`
}

type Person struct {
	Record
	Name    string  `json:"name"`
	Email   *string `json:"email"`
	Avatar  *string `json:"avatar"`
	Retired bool    `json:"retired"`
}

type Project struct {
	Record
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Members     []string `json:"members"`
}

type Tool struct {
	Record
	ProjectSourceID string `json:"projectSourceId"`
	Kind            string `json:"kind"`
	Name            string `json:"name"`
	Position        int    `json:"position"`
}

// Attachment describes a file of the bundle. File is relative to the
// directory files/ of the bundle.
type Attachment struct {
	Record
	Filename      string `json:"filename"`
	ContentType   string `json:"contentType"`
	Bytes         int64  `json:"bytes"`
	SHA256        string `json:"sha256"`
	File          string `json:"file"`
	OwnerKind     string `json:"ownerKind"`
	OwnerSourceID string `json:"ownerSourceId"`
}

// Manifest says where a bundle came from and how much it holds.
type Manifest struct {
	Format         int          `json:"format"`
	Source         string       `json:"source"`
	SourceInstance string       `json:"sourceInstance"`
	ExtractedAt    string       `json:"extractedAt"`
	Extractor      string       `json:"extractor"`
	Counts         map[Kind]int `json:"counts"`
}
