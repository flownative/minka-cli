package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/flownative/minka-cli/internal/markdown"
	"github.com/flownative/minka-cli/internal/richtext"
	"github.com/spf13/cobra"
)

// What Minka answers a post or a page of posts it will not take.
const (
	apiCodePostArchived         = "postArchived"
	apiCodePostNotArchived      = "postNotArchived"
	apiCodePostInTrash          = "postInTrash"
	apiCodePostCategoryUnknown  = "postCategoryUnknown"
	apiCodePostBodyMissing      = "postBodyMissing"
	apiCodePostTitleMissing     = "postTitleMissing"
	apiCodePostsLimitNotANumber = "postsLimitNotANumber"
	apiCodePostsBeforeUnknown   = "postsBeforeUnknown"
)

const postNotFoundHint = "There is no such post, it is a draft, it is in the trash, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you. A draft and a post in the trash are reached in Minka on the web only."

const postsToolNotFoundHint = "There is no such Posts tool, it is in the trash, or it belongs to a project you are not a member of. The tools of a project: minka posts tools <projectId>"

const postBodyHelp = `The body is Markdown by default and is sent as HTML, the way a description is.
Minka keeps headings, bold, italic, strikethrough, inline code, links, lists,
quotes, code blocks with their language, tables and horizontal rules. Images
become links and links Minka does not keep become plain text, each with a
warning. Use --body - to read it from stdin.

--body-format html sends HTML as it is. A body that carries no element Minka
serves is kept as the characters it consists of, so Markdown sent that way
stands in the post afterwards as "# Heading". The command says so in a notice,
and nothing else ever will.

A post that goes out says something: a body without a word and without a file
is refused with postBodyMissing.`

const postAudienceHelp = audienceHelp + `

Somebody named has to be able to see the post. A post written through the API
or on the web is shown to no client of the project, so naming a client, or an
agent working for one, is always refused with personCannotSeeIt. Nothing is
written then, and sending it again changes nothing: name somebody else.
"everybody" refuses nobody: it puts the clients of the project on the list of
the post, and they are not notified of it and cannot open it.

` + namedTwiceHelp

func (a *app) newPostsCmd() *cobra.Command {
	return group("posts", "Read and write the posts of a Posts tool, pin, archive and comment on them",
		a.newPostsToolsCmd(),
		a.newPostsCategoriesCmd(),
		a.newPostsListCmd(),
		a.newPostsShowCmd(),
		a.newPostsCreateCmd(),
		a.newPostsEditCmd(),
		a.newPostsPinCmd(),
		a.newPostsUnpinCmd(),
		a.newPostsArchiveCmd(),
		a.newPostsUnarchiveCmd(),
		a.newPostsDeleteCmd(),
		a.newPostCommentsCmd(),
	)
}

func (a *app) newPostsToolsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tools <projectId>",
		Short: "List the Posts tools of a project",
		Long: `Lists the Posts tools of a project that are in use, in the order of the project
page. A project can have more than one. A tool no longer in use is not listed
and is still read and written at its own ID. Its posts are read with
"posts list <toolId>".`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			tools, err := s.client.PostsTools(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    tools.Raw,
				summary: plural(len(tools.Value), "Posts tool", "Posts tools"),
				breadcrumbs: []Breadcrumb{
					{Action: "list", Cmd: "minka posts list <toolId>", Description: "List the posts of a tool"},
					{Action: "categories", Cmd: "minka posts categories " + args[0], Description: "List the kinds a post can be filed under"},
				},
				text: func(w io.Writer) { renderPostsTools(w, tools.Value) },
			})
		},
	}
}

func (a *app) newPostsCategoriesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "categories <projectId>",
		Short: "List the kinds a post of a project can be filed under",
		Long: `Lists the kinds of post a project offers, in the order its members put them.
Every Posts tool of the project shares the list. posts create --category and
posts edit --category take the ID of one. The list itself is written on the
web.`,
		Args: requireArgs("projectId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			categories, err := s.client.PostCategories(a.ctx, args[0])
			if err != nil {
				return s.fail(err)
			}

			return a.ok(success{
				data:    categories.Raw,
				summary: plural(len(categories.Value), "kind of post", "kinds of post"),
				text: func(w io.Writer) {
					if len(categories.Value) == 0 {
						fmt.Fprintln(w, "No kinds of post.")
						return
					}
					table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
					fmt.Fprintln(table, "ID\tKIND")
					for _, category := range categories.Value {
						fmt.Fprintf(table, "%s\t%s %s\n", category.ID, category.Emoji, category.Name)
					}
					_ = table.Flush()
				},
			})
		},
	}
}

