# Ausgabe, Fehler und Exit-Codes

Was die CLI ausgibt, ist ein Vertrag: Felder dürfen dazukommen, nie
verschwinden oder umbenannt werden. Die Konventionen stammen von
[basecamp-cli](https://github.com/basecamp/basecamp-cli), damit ein Agent, der
eine der beiden kennt, die andere lesen kann (C-001).

## Drei Ausgabemodi

Auf einem Terminal Text, sonst JSON.

| Modus | Ausgabe |
|---|---|
| `--json` | Hülle `{ok, data, summary, notice, breadcrumbs}` |
| `--quiet` | nur `data` |
| `--styled` | Text, auch ohne Terminal |

`data` ist die Ressource der API, roh durchgereicht — nicht neu
zusammengesetzt. Ein Feld, das die API dazubekommt, erreicht so jeden Client,
ohne dass die CLI es kennt (C-002).

Warnungen stehen bei `--json` in `notice`, sonst auf stderr, damit
`--quiet | jq` nie etwas Fremdes im Strom hat.

## Fehler

`{ok: false, error, code, retryable, hint, meta: {apiCode, httpStatus}}`.
`meta.apiCode` ist der Fallcode von Minka (`cardTitleTooLong`), `code` die
Klasse, `hint` der nächste Schritt.

| Exit | `code` | Bedeutung |
|---|---|---|
| 0 | — | ok |
| 1 | `usage`, `config` | Aufruf oder Config-Datei |
| 2 | `not_found` | nicht gefunden, oder nicht sichtbar |
| 3 | `auth_required` | nicht angemeldet |
| 4 | `forbidden` | verboten |
| 6 | `network` | keine vollständige Antwort |
| 7 | `api_error` | Server- oder Protokollfehler |
| 8 | `ambiguous` | mehrdeutige Angabe |
| 9 | `validation` | von Minka abgelehnt |

`retryable` ist nur bei lesenden Anfragen wahr, und bei `messages create` mit
`--client-message-id`, weil ein zweiter Versuch dann nichts Neues schreibt. Ein
fehlgeschlagener Schreibvorgang kann angekommen sein; dort rät der `hint` zum
Nachlesen (C-012).

## Befehle

Plural-Gruppe plus Aktion (`cards move`, `comments react`). Ein Befehl, der
eine Ressource holt, gibt `breadcrumbs` mit: die nächsten sinnvollen Aufrufe
mit fertiger Befehlszeile.

Freitext kommt nie über Shell-Quoting allein: Beschreibung, Kommentar und
Nachricht nehmen `-` für stdin. Titel sind einzeilig und stehen in Flags
(C-007).

Nennt eine Adresse zwei verschiedene Dinge, benennt ein Flag den Kreis, statt
zwei ID-Arten auf dieselbe Position zu legen: `uploads create <datei>
--project <id> | --conversation <id>` (C-010).

Was die API unter derselben ID anbietet, bekommt keine zweite Befehlsreihe: die
ID eines privaten Gesprächs geht an `messages …` und `chats mark-read` (C-009).
