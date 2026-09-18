# Status

Stand: 15.09.2026

## Wo wir stehen

**MVP fertig und reviewt.** Gegen den Fake getestet, im Smoke-Lauf gegen die
lokale Instanz geprüft, Befunde des Reviews eingearbeitet.

**Die CLI zieht API-Änderungen nach**, die die Minka-Koordination meldet oder
die beim Abgleich mit dem Code auffallen. Das Dogfooding in Minka wartet auf
eine interne Alpha.

API-Stand, gegen den gebaut ist: Minka `6694096` (API-1 bis API-5,
erweiterte Allowlist aus E-060, Zuständige an Karten, neue Karten oben in der
Spalte, Fälligkeitsdatum, Zuständige und Datum beim Anlegen, Schritte, erweiterte Allowlist aus `72d0639`, Kommentare als Rich Text, Chat, Abonnenten).
Koordination in Minka: `minka-92`.

Befehle:

```
minka auth login|status|logout
minka me
minka projects list
minka boards list <projectId> | show <boardId>
minka people list <projectId>
minka cards mine
minka cards restore <cardId>
minka trash list <projectId>
minka cards show|create|edit|move|assign|unassign|subscribe|unsubscribe|done|delete
minka steps list|create|complete|reopen|delete
minka comments create|edit|delete|react|unreact <cardId> ...
minka chats list <projectId> | mark-read <chatId>
minka conversations list | with <person> | people <gesprächsId>
minka messages list|create|edit|delete|react|unreact <chatId> ...
minka uploads create <datei> --project <id> | --conversation <id>
minka commands            Katalog aller Befehle mit Flags
minka version
```

## Entscheidungen

Auf Roberts Wunsch folgen die Konventionen **basecamp-cli**. Den Umfang
übernehmen wir nicht, die Prinzipien schon. Mit minka-8a abgestimmt.

- **Ausgabe:** Auf einem Terminal Text, sonst JSON. `--json` gibt eine Hülle
  `{ok, data, summary, notice, breadcrumbs}` aus, `--quiet` nur `data`,
  `--styled` erzwingt Text. `data` ist die API-Ressource unverändert, als
  rohes JSON durchgereicht. Warnungen stehen bei `--json` in `notice`, sonst
  auf stderr.
- **Fehler:** `{ok: false, error, code, retryable, hint, meta: {apiCode,
  httpStatus}}`. Den Minka-Fallcode liefert `meta.apiCode`. `retryable` ist
  nur bei lesenden Requests wahr. Ein fehlgeschlagener Schreibvorgang kann
  schon angekommen sein, der Hint rät dann zum Nachlesen.
- **Exit-Codes wie basecamp-cli:**

  | Exit | Bedeutung |
  |------|-----------|
  | 0 | ok |
  | 1 | Aufruf oder Config-Datei |
  | 2 | nicht gefunden |
  | 3 | nicht angemeldet |
  | 4 | verboten (Minka liefert nie 403) |
  | 6 | Netzwerk |
  | 7 | API/Server |
  | 8 | mehrdeutig |
  | 9 | Validierung |

- **Befehlsnamen** als Plural-Gruppe plus Aktion (`cards move`).
- **Anmeldung:**
  - Das Token kommt nie über ein Flag, sondern verdeckt vom Terminal oder mit
    `--with-token` von stdin. Stdin darf dafür kein Terminal sein, das Token
    muss einzeilig sein und ist auf 4096 Bytes begrenzt.
  - Vor dem Speichern wird es gegen `/me` geprüft.
  - Die Config-Datei wird mit 0600 im Verzeichnis 0700 gespeichert, atomar
    mit fsync. Ein Symlink bleibt erhalten.
  - Eine Config, die andere lesen können, wird abgelehnt.
- **Ein Token geht nur an seinen Host.** `MINKA_TOKEN` gilt nur zusammen mit
  `MINKA_HOST` und nur für diesen Host. Bei `--host` mit einem anderen Host
  kommt das Token aus der Config.
- **Host:**
  - `https://` wird ergänzt, `/api/v1` und Standardports entfernt.
  - `http://` gilt nur für Loopback.
  - Andere Pfade werden abgelehnt.
- **HTTP:**
  - Redirects werden nie verfolgt.
  - IDs werden lokal als UUID geprüft (sonst Exit 1 ohne Request) und
    zusätzlich im Pfad escaped.
  - Timeout 30 s, Antworten höchstens 16 MB.
- **Markdown → HTML** (goldmark), nur Elemente und Attributwerte der
  Server-Allowlist, Stand Minka `72d0639`:
  - Überschriften aller sechs Ebenen, `ol start`, Link-Titel bis 200 Zeichen.
  - Inline-Code wird `code`. Code-Blöcke werden `pre > code`, mit
    `data-language`, wenn die Sprache dem Muster des Servers entspricht.
  - `---` wird `hr`.
  - Tabellen werden so geschrieben, wie der Editor sie schreibt:
    `figure.lexxy-content__table-wrapper > table > tbody`, Kopfzellen
    `th.lexxy-content__table-cell--header`. Die Spaltenausrichtung geht
    verloren.
  - Bilder werden Links, ein Bild in einem Link wird nur sein Label.
  - Rohes HTML im Markdown wird sichtbarer Text.
  - Links mit anderen Zielen als http, https, mailto, tel und relativen URLs
    werden Klartext. Das ist dieselbe Regel wie `RichText::isSafeUrl` auf dem
    Server.
  - Markdown, das Text enthält, aber nichts, was Minka behält, wird
    abgelehnt, statt die Beschreibung zu löschen. Löschen geht mit
    `--description ""`.
- **Klartext oder Markup:** `richtext.CarriesMarkup` folgt der Regel des
  Servers aus Minka `4f5629f`. Markup ist nur ein Tag eines bedienten
  Elements, der bis zur schließenden spitzen Klammer ausgeschrieben ist.
  `"a<i und b<s"` bleibt Klartext. Links, die mit `\\` oder `/\` beginnen,
  gelten wie `//host` als unsicher.
- **HTML → Markdown** für `cards show` im Text: Text wird escaped, `<br>`
  wird ein harter Umbruch. Zurückgeschickt entsteht dasselbe HTML.
- **Kommentare** sind seit Minka `c7c4d1b` Rich Text wie Beschreibungen.
  `comments create` wandelt Markdown um und behält dabei jeden Zeilenumbruch
  (`markdown.KeepLineBreaks`), damit Klartext so ankommt, wie er getippt
  wurde. `--format html` schickt HTML unverändert. `cards show` zeigt
  HTML-Kommentare als Markdown; ältere Kommentare ohne Markup liefert der
  Server als Klartext, sie werden unverändert gezeigt.
