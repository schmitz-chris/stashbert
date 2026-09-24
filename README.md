# StashBert

Selbst gehostetes, bewusst kleines Vorratsinventar für einen Haushalt.

StashBert beantwortet vier Fragen:

1. Was habe ich?
2. Wie viel davon habe ich?
3. Wie viel möchte ich normalerweise davon haben?
4. Was muss nachgekauft werden?

Kernidee: Barcode scannen, Piepton, fertig. Primär bedient über das iPhone als Web-App (PWA). Die API ist so gebaut, dass sich später alles Weitere anschließen lässt: Home Assistant, Bring!, ein ESP32-Hardware-Scanner.

StashBert ist ausdrücklich kein Meal Planner, keine Rezept-App und kein ERP.

## Status

Umsetzung von M1 läuft. Der Stand jedes Tasks steht in [docs/plan.md](docs/plan.md).

| Dokument | Inhalt |
|---|---|
| [docs/architecture.md](docs/architecture.md) | aktueller, verbindlicher Stand (M1) |
| [docs/plan.md](docs/plan.md) | kleinteiliger Umsetzungsplan |
| [docs/adr/](docs/adr/) | Architekturentscheidungen |
| [docs/betrieb.md](docs/betrieb.md) | Betriebsanleitung für den Proxmox-LXC |
| [docs/research.md](docs/research.md) | Recherche, Fakten und Quellen |
| [AGENTS.md](AGENTS.md) | Regeln für KI-Agenten |

## Stack (Kurzfassung)

| Bereich | Entscheidung |
|---|---|
| Backend | Go 1.27, Standardbibliothek `net/http` |
| API | REST/JSON, OpenAPI 3.1 contract-first, Servercode mit oapi-codegen |
| Datenbank | SQLite (modernc, ohne cgo), sqlc, goose |
| Frontend | SPA mit React 19, TypeScript, Vite, Tailwind CSS 4 |
| Barcode | `barcode-detector` (Ponyfill) mit zxing-wasm, selbst gehostet |
| Betrieb | ein Binary bzw. Container, ein HTTP-Port; TLS über den eigenen Reverse Proxy |
| Später | MQTT/Home Assistant, Bring!, ESP32 |
