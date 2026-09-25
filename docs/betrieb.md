# StashBert: Betrieb im Proxmox-LXC

| | |
|---|---|
| Stand | 24.09.2026 |
| Grundlagen | [ADR-0014](adr/0014-betrieb-im-lxc.md), [ADR-0013](adr/0013-keine-anmeldung-in-m1.md), [architecture.md](architecture.md) Kapitel 8 und 9 |

Diese Anleitung beschreibt den Hauptweg: StashBert läuft als systemd-Dienst in einem unprivilegierten Debian-LXC auf Proxmox (amd64), gebaut wird auf dem Mac. Docker als Alternative beschreibt Abschnitt 13.

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
| `MQTT_URL`, `MQTT_USERNAME`, `MQTT_PASSWORD` und weitere `MQTT_*` | leer bzw. Standardwerte | Anbindung an Home Assistant über MQTT; leer bedeutet ohne MQTT. Einrichtung und alle Variablen in `docs/home-assistant.md` |

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

### Sicherung herunterladen und wiederherstellen

`GET /api/v1/backup` liefert eine frische Sicherung als Archiv `stashbert-<YYYYMMDD-HHMMSS>.tar.gz` (Zeit in UTC). Es enthält `stashbert.db`, eine Kopie per `VACUUM INTO`, und den Ordner `images/` mit allen Produktbildern. Ohne Anmeldung (ADR-0013) kann das jeder, der StashBert erreicht.

Herunterladen, z. B. auf dem Mac (oder die Adresse im Browser öffnen), und in den Container kopieren:

```sh
curl -fOJ http://<container-ip>:8080/api/v1/backup
scp stashbert-<YYYYMMDD-HHMMSS>.tar.gz root@<container-ip>:/root/
```

Im Container entpacken, den Dienst stoppen, Datenbank und Bilder ablegen, die alten `-wal`- und `-shm`-Dateien entfernen, den Besitzer setzen und den Dienst starten:

```sh
mkdir /root/wiederherstellen
tar -xzf /root/stashbert-<YYYYMMDD-HHMMSS>.tar.gz -C /root/wiederherstellen
systemctl stop stashbert
cd /var/lib/stashbert
rm -f stashbert.db stashbert.db-wal stashbert.db-shm
rm -rf images
cp /root/wiederherstellen/stashbert.db stashbert.db
cp -R /root/wiederherstellen/images images
chown -R stashbert:stashbert stashbert.db images
systemctl start stashbert
```

- Wer den aktuellen Stand behalten will, verschiebt Datenbank, `-wal`, `-shm` und `images/` an einen anderen Ort, statt sie zu löschen.
- Danach wie in Abschnitt 4 prüfen und `/root/wiederherstellen` löschen.

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
| Port belegt | Im Log steht `listen: … address already in use`. `ss -ltnp` zeigt, wer den Port hat. Einen anderen `PORT` in der Env-Datei setzen, Dienst neu starten und das Ziel im Proxy anpassen. StashBert schreibt erst nach dem Öffnen des Ports ein Backup, und systemd gibt nach 5 Fehlstarts in 5 Minuten auf; danach mit `systemctl reset-failed stashbert` und `systemctl start stashbert` neu anstoßen. |
| `install.sh` bricht ab | Die Meldung nennt den Grund. Häufig: das falsche Binary (es muss `stashbert-linux-amd64` aus `make release` sein) oder `stashbert.service` bzw. `stashbert.env.example` fehlen neben dem Skript. |
| Kamera geht nicht | StashBert über die HTTPS-Adresse des Proxys mit gültigem Zertifikat öffnen, nicht über `http://<container-ip>:8080`. Die Kamera-Freigabe in Safari prüfen (Abschnitt 6). |
| Keine Produktnamen, nur `Neues Produkt <code>` | `OFF_CONTACT` ist leer (`grep OFF_CONTACT /etc/stashbert/stashbert.env`). Setzen und neu starten. Platzhalter, die auf das Nachladen warten, ergänzt StashBert danach im Hintergrund, meist ein Produkt pro Minute. Die Angaben übernimmt es nur, solange weder Name noch Marke noch Packungsgröße von Hand geändert wurden. Der Container braucht dafür Zugang ins Internet. Ist ein Produkt bei Open Food Facts nicht erfasst, bleibt es beim Platzhalter. |

