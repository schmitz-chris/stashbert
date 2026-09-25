# ADR-0020: Backup über die Oberfläche einspielen

- Status: angenommen
- Datum: 2026-09-25
- Ergänzt: ADR-0005 (Datenhaltung), architecture.md 9.3

## Kontext

- Die Einstellungen bieten „Backup herunterladen" (B40, F32): ein tar.gz mit `stashbert.db` und `images/`. Einspielen ging bisher nur im Container mit `stashbert-restore` (L04) oder von Hand.
- Der Nutzer möchte ein Backup auch in der Oberfläche einspielen, etwa beim Umzug auf einen neuen Container oder nach einem Fehler. Nutzerentscheidung vom 25.09.2026: Die Oberfläche sagt „Backup", nicht „Sicherung".
- Die Datenbank ist während des Betriebs geöffnet: ein Verbindungspool, Hintergrundjobs, bei MQTT auch die Outbox. Ein Austausch der Datei im laufenden Betrieb ist nicht sicher.
- Der Request-Validator liest jeden Body vollständig in den Speicher (architecture.md 4.4). Ein Backup kann mit vielen Bildern mehrere hundert MB groß sein; der Container hat 512 MB RAM.
- M1 hat keine Anmeldung (ADR-0013). Wer StashBert erreicht, kann heute schon alles löschen.

## Entscheidung

- **Endpunkt:** `POST /backup/restore` nimmt ein Archiv aus `GET /backup` als `application/gzip` entgegen, höchstens 1 GB. Die Route umgeht den Validator; das Archiv wird beim Lesen entpackt, nie ganz in den Speicher geladen.
- **Erst prüfen, dann tauschen:** Das Archiv wird nach `DATA_DIR/restore/` entpackt und geprüft, bevor sich irgendetwas ändert:
  - Es darf nur `stashbert.db` und Bilder unter `images/` enthalten, keine anderen Pfade und keine Links.
  - Die Datenbank muss `PRAGMA integrity_check` bestehen und darf nicht von einer neueren Version stammen, also keine Migration enthalten, die dieses Binary nicht kennt.
  - Ist alles in Ordnung, liegt das Backup als `DATA_DIR/restore/pending/` bereit.
- **Neustart im Prozess:** Nach der Antwort 202 fährt StashBert herunter wie bei SIGTERM (HTTP-Server, Hintergrundjobs, MQTT, Datenbank) und startet im selben Prozess neu. Beim Start, vor dem Öffnen der Datenbank, spielt es ein bereitliegendes Backup ein. Das funktioniert ohne systemd, auch in Docker und bei `make run`.
- **Nichts geht verloren:** Der bisherige Stand (`stashbert.db`, `-wal`, `-shm`, `images/`) wandert nach `DATA_DIR/vor-restore-<YYYYMMDD-HHMMSS>/`, wie bei `stashbert-restore`. Ein älteres Backup hebt StashBert danach wie gewohnt per Migration auf den aktuellen Stand, mit `pre-migration-…`-Backup.
- **Oberfläche:** In den Einstellungen im Abschnitt „Backup" gibt es „Backup einspielen" mit Dateiauswahl und Rückfrage. Danach wartet die App, bis StashBert wieder antwortet, und lädt alle Daten neu.
- **`stashbert-restore`** bleibt für den Fall, dass StashBert nicht startet.

## Konsequenzen

- Neue Fehlercodes `invalid_backup` (422), `backup_too_new` (422), `backup_too_large` (413) und `restore_in_progress` (409).
- Während des Neustarts ist StashBert etwa eine bis zwei Sekunden nicht erreichbar. MQTT meldet dabei kurz `offline`.
- Die Ordner `vor-restore-…` löscht StashBert nicht; sie wachsen mit jedem Einspielen (docs/betrieb.md).
- Ohne Anmeldung kann jeder im Heimnetz ein Backup einspielen. Durch den aufbewahrten alten Stand geht dabei nichts verloren.

## Alternativen

- **Datenbank im laufenden Betrieb tauschen:** Jeder Teil, der die Datenbank hält, müsste sie loslassen und neu öffnen; fehleranfällig.
- **Beenden und von systemd neu starten lassen:** hängt vom Dienstverwalter ab; bei `make run` bliebe StashBert einfach stehen.
- **Archiv erst ganz hochladen, dann entpacken:** braucht doppelten Platz, ohne Vorteil.
- **Nur im Container per Befehl:** für den Nutzer umständlich, er wünscht es ausdrücklich in der Oberfläche.
