package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/flownative/minka-cli/internal/basecamp"
	"github.com/flownative/minka-cli/internal/bundle"
	"github.com/spf13/cobra"
)

func (a *app) newImportCmd() *cobra.Command {
	return group("import", "Extract the history of another system into an import bundle",
		a.newImportBasecampCmd(),
	)
}

type importReport struct {
	Source         string                  `json:"source"`
	SourceInstance string                  `json:"sourceInstance"`
	DryRun         bool                    `json:"dryRun"`
	Bundle         string                  `json:"bundle,omitempty"`
	Counts         map[bundle.Kind]int     `json:"counts"`
	Avatars        importAvatars           `json:"avatars"`
	SkippedTools   []basecamp.SkippedTools `json:"skippedTools"`
	MissingFiles   []basecamp.MissingFile  `json:"missingFiles"`
	// InvalidAddresses holds IDs only: neither the address nor the name of
	// somebody belongs into an output that ends up in a log.
	InvalidAddresses []string `json:"invalidAddresses"`
	// PeopleLeftOut are the people of the account no record of the bundle
	// refers to; counts.people is who was written.
	PeopleLeftOut int `json:"peopleLeftOut"`
}

type importAvatars struct {
	Count                   int  `json:"count"`
	InitialsDistinguishable bool `json:"initialsDistinguishable"`
}

func (a *app) newImportBasecampCmd() *cobra.Command {
	var projects []string
	var out string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "basecamp --project <id> [--project <id> …] --out <dir> [--dry-run]",
		Short: "Extract projects of a Basecamp account into an import bundle",
		Long: `Extracts projects of a Basecamp account into an import bundle, the directory
Minka's applier reads foreign history from.

It reads Basecamp through the Basecamp CLI, which has to be installed and
signed in (basecamp auth login) as somebody who may see the projects and the
e-mail addresses of other people; Basecamp masks them for anybody else, and
the run ends with exit 4. Nothing is written to Basecamp, and no Minka
host or token is needed.

So far a bundle holds the structure: the people the named projects refer to
with their avatars (their members and the creators of what is extracted, not
the rest of the account), the projects with their members, and of each
project the card tables, chats and message boards that are switched on. Other
tools and tools that are switched off are counted as skipped.

The bundle is built next to --out and takes its place at the end, so a run
that fails leaves no half bundle. A bundle of an earlier run is replaced; a
directory that holds anything else, a bundle of another source included, is
refused. --dry-run walks Basecamp the
same way, downloads no avatar, writes nothing and shows the counts.

A file that cannot be fetched does not end the run: the bundle is written
without it, the report names it under missingFiles, and the command exits 6
when trying again may help, else 7. A later run brings what is missing.

A bundle holds personal data. Keep it out of repositories and delete it when
it has been applied.`,
		Args: requireArgs(),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(projects) == 0 {
				return usageErrorf(cmd, "name at least one project with --project")
			}
			for _, id := range projects {
				if !basecamp.ValidProjectID(id) {
					return usageErrorf(cmd, "--project %q is not an ID; Basecamp IDs are numbers such as 12345678", id)
				}
			}
			if out == "" && !dryRun {
				return usageErrorf(cmd, "name the directory of the bundle with --out, or use --dry-run")
			}

			writer := bundle.Discard()
			switch {
			case !dryRun:
				var err error
				if writer, err = bundle.Create(out, basecamp.SourceName); err != nil {
					return bundleError(err)
				}
			case out != "":
				// A dry run says beforehand what the real one would refuse.
				target, err := filepath.Abs(out)
				if err == nil {
					err = bundle.Check(target, basecamp.SourceName)
				}
				if err != nil {
					return bundleError(err)
				}
			}
			defer writer.Abort()

			extractor := basecamp.Extractor{Source: a.basecampSource, Avatars: a.avatarFetcher}
			result, err := extractor.Extract(a.ctx, projects, writer)
			if err != nil {
				return basecampError(err)
			}
			leftover, err := writer.Commit(bundle.Manifest{
				SourceInstance: result.SourceInstance,
				ExtractedAt:    a.now().UTC().Format(time.RFC3339),
				Extractor:      "minka " + version,
			})
			if err != nil {
				return bundleError(err)
			}

			report := importReport{
				Source:         basecamp.SourceName,
				SourceInstance: result.SourceInstance,
				DryRun:         dryRun,
				Counts:         writer.Counts(),
				Avatars:        importAvatars{Count: result.Avatars},
				SkippedTools:   result.Skipped,
				MissingFiles:   result.Missing,

				InvalidAddresses: result.InvalidAddresses,
				PeopleLeftOut:    result.PeopleLeftOut,
			}
			var notices []string
			if count := len(report.InvalidAddresses); count > 0 {
				notices = append(notices, plural(count, "person comes", "people come")+" without an e-mail address, and so without a login, because the one Basecamp holds is none; data.invalidAddresses names them by ID, and only the source can repair it")
			}
			if leftover != "" {
				notices = append(notices, "the bundle of the run before could not be removed and still lies at "+leftover+"; it holds personal data, remove it by hand")
			}
			summary := "Dry run, nothing written: "
			if !dryRun {
				report.Bundle = writer.Path()
				summary = "Wrote " + report.Bundle + ": "
			}
			summary += plural(report.Counts[bundle.People], "person", "people") + ", " +
				plural(report.Counts[bundle.Projects], "project", "projects") + ", " +
				plural(report.Counts[bundle.Tools], "tool", "tools")

			if len(report.MissingFiles) > 0 {
				return incompleteImport(report, notices)
			}
			return a.ok(success{
				data:    report,
				summary: summary,
				notices: notices,
				text:    func(w io.Writer) { renderImportReport(w, report) },
			})
		},
	}
	cmd.Flags().StringArrayVar(&projects, "project", nil, "ID of a Basecamp project to extract; repeat for several")
	cmd.Flags().StringVar(&out, "out", "", "Directory of the bundle; an earlier bundle there is replaced")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Walk Basecamp and show the counts, write nothing")
	return cmd
}

