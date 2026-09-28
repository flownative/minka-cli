package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// The Posts tool of a project: posts that went out, the kinds they are filed
// under, their archive and the comments under them. A draft is at no address,
// its author included.

// PostsTool is one Posts tool of a project.
type PostsTool struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	AppURL string `json:"appUrl"`
}

// PostsToolReference names the tool a post stands in.
type PostsToolReference struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// PostCategory is one kind of post a project offers. Every Posts tool of the
// project shares the list.
type PostCategory struct {
	ID    string `json:"id"`
	Emoji string `json:"emoji"`
	Name  string `json:"name"`
}

// PostLine is one post as the list of its tool names it, without the body.
type PostLine struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Lead         string          `json:"lead"`       // the first words of the body without markup
	Category     *string         `json:"category"`   // emoji and name, nil for none
	CategoryID   *string         `json:"categoryId"` // nil for none
	Pinned       bool            `json:"pinned"`
	CommentCount int             `json:"commentCount"`
	CreatedAt    string          `json:"createdAt"`
	Creator      PersonReference `json:"creator"`
	AppURL       string          `json:"appUrl"`
}

// PostsPage is one page of the posts of a tool, the pinned ones first. Next
// is where the page after it starts, and nil on the last page.
type PostsPage struct {
	Posts []PostLine `json:"posts"`
	Next  *string    `json:"next"`
}

// ArchivedPost is one post in the archive of its tool.
type ArchivedPost struct {
	ID         string           `json:"id"`
	Title      string           `json:"title"`
	Category   *string          `json:"category"`
	ArchivedAt *string          `json:"archivedAt"` // nil for a post put away before Minka noted the moment
	ArchivedBy *PersonReference `json:"archivedBy"` // nil where nobody is known
	AppURL     string           `json:"appUrl"`
}

// Post is one post that went out, with everything said under it.
type Post struct {
	ID         string             `json:"id"`
	ProjectID  string             `json:"projectId"`
	ToolID     string             `json:"toolId"`
	Project    ProjectReference   `json:"project"`
	Tool       PostsToolReference `json:"tool"`
	Title      string             `json:"title"`
	Body       string             `json:"body"`       // the stored HTML
	BodySource string             `json:"bodySource"` // the body as the editor opens it
	// Attachments are the files hanging in the body, and in no comment.
	Attachments []Attachment `json:"attachments"`
	Category    *string      `json:"category"`
	CategoryID  *string      `json:"categoryId"`
	// Archived says the post lies in the archive of its tool: readable, and
	// closed to every write but going into the trash.
	Archived     bool   `json:"archived"`
	Pinned       bool   `json:"pinned"`
	CommentCount int    `json:"commentCount"`
	CreatedAt    string `json:"createdAt"`
	// EditedAt is the moment the post was last written to, and nil while it
	// stands as it was made. Going into the trash and coming back out of it
	// count as a write, so a post restored on the web carries one although
	// nobody changed it.
	EditedAt *string         `json:"editedAt"`
	Creator  PersonReference `json:"creator"`
	Comments []Comment       `json:"comments"` // oldest first
	AppURL   string          `json:"appUrl"`
}

// CreatePostInput sends a post out. Body is HTML and has to say something.
// Audience is required: "everybody", "chosen" with NotifyIDs, or "nobody".
type CreatePostInput struct {
	Title      string   `json:"title"`
	Body       string   `json:"body"`
	Audience   string   `json:"audience"`
	CategoryID string   `json:"categoryId,omitempty"`
	NotifyIDs  []string `json:"notifyIds,omitempty"`
}

// UpdatePostInput changes a post. What it leaves out stays as it is. The
// kind is sent only where it changes, and an empty CategoryID takes the kind
// off; Minka reads an empty one and null alike.
type UpdatePostInput struct {
	Title      *string `json:"title,omitempty"`
	Body       *string `json:"body,omitempty"`
	CategoryID *string `json:"categoryId,omitempty"`
}

func postPath(postID string) string {
	return "/posts/" + url.PathEscape(postID)
}

func postsToolPath(toolID string) string {
	return "/posts-tools/" + url.PathEscape(toolID)
}

// PostsTools lists the Posts tools of a project that are in use, in the order
// of the project page.
func (c *Client) PostsTools(ctx context.Context, projectID string) (Result[[]PostsTool], error) {
	return request[[]PostsTool](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/posts", nil)
}

// PostCategories lists the kinds of post of a project, in the order its
// members put them.
func (c *Client) PostCategories(ctx context.Context, projectID string) (Result[[]PostCategory], error) {
	return request[[]PostCategory](ctx, c, http.MethodGet, "/projects/"+url.PathEscape(projectID)+"/post-categories", nil)
}

// Posts reads one page of the posts of a tool. A page after the first starts
// at the Next of the page before.
func (c *Client) Posts(ctx context.Context, toolID string, limit int, before string) (Result[PostsPage], error) {
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	if before != "" {
		values.Set("before", before)
	}
	return request[PostsPage](ctx, c, http.MethodGet, postsToolPath(toolID)+"/posts?"+values.Encode(), nil)
}

// ArchivedPosts lists the archive of a tool, the one archived last first.
// Nothing of it is paged.
func (c *Client) ArchivedPosts(ctx context.Context, toolID string) (Result[[]ArchivedPost], error) {
	return request[[]ArchivedPost](ctx, c, http.MethodGet, postsToolPath(toolID)+"/archive", nil)
}

// Post reads one post, in the list or in the archive.
func (c *Client) Post(ctx context.Context, postID string) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodGet, postPath(postID), nil)
}

func (c *Client) CreatePost(ctx context.Context, toolID string, input CreatePostInput) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodPost, postsToolPath(toolID)+"/posts", input)
}

// UpdatePost writes over a post. It tells nobody.
func (c *Client) UpdatePost(ctx context.Context, postID string, input UpdatePostInput) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodPatch, postPath(postID), input)
}

// DeletePost puts a post into the trash, from the list or from the archive.
func (c *Client) DeletePost(ctx context.Context, postID string) error {
	_, err := request[struct{}](ctx, c, http.MethodDelete, postPath(postID), nil)
	return err
}

// PinPost puts a post at the top of its list; one pinned already stays where
// it is.
func (c *Client) PinPost(ctx context.Context, postID string) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodPost, postPath(postID)+"/pin", nil)
}

func (c *Client) UnpinPost(ctx context.Context, postID string) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodPost, postPath(postID)+"/unpin", nil)
}

func (c *Client) ArchivePost(ctx context.Context, postID string) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodPost, postPath(postID)+"/archive", nil)
}

func (c *Client) UnarchivePost(ctx context.Context, postID string) (Result[Post], error) {
	return request[Post](ctx, c, http.MethodPost, postPath(postID)+"/unarchive", nil)
}
