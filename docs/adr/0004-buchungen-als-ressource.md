# ADR-0004: Bestandsänderungen als Buchungen (Ressource `movements`)

- Status: angenommen
- Datum: 2026-09-23

## Kontext

Scannen, [+]/[-] in der Liste, Inventur und Rückgängig ändern alle den Bestand. Ein eigener Scan-Endpunkt neben einem Bestands-Endpunkt wäre inkonsistent.

## Entscheidung

- Der Bestand ändert sich ausschließlich über `POST /api/v1/movements` (Ledger-Muster).
- Das Produkt wird über `product_id` **oder** `barcode` adressiert. Die Arten sind `add`, `consume` und `inventory`.
- Storno über `POST /api/v1/movements/{id}/reversal`. Das legt eine Gegenbuchung an und löscht nichts.
- Der Bestand ist am Produkt gespeichert und wird in derselben Transaktion wie die Buchung geändert. Buchungen werden nur angehängt; die einzige Ausnahme ist das Zusammenführen von Produkten.
- Jede Buchung speichert das Mitglied der Sitzung (`member_id`).
- Die genauen Regeln stehen in `architecture.md`, 6.3.

## Konsequenzen

- ESP32, Kurzbefehl und Web-Oberfläche nutzen später denselben Endpunkt.
- Das Protokoll ist vollständig und eignet sich für Auswertungen.

## Alternativen

- `POST /scan` plus `POST /products/{id}/stock`: zwei Wege für dasselbe.
- Reines Event Sourcing ohne gespeicherten Bestand: für diese Größe unnötig komplex.
