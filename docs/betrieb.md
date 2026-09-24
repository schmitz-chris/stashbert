# StashBert: Betrieb im Proxmox-LXC

| | |
|---|---|
| Stand | 24.09.2026 |
| Grundlagen | [ADR-0014](adr/0014-betrieb-im-lxc.md), [ADR-0013](adr/0013-keine-anmeldung-in-m1.md), [architecture.md](architecture.md) Kapitel 8 und 9 |

Diese Anleitung beschreibt den Hauptweg: StashBert läuft als systemd-Dienst in einem unprivilegierten Debian-LXC auf Proxmox (amd64), gebaut wird auf dem Mac. Docker als Alternative kommt später mit R01 bis R03 dazu.

Befehle mit `ssh` und `scp` laufen auf dem Mac im Wurzelverzeichnis des Repositorys, alle anderen als `root` im Container, sofern nicht anders angegeben. `<container-ip>` steht für die Adresse des Containers.

## 1. Überblick

| Was | Wo |
|---|---|
| Programm | `/usr/local/bin/stashbert`, ein Binary mit API, Web-Oberfläche und Migrationen |
| Daten (`DATA_DIR`) | `/var/lib/stashbert`: `stashbert.db` (dazu `-wal` und `-shm`), `backups/`, `images/` |
| Konfiguration | `/etc/stashbert/stashbert.env` |
| Dienst | systemd-Unit `stashbert` in `/etc/systemd/system/stashbert.service`, läuft als Systembenutzer `stashbert` |
| Port | HTTP auf `PORT` (Standard 8080), auf allen Adressen des Containers |
| Logs | journald, eine JSON-Zeile pro Eintrag |

- StashBert spricht nur HTTP. HTTPS übernimmt der eigene Reverse Proxy (Abschnitt 5).
- M1 hat keine Anmeldung (ADR-0013). Wer die Adresse erreicht, kann alles lesen und buchen. StashBert darf daher nur im Heimnetz erreichbar sein, von unterwegs nur per VPN. Keine Portfreigabe am Router.

## 2. Container anlegen

Empfehlung für den neuen Container:

- Vorlage Debian 13, **unprivilegiert**
- 1 Kern, 512 MB RAM, 8 GB Platte
- feste IP oder eine DHCP-Reservierung im Router, damit Proxy und Lesezeichen gültig bleiben
- den SSH-Schlüssel des Macs beim Anlegen hinterlegen; dann funktionieren `ssh` und `scp` als `root` ohne Passwort
- Start beim Booten des Hosts einschalten

Nesting braucht StashBert nicht. Ist es eingeschaltet, lassen sich die auskommentierten Schutzoptionen aus `deploy/stashbert.service` als Drop-in mit `systemctl edit stashbert` aktivieren (danach prüfen, ob der Dienst läuft).

Für die Prüfung in Abschnitt 4 braucht der Container `curl`. Falls es fehlt:

```sh
apt update && apt install -y curl
```

## 3. Bauen und übertragen

Auf dem Mac (Go und Node.js wie für die Entwicklung):

```sh
make release
```

Das Ergebnis liegt in `bin/release/`:

- `stashbert-linux-amd64`: statisch gelinktes Binary für x86-64 mit eingebetteter Web-Oberfläche
- `SHA256SUMS`: Prüfsumme des Binarys

Die Version im Binary ist `git describe --tags --always --dirty`, ohne Tags also der kurze Commit-Hash. Für den Betrieb aus einem sauberen Stand bauen, sonst endet die Version auf `-dirty`.

Binary, Prüfsumme und die drei Dateien aus `deploy/` in ein Verzeichnis im Container kopieren. `install.sh` erwartet `stashbert.service` und `stashbert.env.example` neben sich.

```sh
ssh root@<container-ip> mkdir -p /root/stashbert
scp bin/release/stashbert-linux-amd64 bin/release/SHA256SUMS \
    deploy/install.sh deploy/stashbert.service deploy/stashbert.env.example \
    root@<container-ip>:/root/stashbert/
```

