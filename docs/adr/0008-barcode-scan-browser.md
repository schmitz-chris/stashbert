# ADR-0008: Barcode-Scan im Browser mit barcode-detector und zxing-wasm

- Status: angenommen
- Datum: 2026-09-23

## Kontext

- Safari liefert die BarcodeDetector-API auch in Version 27 nicht aus, und auf iOS ist sie selbst mit Flag defekt.
- html5-qrcode ist im Wartungsmodus.
- Details und Quellen: `research.md`, Kapitel 7.

## Entscheidung

- **Bibliothek:** npm-Paket `barcode-detector` (Version 3) mit Import **`barcode-detector/ponyfill`**, nie das Polyfill. Darunter läuft zxing-wasm als dessen eigene, exakt gepinnte Abhängigkeit. `zxing-wasm` wird **nicht** direkt installiert, sonst passt die WASM-Datei nicht zum JavaScript.
- **WASM:** Ein Build-Skript kopiert `zxing_reader.wasm` aus dem zu `barcode-detector` gehörenden `zxing-wasm` (Pfad per Node `createRequire` relativ zu `barcode-detector` auflösen). Es prüft den SHA-256 gegen den exportierten Wert `ZXING_WASM_SHA256`; bei Abweichung bricht der Build ab. Eingebunden wird die Datei über `prepareZXingModule` mit `locateFile`, vom Service Worker vorab gecacht.
- **Formate:** `ean_13`, `ean_8`, `upc_a`. UPC-E nicht, weil dessen Prüfziffer anders berechnet wird (architecture.md 7.1).
- **Kamera:** Der Stream bleibt über mehrere Scans offen. Bei `visibilitychange` auf `hidden` werden die Tracks gestoppt. Es gibt nie einen zweiten `getUserMedia`-Aufruf, solange ein Stream läuft.
- **Doppel-Scans:** Derselbe Code wird innerhalb von 2 s nur einmal verarbeitet.
- **Rückmeldung:** farbige Fläche plus Web-Audio-Ton. `navigator.audioSession.type = "playback"` wird gesetzt, falls vorhanden.

## Konsequenzen

- Das Scanner-Modul ist framework-unabhängiger TypeScript-Code in einem eigenen Ordner.
- Kamerawahl und Nahfokus auf dem iPhone 16 Pro werden in Phase 0 geprüft.

## Alternativen

- Native BarcodeDetector-API, html5-qrcode, quagga2, eine native iOS-App (`research.md`, 7.9).
