# ADR-0011: Interne Domänen-Ereignisse ab M1, Integrationen ab M2

- Status: angenommen
- Datum: 2026-09-23

## Kontext

- Home Assistant, MQTT und Bring! sollen später angebunden werden (`research.md`, Kapitel 10 und 11).
- Die Erprobung soll klein bleiben, ein späterer Anschluss aber keinen Umbau erfordern.

## Entscheidung

- **Interface in M1:** `internal/events` definiert ein Interface `Publisher` mit einer Methode, die ein Ereignis entgegennimmt. Die Implementierung ist in M1 ein No-op.
- **Auslösen:** Die Fachlogik löst nach jedem erfolgreichen Commit Ereignisse aus.
- **Typen:** `product.created`, `stock.added`, `stock.consumed`, `stock.adjusted`, `product.empty`, `shopping.changed`.
- **Format:** angelehnt an CloudEvents 1.0 (`id`, `type`, `source`, `time`, `data`).
- **M2:**
  - Transaktionale Outbox und Zustellung per MQTT 5 an Mosquitto, inklusive HA-Discovery.
  - Bring! über HA `todo.*` mit wählbarer Ziel-Liste.
  - Die Details werden dann geplant.

## Konsequenzen

- M2 fügt nur Empfänger hinzu. Die Fachlogik ändert sich nicht.

## Alternativen

- Integrationen gleich in M1: mehr Umfang, bevor feststeht, ob der Ablauf überhaupt trägt.