- **Eingabe über stdin:** `-` liest von stdin, höchstens 1 MB. Ein Terminal
  als stdin wird abgelehnt.
- **`--column`** nimmt eine ID, einen Typ (`triage`, `not_now`, `done`) oder
  einen Titel an. Passt der Wert auf mehrere Spalten, gibt es Exit 8 mit den
  Kandidaten, auch wenn ein Typname zugleich Titel einer anderen Spalte ist.
- **`cards create`** in Not now oder Done wird in der CLI abgelehnt, auch per
  ID.
- **`cards move`:**
  - `--after X` wird zu `previousCardId`, `--before Y` zu `nextCardId`.
  - Ohne Platzierung geht die Karte nach oben.
  - Ein Nachbar, der nicht in der Zielspalte liegt, wird abgelehnt. Der Server
    würde die Karte sonst still nach oben legen.
  - Done nimmt keine Platzierung an, denn es ist nach Abschluss sortiert.
- **`cards done`** ist idempotent.
- **`cards delete`** (Alias `trash`) legt die Karte in den Papierkorb. Wie
  jedes DELETE ist das idempotent, die Wirkung auf dem Server bleibt gleich.
  Nur die Antwort unterscheidet sich: Ein zweites Löschen gibt 404 und damit
  Exit 2 statt Exit 0, denn eine Karte im Papierkorb sieht aus wie eine, die
  es nie gab. Eine Rückfrage gibt es nicht, weil Agenten nicht interaktiv
  arbeiten.
- **Zuständige:**
  - `cards assign` und `cards unassign` nehmen je eine Person: ID, `me`,
    E-Mail-Adresse oder Namen. Ein Name muss genau eine Person treffen, ohne
    Rücksicht auf Groß- und Kleinschreibung; zwei Treffer ergeben Exit 8.
  - Aufgelöst wird gegen die Mitglieder des Projekts, beim Abnehmen zuerst
    gegen die Zuständigen der Karte, damit auch Ausgeschiedene gehen.
  - Eine UUID, die niemand in der Liste trägt, geht unverändert an den
    Server, der mit `personNotAProjectMember` begründet.
  - Doppeltes Zuweisen und Abnehmen ist kein Fehler. Die Zusammenfassung sagt,
    ob sich etwas geändert hat (201 oder 200 beim Zuweisen, `wasAssigned`
    beim Abnehmen).
  - `cards create --assignee` (wiederholbar) löst alle Personen vor dem
    Anlegen auf und schickt sie als `assigneeIds` im selben Request. Karte
    und Zuständige entstehen in einer Transaktion.
- **Abonnenten:**
  - `cards subscribe` und `cards unsubscribe` lösen die Person auf wie
    `cards assign`. Beim Abmelden kommen die Abonnenten der Karte zuerst in
    die Kandidaten; der Server verlangt aber auch dafür ein Mitglied des
    Projekts.
  - Ob sich etwas geändert hat, sagt die Zusammenfassung, verglichen mit der
    Karte vor dem Aufruf (`wasSubscribed` beim Abmelden).
  - Die Kartenansicht zeigt „Subscribers:“. Das Board liefert keine
    Abonnenten.
- **Schritte:**
  - Die Gruppe `steps` hat `list`, `create`, `complete`, `reopen` und
    `delete`.
  - Ein Schritt wird über ID, Position ab 1 oder Text angegeben. Eine Zahl
    ist immer eine Position. Ein Text auf zwei Schritten ergibt Exit 8 mit den
    Positionen.
  - Die API liefert die Schritte nur an der einzelnen Karte, das Board nur
    `stepCount` und `completedStepCount`. `boards show` zeigt daraus
    „steps 1/3", und jeder Befehl mit Schritt-Angabe liest zuerst die Karte.
  - Wiederholtes Abhaken, Öffnen und Löschen ist kein Fehler; die
    Zusammenfassung sagt, ob sich etwas geändert hat.
  - `steps list` gibt als `data` die Liste `steps` aus der Karte unverändert
    aus.
- **Chat:**
  - `chats list` und `chats mark-read`, dazu `messages list`, `create`, `edit`
    und `delete`.
  - `messages list` blättert mit `--before <seq>` zurück und holt mit
    `--after <changeSeq>` alles Geänderte nach. `--before` und `--after`
    zusammen sind ein Aufruffehler.
  - Nachrichten sind seit Minka `a8eeb7a` Rich Text. `messages create` und
    `messages edit` wandeln Markdown um wie `comments create` und behalten
    jeden Zeilenumbruch. `--format text` schickt den Text unverändert, der
    Server macht daraus Absätze, Umbrüche und Links. `--format html` schickt
    HTML.
  - `--client-message-id` macht erneutes Senden sicher. Mit ihr gilt ein
    fehlgeschlagenes Senden (Netzwerk oder 5xx) als `retryable`, ohne sie
    nicht.
  - `chats mark-read` ohne `--seq` liest die letzte Nachricht und markiert
    bis dahin.
  - Eine fremde Nachricht zu ändern oder zu entfernen ergibt 404; der Hint
    sagt, dass nur die Autorin darf.
- **Fälligkeitsdatum:**
  - `--due YYYY-MM-DD` gibt es bei `cards create` und `cards edit`,
    `cards edit --due ""` entfernt es.
  - Die CLI prüft das Datum nicht selbst. Der Server antwortet mit
    `cardDueDateNotADate`, das ergibt Exit 9.
  - Relative Angaben wie `tomorrow` gibt es bewusst nicht. „Heute" hängt von
    der Zeitzone ab, und die Installation legt den Tag in ihrer eigenen aus.

## Tests

`make check` (gofmt, go vet, `go test ./...`) ist grün. Getestet wird gegen
`internal/fakeapi`, einen In-Memory-Nachbau des API-Vertrags: dieselben
Codes, dieselbe Prüfreihenfolge und dasselbe `trim` wie PHP, Done nach
Abschluss sortiert.

### Sabotagebeweise

Schutz in einer Kopie des Codes herausgenommen, der Code kompiliert, der
genannte Test wird rot. Zuletzt am 13.09.2026 nach dem Review ausgeführt:

| Schutz | Test |
|---|---|
| Config-Datei 0600 | `TestSaveWritesAFileOnlyItsOwnerCanRead` |
| Für andere lesbare Config abgelehnt | `TestAConfigFileOthersCanReadIsNotUsed` |
| Symlink der Config bleibt | `TestSaveKeepsASymlinkedConfigFileALink` |
| `http://` nur für Loopback | `TestNormalizeHost` |
| `MINKA_TOKEN` nur an `MINKA_HOST` | `TestTheEnvironmentTokenIsOnlySentToItsHost` |
| Token einzeilig | `TestLoginRefusesATokenThatIsNotOneLine` |
| Abgelehntes Token nicht gespeichert | `TestLoginWithARefusedTokenSavesNothing` |
| ID im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Nur UUIDs als ID | `TestValuesThatAreNoIDsAreRefusedBeforeSending` |
| Redirects nicht verfolgt | `TestARedirectIsNotFollowed` |
| Schreibfehler nicht retryable (HTTP und Netzwerk) | `TestAFailedWriteTellsToReadBackBeforeRetrying`, `TestFailuresHaveACodeAndAnExitCodePerClass` |
| Allowlist (Überschrift ab Ebene 4, rohes Inline-HTML, Code-Block unescaped) | `TestOnlyElementsTheServerKeepsAreWritten` |
| Inline-Code escaped | `TestConversions` |
| Backticks im Inline-Code (HTML → Markdown) | `TestMarkdownOfADescriptionConvertsBackToTheSameHTML` |
| Link-Schemata wie der Server, kein `//` | `TestConversions` |
| Kein Bild-Link im Link | `TestConversions` |
| Leeres Markdown löscht keine Beschreibung | `TestMarkdownWithNothingMinkaKeepsDoesNotRemoveTheDescription` |
| `--description-format` immer geprüft | `TestTheDescriptionFormatIsCheckedWithoutADescription` |
| Warnungen bei `--quiet` auf stderr | `TestQuietPrintsWarningsToStderr` |
| stdin nicht vom Terminal | `TestTextIsNotReadFromATerminal` |
| `cards done` idempotent | `TestDoneCompletesACardOnlyOnce` |
| Name ohne Treffer geht nicht an den Server | `TestSomebodyOutsideTheProjectIsNotAssigned`, `TestCreateAssignsThePeopleItFoundBeforeCreating` |
| Zwei Personen mit gleichem Namen mehrdeutig | `TestANameOfTwoPeopleIsAmbiguous` |
| Abnehmen findet Ausgeschiedene über die Zuständigen | `TestUnassignFindsSomebodyWhoLeftTheProject` |
| Personen-ID im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Schon zugewiesen wird gemeldet (200 statt 201) | `TestAssigningTwiceSucceedsAndSaysSo` |
| E-Mail-Adresse als Person | `TestAssignFindsThePersonByIDMeEmailOrName` |
| Personen vor dem Anlegen aufgelöst | `TestCreateAssignsThePeopleItFoundBeforeCreating` |
| Zuständige im selben Create-Request | `TestCreateAssignsThePeopleItFoundBeforeCreating` |
| `--due` beim Anlegen gesendet | `TestCreateWithADueDate` |
| `--due ""` entfernt das Datum, ohne andere Felder | `TestEditSetsAndRemovesTheDueDate` |
| Schritt über Position, Text oder ID | `TestCompleteAndReopenFindTheStepByPositionTextOrID` |
| Text auf zwei Schritten mehrdeutig | `TestAStepTextOfTwoStepsIsAmbiguous` |
| Position außerhalb der Liste abgelehnt | `TestAPositionOutsideTheListIsRefused` |
| Schritt-ID im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Code-Sprache, `th`-Klasse, kein `thead` (Allowlist) | `TestOnlyElementsTheServerKeepsAreWritten` |
| `ol start` und Link-Titel escaped | `TestConversions` |
| `|` in Tabellenzellen escaped (HTML → Markdown) | `TestToMarkdown` |
| Listenstart und tiefe Überschriften im Rückweg | `TestMarkdownOfADescriptionConvertsBackToTheSameHTML` |
| Kommentare behalten Zeilenumbrüche | `TestACommentIsSentAsHTMLWithItsLineBreaks`, `TestKeepLineBreaksKeepsEveryLine` |
| Alte Klartext-Kommentare unverändert, HTML als Markdown | `TestCommentsShowAsMarkdownAndOldPlainOnesAsTheyAre`, `TestCarriesMarkup` |
| `--format` geprüft, leeres Ergebnis abgelehnt | `TestACommentInHTMLIsSentAsItIs`, `TestACommentWithNothingMinkaKeepsIsRefused` |
| `--before` und `--after` nicht zusammen | `TestMessagesListRefusesWhatIsNoPage` |
| `--before` als `before` gesendet | `TestMessagesListReadsBackAndCatchesUp` |
| Senden mit `--client-message-id` retryable, ID geprüft | `TestAFailedPostCanBeSentAgainWithItsClientMessageID`, `TestMessagesCreateRefusesWhatIsNoMessage` |
| Schon geschrieben wird gemeldet (200) | `TestMessagesCreateWritesOncePerClientMessageID` |
| `mark-read` ohne `--seq` nimmt die letzte Nachricht | `TestMarkReadUsesTheLastMessageUnlessToldOtherwise` |
| Hint „nur die Autorin“ bei 404 | `TestOnlyTheAuthorChangesOrRemovesAMessage` |
| Nachrichten-ID im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Link mit `\\host` wie `//host` abgelehnt | `TestConversions` |
| Klartext-Erkennung nach der Regel des Servers | `TestCarriesMarkup` |
| Chat-Nachrichten: Markdown mit Zeilenumbrüchen, `text` und `html` unverändert, auch beim Ändern | `TestAMessageIsSentAsMarkdownTextOrHTML` |
| Chat-Format geprüft, leeres Ergebnis abgelehnt | `TestAMessageInAnUnknownFormatOrWithNothingMinkaKeepsIsRefused` |
| Person auf der Karte behält ihre E-Mail-Adresse (auch bei `unassign`) | `TestSomebodyOnTheCardIsFoundByEmailAddress`, `TestSubscribeAndUnsubscribeAPerson` |
| Schon abonniert und `wasSubscribed` aus dem Stand vor dem Aufruf | `TestSubscribeAndUnsubscribeAPerson` |
| Beschreibung mit Anhängen oder Erwähnungen: Markdown nur mit `--drop-attachments`, Quelle als HTML erlaubt | `TestADescriptionWithAttachmentsIsNotReplacedUnlessAsked`, `TestHasAttachments` |
| Kartenansicht liest die Quelle, nicht die gespeicherte Fassung | `TestADescriptionWithAttachmentsIsNotReplacedUnlessAsked` |
| Erwähnung als `@Name`, Bild als Markdown-Bild | `TestToMarkdown` |
| `action-text-attachment` gilt als Markup | `TestCarriesMarkup` |
| Abonnenten in der Kartenansicht | `TestSubscribersAppearOnTheCard` |
| Personen-ID bei Abonnenten im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Angehängtes Bild als Markdown-Bild | `TestToMarkdown` |
| 204 ohne Rumpf ist Erfolg | `TestAnEmptyAnswerIsASuccess` (api), `TestDeleteMovesACardToTheTrash` |
| Nachbar in Zielspalte geprüft | `TestMoveNextToACardOutsideTheTargetColumnIsRefused` |
| Done ohne Platzierung | `TestDoneTakesNoPlacement` |
| Anlegen in Done/Not now auch per ID abgelehnt | `TestCreateInNotNowOrDoneIsRefusedHoweverTheColumnIsGiven` |
| Typname und gleicher Titel mehrdeutig | `TestATypeThatIsAlsoTheTitleOfAnotherColumnIsAmbiguous` |
| HTML → Markdown: Zeilenanfänge escaped, `<br>` hart | `TestTextWithoutMarkupStaysText` |
| Entfernte Chat-Nachricht nicht im Verlauf | `TestARemovedMessageIsGoneFromTheTranscript` |
| `cards mine` liest /me/cards ohne Query | `TestMyCardsComeFromEveryProjectInTheOrderOfThePage` |
| Überfällige Karte als „overdue“ gezeigt | `TestMyCardsComeFromEveryProjectInTheOrderOfThePage` |
| Karten in Done zählen nicht zu den eigenen | `TestMyCardsComeFromEveryProjectInTheOrderOfThePage` |
| Karte ohne Fälligkeit steht hinten | `TestMyCardsComeFromEveryProjectInTheOrderOfThePage` |
| Projekt- und Board-Name an der Karte | `TestMyCardsComeFromEveryProjectInTheOrderOfThePage` |
| Karten-ID bei `restore` im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Zurückgeholte Karte steht oben in ihrer Spalte | `TestACardComesBackFromTheTrashOnce` |
| Zweites `restore` endet mit Exit 2 samt Hinweis | `TestACardComesBackFromTheTrashOnce` |
| Fremde Karte bleibt im Papierkorb | `TestACardComesBackFromTheTrashOnce` |
| Personen-ID beim Gespräch im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Name statt ID braucht `--project` | `TestAConversationWithSomebodyIsBegunOnlyOnce` |
| Ein Gespräch je Paar, zweites Mal dasselbe | `TestAConversationWithSomebodyIsBegunOnlyOnce` |
| Nur die beiden lesen ein Gespräch | `TestAConversationIsAChatWithoutAProject` |
| Reaktionen mit allen Namen, gleiche Inhalte zusammen | `TestReactionsStandUnderACommentAndAMessage` |
| Keine Zeile, wo niemand reagiert hat | `TestReactionsStandUnderACommentAndAMessage` |
| Reaktionen auch an Chat-Nachrichten | `TestReactionsStandUnderACommentAndAMessage` |
| Datei geht an die Adresse des genannten Kreises | `TestAFileGoesIntoTheCircleTheFlagNames` |
| Genau ein Kreis, sonst Exit 1 | `TestAnUploadNamesOneCircleToGoInto` |
| Kreis-ID muss eine UUID sein | `TestAnUploadNamesOneCircleToGoInto` |
| Außenstehende laden nichts in ein Gespräch | `TestAFileGoesIntoTheCircleTheFlagNames` |
| Dateiname und Reaktion in der Query escaped | `TestWhatRidesInTheQueryIsEscaped` (api) |
| Projekt-ID eines Uploads im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Medientyp aus dem Dateinamen, nicht nur aus den Bytes | `TestAFileIsUploadedAsItIsAndComesBackAsAnAttachment` |
| Länge des Uploads angekündigt (`Content-Length`) | `TestAFileIsUploadedAsItIsAndComesBackAsAnAttachment` |
| Die Bytes gehen unverändert zum Server | `TestAFileIsUploadedAsItIsAndComesBackAsAnAttachment` |
| Verzeichnis wird nicht hochgeladen | `TestAnUploadIsRefusedWhenTheFileOrItsTypeIsWrong` |
| 413 gilt als Validierungsfehler (Exit 9) | `TestAnUploadIsRefusedWhenTheFileOrItsTypeIsWrong` |
| Hinweis auf `--content-type` bei falschem Typ | `TestAnUploadIsRefusedWhenTheFileOrItsTypeIsWrong` |
| Kommentar mit Anhängen: neuer Text nur mit `--drop-attachments`, Quelle als HTML erlaubt | `TestACommentWithAttachmentsIsNotReplacedUnlessAsked` |
| `comments edit` sendet HTML, nicht das Markdown | `TestOnlyTheAuthorChangesOrRemovesAComment` |
| Hinweis „nur der Autor“ bei 404 | `TestOnlyTheAuthorChangesOrRemovesAComment` |
| Kommentar-ID im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Erwähnungs-Markup geht unverändert zum Server | `TestThePeopleOfACircleCarryTheirMention` |
| Erwähnung eines Gesprächs trägt dessen Handle | `TestThePeopleOfACircleCarryTheirMention` |
| Außenstehende sehen die Teilnehmenden nicht | `TestThePeopleOfACircleCarryTheirMention` |
| Gesprächs-ID bei den Personen im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Projekt-ID beim Papierkorb im Pfad escaped | `TestIdentifiersAreEscapedInThePath` (api) |
| Gekürzter Papierkorb sagt es als Notiz | `TestTheTrashListsWhatWasTakenOutOfAProject` |
| Papierkorb nur für Mitglieder | `TestTheTrashListsWhatWasTakenOutOfAProject` |
| Wer etwas weggeworfen hat, steht dabei | `TestTheTrashListsWhatWasTakenOutOfAProject` |
| Zweimal dieselbe Reaktion zählt einmal | `TestAReactionIsLeftOnceAndComesOffAgain` |
| `unreact` nimmt die eigene, nicht die letzte | `TestAReactionIsLeftOnceAndComesOffAgain` |
| Nachricht muss zum Chat der Adresse gehören | `TestAReactionIsLeftOnceAndComesOffAgain` |
| Reaktion an einer Gesprächsnachricht, nur für die beiden | `TestAReactionIsLeftOnceAndComesOffAgain` |
| Hinweis bei zu langer Reaktion | `TestAReactionIsShortAndSays_Something` |
| Art der Person (`kind`) und für wen ein Agent handelt | `TestAnAgentSaysWhomItActsFor` |
| `principal` ist nur noch ID und Name | `TestAnAgentSaysWhomItActsFor` |
| Ausgeschiedener Agent steht in keiner Personenliste | `TestAnAgentSaysWhomItActsFor` |
| Nachricht zeigt Anhang und Erwähnung im Text | `TestMessagesAsText` |