func renderImportReport(w io.Writer, report importReport) {
	if report.DryRun {
		fmt.Fprintln(w, "Dry run: nothing written")
	} else {
		fmt.Fprintf(w, "Bundle: %s\n", report.Bundle)
	}
	fmt.Fprintf(w, "Source: %s %s\n\n", report.Source, report.SourceInstance)

	for _, kind := range bundle.Kinds {
		if count, ok := report.Counts[kind]; ok {
			fmt.Fprintf(w, "%-12s %d\n", kind, count)
		}
	}
	// Basecamp serves a picture it made from initials the same way as one
	// somebody uploaded, so every avatar is taken.
	fmt.Fprintf(w, "\nAvatars: %d, initials not distinguishable\n", report.Avatars.Count)

	fmt.Fprintf(w, "People of the account nothing refers to, left out: %d\n", report.PeopleLeftOut)
	if count := len(report.InvalidAddresses); count > 0 {
		fmt.Fprintf(w, "Invalid addresses: %d (person %s)\n", count, strings.Join(report.InvalidAddresses, ", "))
	}

	if len(report.MissingFiles) > 0 {
		fmt.Fprintln(w, "\nMissing files:")
		for _, missing := range report.MissingFiles {
			fmt.Fprintf(w, "  %s of %s %s: %s\n", missing.AttachmentSourceID, missing.OwnerKind, missing.OwnerSourceID, missing.Reason)
		}
	}

	if len(report.SkippedTools) > 0 {
		fmt.Fprintln(w, "\nSkipped tools:")
		for _, skipped := range report.SkippedTools {
			state := "switched off"
			if skipped.Enabled {
				state = "not part of Minka"
			}
			fmt.Fprintf(w, "  %-16s %-18s %d\n", skipped.Kind, state, skipped.Count)
		}
	}
}

// incompleteImport is the end of a run that wrote its bundle without some of
// the files. The bundle is whole and can be applied, so the report comes out
// as after any run; the failure tells a caller that a later run has something
// left to bring. It is worth trying again when a file failed for a reason that
// may pass by itself.
func incompleteImport(report importReport, notices []string) *Error {
	e := &Error{
		Code:    CodeAPI,
		Message: "the bundle was written, but " + plural(len(report.MissingFiles), "file", "files") + " could not be fetched and is left out",
		Hint:    "data.missingFiles names each file with the reason. The bundle can be applied as it is; a later run brings what is missing.",
		Data:    report,
		Text:    func(w io.Writer) { renderImportReport(w, report) },
		Notices: notices,
	}
	for _, missing := range report.MissingFiles {
		if missing.Retryable {
			e.Code, e.Retryable = CodeNetwork, true
		}
	}
	return e
}

// basecampError classifies a failed run. Everything a run asks of Basecamp is
// a read, so what may pass by itself is worth a second attempt.
func basecampError(err error) *Error {
	var failed *basecamp.Error
	if !errors.As(err, &failed) {
		return bundleError(err)
	}
	e := &Error{Code: CodeAPI, Message: failed.Message}
	switch failed.Kind {
	case basecamp.NotInstalled:
		e.Code = CodeConfig
		e.Hint = "minka reads Basecamp through the Basecamp CLI. Install it from https://github.com/basecamp/basecamp-cli and sign in with: basecamp auth login"
	case basecamp.NotFound:
		e.Code = CodeNotFound
		e.Hint = "List the projects you can see with: basecamp projects list"
	case basecamp.AuthRequired:
		e.Code = CodeAuth
		e.Hint = "Sign the Basecamp CLI in with: basecamp auth login"
	case basecamp.Forbidden:
		e.Code = CodeForbidden
		e.Hint = "The Basecamp account that is signed in may not read this."
	case basecamp.RateLimited:
		e.Retryable = true
		e.Hint = "Basecamp limits the requests of an account. Try again in a minute."
	case basecamp.Network:
		e.Code = CodeNetwork
		e.Retryable = true
		e.Hint = "Is Basecamp reachable? Check with: basecamp doctor"
	case basecamp.Interrupted:
		e.Code = CodeNetwork
	}
	if failed.Hint != "" {
		e.Hint = failed.Hint
	}
	return e
}

// bundleError is a failure to write the bundle: the directory, not Basecamp.
func bundleError(err error) *Error {
	if errors.Is(err, bundle.ErrNotABundle) {
		return &Error{
			Code:    CodeUsage,
			Message: err.Error(),
			Hint:    "A run replaces everything in --out. Name a directory that is empty, does not exist yet, or holds an earlier bundle.",
		}
	}
	e := &Error{Code: CodeConfig, Message: err.Error()}
	switch {
	case errors.Is(err, bundle.ErrSymlink):
		e.Hint = "A bundle is not written through a link: the one behind it would stay where nobody looks for it. Name the directory itself with --out."
	case errors.Is(err, bundle.ErrLocked):
		e.Hint = "Wait for the other run to end, or name another directory with --out. A run that was killed holds no lock."
	}
	return e
}
