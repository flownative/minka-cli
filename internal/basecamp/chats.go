package basecamp

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/flownative/minka-cli/internal/bundle"
)

// messagesPerPart is how many lines a part holds before it ends at the next
// place it may.
const messagesPerPart = 10000

// line is a line of a chat, which a bundle calls a message.
type line struct {
	recording
	ID     int64  `json:"id"`
	Status string `json:"status"`
	Type   string `json:"type"`
	// Content is missing on a line that is a file somebody dropped.
	Content     string   `json:"content"`
	Attachments []upload `json:"attachments"`
	BoostsCount int      `json:"boosts_count"`
}

// upload is a file of a line that is nothing else. Basecamp gives it no ID:
// the key of the stored file, which stands in its addresses, is all that
// names it. URL is where the file is previewed, which is never the file.
type upload struct {
	URL         string `json:"url"`
	DownloadURL string `json:"download_url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ByteSize    *int64 `json:"byte_size"`
}

const uploadLine = "Chat::Lines::Upload"

// lineTypes are the kinds of line a bundle has a word for, by the names a
// report counts them under. Basecamp hands out neither the lines somebody
// deleted nor the ones it wrote itself, such as who joined, and it grows kinds
// of its own that are in none of these: such a line is left out and reported,
// and whoever adds a word here has seen what Basecamp sends under it.
var lineTypes = map[string]string{
	"Chat::Lines::Text":        "text",
	"Chat::Lines::RichText":    "richText",
	uploadLine:                 "upload",
	"Chat::Lines::Integration": "integration",
}

// UnknownLineKind is a kind of line a bundle has no word for, with the chat it
// was met in and how many lines of it are left out there.
type UnknownLineKind struct {
	ChatSourceID string `json:"chatSourceId"`
	Type         string `json:"type"`
	LeftOut      int    `json:"leftOut"`
}

// extractChats adds the lines of every chat, chat by chat in the order of the
// projects and their tools, and within a chat oldest first.
//
// The lines are written in parts, numbered through the bundle. A part holds
// lines of one chat, and ends once it holds MessagesPerPart of them, but only
// where the second changes: the applier keeps the lines of one second in the
// order of their file by counting them, and begins to count anew in every
// part. So a part may hold somewhat more. A chat without lines has no part. A
// bundle whose lines came to a single part carries it as the one file of the
// kind, which the writer sees to.
func (e *Extractor) extractChats(ctx context.Context, w *bundle.Writer) error {
	size := e.MessagesPerPart
	if size == 0 {
		size = messagesPerPart
	}
	if err := w.InParts(bundle.Messages, size); err != nil {
		return err
	}
	for _, t := range e.talks {
		for _, chat := range t.chats {
			if err := e.extractChat(ctx, w, t.bucket, chat); err != nil {
				return err
			}
			if err := w.EndPart(bundle.Messages); err != nil {
				return err
			}
		}
	}
	return nil
}

// extractChat adds the lines of one chat and the boosts of each.
//
// Basecamp lists a chat newest first and cannot be asked to turn it round or
// to say how long it is, so a chat is read to its end and held as a whole, one
// chat at a time, before its first line is written. A line that arrives
// while the walk goes on moves every page, so a line met twice is written
// once.
//
// A line is held to what a bundle can carry before anything of the chat is
// written: a status other than active ends the run, because a line of a bundle
// has none, and a kind nobody knows leaves the line out.
func (e *Extractor) extractChat(ctx context.Context, w *bundle.Writer, bucket, chat int64) error {
	listed, err := getList[line](ctx, e.Source, fmt.Sprintf("buckets/%d/chats/%d/lines.json", bucket, chat))
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	lines := make([]line, 0, len(listed))
	for _, found := range listed {
		if seen[found.ID] {
			continue
		}
		seen[found.ID] = true
		if _, known := lineTypes[found.Type]; !known {
			e.leaveLineOut(chat, found.Type)
			continue
		}
		if found.Status != bundle.StatusActive {
			return &Error{Kind: API, Message: fmt.Sprintf("line %d of chat %d has the status %q, which a bundle cannot say of a line", found.ID, chat, found.Status)}
		}
		if err := requireTimestamps(fmt.Sprintf("line %d", found.ID), found.CreatedAt, found.UpdatedAt); err != nil {
			return err
		}
		lines = append(lines, found)
	}
	created, err := oldestFirstWithMoments(lines, "line", func(l line) (int64, string) { return l.ID, l.CreatedAt })
	if err != nil {
		return err
	}

	// The second the line before was written in: a moment, whatever the zone
	// and the fraction its time is written with.
	var second int64
	for _, found := range lines {
		body, files, err := e.lineBody(found)
		if err != nil {
			return err
		}
		// A line that says nothing is no line of a bundle: nobody is brought in
		// for it, its boosts are not asked for, and no part ends at it.
		if saysNothing(body) {
			if found.Type == uploadLine {
				e.uploadLinesLeftOut++
			} else {
				e.messagesWithEmptyBody++
			}
			continue
		}
		if at := created[found.ID].Unix(); at != second {
			if err := w.MayCut(bundle.Messages); err != nil {
				return err
			}
			second = at
		}
		origin, err := e.record("line", found.ID, found.recording)
		if err != nil {
			return err
		}
		err = w.Add(bundle.Messages, bundle.Message{
			Record:          origin,
			ProjectSourceID: sourceID(bucket),
			ToolSourceID:    sourceID(chat),
			Body:            body,
		})
		if err != nil {
			return err
		}
		e.measure(bundle.Messages, body)
		e.messagesByType[lineTypes[found.Type]]++
		if err := e.attachFiles(ctx, w, bundle.OwnerMessage, origin, files); err != nil {
			return err
		}
		if found.BoostsCount > 0 {
			if err := e.writeBoosts(ctx, w, bucket, bundle.TargetMessage, found.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// leaveLineOut notes a line a bundle has no word for, counted per chat and
// kind. Nobody comes into the bundle for such a line, its boosts are not asked
// for and no part ends at it. The kind is reported because it is the only thing
// that tells a sound effect from something that matters, and the chat because a
// number nobody can place is a number nobody can check.
func (e *Extractor) leaveLineOut(chat int64, kind string) {
	id := sourceID(chat)
	for i := range e.linesOfAnUnknownKind {
		if met := &e.linesOfAnUnknownKind[i]; met.ChatSourceID == id && met.Type == kind {
			met.LeftOut++
			return
		}
	}
	e.linesOfAnUnknownKind = append(e.linesOfAnUnknownKind, UnknownLineKind{ChatSourceID: id, Type: kind, LeftOut: 1})
}

// lineBody is the text of a line. What somebody wrote goes the way of every
// text, whatever wrote it down: a plain line holds no embed and passes as it
// is, however long it is.
//
// A line that is a file somebody dropped says nothing, so its body is a
// reference per file, in the order of the line. The file has no ID, so the
// reference names it by the key of the stored file, which stands in the address
// it is downloaded from, or else in the one it is previewed at. A file without
// such a key is dropped and counted, and the body of a line that keeps none
// is empty. Such a reference carries nothing besides: a file of a line has no
// caption.
//
// The files the body refers to come back with it, one per reference.
func (e *Extractor) lineBody(found line) (string, []sourceFile, error) {
	if found.Type != uploadLine {
		return e.rewriteBody(found.Content, "", nil)
	}
	var body strings.Builder
	var referred []sourceFile
	for _, file := range found.Attachments {
		key := blobKey(file.DownloadURL)
		if key == "" {
			key = blobKey(file.URL)
		}
		if !blobKeyPattern.MatchString(key) {
			e.bodies.FilesDropped++
			continue
		}
		e.bodies.Files++
		source := sourceFile{
			ref: "blob-" + key, key: key,
			filename: file.Filename, contentType: file.ContentType, byteSize: file.ByteSize, downloadURL: file.DownloadURL,
		}
		referred = append(referred, source)
		fmt.Fprintf(&body, `<import-attachment ref="%s"></import-attachment>`, source.ref)
	}
	return body.String(), referred, nil
}

// blobKeyPattern is what a key may be made of to become part of a reference,
// which becomes the ID of an attachment and part of a path.
var blobKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