### Smoke-Lauf gegen die lokale Instanz

Am 13.09.2026, vor dem Review, mit dem Token `minka-cli-smoke` (Bruno), Board
„API-3 Abnahme" in „Test-Projekt". Angelegte Karte:
`dca9e204-b744-4cd9-abcc-64e338822df1`, liegt in Done und bleibt dort (das
Test-Board ist dafür da).

Grün:

- `auth login --with-token`, `auth status`, `me`, `projects list`,
  `boards list`, `boards show`.
- `cards create` mit Markdown von stdin, Umlauten und Anführungszeichen.
- `cards show`, `cards edit`, `cards move --column triage --top`,
  `comments create -` mit Umbruch und Tab.
- `cards done` zweimal, beim zweiten Mal ohne Move.

Fehlerfälle live:

| Fall | Exit | Code |
|---|---|---|
| unbekannte Karte | 2 | |
| Tab im Titel | 9 | `cardTitleHasControlCharacters` |
| Steuerzeichen im Kommentar | 9 | `commentHasControlCharacters` |
| falsches Token | 3 | |
| unbekannte Spalte | 1 | |
| Anlegen in Done | 1 | |

Live nicht geprüft, weil das Board keine Arbeitsspalte hat:
`--column <Titel>`, `--after`, `--before`, `--bottom`. Gegen den Fake sind
sie getestet.

