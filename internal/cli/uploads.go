package cli

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// maxUploadBytes is what Minka takes for one file. The server decides it and
// refuses a larger file with 413, but a file that is certainly too big is
// better refused before it travels.
const maxUploadBytes = 50 << 20

func (a *app) newUploadsCmd() *cobra.Command {
	return group("uploads", "Put files into a project or a conversation and hang them in texts", a.newUploadsCreateCmd())
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
					Message: fmt.Sprintf("%s is %s, and Minka takes at most %s", path, fileSize(info.Size()), fileSize(maxUploadBytes)),
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
					e.Hint = "Minka takes at most " + fileSize(maxUploadBytes) + " for one file."
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
