# ADR-0010: Betrieb als ein Binary mit einem HTTP-Port

- Status: angenommen, ergänzt durch ADR-0014 (Hauptweg LXC mit systemd, Image optional)
- Datum: 2026-09-23

## Kontext

TLS, DNS und Reverse Proxy betreibt der Nutzer mit eigenen Werkzeugen. StashBert soll portabel und klein sein.

## Entscheidung

- **Ein Go-Binary** enthält API, Web-Oberfläche, Migrationen und `openapi.yaml`, alles eingebettet.
- **Ein HTTP-Port** (Standard 8080), kein TLS in StashBert.
- **Zustand** nur in `DATA_DIR`, **Konfiguration** nur über Umgebungsvariablen (`architecture.md`, 9.2).
- **Image:** `gcr.io/distroless/static-debian13:nonroot`, Multi-Arch. Docker Compose mit genau einem Dienst.
- **Keine Mandantenfähigkeit:** Eine Instanz ist ein Haushalt.

## Konsequenzen

- Läuft im LXC auf Proxmox, in einer VM oder direkt als Binary.
- Die Anforderungen an den vorgelagerten Proxy stehen in `architecture.md`, 9.1.

## Alternativen

- **Getrennter Frontend-Container:** mehr Konfiguration ohne Nutzen.
- **TLS in StashBert:** doppelt zur vorhandenen Infrastruktur.
