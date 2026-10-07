package fakeapi

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// CardBackFile is a file on the back of a card. A file is one name: an upload
// under a name the back has already becomes its next version and keeps the
// identifier, and only the newest version is ever listed or served.
type CardBackFile struct {
	ID       string
	Filename string
	Versions []CardBackFileVersion // oldest first
}

// CardBackFileVersion is one upload a file on the back of a card was given.
type CardBackFileVersion struct {
	BlobID       string
	UploadedByID string
	UploadedAt   time.Time
}

func (f *CardBackFile) newest() CardBackFileVersion {
	return f.Versions[len(f.Versions)-1]
}

// playableTypes are the media types a browser plays where they stand, which
// keep their own type when their bytes are handed out, as pictures do.
var playableTypes = []string{
	"audio/mpeg", "audio/mp4", "audio/x-m4a", "audio/aac", "audio/ogg", "audio/wav", "audio/x-wav", "audio/webm", "audio/flac",
	"video/mp4", "video/x-m4v", "video/quicktime", "video/webm",
}

func (s *Server) registerCardBackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/cards/{cardId}/attachments", s.authenticated(s.cardBackFiles))
	mux.HandleFunc("POST /api/v1/cards/{cardId}/attachments", s.authenticated(s.attachToCardBack))
	mux.HandleFunc("GET /api/v1/cards/{cardId}/attachments/{fileId}", s.authenticated(s.cardBackFile))
	mux.HandleFunc("DELETE /api/v1/cards/{cardId}/attachments/{fileId}", s.authenticated(s.removeFromCardBack))
	mux.HandleFunc("GET /api/v1/cards/{cardId}/attachments/{fileId}/content", s.answeringInBytes(s.cardBackFileContent))
}

// cardOfTheBack is the card whose back the person may read and write: one on
// its board or in its archive, of a project the person is on the team of. A
// client of the project meets the card as missing, whatever the front shows.
func (s *Server) cardOfTheBack(id string, person *Person) (board *Board, card *Card, archived bool) {
	board, _, card, archived = s.cardToWrite(id, person)
	if card == nil || s.isAClient(person.ID) {
		return nil, nil, false
	}
	return board, card, archived
}

