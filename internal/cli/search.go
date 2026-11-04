package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// The orders Minka has for hits. It takes any other word as the first, so a
// word it does not know is refused here rather than ignored there.
var searchSorts = []string{"best", "recent"}

// apiCodeSearchScopeMissing is what a Minka that searches only within files
// answers a search without a tool or a folder.
const apiCodeSearchScopeMissing = "searchScopeMissing"

const searchScopeNotFoundHint = "There is no such Docs & Files tool or folder, it is in the trash, the folder does not stand in the tool --tool names, or it belongs to a project you are not a member of. Find a tool with: minka docs tools <projectId>, a folder with: minka docs list <toolId>"

const searchProjectNotFoundHint = "There is no such project, or you are not a member of it. Find one with: minka projects list"

const searchWithinFilesOnlyHint = "Its Minka is older than this minka expects and searches only the documents and uploads of a Docs & Files tool or folder. Find a tool with: minka docs tools <projectId>"

// searchDeepestNotice says where a search stops and how to reach what lies
// behind it. It holds for every host, an older one without the ceiling too.
const searchDeepestNotice = "minka reads no further than offset 500, and there may be more hits behind it: narrow the search with --kind, --project, --tool or --folder, or a sharper term"

func (a *app) newSearchCmd() *cobra.Command {
	var toolID, folderID, projectID, sort, offset string
	var kindFlags []string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <term>...",
		Short: "Search everything you may read in Minka, or a Docs & Files tool or folder",
		Long: `Searches everything you may read in Minka the way its search on the web
does: cards, comments, posts, to-do lists, groups and to-dos, folders,
documents, uploads, events, the lines of the chats of your projects and of
your own private conversations. Drafts, the trash, archived projects and
lines taken back are never found. Several words are one term, joined by a
space; Minka reads phrases in double quotes and the operators of its web
search in it, and cuts it at 200 characters. A term that starts with a dash,
such as -draft for "not draft", goes behind --, with every flag before it:
behind -- every word is part of the term.

--kind narrows to the hits of tools: board, posts, todos, files, calendar,
chat or conversation, repeated or separated by commas. A comment counts
toward the tool of what it stands under. --project narrows to one project and
leaves the private conversations out.

--tool searches the documents and uploads of a Docs & Files tool, --folder
those of a folder and everything beneath it, both those of a folder within a
tool. Beside them only documents and uploads are found, so --kind has to
name files there, and --project stands beside neither: a tool or folder lies
in one project already.

Every hit names its kind, its title, the project and the text around the
match; a comment names what it stands under, a line of a chat the chat, a
line of a private conversation the other person. A card, and a comment under
one, carries the card's short identifier where its board shows them. A
document hit names the heading the match stands under, and its address then
ends in the anchor of that heading, such as #h-setup-2. What is in the
archive, or in an archived folder or list, is found as well: archived is
true, and its line in the text is marked archived.

--sort is best (the default) or recent. --limit is how many hits to read, 25
unless given: Minka serves 25 a page, and the command reads page after page
only until it has that many. Where more are left, next in data, the
breadcrumb and a notice say where to read on: hand it to --offset exactly as
it came. Minka serves no page beyond offset 500, so the command reads no
further, hands out no next beyond it and says so in a notice: narrow the
search to reach what lies behind.

A project, tool or folder that is not there, that you may not read, or a
folder outside the tool --tool names, exits 2 (not_found). A Minka that
searches only within a Docs & Files tool or folder answers a search without
--tool or --folder with exit 2 as well.`,
		Example: `  minka search tent --json
  minka search tent costs --kind board,posts --project 0199... --json
  minka search '"site plan"' --folder 0199... --sort recent --json
  minka search tent --limit 50 --offset 25 --json
  minka search --kind todos --json -- -draft tent`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return usageErrorf(cmd, "missing term")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			term := strings.TrimSpace(strings.Join(args, " "))
			if term == "" {
				return usageErrorf(cmd, "the term is empty; name what to look for")
			}
			flags := cmd.Flags()
			withinFiles := flags.Changed("tool") || flags.Changed("folder")
			if flags.Changed("tool") {
				if err := requireID(cmd, "--tool", &toolID); err != nil {
					return err
				}
			}
			if flags.Changed("folder") {
				if err := requireID(cmd, "--folder", &folderID); err != nil {
					return err
				}
			}
			kinds, err := searchKindsOf(cmd, kindFlags)
			if err != nil {
				return err
			}
			if withinFiles && len(kinds) > 0 && !slices.Contains(kinds, api.SearchKindFiles) {
				return usageErrorf(cmd, "beside --tool or --folder Minka finds documents and uploads alone, which are of the kind files; --kind %s would find nothing", strings.Join(kinds, ","))
			}
			if flags.Changed("project") {
				if withinFiles {
					return usageErrorf(cmd, "--project stands beside neither --tool nor --folder: a tool or folder lies in one project already, and another project would find nothing")
				}
				if err := requireID(cmd, "--project", &projectID); err != nil {
					return err
				}
			}
			if !slices.Contains(searchSorts, sort) {
				return usageErrorf(cmd, "--sort is best or recent, not %q; Minka would take any other word as best without saying so", sort)
			}
			if limit < 1 {
				return usageErrorf(cmd, "--limit is a number of hits of at least 1, not %d", limit)
			}
			if flags.Changed("offset") {
				if !api.SearchOffsetPattern.MatchString(offset) {
					return usageErrorf(cmd, "--offset takes the next of an earlier search exactly as it came, a whole number, not %q", offset)
				}
				if n, _ := strconv.Atoi(offset); n > api.SearchDeepestOffset {
					return usageErrorf(cmd, "--offset goes up to %d, not %s: Minka serves no page beyond it; narrow the search with --kind, --project, --tool or --folder, or a sharper term", api.SearchDeepestOffset, offset)
				}
			}

			var notices []string
			if utf8.RuneCountInString(term) > api.SearchTermMaxLength {
				notices = append(notices, fmt.Sprintf("the term is longer than %d characters, and Minka searches only for the first %d of them", api.SearchTermMaxLength, api.SearchTermMaxLength))
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			query := api.SearchQuery{Term: term, Kinds: kinds, ProjectID: projectID, ToolID: toolID, FolderID: folderID, Sort: sort, Offset: offset}
			run, err := s.client.SearchUpTo(a.ctx, query, limit)
			if err != nil {
				e := s.fail(err)
				switch {
				case e.APICode == apiCodeSearchScopeMissing && !withinFiles:
					e.Code = CodeNotFound
					e.Message = s.host + " searches only within a Docs & Files tool or folder; give --tool or --folder"
					e.Hint = searchWithinFilesOnlyHint
				case e.Code == CodeNotFound && withinFiles:
					e.Hint = searchScopeNotFoundHint
				case e.Code == CodeNotFound && projectID != "":
					e.Hint = searchProjectNotFoundHint
				}
				return e
			}

			raw := run.Raw
			if raw == nil {
				raw = []json.RawMessage{}
			}
			var next string
			if run.Next != nil {
				next = searchCommand(query, limit, *run.Next)
				notices = append(notices, "there are more hits: "+next)
			}
			if run.Deepest {
				notices = append(notices, searchDeepestNotice)
			}
			return a.ok(success{
				data:        map[string]any{"hits": raw, "next": run.Next},
				summary:     searchSummary(run),
				notices:     notices,
				breadcrumbs: searchBreadcrumbs(run, next),
				text:        func(w io.Writer) { renderSearchHits(w, run.Hits) },
			})
		},
	}
	cmd.Flags().StringArrayVar(&kindFlags, "kind", nil, "Kind of hits: board, posts, todos, files, calendar, chat or conversation; repeated or separated by commas")
	cmd.Flags().StringVar(&projectID, "project", "", "ID of the project to search")
	cmd.Flags().StringVar(&toolID, "tool", "", "ID of the Docs & Files tool to search")
	cmd.Flags().StringVar(&folderID, "folder", "", "ID of the folder to search, with everything beneath it")
	cmd.Flags().StringVar(&sort, "sort", "best", "Order of the hits: best or recent")
	cmd.Flags().IntVar(&limit, "limit", api.SearchPageSize, "Number of hits to read")
	cmd.Flags().StringVar(&offset, "offset", "", "Where to start: next of an earlier search")

	return cmd
}

