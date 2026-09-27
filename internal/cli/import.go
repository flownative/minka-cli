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
	// AddressesUnquoted counts the records a run took a pair of quotation marks
	// off the address of: the one place a run repairs what the source says
	// rather than passing it on. One mailbox written that way in many records
	// counts in each of them.
	AddressesUnquoted int `json:"addressesUnquoted"`
	// PeopleLeftOut are the people of the account no record of the bundle
	// refers to; counts.people is who was written.
	PeopleLeftOut int `json:"peopleLeftOut"`
	// CreatorProbesRefused are the dock entries asked for the creator of a
	// project that Basecamp refused: they are passed over, and a project may
	// come without a creator because of it.
	CreatorProbesRefused int `json:"creatorProbesRefused"`
	// PeopleAsClients are the people the bundle says are clients of the source and
	// ClientMemberships the memberships of a project that are theirs: Minka leaves
	// a client out of the members of every project.
	PeopleAsClients   int `json:"peopleAsClients"`
	ClientMemberships int `json:"clientMemberships"`
	// VisibilityLeftToTheContainer are the records the source says whether clients
	// see them although a bundle has no word for it of such a record: they follow
	// what they hang on.
	VisibilityLeftToTheContainer int `json:"visibilityLeftToTheContainer"`
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
	// the owners a file turned up under after the one it was fetched for: every
	// owner gets a record of its own, with the same bytes and one download.
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
	// TodoListsByStatus splits counts.todolists and TodosByStatus counts.todos.
	// TodosLeftOut counts per kind what the bundle does not carry of the To-dos
	// tools, and TodoPlacesLeftOut names the lists and groups that took records
	// with them.
	TodoListsByStatus map[string]int              `json:"todoListsByStatus"`
	TodosByStatus     map[string]int              `json:"todosByStatus"`
	TodosLeftOut      basecamp.TodosLeftOut       `json:"todosLeftOut"`
	TodoPlacesLeftOut []basecamp.TodoPlaceLeftOut `json:"todoPlacesLeftOut"`
	// TodosWithoutAList names the to-dos that hang on a To-dos tool itself and
	// stand in no list of it, which a bundle has no place for; they count in
	// TodosLeftOut as well.
	TodosWithoutAList []basecamp.TodoWithoutAList `json:"todosWithoutAList"`
	// TodosNotDone are the to-dos the source has open, and TodosWithPeople those
	// that name somebody: the two differences a later run cannot carry, because
	// it reopens nothing and leaves the people of a to-do as they stand.
	TodosNotDone    int `json:"todosNotDone"`
	TodosWithPeople int `json:"todosWithPeople"`
	// TodosWithSubtasks are the to-dos that had subtasks and SubtasksLeftOut how
	// many there were: a bundle carries none.
	TodosWithSubtasks int `json:"todosWithSubtasks"`
	SubtasksLeftOut   int `json:"subtasksLeftOut"`
	// TodosThatRepeat are the to-dos that come again at the source and are
	// carried as ordinary ones: a bundle has no word for a schedule.
	TodosThatRepeat int `json:"todosThatRepeat"`
	// QuestionsLeftOut are the questions left out because Minka has no words
	// for their schedule, each with the field that made it impossible.
	// AnchorsInferred are the questions asked every other week whose startsOn
	// was taken from the day an answer belongs to.
	QuestionsLeftOut []basecamp.ImpossibleQuestion `json:"questionsLeftOut"`
	AnchorsInferred  []basecamp.InferredAnchor     `json:"anchorsInferred"`
	// NobodyAsked are the questions that come asking nobody at all, each with
	// how many people the bundle could not name as asked.
	NobodyAsked []basecamp.NobodyAsked `json:"nobodyAsked"`
	// AskedLeftOut counts the people a question is asked of that the bundle
	// cannot say are asked.
	AskedLeftOut basecamp.AskedLeftOut `json:"askedLeftOut"`
	// AnswersWithoutABody are the answers that say nothing and AnswersTrashed
	// those in the trash; both are left out. AnswersWithoutADay are those
	// Basecamp names no day for, which take the day they were written on.
	AnswersWithoutABody int `json:"answersWithoutABody"`
	AnswersWithoutADay  int `json:"answersWithoutADay"`
	AnswersTrashed      int `json:"answersTrashed"`
	// Forwards says what became of the forwarded mail a run met: what it counts
	// is beside counts.forwards and counts.forwardreplies, which are what a
	// bundle carries.
	Forwards basecamp.ForwardCounts `json:"forwards"`
	// Docs says what became of the Docs & Files a run met, beside counts.folders,
	// counts.documents and counts.uploads.
	Docs basecamp.DocsCounts `json:"docs"`
	// MessagesByType splits counts.messages by what a line was in its chat.
	// MessagesWithEmptyBody are lines somebody wrote that say nothing,
	// UploadLinesLeftOut lines that were files alone, none of which can be
	// referred to: both are left out.
	MessagesByType        map[string]int `json:"messagesByType"`
	MessagesWithEmptyBody int            `json:"messagesWithEmptyBody"`
	UploadLinesLeftOut    int            `json:"uploadLinesLeftOut"`
	// RecordsLeftOut are the records a bundle cannot carry because the source
	// said a word it has none for, counted per kind of record, per word and per
	// place.
	RecordsLeftOut []basecamp.LeftOutRecords `json:"recordsLeftOut"`
	// LongestBody is the longest text per kind, in characters: a number and
	// nothing of the text. LongestBodyRecords names the record each number was
	// measured on, so that it can be looked at at the source: a kind, an
	// identifier and a place, and nothing of the text either. A kind whose
	// longest text has no characters names no record.
	LongestBody        map[bundle.Kind]int                        `json:"longestBody"`
	LongestBodyRecords map[bundle.Kind]basecamp.LongestBodyRecord `json:"longestBodyRecords"`
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