## Review

Ein Reviewer-Agent, der nichts ändert, hat am 13.09.2026 29 Befunde gemeldet,
keinen Blocker.

Eingearbeitet:

- Token-Bindung an den Host
- Retry-Semantik
- Warnungen bei `--quiet`
- Link-Regel des Servers
- verschachtelte Links
- leeres Markdown
- Typ- und Titel-Mehrdeutigkeit
- HTML → Markdown (Escaping, harte Umbrüche, Leerzeichen an Inline-Grenzen,
  Code-Fences)
- CLAUDE.md
- Nachbar-Test
- Fake näher an PHP (`\r`, `trim`, Prüfreihenfolge, Meldungen, Done-Sortierung,
  angehängter Müll im JSON, Flow-Formularargumente)
- UUID-Prüfung
- Anlegen per Spalten-ID
- Done ohne Platzierung
- toter Code
- Logout-Meldung
- `--description-format`
- Host-Helfer
- 401-Hint bei `MINKA_TOKEN`
- stdin vom Terminal
- Symlink, fsync und Standardport in der Config
- fehlende Tests

Bewusst nicht gemacht:

- Die Beschreibungslänge prüft der Fake am gesendeten Text, der Server am
  bereinigten HTML. Das spielt nur bei Markup eine Rolle, das der Server
  entfernt.
- Keine Warnung für die Sprache eines Code-Blocks, die wegfällt.
- `tokenSource` bleibt ein String.

An minka-8a gemeldet: Die Fehlertabelle in `authorization.md` nannte 404 für
einen unbekannten Nachbarn. In Minka `60c7257` ist die Doku korrigiert: Fehlt
der Vorgänger, entscheidet der Nachfolger, fehlen beide, geht die Karte nach
oben; 404 nur bei einer ungültigen ID. Der Fake folgte dem schon.

## Offene Punkte

- Seit Minka `268065d` kommt jeder Text zweimal: gespeichert und als Quelle
  (`descriptionSource`, `bodySource`). Dateien und Erwähnungen stehen darin
  als `action-text-attachment` mit frischer Signatur, `url`, `filename` und
  `content-type`. Die frühere Auskunft, die API liefere `figure`/`img`, hat
  minka-63 korrigiert.
  - `cards show` zeigt Beschreibung und Kommentare aus der Quelle: ein Bild
    als Markdown-Bild, eine Datei als Link, eine Erwähnung als `@Name`.
  - Markdown kann Dateien und Erwähnungen nicht halten. `cards edit
    --description` lehnt deshalb eine neue Beschreibung ab, die sie verlieren
    würde, außer mit `--drop-attachments`. Wer die Quelle als HTML
    zurückschickt (`--description-format html`), behält sie.
  - `comments edit` hat dieselbe Sperre wie `cards edit`: Ein neuer Text, der
    Anhänge oder Erwähnungen des Kommentars verlieren würde, braucht
    `--drop-attachments`; die `bodySource` als HTML zurückgeschickt kommt
    durch. Chat-Nachrichten dagegen: `messages edit` hat keine Sperre, weil der Chat keine einzelne
    Nachricht liefert. Wer eine Nachricht mit Erwähnungen ändert, schickt
    `bodySource` mit `--format html`.
  - Erwähnungen schreiben geht seit Minka `800d27e` (E-115): `people list` und
    `conversations people` liefern je Person `mention: {handle, markup}`. Das
    Markup geht unverändert in einen Text desselben Kreises, als HTML
    gesendet.

- Das Review der Schritte in Minka ist mit `51b44e6` abgeschlossen. Das Board
  liefert je Karte `stepCount` und `completedStepCount`. `DELETE` auf einen
  Schritt einer anderen Karte oder auf eine ungültige Schritt-ID gibt bewusst
  204 (E-067), 404 nur für eine fremde Karte.
- Umbenennen und Umsortieren von Schritten gibt es in der API noch nicht.
- Agenten und Tokens verwaltet Minka über die Oberfläche unter `/account` und
  per Flow-CLI (`account:createagent`, `token:create`); an der API gibt es
  dafür keine Wege, und die CLI braucht deshalb keine.
- Reaktionen (Minka `f006bd4`, E-106) sind gelesen umgesetzt: `reactions` an
  Kommentaren und Nachrichten, in der Textansicht als eine Zeile darunter.
  Schreiben gibt es seit E-114 auch: `comments react|unreact` und `messages
  react|unreact`, seit E-118 auch an Nachrichten privater Gespräche unter
  deren ID. Die Fake-API normalisiert den Text einer Reaktion nicht nach
  Unicode NFC, der Server tut es.
- Die Fake-API kennt keine Organisationen. Dass ein Gespräch mit jemandem aus
  einer fremden Organisation mit 404 endet, ist deshalb nur gegen den Code
  gelesen, nicht nachgebildet. Laut minka-c2 bleibt es ein nacktes 404 ohne
  eigenen Fehlercode (E-032), da ist also nichts nachzuziehen.
- `lastMessageText` ist `body_text` mit zusammengefassten Zeilenumbrüchen, auf
  200 Zeichen gekürzt mit angehängtem „…“ (`previewOf`, im Code nachgesehen).
  Die CLI zeigt es unverändert und kürzt nicht noch einmal.
