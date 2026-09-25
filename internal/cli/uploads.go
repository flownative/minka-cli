package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/flownative/minka-cli/internal/api"
	"github.com/spf13/cobra"
)

// maxUploadBytes is what Minka takes for one file. The server decides it and
// refuses a larger file with 413, but a file that is certainly too big is
// better refused before it travels.
const maxUploadBytes = 100 << 20

// maxUploadSize is how that limit is said. Every size this command prints is
// counted in thousands, which turns a round limit into 104.9 MB and leaves the
// reader to work out what would fit.
const maxUploadSize = "100 MiB"

func (a *app) newUploadsCmd() *cobra.Command {
	return group("uploads", "Put files into a project or a conversation and hang them in texts",
		a.newUploadsCreateCmd(),
		a.newUploadsDownloadCmd(),
	)
}

func (a *app) newUploadsDownloadCmd() *cobra.Command {
	var variant, out string
	var force bool

	cmd := &cobra.Command{
		Use:   "download <blobId>",
		Short: "Fetch the bytes of a file of Minka",
		Long: `Fetches a file of a project or a private conversation and writes it to disk.

The identifier is the id of an attachment: cards show, comments and messages
name the files hanging in their text under "attachments", each with the id
this command takes. The address names the file alone; which circle it is in
decides who may have it.

Without -o the file is written into the current directory under the name it
has in Minka, and a file of that name that is there already is not touched:
give --force to replace it, or -o to name another target. -o - writes the
bytes to standard output and nothing else, which --json and --quiet have no
room for.

--variant asks for one rendering. Every file has "original", the file as it
was uploaded. A picture wide enough to need one also has "display", the
smaller rendering; a small picture and everything that is no picture have the
original alone, and asking for a rendering a file has not got is refused
before any byte travels.`,
		Example: `  minka uploads download 0199...
  minka uploads download 0199... --variant display -o thumbnail.jpg
  minka uploads download 0199... -o - > plan.png`,
		Args: requireArgs("blobId"),
		RunE: func(cmd *cobra.Command, args []string) error {
			// The bytes are the answer here, so standard output carries them
			// and nothing else. A caller that asked for an envelope there
			// asked for two answers in one stream; from here on everything
			// this command has to say, a failure included, goes the other way.
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
			file, err := s.client.File(a.ctx, args[0])
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "There is no such file, or it hangs in a project or a conversation you are not in. The id comes from the attachments of a card, a comment or a message."
				}
				return e
			}

			wanted, found := renderingOf(file.Value, variant)
			if !found {
				return &Error{
					Code:    CodeUsage,
					Message: fmt.Sprintf("%s has no rendering %q", file.Value.Filename, variant),
					Hint:    "It has " + renderingNames(file.Value) + ". A small picture and everything that is no picture have the original alone.",
				}
			}

			target := out
			if target == "" {
				if !isAPlainName(file.Value.Filename) {
					return &Error{
						Code:    CodeUsage,
						Message: fmt.Sprintf("%s is called %q in Minka, which is no name for a file here", args[0], file.Value.Filename),
						Hint:    "Name the target yourself: -o <path>.",
					}
				}
				target = file.Value.Filename
			}

			fetched, err := s.client.FileBytes(a.ctx, args[0], wanted.Name)
			if err != nil {
				e := s.fail(err)
				switch e.APICode {
				case apiCodeVariantNotAvailable:
					e.Hint = "The rendering was gone by the time the bytes were asked for. Fetch the original instead: --variant original"
				case api.CodeRedirectRefused:
					e.Hint = "The file is served from another address, and only an https one is followed there. Whoever runs " + s.host + " can say where its files come from."
				}
				return e
			}
			defer fetched.Body.Close()

			written, err := a.writeTheBytes(target, toStandardOutput, force, wanted.ByteSize, fetched.Body)
			if err != nil {
				return err
			}
			if written != wanted.ByteSize {
				kept := "Nothing was kept."
				if toStandardOutput {
					kept = "What arrived is on standard output already and is not the whole file."
				}
				return &Error{
					Code: CodeAPI,
					Message: fmt.Sprintf("%s sent %s of %s where the file says %s",
						s.host, fileSize(written), file.Value.Filename, fileSize(wanted.ByteSize)),
					Hint: kept + " Try again; if it repeats, the file on the server is not what it says it is.",
				}
			}

			where := target
			if toStandardOutput {
				where = "standard output"
			}
			summary := fmt.Sprintf("Fetched %s (%s, %s, %s) to %s",
				file.Value.Filename, wanted.Name, wanted.ContentType, fileSize(written), where)

			data := map[string]any{
				"file":     file.Raw,
				"variant":  wanted.Name,
				"path":     pathOrNothing(target),
				"byteSize": written,
			}
			if toStandardOutput {
				data["path"] = nil
			}

			return a.ok(success{
				data:        data,
				summary:     summary,
				breadcrumbs: renderingBreadcrumbs(file.Value, wanted.Name),
				text: func(w io.Writer) {
					// Standard output carries the bytes, so the word about
					// them goes the other way.
					if toStandardOutput {
						w = a.stderr
					}
					fmt.Fprintf(w, "%s.\n", summary)
				},
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&variant, "variant", "original", "Rendering to fetch: original, or display where a picture has one")
	flags.StringVarP(&out, "out", "o", "", "Where to write it, - for standard output (default: the name it has in Minka)")
	flags.BoolVar(&force, "force", false, "Replace a file of that name that is already there")

	return cmd
}

// writeTheBytes puts the file where it was asked for and says how much
// arrived. One byte more than the file says it weighs is read, so that an
// answer which does not stop is noticed instead of filling the disk.
//
// A file that is already there is never written through: a download must not
// destroy what somebody has. What was half written is taken away again, so
// that no stump is left behind under the name of a whole file.
func (a *app) writeTheBytes(target string, toStandardOutput, force bool, byteSize int64, content io.Reader) (int64, error) {
	content = io.LimitReader(content, byteSize+1)
	if toStandardOutput {
		written, err := io.Copy(a.stdout, content)
		if err != nil {
			return written, &Error{Code: CodeNetwork, Message: "the bytes of the file did not arrive whole: " + err.Error(), Retryable: true}
		}
		return written, nil
	}

	if entry, err := os.Lstat(target); err == nil && entry.IsDir() {
		return 0, &Error{Code: CodeUsage, Message: fmt.Sprintf("%s is a directory, not a file", target)}
	}
	// The name of the file comes out of an answer, so the server has a say in
	// which entry of this directory is meant. --force therefore takes that
	// entry away first — the link itself, never what it points at — and the
	// bytes then land in a file of this command's own making, as they do
	// without --force: created and nothing else, so that a link is never
	// written through and the mode below is the mode the file really has.
	if force {
		if err := os.Remove(target); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return 0, &Error{Code: CodeUsage, Message: fmt.Sprintf("cannot replace %s: %v", target, err)}
		}
	}
	// The file comes out of a project only its members see, and it is written
	// so that it stays that way.
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		e := &Error{Code: CodeUsage, Message: fmt.Sprintf("cannot write %s: %v", target, err)}
		if errors.Is(err, fs.ErrExist) {
			e.Message = fmt.Sprintf("%s is already there", target)
			e.Hint = "Give --force to replace it, or -o <path> to write it somewhere else."
		}
		return 0, e
	}

	written, err := io.Copy(file, content)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil || written != byteSize {
		_ = os.Remove(target)
	}
	if err != nil {
		return written, &Error{Code: CodeNetwork, Message: fmt.Sprintf("writing %s: %v", target, err), Retryable: true}
	}
	return written, nil
}

