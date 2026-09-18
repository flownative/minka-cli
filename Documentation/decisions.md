# Entscheidungsregister

Je Entscheidung: was gilt, warum, wofür, wo umgesetzt. Nachschlagen, nicht am
Stück lesen. IDs bleiben stehen und werden nie wiederverwendet; kippt eine
Entscheidung, bekommt die neue eine neue ID und die alte die Zeile „überholt
durch".

Entscheidungen über die **API** trägt Minka selbst
(`/Users/robert/Sites/minka/Documentation/decisions.md`, E-…). Hier steht nur,
was die CLI für sich entscheidet.

<a id="c-001"></a>
## C-001 — Die Konventionen von basecamp-cli, nicht sein Umfang

*Gilt für: Befehlsnamen, Ausgabe, Exit-Codes*

Befehle heißen Plural-Gruppe plus Aktion, die Ausgabe folgt basecamp-cli.
Nicht übernommen: Umfang und Funktionsbereiche.

**Warum:** Ein Agent, der eine der beiden CLIs kennt, kann die andere lesen,
ohne sie zu lernen. **Umgesetzt als:** `internal/cli`, siehe
[Ausgabe](stable/output-contract.md).

<a id="c-002"></a>
## C-002 — `data` ist die Ressource der API, roh durchgereicht

*Gilt für: jede Ausgabe mit `--json` und `--quiet`*

Die CLI reicht das JSON der API unverändert durch, statt es neu
zusammenzusetzen.

**Warum:** Ein Feld, das die API dazubekommt, erreicht jeden Client sofort, und
die CLI kann nichts verfälschen. **Umgesetzt als:** `api.Result.Raw`,
`internal/cli/output.go`.

<a id="c-003"></a>
## C-003 — Exit-Codes sind ein Vertrag

*Gilt für: jeden Fehlerweg*

Eine Fehlerklasse hat genau einen Exit-Code, die Tabelle steht in
`internal/cli/errors.go` und in der SKILL.md.

**Warum:** Ein Agent entscheidet am Exit-Code, ob er wiederholt, nachfragt oder
aufgibt — ohne Text zu lesen. **Umgesetzt als:** `internal/cli/errors.go`.

<a id="c-004"></a>
## C-004 — Ein Token geht nur an seinen Host

*Gilt für: Anmeldung, jede Anfrage*

`MINKA_TOKEN` gilt nur mit `MINKA_HOST` und nur für diesen Host; für jeden
anderen kommt das Token aus der Konfigurationsdatei oder gar keins.

**Warum:** Sonst trüge ein `--host` das Token einer Instanz an eine fremde.
**Umgesetzt als:** `internal/cli/root.go` (`session`), siehe
[Token](stable/security.md).

<a id="c-005"></a>
## C-005 — Tests laufen gegen eine nachgebaute API

*Gilt für: alle Tests außer dem Live-Lauf*

`internal/fakeapi` bildet den Vertrag nach, bis hin zu den bytegleichen 401-
und 404-Rümpfen. Ändert sich die API, wird zuerst der Fake angepasst.

**Warum:** Tests gegen eine laufende Instanz wären langsam, unzuverlässig und
würden fremde Daten anfassen. **Umgesetzt als:** `internal/fakeapi`.

<a id="c-006"></a>
## C-006 — Jede Schutzmaßnahme hat einen Sabotagebeweis

*Gilt für: Dateirechte, Escaping, Allowlist, Sperren*

Schutz in einer Kopie herausnehmen, kompilieren, Test rot sehen. Ein Beweis,
der nicht kompiliert, zählt nicht; bleibt der Test grün, wird er geschärft.

**Warum:** Ein Test, der auch ohne den Schutz grün bleibt, sichert nichts.
**Umgesetzt als:** Verfahren, siehe [Testen](stable/testing.md).

<a id="c-007"></a>
## C-007 — Freitext kommt über stdin

*Gilt für: Beschreibung, Kommentar, Nachricht*

Mehrzeiliger Text wird mit `-` von stdin gelesen; Titel sind einzeilig und
stehen in Flags.

