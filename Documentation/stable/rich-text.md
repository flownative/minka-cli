# Rich Text in beide Richtungen

Beschreibungen, Kommentare und Nachrichten sind Rich Text. Die CLI nimmt
Markdown an und gibt Markdown aus, damit ein Text, den sie zeigt, sich ändern
und zurückschicken lässt (C-008).

## Markdown → HTML

`internal/markdown` wandelt mit goldmark um und schreibt nur Elemente und
Attributwerte, die die Allowlist des Servers behält. Maßgeblich ist die
Allowlist im Minka-Code, nicht diese Aufzählung:

- Überschriften aller sechs Ebenen, `ol start`, Link-Titel.
- Inline-Code wird `code`, ein Block `pre > code` mit `data-language`, wenn die
  Sprache dem Muster des Servers entspricht.
- Tabellen entstehen so, wie der Editor sie schreibt:
  `figure.lexxy-content__table-wrapper > table > tbody`, Kopfzellen
  `th.lexxy-content__table-cell--header`. Die Spaltenausrichtung geht verloren.
- Bilder werden Links; ein Bild in einem Link wird sein Label.
- Rohes HTML im Markdown wird sichtbarer Text.
- Ein Link, dessen Ziel nicht http, https, mailto, tel oder relativ ist, wird
  Klartext — dieselbe Regel wie `RichText::isSafeUrl`, samt der Behandlung von
  `\\` und `/\` am Anfang.

Jede dieser Änderungen meldet die CLI als `notice`.

Markdown, das Text enthält, aber nichts, was Minka behält, wird abgelehnt,
statt eine Beschreibung stillschweigend zu leeren. Leeren geht mit
`--description ""`.

Kommentare und Nachrichten behalten jeden Zeilenumbruch, damit Klartext so
ankommt, wie er getippt wurde.

## HTML → Markdown

`internal/richtext` wandelt zurück, was `cards show --styled` und `messages
list --styled` zeigen. Zeichen mit Bedeutung werden escaped, `<br>` wird ein
harter Umbruch, und der Rückweg ergibt dasselbe HTML.

`CarriesMarkup` entscheidet, ob ein Text Markup trägt, nach der Regel des
Servers: Markup ist nur ein bis zur schließenden spitzen Klammer
ausgeschriebener Tag eines bedienten Elements. `a<i und b<s` bleibt Klartext.

## Quellen, Dateien und Erwähnungen

Jeder Text kommt zweimal: gespeichert (`description`, `body`, `bodyHtml`) und
als Quelle (`descriptionSource`, `bodySource`). In der Quelle stehen Dateien
und Erwähnungen als `action-text-attachment` mit frisch signiertem Handle.

- Gezeigt wird immer die Quelle: ein Bild als Markdown-Bild, eine Datei als
  Link, eine Erwähnung als `@Name`. Der Klartext neben einer Nachricht hat
  beides nicht.
- Markdown kann beides nicht halten. Ein neuer Text, der Anhänge oder
  Erwähnungen verlöre, wird abgelehnt; `--drop-attachments` erlaubt es
  ausdrücklich. Wer die Quelle als HTML zurückschickt, behält sie.
- Ein Handle gilt nur in seinem Kreis — Projekt oder Gespräch. In einem
  fremden Text entfernt der Server es beim Speichern stillschweigend, deshalb
  raten Hilfe und SKILL.md, den Text danach zurückzulesen.