- Uploads: seit Minka `30a19d2` in der API und seit dem 16.09.2026 in der CLI
  als `uploads create` (Shaun hat entschieden, dass sie dazugehören):
  `POST /api/v1/projects/{projectId}/uploads?filename=…`, Rumpf sind die rohen
  Bytes, `Content-Type` ist Pflicht, höchstens 50 MiB. Die Antwort trägt
  `attachmentSource`, das fertige `action-text-attachment` mit frischer
  Signatur, das unverändert in `descriptionSource` oder `bodySource` gehört.
  Damit hängt die CLI Dateien an Texte an, obwohl das Briefing Uploads
  ausschloss. Was dabei gilt (von minka-c2, im Code nachgesehen):
  - Die Signatur in `attachmentSource` verfällt nie, sie steht ja dauerhaft im
    gespeicherten Text. Ein Handle aus einem fremden Projekt wird beim
    Speichern aber stillschweigend aus dem Text genommen, ohne Fehler. Die CLI
    setzt `attachmentSource` also nur in Texte desselben Projekts; die Hilfe
    und die SKILL.md sagen, dass man den Text nach dem Speichern zurücklesen
    sollte. Die Fake-API bildet dieses stille Entfernen nicht nach.
  - Ein Upload, auf den nach sieben Tagen kein gespeicherter Text zeigt, wird
    weggeräumt (`uploads:collectGarbage`, Vorgaben 168 und 24 Stunden). Erst
    das Speichern des Textes macht ihn dauerhaft; das gehört in die Hilfe.
  - Uploads hängen am Projekt, nicht am Board oder Chat. Zum Testen reicht das
    Test-Projekt.
  - Der Server liest den Medientyp aus den Bytes und widerspricht dem Header
    nur, wenn eine der beiden Seiten ein Bild ist (`UploadedBytes`). Die
    Fake-API hält sich an dieselbe Regel.
- Projekte haben seit Minka `ccd0439` Chat-Werkzeuge (`type=chat`).
  `boards list` zeigt nur Boards.
- Die erweiterte Allowlist (`72d0639`) und Kommentare als Rich Text
  (`c7c4d1b`) kamen ohne Nachricht der Minka-Koordination; beides fiel beim
  Abgleich mit dem Code auf. Ist an minka-63 gemeldet.

- Projekt archivieren und in den Papierkorb legen (E-129) ist in Arbeit: Ein
  archiviertes Projekt nimmt keine Schreibvorgänge mehr an, eines im
  Papierkorb antwortet überall 404. Neue API-Endpunkte gibt es dafür nicht;
  minka-92 meldet die Einzelheiten nach dem Merge. Dann ist zu prüfen, ob die
  Fehlerhinweise der CLI den Fall „Projekt archiviert" erwähnen sollten.
- Spaltenfarben (`color` an Spalten) kommen mit Minka M2-2d, nur lesend. Die
  JSON-Ausgabe reicht sie ungeändert durch. Ob die Textausgabe sie zeigt,
  wird entschieden, wenn der Commit da ist.

- Das Token `minka-cli-smoke` liegt in `~/.config/minka/config.json`.
  Widerrufen beim Umstieg auf das Dogfooding zur internen Alpha. Das
  Agentenkonto „Claude (Robert)" ist schon Mitglied im Minka-Projekt.
- Dateien in Gesprächen (Minka `f722d09`, E-112) sind umgesetzt: `uploads
  create` nimmt jetzt die Datei als Argument und den Kreis als Flag
  (`--project` oder `--conversation`), weil die Adresse den Kreis nennt. Live
  geprüft ist die neue Form für Projekte sowie die Fehlerfälle (kein Kreis,
  beide Kreise, unbekanntes Gespräch). Der Weg in ein echtes Gespräch fehlt,
  solange der Schreibtest für Gespräche nicht freigegeben ist.
- Reaktionen schreiben (Minka E-114) ist am 16.09.2026 live geprüft: `comments
  react` legt 👍 an (zweites Mal „already“), die Textansicht zeigt es, eine
  21 Zeichen lange Reaktion endet mit Exit 9 `reactionTooLong`, `unreact`
  räumt sie weg und ein `unreact` ohne Reaktion endet mit Exit 0. Dasselbe an
  einer Chat-Nachricht in „Chat1 Abnahme“. Testkarte und Nachricht gelöscht.
- Reaktionen (Minka `f006bd4`) sind am 16.09.2026 live geprüft, soweit es geht:
  `reactions` kommt an Chat-Nachrichten und an einem frisch geschriebenen
  Kommentar als leeres Array an. Mit Inhalt gefüllt ist es nur gegen den Fake
  geprüft, weil über die API niemand reagieren kann. Testkarte wieder gelöscht.
- Private Gespräche sind am 18.09.2026 vollständig live geprüft, schreibend —
  auf der Test-Instanz `dev.minka.flownative.com`, die Robert dafür
  eingerichtet hat, als Agentenkonto „Claude" (handelt für Robert Lemke), im
  Gespräch mit Shaun:
  - `conversations with` per E-Mail-Adresse und `--project`; der zweite Aufruf
    liefert dasselbe Gespräch, die eigene ID endet mit Exit 2.
  - Schreiben, Ändern (`editedAt`), `uploads create --conversation`, das
    `attachmentSource` zusammen mit einem Erwähnungs-Markup als HTML gesendet:
    beides steht danach im `bodySource`.
  - `messages react|unreact` (E-118), `chats mark-read`, `unreadCount` zählt
    eigene Nachrichten mit, `lastMessageText` ist auf 200 Zeichen gekürzt.
  - `messages delete` nimmt die Nachricht aus dem Verlauf (E-097).
  - Shauns Antwort kam an: gelesen, mit 🎉 beantwortet, Lesemarke gesetzt,
    `unreadCount` danach 0. Damit ist auch der Rückweg geprüft.
  - Dabei aufgefallen und behoben: `messages list --styled` zeigte weder
    Anhang noch Erwähnung, weil es den `bodyText` nahm. Es liest jetzt die
    Quelle, wie `cards show` es bei Kommentaren tut.
  - Dabei aufgefallen und an minka-92 gemeldet: Die `seq`-Nummern haben Lücken
    (1, dann 3), ohne dass eine Nachricht entfernt wurde. minka-92 hat es am
    Code bestätigt: `DbalChatRepository::nextNumberOf` ist ein Zähler je Chat,
    aus dem `seq` und `change_seq` gezogen werden; jede Änderung verbraucht
    eine Nummer. `seq` ist also eine aufsteigende Ordnungsnummer, keine
    lückenlose Position. SKILL.md, Typdoku und Hilfe sagen das jetzt so.
  - Am 16.09.2026 war es nur lesend geprüft: `conversations list` des
    Smoke-Kontos antwortete mit einer leeren Liste.
