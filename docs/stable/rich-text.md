# Rich text in both directions

Descriptions, comments and messages are rich text. The CLI accepts Markdown and
prints Markdown, so that a text it shows can be edited and sent back (C-008).

The working notes on the back of a card are the one text that is none of this.
Minka keeps them as they arrive and hands them back as they were kept, so the
CLI converts nothing in either direction and prints the body alone on standard
output — what it has to say about the notes goes the other way, so that what
was shown can be sent back byte for byte (C-042).

A write replaces them as a whole, so it says what it replaces: `cards notes
set` needs `--expect <updatedAt>` — the time that was read, or `none` for notes
nobody had written yet — or `--force`, and there is no default. The CLI does not
read the notes itself to fill `--expect` in, because a guard protects only what
the writer read before beginning to write (C-042).

## Markdown → HTML

`internal/markdown` converts with goldmark and writes only elements and
attribute values that the allowlist of the server keeps. The allowlist in the
Minka code decides, not this list:

- Headings of all six levels, `ol start`, link titles.
- Inline code becomes `code`, a block `pre > code` with `data-language` if the
  language matches the pattern of the server.
- Tables come out the way the editor writes them:
  `figure.lexxy-content__table-wrapper > table > tbody`, header cells
  `th.lexxy-content__table-cell--header`. Column alignment is lost.
- Images become links; an image inside a link becomes its label.
- Raw HTML in the Markdown becomes visible text.
- A link whose target is not http, https, mailto, tel or relative becomes
  plain text — the same rule as `RichText::isSafeUrl`, including the handling
  of a leading `\\` and `/\`.

The CLI reports each of these changes as a `notice`.

Markdown that contains text but nothing Minka keeps is refused instead of
silently emptying a description. Emptying is done with `--description ""`.

Comments and messages keep every line break, so that plain text arrives the
way it was typed.

## HTML → Markdown

`internal/richtext` converts back what `cards show --styled` and `messages
list --styled` show. Characters with a meaning are escaped, `<br>` becomes a
hard break, and the way back yields the same HTML.

`CarriesMarkup` decides whether a text carries markup, by the rule of the
server: markup is only a tag of a served element that is written out up to its
closing angle bracket. `a<i und b<s` stays plain text. Beside the elements
Markdown writes, the server keeps three that a text carries rather than
writes: `action-text-attachment`, `minka-embed` and `minka-ref`.

## References to another record

Minka turns `#MNK-1`, `/go/card/MNK-1` and an address of its own card, post,
chat, chat line (a chat's or a private conversation's address ending in
`#chat-message-…`), to-do, to-do list, document, upload or check-in question
into a `<minka-ref kind id>` when a text is saved, whichever way it comes
in (Minka's E-205). The Markdown conversion has nothing to do for it but leave
those names alone: `#MNK-1` stays text in every block, a bare address becomes
a link whose words are that address (what Minka looks for), trailing
punctuation stays outside, and code stays code. A link with words of its own
is kept as a link by Minka, not turned into a reference.

Every answer of the API adds `title`, `short-id` (only for a card whose board
shows short identifiers) and an absolute `href` for the reader; a target the
reader cannot open, one in the trash and one that does not exist keep only
`kind` and `id`, alike on purpose. Saving drops the additions, so a text sent
back as read is safe. `--json` carries the text exactly as served.

`internal/richtext` shows a reference so that the view stays the editable
text of C-008 ([C-091](../decisions.md#c-091)):

| Served | Shown |
|---|---|
| `href` that is a web address (every kind, with or without `short-id`) | `<href>` |
| `href` that cannot stand in angle brackets (defensive: Minka never serves one) | `[href](href)` |
| no `href` but `title` or `short-id` (defensive: Minka never serves one) | `#MNK-1 Title` or `Title` as text |
| only `kind` and `id` | Minka's own words per kind, see below; `(a reference you cannot open)` for a kind the CLI does not know |

The words for a target the reader cannot open are Minka's own, those of its
`reference.unavailable.*` translations, in parentheses:

| `kind` | Shown |
|---|---|
| `card` | `(a card you cannot open)` |
| `post` | `(a post you cannot open)` |
| `chat` | `(a chat you cannot open)` |
| `chatmessage` | `(a chat message you cannot open)` |
| `todo` | `(a to-do you cannot open)` |
| `todolist` | `(a to-do list you cannot open)` |
| `document` | `(a document you cannot open)` |
| `upload` | `(a file you cannot open)` |
| `question` | `(a question you cannot open)` |

Sent back, `<href>` becomes a link that shows its own address, which Minka
turns into the reference again. The title is left out, because a link with
words of its own stays a link; a card's short identifier reads in
`/go/card/MNK-1`, and `--json` carries title and short identifier for whoever
wants them. A bare `#MNK-1` is not shown either: a letter, a digit or a `-`
that touches it keeps Minka from reading it. An exclamation mark right before
any link is escaped, so the link does not turn into an image.

`richtext.Dropped` holds a new text against the old one for the guard below. A
visible reference counts as kept where the new text holds the element, its
`href` as a link showing its own address or as an address in the words
(one trailing slash and sentence punctuation aside), or `#` and its
`short-id` standing free and not glued to an address; code and the words of a link with words of its own do not
count. A hidden reference has nothing to be written by and counts as kept only
as the element.

## HTML that carries no markup is kept as characters, not refused

`RichText::fromUntrustedValue` asks that question **before** it cleans
anything, and a value that answers no is escaped, its line breaks turned into
`<br>`, and kept inside one `<div>`. The answer is the ordinary 200 or 201, so
Markdown sent with `--description-format html`, `--format html` or
`--body-format html` stands there afterwards as the characters somebody typed.

Nothing else reports that, so every one of those flags draws a `notice` naming
what becomes of the text ([C-068](../decisions.md#c-068)). It is not refused:
the server accepts it, and refusing what the server takes would be wrong about
it. The Markdown path cannot meet the case at all, because the conversion
happens here.

## Sources, files and mentions

Every text comes twice: as stored (`description`, `body`, `bodyHtml`) and as
source (`descriptionSource`, `bodySource`). In the source, files and mentions
are `action-text-attachment` elements with a freshly signed handle.

- What is shown is always the source: an image as a Markdown image, a file as
  a link, a mention as `@Name`. The plain text next to a message has neither.
- Markdown can hold neither a file nor a mention, nor `minka-embed` for an
  address the server drew as a card; a reference it holds only as the address
  above. A new text that would lose any of them is refused, in every command
  that replaces a text, `messages edit` included; `--drop-attachments` allows
  it explicitly. The refusal and the notice of a drop say whether attachments
  or mentions, references to other records or both were lost. `messages edit`
  finds the message by reading the transcript back from its end, since no
  request reads one message alone; where the chat or the message is not there
  Minka's answer comes, any other failure to read stops the edit. A text sent
  with `--format text` is held against the old one as the characters it is,
  unless it carries a tag of a served element (`CarriesMarkup`): Minka then
  takes it as HTML, and so does the guard.
  Sending the source back as HTML keeps them. Where Minka refuses the write
  whatever is sent — an archived project, a text of the archive, a comment of
  somebody else — the guard stands aside and Minka's refusal is the answer
  ([C-082](../decisions.md#c-082)). The text the command reads says whether
  it is archived; whether its project is, it does not, so the archived
  projects are read only where the guard is about to refuse. Where the
  readings disagree the guard stays. A text taken out of the archive between
  the read and the write is written without the guard.
- A handle is valid only within its circle — project or conversation. In a
  text elsewhere the server silently removes it on saving, which is why the
  help and the SKILL.md advise reading the text back afterwards.