// renderingOf finds the rendering that was asked for among the ones the file
// says it has. Nothing else is ever fetched: a name the file does not carry
// is refused here, where it can be said which ones it does carry.
func renderingOf(file api.File, name string) (api.FileVariant, bool) {
	for _, variant := range file.Variants {
		if variant.Name == name {
			return variant, true
		}
	}
	return api.FileVariant{}, false
}

func renderingNames(file api.File) string {
	var names []string
	for _, variant := range file.Variants {
		names = append(names, variant.Name)
	}
	if len(names) == 0 {
		return "no rendering at all"
	}
	return strings.Join(names, ", ")
}

func renderingBreadcrumbs(file api.File, fetched string) []Breadcrumb {
	var crumbs []Breadcrumb
	for _, variant := range file.Variants {
		if variant.Name == fetched {
			continue
		}
		crumbs = append(crumbs, Breadcrumb{
			Action:      "variant-" + variant.Name,
			Cmd:         fmt.Sprintf("minka uploads download %s --variant %s", file.ID, variant.Name),
			Description: fmt.Sprintf("Fetch the %s rendering instead (%s)", variant.Name, fileSize(variant.ByteSize)),
		})
	}
	return crumbs
}

// pathOrNothing names where the file was written, spelled out from the root
// so that a caller who ran the command elsewhere can still find it.
func pathOrNothing(target string) string {
	if absolute, err := filepath.Abs(target); err == nil {
		return absolute
	}
	return target
}