// searchKindsOf reads the words of --kind, each once in the order given.
// Minka refuses a word it does not know, an empty one between commas
// included, so the whole search is refused here instead.
func searchKindsOf(cmd *cobra.Command, flags []string) ([]string, error) {
	var kinds []string
	for _, flag := range flags {
		for _, kind := range strings.Split(flag, ",") {
			kind = strings.TrimSpace(kind)
			if !slices.Contains(api.SearchKinds, kind) {
				return nil, usageErrorf(cmd, "--kind takes %s, not %q", strings.Join(api.SearchKinds, ", "), kind)
			}
			if !slices.Contains(kinds, kind) {
				kinds = append(kinds, kind)
			}
		}
	}
	return kinds, nil
}

// searchCommand is the search that reads on from next, with everything else
// as it was given. The term comes first, so a flag appended to the command is
// read as a flag. Only a term that starts with a dash, as Minka's NOT does,
// goes last behind "--", where it is not read as a flag itself.
func searchCommand(query api.SearchQuery, limit int, next string) string {
	term := shellWord(query.Term)
	command := "minka search " + term
	if strings.HasPrefix(query.Term, "-") {
		command = "minka search"
	}
	if len(query.Kinds) > 0 {
		command += " --kind " + strings.Join(query.Kinds, ",")
	}
	if query.ProjectID != "" {
		command += " --project " + query.ProjectID
	}
	if query.ToolID != "" {
		command += " --tool " + query.ToolID
	}
	if query.FolderID != "" {
		command += " --folder " + query.FolderID
	}
	if query.Sort != "best" {
		command += " --sort " + query.Sort
	}
	if limit != api.SearchPageSize {
		command += " --limit " + strconv.Itoa(limit)
	}
	command += " --offset " + next
	if strings.HasPrefix(query.Term, "-") {
		command += " -- " + term
	}
	return command
}

