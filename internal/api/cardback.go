package api

import (
	"context"
	"net/http"
	"net/url"
)

// CardBackFile is a file on the back of a card as its newest version. A file
// is one name on the back: an upload under that name again becomes its next
// version and keeps the ID, and the earlier versions are neither listed nor
// served.
type CardBackFile struct {
	ID          string          `json:"id"`
	CardID      string          `json:"cardId"`
	Filename    string          `json:"filename"`
	Version     int             `json:"version"` // 1 for the first
	ContentType string          `json:"contentType"`
	ByteSize    int64           `json:"byteSize"`
	UploadedAt  string          `json:"uploadedAt"`
	UploadedBy  PersonReference `json:"uploadedBy"`
	Content     string          `json:"content"` // where the bytes are, absolute
	AppURL      string          `json:"appUrl"`  // the file on the card page, to link to from a comment
}

func cardBackPath(cardID string) string {
	return "/cards/" + url.PathEscape(cardID) + "/attachments"
}

// CardBackFiles lists the files on the back of a card by name.
func (c *Client) CardBackFiles(ctx context.Context, cardID string) (Result[[]CardBackFile], error) {
	return request[[]CardBackFile](ctx, c, http.MethodGet, cardBackPath(cardID), nil)
}

// CardBackFile reads one file on the back of a card.
func (c *Client) CardBackFile(ctx context.Context, cardID, fileID string) (Result[CardBackFile], error) {
	return request[CardBackFile](ctx, c, http.MethodGet, cardBackPath(cardID)+"/"+url.PathEscape(fileID), nil)
}

// AttachToCardBack puts an upload of the card's project on its back, one the
// person of the token stored and nothing holds yet. Under a name the back has
// already, it becomes that file's next version.
func (c *Client) AttachToCardBack(ctx context.Context, cardID, blobID string) (Result[CardBackFile], error) {
	return request[CardBackFile](ctx, c, http.MethodPost, cardBackPath(cardID), map[string]string{"upload": blobID})
}

// RemoveFromCardBack takes a file off the back of a card with every version,
// and Minka discards their uploads with it.
func (c *Client) RemoveFromCardBack(ctx context.Context, cardID, fileID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, cardBackPath(cardID)+"/"+url.PathEscape(fileID), nil)
	return err
}

// CardBackFileBytes opens the bytes of the newest version of a file on the
// back of a card. The caller closes them. As with FileBytes, the address is
// built here and never taken from the "content" of an answer.
func (c *Client) CardBackFileBytes(ctx context.Context, cardID, fileID string) (*FileBytes, error) {
	return c.fetchBytes(ctx, cardBackPath(cardID)+"/"+url.PathEscape(fileID)+"/content")
}
