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
	Source         string              `json:"source"`
	SourceInstance string              `json:"sourceInstance"`
	DryRun         bool                `json:"dryRun"`
	Bundle         string              `json:"bundle,omitempty"`
	Counts         map[bundle.Kind]int `json:"counts"`
	// Parts names the files of a kind that is written in several.
	Parts        map[bundle.Kind][]string `json:"parts,omitempty"`
	Avatars      importAvatars            `json:"avatars"`
	SkippedTools []basecamp.SkippedTools  `json:"skippedTools"`
	MissingFiles []basecamp.MissingFile   `json:"missingFiles"`
	// InvalidAddresses holds IDs only: neither the address nor the name of
	// somebody belongs into an output that ends up in a log.
	InvalidAddresses []string `json:"invalidAddresses"`
	// PeopleLeftOut are the people of the account no record of the bundle
	// refers to; counts.people is who was written.
	PeopleLeftOut int `json:"peopleLeftOut"`
	// CreatorProbesRefused are the dock entries asked for the creator of a
	// project that Basecamp refused: they are passed over, and a project may
	// come without a creator because of it.
	CreatorProbesRefused int `json:"creatorProbesRefused"`
	// CardsByStatus splits counts.cards; CardsOnHold are those of them that
	// were set aside within their column.
	CardsByStatus map[string]int `json:"cardsByStatus"`
	CardsOnHold   int            `json:"cardsOnHold"`
	// CardsLeftOut are archived and trashed cards whose column is in no board
	// that is extracted. CommentsTrashed are comments left out because they
	// are in the trash.
	CardsLeftOut    int `json:"cardsLeftOut"`
	CommentsTrashed int `json:"commentsTrashed"`
	// MentionsUnresolved stayed a name in the text. FilesInBodies counts the
	// references to a file that were written, FilesDropped the embedded files
	// Basecamp lists no file for.
	MentionsUnresolved int `json:"mentionsUnresolved"`
	FilesInBodies      int `json:"filesInBodies"`
	FilesDropped       int `json:"filesDropped"`
	// FilesExpected are the files the references name, each once, and
	// FilesBytesExpected what Basecamp says they weigh: what a run sets out to
	// bring, and all a dry run knows of them. The rest says what became of
	// them: FilesBytes is what the files of the bundle weigh, FilesFetched
	// were downloaded, FilesReused taken over from the bundle the run
	// replaces, FilesGone are those Basecamp no longer has.
	FilesExpected      int   `json:"filesExpected"`
	FilesBytesExpected int64 `json:"filesBytesExpected"`
	FilesBytes         int64 `json:"filesBytes"`
	FilesFetched       int   `json:"filesFetched"`
	FilesReused        int   `json:"filesReused"`
	FilesGone          int   `json:"filesGone"`
	// FileNamesChanged are files whose name could not stay as it was,
	// CaptionsDropped captions too long for a reference, FilesOfSeveralOwners
	// the times a file turned up under another owner than its first.
	FileNamesChanged     int `json:"fileNamesChanged"`
	CaptionsDropped      int `json:"captionsDropped"`
	FilesOfSeveralOwners int `json:"filesOfSeveralOwners"`
	// LinkPreviews are the previews of a link that became the link,
	// PicturesLinked the pictures at a foreign address that became a link to it.
	LinkPreviews   int `json:"linkPreviews"`
	PicturesLinked int `json:"picturesLinked"`
	// Embeds are the embedded frames that became an embed, EmbedsLeftOut those
	// that became nothing, because their address is none an embed may carry.
	Embeds        int `json:"embeds"`
	EmbedsLeftOut int `json:"embedsLeftOut"`
	// References are the links to a card, a post or a chat of the account
	// that became a reference, by the kind of record.
	References basecamp.ReferenceCounts `json:"references"`
	// PostsByStatus splits counts.posts. PostsLeftOut are archived and trashed
	// posts of a message board that is not extracted, PostsWithEmptyBody the
	// posts that came with a title alone.
	PostsByStatus      map[string]int `json:"postsByStatus"`
	PostsLeftOut       int            `json:"postsLeftOut"`
	PostsWithEmptyBody int            `json:"postsWithEmptyBody"`
	// QuestionsLeftOut are the questions left out because Minka has no words
	// for their schedule, each with the field that made it impossible.
	// AnchorsInferred are the questions asked every other week whose startsOn
	// was taken from the day an answer belongs to.
	QuestionsLeftOut []basecamp.ImpossibleQuestion `json:"questionsLeftOut"`
	AnchorsInferred  []basecamp.InferredAnchor     `json:"anchorsInferred"`
	// AskedLeftOut counts the people a question is asked of that the bundle
	// cannot say are asked.
	AskedLeftOut basecamp.AskedLeftOut `json:"askedLeftOut"`
	// AnswersWithoutABody are the answers that say nothing and AnswersTrashed
	// those in the trash; both are left out. AnswersWithoutADay are those
	// Basecamp names no day for, which take the day they were written on.
	AnswersWithoutABody int `json:"answersWithoutABody"`
	AnswersWithoutADay  int `json:"answersWithoutADay"`
	AnswersTrashed      int `json:"answersTrashed"`
	// MessagesByType splits counts.messages by what a line was in its chat.
	// MessagesWithEmptyBody are lines somebody wrote that say nothing,
	// UploadLinesLeftOut lines that were files alone, none of which can be
	// referred to: both are left out.
	MessagesByType        map[string]int `json:"messagesByType"`
	MessagesWithEmptyBody int            `json:"messagesWithEmptyBody"`
	UploadLinesLeftOut    int            `json:"uploadLinesLeftOut"`
	// LongestBody is the longest text per kind, in characters: a number and
	// nothing of the text.
	LongestBody map[bundle.Kind]int `json:"longestBody"`
	// RequestsRetried counts the requests that were asked again because they
	// failed for a reason that may pass, each further attempt counted.
	RequestsRetried int `json:"requestsRetried"`
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

