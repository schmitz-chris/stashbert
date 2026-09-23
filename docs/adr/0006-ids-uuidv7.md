# ADR-0006: IDs als UUIDv7

- Status: angenommen
- Datum: 2026-09-23

## Kontext

IDs sind Teil der API und später nicht mehr ohne Breaking Change zu ändern. Denkbare Erweiterungen sind Offline-Buchen, Import und Zusammenführen von Daten.

## Entscheidung

- Alle IDs, die in der API sichtbar sind (Produkte, Buchungen, Mitglieder), sind **UUIDv7** nach RFC 9562.
- In M1 erzeugt der Server die IDs mit `github.com/google/uuid` (`uuid.NewV7`).
- Gespeichert werden sie als TEXT in kanonischer Kleinschreibung.
- Barcodes werden über ihren normalisierten Code identifiziert.

## Konsequenzen

- IDs sind zeitlich sortierbar, das hilft beim Cursor der Buchungsliste.
- Später können Clients eigene UUIDv7 mitsenden. Das ist nicht Teil von M1.

## Alternativen

- **Fortlaufende Integer:** einfacher, aber aufzählbar und bei Import oder Offline-Betrieb kollisionsanfällig.
- **ULID:** ähnlich, aber kein RFC.
