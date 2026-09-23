# Notizen: Scanner-Test R (P0-3)

Wegwerf-Prototyp für den Frontend-Vergleich (ADR-0007), Variante R: React 19, Vite 8, TypeScript, Tailwind CSS 4.

## Werte für den Vergleich

| Wert | Ergebnis |
|---|---|
| Korrekturschleifen bis zum grünen Build | 0 (der erste `npm run build` war fehlerfrei) |
| Aufgetretene Fehler | Keine Build- oder Typfehler. Nach einer Sichtprüfung im Browser drei kleine Layout-Korrekturen: Platzhaltertext „Noch kein Treffer" brach mitten im Wort um, lange Kamera-Labels ließen das Status-Raster überlaufen, der weiße ROI-Rahmen war auf weißem Grund unsichtbar (jetzt mit dunklem Außenring). |
| Größe des Build-Ordners | 1372 KB (`du -sk dist`), davon 1068 KB `zxing_reader.wasm`; JS 274 KB (87 KB gzip), CSS 12 KB |

Stolperstellen, die vorab aus Doku und Paketquellen erkannt wurden:

- Der Konstruktor von `BarcodeDetector` lädt das WASM sofort mit den Overrides, die in diesem Moment gelten. `prepareZXingModule` muss deshalb vor dem ersten `new BarcodeDetector()` laufen, sonst geht der Abruf an jsDelivr.
- Die Vorlage `react-ts` setzt `erasableSyntaxOnly`. Konstruktor-Parameter-Properties (`constructor(private x: T)`) sind damit verboten.

## Spezifikation (14 Punkte)

