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
	return group("uploads", "Put files into a project and hang them in texts", a.newUploadsCreateCmd())
}

func (a *app) newUploadsCreateCmd() *cobra.Command {
	var name, contentType string

	cmd := &cobra.Command{
		Use:   "create <projectId> <file>",
		Short: "Upload a file into a project",
		Long: `Uploads a file into a project and answers with attachmentSource, the element
that hangs the file in a text. Put it into the descriptionSource of a card or
the bodySource of a comment or a message, unchanged, and send that back as
HTML:

  SOURCE=$(minka uploads create <projectId> shot.png --quiet | jq -r .attachmentSource)
  minka cards show <cardId> --quiet | jq -r .descriptionSource > text.html
  printf '%s' "$SOURCE" >> text.html
  minka cards edit <cardId> --description - --description-format html < text.html

The attachment belongs to the project it was uploaded into. In a text of
another project it is dropped on saving, without an error, so read the text
back after saving if you want to be sure.

A file is kept only once a text that carries it is saved. An upload no saved
text points to is removed after seven days.

The media type comes from the name of the file and, failing that, from its
first bytes. Give --content-type where that guess is wrong: the server reads
the bytes itself and refuses a file that contradicts what the type says.`,
		Example: `  minka uploads create 0199... screenshot.png
  minka uploads create 0199... report.csv --content-type text/csv`,
		Args: requireArgs("projectId", "file"),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[1]
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
			upload, err := s.client.UploadFile(a.ctx, args[0], filename, mediaType, file, info.Size())
			if err != nil {
				e := s.fail(err)
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

			summary := fmt.Sprintf("Uploaded %s as %s, %s", upload.Value.Filename, upload.Value.Kind, fileSize(upload.Value.Size))
			return a.ok(success{
				data:    upload.Raw,
				summary: summary,
				notices: []string{"the file is kept only once a text that carries it is saved, and is removed after seven days if none does"},
				breadcrumbs: []Breadcrumb{
					{Action: "attach-to-card", Cmd: "minka cards edit <cardId> --description - --description-format html", Description: "Send a description that carries attachmentSource"},
					{Action: "attach-to-comment", Cmd: "minka comments create <cardId> - --format html", Description: "Write a comment that carries attachmentSource"},
				},
				text: func(w io.Writer) {
					fmt.Fprintf(w, "%s.\n%s\n\n%s\n", summary, upload.Value.AppURL, upload.Value.AttachmentSource)
				},
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&name, "name", "", "Name the file gets in Minka, by default the name it has here")
	flags.StringVar(&contentType, "content-type", "", "Media type of the file, by default guessed from its name and its bytes")

	return cmd
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