func (a *app) newPostsListCmd() *cobra.Command {
	var limit int
	var before string
	var archived bool

	cmd := &cobra.Command{
		Use:   "list <toolId>",
		Short: "List the posts of a Posts tool, a page at a time, or its archive",
		Long: `Lists the posts of a tool that went out: the pinned ones first, the one pinned
last on top, then the newest. data is the page as Minka answers it: {posts,
next}. Drafts are not listed, the author's own included, and neither is a post
in the archive or in the trash.

next is where the page after this one starts, and null on the last page. Hand
it to --before exactly as it came; it is no ID. The next page starts behind
the place the last post stood at — pinned or not, and when it was written —
and not behind the post, so only that place is stable. A post you have not
read yet that gets pinned between two pages jumps above the place and is on no
later page: it can be missed. A post that was last on a page and gets unpinned
comes back on a later one.

--limit is the size of the page, 50 unless given. Minka serves at most 100 and
cuts a larger page down to that, which next then says.

--archived lists the archive of the tool instead, the one archived last first,
with when and by whom. It is not paged, so --limit and --before do not go with
it. A post in the archive is read with posts show and brought back with posts
unarchive.

Every post carries a lead: the first words of its body without markup.`,
		Example: `  minka posts list 0199... --json
  minka posts list 0199... --before MjAyNi0wOS0xMlQx... --json
  minka posts list 0199... --archived`,
		Args: requireArgs("toolId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if archived && (cmd.Flags().Changed("limit") || cmd.Flags().Changed("before")) {
				return usageErrorf(cmd, "--archived lists the whole archive and takes neither --limit nor --before")
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			if archived {
				posts, err := s.client.ArchivedPosts(a.ctx, args[0])
				if err != nil {
					return postsToolRefusal(s, err)
				}
				return a.ok(success{
					data:    posts.Raw,
					summary: plural(len(posts.Value), "post", "posts") + " in the archive",
					breadcrumbs: []Breadcrumb{
						{Action: "show", Cmd: "minka posts show <postId>", Description: "Read a post of the archive"},
						{Action: "unarchive", Cmd: "minka posts unarchive <postId>", Description: "Bring a post back among the posts"},
					},
					text: func(w io.Writer) { renderArchivedPosts(w, posts.Value) },
				})
			}

			page, err := s.client.Posts(a.ctx, args[0], limit, before)
			if err != nil {
				e := postsToolRefusal(s, err)
				switch e.APICode {
				case apiCodePostsLimitNotANumber:
					e.Hint = "--limit is a whole number of at least 1."
				case apiCodePostsBeforeUnknown:
					e.Hint = "--before takes the next of the page before exactly as it came, and nothing else. The first page: minka posts list " + args[0]
				}
				return e
			}

			summary := plural(len(page.Value.Posts), "post", "posts")
			var crumbs []Breadcrumb
			if page.Value.Next != nil {
				summary += ", and more on the next page"
				crumbs = append(crumbs, Breadcrumb{Action: "next", Cmd: "minka posts list " + args[0] + " --before " + *page.Value.Next, Description: "Read the next page"})
			}
			crumbs = append(crumbs,
				Breadcrumb{Action: "show", Cmd: "minka posts show <postId>", Description: "Read a post with its body and comments"},
				Breadcrumb{Action: "create", Cmd: "minka posts create --tool " + args[0] + " --title <title> --body - --audience nobody", Description: "Send a post out here"},
			)
			return a.ok(success{
				data:        page.Raw,
				summary:     summary,
				breadcrumbs: crumbs,
				text: func(w io.Writer) {
					renderPostLines(w, page.Value.Posts)
					if page.Value.Next != nil {
						fmt.Fprintf(w, "\nNext page: minka posts list %s --before %s\n", args[0], *page.Value.Next)
					}
				},
			})
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "Number of posts on the page")
	cmd.Flags().StringVar(&before, "before", "", "Where the page starts: next of the page before")
	cmd.Flags().BoolVar(&archived, "archived", false, "List the archive of the tool instead")

	return cmd
}

func (a *app) newPostsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <postId>",
		Short: "Show a post with its body and comments",
		Long: `Shows a post with its kind, its body and its comments. In text output the body
is converted to Markdown, which posts edit --body turns back into the same
HTML; in JSON it is the HTML the server stores.

A post in the archive is shown as well and says so: reading works, writing on
it is refused. A draft and a post in the trash are at no address, their author
included: they exit 2 (not_found).`,
		Args: requireArgs("postId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			post, err := s.client.Post(a.ctx, args[0])
			if err != nil {
				return postRefusal(s, err)
			}
			return a.ok(postSuccess(post, postSummary(post.Value), nil, func(w io.Writer) { renderPost(w, post.Value) }))
		},
	}
}