## 13. Alternative: Docker

Statt im LXC kann StashBert auch als Container mit Docker Compose laufen (ADR-0010, ADR-0014). Der Hauptweg bleibt der LXC; dieser Abschnitt nennt nur, was unter Docker anders ist. Reverse Proxy (Abschnitt 5) und iPhone (Abschnitt 6) gelten unverändert, mit `http://<docker-host>:8080` als Ziel des Proxys.

### Voraussetzungen

- Ein Linux-Host (amd64 oder arm64) mit Docker Engine und dem Compose-Plugin (`docker compose`). Docker in einem LXC braucht Nesting; Proxmox empfiehlt dafür eher eine VM (ADR-0014).
- Die Befehle laufen auf dem Docker-Host, die mit `docker compose` im Verzeichnis `~/stashbert`. Nur `scp` läuft auf dem Mac im Wurzelverzeichnis des Repositorys.
- Befehle mit `sudo` brauchen root, weil die Dateien in `data/` dem Benutzer 65532 des Containers gehören. Als root entfällt `sudo`.

| Was | Wo |
|---|---|
| Dienst | `compose.yaml` aus `deploy/compose.yaml`, ein Dienst `stashbert` |
| Konfiguration | `.env` aus `deploy/.env.example` |
| Daten | `data/`, im Container `/data`: `stashbert.db` (dazu `-wal` und `-shm`), `backups/`, `images/` |
| Port | 8080 auf allen Adressen des Hosts |
| Logs | `docker compose logs`, eine JSON-Zeile pro Eintrag |

Docker veröffentlicht den Port an Firewall-Regeln wie ufw vorbei. Es gilt dasselbe wie in Abschnitt 1: nur im Heimnetz erreichbar, keine Portfreigabe am Router.

### Verzeichnis anlegen

Auf dem Docker-Host:

```sh
mkdir -p ~/stashbert/data
```

Auf dem Mac die beiden Dateien kopieren; `.env.example` wird dabei zu `.env`. Läuft Docker auf dem Mac selbst, genügt `cp` statt `scp`.

```sh
scp deploy/compose.yaml <benutzer>@<docker-host>:stashbert/compose.yaml
scp deploy/.env.example <benutzer>@<docker-host>:stashbert/.env
```

Auf dem Docker-Host das Datenverzeichnis dem Benutzer des Containers geben:

```sh
cd ~/stashbert
sudo chown 65532:65532 data
```

Das Image läuft als Benutzer 65532. Gehört `data/` einem anderen Benutzer (auch wenn es beim ersten Start fehlt, dann legt Docker es für root an), beendet sich StashBert mit `unable to open database file`, und Docker startet es immer wieder neu. Unter Docker Desktop auf dem Mac entfällt `chown`: Dort darf der Container in eingebundenen Verzeichnissen schreiben, und die Dateien gehören auf dem Mac dem eigenen Benutzer.

### Image

**Aus GHCR:** Die Images liegen unter `ghcr.io/schmitz-chris/stashbert`, im privaten Repository. Die CI baut sie für amd64 und arm64 bei jedem Versions-Tag (R03); der Image-Tag ist die Version ohne das führende `v`, dazu `latest`. Einmal auf dem Docker-Host anmelden, mit dem GitHub-Benutzernamen und als Passwort einem Personal Access Token (classic) mit dem Recht `read:packages`. Fein granulierte Tokens nimmt GHCR nicht an.

```sh
docker login ghcr.io -u <github-benutzer>
```

**Lokal gebaut:** Solange es in GHCR kein Image gibt, oder ohne Zugang dazu, baut man es selbst, und zwar auf dem Docker-Host in einem Klon des Repositorys. `make docker` baut für die Architektur des Rechners, auf dem es läuft; ein Image vom Mac (arm64) läuft nicht auf einem amd64-Host.

```sh
make docker
docker tag stashbert:dev ghcr.io/schmitz-chris/stashbert:dev
```