So far a bundle holds the structure, the boards, the message boards, the chats
and the check-ins: the people the named projects refer to with their avatars
(their members and whoever a record names, not the rest of the account), the
projects with their members, of each project the card tables, chats, message
boards and questionnaires that are switched on, of each card table the columns,
the cards with the archived and the trashed ones, their steps, comments and
boosts, of each message board the messages with the archived and the trashed
ones as posts, their categories, comments and boosts, of each chat its lines
with their boosts as messages, and of each questionnaire its questions with
their answers, comments and boosts.

A question comes with the schedule Minka has words for: the weekdays counted
from Monday, the time of day, the day the rhythm is counted from, and whom it is
asked of, which are the subscribers of the question and not whoever answered it.
Only a member of the project who is still there and has an address can be asked
(askedLeftOut). A question whose schedule Minka cannot express is left out with
its answers and named with the field that made it impossible
(questionsLeftOut); one whose schedule has run out comes paused; one asked every
other week is anchored on the day it was last answered (anchorsInferred) and
left out where nobody ever answered it. An answer that says nothing or lies in
the trash is left out (answersWithoutABody, answersTrashed), and one Basecamp
names no day for takes the day it was written in the zone of whoever wrote it
(answersWithoutADay). Other tools and tools that are switched off are counted as
skipped. A mention and a file in a text become a reference, and so does a file
dropped into a chat. A video or a player embedded in a text, an iframe,
becomes an embed that carries its address and nothing else (embeds); one whose
address Minka would not take is left out (embedsLeftOut). A link to a card, a
message or a chat of the account, or to a line of a chat, becomes a reference
to that record, with its address and its words, whether the record is in the
bundle or not (references, by kind).

Every file a reference names comes with a record and its bytes under files/,
whatever its size and type, streamed from the Basecamp CLI (basecamp files
download). A file that is still whole in the bundle being replaced is taken
over from there instead of being fetched again; the report counts both as
filesFetched and filesReused. A file Basecamp lists and no longer has gets a
record that says so (filesGone). The name of a file is made a plain one
(fileNamesChanged), and no output ever says a name, a caption or an address.

The lines of chats are written in parts, messages-0001.jsonl and on, each
holding lines of one chat, and the manifest names them under parts. A part
ends once it holds 10,000 lines, where the second changes. Only a bundle
whose lines came to a single part carries the one file messages.jsonl.

The bundle is built next to --out and takes its place at the end, so a run
that fails leaves no half bundle. A bundle of an earlier run is replaced; a
directory that holds anything else, a bundle of another source included, is
refused. --dry-run walks Basecamp the
same way, downloads no avatar and no file, writes nothing and shows the counts
and what a run would fetch: filesExpected and filesBytesExpected.

A request that fails for a reason that may pass, such as a gateway in front
of Basecamp that fails for a moment, is asked again a few times with growing
pauses before the run ends, and at most 100 requests of a run are; the report
counts them as requestsRetried.