func searchSummary(run api.SearchRun) string {
	summary := plural(len(run.Hits), "hit", "hits")
	if run.Next != nil || run.Deepest {
		summary += ", and more behind them"
	}
	return summary
}

func searchBreadcrumbs(run api.SearchRun, next string) []Breadcrumb {
	var crumbs []Breadcrumb
	for _, hit := range run.Hits {
		if crumb, ok := hitBreadcrumb(hit); ok {
			crumbs = append(crumbs, crumb)
			break
		}
	}
	if next != "" {
		crumbs = append(crumbs, Breadcrumb{Action: "next", Cmd: next, Description: "Read the hits behind these"})
	}
	return crumbs
}

// searchShowCommands read what a hit leads to, by the kind of its target. A
// line of a chat or a conversation has none: the transcript is read from its
// newest end, so no command opens it at the line.
var searchShowCommands = map[string]string{
	api.SearchHitCard:     "minka cards show ",
	api.SearchHitPost:     "minka posts show ",
	api.SearchHitTodoList: "minka todolists show ",
	api.SearchHitTodo:     "minka todos show ",
	api.SearchHitFolder:   "minka docs folders show ",
	api.SearchHitDocument: "minka docs show ",
	api.SearchHitUpload:   "minka docs uploads show ",
	api.SearchHitEvent:    "minka events show ",
}

// hitBreadcrumb reads what the hit leads to, where the API shows it and a
// command reads it. It is offered for the first hit that has one, which need
// not be the first hit.
func hitBreadcrumb(hit api.SearchHit) (Breadcrumb, bool) {
	kind, id := hit.Target()
	command, ok := searchShowCommands[kind]
	if !ok || hit.TargetKind != "" && hit.APIURL == nil {
		return Breadcrumb{}, false
	}
	description := "Read the " + searchKindWord(kind) + " found"
	if hit.Kind == api.SearchHitComment {
		description = "Read the " + searchKindWord(kind) + " a comment found stands under"
	}
	return Breadcrumb{Action: "show", Cmd: command + id, Description: description}, true
}

// searchKindWords are the kinds of hit as the text names them.
var searchKindWords = map[string]string{
	api.SearchHitTodoList:  "to-do list",
	api.SearchHitTodo:      "to-do",
	api.SearchHitTodoGroup: "to-do group",
}

func searchKindWord(kind string) string {
	if word, ok := searchKindWords[kind]; ok {
		return word
	}
	return kind
}

func renderSearchHits(w io.Writer, hits []api.SearchHit) {
	if len(hits) == 0 {
		fmt.Fprintln(w, "No hits.")
		return
	}
	for _, hit := range hits {
		fmt.Fprintf(w, "- %s  %s\n", hit.ID, searchHitLine(hit))
		if excerpt := oneLine(hit.Excerpt); excerpt != "" {
			fmt.Fprintf(w, "  %s\n", excerpt)
		}
		fmt.Fprintf(w, "  %s\n", hit.AppURL)
	}
}

// searchHitLine names a hit by its kind and what it is: a comment by what it
// stands under, a line by its chat or the other person of the conversation.
func searchHitLine(hit api.SearchHit) string {
	title := hit.Title
	if hit.ShortID != nil && *hit.ShortID != "" {
		title = "#" + *hit.ShortID + " " + title
	}
	var line string
	switch hit.Kind {
	case api.SearchHitComment:
		line = "comment on " + searchKindWord(hit.TargetKind) + " " + title
	case api.SearchHitChat:
		line = "chat line in " + title
	case api.SearchHitConversation:
		line = "message with " + title
	default:
		line = searchKindWord(hit.Kind) + " " + title
		if hit.Section != nil {
			line += " › " + hit.Section.Heading
		}
	}
	var details []string
	if hit.Project != "" {
		details = append(details, "in "+hit.Project)
	}
	if hit.Archived {
		details = append(details, "archived")
	}
	if len(details) > 0 {
		line += " (" + strings.Join(details, ", ") + ")"
	}
	return line
}