So far a bundle holds the structure, the boards, the message boards, the
chats, the check-ins, the to-dos, the forwarded mail and the Docs & Files: the
people the named
projects refer to with their avatars (their members and whoever a record names,
not the rest of the account), the projects with their members, of each project
the card tables, chats, message boards, questionnaires, to-do sets, inboxes and
Docs & Files tools
that are switched
on, of each card table the columns, the cards with the archived and the
trashed ones, their steps, comments and boosts, of each message board the
messages with the archived and the trashed ones as posts, their categories,
comments and boosts, of each chat its lines with their boosts as messages, of
each questionnaire its questions with their answers, comments and boosts, of
each to-do set its lists with their groups and to-dos, of each inbox its
forwarded mail with the answers by mail, the comments and the boosts of each,
and of each Docs & Files tool its folders, documents and uploads with the
comments, the boosts and the files of each.

The lists of a to-do set that are in use stand in the bundle from the bottom up,
because Minka puts every new list on top of its tool; the ones the source has put
away stand before them in the order it lists them, which decides nothing, Minka
ordering an archive by the moment a record was put away. The groups and the
to-dos of a list come in the order of the list. A list comes with the state the
source left it in, a
group and a to-do with the trash alone, because Minka gives neither of the two a
shelf: one the source has put on the shelf is left out and counted, and so is
whatever stood in a list or a group the bundle does not carry — todosLeftOut
counts the lists, groups and to-dos it holds none of, and todoPlacesLeftOut names
every place that took records with it. A to-do that hangs on a To-dos tool itself
and stands in no list of it has nowhere to go either: it is left out, and
todosWithoutAList names it with its project, so that somebody can move it into a
list at the source and a later run carries it. Nothing
hangs on a group, which the format has no word for. A to-do carries its notes,
its days, whom it is assigned to, who is told when it is done and the tick the
source put on it, and it is never said to be older than that tick. Subtasks are
not carried at all: todosWithSubtasks and subtasksLeftOut count what fell away.
A to-do that comes again at the source comes here as an ordinary to-do, because
Minka has no repeating one: the schedule stays behind and todosThatRepeat says how
many there were.
Two things a later run cannot repair, which the report says with a notice each:
it leaves the people of a to-do as they stand in Minka (todosWithPeople), and it
reopens nothing it once ticked off (todosNotDone).

The forwarded mail of an inbox comes after every other kind but the Docs &
Files, so that the bytes of a file standing in a card and in a mail alike are
fetched for the card. A mail carries its subject as its title, the sender as an
address that never becomes a person, and the body as the source holds it, line
breaks and all: Minka turns those into paragraphs. What the source keeps on its
shelf comes into the trash, where it
can be brought back, and the report says how many (forwards.fromTheShelf); a
mail of an inbox that is switched off is not in the bundle at all and counts as
a skipped tool. A subject nobody wrote becomes "(no subject)", and one that is
longer than the 200 characters a title holds, or that runs over several lines,
is folded into one line and cut short (forwards.withoutASubject,
forwards.subjectsChanged). A mail whose sender is no e-mail address Minka takes
is left out and named by ID (forwards.withoutAnAddress): the field takes nothing
else. Every address a bundle carries — a person's, a mail's sender, an answer's
— is read the way Minka reads it, and one pair of quotation marks around the
whole of an address comes off first (addressesUnquoted), which is the one thing
a run repairs rather than passes on. Where what a mail says would reach Minka
as nothing at all — no words, and a file that will never come — the run writes
a line of its own into the body (forwards.givenALineOfTheRun), because such a
mail is written empty there and lost on every run after this one.

An answer by mail is a record of its own and no comment. It belongs to somebody
of the account or, where it came from outside it, to the address alone, and
never to both: somebody from outside never becomes a person
(forwards.repliesFromOutside). One the source names neither for is left out and
named (forwards.repliesWithoutASender), and a boost on an answer is counted and
not carried (forwards.reactionsOnRepliesLeftOut), because Minka takes no
reaction on one. Comments and boosts hang on the mail itself, and a mail says
how many of neither, so both are asked for of every one of them. A body longer
than the 1,000,000 characters Minka holds is counted
(forwards.longerThanMinkaTakes) and comes as it is: Minka passes such a mail
over on every run, with everything hanging on it.

The Docs & Files of a project come last of all, so that the bytes of a file
standing in a card and in a document alike are fetched for the card. A folder, a
document and an upload each carry the state the source left them in — active,
archived or trashed — and never the state of the folder they stand in: what
stands in a
folder that was archived or thrown away goes along through the folder in Minka
and comes back with it. For an entry that is in use the order of the file
decides: Minka puts each new one on top of its kind, so the bundle carries them
from the bottom up and the source's order comes out again. For one that has left
its place it decides nothing — Minka orders an archive by the moment a record was
put away and takes that moment from the record itself — so those stand first and
in the order the source lists them. A folder stands after the folder it is in.
A folder whose status a bundle has no
word for is left out with everything below it, which is left out with it
(docs.leftOut counts the folders, documents and uploads, docs.placesLeftOut
names every place that took entries with it); so are the entries of a Docs &
Files tool that is switched off, which counts as a skipped tool. A folder the
source has put away that hands out no list of what stands in it is passed over
and counted (docs.foldersThatSaidNothingOfWhatIsInThem): what is in use below
such a folder reaches no bundle, and nothing can count it.