A file that cannot be fetched does not end the run: the bundle is written
without it, the report names it under missingFiles, and the command exits 6
when trying again may help, else 7. A later run brings what is missing. A
download that brings no byte for 60 seconds, or does not end within an hour,
is given up and counts as such a file. A Basecamp CLI that is not signed in
ends the run, for a file as for a request. A run that ends before its bundle
is in place loses the files it fetched: only a bundle in place hands files
over to the next run.

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

			// Every request of the run goes through the one source that asks
			// again, whatever kind it is of and whether anything is written.
			source := basecamp.NewRetrying(a.basecampSource, a.retryPause)
			extractor := basecamp.Extractor{Source: source, Avatars: a.avatarFetcher, Files: a.basecampFiles}
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
				Parts:          writer.Parts(),
				Avatars:        importAvatars{Count: result.Avatars},
				SkippedTools:   result.Skipped,
				MissingFiles:   result.Missing,

				InvalidAddresses:     result.InvalidAddresses,
				PeopleLeftOut:        result.PeopleLeftOut,
				CreatorProbesRefused: result.CreatorProbesRefused,

				CardsByStatus:      result.CardsByStatus,
				CardsOnHold:        result.CardsOnHold,
				CardsLeftOut:       result.CardsLeftOut,
				CommentsTrashed:    result.CommentsTrashed,
				MentionsUnresolved: result.Bodies.MentionsUnresolved,
				FilesInBodies:      result.Bodies.Files,
				FilesDropped:       result.Bodies.FilesDropped,

				FilesExpected:        result.Files.Expected,
				FilesBytesExpected:   result.Files.BytesExpected,
				FilesBytes:           result.Files.Bytes,
				FilesFetched:         result.Files.Fetched,
				FilesReused:          result.Files.Reused,
				FilesGone:            result.Files.Gone,
				FileNamesChanged:     result.Files.NamesChanged,
				CaptionsDropped:      result.Bodies.CaptionsDropped,
				FilesOfSeveralOwners: result.Files.SecondOwners,
				LinkPreviews:         result.Bodies.LinkPreviews,
				PicturesLinked:       result.Bodies.PicturesLinked,
				Embeds:               result.Bodies.Embeds,
				EmbedsLeftOut:        result.Bodies.EmbedsLeftOut,
				References:           result.Bodies.References,

				PostsByStatus:      result.PostsByStatus,
				PostsLeftOut:       result.PostsLeftOut,
				PostsWithEmptyBody: result.PostsWithEmptyBody,
				MessagesByType:     result.MessagesByType,
				UploadLinesLeftOut: result.UploadLinesLeftOut,

				QuestionsLeftOut:    result.QuestionsLeftOut,
				AnchorsInferred:     result.AnchorsInferred,
				AskedLeftOut:        result.AskedLeftOut,
				AnswersWithoutABody: result.AnswersWithoutABody,
				AnswersWithoutADay:  result.AnswersWithoutADay,
				AnswersTrashed:      result.AnswersTrashed,

				MessagesWithEmptyBody: result.MessagesWithEmptyBody,
				LongestBody:           result.LongestBody,
				RequestsRetried:       source.Retried(),
			}
			var notices []string
			if count := len(report.InvalidAddresses); count > 0 {
				notices = append(notices, plural(count, "person comes", "people come")+" without an e-mail address, and so without a login, because the one Basecamp holds is none; data.invalidAddresses names them by ID, and only the source can repair it")
			}
			if count := report.CreatorProbesRefused; count > 0 {
				notices = append(notices, plural(count,
					"tool of a project could not be asked who created the project, because Basecamp refuses it to this identity; a project may therefore come without a creator",
					"tools of projects could not be asked who created their project, because Basecamp refuses them to this identity; a project may therefore come without a creator"))
			}
			if count := report.FilesDropped; count > 0 {
				notices = append(notices, plural(count,
					"file embedded in a text or dropped into a chat could not be named, because Basecamp lists no file for it; of an embedded one only the caption stays in the text",
					"files embedded in texts or dropped into chats could not be named, because Basecamp lists no file for them; of an embedded one only the caption stays in the text"))
			}
			if count := report.FilesGone; count > 0 {
				notices = append(notices, plural(count,
					"file of a text or a chat is gone: Basecamp lists it and no longer has it, and its record in the bundle says so",
					"files of texts or chats are gone: Basecamp lists them and no longer has them, and their records in the bundle say so"))
			}
			if count := report.CaptionsDropped; count > 0 {
				notices = append(notices, plural(count,
					"caption of a file is left off its reference, because it is longer than 1000 characters; the file comes",
					"captions of files are left off their references, because they are longer than 1000 characters; the files come"))
			}
			if count := report.EmbedsLeftOut; count > 0 {
				notices = append(notices, plural(count,
					"embedded frame is left out of its text, because its address is none an embed may carry",
					"embedded frames are left out of their texts, because their addresses are none an embed may carry"))
			}
			if count := len(report.QuestionsLeftOut); count > 0 {
				notices = append(notices, plural(count,
					"question of a check-in is left out, because Minka's schedule has no words for what Basecamp asks; data.questionsLeftOut names it with the field that made it impossible, and its answers are left out with it",
					"questions of check-ins are left out, because Minka's schedule has no words for what Basecamp asks; data.questionsLeftOut names each with the field that made it impossible, and their answers are left out with them"))
			}
			if count := len(report.AnchorsInferred); count > 0 {
				notices = append(notices, plural(count,
					"question asked every other week is anchored on the day it was last answered, because Basecamp does not say which of two weeks it asks in; data.anchorsInferred names it with that day",
					"questions asked every other week are anchored on the days they were last answered, because Basecamp does not say which of two weeks they ask in; data.anchorsInferred names each with that day"))
			}
			if count := report.AskedLeftOut.NotInTheProject + report.AskedLeftOut.Retired + report.AskedLeftOut.WithoutAnAddress; count > 0 {
				notices = append(notices, plural(count,
					"person a question asks cannot be said to be asked, because they are no member of the project, have left, or come without an address; the question comes without them",
					"people questions ask cannot be said to be asked, because they are no members of their project, have left, or come without an address; the questions come without them"))
			}
			if count := report.AnswersWithoutADay; count > 0 {
				notices = append(notices, plural(count,
					"answer of a check-in belongs to no day of its own and is dated by the day it was written, read in the zone of whoever wrote it",
					"answers of check-ins belong to no day of their own and are dated by the days they were written, read in the zones of whoever wrote them"))
			}
			if count := report.AnswersWithoutABody; count > 0 {
				notices = append(notices, plural(count,
					"answer of a check-in is left out, because it says nothing",
					"answers of check-ins are left out, because they say nothing"))
			}
			if count := report.UploadLinesLeftOut; count > 0 {
				notices = append(notices, plural(count,
					"line of a chat is left out, because it was files alone and Basecamp names none of them",
					"lines of chats are left out, because they were files alone and Basecamp names none of them"))
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
				plural(report.Counts[bundle.Tools], "tool", "tools") + ", " +
				plural(report.Counts[bundle.Cards], "card", "cards") + ", " +
				plural(report.Counts[bundle.Posts], "post", "posts") + ", " +
				plural(report.Counts[bundle.Messages], "message", "messages")

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
	if report.CreatorProbesRefused > 0 {
		fmt.Fprintf(w, "Tools this identity may not see, asked for the creator of a project and passed over: %d\n", report.CreatorProbesRefused)
	}

	fmt.Fprintf(w, "\nCards: %d active, %d archived, %d trashed; %d of them on hold\n",
		report.CardsByStatus[bundle.StatusActive], report.CardsByStatus[bundle.StatusArchived], report.CardsByStatus[bundle.StatusTrashed], report.CardsOnHold)
	fmt.Fprintf(w, "Archived and trashed cards whose column is in no extracted board, left out: %d\n", report.CardsLeftOut)
	fmt.Fprintf(w, "Comments in the trash, left out: %d\n", report.CommentsTrashed)
	fmt.Fprintf(w, "Posts: %d active, %d archived, %d trashed; %d of them without a text\n",
		report.PostsByStatus[bundle.StatusActive], report.PostsByStatus[bundle.StatusArchived], report.PostsByStatus[bundle.StatusTrashed], report.PostsWithEmptyBody)
	fmt.Fprintf(w, "Archived and trashed posts of a message board that is not extracted, left out: %d\n", report.PostsLeftOut)
	fmt.Fprintf(w, "Messages: %d text, %d rich text, %d upload, %d integration\n",
		report.MessagesByType["text"], report.MessagesByType["richText"], report.MessagesByType["upload"], report.MessagesByType["integration"])
	fmt.Fprintf(w, "Lines that say nothing, left out: %d\n", report.MessagesWithEmptyBody)
	if names := report.Parts[bundle.Messages]; len(names) > 0 {
		fmt.Fprintf(w, "Messages are written in %d parts, %s to %s\n", len(names), names[0], names[len(names)-1])
	}
	fmt.Fprintf(w, "Lines that were files Basecamp names none of, left out: %d\n", report.UploadLinesLeftOut)
	renderCheckins(w, report)
	fmt.Fprintf(w, "In the texts: %s referred to, %s Basecamp lists nothing for dropped, %s, %s, %s\n",
		plural(report.FilesInBodies, "file", "files"), plural(report.FilesDropped, "file", "files"),
		plural(report.MentionsUnresolved, "mention left as a name", "mentions left as names"),
		plural(report.LinkPreviews, "preview of a link turned into the link", "previews of links turned into links"),
		plural(report.PicturesLinked, "picture at a foreign address turned into a link", "pictures at foreign addresses turned into links"))
	if report.Embeds > 0 {
		fmt.Fprintf(w, "Embedded frames turned into embeds: %d\n", report.Embeds)
	}
	if report.EmbedsLeftOut > 0 {
		fmt.Fprintf(w, "Embedded frames with an address no embed may carry, left out: %d\n", report.EmbedsLeftOut)
	}
	if references := report.References; references != (basecamp.ReferenceCounts{}) {
		fmt.Fprintf(w, "Links turned into references: %d to cards, %d to posts, %d to chats\n", references.Cards, references.Posts, references.Tools)
	}
	if report.DryRun {
		fmt.Fprintf(w, "Files a run would fetch: %d, %d bytes as Basecamp gives their sizes\n", report.FilesExpected, report.FilesBytesExpected)
	} else {
		fmt.Fprintf(w, "Files: %d of %d brought, %d bytes of %d expected; %d fetched, %d taken over from the bundle before, %d gone at Basecamp\n",
			report.FilesFetched+report.FilesReused, report.FilesExpected, report.FilesBytes, report.FilesBytesExpected,
			report.FilesFetched, report.FilesReused, report.FilesGone)
	}
	fmt.Fprintf(w, "Names of files changed: %d; captions too long for a reference, left off: %d; files met under a second owner: %d\n",
		report.FileNamesChanged, report.CaptionsDropped, report.FilesOfSeveralOwners)
	fmt.Fprintf(w, "Longest text in characters: cards %d, comments %d, posts %d, messages %d, answers %d\n",
		report.LongestBody[bundle.Cards], report.LongestBody[bundle.Comments], report.LongestBody[bundle.Posts], report.LongestBody[bundle.Messages],
		report.LongestBody[bundle.Answers])

	if report.RequestsRetried > 0 {
		fmt.Fprintf(w, "Requests asked again after a failure that passed: %d\n", report.RequestsRetried)
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

// renderCheckins says what became of the questions of the check-ins a run met.
// A run that met none says nothing about them.
func renderCheckins(w io.Writer, report importReport) {
	_, met := report.Counts[bundle.Questions]
	if !met && len(report.QuestionsLeftOut) == 0 {
		return
	}

	impossible := make([]string, 0, len(report.QuestionsLeftOut))
	for _, left := range report.QuestionsLeftOut {
		impossible = append(impossible, fmt.Sprintf("%s (%s)", left.QuestionSourceID, left.Field))
	}
	fmt.Fprintf(w, "Questions whose schedule Minka has no words for, left out: %d", len(impossible))
	if len(impossible) > 0 {
		fmt.Fprintf(w, " — %s", strings.Join(impossible, ", "))
	}
	fmt.Fprintln(w)

	if len(report.AnchorsInferred) > 0 {
		anchored := make([]string, 0, len(report.AnchorsInferred))
		for _, anchor := range report.AnchorsInferred {
			anchored = append(anchored, anchor.QuestionSourceID+" from "+anchor.From)
		}
		fmt.Fprintf(w, "Questions asked every other week anchored on the day they were last answered: %d — %s\n",
			len(anchored), strings.Join(anchored, ", "))
	}
	fmt.Fprintf(w, "People a question asks that the bundle cannot say are asked: %d no member of the project, %d no longer there, %d without an address\n",
		report.AskedLeftOut.NotInTheProject, report.AskedLeftOut.Retired, report.AskedLeftOut.WithoutAnAddress)
	fmt.Fprintf(w, "Answers that say nothing, left out: %d; answers in the trash, left out: %d\n",
		report.AnswersWithoutABody, report.AnswersTrashed)
	fmt.Fprintf(w, "Answers Basecamp names no day for, dated by when they were written: %d\n", report.AnswersWithoutADay)
}

// incompleteImport is the end of a run that wrote its bundle without some of
// the files. The bundle is whole and can be applied, so the report comes out
// as after any run; the failure tells a caller that a later run has something
// left to bring. It is worth trying again when a file failed for a reason that
// may pass by itself.
func incompleteImport(report importReport, notices []string) *Error {
	e := &Error{
		Code:    CodeAPI,
		Message: "the bundle was written, but " + plural(len(report.MissingFiles), "file could not be fetched and is left out", "files could not be fetched and are left out"),
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
	case basecamp.Unavailable:
		e.Retryable = true
		e.Hint = "Basecamp, or a gateway in front of it, failed several times in a row. That usually passes: try again in a few minutes."
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
