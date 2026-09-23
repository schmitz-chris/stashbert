# ADR-0003: REST-API contract-first mit OpenAPI und oapi-codegen

- Status: angenommen
- Datum: 2026-09-23

## Kontext

Die API soll alles anschließbar machen (Web-Oberfläche, Home Assistant, ESP32, Skripte). Agenten brauchen einen präzisen, maschinell prüfbaren Vertrag.

## Entscheidung

- **Stil:** REST/JSON unter `/api/v1`.
- **Vertrag:** `api/openapi.yaml` im Format OpenAPI 3.1 ist die Quelle der Wahrheit.
- **Servercode:** Der Go-Servercode wird mit **oapi-codegen v2** erzeugt, mit den Optionen `std-http-server` und `strict-server`.
  - Das Werkzeug wird als Go-Tool-Abhängigkeit geführt: `go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen`.
  - Aufruf über `go tool oapi-codegen`.
- **Request-Validierung** gegen die Spec mit `github.com/oapi-codegen/nethttp-middleware`.
- **Fehler:** RFC 9457 Problem Details (`application/problem+json`) mit zusätzlichem Feld `code`.
- **Idempotenz:** Header `Idempotency-Key`. Das ist ein IETF-Entwurf, noch kein RFC, aber de-facto-Standard.
- **Frontend-Client:** Typen mit `openapi-typescript`, Aufrufe mit `openapi-fetch`, beides aus derselben Datei.
- **Ablauf bei jeder API-Änderung:**
  1. `api/openapi.yaml` ändern.
  2. `make generate`.
  3. Implementieren.
  4. Tests.
- Generierte Dateien werden eingecheckt und nie von Hand geändert.

## Konsequenzen

- Die Hilfsfunktionen für Problem Details und die Einbindung der Validierung schreiben wir einmalig selbst.
- Ab dem ersten Release prüft die CI Breaking Changes an `api/openapi.yaml`. Das ist ein eigener Task nach M1.

## Alternativen

- **huma** (Code-first): Validierung und RFC 9457 sind eingebaut, aber die Community ist kleiner und es gibt weniger Trainingsmaterial.
- **TypeSpec:** kompakter, aber wenig Trainingsmaterial.
- **GraphQL, gRPC, tRPC:** passen nicht zu Geräten und Home Assistant, siehe `research.md`.
