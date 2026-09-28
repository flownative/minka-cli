package api

import (
	"context"
	"net/http"
	"net/url"
)

// CommentedKind says which kind of item an identifier names where the comments
// under it have an address of their own. Each kind has an address of its own,
// and the identifier of one is nothing at the address of another.
type CommentedKind string

const (
	CommentedDocument CommentedKind = "document"
	CommentedUpload   CommentedKind = "upload"
)

// Commented names one item the comments hang on: a document or an upload of a
// Docs & Files tool.
type Commented struct {
	Kind CommentedKind
	ID   string
}

func (c Commented) path() string {
	if c.Kind == CommentedUpload {
		return "/files-uploads/" + url.PathEscape(c.ID)
	}
	return "/documents/" + url.PathEscape(c.ID)
}

func (c *Client) WriteCommentOn(ctx context.Context, item Commented, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost, item.path()+"/comments", map[string]string{"body": body})
}

// EditCommentOn changes a comment of the person of the token.
func (c *Client) EditCommentOn(ctx context.Context, item Commented, commentID, body string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPatch, item.path()+"/comments/"+url.PathEscape(commentID), map[string]string{"body": body})
}

// DeleteCommentOn removes a comment of the person of the token. A removed
// comment leaves nothing behind, so removing it again answers 404.
func (c *Client) DeleteCommentOn(ctx context.Context, item Commented, commentID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, item.path()+"/comments/"+url.PathEscape(commentID), nil)
	return err
}

// ReactToCommentOn leaves a reaction on a comment, 201 for a new one and
// 200 where the same one of yours stood already.
func (c *Client) ReactToCommentOn(ctx context.Context, item Commented, commentID, content string) (Result[Comment], error) {
	return request[Comment](ctx, c, http.MethodPost,
		item.path()+"/comments/"+url.PathEscape(commentID)+"/reactions", map[string]string{"content": content})
}

// UnreactFromCommentOn takes a reaction off a comment, the way it comes off
// a comment on a card.
func (c *Client) UnreactFromCommentOn(ctx context.Context, item Commented, commentID, content string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete,
		item.path()+"/comments/"+url.PathEscape(commentID)+"/reactions?content="+url.QueryEscape(content), nil)
	return err
}
