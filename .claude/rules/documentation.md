---
paths:
  - "Documentation/**"
---

# Dokumentationsstruktur und -Lebenszyklus

Der vollständige Index ist `Documentation/README.md` — eine Zeile pro Dokument
mit dem Hinweis, wann man es braucht. Keine Unter-Indizes anlegen.

Grundsatz über allem: **Status quo statt Werdegang.** Die Dokumentation sagt,
was gilt; wie es dazu kam, steht in git.

## Typen

- **`stable/`** — Etabliertes Wissen, immer aktuell mit dem Code.
- **`guides/`** — Erprobte Schritt-für-Schritt-Abläufe.
- **`work-in-progress/`** — Aktive Arbeit, ändert sich häufig.

Ein Archiv gibt es nicht: Was niemand mehr braucht, wird gelöscht. Messwerte
und Rückblicke gehören in die Commit-Message.

## Entscheidungen

Zwei Leser, zwei Orte: Die Regel steht normativ unter `stable/` und nennt die
ID, die Begründung steht in einem Satz im Register (`Documentation/decisions.md`).

- IDs (`C-001`) sind stabil und werden nie wiederverwendet.
- Ein Eintrag hat höchstens acht Zeilen: *was gilt*, *warum* (ein Satz), *gilt
  für*, *umgesetzt als* (Komponente, nicht Commit-Hash).
- Ändert sich eine Entscheidung im selben Geltungsbereich, wird der Eintrag
  überschrieben; kippt sie, bekommt die neue eine neue ID und die alte die
  Zeile „überholt durch C-xxx".
- Entscheidungen über die **API** gehören nach Minka, nicht hierher. Hier steht
  nur, was die CLI für sich entscheidet.

## Ton

Präsens, normativ. Kein „früher", „seit", „inzwischen", kein Datum und kein
Commit-Hash im Fließtext. Eine Live-Prüfung ist kein Dokument, sondern ein
Satz im Stand — und wenn sie etwas verändert hat, eine Zeile in der
Commit-Message.

## Lebenszyklus

Doku-Update gehört in denselben Commit wie die Code-Änderung. Veraltetes
löschen statt stehen lassen. Beim Anlegen oder Verschieben den Index
mitpflegen.
