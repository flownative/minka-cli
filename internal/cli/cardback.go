package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"unicode"
	"unicode/utf8"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

const (
	apiCodeUploadNotAttachable = "uploadNotAttachable"
	apiCodeCardTakesNoChanges  = "cardTakesNoChanges"
)

// cardBackNotFoundHint says why the back of a card answers 404. Minka keeps
// the whole back for the team, so a client meets a card it sees the front of
// as missing here.
const cardBackNotFoundHint = "There is no such card, or its back is not yours to see: the back of a card, its notes and its attachments, is the team's alone, and a client of the project gets this answer for a card whose front it sees. A card in the trash has no back to read."

// cardBackFileNotFoundHint adds the file itself and an older Minka, which has
// no address for the files of a back at all.
const cardBackFileNotFoundHint = "There is no such attachment on the back of this card, or the card or its back is not yours to see: the back is the team's alone, and a client of the project gets this answer. List what is there with minka cards notes attachments list <cardId>. A Minka older than the files on the back of a card (not in a release yet) answers this for every attachment."

// cardBackHelp is the rule for the back of a card, said in the help of the
// group so that whoever writes the back meets it where the commands are.
const cardBackHelp = `The back of a card is the working memory of whoever works on the card, agents
above all: plan, intermediate state, handover. The front, the description and
the comments, is the assignment and the conversation, written for people. What
still holds after the card is done, a decision with its reason, a rule, a
how-to, belongs in the repository before the card goes to Done.

How to keep the notes:

  - In the language of the card's front.
  - The state, not a chronicle: rewrite what changed, drop what no longer
    holds. What happened is in the comments, in Git and the pull requests.
  - Written at the milestones: plan stands, a package merged, a handover,
    blocked. About 40 to 60 lines, a guide and no limit.
  - In this outline:
      Status · Plan (packages with their state) · Settled (by whom, when) ·
      Where things are (branch, worktree, migration, numbers, pull requests) ·
      Next step · Pitfalls
  - They stay when the card is done.

Research, concepts and screenshots go on the back as attachments, not into
files on one machine. Name each attachment in the notes with one line of what
it holds, so that whoever reads the back knows what to open.

The back is the team's alone: a client of the project gets exit 2
(not_found) for the notes and the attachments of a card whose front it sees.`

func (a *app) newCardsNotesAttachmentsCmd() *cobra.Command {
	return group("attachments", "Put files on the back of a card, read and remove them",
		a.newCardsNotesAttachmentsListCmd(),
		a.newCardsNotesAttachmentsCreateCmd(),
		a.newCardsNotesAttachmentsShowCmd(),
		a.newCardsNotesAttachmentsDownloadCmd(),
		a.newCardsNotesAttachmentsDeleteCmd(),
	)
}

// attachmentHelp says how a command names one attachment.
const attachmentHelp = `<attachment> is the ID of the attachment, its position in the list, or its
file name. A name is one file per back as written, case and all: plan.md and
Plan.md are two. The name as written wins; otherwise case is ignored where
that leaves one file. A name that is a number counts as the position.`

func (a *app) newCardsNotesAttachmentsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <cardId>",
		Short: "List the files on the back of a card",
		Long: `Lists the files on the back of a card by name, each as its newest version:
who put it there and when, its size and its type. Earlier versions are kept by
Minka but neither listed nor served.`,
		Example: `  minka cards notes attachments list MNK-3T
  minka cards notes attachments list 0199... --json`,
		Args: requireArgs("cardId"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			files, err := s.client.CardBackFiles(a.ctx, args[0])
			if err != nil {
				return cardBackRefusal(s, err, cardBackNotFoundHint+" A Minka older than the files on the back of a card (not in a release yet) answers this for every card.")
			}
			return a.ok(success{
				data:        files.Raw,
				summary:     plural(len(files.Value), "file", "files") + " on the back of the card",
				breadcrumbs: attachmentsBreadcrumbs(args[0], files.Value),
				text:        func(w io.Writer) { renderCardBackFiles(w, files.Value) },
			})
		},
	}
}