func (a *app) newPostsCreateCmd() *cobra.Command {
	var toolID, title, body, bodyFormat, categoryID, audience string
	var notify []string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Send a post out in a Posts tool",
		Long: "Sends a post out in a Posts tool. There are no drafts here: what this writes\n" +
			"has gone out, and it is shown to no client of the project.\n\n" +
			postAudienceHelp + "\n\n" +
			postBodyHelp + "\n\n" +
			`--category files the post under a kind of the project, by its ID: minka
posts categories <projectId> lists them.`,
		Example: `  minka posts create --tool 0199... --title "The tent is ordered" --body "It comes on Friday." --audience everybody --json
  minka posts create --tool 0199... --title "Setting up" --audience chosen --notify 0199... --body - < post.md`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireID(cmd, "--tool", toolID); err != nil {
				return err
			}
			if cmd.Flags().Changed("category") {
				if err := requireID(cmd, "--category", categoryID); err != nil {
					return err
				}
			}
			if err := checkBodyFormat(cmd, bodyFormat); err != nil {
				return err
			}
			if err := checkAudience(cmd, "post", audience); err != nil {
				return err
			}

			input := api.CreatePostInput{Title: title, Audience: audience, CategoryID: categoryID}
			var notices []string
			if cmd.Flags().Changed("body") {
				html, warnings, err := a.postBody(body, bodyFormat)
				if err != nil {
					return err
				}
				input.Body, notices = html, warnings
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if input.NotifyIDs, err = a.notifiedPeople(cmd, s, notify); err != nil {
				return err
			}

			post, err := s.client.CreatePost(a.ctx, toolID, input)
			if err != nil {
				e := postRefusal(s, err)
				if e.Code == CodeNotFound {
					e.Hint = postsToolNotFoundHint
				}
				return e
			}

			summary := "Sent post " + post.Value.Title + " out in " + post.Value.Tool.Title
			return a.ok(postSuccess(post, summary, notices, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\nPost: %s\n%s\n", summary, post.Value.ID, post.Value.AppURL)
			}))
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&toolID, "tool", "", "ID of the Posts tool the post goes into")
	flags.StringVar(&title, "title", "", "Title of the post")
	flags.StringVar(&body, "body", "", "Body of the post, - to read it from stdin")
	flags.StringVar(&bodyFormat, "body-format", "markdown", "Format of the body: markdown or html")
	flags.StringVar(&categoryID, "category", "", "ID of the kind the post is filed under")
	flags.StringVar(&audience, "audience", "", "Whom the post reaches: everybody, chosen or nobody")
	flags.StringArrayVar(&notify, "notify", nil, "Person to tell about the post, with --audience chosen; can be given more than once")

	return cmd
}

