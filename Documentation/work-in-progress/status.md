# Stand

Der erste Blick jeder neuen Session. Was gilt, steht unter `stable/`; hier
steht, wo die Arbeit gerade ist.

## Wo wir stehen

Die CLI bedient, was `/api/v1` anbietet: Projekte, Boards, Karten samt
Papierkorb, Spalten, Zuständige, Fälligkeiten, Schritte, Abonnenten,
Kommentare, Chats, private Gespräche, Uploads, Erwähnungen, Reaktionen und die
eigenen Karten über alle Projekte. `minka commands` zählt sie alle auf, aus dem
Befehlsbaum selbst gebaut.

Alles ist gegen die Fake-API getestet und live geprüft — die Kette
Upload → Anhang im Text → Anzeige ebenso wie private Gespräche in beide
Richtungen. Was nur gegen den Fake geprüft ist, steht unten.

Koordination der API: die Minka-Session `minka-92`. Sie meldet Änderungen, die
gegen den Minka-Code abgeglichen werden; Abweichungen gehen an sie zurück.

Das Dogfooding wartet auf die interne Alpha. Das Smoke-Token
`minka-cli-smoke` (Konto Bruno, lokale Instanz) wird dann widerrufen.

## Nur gegen den Fake geprüft

- Dass ein Gespräch mit jemandem aus einer fremden Organisation 404 gibt: Die
  Fake-API kennt keine Organisationen. Am Minka-Code gelesen.
- Dass ein fremdes Attachment beim Speichern stillschweigend verschwindet: Der
  Fake entfernt es nicht.
- Unicode-NFC für den Inhalt einer Reaktion: Der Server normalisiert, der Fake
  nicht.
- „agent for X" in `people list` ist live gesehen, ein ausgeschiedener Agent
  nicht — er steht in keiner Liste mehr.

## Offen in der API

- Schritte umbenennen und umsortieren gibt es nicht.
- Done über 25 Karten hinaus blättern gibt es nicht; wird gebraucht, wenn das
  Dogfooding es braucht.
- Eine falsche HTTP-Methode beantwortet Flow mit HTML statt JSON.
- Spaltenfarben kommen später, nur lesend.
- Projekt archivieren und in den Papierkorb legen (E-129) steht. Ein Projekt
  im Papierkorb antwortet überall 404. Ein archiviertes bleibt unter seinen
  Adressen lesbar, fehlt aber in `projects list` und `me cards`; Schreiben
  antwortet 404 `notFound`, bei Abonnenten 422 `personNotAProjectMember`. Der
  Hinweis der CLI zu 404 nennt diesen Fall nicht.
- Zugesagt von der Koordination, noch nicht gemerged: Schreiben eines Mitglieds
  in ein archiviertes Projekt antwortet einheitlich 422 `projectArchived`, auch
  Uploads; `chats read` bleibt erlaubt; die Projektressource bekommt `status`
  (`active|archived`). Mit der Merge-Meldung: gegen den Code abgleichen, Fake
  anpassen, Hinweis zu `projectArchived` und `status` in der Ausgabe ergänzen.
  Bis dahin bleibt der Hinweistext, wie er ist.

## Nächste Schritte

- Auf Meldungen von `minka-92` reagieren: gegen den Code abgleichen, Fake
  zuerst, dann die CLI, Test und Sabotagebeweis, live prüfen, Doku im selben
  Commit.
- Die interne Alpha vorbereiten: Token wechseln, SKILL.md gegenlesen.