- Erwähnungen (Minka `800d27e`, E-115) sind am 16.09.2026 live geprüft: Das
  `mention.markup` aus `people list` als Kommentar mit `--format html`
  gespeichert kommt unverändert zurück und steht in `cards show --styled` als
  „@Bruno". Am 18.09.2026 auch in einem Gespräch, zusammen mit einem Anhang.
- Der Papierkorb (Minka E-116) ist am 16.09.2026 live geprüft: `trash list`
  zeigt die eigenen Testkarten mit Board, Spalte, Zeitpunkt und „by Bruno",
  alle `restorable`, ein fremdes Projekt endet mit Exit 2. Eine Karte daraus
  mit `cards restore` zurückgeholt und wieder gelöscht.
- Reaktionen schreiben (Minka E-114) ist am 16.09.2026 live geprüft: `comments
  react` legt 👍 an (zweites Mal „already"), die Textansicht zeigt es, eine
  21 Zeichen lange Reaktion endet mit Exit 9 `reactionTooLong`, `unreact`
  räumt sie weg, ein `unreact` ohne Reaktion endet mit Exit 0. Dasselbe an
  einer Chat-Nachricht in „Chat1 Abnahme".
- `kind`, `principal` und `retired` (Minka `18f69c7` E-124, `e6fc150` E-126)
  sind am 17. und 18.09.2026 live geprüft: auf der lokalen Instanz meldet `me`
  `human` ohne `principal`, auf der Test-Instanz das Agentenkonto `kind:
  agent`, `principal` als Paar aus ID und Name und `retired: false`;
  `people list` zeigt die Spalte KIND mit „agent for Robert Lemke". `retired`
  steht nur an `GET /me`, nicht an den Personenlisten; wer ausscheidet,
  verschwindet ohnehin aus ihnen (`StillInService`), und seine Anfragen enden
  mit Exit 3.
- Die Konfiguration hat seit dem 18.09.2026 zwei Hosts, Vorgabe ist
  `dev.minka.flownative.com` (Agentenkonto „Claude"). Für die lokale Instanz
  `--host https://minka.localbeach.net` mitgeben; dort gilt weiter das
  Smoke-Konto Bruno und die Freigabe nur für Board und Chat „…Abnahme".
- `cards restore` (Minka `20d5e44`, E-102) ist am 16.09.2026 live geprüft, an
  einer Testkarte im Board „API-3 Abnahme“: Nach `cards delete` ist die Karte
  weg (Exit 2), `cards restore` holt sie zurück nach Triage, das zweite
  `restore` endet mit Exit 2 und dem Hinweis, eine unbekannte ID ebenso.
  Testkarte danach wieder gelöscht.
- `cards mine` (Minka `a91a133`, E-101) ist am 16.09.2026 live geprüft: Eine
  eigene Karte mit Fälligkeit in der Vergangenheit steht in der Liste als
  „overdue“, nach `cards done` ist sie draußen. Dabei ist ein Fehler in Minka
  aufgefallen: `DELETE /api/v1/cards/{id}` antwortete mit 500
  (`DbalCardRepository::moveToTrash`, Exception 1789508136). Ursache war laut
  minka-c2 kein Fehler im Code, sondern das Merge-Fenster von E-102: Der Code
  kannte `trashed_at` schon, die Migration lief erst danach. Am 16.09.2026
  nachgeprüft: Karte in Done → `cards delete` → 204, `cards restore` bringt sie
  nach Done zurück.
- Uploads (Minka `30a19d2`) sind am 16.09.2026 live geprüft, im Test-Projekt:
  Ein PNG kommt als `kind: image` zurück, eine CSV-Datei als `kind: file` mit
  `contentType: text/csv`. Das `attachmentSource` aus der Antwort, unverändert
  als Beschreibung einer Testkarte gespeichert, steht danach im
  `descriptionSource` und erscheint in `cards show --styled` als
  Markdown-Bild auf die `display`-Variante. Eine CSV-Datei mit der Endung
  `.png` endet mit Exit 9 `contentTypeMismatch`, ein fremdes Projekt mit
  Exit 2. Testkarte danach gelöscht; die Uploads räumt Minka nach sieben Tagen
  selbst weg.
- Entfernte Chat-Nachrichten ohne Grabstein (Minka `2b6b3b1`, E-097) sind am
  16.09.2026 live im Chat „Chat1 Abnahme“ geprüft, an einer eigenen Nachricht:
  Nach `messages delete` ist sie weder ohne Parameter noch mit `--before` im
  Verlauf. `--after` trägt sie weiter, mit `removed: true` und leerem
  `bodyText`, `bodyHtml` und `bodySource`; die Textausgabe zeigt „(removed)“.
  `unreadCount` fällt von 1 auf 0, das zweite `delete` endet wieder mit Exit 0.
- Kommentare ändern und entfernen (Minka `6694096`) sind am 16.09.2026 live
  geprüft, an einer Testkarte im Board „API-3 Abnahme“: Markdown mit
  Zeilenumbruch kommt als `<strong>` und `<br>` an, `editedAt` ist gesetzt, die
  Textansicht zeigt „edited“. Eine unbekannte Kommentar-ID endet mit Exit 2 und
  dem Autor-Hinweis, das zweite `delete` ebenso. Testkarte danach gelöscht.
  Die Sperre bei Anhängen ist nur gegen den Fake getestet.
- Die Quellfelder (Minka `268065d`) sind am 16.09.2026 nur lesend live
  geprüft: `descriptionSource` an der Smoke-Karte, `bodySource` an ihrem
  Kommentar und an Chat-Nachrichten kommen an. Eine Karte mit Anhängen oder
  Erwähnungen gibt es für Bruno nicht, und die API kann keine anlegen. Die
  Darstellung von Anhängen und Erwähnungen und die Sperre in `cards edit` sind
  deshalb nur gegen den Fake getestet.
- Abonnenten (Minka `e610aac`) sind am 15.09.2026 live geprüft, an einer
  Testkarte im Board „API-3 Abnahme“:
  - Wer eine Karte anlegt, ist abonniert. `subscribe me` meldet „already“.
  - Clara per E-Mail-Adresse kommt dazu.
  - „Live“ ist mehrdeutig (Exit 8), eine fremde UUID endet mit Exit 9
    `personNotAProjectMember`.
  - `unsubscribe me` zweimal meldet erst `wasSubscribed: true`, dann „not
    subscribed“. Ein Kommentar danach setzt Bruno nicht zurück auf die Liste.
  - Die Testkarte ist wieder gelöscht.
- Chat-Nachrichten als Rich Text (Minka `a8eeb7a`) sind am 15.09.2026 live
  geprüft, im Chat „Chat1 Abnahme“:
  - Markdown mit Umbruch, Inline-Code, Link und Tabelle kommt als HTML an.
  - `--format text` macht der Server zu Absatz, Umbruch und Link.
  - `--format html` mit `mark` behält die Palettenfarbe und entfernt
    `script`.
  - `bodyText` wird aus dem HTML abgeleitet, auch beim Ändern.
  - Nur `<hr>` endet mit Exit 9 `chatMessageMissing`.
  - Die Testnachrichten sind wieder entfernt, der Chat ist als gelesen
    markiert.
- Die Chat-API (Minka `f9cc6f5`) ist am 15.09.2026 live geprüft, schreibend im
  Chat „Chat1 Abnahme“ in „Test-Projekt“, von minka-63 dafür freigegeben:
  - `chats list` zeigt die Chats mit ungelesenen Nachrichten.
  - `messages list` mit `--limit`, `--before` und `--after` funktioniert,
    `--limit 0` endet mit Exit 9 `chatMessagesLimitNotANumber`.
  - `messages create` mit `--client-message-id` schreibt die Nachricht; ein
    zweites Senden mit derselben ID liefert dieselbe Nachricht.
  - Zeilenumbruch und Autolink kommen im `bodyHtml` an.
  - `messages edit` erhöht `changeSeq`, und `--after` holt die Änderung nach.
  - `messages delete` zweimal ergibt beide Male Exit 0.
  - `chats mark-read` setzt die ungelesenen Nachrichten auf 0.
  - Eine leere Nachricht endet mit Exit 9 `chatMessageMissing`.
- Kommentare als Rich Text (Minka `c7c4d1b`) sind am 15.09.2026 live geprüft:
  - Markdown mit Zeilenumbruch, Inline-Code und Liste speichert der Server so,
    wie die CLI es schickt; `--format html` bleibt unverändert.
  - `cards show` zeigt beide als Markdown. Der alte Klartext-Kommentar der
    Smoke-Karte kommt als Klartext zurück und wird unverändert gezeigt.
  - Die Testkarte ist wieder gelöscht.
- Die erweiterte Allowlist (Minka `72d0639`) ist am 15.09.2026 live geprüft:
  - `h4`, `h6`, ein Link mit Titel, `ol start="3"`, `pre data-language="go"`
    und eine Tabelle mit `|` in einer Zelle speichert der Server so, wie die
    CLI sie schickt. Er ergänzt nur `rel="noopener"` an externen Links.
  - `cards show` → `cards edit` ergibt dasselbe HTML.
  - Die Testkarte ist wieder gelöscht.
- Schritte sind am 14.09.2026 live geprüft (Minka `ee39f45`):
  - Anlegen, auch über stdin, und die leere Liste funktionieren.
  - Abhaken per Position, doppeltes Abhaken meldet „already", Öffnen per Text
    funktioniert.
  - Ein Text auf zwei Schritten endet mit Exit 8, eine Position außerhalb der
    Liste mit Exit 1, ein Tab im Schritt mit Exit 9 `stepHasControlCharacters`.
  - Löschen per Position funktioniert, und `cards show` zeigt „Steps (0/2)".
  - Die Testkarte ist wieder gelöscht.
- Fälligkeitsdatum und Anlegen mit Zuständigen sind am 14.09.2026 live
  geprüft:
  - `cards create --assignee me --due 2026-09-28` legte Karte, Person und
    Datum in einem Request an, und das Board zeigt beides.
  - `--due` ändern, dasselbe Datum noch einmal und `--due ""` funktionieren.
  - `2026-02-30` endet mit Exit 9 `cardDueDateNotADate`.
  - Ein Nicht-Mitglied beim Anlegen endet mit Exit 9, das Board hatte danach
    genauso viele Karten wie vorher.
  - Die Testkarte ist wieder gelöscht.
- Zuständige sind am 14.09.2026 live geprüft:
  - `people list` für „Test-Projekt" funktioniert. Dort heißen zwei
    Mitglieder „Live", der Fall „Name mehrdeutig" ist also real.
  - `cards create --assignee me` weist zu, und die neue Karte lag oben in
    Triage.
  - Doppeltes Zuweisen meldet „already", Abnehmen und doppeltes Abnehmen
    melden sich richtig.
  - Eine fremde UUID endet mit Exit 9 `personNotAProjectMember`, ein
    unbekannter Name mit Exit 1.
  - Die Testkarte ist wieder gelöscht.
  - Die ID der neuen Karte war schon eine UUID v7 und ging problemlos durch
    die ID-Prüfung.
- `cards delete` ist am 13.09.2026 live geprüft: Eine frisch angelegte
  Karte kam in den Papierkorb, danach gaben `cards show` und ein zweites
  `cards delete` Exit 2, und das Board listet sie nicht mehr.
- Die Allowlist nach E-060 ist am 13.09.2026 auch live geprüft: Die
  Smoke-Karte bekam `#` bis `####`, Inline-Code, `---` und einen Code-Block.
  Der Server speichert `h1`–`h3`, `code`, `hr` und `pre > code` wie gesendet,
  und `cards show` → `cards edit` ergibt dasselbe HTML.
- Später vielleicht:
  - Keychain statt Datei
  - `.minka.json` im Repository mit Standard-Board
  - `--help --agent`
  - Installation für andere

## Wünsche an die API

Die ersten vier am 13.09.2026 an minka-8a gemeldet. Wie die Koordination sie
behandelt:

1. **Allowlist erweitern** (`h2`/`h3`, `code` inline, `hr`): erledigt in
   Minka `1f46b37` (E-060), in der CLI umgesetzt. Tabellen, `ol[start]` und
   Checklisten folgen später mit Lexxy (U3). Bis dahin verliert der
   Web-Editor `hr` beim Speichern.
2. **Falsche HTTP-Methode** als JSON-404/405 statt Flows HTML-404: bleibt
   Schuld, nicht vor dem Dogfooding.
3. **Karten entfernen** (`DELETE /cards/{id}` → Papierkorb): erledigt in
   Minka `60c7257`, in der CLI `cards delete`.
4. **Done über 25 Karten hinaus lesen** (Blättern): später, wenn das
   Dogfooding es braucht.
5. **Erwähnungen für Agenten**: am 16.09.2026 gemeldet und am selben Tag
   erledigt (Minka `800d27e`, E-115). `mention: {handle, markup}` steht an
   jedem Eintrag von `/api/v1/projects/{projectId}/people` und des neuen
   `/api/v1/messages/{conversationId}/people`; in der CLI umgesetzt als
   `people list` und `conversations people`.
6. **Papierkorb-Liste**: am 16.09.2026 erbeten, erledigt mit E-116.
   `GET /api/v1/projects/{projectId}/trash`, in der CLI `trash list`.
