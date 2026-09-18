# Testen

`make check` ist die Schwelle: gofmt-Prüfung, `go vet`, alle Tests. Läuft vor
jedem Bericht und jedem Commit.

## Gegen die Fake-API, nicht gegen eine Instanz

`internal/fakeapi` bildet den Vertrag von `/api/v1` nach: dieselben Pfade,
Statuscodes und Fehlercodes, in derselben Reihenfolge geprüft, mit den
bytegleichen 401- und 404-Rümpfen. Sie hält die Anfragen fest, sodass ein Test
sieht, was wirklich hinausging (C-005).

Ändert sich die API, wird zuerst der Fake angepasst, dann die CLI. Was der
Fake nicht nachbildet, gehört als Satz in den Status, damit niemand eine
Sicherheit annimmt, die es nicht gibt.

## Sabotagebeweise

Jede Schutzmaßnahme bekommt einen Test und einen Beweis, dass er greift: den
Schutz in einer Kopie des Repositories herausnehmen, prüfen, dass der Code
kompiliert, und den Test rot werden sehen (C-006).

Ein Beweis, der nicht kompiliert, ist keiner — dann sagt der Compiler nein,
nicht der Test. Bleibt ein Test grün, fehlt eine Zusicherung: erst den Test
schärfen, dann den Beweis wiederholen.

Die Kopie liegt im Scratchpad, nie im Repository.

## Live prüfen

Ein Live-Lauf zeigt, was kein Fake zeigt: die echten Antworten des Servers.
Dafür gelten Regeln, siehe [Live-Prüfung](../guides/live-check.md).

## Was ein Test festhält

- Den Vertrag: Hülle, Exit-Code, `meta.apiCode`, die gesendete Anfrage.
- Die Regel, nicht die Formulierung — außer der Hinweis ist selbst die Hilfe,
  die jemand braucht (der Autor-Hinweis bei 404, der Rat zu
  `--content-type`).
- Die Fälle, die eine API-Meldung beschreibt: 201 gegen 200, doppelt
  ausgeführt, von jemand anderem, an der falschen Adresse.
