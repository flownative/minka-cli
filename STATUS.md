# Status

Stand: 13.09.2026

## Wo wir stehen

**MVP fertig und reviewt.** Gegen den Fake getestet, im Smoke-Lauf gegen die
lokale Instanz geprüft, Befunde des Reviews eingearbeitet.

**Die CLI ruht auf diesem Stand** (Roberts Priorität, über minka-8a): Das
Dogfooding in Minka wartet auf eine interne Alpha. Neue Features gibt es nur
auf neuen Auftrag.

API-Stand, gegen den gebaut ist: Minka `ee39f45` (API-1 bis API-5,
erweiterte Allowlist aus E-060, Zuständige an Karten, neue Karten oben in der
Spalte, Fälligkeitsdatum, Zuständige und Datum beim Anlegen, Schritte).
Koordination in Minka: `minka-63`.

Befehle:

```
minka auth login|status|logout
minka me
minka projects list
minka boards list <projectId> | show <boardId>
minka people list <projectId>
minka cards show|create|edit|move|assign|unassign|done|delete
minka steps list|create|complete|reopen|delete
minka comments create <cardId> <text|->
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
- **Markdown → HTML** (goldmark), nur Elemente der Server-Allowlist:
  - `#` bis `###` werden `h1` bis `h3`, tiefere Überschriften `h3`.
  - Inline-Code wird `code`, Code-Blöcke werden `pre > code` (ohne Sprache).
  - `---` wird `hr`.
  - Tabellen werden ausgerichteter Text in `pre`.
  - Bilder werden Links, ein Bild in einem Link wird nur sein Label.
  - Rohes HTML im Markdown wird sichtbarer Text.
  - Links mit anderen Zielen als http, https, mailto, tel und relativen URLs
    werden Klartext. Das ist dieselbe Regel wie `RichText::isSafeUrl` auf dem
    Server.
  - Markdown, das Text enthält, aber nichts, was Minka behält, wird
    abgelehnt, statt die Beschreibung zu löschen. Löschen geht mit
    `--description ""`.
- **HTML → Markdown** für `cards show` im Text: Text wird escaped, `<br>`
  wird ein harter Umbruch. Zurückgeschickt entsteht dasselbe HTML.
- **Kommentare** sind Klartext, ohne Umwandlung.
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
- **Schritte:**
  - Die Gruppe `steps` hat `list`, `create`, `complete`, `reopen` und
    `delete`.
  - Ein Schritt wird über ID, Position ab 1 oder Text angegeben. Eine Zahl
    ist immer eine Position. Ein Text auf zwei Schritten ergibt Exit 8 mit den
    Positionen.
  - Die API liefert Schritte nur an der einzelnen Karte, nicht im Board.
    Deshalb liest jeder Befehl mit Schritt-Angabe zuerst die Karte.
  - Wiederholtes Abhaken, Öffnen und Löschen ist kein Fehler; die
    Zusammenfassung sagt, ob sich etwas geändert hat.
  - `steps list` gibt als `data` die Liste `steps` aus der Karte unverändert
    aus.
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
| 204 ohne Rumpf ist Erfolg | `TestAnEmptyAnswerIsASuccess` (api), `TestDeleteMovesACardToTheTrash` |
| Nachbar in Zielspalte geprüft | `TestMoveNextToACardOutsideTheTargetColumnIsRefused` |
| Done ohne Platzierung | `TestDoneTakesNoPlacement` |
| Anlegen in Done/Not now auch per ID abgelehnt | `TestCreateInNotNowOrDoneIsRefusedHoweverTheColumnIsGiven` |
| Typname und gleicher Titel mehrdeutig | `TestATypeThatIsAlsoTheTitleOfAnotherColumnIsAmbiguous` |
| HTML → Markdown: Zeilenanfänge escaped, `<br>` hart | `TestTextWithoutMarkupStaysText` |

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

- Schritte (`ee39f45`) sind gebaut, während in Minka noch ein Review läuft.
  minka-63 meldet, falls sich der Vertrag noch ändert.
- Gegenüber der Nachricht zu `ee39f45` gilt laut Code:
  - Das Board listet keine Schritte.
  - `DELETE` auf einen Schritt einer anderen Karte oder auf eine ungültige
    Schritt-ID gibt 204, nicht 404. 404 gibt es nur für eine fremde Karte.
- Umbenennen und Umsortieren von Schritten gibt es in der API noch nicht.
- Projekte haben seit Minka `ccd0439` Chat-Werkzeuge (`type=chat`), aber noch
  keine API; die kommt mit Chat-3. `boards list` zeigt nur Boards.

- Spaltenfarben (`color` an Spalten) kommen mit Minka M2-2d, nur lesend. Die
  JSON-Ausgabe reicht sie ungeändert durch. Ob die Textausgabe sie zeigt,
  wird entschieden, wenn der Commit da ist.

- Das Token `minka-cli-smoke` liegt in `~/.config/minka/config.json`.
  Widerrufen beim Umstieg auf das Dogfooding zur internen Alpha. Das
  Agentenkonto „Claude (Robert)" ist schon Mitglied im Minka-Projekt.
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

## Wünsche an die API (an minka-8a)

Gemeldet am 13.09.2026. Wie minka-8a sie behandelt:

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