A document carries its title, its text, its files, its comments and its boosts.
An upload is one stored file beside a text: its title is what the source calls
it, and the name of its file stands in where the source calls it nothing
(docs.titlesFromTheFileName), because Minka derives nothing from a file. A title
Minka would refuse is folded to fit (docs.namesMadeIntoATitle), and a name too
long the way Minka shortens one: the extension stays and the stem is cut. Only the
file the source shows now comes; earlier copies of it stay at the source. An
upload Basecamp names no stored file for, one that lives in a cloud, is left out
and named by ID with its comments and its boosts
(docs.withoutAStoredFile): a comment pointing at an upload the bundle does not
carry would stop the applier. A record that lost a file no run will bring is
named (docs.onlyAFileNoRunWillBring), and the two kinds end differently because
the one field is optional and the other is not: a document whose whole text was
that file arrives without words, its title, comments and boosts with it,
while an upload cannot be written without its file at all and Minka asks for the
file again on every run. A file this run merely could not fetch is not that, and
the record waits for it. A document, or the description of an
upload, longer than the 1,000,000 characters Minka holds is counted
(docs.longerThanMinkaTakes) and comes as it is, as a mail does. A folder takes
neither comments nor boosts, here as in Basecamp, so neither is asked for.

A question comes with the schedule Minka has words for: the weekdays counted
from Monday, the time of day, the day the rhythm is counted from, and whom it is
asked of, which are the subscribers of the question and not whoever answered it.
Only a member of the project who is no client, is still there and has an address
can be asked (askedLeftOut). A question whose schedule Minka cannot express is
left out with its answers and named with the field that made it impossible
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
download). A file of a text or a chat is fetched at a storage address, the one
file of an upload of Docs & Files by the identifier of the upload and its
project: Basecamp names no stored file on such a record, and the address it gives
leads to a store of its own that no run follows.
A file that is still whole in the bundle being replaced is taken over from
there instead of being fetched again; the report counts both as filesFetched
and filesReused. A file Basecamp lists and no longer has gets a record that
says so (filesGone). Where one file stands under several owners, every one of
them gets a record of its own, named after the file and its owner, with the
same bytes and one download; filesOfSeveralOwners counts the owners beyond the
first. A file stands in one place in Minka, so a record naming another's file
is shown none of it where the two stand in different projects. The name of a
file is made a plain one (fileNamesChanged), and no output ever says a name, a
caption or an address.

What the source says about clients comes with it: whether somebody is a client
and which company they belong to, whether a project works with clients and with
which company, and whether the source shows a tool, a post, a to-do list or a
check-in question to clients. That word is always the record's own and never the
word of the tool it stands in: a post of a board the clients see can be one the
source keeps from them. Where the source says nothing, nothing is written and
Minka leaves what it holds alone. A record that follows what it hangs on — a
to-do, a group, a card, an answer, a comment — carries no word of its own, and
where the source offers one it is counted (visibilityLeftToTheContainer). A
client is no member of a project in Minka, so clientMemberships says how many
memberships the import leaves out and no question is ever said to be asked of
one. A folder, a document and an upload of Docs & Files carry the word at every
depth, and not only at the top of their tool: Minka reads it below the top as
well, and something hidden from clients below a folder the source shows makes
Minka hide that folder with everything in it. A company the source names
without an
identifier, or with a name Minka refuses, ends the run where the record that
names it comes in: writing the record without it would tell Minka to drop the
company it holds.