func (a *app) newPostsEditCmd() *cobra.Command {
	var title, body, bodyFormat, categoryID string
	var noCategory, dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <postId>",
		Short: "Change the title, the body or the kind of a post",
		Long: "Changes the title, the body, the kind or several of them. What is not given\n" +
			"stays as it is, and changing a post tells nobody. Anybody in the project may\n" +
			"change any post; the same text again succeeds and changes nothing.\n\n" +
			`--title "" does not empty the title: a post needs one, so an empty title exits
9 (validation) with meta.apiCode postTitleMissing. Neither does --body "": a
post that went out says something, and an empty body exits 9 with
postBodyMissing.

--category files the post under another kind, by its ID; --no-category takes
the kind off.` + "\n\n" +
			postBodyHelp + `

Files and mentions stand in a body as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource from posts show --json,
change it and send it back with --body-format html. A new body that would drop
them is refused unless --drop-attachments is given; the command reads the post
first to know.`,
		Example: `  minka posts edit 0199... --title "The tent comes on Saturday"
  minka posts edit 0199... --body - < post.md
  minka posts edit 0199... --no-category`,
		Args: requireArgs("postId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkBodyFormat(cmd, bodyFormat); err != nil {
				return err
			}
			if noCategory && cmd.Flags().Changed("category") {
				return usageErrorf(cmd, "--category and --no-category exclude each other")
			}

			var input api.UpdatePostInput
			var notices []string
			if cmd.Flags().Changed("title") {
				input.Title = &title
			}
			if cmd.Flags().Changed("category") {
				if err := requireID(cmd, "--category", categoryID); err != nil {
					return err
				}
				input.CategoryID = &categoryID
			}
			if noCategory {
				none := ""
				input.CategoryID = &none
			}
			if cmd.Flags().Changed("body") {
				html, warnings, err := a.postBody(body, bodyFormat)
				if err != nil {
					return err
				}
				input.Body, notices = &html, warnings
			}
			if input.Title == nil && input.Body == nil && input.CategoryID == nil {
				return usageErrorf(cmd, "nothing to change, give --title, --body, --category or --no-category")
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if input.Body != nil {
				current, err := s.client.Post(a.ctx, args[0])
				if err != nil {
					return postRefusal(s, err)
				}
				// A post in the archive takes no body at all, so Minka's refusal
				// is the answer and nothing can be lost.
				if !current.Value.Archived {
					dropped, err := a.guardOverFiles(s, current.Value.ProjectID, firstNonEmpty(current.Value.BodySource, current.Value.Body), *input.Body, dropAttachments,
						"the post has attachments or mentions, and the new body would drop them",
						"Change the bodySource from posts show --json and send it back with --body-format html, or give --drop-attachments.")
					if err != nil {
						return err
					}
					notices = append(notices, dropped...)
				}
			}

			post, err := s.client.UpdatePost(a.ctx, args[0], input)
			if err != nil {
				return postRefusal(s, err)
			}

			summary := "Changed post " + post.Value.Title
			return a.ok(postSuccess(post, summary, notices, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\n%s\n", summary, post.Value.AppURL)
			}))
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&title, "title", "", "New title; an empty one is refused, a title cannot be removed")
	flags.StringVar(&body, "body", "", "New body, - to read it from stdin")
	flags.StringVar(&bodyFormat, "body-format", "markdown", "Format of the body: markdown or html")
	flags.StringVar(&categoryID, "category", "", "ID of the kind to file the post under")
	flags.BoolVar(&noCategory, "no-category", false, "Take the kind off the post")
	flags.BoolVar(&dropAttachments, "drop-attachments", false, "Replace a body that has attachments or mentions, which drops them")

	return cmd
}

func (a *app) newPostsPinCmd() *cobra.Command {
	return a.newPostsWriteCmd("pin", "Pin a post to the top of its list",
		`Pins a post to the top of the list of its tool. Pinned posts stand above all
others, the one pinned last on top; there is no limit to how many. Pinning a
pinned post succeeds and leaves it where it is. A post in the archive keeps
its pin and is refused with postArchived.`,
		(*api.Client).PinPost, "Pinned post %s")
}

func (a *app) newPostsUnpinCmd() *cobra.Command {
	return a.newPostsWriteCmd("unpin", "Let a pinned post go back among the others",
		`Lets a pinned post go back among the others, where its moment decides its
place. A post that is not pinned succeeds all the same.`,
		(*api.Client).UnpinPost, "Unpinned post %s")
}

func (a *app) newPostsArchiveCmd() *cobra.Command {
	return a.newPostsWriteCmd("archive", "Put a post away into the archive of its tool",
		`Takes a post off the list of its tool into the archive, as putting it away on
the web does, and answers with the post, which now says archived: true. It is
counted nowhere, still read with posts show together with its comments, listed
by "posts list <toolId> --archived", and every write on it is refused with 9
(validation) and meta.apiCode postArchived until posts unarchive brings it
back. A pinned post keeps its pin and stands on top again when it comes back.

A post in the archive already exits 9 with postArchived, one in the trash with
postInTrash: the trash is not the archive.`,
		(*api.Client).ArchivePost, "Put post %s into the archive")
}

func (a *app) newPostsUnarchiveCmd() *cobra.Command {
	return a.newPostsWriteCmd("unarchive", "Bring a post back from the archive of its tool",
		`Brings a post back among the posts of its tool, where its moment decides its
place, or on top where it is pinned. A post that is not in the archive exits 9
with postNotArchived; one in the trash exits 2 (not_found).`,
		(*api.Client).UnarchivePost, "Brought post %s back from the archive")
}

