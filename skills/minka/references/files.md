# Putting a file into a text and fetching it back

## Commands

```bash
minka uploads create <file> (--project <projectId> | --conversation <conversationId>) [--name <name>] [--content-type <type>] --json
minka uploads download <blobId> [--variant original|display] [-o <path>|-o -] [--force] --json
```

## Uploads

- `uploads create <file> --project <projectId>` puts a file into a project and
  answers with `{id, filename, contentType, size, kind, appUrl,
  attachmentSource}`. `--conversation <conversationId>` puts it into a private
  conversation instead. Exactly one of the two is given.
  `kind` is `image` for a picture, which is shown where it stands, and `file`
  for everything else, which is handed over.
- `attachmentSource` is the whole point: the `<action-text-attachment>` element
  with a signature only the server can make. Put it into a `descriptionSource`
  or a `bodySource`, unchanged, and send that back as HTML:

```bash
SOURCE=$(minka uploads create shot.png --project <projectId> --quiet | jq -r .attachmentSource)
minka cards show <cardId> --quiet | jq -r .descriptionSource > text.html
printf '\n%s' "$SOURCE" >> text.html
minka cards edit <cardId> --description - --description-format html < text.html
```

- The attachment belongs to the circle it went into: a file of a project hangs
  in the texts of that project, a file of a conversation in the messages of
  that conversation. Anywhere else the server drops it while saving, without an
  error, so read the text back if it matters.
- A file is kept for good only once a text or an upload of a Docs & Files tool
  carries it. One that nothing carries is kept for **at least** seven days and
  taken away some time after that — a floor, not a deadline you can count on.
- The `id` of the answer is what `docs uploads create --blob <blobId>` takes
  to let the file stand on its own in a Docs & Files tool. A file goes into one
  place only: a text or an upload.
- The bytes of a picture are not the ones that went in: Minka turns a picture
  the right way up, strips it of everything but its colour profile — the place
  a camera wrote into it among the rest — and encodes it again, so `size`, and
  the `byteSize` of a download later on, say what Minka keeps and not what the
  file weighs here. What it shows is the same picture, though a JPEG is encoded
  afresh and is therefore not the same to the last pixel. A GIF, an SVG and
  everything that is no picture keep their bytes.
- The media type comes from the name of the file, and from its first bytes
  where the name says nothing. `--content-type` gives it instead. The server
  reads the bytes itself and keeps the type they show. It refuses a file only
  where the bytes and the type disagree **and one of the two is a picture it
  shows** (AVIF, GIF, JPEG, PNG, WebP): exit 9 with `meta.apiCode`
  `contentTypeMismatch`. An SVG counts as a document. Otherwise the type the
  bytes show is kept without a word, whatever was sent.
- At most 100 MiB. A larger file exits 1 before it travels, and a server that
  refuses one exits 9 with `tooLarge`.
- `--name` gives the file another name in Minka than it has here.

## Fetching a file back

- Every text says which files hang in it. `cards show` carries `attachments`
  for the description and one list per comment, `messages list` one per
  message: `{id, filename, contentType, byteSize, isImage, url}`, the file as
  Minka keeps it. A text without files carries an empty list. The board view
  of a card carries none; read the card itself.
- `uploads download <blobId>` fetches the bytes. `<blobId>` is the `id` of an
  attachment, so reading a card is enough to fetch its picture — nothing has
  to be taken out of the text.
- Without `-o` the file is written into the current directory under the name
  it has in Minka. A file of that name that is already there is not touched:
  exit 1, and `--force` replaces it. `-o <path>` names a target,
  `-o -` puts the bytes on standard output and the word about them on
  standard error; `--json` and `--quiet` are refused together with `-o -`,
  because standard output cannot carry both.
- `data` is `{file, variant, path, byteSize}`. `file` is the answer of the API
  about the file, `path` where the bytes were written, `byteSize` how many
  arrived.
- `--variant` asks for one rendering. `original` is the whole file and is
  always there. `display` is the smaller rendering a large picture got — only
  `variants` says whether a file has one, so never assume it. There is no
  command that asks for `variants` alone: they stand in `data.file.variants`
  of a download that worked, and a `--variant` the file has not got exits 1
  before any byte travels, with `hint` naming the ones it does have. Fetching
  the original and reading `variants` off it is the way to find out.
- A file of a project or a conversation you are not in exits 2, the same
  answer an identifier that names nothing gives: the two cannot be told apart.
- What arrives has to weigh what the file says it weighs. Anything else exits
  7 and nothing is kept.
