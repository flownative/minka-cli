# Live-Prüfung

Eine neue API-Meldung ist erst geprüft, wenn sie gegen eine laufende Instanz
lief. Bis dahin steht im Status, dass sie nur gegen den Fake geprüft ist.

## Wo geschrieben werden darf

| Instanz | Konto | Schreiben erlaubt |
|---|---|---|
| `dev.minka.flownative.com` | Agentenkonto „Claude", handelt für Robert | überall im Projekt „Flownative HQ", und privat an Shaun |
| `minka.localbeach.net` | Smoke-Konto Bruno | nur Board „API-3 Abnahme" und Chat „Chat1 Abnahme" im „Test-Projekt" |

Der Chat „Sommerfest" auf der lokalen Instanz ist Roberts Demo und bleibt
unberührt. Gelöscht wird nur, was der Lauf selbst angelegt hat.

Vorgabe-Host ist die Test-Instanz; die lokale braucht
`--host https://minka.localbeach.net`.

Alles darüber hinaus — einer weiteren Person schreiben, in einem fremden
Projekt etwas anlegen — ist eine Entscheidung von Robert oder Shaun, nicht des
Koordinators.

## Ablauf

1. `make build`, damit `bin/minka` den geprüften Stand hat.
2. Eigene Daten anlegen: eine Karte, eine Nachricht, ein Kommentar. Nie an
   fremden weiterarbeiten.
3. Den gemeldeten Fall durchspielen, auch die Absagen: zweiter Aufruf, fremde
   ID, zu langer Wert, fehlende Berechtigung.
4. Die Textansicht ansehen, nicht nur `--quiet`. Ein Fehler in der Darstellung
   fällt nur dort auf.
5. Aufräumen: eigene Karten löschen, eigene Nachrichten entfernen. Ein Upload,
   auf den kein gespeicherter Text zeigt, verschwindet nach sieben Tagen von
   selbst.
6. Was der Lauf gezeigt hat, in den Status; was er an der CLI aufgedeckt hat,
   sofort beheben.

## Bilder

Ein Testbild soll sichtbar sein. Ein 1×1-Pixel kommt zwar korrekt an, aber
niemand sieht daran, ob die Anzeige stimmt.