// newPostsWriteCmd is a write on one post that takes nothing but the post and
// answers with it.
func (a *app) newPostsWriteCmd(name, short, long string, write func(*api.Client, context.Context, string) (api.Result[api.Post], error), done string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <postId>",
		Short: short,
		Long:  long,
		Args:  requireArgs("postId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			post, err := write(s.client, a.ctx, args[0])
			if err != nil {
				return postRefusal(s, err)
			}
			summary := fmt.Sprintf(done, post.Value.Title)
			return a.ok(postSuccess(post, summary, nil, func(w io.Writer) {
				fmt.Fprintf(w, "%s.\n%s\n", summary, post.Value.AppURL)
			}))
		},
	}
}

func (a *app) newPostsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "delete <postId>",
		Aliases: []string{"trash"},
		Short:   "Move a post to the trash",
		Long: `Moves a post to the trash, from the list or from the archive, as removing it on
the web does. Anybody in the project may remove any post.

A post in the trash reads like one that never existed: showing, changing or
deleting it again exits with 2 (not_found), and archiving it with 9
(postInTrash). It comes back in Minka on the web only, among the posts and out
of the archive.`,
		Args: requireArgs("postId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeletePost(a.ctx, args[0]); err != nil {
				return postRefusal(s, err)
			}
			return a.ok(success{
				data:    map[string]any{"id": args[0], "deleted": true},
				summary: "Moved post " + args[0] + " to the trash",
				text: func(w io.Writer) {
					fmt.Fprintf(w, "Moved post %s to the trash.\n", args[0])
				},
			})
		},
	}
}

func (a *app) newPostCommentsCmd() *cobra.Command {
	return group("comments", "Read, write, change, remove and react to comments on a post",
		a.newPostCommentsListCmd(),
		a.newPostCommentsCreateCmd(),
		a.newPostCommentsEditCmd(),
		a.newPostCommentsDeleteCmd(),
		a.newPostCommentsReactCmd(),
		a.newPostCommentsUnreactCmd(),
	)
}

func (a *app) newPostCommentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <postId>",
		Short: "List the comments on a post, the oldest first",
		Long: `Lists every comment on a post, the oldest first, with its reactions. Nothing is
paged. posts show carries the same comments together with the post.`,
		Args: requireArgs("postId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			comments, err := s.client.CommentsOn(a.ctx, postItem(args[0]))
			if err != nil {
				return postRefusal(s, err)
			}
			return a.ok(success{
				data:        comments.Raw,
				summary:     plural(len(comments.Value), "comment", "comments"),
				breadcrumbs: postCommentBreadcrumbs(args[0]),
				text: func(w io.Writer) {
					if len(comments.Value) == 0 {
						fmt.Fprintln(w, "No comments.")
						return
					}
					renderEntryComments(w, comments.Value)
				},
			})
		},
	}
}

