# ADR-0014: Betrieb im Proxmox-LXC mit systemd

- Status: angenommen
- Datum: 2026-09-24
- Ergänzt: ADR-0010 (das Container-Image wird optional)

## Kontext

- Der Nutzer betreibt Proxmox mit LXC-Containern und möchte StashBert dort direkt laufen lassen. Den Container legt er selbst an.
- StashBert ist bereits ein statisch gelinktes Go-Binary ohne cgo, mit eingebetteter Web-Oberfläche, Migrationen und Spec (ADR-0010).
- Docker in einem LXC braucht Nesting und hat bekannte Stolpersteine; Proxmox empfiehlt Docker eher in einer VM.
- Der Proxmox-Host ist amd64. Entwickelt wird auf einem Mac mit arm64.
- Das Repository ist privat; ein Download aus GitHub-Releases bräuchte im Container ein Token.

## Entscheidung

- **Hauptweg:** StashBert läuft als systemd-Dienst in einem unprivilegierten Debian-LXC.
  - Binary unter `/usr/local/bin/stashbert`, Daten unter `/var/lib/stashbert` (`DATA_DIR`), Konfiguration in `/etc/stashbert/stashbert.env`.
  - Eigener Systembenutzer `stashbert`.
  - Schutzoptionen der Unit nur so weit, wie sie in unprivilegierten LXC ohne besondere Einstellungen funktionieren; weitergehende Optionen stehen auskommentiert in der Unit.
- **Artefakt:** `make release` baut auf dem Mac per Cross-Compile `stashbert-linux-amd64` samt Web-Oberfläche und Prüfsumme. arm64 kommt erst, wenn es gebraucht wird.
- **Übertragung:** vorerst per `scp` in den Container; `deploy/install.sh` installiert oder aktualisiert dort.
- **Docker:** Image und Compose (R01 bis R03) bleiben als portable Option im Plan, sind aber zurückgestellt.

## Konsequenzen

- Kein Docker-Daemon, kein Nesting, wenig RAM, sofortiger Start.
- Sicherung zusätzlich über Proxmox-Snapshots bzw. vzdump des ganzen Containers.
- Ein Update ist: neues Binary kopieren, `install.sh` erneut ausführen. Vor Migrationen schreibt StashBert selbst ein Backup (architecture.md 9.3).
- Die systemd-Unit lässt sich hier nicht in einem echten LXC testen; das prüft der Nutzer.

## Alternativen

- **Docker im LXC:** zusätzliche Schicht und Nesting, ohne Vorteil für ein einzelnes Binary.
- **VM mit Docker:** mehr Ressourcen, sinnvoll erst bei mehreren Diensten.
- **Download aus GitHub-Releases im Container:** braucht ein Token für das private Repository; später möglich.