Im Container die Prüfsumme prüfen. Erwartet wird `stashbert-linux-amd64: OK`.

```sh
cd /root/stashbert
sha256sum -c SHA256SUMS
```

## 4. Installieren

```sh
cd /root/stashbert
sh install.sh ./stashbert-linux-amd64
```

Das Skript:

1. prüft, dass es als root läuft, dass `systemctl` vorhanden ist und dass Unit und Beispiel neben ihm liegen,
2. prüft das Binary mit `-version`; ein falsches Binary (etwa `bin/stashbert` vom Mac) bricht hier ab,
3. legt den Systembenutzer `stashbert` an, falls er fehlt,
4. installiert das Binary nach `/usr/local/bin/stashbert` (Modus 0755) und die Unit nach `/etc/systemd/system/stashbert.service`,
5. legt `/etc/stashbert/stashbert.env` aus `stashbert.env.example` an, **nur wenn die Datei fehlt** (Modus 0640, Besitzer `root`, Gruppe `stashbert`),
6. aktiviert und startet den Dienst beim ersten Mal, sonst startet es ihn neu,
7. zeigt die installierte Version und den Status des Dienstes.

systemd legt `/var/lib/stashbert` beim ersten Start an. StashBert erstellt dort die Datenbank und schreibt gleich das erste Backup.

### Konfiguration anpassen

Die Env-Datei mit einem Editor öffnen, zum Beispiel `nano /etc/stashbert/stashbert.env`:

| Variable | Standard | Bedeutung |
|---|---|---|
| `PORT` | `8080` | HTTP-Port (1 bis 65535) |
| `OFF_CONTACT` | leer | Kontakt für den User-Agent bei Open Food Facts, z. B. `du@example.org`. Leer bedeutet: keine Abfragen, unbekannte Produkte bekommen nur einen Platzhalter |
| `BACKUP_KEEP` | `14` | Anzahl aufbewahrter täglicher Backups (1 bis 365) |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` oder `error` |

Mindestens `OFF_CONTACT` mit einer eigenen Kontaktadresse setzen:

```
OFF_CONTACT=du@example.org
```

`DATA_DIR` setzt die Unit auf `/var/lib/stashbert`; die Variable gehört nicht in die Env-Datei. Nach jeder Änderung:

```sh
systemctl restart stashbert
```

### Prüfen

```sh
systemctl status stashbert
curl http://127.0.0.1:8080/api/v1/health
```

Die Antwort ist `{"status":"ok","version":"<version>"}`. Bei einem anderen `PORT` die Zahl anpassen. Vom Mac aus geht dasselbe mit `curl http://<container-ip>:8080/api/v1/health`.

## 5. Reverse Proxy

StashBert hat kein TLS. Der vorgelagerte Proxy muss (architecture.md 9.1):

- **HTTPS** mit einem Zertifikat anbieten, dem das iPhone vertraut. Ohne sichere Verbindung gibt iOS die Kamera nicht frei, und der Service Worker der App startet nicht.
- **alle Pfade** an `http://<container-ip>:8080` weiterleiten, ohne Umschreibung auf einen Unterpfad. StashBert liegt also an der Wurzel einer eigenen Adresse, z. B. `https://stashbert.example.org/`, nicht unter `https://example.org/stashbert/`.
- die Header **`X-Forwarded-For`** und **`X-Forwarded-Proto`** setzen.

Eine konkrete Proxy-Konfiguration gehört nicht zu dieser Anleitung. Auch der Proxy ist nur im Heimnetz bzw. per VPN erreichbar (Abschnitt 1).

## 6. iPhone einrichten