// isAPlainName says whether a name out of an answer can be the name of a file
// here. A name carrying a separator, or one that is a directory of its own,
// would write somewhere other than where the command was run.
func isAPlainName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

func (a *app) newUploadsCreateCmd() *cobra.Command {
	var name, contentType, projectID, conversationID string

	cmd := &cobra.Command{
		Use:   "create <file> (--project <projectId> | --conversation <conversationId>)",
		Short: "Upload a file into a project or a private conversation",
		Long: `Uploads a file and answers with attachmentSource, the element that hangs the
file in a text. Put it into the descriptionSource of a card or the bodySource
of a comment or a message, unchanged, and send that back as HTML:

  SOURCE=$(minka uploads create shot.png --project <projectId> --quiet | jq -r .attachmentSource)
  minka cards show <cardId> --quiet | jq -r .descriptionSource > text.html
  printf '%s' "$SOURCE" >> text.html
  minka cards edit <cardId> --description - --description-format html < text.html

A file belongs to the circle it was uploaded into: a file of a project hangs
in the texts of that project, a file of a conversation in the messages of that
conversation, and nowhere else. A foreign attachment is dropped while saving,
without an error, so read the text back after saving if you want to be sure.

A file is kept only once a text that carries it is saved. An upload no saved
text points to is removed after seven days.

The media type comes from the name of the file and, failing that, from its
first bytes. Give --content-type where that guess is wrong: the server reads
the bytes itself and refuses a file that contradicts what the type says.`,
		Example: `  minka uploads create screenshot.png --project 0199...
  minka uploads create report.csv --project 0199... --content-type text/csv
  minka uploads create shot.png --conversation 0199...`,
		Args: requireArgs("file"),
		RunE: func(cmd *cobra.Command, args []string) error {
			flags := cmd.Flags()
			if flags.Changed("project") == flags.Changed("conversation") {
				return usageErrorf(cmd, "give --project or --conversation, not both and not neither")
			}
			if flags.Changed("project") {
				if err := requireID(cmd, "--project", projectID); err != nil {
					return err
				}
			} else if err := requireID(cmd, "--conversation", conversationID); err != nil {
				return err
			}

			path := args[0]
			file, err := os.Open(path)
			if err != nil {
				return &Error{Code: CodeUsage, Message: fmt.Sprintf("cannot read %s: %v", path, err)}
			}
			defer file.Close()

			info, err := file.Stat()
			switch {
			case err != nil:
				return &Error{Code: CodeUsage, Message: fmt.Sprintf("cannot read %s: %v", path, err)}
			case info.IsDir():
				return &Error{Code: CodeUsage, Message: fmt.Sprintf("%s is a directory, not a file", path)}
			case info.Size() > maxUploadBytes:
				return &Error{
					Code:    CodeUsage,
					Message: fmt.Sprintf("%s is %s, and Minka takes at most %s", path, fileSize(info.Size()), maxUploadSize),
				}
			}

			filename := name
			if filename == "" {
				filename = filepath.Base(path)
			}
			mediaType := contentType
			if mediaType == "" {
				if mediaType, err = mediaTypeOf(filename, file); err != nil {
					return &Error{Code: CodeUsage, Message: fmt.Sprintf("cannot read %s: %v", path, err)}
				}
			}

			s, err := a.session()
			if err != nil {
				return err
			}
			upload := s.client.UploadToProject
			target := projectID
			if conversationID != "" {
				upload, target = s.client.UploadToConversation, conversationID
			}
			uploaded, err := upload(a.ctx, target, filename, mediaType, file, info.Size())
			if err != nil {
				e := s.fail(err)
				if e.Code == CodeNotFound {
					e.Hint = "There is no such project or conversation, or you are not in it."
				}
				switch e.APICode {
				case "contentTypeMismatch":
					e.Hint = "The bytes are not what their media type says. Give the right one with --content-type."
				case "notReadable":
					e.Hint = "The picture is broken or in a format Minka cannot read."
				case "tooLarge":
					e.Hint = "Minka takes at most " + maxUploadSize + " for one file."
				}
				return e
			}

			summary := fmt.Sprintf("Uploaded %s as %s, %s", uploaded.Value.Filename, uploaded.Value.Kind, fileSize(uploaded.Value.Size))
			return a.ok(success{
				data:        uploaded.Raw,
				summary:     summary,
				notices:     []string{"the file is kept only once a text that carries it is saved, and is removed after seven days if none does"},
				breadcrumbs: attachmentBreadcrumbs(conversationID != ""),
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n\n%s\n", summary, uploaded.Value.AppURL, uploaded.Value.AttachmentSource)
				},
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&projectID, "project", "", "Project the file goes into")
	flags.StringVar(&conversationID, "conversation", "", "Private conversation the file goes into")
	flags.StringVar(&name, "name", "", "Name the file gets in Minka, by default the name it has here")
	flags.StringVar(&contentType, "content-type", "", "Media type of the file, by default guessed from its name and its bytes")

	return cmd
}

// attachmentBreadcrumbs say where an uploaded file can hang, which for a
// conversation is nowhere but that conversation.
func attachmentBreadcrumbs(inAConversation bool) []Breadcrumb {
	if inAConversation {
		return []Breadcrumb{
			{Action: "attach-to-message", Cmd: "minka messages create <conversationId> - --format html", Description: "Say something that carries attachmentSource"},
		}
	}
	return []Breadcrumb{
		{Action: "attach-to-card", Cmd: "minka cards edit <cardId> --description - --description-format html", Description: "Send a description that carries attachmentSource"},
		{Action: "attach-to-comment", Cmd: "minka comments create <cardId> - --format html", Description: "Write a comment that carries attachmentSource"},
	}
}

// mediaTypeOf is what the file says it is: the type its name stands for, or
// the one its first bytes look like. Reading those bytes puts the file back
// where it was, so that it is sent whole.
func mediaTypeOf(filename string, file io.ReadSeeker) (string, error) {
	if byName := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); byName != "" {
		return byName, nil
	}

	head := make([]byte, 512)
	read, err := file.Read(head)
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	return http.DetectContentType(head[:read]), nil
}

// fileSize says how big a file is the way a person reads it.
func fileSize(bytes int64) string {
	const unit = 1000
	if bytes < unit {
		return fmt.Sprintf("%d bytes", bytes)
	}
	size, units := float64(bytes)/unit, []string{"kB", "MB", "GB"}
	for _, name := range units {
		if size < unit || name == units[len(units)-1] {
			return fmt.Sprintf("%.1f %s", size, name)
		}
		size /= unit
	}
	return fmt.Sprintf("%d bytes", bytes)
}