func (a *app) newPostCommentsCreateCmd() *cobra.Command {
	var format string

	cmd := &cobra.Command{
		Use:   "create <postId> <text|->",
		Short: "Comment on a post",
		Long:  "Writes a comment on a post.\n\n" + commentFormatHelp,
		Example: `  minka posts comments create 0199... "Looks right to me"
  minka posts comments create 0199... - < comment.md`,
		Args: requireArgs("postId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the comment", args[1])
			if err != nil {
				return err
			}
			body, notices, err := commentBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.WriteCommentOn(a.ctx, postItem(args[0]), body)
			if err != nil {
				return postRefusal(s, err)
			}

			summary := "Commented on post " + args[0]
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: postCommentBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")

	return cmd
}

func (a *app) newPostCommentsEditCmd() *cobra.Command {
	var format string
	var dropAttachments bool

	cmd := &cobra.Command{
		Use:   "edit <postId> <commentId> <text|->",
		Short: "Change a comment you wrote on a post",
		Long: "Changes the text of a comment. Only its author can.\n\n" + commentFormatHelp + `

Files and mentions stand in a comment as action-text-attachment elements, which
Markdown cannot hold. To keep them, take the bodySource of the comment from
posts show --json, change it and send it back with --format html. A new text
that would drop them from a comment of yours is refused unless
--drop-attachments is given; the command reads the post first to know.`,
		Example: `  minka posts comments edit 0199... 0199... "Looks right after all"`,
		Args:    requireArgs("postId", "commentId", "text"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := commentBody(cmd, "", format); err != nil {
				return err
			}
			text, err := a.readValue("the comment", args[2])
			if err != nil {
				return err
			}
			body, notices, err := commentBody(cmd, text, format)
			if err != nil {
				return err
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			post, err := s.client.Post(a.ctx, args[0])
			if err != nil {
				return postRefusal(s, err)
			}
			dropped, err := a.keepsTheAttachmentsOfAComment(s, post.Value, args[1], body, dropAttachments)
			if err != nil {
				return err
			}
			notices = append(notices, dropped...)

			comment, err := s.client.EditCommentOn(a.ctx, postItem(args[0]), args[1], body)
			if err != nil {
				return postCommentRefusal(s, err)
			}

			summary := fmt.Sprintf("Changed comment %s on post %s", args[1], args[0])
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				notices:     notices,
				breadcrumbs: postCommentBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
	cmd.Flags().StringVar(&format, "format", "markdown", "Format of the text: markdown or html")
	cmd.Flags().BoolVar(&dropAttachments, "drop-attachments", false, "Replace a comment that has attachments or mentions, which drops them")

	return cmd
}

// keepsTheAttachmentsOfAComment holds a new text against the comment it
// replaces, where anything could be lost: a comment of somebody else and a
// comment under a post in the archive are refused by Minka whatever is sent,
// so its answer comes first there and this side keeps no guard of its own.
func (a *app) keepsTheAttachmentsOfAComment(s *session, post api.Post, commentID, body string, allowed bool) ([]string, error) {
	if post.Archived {
		return nil, nil
	}
	for _, current := range post.Comments {
		if current.ID != commentID || !richtext.HasAttachments(firstNonEmpty(current.BodySource, current.Body)) {
			continue
		}
		me, err := a.meID(s)()
		if err != nil {
			return nil, err
		}
		if current.Author.ID != me {
			return nil, nil
		}
		return a.guardOverFiles(s, post.ProjectID, firstNonEmpty(current.BodySource, current.Body), body, allowed,
			"the comment has attachments or mentions, and the new text would drop them",
			"Change the bodySource of the comment from posts show --json and send it back with --format html, or give --drop-attachments.")
	}
	return nil, nil
}

// guardOverFiles holds a new text against the one it replaces, and stands
// aside where Minka refuses the write whatever is sent. The post does not say
// whether its project is archived, so that is asked only where the guard is
// about to refuse.
func (a *app) guardOverFiles(s *session, projectID, current, replacement string, allowed bool, message, hint string) ([]string, error) {
	dropped, refusal := keepsTheAttachments(current, replacement, allowed, message, hint)
	if refusal == nil {
		return dropped, nil
	}
	archived, err := s.client.Projects(a.ctx, true)
	if err != nil {
		return nil, s.fail(err)
	}
	for _, project := range archived.Value {
		if project.ID == projectID && project.Status == "archived" {
			return nil, nil
		}
	}
	return nil, refusal
}

func (a *app) newPostCommentsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <postId> <commentId>",
		Short: "Remove a comment you wrote on a post",
		Long: `Removes a comment. Only its author can. A removed comment is gone, not marked:
removing it again exits with 2 (not_found), like a comment that never existed.`,
		Args: requireArgs("postId", "commentId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.DeleteCommentOn(a.ctx, postItem(args[0]), args[1]); err != nil {
				return postCommentRefusal(s, err)
			}

			summary := fmt.Sprintf("Removed comment %s from post %s", args[1], args[0])
			return a.ok(success{
				data:        map[string]any{"postId": args[0], "commentId": args[1], "deleted": true},
				summary:     summary,
				breadcrumbs: postCommentBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newPostCommentsReactCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "react <postId> <commentId> <content>",
		Short: "React to a comment on a post",
		Long: "Leaves a reaction on a comment.\n\n" + reactionHelp + `

Only the comments take reactions through the API. A post itself does not,
although the web offers it.`,
		Example: `  minka posts comments react 0199... 0199... 👍`,
		Args:    requireArgs("postId", "commentId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			comment, err := s.client.ReactToCommentOn(a.ctx, postItem(args[0]), args[1], args[2])
			if err != nil {
				return reactionError(postReactionRefusal(s, err))
			}

			summary := fmt.Sprintf("Reacted %s to comment %s", args[2], args[1])
			if comment.StatusCode == http.StatusOK {
				summary = fmt.Sprintf("You had reacted %s to comment %s already", args[2], args[1])
			}
			return a.ok(success{
				data:        comment.Raw,
				summary:     summary,
				breadcrumbs: postCommentBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func (a *app) newPostCommentsUnreactCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unreact <postId> <commentId> <content>",
		Short: "Take a reaction off a comment on a post",
		Long:  "Takes a reaction off a comment.\n\n" + reactionHelp,
		Args:  requireArgs("postId", "commentId", "content"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.client.UnreactFromCommentOn(a.ctx, postItem(args[0]), args[1], args[2]); err != nil {
				return reactionError(postReactionRefusal(s, err))
			}

			summary := fmt.Sprintf("Took %s off comment %s", args[2], args[1])
			return a.ok(success{
				data:        map[string]any{"postId": args[0], "commentId": args[1], "content": args[2]},
				summary:     summary,
				breadcrumbs: postCommentBreadcrumbs(args[0]),
				text:        func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

func postItem(postID string) api.Commented {
	return api.Commented{Kind: api.CommentedPost, ID: postID}
}

// postBody reads and converts the body of a post. Markdown is converted here
// because Minka keeps a body without an element it serves as the characters it
// consists of, and says nothing about it.
func (a *app) postBody(value, format string) (string, []string, error) {
	text, err := a.readValue("the body", value)
	if err != nil {
		return "", nil, err
	}
	if format == "html" {
		return text, markupNotice(text), nil
	}
	result := markdown.ToHTML(text)
	if result.HTML == "" && strings.TrimSpace(text) != "" {
		return "", nil, &Error{
			Code:    CodeUsage,
			Message: "the body has nothing Minka keeps",
			Hint:    "A post says something in words, in a picture or in a file.",
		}
	}
	return result.HTML, result.Warnings, nil
}

// postRefusal classifies a failed request about a post. Each code names a
// state the post or somebody named on it is in, and the hint says what can be
// done about it, including where that is nothing.
func postRefusal(s *session, err error) *Error {
	e := s.fail(err)
	if hint, ok := audienceHint(e.APICode); ok {
		e.Hint = hint
		return e
	}
	switch {
	case e.APICode == apiCodePersonCannotSeeIt:
		e.Hint = "Nothing was written, and the same request will be refused again: somebody named with --notify is a client of the project, or an agent working for one, and a post written through the API or on the web is shown to no client. Name somebody else."
	case e.APICode == apiCodePostArchived:
		e.Hint = "The post lies in the archive of its tool. It can be read; nothing is written on it until it is back among the posts: minka posts unarchive <postId>."
	case e.APICode == apiCodePostNotArchived:
		e.Hint = "The post is not in the archive; it stands among the posts already."
	case e.APICode == apiCodePostInTrash:
		e.Hint = "The post is in the trash, which is not the archive. It comes back in Minka on the web, and can be archived once it is back among the posts."
	case e.APICode == apiCodePostCategoryUnknown:
		e.Hint = "The project offers no such kind of post. The kinds: minka posts categories <projectId>."
	case e.APICode == apiCodePostBodyMissing:
		e.Hint = "A post that goes out says something: give --body with words, a picture or a file."
	case e.APICode == apiCodePostTitleMissing:
		e.Hint = "A post needs a title: it can be replaced, never removed."
	case e.Code == CodeNotFound:
		e.Hint = postNotFoundHint
	}
	return e
}

// postsToolRefusal classifies a failed read of a Posts tool.
func postsToolRefusal(s *session, err error) *Error {
	e := s.fail(err)
	if e.Code == CodeNotFound {
		e.Hint = postsToolNotFoundHint
	}
	return e
}

// postCommentRefusal is a refusal of a write on one comment. An address that
// has nothing may be the post, the comment or its author.
func postCommentRefusal(s *session, err error) *Error {
	e := postRefusal(s, err)
	if e.Code == CodeNotFound {
		e.Hint = "The post does not exist or is not visible to you, the comment was removed or hangs under another post, or somebody else wrote it: only the author can change or remove a comment."
	}
	return e
}

// postReactionRefusal is a refusal of a reaction on a comment, which anybody
// may leave: an address that has nothing is the post or the comment.
func postReactionRefusal(s *session, err error) *Error {
	e := postRefusal(s, err)
	if e.Code == CodeNotFound {
		e.Hint = "The post does not exist or is not visible to you, or the comment was removed or hangs under another post. The comments of a post: minka posts comments list <postId>"
	}
	return e
}

func postSummary(post api.Post) string {
	if post.Archived {
		return "Post " + post.Title + " in the archive of " + post.Tool.Title
	}
	return "Post " + post.Title + " in " + post.Tool.Title
}

func postSuccess(post api.Result[api.Post], summary string, notices []string, text func(io.Writer)) success {
	p := post.Value
	crumbs := []Breadcrumb{
		{Action: "comment", Cmd: "minka posts comments create " + p.ID + " <text|->", Description: "Comment on the post"},
		{Action: "edit", Cmd: "minka posts edit " + p.ID + " --body -", Description: "Write over the post"},
	}
	switch {
	case p.Archived:
		crumbs = []Breadcrumb{{Action: "unarchive", Cmd: "minka posts unarchive " + p.ID, Description: "Bring the post back among the posts"}}
	case p.Pinned:
		crumbs = append(crumbs, Breadcrumb{Action: "unpin", Cmd: "minka posts unpin " + p.ID, Description: "Let the post go back among the others"})
	default:
		crumbs = append(crumbs, Breadcrumb{Action: "pin", Cmd: "minka posts pin " + p.ID, Description: "Pin the post to the top of its list"})
	}
	crumbs = append(crumbs, Breadcrumb{Action: "list", Cmd: "minka posts list " + p.ToolID, Description: "List the posts beside it"})
	return success{data: post.Raw, summary: summary, notices: notices, breadcrumbs: crumbs, text: text}
}

func postCommentBreadcrumbs(postID string) []Breadcrumb {
	return []Breadcrumb{{Action: "show", Cmd: "minka posts show " + postID, Description: "Read the post with its comments"}}
}

func renderPostsTools(w io.Writer, tools []api.PostsTool) {
	if len(tools) == 0 {
		fmt.Fprintln(w, "No Posts tools.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ID\tTITLE")
	for _, tool := range tools {
		fmt.Fprintf(table, "%s\t%s\n", tool.ID, tool.Title)
	}
	_ = table.Flush()
}

func renderPostLines(w io.Writer, posts []api.PostLine) {
	if len(posts) == 0 {
		fmt.Fprintln(w, "No posts.")
		return
	}
	for _, post := range posts {
		var details []string
		if post.Pinned {
			details = append(details, "pinned")
		}
		if post.Category != nil {
			details = append(details, *post.Category)
		}
		if post.CommentCount > 0 {
			details = append(details, plural(post.CommentCount, "comment", "comments"))
		}
		line := fmt.Sprintf("- %s  %s", post.ID, post.Title)
		if len(details) > 0 {
			line += " (" + strings.Join(details, ", ") + ")"
		}
		fmt.Fprintf(w, "%s\n  %s, %s\n", line, post.Creator.Name, formatTime(post.CreatedAt))
		if post.Lead != "" {
			fmt.Fprintf(w, "  %s\n", oneLine(post.Lead))
		}
	}
}

func renderArchivedPosts(w io.Writer, posts []api.ArchivedPost) {
	if len(posts) == 0 {
		fmt.Fprintln(w, "The archive is empty.")
		return
	}
	for _, post := range posts {
		line := fmt.Sprintf("- %s  %s", post.ID, post.Title)
		if post.Category != nil {
			line += " (" + *post.Category + ")"
		}
		fmt.Fprintln(w, line)
		var when []string
		if post.ArchivedAt != nil {
			when = append(when, formatTime(*post.ArchivedAt))
		}
		if post.ArchivedBy != nil {
			when = append(when, "by "+post.ArchivedBy.Name)
		}
		if len(when) > 0 {
			fmt.Fprintf(w, "  archived %s\n", strings.Join(when, " "))
		}
	}
}

func renderPost(w io.Writer, post api.Post) {
	fmt.Fprintf(w, "# %s\n", post.Title)
	fmt.Fprintf(w, "Post: %s\nTool: %s (%s)\nProject: %s\n", post.ID, post.Tool.Title, post.ToolID, post.ProjectID)
	if post.Category != nil {
		fmt.Fprintf(w, "Kind: %s\n", *post.Category)
	}
	if post.Pinned {
		fmt.Fprintln(w, "Pinned: yes")
	}
	if post.Archived {
		fmt.Fprintln(w, "Archived: yes, it takes no change until it is back")
	}
	fmt.Fprintf(w, "Created: %s by %s\n", formatTime(post.CreatedAt), post.Creator.Name)
	if post.EditedAt != nil {
		fmt.Fprintf(w, "Edited: %s\n", formatTime(*post.EditedAt))
	}
	fmt.Fprintf(w, "URL: %s\n", post.AppURL)

	if body := richtext.ToMarkdown(firstNonEmpty(post.BodySource, post.Body)); body != "" {
		fmt.Fprintf(w, "\n## Body\n\n%s\n", body)
	}
	renderEntryFiles(w, post.Attachments)
	renderEntryComments(w, post.Comments)
}