1. In Safari die HTTPS-Adresse des Proxys öffnen, nicht `http://<container-ip>:8080`.
2. Die Ansicht „Scan" öffnen und die Kamera-Abfrage erlauben.
3. Die Kamera dauerhaft erlauben: in Safari die Website-Einstellungen dieser Seite öffnen (über das Menü in der Adressleiste) und für die Kamera „Erlauben" wählen. Alternativ gilt die Einstellung für alle Websites in der App Einstellungen unter Safari, Kamera. Die genauen Menünamen unterscheiden sich je nach iOS-Version.
4. Über „Teilen" die Option „Zum Home-Bildschirm" wählen. StashBert startet dann wie eine App ohne Safari-Leiste.

Als Home-Bildschirm-App fragt iOS je nach Version trotzdem gelegentlich erneut nach der Kamera (research.md 7.3). Das liegt an iOS, nicht an StashBert.

## 7. Update

Optional vorher auf dem Proxmox-Host einen Snapshot anlegen (`<ctid>` ist die Nummer des Containers):

```sh
pct snapshot <ctid> vor_update
```

Auf dem Mac bauen und kopieren, wie in Abschnitt 3:

```sh
make release
scp bin/release/stashbert-linux-amd64 bin/release/SHA256SUMS \
    deploy/install.sh deploy/stashbert.service deploy/stashbert.env.example \
    root@<container-ip>:/root/stashbert/
```

Im Container:

```sh
cd /root/stashbert
sha256sum -c SHA256SUMS
sh install.sh ./stashbert-linux-amd64
```

- Binary und Unit werden ersetzt, der Dienst startet neu.
- `/etc/stashbert/stashbert.env` bleibt unverändert. Neue Variablen aus dem Beispiel kommen nicht automatisch hinein; vergleichen mit `diff /etc/stashbert/stashbert.env /root/stashbert/stashbert.env.example`.
- Direkte Änderungen an `/etc/systemd/system/stashbert.service` überschreibt jedes Update. Eigene Anpassungen daher als Drop-in mit `systemctl edit stashbert`.
- Stehen Migrationen an, schreibt StashBert vorher automatisch `backups/pre-migration-<YYYYMMDD-HHMMSS>.db`.
- Die App auf dem iPhone zeigt beim nächsten Start „Neue Version verfügbar, tippen zum Aktualisieren". Erst der Tipp lädt die neue Oberfläche.

## 8. Logs

```sh
journalctl -u stashbert                     # alle Einträge
journalctl -u stashbert -f                  # live mitlesen
journalctl -u stashbert -n 50 --no-pager    # die letzten 50 Zeilen
```

Mehr Details liefert `LOG_LEVEL=debug` in der Env-Datei, danach `systemctl restart stashbert`. Für den Dauerbetrieb reicht `info`.

## 9. Backup

StashBert sichert die Datenbank selbst per `VACUUM INTO` nach `/var/lib/stashbert/backups` (architecture.md 9.3):

| Datei | Wann | Aufbewahrung |
|---|---|---|
| `stashbert-<YYYYMMDD-HHMMSS>.db` | beim Start des Dienstes und danach alle 24 Stunden | die neuesten `BACKUP_KEEP` (Standard 14), ältere löscht StashBert |
| `pre-migration-<YYYYMMDD-HHMMSS>.db` | vor Migrationen einer vorhandenen Datenbank | bleibt, bis man sie selbst löscht |

- Die Zeitstempel sind in UTC.
- Jeder Start schreibt ein Backup, auch der durch `install.sh` oder `systemctl restart`. Viele Neustarts verkürzen also den Zeitraum, den die aufbewahrten Dateien abdecken.
- Die Backups enthalten nur die Datenbank, nicht die Produktbilder in `/var/lib/stashbert/images`.

```sh
ls -l /var/lib/stashbert/backups
```

Zusätzlich:

- **Proxmox:** Snapshots (etwa vor Updates) und regelmäßige vzdump-Sicherungen sichern den ganzen Container samt Bildern, am besten auf einen anderen Speicher. Snapshots setzen einen Speicher voraus, der sie unterstützt, etwa LVM-Thin oder ZFS.
- **Außerhalb des Containers:** Backups und Bilder regelmäßig woanders ablegen, z. B. auf dem Mac. Nicht die laufende `stashbert.db` kopieren, sondern die Dateien aus `backups/`.