- [x] 1. Aufbau: eine Seite, TypeScript, Tailwind CSS 4 (`@import "tailwindcss"`, Plugin `@tailwindcss/vite`), kein Backend, `<title>Scanner-Test R</title>`. `zxing-wasm` ist nicht direkt installiert (kommt über `barcode-detector`).
- [x] 2. Start: Button „Scannen starten" öffnet die Kamera erst beim Tippen mit `facingMode: "environment"`, `width: { ideal: 1280 }`, `height: { ideal: 720 }`. Im selben Tap: `navigator.audioSession.type = "playback"` (falls vorhanden) und `new AudioContext()` (`src/scanner/feedback.ts`).
- [x] 3. Kamera-Auswahl: Liste „Kamera" aus `enumerateDevices()` (`videoinput`), Auswahl in `localStorage`, beim nächsten Start verwendet. Beim Wechsel wird der alte Stream gestoppt, bevor `getUserMedia` erneut läuft (`src/scanner/scanner.ts`, `start()` ruft zuerst `stop()`).
- [x] 4. Zoom: Schieberegler „Zoom" nur bei `getCapabilities().zoom`, mit dessen min, max und step.
- [x] 5. Licht: Button „Licht" nur bei `getCapabilities().torch`.
- [x] 6. Dekodierung: Import aus `barcode-detector/ponyfill`, Formate `ean_13`, `ean_8`, `upc_a`. `npm run copy:wasm` (`scripts/copy-wasm.mjs`, nur Node) läuft vor `dev` und `build`, löst `zxing-wasm` per `createRequire` relativ zu `barcode-detector` auf, kopiert `zxing_reader.wasm` nach `public/` und bricht bei abweichendem SHA-256 gegenüber `ZXING_WASM_SHA256` ab. Eingebunden über `prepareZXingModule` mit `locateFile`, kein Abruf von jsDelivr.
- [x] 7. Leseschleife: höchstens 10 Versuche pro Sekunde, der nächste Versuch startet erst nach dem Ende des vorigen (`src/scanner/loop.ts`). Dekodiert wird nur der Streifen 80 % × 30 % in der Bildmitte, als Rahmen über dem Video eingezeichnet.
- [x] 8. Treffer: derselbe Code innerhalb von 2000 ms wird ignoriert (`src/scanner/hits.ts`). Sonst: Code groß, Dauer des `detect`-Aufrufs in ms, Zähler, Liste der letzten 10 Codes mit Uhrzeit.
- [x] 9. Rückmeldung: 300 ms grüne Fläche über dem Video und 880 Hz für 120 ms über Web Audio. Checkbox „Ton über Audio-Element" spielt stattdessen `public/beep.wav` über `<audio>`. `npm run gen:beep` (`scripts/gen-beep.mjs`, nur Node) erzeugt die Datei vor `dev` und `build`.
- [x] 10. Wach bleiben: Screen Wake Lock, solange der Scanner läuft (falls verfügbar).
- [x] 11. Hintergrund: bei `visibilitychange` auf `hidden` werden alle Tracks gestoppt. Danach erscheint „Tippen zum Fortsetzen", ohne automatischen Neustart.
- [x] 12. Statusbereich: User-Agent, `display-mode` (`standalone` oder `browser`), Label der gewählten Kamera, Fehlermeldungen.
- [x] 13. Home-Bildschirm: `public/manifest.webmanifest` (`display: standalone`, Name „Scanner-Test R") und `public/apple-touch-icon.png` (180 × 180 px, einfarbig, erzeugt mit `npm run gen:icon`, nur Node, eingecheckt). Kein Service Worker.
- [x] 14. Auslieferung: `npm run build`, dann `npm run preview -- --host 0.0.0.0 --port 8082`. `preview.allowedHosts: true` in `vite.config.ts`.

## Umsetzungsentscheidungen

- **Abhängigkeiten:** `react`, `react-dom`, `@vitejs/plugin-react`, `vite`, `typescript`, `tailwindcss`, `@tailwindcss/vite`, `barcode-detector`, dazu `@types/react` und `@types/react-dom` aus der Vorlage. Ohne diese Typpakete lässt sich React nicht mit TypeScript bauen; die React-Doku nennt sie für TypeScript. Aus der Vorlage entfernt: `oxlint` samt Konfiguration (Lint-Werkzeug, nicht in der Liste) und `@types/node` (nicht nötig, `vite.config.ts` nutzt keine Node-APIs).
- **Gespeicherte Kamera:** Ist eine Kamera gespeichert, lautet der Aufruf `getUserMedia({ video: { deviceId: { exact }, width, height } })` ohne `facingMode`. Gibt es die Kamera nicht mehr (`OverconstrainedError` oder `NotFoundError`), wird die Auswahl gelöscht, der Fehler angezeigt und mit dem Standardaufruf aus Punkt 2 gestartet. „Tippen zum Fortsetzen" nimmt die zuletzt aktive Kamera.
- **Licht:** WebKit und Chromium melden `torch` als Boolean, die Image-Capture-Spezifikation als Liste. Der Button erscheint, wenn `torch` `true` ist oder die Liste `true` enthält.
- **Audio-Element auf iOS:** iOS erlaubt `play()` außerhalb einer Geste erst, wenn das Element einmal in einer Geste abgespielt wurde. Beim Tippen auf Start, Fortsetzen und die Checkbox wird es deshalb stumm angespielt und sofort pausiert. Ob das auf den iPhones reicht, zeigt der Test in P0-5.
- **Rahmen und Video:** Das Video wird ohne Beschnitt angezeigt (Seitenverhältnis des Streams, höchstens 60 % der Bildschirmhöhe), damit der eingezeichnete Rahmen genau dem dekodierten Streifen entspricht.
- **Scanner-Logik** liegt in `src/scanner/` (Kamera, Dekoder, Schleife, Treffer, Rückmeldung, Sitzung), die Komponente `src/App.tsx` enthält nur Zustand und Oberfläche.

## Befehle

```sh
npm install
npm run build
npm run preview -- --host 0.0.0.0 --port 8082
```

`npm run gen:icon` erzeugt das Icon neu; es ist eingecheckt und nicht Teil des Builds.
