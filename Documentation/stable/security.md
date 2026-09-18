# Token, Hosts und Anfragen

Ein Token ist ein Schlüssel zu einem Konto. Die CLI behandelt ihn so: Er steht
nur in der Konfigurationsdatei, geht nur an seinen Host, und er darf nirgends
sonst auftauchen — nicht im Repository, nicht in einem Commit, nicht in einem
Test, nicht in einer Logzeile.

## Anmeldung

- Das Token kommt nie über ein Flag, sondern verdeckt vom Terminal oder mit
  `--with-token` von stdin. Ein Flag stünde in der History und in `ps`.
- Für `--with-token` darf stdin kein Terminal sein; das Token ist einzeilig und
  auf 4096 Bytes begrenzt.
- Vor dem Speichern prüft die CLI es gegen `/me`, damit kein unbrauchbares
  Token in der Datei landet.

## Konfigurationsdatei

`$MINKA_CONFIG`, sonst `$XDG_CONFIG_HOME/minka/config.json`, sonst
`~/.config/minka/config.json`.

- Datei 0600 in einem Verzeichnis 0700, atomar geschrieben mit `fsync` und
  `rename`; ein Symlink bleibt ein Symlink.
- Eine Datei, die andere lesen können, wird abgelehnt statt stillschweigend
  benutzt.
- Sie hält mehrere Hosts mit je einem Token und einen Vorgabe-Host.

## Ein Token geht nur an seinen Host

`MINKA_TOKEN` gilt allein zusammen mit `MINKA_HOST` und nur für diesen Host.
Wer mit `--host` einen anderen Host anspricht, bekommt dessen Token aus der
Konfigurationsdatei oder eine Absage — nie das Token des ersten Hosts (C-004).

## Host-Namen

`https://` wird ergänzt, `/api/v1` und ein Standardport entfernt. `http://`
gilt nur für Loopback. Andere Pfade werden abgelehnt.

## Anfragen

- Redirects werden nie verfolgt: eine Umleitung trüge das Token an eine
  Adresse, die niemand genannt hat.
- Jede ID wird lokal als UUID geprüft — eine, die keine ist, endet mit Exit 1,
  ohne dass eine Anfrage hinausgeht — und im Pfad zusätzlich escaped. Was in
  der Query reitet (Dateiname, Inhalt einer Reaktion), wird ebenso escaped.
- Timeout 30 Sekunden, für den Rumpf eines Uploads 10 Minuten. Eine Antwort
  liest die CLI höchstens 16 MB weit.
- Was der Server sicher ablehnt, lehnt die CLI vorher ab: eine Datei über 50
  MiB, ein Verzeichnis, ein Aufruf ohne Kreis. Das spart eine nutzlose
  Übertragung und gibt einen klareren Fehler (C-013).