func (a *app) newCardsNotesAttachmentsCreateCmd() *cobra.Command {
	var blobID, name, contentType string

	cmd := &cobra.Command{
		Use:   "create <cardId> (<file> | --blob <blobId>)",
		Short: "Put a file on the back of a card",
		Long: `Puts a file of this machine on the back of a card. Under a name the back has
already, the file becomes that attachment's next version: it keeps its ID and
its address, and the earlier version is kept by Minka but no longer listed or
served. Writing notifies nobody.

Minka takes it in two requests: the file is stored in the project of the card
first, and then put on the back. Given a file, the command does both. It reads
the card and its back before the file travels, so a card that is not there,
whose back is not yours, or that lies in the archive costs no transfer. A step
that fails ends the run, and nothing after it is sent.

Where the file was stored and the second step was refused, the error hands
over the stored file under "data". Where the card was the reason, give --blob
<blobId> with its id in place of the file to try again without sending it
anew; where something took the upload in the meantime (uploadNotAttachable),
run the command again, which stores the file anew. --blob also takes a file
stored earlier in the project of the card with "uploads create --project". It
has to be one you stored and that nothing holds yet, or Minka refuses it with
uploadNotAttachable.

` + unattachedFileHelp + `

` + mediaTypeHelp,
		Example: `  minka cards notes attachments create MNK-3T research.md
  minka cards notes attachments create MNK-3T shot.png --name stage-plan.png
  minka cards notes attachments create MNK-3T --blob 0199...`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 2 {
				return usageErrorf(cmd, "unexpected argument %q", args[2])
			}
			return requireArgs("cardId")(cmd, args[:min(len(args), 1)])
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			storing := len(args) == 2
			switch {
			case storing == flags.Changed("blob"):
				return usageErrorf(cmd, "give a file or --blob, not both and not neither")
			case !storing:
				if err := requireID(cmd, "--blob", &blobID); err != nil {
					return err
				}
				for _, flag := range []string{"name", "content-type"} {
					if flags.Changed(flag) {
						return usageErrorf(cmd, "--%s belongs to storing a file; the file --blob names is stored already", flag)
					}
				}
			}

			var file *fileToStore
			if storing {
				var err error
				if file, err = openFileToStore(args[1], name, contentType); err != nil {
					return err
				}
				defer file.close()
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			cardID := args[0]

			var stored *api.Result[api.Upload]
			if storing {
				card, err := s.client.Card(a.ctx, cardID)
				if err != nil {
					return cardBackRefusal(s, err, cardBackNotFoundHint)
				}
				// A client of the project reads the front of the card and not
				// its back; the back tells, before the file travels, and
				// before the archive, as Minka asks.
				if _, err := s.client.CardBackFiles(a.ctx, cardID); err != nil {
					return cardBackRefusal(s, err, cardBackNotFoundHint+" A Minka older than the files on the back of a card (not in a release yet) answers this for every card.")
				}
				// Minka would refuse it only after the transfer, and leave the
				// stored file standing in nothing for days.
				if card.Value.Archived {
					return &Error{
						Code:    CodeValidation,
						APICode: apiCodeCardArchived,
						Message: "the card " + card.Value.Title + " lies in the archive of its board, nothing was sent",
						Hint:    cardBackArchivedHint,
					}
				}
				uploaded, err := s.client.UploadToProject(a.ctx, card.Value.ProjectID, file.filename, file.mediaType, file.content, file.size)
				if err != nil {
					return storeRefusal(s, err)
				}
				stored, blobID = &uploaded, uploaded.Value.ID
			}

			attached, err := s.client.AttachToCardBack(a.ctx, cardID, blobID)
			if err != nil {
				return attachRefusal(s, err, blobID, stored)
			}

			summary := fmt.Sprintf("Put %s on the back of the card", attached.Value.Filename)
			if attached.Value.Version > 1 {
				summary = fmt.Sprintf("Put %s on the back of the card as its version %d", attached.Value.Filename, attached.Value.Version)
			}
			return a.ok(success{
				data:        attached.Raw,
				summary:     summary,
				notices:     []string{"name the attachment in the notes with one line of what it holds: minka cards notes show " + cardID},
				breadcrumbs: cardBackFileBreadcrumbs(attached.Value),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\nAttachment: %s\n%s\n", summary, attached.Value.ID, attached.Value.AppURL)
				},
			})
		},
	}

	cmd.Flags().StringVar(&blobID, "blob", "", "ID of a file stored already, in place of a file of this machine")
	addFileToStoreFlags(cmd, &name, &contentType)

	return cmd
}

func (a *app) newCardsNotesAttachmentsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <cardId> <attachment>",
		Short: "Print a file on the back of a card as text",
		Long: `Prints the newest version of a file on the back of a card, as text and byte
for byte as it was stored. What the command has to say about it — the name,
the version, who put it there — goes to standard error. With --json, and
wherever standard output is no terminal, the answer is the envelope, with the
text as "text" of data beside the attachment as "file"; --styled prints the
text alone there as well. To put the file on disk, download is the command.

Only a text is printed: a file whose bytes are no UTF-8, or that carries a
zero byte, a picture, audio or video, is refused with exit 1 before anything
is printed. So is, outside the envelope, a text that carries control
characters other than tab, line feed and carriage return, which a terminal
would act on. Fetch either with "cards notes attachments download" instead.

` + attachmentHelp,
		Example: `  minka cards notes attachments show MNK-3T research.md
  minka cards notes attachments show MNK-3T 1 --styled | less
  minka cards notes attachments show MNK-3T 0199... --json`,
		Args: requireArgs("cardId", "attachment"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			file, err := a.attachmentOnTheBack(s, args[0], args[1])
			if err != nil {
				return err
			}
			if !mayBeText(file.Value.ContentType) {
				return notATextError(file.Value)
			}

			// Escape sequences in a text would be acted on by a terminal;
			// the envelope writes them out as escapes.
			printed := a.format() == formatText
			content, err := a.readTheAttachment(s, file.Value, printed)
			if err != nil {
				return err
			}

			summary := fmt.Sprintf("%s, %s", file.Value.Filename, attachmentDetails(file.Value))
			return a.ok(success{
				data:        map[string]any{"file": file.Raw, "text": string(content)},
				summary:     summary,
				breadcrumbs: cardBackFileBreadcrumbs(file.Value),
				// The text is the answer here, so it stands alone on its
				// stream, as the notes do.
				text: func(w io.Writer) {
					_, _ = w.Write(content)
					fmt.Fprintf(a.stderr, "\n%s.\n", summary)
				},
			})
		},
	}
}