A record the source describes with a word a bundle has none for is left out
and the run goes on: a line of a chat of an unknown kind or with a status other
than active, a column of a kind a bundle has no role for, and a card, a post, a
comment, a question or an answer with a status a bundle does not know, a draft
among them. recordsLeftOut counts them per kind of record, per word and per
place, so a later run can bring what a bundle by then has a word for.

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
			extractor := basecamp.Extractor{Source: source, Avatars: a.avatarFetcher, Files: a.basecampFiles, Now: a.now}
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
				AddressesUnquoted:    result.AddressesUnquoted,
				PeopleLeftOut:        result.PeopleLeftOut,
				CreatorProbesRefused: result.CreatorProbesRefused,

				PeopleAsClients:              result.PeopleAsClients,
				ClientMemberships:            result.ClientMemberships,
				VisibilityLeftToTheContainer: result.VisibilityLeftToTheContainer,

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
				FilesOfSeveralOwners: result.Files.OwnersBeyondTheFirst,
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
				RecordsLeftOut:     result.RecordsLeftOut,

				TodoListsByStatus: result.TodoListsByStatus,
				TodosByStatus:     result.TodosByStatus,
				TodosLeftOut:      result.TodosLeftOut,
				TodoPlacesLeftOut: result.TodoPlacesLeftOut,
				TodosWithoutAList: result.TodosWithoutAList,
				TodosNotDone:      result.TodosNotDone,
				TodosWithPeople:   result.TodosWithPeople,
				TodosWithSubtasks: result.TodosWithSubtasks,
				SubtasksLeftOut:   result.SubtasksLeftOut,
				TodosThatRepeat:   result.TodosThatRepeat,

				QuestionsLeftOut:    result.QuestionsLeftOut,
				AnchorsInferred:     result.AnchorsInferred,
				NobodyAsked:         result.NobodyAsked,
				AskedLeftOut:        result.AskedLeftOut,
				AnswersWithoutABody: result.AnswersWithoutABody,
				AnswersWithoutADay:  result.AnswersWithoutADay,
				AnswersTrashed:      result.AnswersTrashed,

				Forwards: result.Forwards,
				Docs:     result.Docs,

				MessagesWithEmptyBody: result.MessagesWithEmptyBody,
				LongestBody:           result.LongestBody,
				LongestBodyRecords:    result.LongestBodyRecords,
				RequestsRetried:       source.Retried(),
			}
			var notices []string
			if count := len(report.InvalidAddresses); count > 0 {
				notices = append(notices, plural(count, "person comes", "people come")+" without an e-mail address, and so without a login, because the one Basecamp holds is none; data.invalidAddresses names them by ID, and only the source can repair it")
			}
			if count := report.AddressesUnquoted; count > 0 {
				notices = append(notices, plural(count,
					"record came with its e-mail address inside a pair of quotation marks, which the run took off before reading it: with them Minka takes no address at all, and they are the one thing a run repairs rather than passes on",
					"records came with an e-mail address inside a pair of quotation marks, which the run took off before reading them: with them Minka takes no address at all, and they are the one thing a run repairs rather than passes on. One mailbox written that way in many records counts in each of them"))
			}
			if count := report.CreatorProbesRefused; count > 0 {
				notices = append(notices, plural(count,
					"tool of a project could not be asked who created the project, because Basecamp refuses it to this identity; a project may therefore come without a creator",
					"tools of projects could not be asked who created their project, because Basecamp refuses them to this identity; a project may therefore come without a creator"))
			}
			if count := report.ClientMemberships; count > 0 {
				notices = append(notices, plural(count,
					"membership of a project belongs to a client, and Minka leaves a client out of the members of every project: the bundle says it, the import does not carry it over",
					"memberships of projects belong to clients, and Minka leaves a client out of the members of every project: the bundle says them, the import does not carry them over"))
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
			if count := len(report.NobodyAsked); count > 0 {
				notices = append(notices, plural(count,
					"question of a check-in comes asking nobody at all and will ask nobody again, because whom it is asked of is what the first run makes of it; data.nobodyAsked names it",
					"questions of check-ins come asking nobody at all and will ask nobody again, because whom they are asked of is what the first run makes of them; data.nobodyAsked names each"))
			}
			if count := askedLeftOut(report); count > 0 {
				notices = append(notices, plural(count,
					"person a question asks cannot be said to be asked, because they are no member of the project, are a client, have left, or come without an address; the question comes without them",
					"people questions ask cannot be said to be asked, because they are no members of their project, are clients, have left, or come without an address; the questions come without them"))
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
			if count := len(report.TodoPlacesLeftOut); count > 0 {
				notices = append(notices, plural(count,
					"list or group of a To-dos tool is left out with what stood in it, which is left out with it; data.todoPlacesLeftOut names it with how many groups and to-dos fell with it",
					"lists or groups of To-dos tools are left out with what stood in them, which is left out with them; data.todoPlacesLeftOut names each with how many groups and to-dos fell with it"))
			}
			if count := len(report.TodosWithoutAList); count > 0 {
				notices = append(notices, plural(count,
					"to-do hangs on a To-dos tool itself and stands in no list of it, which a bundle has no place for: it is left out, and data.todosWithoutAList names it with its project, so that somebody can move it into a list at the source and a later run carries it",
					"to-dos hang on To-dos tools themselves and stand in no list of them, which a bundle has no place for: they are left out, and data.todosWithoutAList names each with its project, so that somebody can move them into lists at the source and a later run carries them"))
			}
			if count := report.TodosThatRepeat; count > 0 {
				notices = append(notices, plural(count,
					"to-do comes again at the source and is carried as an ordinary one, because Minka has no repeating to-do: the schedule stays behind, and the to-do keeps the days it has",
					"to-dos come again at the source and are carried as ordinary ones, because Minka has no repeating to-do: their schedules stay behind, and the to-dos keep the days they have"))
			}
			if count := report.SubtasksLeftOut; count > 0 {
				notices = append(notices, plural(count,
					"subtask of a to-do is left out, because the format has no word for one; the to-do it hung on is poorer here than at the source, and data.todosWithSubtasks counts those to-dos",
					"subtasks of to-dos are left out, because the format has no word for them; the to-dos they hung on are poorer here than at the source, and data.todosWithSubtasks counts those to-dos"))
			}
			if count := report.TodosWithPeople; count > 0 {
				notices = append(notices, plural(count,
					"to-do names somebody as an assignee or as somebody to tell when it is done, and a later run leaves the people of a to-do as they stand in Minka: a change at the source never arrives",
					"to-dos name somebody as an assignee or as somebody to tell when they are done, and a later run leaves the people of a to-do as they stand in Minka: a change at the source never arrives"))
			}
			if count := report.TodosNotDone; count > 0 {
				notices = append(notices, plural(count,
					"to-do is open at the source, and a later run reopens nothing: one that somebody has ticked off in Minka stays done",
					"to-dos are open at the source, and a later run reopens nothing: one that somebody has ticked off in Minka stays done"))
			}
			if count := len(report.Forwards.WithoutAnAddress); count > 0 {
				notices = append(notices, plural(count,
					"forwarded mail is left out, because what the source calls its sender is no e-mail address Minka takes; data.forwards.withoutAnAddress names it by ID, the field takes nothing else, and only the source can repair it",
					"forwarded mails are left out, because what the source calls their senders are no e-mail addresses Minka takes; data.forwards.withoutAnAddress names them by ID, the field takes nothing else, and only the source can repair it"))
			}
			if count := len(report.Forwards.RepliesWithoutASender); count > 0 {
				notices = append(notices, plural(count,
					"answer by mail is left out, because the source names neither who wrote it nor an address it came from; data.forwards.repliesWithoutASender names it by ID",
					"answers by mail are left out, because the source names neither who wrote them nor an address they came from; data.forwards.repliesWithoutASender names them by ID"))
			}
			if count := report.Forwards.FromTheShelf; count > 0 {
				notices = append(notices, plural(count,
					"forwarded mail is on the shelf at the source and comes into the trash here, where it can be brought back: Minka gives a forward no shelf",
					"forwarded mails are on the shelf at the source and come into the trash here, where they can be brought back: Minka gives a forward no shelf"))
			}
			if count := report.Forwards.GivenALineOfTheRun; count > 0 {
				notices = append(notices, plural(count,
					"forwarded mail or answer by mail carries a line of the run's own, because what it says would reach Minka as nothing at all and a mail that arrives empty is lost on every run",
					"forwarded mails or answers by mail carry a line of the run's own, because what they say would reach Minka as nothing at all and a mail that arrives empty is lost on every run"))
			}
			if count := report.Forwards.WithoutASubject + report.Forwards.SubjectsChanged; count > 0 {
				notices = append(notices, plural(count,
					"forwarded mail carries a title the run made of its subject: a mail without one, or one that had to be folded into a single line or cut short, because a title here is one line of at most 200 characters",
					"forwarded mails carry titles the run made of their subjects: mails without one, or ones that had to be folded into a single line or cut short, because a title here is one line of at most 200 characters"))
			}
			if count := report.Forwards.WithoutAForwarder; count > 0 {
				notices = append(notices, plural(count,
					"forwarded mail names nobody who forwarded it, and Minka gives such a mail to the owner of the organization without a word",
					"forwarded mails name nobody who forwarded them, and Minka gives such a mail to the owner of the organization without a word"))
			}
			if count := report.Forwards.LongerThanMinkaTakes; count > 0 {
				notices = append(notices, plural(count,
					"forwarded mail or answer by mail is longer than Minka holds and is passed over there on every run, with its answers and its comments; data.longestBodyRecords names the longest of each kind, and the body is not cut short",
					"forwarded mails or answers by mail are longer than Minka holds and are passed over there on every run, with their answers and their comments; data.longestBodyRecords names the longest of each kind, and the bodies are not cut short"))
			}
			if count := report.Forwards.ReactionsOnRepliesLeftOut; count > 0 {
				notices = append(notices, plural(count,
					"boost on an answer by mail is left out, because Minka takes no reaction on one; the answer comes",
					"boosts on answers by mail are left out, because Minka takes no reaction on one; the answers come"))
			}
			if count := len(report.Docs.SilentAboutClientsInsideAFolder); count > 0 {
				notices = append(notices, plural(count,
					"folder, document or upload standing inside a folder says nothing about whether clients see it, so nothing is written for it and Minka reads it as one that changes nothing above it. Where the folder at the top of its tool is shown to clients, that word is the one thing which would have hidden it, so compare that entry against the source before the bundle is applied — data.docs.silentAboutClientsInsideAFolder names it — and where the top is hidden or says nothing itself, nothing is lost",
					"folders, documents or uploads standing inside folders say nothing about whether clients see them, so nothing is written for them and Minka reads them as entries that change nothing above them. Where the folder at the top of their tool is shown to clients, that word is the one thing which would have hidden it, so compare those entries against the source before the bundle is applied — data.docs.silentAboutClientsInsideAFolder names each — and where the top is hidden or says nothing itself, nothing is lost"))
			}
			if count := report.Docs.FoldersThatSaidNothingOfWhatIsInThem; count > 0 {
				notices = append(notices, plural(count,
					"folder of a Docs & Files tool would not say what stands in it, so what is in use below it is in no bundle and no later run finds it either; nothing can count it, and the folder itself comes",
					"folders of Docs & Files tools would not say what stands in them, so what is in use below them is in no bundle and no later run finds it either; nothing can count it, and the folders themselves come"))
			}
			if count := len(report.Docs.WithoutAStoredFile); count > 0 {
				notices = append(notices, plural(count,
					"upload of a Docs & Files tool is left out, because Basecamp names no stored file for it: what lies in a cloud is no file a run can fetch, so its comments and its boosts go with it. data.docs.withoutAStoredFile names it by ID",
					"uploads of Docs & Files tools are left out, because Basecamp names no stored file for them: what lies in a cloud is no file a run can fetch, so their comments and their boosts go with them. data.docs.withoutAStoredFile names them by ID"))
			}
			if count := len(report.Docs.OnlyAFileNoRunWillBring); count > 0 {
				notices = append(notices, plural(count,
					"record of a Docs & Files tool lost a file no run will bring; data.docs.onlyAFileNoRunWillBring names it by ID. A document whose whole text was that file arrives without words, its title, comments and boosts with it; an upload whose own file it is cannot be written at all and Minka asks for the file again on every run",
					"records of Docs & Files tools lost a file no run will bring; data.docs.onlyAFileNoRunWillBring names them by ID. A document whose whole text was that file arrives without words, its title, comments and boosts with it; an upload whose own file it is cannot be written at all and Minka asks for the file again on every run"))
			}
			if count := report.Docs.LongerThanMinkaTakes; count > 0 {
				notices = append(notices, plural(count,
					"document, or description of an upload, is longer than Minka holds and is passed over there on every run, with its comments and its boosts; data.longestBodyRecords names the longest of each kind, and the text is not cut short",
					"documents, or descriptions of uploads, are longer than Minka holds and are passed over there on every run, with their comments and their boosts; data.longestBodyRecords names the longest of each kind, and the texts are not cut short"))
			}
			if count := report.Docs.TitlesFromTheFileName; count > 0 {
				notices = append(notices, plural(count,
					"upload is called nothing at the source, so the name of its file is its title here: Minka derives nothing from a file and refuses an upload without one. data.docs.withoutAName counts those with neither, which carry a title of the run's own",
					"uploads are called nothing at the source, so the names of their files are their titles here: Minka derives nothing from a file and refuses an upload without one. data.docs.withoutAName counts those with neither, which carry a title of the run's own"))
			}
			if count := report.Docs.NamesMadeIntoATitle; count > 0 {
				notices = append(notices, plural(count,
					"upload carries a title the run had to fold to fit: whatever it was called — what the source calls it, or the name of its file where the source calls it nothing — held a character nobody can see or was longer than the 200 characters a title here holds, and a name too long is shortened the way Minka shortens one, the extension staying and the stem cut",
					"uploads carry titles the run had to fold to fit: whatever they were called — what the source calls them, or the names of their files where the source calls them nothing — held a character nobody can see or were longer than the 200 characters a title here holds, and a name too long is shortened the way Minka shortens one, the extension staying and the stem cut"))
			}
			if count := len(report.Docs.PlacesLeftOut); count > 0 {
				notices = append(notices, plural(count,
					"folder or Docs & Files tool is left out with what stood in it, which is left out with it; data.docs.placesLeftOut names it with how many folders, documents and uploads fell with it",
					"folders or Docs & Files tools are left out with what stood in them, which is left out with them; data.docs.placesLeftOut names each with how many folders, documents and uploads fell with it"))
			}
			if count := recordsLeftOut(report); count > 0 {
				notices = append(notices, plural(count,
					"record is left out, because the source describes it with a word a bundle has none for; data.recordsLeftOut names the kind of record, the word and where it stood, and a later run brings it once a bundle has a word for it",
					"records are left out, because the source describes them with words a bundle has none for; data.recordsLeftOut names each kind of record with the word and where it stood, and a later run brings them once a bundle has words for them"))
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

// measuredKinds are the kinds whose longest text a run measures, in the order
// the report names them.
var measuredKinds = []bundle.Kind{bundle.Cards, bundle.Comments, bundle.Posts, bundle.Messages, bundle.Answers, bundle.TodoLists, bundle.Todos,
	bundle.Forwards, bundle.ForwardReplies, bundle.Documents, bundle.Uploads}

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
	if report.AddressesUnquoted > 0 {
		fmt.Fprintf(w, "Records whose address a pair of quotation marks came off: %d\n", report.AddressesUnquoted)
	}
	if report.CreatorProbesRefused > 0 {
		fmt.Fprintf(w, "Tools this identity may not see, asked for the creator of a project and passed over: %d\n", report.CreatorProbesRefused)
	}

	if report.PeopleAsClients > 0 || report.ClientMemberships > 0 {
		fmt.Fprintf(w, "Clients: %d, with %d memberships Minka leaves out of their projects\n", report.PeopleAsClients, report.ClientMemberships)
	}
	if report.VisibilityLeftToTheContainer > 0 {
		fmt.Fprintf(w, "Words about clients on a record that follows what it hangs on, left out: %d\n", report.VisibilityLeftToTheContainer)
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
	if count := recordsLeftOut(report); count > 0 {
		fmt.Fprintf(w, "Records the source describes with a word a bundle has none for, left out: %d\n", count)
		for _, one := range report.RecordsLeftOut {
			fmt.Fprintf(w, "  %-8s %-6s %-24s in %s %s: %d\n", one.Record, one.Field, one.Value, one.In, one.InSourceID, one.LeftOut)
		}
	}
	renderCheckins(w, report)
	renderTodos(w, report)
	renderForwards(w, report)
	renderDocs(w, report)
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
	fmt.Fprintf(w, "Names of files changed: %d; captions too long for a reference, left off: %d; owners of a file beyond the first: %d\n",
		report.FileNamesChanged, report.CaptionsDropped, report.FilesOfSeveralOwners)
	fmt.Fprintf(w, "Longest text in characters: cards %d, comments %d, posts %d, messages %d, answers %d, to-do lists %d, to-dos %d, forwards %d, answers by mail %d, documents %d, uploads %d\n",
		report.LongestBody[bundle.Cards], report.LongestBody[bundle.Comments], report.LongestBody[bundle.Posts], report.LongestBody[bundle.Messages],
		report.LongestBody[bundle.Answers], report.LongestBody[bundle.TodoLists], report.LongestBody[bundle.Todos],
		report.LongestBody[bundle.Forwards], report.LongestBody[bundle.ForwardReplies],
		report.LongestBody[bundle.Documents], report.LongestBody[bundle.Uploads])
	for _, kind := range measuredKinds {
		if one, has := report.LongestBodyRecords[kind]; has {
			fmt.Fprintf(w, "  %-8s %-8s in %s %s: %d\n", one.Record, one.SourceID, one.In, one.InSourceID, one.Characters)
		}
	}

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

// askedLeftOut is how many people a run could not say a question asks, over
// every reason.
func askedLeftOut(report importReport) int {
	left := report.AskedLeftOut
	return left.NotInTheProject + left.Clients + left.Retired + left.WithoutAnAddress
}

// recordsLeftOut is how many records a run left out for want of a word for what
// the source said of them, over every kind of record, word and place. It counts
// records and not entries: one entry may stand for many.
func recordsLeftOut(report importReport) int {
	count := 0
	for _, one := range report.RecordsLeftOut {
		count += one.LeftOut
	}
	return count
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
	fmt.Fprintf(w, "People a question asks that the bundle cannot say are asked: %d no member of the project, %d a client, %d no longer there, %d without an address\n",
		report.AskedLeftOut.NotInTheProject, report.AskedLeftOut.Clients, report.AskedLeftOut.Retired, report.AskedLeftOut.WithoutAnAddress)
	if len(report.NobodyAsked) > 0 {
		asking := make([]string, 0, len(report.NobodyAsked))
		for _, question := range report.NobodyAsked {
			asking = append(asking, fmt.Sprintf("%s (%d left out)", question.QuestionSourceID, question.LeftOut))
		}
		fmt.Fprintf(w, "Questions that come asking nobody at all: %d — %s\n", len(asking), strings.Join(asking, ", "))
	}
	fmt.Fprintf(w, "Answers that say nothing, left out: %d; answers in the trash, left out: %d\n",
		report.AnswersWithoutABody, report.AnswersTrashed)
	fmt.Fprintf(w, "Answers Basecamp names no day for, dated by when they were written: %d\n", report.AnswersWithoutADay)
}

// renderTodos says what became of the to-do lists a run met. A run that met no
// To-dos tool says nothing about them.
func renderTodos(w io.Writer, report importReport) {
	_, met := report.Counts[bundle.TodoLists]
	if !met && report.TodosLeftOut == (basecamp.TodosLeftOut{}) {
		return
	}
	fmt.Fprintf(w, "To-do lists: %d active, %d archived, %d trashed\n",
		report.TodoListsByStatus[bundle.StatusActive], report.TodoListsByStatus[bundle.StatusArchived], report.TodoListsByStatus[bundle.StatusTrashed])
	fmt.Fprintf(w, "To-dos: %d active, %d trashed; %d of them still open, %d carrying people\n",
		report.TodosByStatus[bundle.StatusActive], report.TodosByStatus[bundle.StatusTrashed], report.TodosNotDone, report.TodosWithPeople)
	fmt.Fprintf(w, "To-dos that had subtasks: %d, with %d subtasks a bundle has no word for\n", report.TodosWithSubtasks, report.SubtasksLeftOut)
	fmt.Fprintf(w, "To-dos that come again at the source, carried as ordinary ones: %d\n", report.TodosThatRepeat)
	left := report.TodosLeftOut
	fmt.Fprintf(w, "Left out of the To-dos tools: %d lists, %d groups, %d to-dos\n", left.Lists, left.Groups, left.Todos)
	for _, place := range report.TodoPlacesLeftOut {
		fmt.Fprintf(w, "  with %s %s fell: %d groups, %d to-dos\n", place.Record, place.SourceID, place.Groups, place.Todos)
	}
	if len(report.TodosWithoutAList) > 0 {
		named := make([]string, 0, len(report.TodosWithoutAList))
		for _, one := range report.TodosWithoutAList {
			named = append(named, one.SourceID+" of project "+one.ProjectSourceID)
		}
		fmt.Fprintf(w, "To-dos hanging on a To-dos tool itself and in no list of it, left out: %d — %s\n",
			len(named), strings.Join(named, ", "))
	}
}

// renderForwards says what became of the forwarded mail a run met. A run that
// met no Email forwards tool says nothing about it.
func renderForwards(w io.Writer, report importReport) {
	forwards := report.Forwards
	if _, met := report.Counts[bundle.Forwards]; !met && len(forwards.WithoutAnAddress) == 0 {
		return
	}
	fmt.Fprintf(w, "Forwarded mail: %d active, %d trashed; %d of them on the shelf at the source\n",
		forwards.ByStatus[bundle.StatusActive], forwards.ByStatus[bundle.StatusTrashed], forwards.FromTheShelf)
	fmt.Fprintf(w, "Mails without a subject: %d; subjects folded or cut short to make a title: %d\n",
		forwards.WithoutASubject, forwards.SubjectsChanged)
	fmt.Fprintf(w, "Mails and answers carrying a line of the run's own, because what they say would arrive as nothing: %d\n", forwards.GivenALineOfTheRun)
	fmt.Fprintf(w, "Mails nobody is named for, which Minka gives to the owner of the organization: %d\n", forwards.WithoutAForwarder)
	if len(forwards.WithoutAnAddress) > 0 {
		fmt.Fprintf(w, "Mails whose sender is no address Minka takes, left out: %d (forward %s)\n",
			len(forwards.WithoutAnAddress), strings.Join(forwards.WithoutAnAddress, ", "))
	}
	fmt.Fprintf(w, "Answers by mail: %d, %d of them from outside the account\n",
		report.Counts[bundle.ForwardReplies], forwards.RepliesFromOutside)
	if len(forwards.RepliesWithoutASender) > 0 {
		fmt.Fprintf(w, "Answers naming neither a writer nor an address, left out: %d (reply %s)\n",
			len(forwards.RepliesWithoutASender), strings.Join(forwards.RepliesWithoutASender, ", "))
	}
	fmt.Fprintf(w, "Boosts on an answer by mail, which Minka takes none of, left out: %d\n", forwards.ReactionsOnRepliesLeftOut)
	if forwards.LongerThanMinkaTakes > 0 {
		fmt.Fprintf(w, "Mails and answers longer than Minka holds, which it passes over on every run: %d\n", forwards.LongerThanMinkaTakes)
	}
}

// renderDocs says what became of the Docs & Files a run met. A run that met no
// Docs & Files tool says nothing about them, and one that met a tool whose
// entries are all left out says what became of them: a report that fell silent
// there would be the one place a whole tool goes missing without a word.
func renderDocs(w io.Writer, report importReport) {
	docs := report.Docs
	metNothing := report.Counts[bundle.Folders] == 0 && report.Counts[bundle.Documents] == 0 && report.Counts[bundle.Uploads] == 0 &&
		docs.LeftOut == (basecamp.DocsLeftOut{}) && len(docs.WithoutAStoredFile) == 0 &&
		len(docs.OnlyAFileNoRunWillBring) == 0 && docs.FoldersThatSaidNothingOfWhatIsInThem == 0 &&
		len(docs.SilentAboutClientsInsideAFolder) == 0
	if metNothing {
		return
	}
	fmt.Fprintf(w, "Folders: %d active, %d archived, %d trashed\n",
		docs.FoldersByStatus[bundle.StatusActive], docs.FoldersByStatus[bundle.StatusArchived], docs.FoldersByStatus[bundle.StatusTrashed])
	fmt.Fprintf(w, "Documents: %d active, %d archived, %d trashed; %d of them without words\n",
		docs.DocumentsByStatus[bundle.StatusActive], docs.DocumentsByStatus[bundle.StatusArchived], docs.DocumentsByStatus[bundle.StatusTrashed],
		docs.DocumentsWithoutWords)
	fmt.Fprintf(w, "Uploads: %d active, %d archived, %d trashed; %d titled by the name of their file, %d of them with neither\n",
		docs.UploadsByStatus[bundle.StatusActive], docs.UploadsByStatus[bundle.StatusArchived], docs.UploadsByStatus[bundle.StatusTrashed],
		docs.TitlesFromTheFileName, docs.WithoutAName)
	if docs.NamesMadeIntoATitle > 0 {
		fmt.Fprintf(w, "Titles of uploads folded into one line or cut short to fit: %d\n", docs.NamesMadeIntoATitle)
	}
	if len(docs.WithoutAStoredFile) > 0 {
		fmt.Fprintf(w, "Uploads Basecamp names no stored file for, left out: %d (upload %s), with %d comments and %d boosts\n",
			len(docs.WithoutAStoredFile), strings.Join(docs.WithoutAStoredFile, ", "),
			docs.CommentsWithoutTheirUpload, docs.BoostsWithoutTheirUpload)
	}
	if len(docs.OnlyAFileNoRunWillBring) > 0 {
		fmt.Fprintf(w, "Records that lost a file no run will bring, the documents among them wordless: %d (record %s)\n",
			len(docs.OnlyAFileNoRunWillBring), strings.Join(docs.OnlyAFileNoRunWillBring, ", "))
	}
	if docs.LongerThanMinkaTakes > 0 {
		fmt.Fprintf(w, "Documents and descriptions longer than Minka holds, which it passes over on every run: %d\n", docs.LongerThanMinkaTakes)
	}
	if len(docs.SilentAboutClientsInsideAFolder) > 0 {
		fmt.Fprintf(w, "Entries inside a folder that say nothing about whether clients see them, which cannot hide the folder above them: %d\n",
			len(docs.SilentAboutClientsInsideAFolder))
		for _, entry := range docs.SilentAboutClientsInsideAFolder {
			fmt.Fprintf(w, "  %-8s %-8s in project %s\n", entry.Record, entry.SourceID, entry.ProjectSourceID)
		}
	}
	if docs.FoldersThatSaidNothingOfWhatIsInThem > 0 {
		fmt.Fprintf(w, "Folders that would not say what stands in them, so that what is in use below them is in no bundle: %d\n",
			docs.FoldersThatSaidNothingOfWhatIsInThem)
	}
	left := docs.LeftOut
	fmt.Fprintf(w, "Left out of the Docs & Files tools: %d folders, %d documents, %d uploads\n", left.Folders, left.Documents, left.Uploads)
	for _, place := range docs.PlacesLeftOut {
		fmt.Fprintf(w, "  with %s %s fell: %d folders, %d documents, %d uploads\n", place.Record, place.SourceID, place.Folders, place.Documents, place.Uploads)
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
