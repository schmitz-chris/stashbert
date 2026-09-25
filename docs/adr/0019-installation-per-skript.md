# ADR-0019: Installation per Proxmox-Skript und GitHub-Releases

- Status: angenommen
- Datum: 2026-09-25
- Ergänzt: ADR-0014 (Artefakt, Übertragung, Anlegen des Containers)

## Kontext

- Bisher legt der Nutzer den Container von Hand an, baut auf dem Mac mit `make release`, kopiert per `scp` und ruft `install.sh` auf (ADR-0014). Jedes Update sind mehrere Schritte.
- Der Nutzer möchte eine Einrichtung wie mit den Proxmox VE Helper-Scripts (community-scripts.org): ein Befehl auf dem Proxmox-Host.
- Die community-scripts nehmen neue Anwendungen erst ab 600 Sternen, 6 Monaten Alter, aktiver Pflege und offiziellen Releases auf; neue Skripte gehen zuerst nach ProxmoxVED. Ihr Gerüst `build.func` lädt das Installationsskript fest aus dem eigenen Repository. Nachnutzen ginge nur mit einem Fork.
- Nutzerentscheidung vom 25.09.2026: Das Repository ist öffentlich (MIT-Lizenz). Downloads aus GitHub-Releases brauchen damit kein Token mehr; ADR-0014 ging noch von einem privaten Repository aus.
- Debian 13 im unprivilegierten LXC braucht `nesting=1` (aktuelles systemd; ohne bleibt die Konsole leer). Die Weboberfläche von Proxmox setzt es beim Anlegen, `pct create` nicht.

## Entscheidung

- **Releases:** Ein Tag `v<major>.<minor>.<patch>` (SemVer, erstes Release `v0.1.0`) startet einen GitHub-Actions-Workflow. Er führt `make check` aus, baut mit `make release` und legt ein GitHub-Release mit automatisch erzeugten Notizen an. Tags mit Bindestrich (z. B. `v0.2.0-rc1`) werden Vorabversionen und zählen nicht als neuestes Release.
- **Assets** (nur amd64, ADR-0014):
  - `stashbert_<version>_linux_amd64.tar.gz`: Binary, `install.sh`, Unit, Env-Beispiel, `stashbert-update` und `stashbert-restore` in einem Ordner gleichen Namens;
  - `stashbert-update` einzeln, für die Erstinstallation;
  - `proxmox.sh`;
  - `SHA256SUMS` über alle Assets.
- **Kein Debian-Paket:** Das Binary ist statisch gelinkt und braucht keine weiteren Pakete; `install.sh` legt Benutzer, Unit und Konfiguration an. Ein `.deb` bräuchte ein neues Werkzeug und für `apt upgrade` ein eigenes, signiertes Paketarchiv.
- **Update im Container:** `stashbert-update` ermittelt das neueste Release über die Weiterleitung von `releases/latest`, lädt Archiv und `SHA256SUMS`, prüft die Prüfsumme und ruft `install.sh` auf. Ist die Version schon installiert, ändert es nichts. Wie bei den community-scripts ruft `/usr/bin/update` es auf; das legt das Proxmox-Skript an.
- **Restore im Container:** `stashbert-restore <archiv>` spielt eine Sicherung aus `GET /backup` ein. Der bisherige Stand wird verschoben, nicht gelöscht.
- **Proxmox-Skript:** ein eigenes `proxmox.sh` im Stil der community-scripts, aber ohne deren Gerüst. Aufruf auf dem Proxmox-Host:

  ```
  bash -c "$(curl -fsSL https://github.com/schmitz-chris/stashbert/releases/latest/download/proxmox.sh)"
  ```

  - Es legt einen unprivilegierten Debian-13-LXC an: `nesting=1`, 1 Kern, 512 MB RAM, 8 GB Platte, DHCP oder feste IP, Start mit dem Host.
  - Es installiert über `stashbert-update`, trägt auf Wunsch `OFF_CONTACT` und MQTT ein und spielt auf Wunsch eine Sicherung ein (Datei auf dem Host oder Adresse einer laufenden Instanz).
  - Die Konsole des Containers meldet root automatisch an, wie bei den community-scripts. Zugang dazu hat nur, wer Proxmox verwalten darf.
  - `--dry-run` zeigt die ändernden Befehle, ohne sie auszuführen.
- **Der Weg ohne Skript bleibt:** Archiv in einen Container kopieren, entpacken, `sh install.sh`.

## Konsequenzen

- Ein Update ist `update` in der Konsole des Containers oder `pct exec <id> -- update` auf dem Host.
- Jedes Release braucht einen Tag. Ohne Tag erzeugt `make release` wie bisher eine Version aus dem Commit-Hash.
- Die Git-Historie ist öffentlich, samt Autor-Adresse und älterer Commits.
- Das Proxmox-Skript lässt sich hier nur mit Attrappen für `pct` und Co. prüfen; auf einem echten Proxmox prüft es der Nutzer.
- Mit `nesting=1` würden die auskommentierten Schutzoptionen der Unit funktionieren. Sie zu aktivieren ist ein eigener Schritt.
- Später möglich: Aufnahme in die community-scripts, sobald die Kriterien erfüllt sind; arm64; ein Hinweis auf neue Versionen in den Einstellungen.

## Alternativen

- **Gerüst der community-scripts (Fork):** hängt an einem fremden, sich laufend ändernden Gerüst mit Telemetrie-Abfrage.
- **Privates Repository mit Token:** Token auf dem Host und im Container verwalten und erneuern.
- **Debian-Paket mit eigenem Paketarchiv:** mehr Aufwand als Nutzen für einen Haushalt.
- **Download auf dem Host und `pct push`:** eine zweite Download-Logik neben `stashbert-update`.