**Warum:** Shell-Quoting ist die häufigste Fehlerquelle eines Agenten, der
Text mit Anführungszeichen und Umbrüchen schickt. **Umgesetzt als:**
`app.readValue`.

<a id="c-008"></a>
## C-008 — Markdown hinein, Markdown heraus

*Gilt für: Beschreibungen, Kommentare, Nachrichten*

Die CLI nimmt Markdown und zeigt Markdown; die Quelle eines Textes ist
maßgeblich, und Anhänge und Erwähnungen gehen nur mit `--drop-attachments`
verloren.

**Warum:** Was die CLI zeigt, soll sich ändern und zurückschicken lassen, ohne
etwas zu verlieren. **Umgesetzt als:** `internal/markdown`, `internal/richtext`,
siehe [Rich Text](stable/rich-text.md).

<a id="c-009"></a>
## C-009 — Was dieselbe Adresse hat, bekommt denselben Befehl

*Gilt für: private Gespräche*

Die ID eines Gesprächs geht an `messages list|create|edit|delete|react|unreact`
und `chats mark-read`; eigene Befehle gibt es nur für das, was es nur dort
gibt (`conversations list|with|people`).

**Warum:** Die API bedient beides unter denselben Endpunkten; eine zweite
Befehlsreihe wäre doppelte Oberfläche für dieselbe Sache. **Umgesetzt als:**
`internal/cli/conversations.go`.

<a id="c-010"></a>
## C-010 — Der Kreis eines Uploads steht in einem Flag

*Gilt für: `uploads create`*

Die Datei ist das Argument, der Kreis ein Flag: `--project` oder
`--conversation`, genau eines von beiden.

**Warum:** Zwei ID-Arten auf derselben Position ließen die CLI raten, an welche
Adresse sie schickt. **Umgesetzt als:** `internal/cli/uploads.go`.

<a id="c-011"></a>
## C-011 — Live wird nur dort geschrieben, wo es freigegeben ist

*Gilt für: jeden Live-Lauf*

Erlaubt sind die Test-Instanz mit dem Agentenkonto und auf der lokalen Instanz
Board und Chat „…Abnahme". Alles andere entscheiden Robert oder Shaun.

**Warum:** Ein Schreibvorgang erreicht echte Menschen und fremde Daten.
**Umgesetzt als:** [Live-Prüfung](guides/live-check.md).

<a id="c-012"></a>
## C-012 — `retryable` nur, wo ein zweiter Versuch nichts anrichtet

*Gilt für: jeden Fehler*

Wahr bei lesenden Anfragen und bei `messages create` mit
`--client-message-id`; sonst falsch, mit dem Hinweis, den Stand nachzulesen.

**Warum:** Ein wiederholter Schreibvorgang legt sonst eine Karte oder einen
Kommentar zweimal an. **Umgesetzt als:** `internal/cli/errors.go`.

<a id="c-013"></a>
## C-013 — Was der Server sicher ablehnt, lehnt die CLI vorher ab

*Gilt für: Uploads, IDs, Pflicht-Flags*

Eine Datei über 50 MiB, ein Verzeichnis, eine ID, die keine UUID ist, ein
Aufruf ohne Kreis: Exit 1, ohne Anfrage.

**Warum:** Spart eine nutzlose Übertragung und gibt einen Fehler, der sagt, was
zu tun ist. **Umgesetzt als:** `internal/cli/uploads.go`, `requireArgs`.

<a id="c-014"></a>
## C-014 — Die SKILL.md ist der Vertrag für Agenten

*Gilt für: `skills/minka/SKILL.md`*

Sie liegt im Repository, wird mit jeder Änderung an Befehlen oder Verhalten
nachgezogen und beschreibt, was ein Agent wissen muss — nicht, was die CLI
intern tut.

**Warum:** Agenten sind die Hauptnutzer; ihre Beschreibung veraltet sonst als
Erstes. **Umgesetzt als:** `skills/minka/SKILL.md`.
