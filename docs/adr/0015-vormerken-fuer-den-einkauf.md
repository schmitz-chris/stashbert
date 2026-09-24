# ADR-0015: Vormerken für den Einkauf

- Status: angenommen
- Datum: 2026-09-24

## Kontext

- Die Einkaufsliste wird berechnet: Ein Produkt steht darauf, wenn der Bestand unter dem Soll bzw. Mindestbestand liegt.
- Im Alltag gibt es einen zweiten Fall: Man sieht ein Produkt und möchte spontan mehr davon, ohne das Soll zu ändern. Oft ist das einmalig.
- Der Nutzer wünscht dafür einen dritten Scan-Modus „Einkaufen" neben „Einlagern" und „Entnehmen".
- Eine Mengenlogik (Merkmenge, Maximum oder Summe mit dem Fehlbestand) wurde verworfen: zu komplex für das Problem.

## Entscheidung

- **Vormerkung als Ja/Nein-Merker am Produkt** (`products.marked`), keine Menge.
- **Einkaufen ändert nie den Bestand.** Nur Einlagern und Entnehmen (sowie Inventur, Storno, Zusammenführen) ändern ihn.
  - Bekanntes Produkt: wird vorgemerkt, sonst nichts.
  - Unbekannter Barcode: das Produkt wird wie beim Einlagern angelegt (Lookup nach 7.2), aber **ohne Buchung**, also mit Bestand 0, und vorgemerkt.
- **Die Einkaufsliste** enthält alle Produkte mit `missing > 0` **oder** `marked`. Vorgemerkte Produkte ohne Fehlbestand erscheinen ohne Mengenangabe.
- **Die Vormerkung endet** mit der nächsten Buchung `add` dieses Produkts (Nachschub eingelagert) oder von Hand in der Einkaufsansicht.
- **Zusammenführen:** Das Ziel ist vorgemerkt, wenn Quelle oder Ziel es war.
- **Soll und Mindestbestand** bleiben unberührt.

## Konsequenzen

- Neue Spalte, zwei Endpunkte (`markShoppingItem`, `unmarkShoppingItem`), ein Feld in `Product` und `ShoppingItem`.
- Ereignisse für Vormerkungen kommen erst mit M2 (Outbox und Bring!-Sync); in M1 gehen Ereignisse ohnehin ins Leere. Die Nutzlast von `shopping.changed` wird dann um `marked` erweitert.
- Ein unbekanntes Produkt, das im Regal steht und im Modus Einkaufen gescannt wird, hat in StashBert zunächst Bestand 0, bis es eingelagert oder per Inventur korrigiert wird. Das ist gewollt: einfache Regel vor Genauigkeit.

## Alternativen

- **Merkmenge mit Mengenrechnung:** zu komplex.
- **Soll erhöhen:** verändert das Soll dauerhaft, obwohl der Wunsch oft einmalig ist.
- **Freie Texteinträge auf der Einkaufsliste:** nutzen den Scanner nicht und haben keine Verbindung zum Produkt.