func (s *Server) cardBackFiles(w http.ResponseWriter, r *http.Request, person *Person) {
	board, card, _ := s.cardOfTheBack(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	// By name, as the server orders them: case folded first, then as written.
	files := slices.Clone(card.BackFiles)
	slices.SortStableFunc(files, func(a, b *CardBackFile) int {
		if order := strings.Compare(strings.ToLower(a.Filename), strings.ToLower(b.Filename)); order != 0 {
			return order
		}
		return strings.Compare(a.Filename, b.Filename)
	})
	list := []any{}
	for _, file := range files {
		list = append(list, s.cardBackFileResource(board, card, file))
	}
	writeJSON(w, http.StatusOK, list)
}

// attachToCardBack puts an upload on the back. The order is the server's: the
// card first, then the field, and then one write that takes the card and the
// upload together. Why that write took nothing is asked afterwards, the card
// before the upload, so a card that takes no changes is named as the reason
// whatever the upload is.
func (s *Server) attachToCardBack(w http.ResponseWriter, r *http.Request, person *Person) {
	board, card, archived := s.cardOfTheBack(r.PathValue("cardId"), person)
	if card == nil {
		notFound(w)
		return
	}
	fields, ok := readBody(w, r)
	if !ok {
		return
	}
	blobID, ok := stringField(w, fields, "upload")
	if !ok {
		return
	}
	if blobID == nil || !uuidPattern.MatchString(strings.ToLower(*blobID)) {
		unprocessable(w, "uploadMissing", `Putting a file on the back of a card needs the field "upload", the id of an upload into the card's project.`)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	upload := s.storedFileOf(board.ProjectID, strings.ToLower(*blobID))
	if upload == nil || upload.UploaderID != person.ID || s.fileIsInUse(upload) {
		unprocessable(w, "uploadNotAttachable", "The upload has to be one you stored in the project of this card, and nothing else may hold it yet.")
		return
	}

	version := CardBackFileVersion{BlobID: upload.ID, UploadedByID: person.ID, UploadedAt: s.now}
	for _, file := range card.BackFiles {
		if file.Filename == upload.Filename {
			file.Versions = append(file.Versions, version)
			writeJSON(w, http.StatusCreated, s.cardBackFileResource(board, card, file))
			return
		}
	}
	file := &CardBackFile{ID: newID(), Filename: upload.Filename, Versions: []CardBackFileVersion{version}}
	card.BackFiles = append(card.BackFiles, file)
	writeJSON(w, http.StatusCreated, s.cardBackFileResource(board, card, file))
}

func (s *Server) cardBackFile(w http.ResponseWriter, r *http.Request, person *Person) {
	board, card, _, file := s.fileOfTheBack(r, person)
	if file == nil {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, s.cardBackFileResource(board, card, file))
}

// removeFromCardBack takes the file with every version, and discards their
// uploads with it: a loose upload would show to its uploader.
func (s *Server) removeFromCardBack(w http.ResponseWriter, r *http.Request, person *Person) {
	board, card, archived, file := s.fileOfTheBack(r, person)
	if file == nil {
		notFound(w)
		return
	}
	if s.refusedAsAClosedCard(w, board, archived) {
		return
	}
	for _, version := range file.Versions {
		for _, upload := range s.Uploads {
			if upload.ID == version.BlobID {
				upload.Swept = true
			}
		}
	}
	card.BackFiles = slices.DeleteFunc(card.BackFiles, func(one *CardBackFile) bool { return one == file })
	w.WriteHeader(http.StatusNoContent)
}

// cardBackFileContent hands out the bytes of the newest version as stored. A
// picture or playable media keeps its own type, and everything else, Markdown
// included, is an octet stream, so that nothing uploaded renders in Minka's
// origin.
func (s *Server) cardBackFileContent(w http.ResponseWriter, r *http.Request, person *Person) {
	_, _, _, file := s.fileOfTheBack(r, person)
	if file == nil {
		notFound(w)
		return
	}
	var upload *Upload
	for _, one := range s.Uploads {
		if one.ID == file.newest().BlobID && !one.Swept {
			upload = one
		}
	}
	if upload == nil {
		notFound(w)
		return
	}

	download := r.URL.Query().Get("download") == "1"
	if s.StoreURL != "" && !download && isPicture(upload.ContentType) && namesItsOwnType(upload) {
		w.Header().Set("Location", s.StoreURL+"/"+upload.ID+"/"+VariantOriginal)
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Vary", "Authorization")
		w.WriteHeader(http.StatusFound)
		return
	}

	content := upload.Content
	if s.SendsAFileShort && len(content) > 0 {
		content = content[:len(content)-1]
	}
	shownWhereItStands := !download && (isPicture(upload.ContentType) || slices.Contains(playableTypes, upload.ContentType))
	disposition, mediaType := "attachment", "application/octet-stream"
	if shownWhereItStands {
		disposition, mediaType = "inline", upload.ContentType
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, upload.Filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Header().Set("Cache-Control", "private, max-age=31536000")
	w.Header().Set("Vary", "Authorization")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(content)
	}
}

// fileOfTheBack is the file the address names on the back of the card it
// names, and nothing where either is not there for the person.
func (s *Server) fileOfTheBack(r *http.Request, person *Person) (board *Board, card *Card, archived bool, file *CardBackFile) {
	board, card, archived = s.cardOfTheBack(r.PathValue("cardId"), person)
	if card == nil {
		return nil, nil, false, nil
	}
	for _, file := range card.BackFiles {
		if file.ID == r.PathValue("fileId") {
			return board, card, archived, file
		}
	}
	return nil, nil, false, nil
}

func (s *Server) cardBackFileResource(board *Board, card *Card, file *CardBackFile) map[string]any {
	newest := file.newest()
	contentType, byteSize := "application/octet-stream", 0
	for _, upload := range s.Uploads {
		if upload.ID == newest.BlobID {
			contentType, byteSize = upload.ContentType, len(upload.Content)
		}
	}
	return map[string]any{
		"id": file.ID, "cardId": card.ID, "filename": file.Filename, "version": len(file.Versions),
		"contentType": contentType, "byteSize": byteSize,
		"uploadedAt": timestamp(newest.UploadedAt), "uploadedBy": s.personReference(newest.UploadedByID),
		"content": s.URL + "/api/v1/cards/" + card.ID + "/attachments/" + file.ID + "/content",
		"appUrl":  s.cardURL(board, card) + "/attachments/" + file.ID,
	}
}

// heldByACardBack says whether a version of a file on the back of any card
// holds the upload, a card in the archive or in the trash included.
func (s *Server) heldByACardBack(blobID string) bool {
	var cards []*Card
	for _, board := range s.Boards {
		for _, column := range board.Columns {
			cards = append(cards, column.Cards...)
		}
	}
	for _, archived := range s.Archive {
		cards = append(cards, archived.Card)
	}
	for _, trashed := range s.Trash {
		cards = append(cards, trashed.Card)
	}
	for _, card := range cards {
		for _, file := range card.BackFiles {
			for _, version := range file.Versions {
				if version.BlobID == blobID {
					return true
				}
			}
		}
	}
	return false
}
