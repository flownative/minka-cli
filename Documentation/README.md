# Dokumentation der Minka-CLI

Das Kartenblatt über die gesamte Dokumentation: eine Zeile pro Dokument, mit
einem Satz dazu, wann man es braucht. Es gibt bewusst nur **diesen einen**
Index.

**Regeln:** `stable/` beschreibt die geltende Praxis · `guides/` sind erprobte
Abläufe · `work-in-progress/` ändert sich häufig. Grundsatz über allem: Die
Dokumentation sagt, was gilt; wie es dazu kam, steht in git.

## Geltende Regeln

- [Ausgabe, Fehler und Exit-Codes](stable/output-contract.md): Die drei
  Ausgabemodi, die Hülle, die Exit-Code-Tabelle, wie Befehle geschnitten sind —
  vor jeder Arbeit an einem Befehl lesen.
- [Token, Hosts und Anfragen](stable/security.md): Wie das Token
  aufbewahrt und verschickt wird, was am HTTP-Client festliegt, welche Prüfung
  vor einer Anfrage passiert — vor jeder Arbeit an Anmeldung, Client oder IDs.
- [Rich Text in beide Richtungen](stable/rich-text.md): Markdown zu HTML und
  zurück, die Allowlist des Servers, Quellen, Dateien und Erwähnungen — vor
  jeder Arbeit an Beschreibungen, Kommentaren oder Nachrichten.
- [Testen](stable/testing.md): Fake-API, Sabotagebeweise, was ein Test
  festhält — vor dem ersten eigenen Test lesen.

## Anleitungen

- [Live-Prüfung](guides/live-check.md): Wo geschrieben werden darf, in welcher
  Reihenfolge geprüft wird, was danach aufgeräumt wird — vor jedem Lauf gegen
  eine echte Instanz.

## Entscheidungen

- [Entscheidungsregister](decisions.md): Je Entscheidung, was gilt, warum und
  wo sie umgesetzt ist (C-001…). Nachschlagen, nicht am Stück lesen.
  Entscheidungen über die API selbst stehen in Minkas Register (E-…).

## Work in Progress

- [Stand](work-in-progress/status.md): Was die CLI kann, was nur gegen den Fake
  geprüft ist, was in der API offen ist — der erste Blick jeder neuen Session.

---

Bei Code-Änderungen gehört das Doku-Update in denselben Commit. Veraltetes
wird gelöscht, nicht stehen gelassen; beim Anlegen oder Verschieben eines
Dokuments wird dieser Index mitgepflegt.
