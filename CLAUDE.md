# CLAUDE.md

Anweisungen für Claude Code für die Arbeit an diesem Repository.

## Unterhaltung

Die Sprache für Unterhaltungen mit Robert ist Deutsch mit informellem "du".
Code, Kommentare, Commit-Messages, Hilfetexte und Ausgaben der CLI sind
Englisch.

## Was das ist

`minka` ist die Kommandozeile für die JSON-API von Minka (`/api/v1`). Zuerst
für das Dogfooding: Robert und Claude-Sessions pflegen damit die
Entwicklungsaufgaben von Minka im lokalen Minka-Board. Sie ist ausdrücklich
auch für KI-Agenten gebaut: stabile `--json`-Ausgabe, Exit-Codes je
Fehlerklasse, eine Befehlsbeschreibung in `skills/minka/SKILL.md`.

Vorbild für Konzepte ist [basecamp-cli](https://github.com/basecamp/basecamp-cli):
Prinzipien übernehmen, nicht den Umfang. Nicht dazu gehören: KI-Integrationen,
Uploads, Benachrichtigungen, eine interaktive TUI, alles, was die API nicht
anbietet.

Proprietär wie Minka, bis Minka veröffentlicht wird. Nur ein lokales
Git-Repository, **kein Remote, niemals pushen**, solange Robert es nicht
ausdrücklich verlangt.

## Stand und Status

Der aktuelle Stand, offene Punkte und Wünsche an die API stehen in
[STATUS.md](STATUS.md). Vor jeder Arbeit lesen, danach nachführen.

## Stack und Aufbau

Go (siehe `go.mod`), [cobra](https://github.com/spf13/cobra) für Befehle,
[goldmark](https://github.com/yuin/goldmark) für Markdown,
`golang.org/x/net/html` für HTML, `golang.org/x/term` für die verdeckte
Token-Eingabe.

```
cmd/minka/            main: ruft cli.Run auf und beendet mit dessen Exit-Code
internal/api/         HTTP-Client und Typen der Ressourcen
internal/config/      Hosts und Tokens in einer Datei mit 0600
internal/markdown/    Markdown → HTML, nur Elemente, die der Server behält
internal/richtext/    HTML einer Beschreibung → Markdown zum Lesen
internal/cli/         Befehle, Ausgabe, Exit-Codes
internal/fakeapi/     In-Memory-Minka-API für Tests (httptest)
skills/minka/         SKILL.md für Agenten
```

## Befehle

```bash
make build        # bin/minka
make test         # go test ./...
make check        # gofmt-Prüfung, go vet, Tests; vor jedem Bericht und Commit
```

## Der API-Vertrag

Die Quelle der Wahrheit liegt im Minka-Repository, **nur lesen**:

- `/Users/robert/Sites/minka/Documentation/stable/02-architecture/authorization.md`,
  Abschnitt „API" (Endpunkte, Fehlercodes)
- `/Users/robert/Sites/minka/Documentation/decisions.md`, E-057 mit
  „Umgesetzt als"
- Code unter `DistributionPackages/Flownative.Minka/Classes/Presentation/Api/`

`internal/fakeapi` bildet diesen Vertrag nach. Ändert sich die API, zuerst
den Fake anpassen, dann die CLI.

Die API ist „intern, instabil". Fragen, Widersprüche und Wünsche an die API
gehen per Nachricht an die Minka-Koordinator-Session, nicht als Änderung ins
Minka-Repository. Bis 14.09.2026 war das `minka-8a`, seitdem ist es
`minka-63`. Nachrichten über API-Änderungen mit dem Code
abgleichen: Maßgeblich sind Controller, Resources und Tests im Commit.

## Regeln gegenüber dem Minka-Repository

`/Users/robert/Sites/minka` ist **nur zum Lesen** da. Keine Dateien ändern,
keine Git-Befehle, die etwas ändern, kein `composer`, `phpunit`,
`npm run test:browser`, keine Migrationen: Es gibt genau eine
Testdatenbank, und andere Agenten testen parallel darin.

Erlaubt: lesen, `beach exec ./flow token:create|token:list|token:revoke`,
Live-Aufrufe gegen `https://minka.localbeach.net`. **Geschrieben wird live nur
im Board „API-3 Abnahme" und im Chat „Chat1 Abnahme", beide im Projekt
„Test-Projekt".** Der Chat „Sommerfest" dort ist Roberts Demo und bleibt
unberührt. Gelöscht werden nur Karten und Nachrichten, die ein Test selbst
angelegt hat.

Tokens stehen nie im Repository, in Commits, Tests oder Logs, nur in der
Konfigurationsdatei der CLI (`~/.config/minka/config.json`, 0600). Für
Entwicklung und Smoke-Tests: Token für `bruno@example.com`.

## Konventionen

- **Konventionen von basecamp-cli**, damit ein Agent, der eine der beiden CLIs
  kennt, die andere lesen kann: Befehle als Plural-Gruppe plus Aktion
  (`cards move`), Text auf einem Terminal, sonst JSON.
- **Die JSON-Ausgabe ist ein Vertrag.** `--json`:
  `{ok, data, summary, notice, breadcrumbs}`, `data` ist die Ressource der API
  unverändert; `--quiet`: nur `data`; Fehler:
  `{ok: false, error, code, retryable, hint, meta: {apiCode, httpStatus}}`.
  Felder dürfen dazukommen, nie verschwinden oder umbenannt werden.
- **Exit-Codes sind ein Vertrag**, dieselben wie bei basecamp-cli. Tabelle in
  `internal/cli/errors.go` und `skills/minka/SKILL.md`.
- **Freitext kommt nie über Shell-Quoting allein.** Beschreibung und
  Kommentar nehmen `-` für stdin an; Titel sind einzeilig und kommen als Flag.
- **Ein Token geht nur an seinen Host.** `MINKA_TOKEN` gilt allein für
  `MINKA_HOST`, sonst kommt das Token aus der Config-Datei für genau den Host.
- **Tests gegen den Fake**, nicht gegen die lokale Instanz. Jede Schutzmaßnahme
  (Dateirechte, Pfad-Escaping, keine Redirects, Allowlist des Markdowns …)
  bekommt einen Test und einen Sabotagebeweis: Schutz herausnehmen, Test wird
  rot, Schutz zurück. Den Beweis in STATUS.md notieren.
- Kommentare nach dem Skill `code-comments` (Intention statt Mechanik,
  zeitlos, keine Verweise auf Karten, Doku oder Entscheidungs-IDs).
- Commit-Messages nach dem Skill `git-workflow`, Englisch, keine
  Session-Links.

## Arbeitsweise mit Unteragenten

Wie in Minka (`Documentation/work-in-progress/bootstrap/operating-model.md`):
Implementierer baut, ein Reviewer, der nichts ändert, prüft. Niemand prüft
sein eigenes Werk. Recherche, die viel Text erzeugt, geht an einen
Unteragenten mit knappem Berichtsformat.

## Zentrale Best Practices (Flownative Skills)

Skills aus github.com/flownative/flownative-best-practices, per Symlink unter
`.claude/skills`. Aktualisieren: `git -C .claude/skills pull origin main`.

| Aufgabe | Skill |
|---------|-------|
| Code-Kommentare | `/code-comments` |
| Refactoring, Code entfernen, Code Review | `/code-quality`, `/review` |
| Commit Message, Branch aufräumen | `/git-workflow` |
| Neue Best Practice festhalten | `/learn-best-practice` |

Die Skills sind überwiegend für PHP, Flow und Neos geschrieben. Übertragbar
sind die Regeln zu Kommentaren, Benennung, Commits und Reviews; bei
Widersprüchen gewinnt dieses Repository.
