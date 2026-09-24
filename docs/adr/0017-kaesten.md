# ADR-0017: Kästen (Wasser, Bier) ohne eigenen Barcode

- Status: angenommen
- Datum: 2026-09-24

## Kontext

- Getränkekästen im Haushalt (Wasser, Bier) haben keinen eigenen Barcode; gescannt wird der Barcode der Flasche.
- Der Nutzer möchte beim Scannen gefragt werden, ob es eine Flasche oder ein Kasten ist, und bei einem Kasten, wie groß er ist.
- Gezählt wird in Flaschen. Bier wird nach einem Fest am nächsten Tag per Inventur korrigiert, nicht beim Fest selbst.
- Die Einkaufsliste soll für solche Produkte in Kästen rechnen.
- Die bestehende Stückzahl pro Barcode (`barcodes.units`) hilft hier nicht, weil Flasche und Kasten denselben Barcode haben.

## Entscheidung

- **Kastengröße am Produkt:** neue Spalte `products.crate_size` (NULL oder 2 bis 100, Flaschen pro Kasten). Bestand, Soll und Buchungen bleiben in Flaschen.
- **Scannen im Modus Einlagern:**
  - Hat das Produkt eine Kastengröße, fragt die App vor dem Buchen: „Flasche" oder „Kasten (N Flaschen)". Gebucht wird 1 bzw. N. Solange die Frage offen ist, werden weitere Scans ignoriert.
  - Hat das Produkt keine Kastengröße (auch bei neu angelegten Produkten), wird wie bisher sofort 1 gebucht. Die Ergebniskarte bietet „War ein Kasten": Auswahl der Größe (6, 12, 20, 24 oder eine andere Zahl), dann wird die Kastengröße am Produkt gespeichert und der Rest (N − 1) gebucht. Beim nächsten Scan kommt die Frage von selbst.
  - Im Modus Entnehmen gibt es keine Frage; gebucht wird eine Flasche. Größere Änderungen laufen über „Bestand setzen" (Inventur).
  - Im Modus Einkaufen gibt es keine Frage.
- **Welches Produkt vor dem Buchen?** Die Scan-Ansicht findet das Produkt zum Barcode in der zwischengespeicherten Produktliste (die Barcodes stehen dort schon); ein neuer API-Endpunkt ist dafür nicht nötig. Ist die Liste nicht geladen oder der Barcode unbekannt, wird wie bisher sofort gebucht.
- **Produktseite:** Feld „Kastengröße" (Flaschen pro Kasten, leer für „kein Kasten").
- **Einkaufsliste:** Bei Produkten mit Kastengröße wird die fehlende Menge in Kästen aufgerundet angezeigt, z. B. „1 Kasten · fehlen 17 Flaschen"; der geteilte Text lautet „1 Kasten Jever Pilsener". Vorgemerkte Produkte ohne Fehlbestand bleiben ohne Menge.

## Konsequenzen

- Migration, ein Feld in `Product`, `ProductCreate`, `ProductPatch` und `ShoppingItem`.
- Drei kleine Tasks: B33 (Datenmodell und API), F27 (Produktseite und Einkaufsliste), F28 (Frage beim Scannen).
- Der Scan-Ablauf bleibt für alle Produkte ohne Kastengröße „scannen, Piep, fertig".

## Alternativen

- **Eigener Barcode je Kasten mit `units`:** Kästen haben keinen eigenen Barcode.
- **Kästen statt Flaschen zählen:** zu grob für Bier, das flaschenweise korrigiert werden soll.
- **Immer fragen, auch ohne Kastengröße:** stört bei allen anderen Produkten.
