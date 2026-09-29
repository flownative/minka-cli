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
- Markdown can hold none of it: not a file, not a mention, and not the two
  further elements a text carries — `minka-embed` for an address the server
  drew as a card, `minka-ref` for a reference to another record. A new text
  that would lose any of them is refused; `--drop-attachments` allows it
  explicitly, and the refusal calls them all attachments, after the flag.
  Sending the source back as HTML keeps them. Where Minka refuses the write
  whatever is sent — an archived project, a text of the archive, a comment of
  somebody else — the guard stands aside and Minka's refusal is the answer
  ([C-082](../decisions.md#c-082)).
- A handle is valid only within its circle — project or conversation. In a
  text elsewhere the server silently removes it on saving, which is why the
  help and the SKILL.md advise reading the text back afterwards.
