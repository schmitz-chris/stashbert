# ADR-0021: Produkterkennung per Foto mit OpenAI, Gemini oder Claude

- Status: angenommen
- Datum: 2026-09-25
- Ändert: architecture.md 4.2 (ein Geheimnis darf in den Einstellungen gesetzt werden)

## Kontext

- Manche Barcodes stehen in keiner Datenbank. Am 25.09.2026 fanden weder Open Food Facts mit seinen Schwesterdatenbanken noch UPCitemdb, OpenGTINDB oder eine Websuche die Codes `4069365036600` und `7061318240833`. Die Produkte bleiben dann Platzhalter, bis jemand sie von Hand benennt.
- Die Packung enthält fast immer Name, Marke und Menge. Ein Sprachmodell mit Bildverständnis kann sie vom Foto ablesen. Ohne Foto, nur mit dem Barcode, würde es raten.
- Nutzerentscheidung vom 25.09.2026:
  - OpenAI (ChatGPT) einbauen, dazu Google Gemini und Claude (Anthropic) zum Vergleich;
  - nur auf der Produktseite;
  - den API-Schlüssel in den Einstellungen eingeben, nicht in der Konfigurationsdatei.
- Alle drei Anbieter berechnen die Nutzung. Gemini hat einen kostenlosen Zugang mit Mengenbegrenzung; dort dürfen Eingaben zur Verbesserung der Google-Produkte genutzt werden.
- architecture.md 4.2 hielt Geheimnisse bisher aus den Einstellungen heraus, weil es keine Anmeldung gibt (ADR-0013) und `GET /backup` die ganze Datenbank ausliefert.

## Entscheidung

- **Anbieter:**
  - OpenAI über die Responses API: Bild als `input_image`, Antwort per Structured Outputs (`text.format` mit `json_schema`);
  - Google Gemini über `generateContent`: Bild als `inline_data`, Antwort mit `responseMimeType: application/json` und `responseSchema`.
  - Claude über die Messages API: Bild als `image`-Block mit base64, feste Struktur der Antwort über die strukturierte Ausgabe der API oder einen erzwungenen Tool-Aufruf.
  - Alle drei werden direkt per `net/http` angesprochen, ohne SDK und ohne neue Abhängigkeit. Eine kleine Schnittstelle in `internal/recognize` kapselt sie.
- **Modell:**
  - Pro Anbieter gibt es einen Standardwert, beim Bau aus der aktuellen Doku ermittelt.
  - In den Einstellungen ist es änderbar, weil sich die Modellnamen schnell ändern.
- **Ablauf auf der Produktseite:**
  - Hat ein Produkt ein Foto und ist ein Anbieter eingerichtet, gibt es „Mit KI erkennen".
  - Der Server schickt das gespeicherte Foto an den Anbieter und bekommt Name, Marke und Menge zurück.
  - Die Oberfläche zeigt das als Vorschlag im Formular. Gespeichert wird erst, wenn der Nutzer speichert. Ein Feld, das auf dem Foto nicht zu lesen ist, bleibt leer, statt geraten zu werden.
- **Schlüssel in den Einstellungen:**
  - Anbieter, Modell und Schlüssel stehen in der Tabelle `settings`.
  - Der Schlüssel wird nur geschrieben, nie ausgeliefert. Die API zeigt nur, ob er gesetzt ist, und seine letzten vier Zeichen.
  - Beim Speichern prüft StashBert den Schlüssel mit einer kostenlosen Abfrage der Modell-Liste.
- **Nicht im Backup:** `GET /backup` entfernt den Schlüssel aus der Kopie der Datenbank, bevor sie ins Archiv geht. Die täglichen Backups im Container behalten ihn; sie verlassen den Container nicht. Nach „Backup einspielen" muss der Schlüssel gegebenenfalls neu eingegeben werden.
- **Nie im Log:** Der Schlüssel erscheint nie im Log, in Fehlermeldungen oder in MQTT-Nachrichten.

## Konsequenzen

- Neue Endpunkte für die Einrichtung und für die Erkennung, neue Fehlercodes (architecture.md 6).
- Ohne Anmeldung kann jeder im Heimnetz den Schlüssel setzen, ändern oder löschen und die Erkennung auslösen. Lesen kann ihn niemand über die Oberfläche oder die API.
- Fotos gehen an OpenAI, Google bzw. Anthropic. Die Oberfläche sagt das in den Einstellungen.
- Kosten entstehen nur beim Tippen auf „Mit KI erkennen", nie automatisch.
- Die Standardmodelle müssen bei Bedarf angepasst werden, wenn ein Anbieter ein Modell abschaltet.

## Alternativen

- **Barcode an ein Sprachmodell mit Websuche:** findet für nicht erfasste Codes nichts und neigt zum Raten.
- **Eigener Dienst neben StashBert:** zusätzlicher Container und zusätzliche Konfiguration, ohne Vorteil für einen Haushalt (ADR-0010).
- **Schlüssel in der Konfigurationsdatei:** sicherer ohne Anmeldung, aber vom Nutzer ausdrücklich nicht gewünscht.
- **Texterkennung im Browser:** iOS gibt Safari keinen Zugriff auf seine Texterkennung; Bibliotheken dafür sind groß und lesen Packungen schlecht.
