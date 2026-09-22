# Stashbert

Selbst gehostetes, bewusst kleines Vorratsinventar für einen Haushalt.

Stashbert beantwortet vier Fragen:

1. Was habe ich?
2. Wie viel davon habe ich?
3. Wie viel möchte ich normalerweise davon haben?
4. Was muss nachgekauft werden?

Kernidee: Barcode scannen, Piepton, fertig. Primär bedient über das iPhone als Web-App (PWA), später zusätzlich über einen eigenen ESP32-Hardware-Scanner. Home Assistant und Bring! lassen sich anbinden, sind aber keine Voraussetzung.

Stashbert ist ausdrücklich kein Meal Planner, keine Rezept-App und kein ERP.

## Status

Planungsphase. Es gibt noch keinen Anwendungscode.

- Architektur und Technologieentscheidungen: [docs/architecture.md](docs/architecture.md)

## Geplanter Stack (Kurzfassung)

| Bereich | Entscheidung |
|---|---|
| Frontend | Svelte 5 + Vite als statische SPA/PWA, TypeScript |
| Barcode | `barcode-detector` (Ponyfill auf Basis von zxing-wasm), WASM selbst gehostet |
| Backend | Node.js LTS + Hono, TypeScript (native Type Stripping) |
| Datenbank | SQLite (eine Datei, WAL) |
| API | REST/JSON unter `/api/v1`, OpenAPI aus Zod-Schemas |
| Home Assistant | generische Webhooks (Outbox) + REST-Sensor, optional später MQTT |
| Einkaufsliste | Stashbert berechnet, Home Assistant überträgt per `todo.*` nach Bring! |
| Deployment | ein Container, Docker Compose, ein Volume `/data` |

Details, Begründungen und Alternativen stehen im Architektur-Dokument.