```sh
mkdir -p ~/stashbert-sicherung
scp -r root@<container-ip>:/var/lib/stashbert/backups \
    root@<container-ip>:/var/lib/stashbert/images ~/stashbert-sicherung/
```

## 10. Restore

Nach architecture.md 9.3, im Container:

```sh
ls -l /var/lib/stashbert/backups
systemctl stop stashbert
cd /var/lib/stashbert
rm -f stashbert.db stashbert.db-wal stashbert.db-shm
cp backups/stashbert-<YYYYMMDD-HHMMSS>.db stashbert.db
chown stashbert:stashbert stashbert.db
systemctl start stashbert
```

- Wer den aktuellen Stand behalten will, verschiebt die drei Dateien an einen anderen Ort, statt sie zu löschen.
- Ein `pre-migration-…`-Backup wird genauso zurückgespielt.
- Stammt das Backup von einer älteren Version, migriert StashBert es beim Start und schreibt vorher ein `pre-migration-…`-Backup.
- Danach wie in Abschnitt 4 prüfen.

## 11. Deinstallation

```sh
systemctl disable --now stashbert
rm /etc/systemd/system/stashbert.service
rm -rf /etc/systemd/system/stashbert.service.d   # nur vorhanden nach systemctl edit
systemctl daemon-reload
rm /usr/local/bin/stashbert
rm -r /etc/stashbert
rm -rf /root/stashbert                           # hochgeladene Dateien
userdel stashbert
```

Die Daten bleiben dabei in `/var/lib/stashbert`; eine spätere Installation übernimmt sie wieder. Erst wenn sie wirklich weg sollen (Datenbank, Bilder und alle Backups, unwiderruflich):

```sh
rm -r /var/lib/stashbert
```

## 12. Fehlersuche

| Problem | Ursache und Abhilfe |
|---|---|
| Dienst startet nicht oder startet ständig neu | `systemctl status stashbert` und `journalctl -u stashbert -n 50 --no-pager`. Beim Abbruch steht dort eine Zeile mit `"msg":"stashbert stopped"` und dem Grund, z. B. `load config: …` bei einem ungültigen Wert in der Env-Datei. Wert korrigieren, `systemctl restart stashbert`. Bis dahin versucht systemd alle 5 Sekunden einen neuen Start. |
| Port belegt | Im Log steht `listen: … address already in use`. `ss -ltnp` zeigt, wer den Port hat. Einen anderen `PORT` in der Env-Datei setzen, Dienst neu starten und das Ziel im Proxy anpassen. Achtung: In diesem Fall schreibt jeder neue Startversuch vorher ein Backup und verdrängt ältere. Den Dienst bis zur Lösung mit `systemctl stop stashbert` anhalten. |
| `install.sh` bricht ab | Die Meldung nennt den Grund. Häufig: das falsche Binary (es muss `stashbert-linux-amd64` aus `make release` sein) oder `stashbert.service` bzw. `stashbert.env.example` fehlen neben dem Skript. |
| Kamera geht nicht | StashBert über die HTTPS-Adresse des Proxys mit gültigem Zertifikat öffnen, nicht über `http://<container-ip>:8080`. Die Kamera-Freigabe in Safari prüfen (Abschnitt 6). |
| Keine Produktnamen, nur `Neues Produkt <code>` | `OFF_CONTACT` ist leer (`grep OFF_CONTACT /etc/stashbert/stashbert.env`). Setzen und neu starten. Platzhalter, die auf das Nachladen warten, ergänzt StashBert danach im Hintergrund, meist ein Produkt pro Minute. Die Angaben übernimmt es nur, solange weder Name noch Marke noch Packungsgröße von Hand geändert wurden. Der Container braucht dafür Zugang ins Internet. Ist ein Produkt bei Open Food Facts nicht erfasst, bleibt es beim Platzhalter. |
