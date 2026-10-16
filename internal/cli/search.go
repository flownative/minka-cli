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

const searchScopeNotFoundHint = "There is no such Docs & Files tool or folder, it is in the trash, the folder does not stand in the tool --tool names, or it belongs to a project you are not a member of. Find a tool with: minka docs tools <projectId>, a folder with: minka docs list <toolId>"

func (a *app) newSearchCmd() *cobra.Command {
	var toolID, folderID, sort, offset string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <term>...",
		Short: "Search the documents and uploads of a Docs & Files tool or of a folder",
		Long: `Searches the documents and uploads of a Docs & Files tool (--tool), of a
folder and everything beneath it (--folder), or of a folder within a tool
(both), the way the search of Minka on the web finds them. Several words are
one term, joined by a space; Minka reads phrases in double quotes and the
operators of its web search in it, and cuts it at 200 characters. A term that
starts with a dash, such as -draft for "not draft", goes behind --, with
every flag before it: behind -- every word is part of the term.

Every hit is a document or an upload with its title, the text around the match
and its address. A document hit names the heading the match stands under, and
its address then ends in the anchor of that heading, such as #h-setup-2, which
opens the document at that section.

--sort is best (the default) or recent. --limit is how many hits to read, 25
unless given: Minka serves 25 a page, and the command reads page after page
only until it has that many. Where more are left, next in data, the
breadcrumb and a notice say where to read on: hand it to --offset exactly as
it came. Minka takes no offset beyond 999999, so no hit behind that is reached.

What is in the archive, or in an archived folder, is found as well and says so:
archived is true, and its line in the text is marked archived. Drafts and what
is in the trash are never found.

A tool or folder that is not there, that you may not read, or a folder outside
the tool --tool names, exits 2 (not_found).`,
		Example: `  minka search tent costs --folder 0199... --json
  minka search '"site plan"' --tool 0199... --sort recent --json
  minka search tent --tool 0199... --limit 50 --offset 25 --json
  minka search --tool 0199... --json -- -draft tent`,
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
			if !flags.Changed("tool") && !flags.Changed("folder") {
				return usageErrorf(cmd, "a search stays within a Docs & Files tool or a folder: give --tool, --folder or both")
			}
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
			if !slices.Contains(searchSorts, sort) {
				return usageErrorf(cmd, "--sort is best or recent, not %q; Minka would take any other word as best without saying so", sort)
			}
			if limit < 1 {
				return usageErrorf(cmd, "--limit is a number of hits of at least 1, not %d", limit)
			}
			if flags.Changed("offset") && !api.SearchOffsetPattern.MatchString(offset) {
				return usageErrorf(cmd, "--offset takes the next of an earlier search exactly as it came, a whole number, not %q", offset)
			}

			var notices []string
			if utf8.RuneCountInString(term) > api.SearchTermMaxLength {
				notices = append(notices, fmt.Sprintf("the term is longer than %d characters, and Minka searches only for the first %d of them", api.SearchTermMaxLength, api.SearchTermMaxLength))
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			query := api.SearchQuery{Term: term, ToolID: toolID, FolderID: folderID, Sort: sort, Offset: offset}
			run, err := s.client.SearchUpTo(a.ctx, query, limit)
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = searchScopeNotFoundHint
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
			return a.ok(success{
				data:        map[string]any{"hits": raw, "next": run.Next},
				summary:     searchSummary(run),
				notices:     notices,
				breadcrumbs: searchBreadcrumbs(run, next),
				text:        func(w io.Writer) { renderSearchHits(w, run.Hits) },
			})
		},
	}
	cmd.Flags().StringVar(&toolID, "tool", "", "ID of the Docs & Files tool to search")
	cmd.Flags().StringVar(&folderID, "folder", "", "ID of the folder to search, with everything beneath it")
	cmd.Flags().StringVar(&sort, "sort", "best", "Order of the hits: best or recent")
	cmd.Flags().IntVar(&limit, "limit", api.SearchPageSize, "Number of hits to read")
	cmd.Flags().StringVar(&offset, "offset", "", "Where to start: next of an earlier search")

	return cmd
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
	if run.Next != nil {
		summary += ", and more behind them"
	}
	return summary
}

func searchBreadcrumbs(run api.SearchRun, next string) []Breadcrumb {
	var crumbs []Breadcrumb
	if len(run.Hits) > 0 {
		crumbs = append(crumbs, hitBreadcrumb(run.Hits[0]))
	}
	if next != "" {
		crumbs = append(crumbs, Breadcrumb{Action: "next", Cmd: next, Description: "Read the hits behind these"})
	}
	return crumbs
}

func hitBreadcrumb(hit api.SearchHit) Breadcrumb {
	if hit.Kind == api.SearchHitUpload {
		return Breadcrumb{Action: "show", Cmd: "minka docs uploads show " + hit.ID, Description: "Read the upload found first"}
	}
	return Breadcrumb{Action: "show", Cmd: "minka docs show " + hit.ID, Description: "Read the document found first"}
}

func renderSearchHits(w io.Writer, hits []api.SearchHit) {
	if len(hits) == 0 {
		fmt.Fprintln(w, "No hits.")
		return
	}
	for _, hit := range hits {
		title := hit.Title
		if hit.Section != nil {
			title += " › " + hit.Section.Heading
		}
		var details []string
		if hit.Kind == api.SearchHitUpload {
			details = append(details, "upload")
		}
		if hit.Archived {
			details = append(details, "archived")
		}
		if len(details) > 0 {
			title += " (" + strings.Join(details, ", ") + ")"
		}
		fmt.Fprintf(w, "- %s  %s\n", hit.ID, title)
		if excerpt := oneLine(hit.Excerpt); excerpt != "" {
			fmt.Fprintf(w, "  %s\n", excerpt)
		}
		fmt.Fprintf(w, "  %s\n", hit.AppURL)
	}
}