In `.env` dann `STASHBERT_VERSION=dev` setzen. Compose nimmt das lokale Image; `docker compose pull` entfällt bei diesem Weg.

### Konfigurieren und starten

`.env` mit einem Editor öffnen, z. B. `nano .env`. Die Variablen sind dieselben wie in Abschnitt 4 (`OFF_CONTACT`, `BACKUP_KEEP`, `LOG_LEVEL`), dazu `STASHBERT_VERSION` für den Image-Tag, Standard `latest`. Mindestens `OFF_CONTACT` setzen. `DATA_DIR` und `PORT` gehören nicht in `.env`: Die Daten liegen im Container fest unter `/data`, und StashBert lauscht dort auf 8080.

```sh
cd ~/stashbert
docker compose up -d
docker compose ps
curl http://127.0.0.1:8080/api/v1/health
```

- Die Antwort ist `{"status":"ok","version":"<version>"}`.
- `docker compose ps` zeigt nach wenigen Sekunden `(healthy)` in der Spalte `STATUS`. Docker ruft dafür alle 30 Sekunden `/stashbert -healthcheck` im Container auf; nach drei Fehlschlägen in Folge steht dort `(unhealthy)`. Einen ungesunden Container startet Docker nicht neu.
- `restart: unless-stopped` startet StashBert neu, wenn es sich beendet (etwa bei einem ungültigen Wert in `.env`), und nach einem Neustart des Hosts, außer nach `docker compose stop`.
- Ist Port 8080 auf dem Host belegt, in `compose.yaml` unter `ports` die linke Zahl ändern, z. B. `"9000:8080"`, und das Ziel im Proxy anpassen.
- Nach jeder Änderung an `.env` oder `compose.yaml` wieder `docker compose up -d`; Compose ersetzt dann den Container. `docker compose restart` übernimmt Änderungen an `.env` nicht.
- `docker compose down` hält StashBert an und entfernt den Container; die Daten in `data/` bleiben.

### Update

Die neue Version in `.env` bei `STASHBERT_VERSION` eintragen, dann:

```sh
cd ~/stashbert
docker compose pull
docker compose up -d
```

- Compose ersetzt den Container, die Daten in `data/` bleiben.
- Bei `STASHBERT_VERSION=latest` entfällt die Änderung in `.env`; `docker compose pull` holt das neueste Image.
- Bei einem lokal gebauten Image statt `docker compose pull` im Repository wieder `make docker` und `docker tag stashbert:dev ghcr.io/schmitz-chris/stashbert:dev`, dann `docker compose up -d`. Compose bemerkt das neue Image und ersetzt den Container.
- Stehen Migrationen an, schreibt StashBert vorher automatisch `backups/pre-migration-<YYYYMMDD-HHMMSS>.db`.

### Logs

```sh
docker compose logs                 # alle Einträge
docker compose logs -f              # live mitlesen
docker compose logs --tail 50       # die letzten 50 Zeilen
```

Die Logs gehören zum Container. Nach einem Update, einer Änderung an `.env` oder `docker compose down` beginnen sie neu.

### Backup

Wie in Abschnitt 9, nur liegen die Dateien auf dem Host in `~/stashbert/data/backups`. Jeder Start des Containers schreibt ein Backup, auch `docker compose up -d` nach einer Änderung.

```sh
cd ~/stashbert
sudo ls -l data/backups
```

Außerhalb des Hosts regelmäßig `data/backups` und `data/images` sichern, nicht die laufende `data/stashbert.db`.

### Restore

Nach architecture.md 9.3, mit dem Container statt systemd:

```sh
cd ~/stashbert
sudo ls -l data/backups
docker compose stop
sudo rm -f data/stashbert.db data/stashbert.db-wal data/stashbert.db-shm
sudo cp data/backups/stashbert-<YYYYMMDD-HHMMSS>.db data/stashbert.db
sudo chown 65532:65532 data/stashbert.db
docker compose start
docker compose ps
```

- Unter Docker Desktop auf dem Mac gehen dieselben Befehle ohne `sudo`, und `chown` entfällt.
- Die Hinweise aus Abschnitt 10 gelten genauso.