func (a *app) newCardsNotesAttachmentsDownloadCmd() *cobra.Command {
	var out string
	var force bool

	cmd := &cobra.Command{
		Use:   "download <cardId> <attachment>",
		Short: "Fetch a file on the back of a card to disk",
		Long: `Fetches the newest version of a file on the back of a card and writes it to
disk, whatever it is.

Without -o the file is written into the current directory under the name it
has on the back, and a file of that name that is there already is not
touched: give --force to replace it, or -o to name another target. -o - writes
the bytes to standard output and nothing else, which --json and --quiet have
no room for.

` + attachmentHelp,
		Example: `  minka cards notes attachments download MNK-3T stage-plan.png
  minka cards notes attachments download MNK-3T 2 -o plan.png --force
  minka cards notes attachments download MNK-3T 0199... -o - > plan.png`,
		Args: requireArgs("cardId", "attachment"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// The bytes are the answer here, as with uploads download.
			toStandardOutput := out == "-"
			if toStandardOutput {
				if a.jsonFlag || a.quietFlag {
					return usageErrorf(cmd, "-o - puts the bytes on standard output, where --json and --quiet put their own answer")
				}
				a.bytesAreTheOutput = true
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			file, err := a.attachmentOnTheBack(s, args[0], args[1])
			if err != nil {
				return err
			}

			target := out
			if target == "" {
				if !isAPlainName(file.Value.Filename) {
					return &Error{
						Code:    CodeUsage,
						Message: fmt.Sprintf("the attachment is called %q, which is no name for a file here", file.Value.Filename),
						Hint:    "Name the target yourself: -o <path>.",
					}
				}
				target = file.Value.Filename
			}
			// A target that would be refused costs no transfer.
			if !toStandardOutput {
				if err := targetToWrite(target, force); err != nil {
					return err
				}
			}

			fetched, err := s.client.CardBackFileBytes(a.ctx, file.Value.CardID, file.Value.ID)
			if err != nil {
				return attachmentBytesRefusal(s, err)
			}
			defer fetched.Body.Close()

			written, err := a.writeTheBytes(target, toStandardOutput, force, file.Value.ByteSize, fetched.Body)
			if err != nil {
				return err
			}
			if written != file.Value.ByteSize {
				kept := "Nothing was kept."
				if toStandardOutput {
					kept = "What arrived is on standard output already and is not the whole file."
				}
				return shortAttachmentError(s, file.Value, written, kept)
			}

			where := target
			if toStandardOutput {
				where = "standard output"
			}
			summary := fmt.Sprintf("Fetched %s (version %d, %s) to %s", file.Value.Filename, file.Value.Version, fileSize(written), where)
			data := map[string]any{"file": file.Raw, "path": pathOrNothing(target), "byteSize": written}
			if toStandardOutput {
				data["path"] = nil
			}
			return a.ok(success{
				data:        data,
				summary:     summary,
				breadcrumbs: cardBackFileBreadcrumbs(file.Value),
				text: func(w io.Writer) {
					if toStandardOutput {
						w = a.stderr
					}
					fmt.Fprintf(w, "%s.\n", summary)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&out, "out", "o", "", "Where to write it, - for standard output (default: the name it has on the back)")
	flags.BoolVar(&force, "force", false, "Replace a file of that name that is already there")

	return cmd
}

func (a *app) newCardsNotesAttachmentsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <cardId> <attachment>",
		Short: "Take a file off the back of a card",
		Long: `Takes a file off the back of a card, without asking: every version of it, and
Minka discards their uploads in the same change, so nothing of it can be
fetched afterwards. Take its line out of the notes as well.

` + attachmentHelp,
		Example: `  minka cards notes attachments delete MNK-3T research.md`,
		Args:    requireArgs("cardId", "attachment"),
		RunE: func(_ *cobra.Command, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			if err := s.resolveCard(a.ctx, &args[0]); err != nil {
				return err
			}
			file, err := a.attachmentOnTheBack(s, args[0], args[1])
			if err != nil {
				return err
			}
			if err := s.client.RemoveFromCardBack(a.ctx, file.Value.CardID, file.Value.ID); err != nil {
				return cardBackRefusal(s, err, cardBackFileNotFoundHint)
			}

			summary := fmt.Sprintf("Took %s off the back of the card, with every version", file.Value.Filename)
			return a.ok(success{
				data:    file.Raw,
				summary: summary,
				breadcrumbs: []Breadcrumb{
					{Action: "notes", Cmd: "minka cards notes show " + file.Value.CardID, Description: "Read the notes, to take the attachment's line out"},
					{Action: "attachments", Cmd: "minka cards notes attachments list " + file.Value.CardID, Description: "List the files on the back of the card"},
				},
				text: func(w io.Writer) { fmt.Fprintf(w, "%s.\n", summary) },
			})
		},
	}
}

// attachmentOnTheBack finds the attachment a command names. An ID goes to
// Minka as it is; a position or a name is looked up on the back first.
func (a *app) attachmentOnTheBack(s *session, cardID, value string) (api.Result[api.CardBackFile], error) {
	if id, isAnID := asID(strings.TrimSpace(value)); isAnID {
		file, err := s.client.CardBackFile(a.ctx, cardID, id)
		if err != nil {
			return file, cardBackRefusal(s, err, cardBackFileNotFoundHint)
		}
		return file, nil
	}

	files, err := s.client.CardBackFiles(a.ctx, cardID)
	if err != nil {
		return api.Result[api.CardBackFile]{}, cardBackRefusal(s, err, cardBackNotFoundHint+" A Minka older than the files on the back of a card (not in a release yet) answers this for every card.")
	}
	ids, names := make([]string, len(files.Value)), make([]string, len(files.Value))
	for i, file := range files.Value {
		ids[i], names[i] = file.ID, file.Filename
	}
	at, _, err := findLine(namedLines{noun: "attachment", owner: "the card", ids: ids, titles: names, caseMatters: true, described: describeAttachments(files.Value)}, value)
	if err != nil {
		return api.Result[api.CardBackFile]{}, err
	}
	// What the list said about this one file is the answer a read of it would
	// give, so it is handed on as it came.
	// The same bytes were read as a list of attachments a moment ago, so they
	// split into as many elements.
	var raws []json.RawMessage
	_ = json.Unmarshal(files.Raw, &raws)
	return api.Result[api.CardBackFile]{Value: files.Value[at], Raw: raws[at]}, nil
}

// readTheAttachment fetches the bytes of the newest version whole, and no
// more than the attachment says it weighs. The first bytes are looked at
// before the rest is read, so that a file that is no text costs no more than
// them; printed says that the text goes to a terminal as it is.
func (a *app) readTheAttachment(s *session, file api.CardBackFile, printed bool) ([]byte, error) {
	fetched, err := s.client.CardBackFileBytes(a.ctx, file.CardID, file.ID)
	if err != nil {
		return nil, attachmentBytesRefusal(s, err)
	}
	defer fetched.Body.Close()

	content := make([]byte, 0, file.ByteSize+1)
	body := io.LimitReader(fetched.Body, file.ByteSize+1)
	head := make([]byte, min(file.ByteSize+1, textProbeBytes))
	read, err := io.ReadFull(body, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, &Error{Code: CodeNetwork, Message: "the bytes of the attachment did not arrive whole: " + err.Error(), Retryable: true}
	}
	content = append(content, head[:read]...)
	if !isTextSoFar(content, printed) {
		return nil, notATextError(file)
	}

	buffer := bytes.NewBuffer(content)
	if _, err := io.Copy(buffer, body); err != nil {
		return nil, &Error{Code: CodeNetwork, Message: "the bytes of the attachment did not arrive whole: " + err.Error(), Retryable: true}
	}
	content = buffer.Bytes()
	if written := int64(len(content)); written != file.ByteSize {
		return nil, shortAttachmentError(s, file, written, "Nothing was printed.")
	}
	if !isText(content, printed) {
		return nil, notATextError(file)
	}
	return content, nil
}

// textProbeBytes is how much of a file is looked at before the rest is read.
const textProbeBytes = 4096

// targetToWrite refuses, before the bytes travel, a target writeTheBytes
// would refuse after: a directory, or a file that is there without --force.
func targetToWrite(target string, force bool) error {
	entry, err := os.Lstat(target)
	switch {
	case err != nil:
		return nil
	case entry.IsDir():
		return &Error{Code: CodeUsage, Message: fmt.Sprintf("%s is a directory, not a file", target)}
	case !force:
		return &Error{
			Code:    CodeUsage,
			Message: fmt.Sprintf("%s is already there", target),
			Hint:    "Give --force to replace it, or -o <path> to write it somewhere else.",
		}
	}
	return nil
}

func shortAttachmentError(s *session, file api.CardBackFile, written int64, kept string) *Error {
	return &Error{
		Code:    CodeAPI,
		Message: fmt.Sprintf("%s sent %s of %s where the attachment says %s", s.host, fileSize(written), file.Filename, fileSize(file.ByteSize)),
		Hint:    kept + " Try again; if it repeats, the file on the server is not what it says it is.",
	}
}

func attachmentBytesRefusal(s *session, err error) *Error {
	e := cardBackRefusal(s, err, cardBackFileNotFoundHint)
	if e.APICode == api.CodeRedirectRefused {
		e.Hint = "The file is served from another address, and only an https one is followed there. Whoever runs " + s.host + " can say where its files come from."
	}
	return e
}

// mayBeText says whether a file of this stored type can be a text at all. A
// picture, audio and video are not, whatever their bytes look like.
func mayBeText(contentType string) bool {
	for _, prefix := range []string{"image/", "audio/", "video/"} {
		if strings.HasPrefix(strings.ToLower(contentType), prefix) {
			return false
		}
	}
	return true
}

// isText says whether bytes can be printed as text: UTF-8 without a zero byte,
// which no text of a person carries and every binary format soon does. Where
// the text goes to a terminal as it is, a control character other than tab,
// line feed and carriage return is refused as well: a terminal acts on it.
func isText(content []byte, printed bool) bool {
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return false
	}
	return !printed || !bytes.ContainsFunc(content, isAControlATerminalActsOn)
}

// isTextSoFar is isText for the beginning of a file, whose last character may
// be cut off where the beginning ends.
func isTextSoFar(head []byte, printed bool) bool {
	for cut := 0; cut < utf8.UTFMax && cut < len(head); cut++ {
		if isText(head[:len(head)-cut], printed) {
			return true
		}
	}
	return len(head) == 0
}

func isAControlATerminalActsOn(r rune) bool {
	return unicode.IsControl(r) && r != '\t' && r != '\n' && r != '\r'
}

func notATextError(file api.CardBackFile) *Error {
	return &Error{
		Code:    CodeUsage,
		Message: fmt.Sprintf("%s is no text (%s), nothing was printed", file.Filename, file.ContentType),
		Hint:    fmt.Sprintf("Fetch it to disk with: minka cards notes attachments download %s %s", file.CardID, file.ID),
	}
}

// cardBackArchivedHint says what an archived card allows of its back.
const cardBackArchivedHint = "The card lies in the archive of its board: its back can be read, and nothing is written on it until the card is back. Bring it back with: minka cards unarchive <cardId>"

// cardBackRefusal classifies a failed request on the back of a card.
func cardBackRefusal(s *session, err error, notFoundHint string) *Error {
	e := s.fail(err)
	switch {
	case e.Code == CodeNotFound:
		e.Hint = notFoundHint
	case e.APICode == apiCodeCardArchived:
		e.Hint = cardBackArchivedHint
	case e.APICode == "cardNotesHasInvisibleCharacters":
		e.Hint = "The notes were not written. " + invisibleCharactersHint
	case e.APICode == apiCodeCardTakesNoChanges:
		e.Hint = "The card takes no changes: its column or its board is no longer in use. Its back can still be read."
	}
	return e
}

// attachRefusal classifies an upload Minka did not put on the back. stored is
// the file this run stored a moment before, and nil where --blob named it.
func attachRefusal(s *session, err error, blobID string, stored *api.Result[api.Upload]) *Error {
	e := cardBackRefusal(s, err, cardBackNotFoundHint)
	switch {
	case e.APICode == apiCodeUploadNotAttachable && stored != nil:
		e.Hint = "The file was stored in this same run as yours and in the project of the card, so something took it in the meantime, and a file goes into one place only. Running the command again stores the file anew."
	case e.APICode == apiCodeUploadNotAttachable:
		e.Hint = "The upload has to be one you stored in the project of this card, and nothing may hold it yet: no text, no upload of Docs & Files, no other back. Minka tells no cause apart, and one swept after standing in nothing for seven days or more reads the same. Give the file itself in place of --blob to store it anew."
	}
	if stored == nil {
		return e
	}
	// The transfer is not lost: the file stands stored, and a second try can
	// name it rather than send it again.
	e.Data = stored.Raw
	if e.APICode != apiCodeUploadNotAttachable {
		e.Hint = strings.TrimSpace(e.Hint + " The file is stored as " + blobID + " and kept for at least seven days while nothing carries it: give --blob " + blobID + " in place of the file to try again without sending it anew.")
	}
	return e
}

// attachmentDetails says which version a file is and who put it there.
func attachmentDetails(file api.CardBackFile) string {
	return fmt.Sprintf("version %d, %s, %s, put there by %s, %s",
		file.Version, file.ContentType, fileSize(file.ByteSize), file.UploadedBy.Name, formatTime(file.UploadedAt))
}

func renderCardBackFiles(w io.Writer, files []api.CardBackFile) {
	if len(files) == 0 {
		fmt.Fprintln(w, "No files on the back of the card.")
		return
	}
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "#\tNAME\tVERSION\tTYPE\tSIZE\tPUT THERE\tBY\tID")
	for i, file := range files {
		fmt.Fprintf(table, "%d\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n", i+1, file.Filename, file.Version, file.ContentType,
			fileSize(file.ByteSize), formatTime(file.UploadedAt), file.UploadedBy.Name, file.ID)
	}
	_ = table.Flush()
}

func describeAttachments(files []api.CardBackFile) string {
	if len(files) == 0 {
		return "The back of the card has no files."
	}
	lines := make([]string, len(files))
	for i, file := range files {
		lines[i] = fmt.Sprintf("%d. %s (%s)", i+1, file.Filename, file.ID)
	}
	return "Attachments: " + strings.Join(lines, "; ")
}

func attachmentsBreadcrumbs(cardID string, files []api.CardBackFile) []Breadcrumb {
	crumbs := []Breadcrumb{
		{Action: "attach", Cmd: "minka cards notes attachments create " + cardID + " <file>", Description: "Put a file on the back of the card"},
	}
	if len(files) > 0 {
		crumbs = append(crumbs, Breadcrumb{Action: "show", Cmd: "minka cards notes attachments show " + cardID + " " + files[0].ID, Description: "Print " + files[0].Filename + " as text"})
	}
	return append(crumbs, Breadcrumb{Action: "notes", Cmd: "minka cards notes show " + cardID, Description: "Read the notes, which name each attachment"})
}

func cardBackFileBreadcrumbs(file api.CardBackFile) []Breadcrumb {
	return []Breadcrumb{
		{Action: "show", Cmd: "minka cards notes attachments show " + file.CardID + " " + file.ID, Description: "Print it as text"},
		{Action: "download", Cmd: "minka cards notes attachments download " + file.CardID + " " + file.ID, Description: "Fetch it to disk"},
		{Action: "notes", Cmd: "minka cards notes show " + file.CardID, Description: "Read the notes, which name each attachment"},
	}
}
