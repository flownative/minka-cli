# CLAUDE.md

Anweisungen für Claude Code für die Arbeit an diesem Repository.

## Unterhaltung

Auftraggeber ist Robert; Shaun vertritt ihn mit denselben Rechten. Die Sprache
für Unterhaltungen mit beiden ist Deutsch mit informellem "du". Code,
Kommentare, Commit-Messages, Hilfetexte und Ausgaben der CLI sind Englisch.

## Was das ist

`minka` ist die Kommandozeile für die JSON-API von Minka (`/api/v1`). Zuerst
für das Dogfooding: Robert und Claude-Sessions pflegen damit die
Entwicklungsaufgaben von Minka. Sie ist ausdrücklich auch für KI-Agenten
gebaut: stabile `--json`-Ausgabe, Exit-Codes je Fehlerklasse, eine
Befehlsbeschreibung in `skills/minka/SKILL.md`.

Vorbild für Konzepte ist [basecamp-cli](https://github.com/basecamp/basecamp-cli):
Prinzipien übernehmen, nicht den Umfang. Nicht dazu gehören: KI-Integrationen,
Benachrichtigungen, eine interaktive TUI, alles, was die API nicht anbietet.

Proprietär wie Minka, bis Minka veröffentlicht wird. Das Repository liegt auf
GitHub unter `git@github.com:flownative/minka-cli.git`; der Stand kommt
regelmäßig nach `main`. Andere Branches und alles, was Geschichte umschreibt,
nur auf Ansage des Auftraggebers.

## Dokumentation

- **Index über alle Dokumente:** [Documentation/README.md](Documentation/README.md)
  — eine Zeile pro Dokument mit dem Hinweis, wann man es braucht. Minimal
  starten, bei Bedarf nachladen.
- **Erster Blick jeder Session:** [Stand](Documentation/work-in-progress/status.md).
- **Entscheidungsregister:** [Documentation/decisions.md](Documentation/decisions.md)
  (C-001…) — je Entscheidung kurz: was gilt, warum, wo umgesetzt.
- Struktur und Lebenszyklus der Doku: `.claude/rules/documentation.md` (lädt
  automatisch bei Arbeit unter `Documentation/`).

Nichts über vorhandenen Code annehmen, sondern nachsehen. Doku-Update gehört in
denselben Commit wie die Code-Änderung.

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
internal/richtext/    HTML eines Textes → Markdown zum Lesen
internal/cli/         Befehle, Ausgabe, Exit-Codes
internal/fakeapi/     In-Memory-Minka-API für Tests (httptest)
skills/minka/         SKILL.md für Agenten
```

```bash
make build        # bin/minka
make test         # go test ./...
make check        # gofmt-Prüfung, go vet, Tests; vor jedem Bericht und Commit
```

## Der API-Vertrag

Die Quelle der Wahrheit liegt im Minka-Repository, **nur lesen**:

- `Documentation/stable/02-architecture/authorization.md`, Abschnitt „API"
- `Documentation/decisions.md` (E-…)
- Code unter `DistributionPackages/Flownative.Minka/Classes/Presentation/Api/`

`internal/fakeapi` bildet diesen Vertrag nach. Ändert sich die API, zuerst den
Fake anpassen, dann die CLI.

Die API ist „intern, instabil". Fragen, Widersprüche und Wünsche gehen per
Nachricht an die Minka-Koordinator-Session (derzeit `minka-92`), nicht als
Änderung ins Minka-Repository. Jede Meldung über eine API-Änderung mit dem Code
abgleichen: Maßgeblich sind Controller, Resources und Tests, nicht die
Meldung. Weicht beides ab, geht das zurück an die Koordination.

## Regeln gegenüber dem Minka-Repository

`/Users/robert/Sites/minka` ist **nur zum Lesen** da. Keine Dateien ändern,
keine Git-Befehle, die etwas ändern, kein `composer`, `phpunit`,
`npm run test:browser`, keine Migrationen: Es gibt genau eine Testdatenbank,
und andere Agenten testen parallel darin.

Erlaubt: lesen, `beach exec ./flow token:create|token:list|token:revoke`,
Live-Aufrufe. Wo live geschrieben werden darf, steht in
[Live-Prüfung](Documentation/guides/live-check.md).

Tokens stehen nie im Repository, in Commits, Tests oder Logs, nur in der
Konfigurationsdatei der CLI (`~/.config/minka/config.json`, 0600).

## Konventionen

Die geltenden Regeln stehen unter `Documentation/stable/`; ihre Begründungen im
Register. Für die Arbeit selbst gilt:

- Kommentare nach dem Skill `code-comments` (Intention statt Mechanik,
  zeitlos, keine Verweise auf Karten, Doku oder Entscheidungs-IDs).
- Commit-Messages nach dem Skill `git-workflow`, Englisch, keine
  Session-Links.
- Jede Schutzmaßnahme bekommt einen Test und einen Sabotagebeweis
  ([Testen](Documentation/stable/testing.md)).

## Arbeitsweise mit Unteragenten

Wie in Minka: Implementierer baut, ein Reviewer, der nichts ändert, prüft.
Niemand prüft sein eigenes Werk. Recherche, die viel Text erzeugt, geht an
einen Unteragenten mit knappem Berichtsformat.

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
