package cli

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/flownative/minka-cli/internal/config"
	"github.com/flownative/minka-cli/internal/export"
	"github.com/spf13/cobra"
)

// exportParallel is how many documents an export reads, and how many files it
// downloads, at a time: quick enough for a book of hundreds of pages, and no
// burden on the host.
const exportParallel = 4

func (a *app) newDocsExportCmd() *cobra.Command {
	var (
		out, format, toolID string
		force               bool
	)
	cmd := &cobra.Command{
		Use:   "export <folderId> --out <dir> [--format markdown|html] [--tool <toolId>] [--force]",
		Short: "Write a book or a folder into a directory as files, and bring it up to date",
		Long: `Writes the book a folder is, or the chapter of a book a folder is, into a
directory: a directory per chapter, a file per page, and in every directory an
index of what stands in it and below it in the book's order, README.md
(index.html with --format html). A page left out of the navigation says so in
the index. The book is read in one request and every page in one request of
its own, a few at a time.

A folder in no book is walked through its tool, one request per folder:
name the tool with --tool. Its uploads are downloaded beside the pages. A
book's tree holds no uploads, so a book exports its chapters and pages as its
readers read them.

A page is Markdown with a frontmatter (title, id, appUrl, inNavigation,
updatedAt), or with --format html a minimal page of the HTML Minka stores.
Every heading carries the anchor Minka's page gives it, <a id="h-…"></a> above
it in Markdown and the id of the heading in HTML, so a link at "#h-…" opens
the same section in the file. The files hanging in a page are downloaded into
a directory beside it, <page>.assets/. A reference or a link to a page, a
chapter or an upload of the export is a link to its file, at the section it
names; anything else keeps its address in Minka.

Run it again into the same directory to bring it up to date: every page is
read again, a file whose bytes are the same is left as it is, a file Minka no
longer holds is removed, and a name stays with its entry while the title
gives it. The directory keeps a manifest, .minka-export.json, of what the
export wrote, and nothing else in the directory is ever written over or
removed: a path in the way is refused before anything is written. A
directory that holds files without a manifest, and a file of the export that
was changed since, are refused unless --force.`,
		Example: `  minka docs export 0199... --out handbook
  minka docs export 0199... --out handbook --json
  minka docs export 0199... --tool 0199... --out plans --format html`,
		Args: requireArgs("folderId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(out) == "" {
				return usageErrorf(cmd, "--out names the directory to export into")
			}
			if format != export.FormatMarkdown && format != export.FormatHTML {
				return usageErrorf(cmd, "the format is markdown or html, not %q", format)
			}
			if cmd.Flags().Changed("tool") {
				if err := requireID(cmd, "--tool", &toolID); err != nil {
					return err
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			host, err := url.Parse(s.host)
			if err != nil {
				return &Error{Code: CodeConfig, Message: "the host " + s.host + " is no address"}
			}
			lockDir, err := a.exportLockDir()
			if err != nil {
				return &Error{Code: CodeConfig, Message: err.Error()}
			}
			report, err := export.Run(a.ctx, s.client, export.Options{
				FolderID: args[0], ToolID: toolID, Out: out, Format: format, Host: host,
				Force: force, Parallel: exportParallel, LockDir: lockDir,
			})
			if err != nil {
				return exportRefusal(s, err)
			}

			var notices []string
			for _, gone := range report.Gone {
				notices = append(notices, fmt.Sprintf("the page %s (%s) was gone when it was read and is left out", gone.Title, gone.ID))
			}
			return a.ok(success{
				data:        report,
				summary:     exportSummary(report),
				notices:     notices,
				breadcrumbs: exportBreadcrumbs(args[0], toolID, out, format, report),
				text:        func(w io.Writer) { renderExport(w, report) },
			})
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "directory to export into; made where it does not exist")
	cmd.Flags().StringVar(&format, "format", export.FormatMarkdown, "markdown or html")
	cmd.Flags().StringVar(&toolID, "tool", "", "ID of the tool, to walk a folder that stands in no book")
	cmd.Flags().BoolVar(&force, "force", false, "export into a directory that holds files without a manifest, and write over or remove files of the export changed since")

	return cmd
}

// exportLockDir is where the locks of exports lie: beside the config file,
// as the other files minka keeps for itself.
func (a *app) exportLockDir() (string, error) {
	path, err := config.Path(a.getenv)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "export-locks"), nil
}

func exportRefusal(s *session, err error) *Error {
	var refusal *export.RefusalError
	var failedWrite *export.WriteError
	switch {
	case errors.As(err, &refusal):
		e := &Error{Code: CodeUsage, Message: refusal.Message, Hint: refusal.Hint}
		if len(refusal.Paths) > 0 {
			e.Data = map[string]any{"paths": refusal.Paths}
			e.Text = func(w io.Writer) {
				for _, p := range refusal.Paths {
					fmt.Fprintf(w, "  %s\n", p)
				}
			}
		}
		return e
	case errors.As(err, &failedWrite):
		return &Error{Code: CodeConfig, Message: "writing the export: " + failedWrite.Error(), Hint: "What was written so far is in the manifest; run again once the directory can be written."}
	case errors.Is(err, export.ErrNoBook):
		e := s.fail(err)
		e.Message = "the folder stands in no book"
		e.Hint = "A folder in no book is walked through its tool: add --tool <toolId>. " + bookNotFoundHint
		return e
	case errors.Is(err, export.ErrNotInTheBook):
		return &Error{Code: CodeNotFound, Message: "the folder is no chapter of its book in use", Hint: "A chapter in the archive is not part of its book's tree; export the book, or bring the chapter back."}
	}
	e := s.fail(err)
	if e.Code == CodeNotFound {
		e.Hint = "The folder, a page or a file of it is not there, it belongs to a project you are not a member of, or you are a client of the project and it is not shown to you."
	}
	return e
}

func exportSummary(report *export.Report) string {
	what := "folder"
	if report.IsBook {
		what = "book"
		if report.Book != nil && report.Book.ID != report.FolderID {
			what = "chapter"
		}
	}
	return fmt.Sprintf("Exported %s %s to %s: %d written, %d unchanged, %d removed", what, report.Title, report.Out, len(report.Written), report.Unchanged, len(report.Removed))
}

func renderExport(w io.Writer, report *export.Report) {
	fmt.Fprintln(w, exportSummary(report))
	fmt.Fprintf(w, "Format: %s\n", report.Format)
	fmt.Fprintf(w, "Folders: %d, pages: %d, uploads: %d, files in pages: %d\n", report.Folders, report.Documents, report.Uploads, report.Assets)
	for _, p := range report.Written {
		fmt.Fprintf(w, "  + %s\n", p)
	}
	for _, p := range report.Removed {
		fmt.Fprintf(w, "  - %s\n", p)
	}
	index := "README.md"
	if report.Format == export.FormatHTML {
		index = "index.html"
	}
	fmt.Fprintf(w, "Start at: %s\n", filepath.Join(report.Out, index))
}

func exportBreadcrumbs(folderID, toolID, out, format string, report *export.Report) []Breadcrumb {
	again := "minka docs export " + folderID
	if toolID != "" {
		again += " --tool " + toolID
	}
	again += " --out " + shellWord(out)
	if format != export.FormatMarkdown {
		again += " --format " + format
	}
	crumbs := []Breadcrumb{{Action: "refresh", Cmd: again, Description: "Bring the export up to date"}}
	if report.IsBook {
		crumbs = append(crumbs, Breadcrumb{Action: "book", Cmd: "minka docs book " + folderID, Description: "Read the book as one tree"})
	}
	return append(crumbs, Breadcrumb{Action: "show", Cmd: "minka docs show <documentId>", Description: "Read a page with its comments"})
}
