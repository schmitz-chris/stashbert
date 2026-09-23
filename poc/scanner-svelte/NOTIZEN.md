# Scanner-Test S: Notizen (P0-2)

Variante S des Frontend-Vergleichs (ADR-0007): Svelte 5 + SvelteKit 2 im SPA-Modus, TypeScript, Tailwind CSS 4. Umgesetzt nach der „Spezifikation der Scanner-Testseite" in `docs/plan.md`.

## Befehle

```sh
npm install
npm run build                                        # vorher: copy:wasm und gen:beep (prebuild)
npm run preview -- --host 0.0.0.0 --port 8081
npm run dev                                          # vorher: copy:wasm und gen:beep (predev)
npm run gen:icon                                     # apple-touch-icon.png neu erzeugen (eingecheckt)
```

Optional HTTPS ohne Reverse Proxy (Ordner `.cert/` ist per `.gitignore` ausgeschlossen):

```sh
mkdir -p .cert
openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=localhost" \
  -keyout .cert/key.pem -out .cert/cert.pem
```

Liegen beide Dateien vor, liefert `npm run preview` HTTPS aus. Zum Zurückschalten auf HTTP den Ordner löschen.

## Werte für den Vergleich

| Wert | Ergebnis |
|---|---|
| Korrekturschleifen bis zum grünen Build | 1 (Svelte-Autofixer, siehe unten). Der erste `npm run build` war grün. |
| Svelte-4-Syntax | keine gemeldet (weder Autofixer noch Build) |
| Build-Warnungen | keine |
| Größe `build/` | 1224 KB (1.253.467 Byte, Summe der Dateien; `du -sk build`: 1268 KB) |
| davon `zxing_reader.wasm` | 1068 KB |
| ohne WASM | 156 KB, davon JavaScript 135 KB (gzip 49 KB) |

Aufgetretene Meldungen:

1. **Svelte-Autofixer** (`+page.svelte`): „Each block should have a key" für die Liste der Fehlermeldungen. Behoben: Fehlermeldungen haben jetzt eine laufende `id` als Schlüssel. Zweiter Lauf ohne Befund.
2. **svelte-check** (einmalig per `npx svelte-check@4.7.6`, nicht installiert): `vite.config.ts` findet die Typen für `node:fs` nicht, weil `@types/node` nicht zu den erlaubten Abhängigkeiten gehört. Nicht behoben; der Build ist davon nicht betroffen. Sonst keine Befunde in 417 Dateien.

Nicht durch Werkzeuge ausgelöste Nacharbeit: `copy:wasm` prüft den Hash jetzt vor dem Schreiben, Formatliste als `BarcodeFormat[]` typisiert, `README.md` und `.vscode/` der Vorlage entfernt.

## Angelegt mit

```sh
npx sv@0.17.1 create --template minimal --types ts \
  --add tailwindcss="plugins:none" sveltekit-adapter="adapter:static" \
  --install npm poc/scanner-svelte
npm install barcode-detector@^3.2.2
npm uninstall svelte-check
```

Installierte Versionen: `@sveltejs/kit` 2.70.3, `svelte` 5.57.1, `@sveltejs/adapter-static` 3.0.10, `@sveltejs/vite-plugin-svelte` 7.3.0, `vite` 8.3.0, `tailwindcss` und `@tailwindcss/vite` 4.3.3, `typescript` 6.0.3, `barcode-detector` 3.2.2 (bringt `zxing-wasm` 3.1.3 mit).

## Spezifikation (14 Punkte)

- [x] **1. Aufbau:** eine Seite (`src/routes/+page.svelte`), TypeScript, Tailwind CSS 4 (`src/routes/layout.css`), keine API-Aufrufe. `<title>Scanner-Test S</title>` steht in `src/app.html`, damit er auch ohne SSR im ausgelieferten HTML steht. Abhängigkeiten nur Kit, Svelte, Adapter, Vite-Plugins, `vite`, `typescript`, `tailwindcss`, `@tailwindcss/vite`, `barcode-detector`. `svelte-check` samt `check`-Skripten entfernt.
- [x] **2. Start:** Button „Scannen starten" ruft `Scanner.start`. Im selben Tap: `navigator.audioSession.type = "playback"` (falls vorhanden), `new AudioContext()`, dann `getUserMedia` mit `facingMode: "environment"`, `width/height ideal 1280/720` (`camera.ts`, `videoConstraints`).
- [x] **3. Kamera-Auswahl:** Liste „Kamera" aus `enumerateDevices()` (`videoinput`). Auswahl in `localStorage` (`scanner-test.cameraId`), beim Start mit `deviceId: { exact }`. Schlägt das mit `OverconstrainedError` oder `NotFoundError` fehl, wird die Auswahl gelöscht und mit den Vorgaben gestartet. Beim Wechsel wird der alte Stream gestoppt, bevor der neue geöffnet wird.
- [x] **4. Zoom:** Schieberegler nur bei `getCapabilities().zoom`, mit dessen `min`, `max`, `step`; gesetzt über `applyConstraints({ advanced: [{ zoom }] })`.
- [x] **5. Licht:** Button „Licht" nur, wenn `getCapabilities().torch` `true` ist oder `true` enthält.
- [x] **6. Dekodierung:** Import aus `barcode-detector/ponyfill`, Formate `ean_13`, `ean_8`, `upc_a`. `scripts/copy-wasm.js` löst `zxing-wasm` per `createRequire` relativ zu `barcode-detector` auf, prüft den SHA-256 gegen `ZXING_WASM_SHA256` aus `barcode-detector/ponyfill` (Abbruch mit Exit-Code 1) und schreibt `public/zxing_reader.wasm`. Läuft als `predev` und `prebuild`. `prepareZXingModule` mit `locateFile` wird in `createDetector` direkt vor `new BarcodeDetector()` aufgerufen. Im Test wurde die WASM-Datei nur von `localhost:8081` geladen.
- [x] **7. Leseschleife:** `read-loop.ts`: nächster Versuch erst nach Ende des vorigen und frühestens 100 ms nach dessen Start (gemessen: 30 Versuche in 3 s). Dekodiert wird der Streifen 80 % × 30 % in der Bildmitte (`stripRegion`), derselbe Bereich ist als roter Rahmen über dem Video eingezeichnet.
- [x] **8. Treffer:** gleicher Code wie der letzte Treffer innerhalb von 2000 ms wird ignoriert (`hits.ts`). Anzeige: Code groß, Dauer des `detect`-Aufrufs in ms, Zähler. Liste der letzten 10 Codes mit Uhrzeit.
- [x] **9. Rückmeldung:** 300 ms grüne Fläche über dem Video, Ton 880 Hz für 120 ms per Web Audio (`feedback.ts`). Checkbox „Ton über Audio-Element" spielt stattdessen `public/beep.wav` (erzeugt von `scripts/gen-beep.js`, 880 Hz, 120 ms, 16 Bit mono). Das `<audio>`-Element wird bei Start, Fortsetzen und Tippen auf die Checkbox stumm gestartet und sofort pausiert.
- [x] **10. Wach bleiben:** `navigator.wakeLock.request("screen")`, solange der Scanner läuft (falls verfügbar), Freigabe beim Stoppen.
- [x] **11. Hintergrund:** `<svelte:document onvisibilitychange>`: bei `hidden` werden alle Tracks gestoppt, danach erscheint „Tippen zum Fortsetzen". Kein automatischer Neustart.
- [x] **12. Statusbereich:** User-Agent, `display-mode` (`standalone` oder `browser`, über `MediaQuery`), Label der aktiven Kamera, Fehlermeldungen.
- [x] **13. Home-Bildschirm:** `public/manifest.webmanifest` (`name` „Scanner-Test S", `display: standalone`), `public/apple-touch-icon.png` (180 × 180, einfarbig, erzeugt von `scripts/gen-icon.js` nur mit Node). Kein Service Worker.
- [x] **14. Auslieferung:** `npm run build`, dann `npm run preview -- --host 0.0.0.0 --port 8081`; `preview.allowedHosts: true` in `vite.config.ts`. Mit `.cert/key.pem` und `.cert/cert.pem` setzt `vite.config.ts` `preview.https` (geprüft: HTTP/2 über HTTPS, Status 200).

## Entscheidungen und Abweichungen

- **`public/` statt `static/`:** SvelteKit nutzt standardmäßig `static/`. Die Spezifikation und die `.gitignore` im Wurzelverzeichnis nennen `public/`, deshalb `files: { assets: 'public' }`. Die Option ist in SvelteKit 2 als „deprecated" markiert, funktioniert aber.
- **Fallback-Seite `index.html`:** Die SvelteKit-Doku rät zu `200.html`, um Konflikte mit vorgerenderten Seiten zu vermeiden. Hier wird nichts vorgerendert, und die spätere Auslieferung im Go-Binary (B29) erwartet `index.html`.
- **Konfiguration in `vite.config.ts`:** `sv` 0.17.1 legt keine `svelte.config.js` mehr an; seit Kit 2.62 werden die Optionen direkt an `sveltekit({...})` übergeben.
- **`beep.wav` wird bei jedem `dev` und `build` erzeugt**, weil sie wie die WASM-Datei per `.gitignore` ausgeschlossen ist. Das Icon ist eingecheckt.
- **Doppel-Erkennung:** Vergleich nur mit dem letzten angenommenen Treffer. Bleibt ein Code im Bild, zählt er alle 2 s erneut.
- **Fehlermeldungen:** die letzten 5; eine Meldung, die der vorigen gleicht, wird nicht wiederholt.
- **Kamera-Auswahl** ist gesperrt, solange eine Kamera geöffnet wird, damit nie zwei `getUserMedia`-Aufrufe gleichzeitig laufen. Ein Wechsel im pausierten Zustand speichert nur.
- **Typen:** `zoom`, `torch` und `navigator.audioSession` fehlen in den DOM-Typen von TypeScript und sind lokal ergänzt.
- **Vorschau:** Unbekannte Pfade liefern in `vite preview` 404, weil SvelteKit dort den Fallback nicht nutzt. Für den Test zählt nur `/`.

## Lokaler Browsertest (ohne Kamera)

Im Browser-Fenster war keine Kamera erlaubt. Deshalb wurde `getUserMedia` per Konsole durch einen `canvas.captureStream()` mit gezeichnetem EAN-13 ersetzt:

- `4006381333931` und `5901234123457` erkannt, `detect` rund 6 ms, grüne Fläche sichtbar.
- Gleicher Code im Bild: ein Treffer alle 2 s; Liste bleibt bei 10 Einträgen.
- `visibilitychange` auf `hidden`: Track `ended`, danach nur „Tippen zum Fortsetzen", kein weiterer `getUserMedia`-Aufruf.
- Gespeicherte, nicht vorhandene Kamera: erst `deviceId: { exact }`, dann Vorgaben, `localStorage` geleert.
- Kamerawechsel: alter Track war beim neuen `getUserMedia`-Aufruf bereits `ended`.
- Zoom und Licht mit simulierten Capabilities: Regler 1 bis 5, Schritt 0,5; `applyConstraints` mit `zoom` bzw. `torch`.
- Checkbox: `<audio>` stumm gestartet und pausiert; Treffer spielen das Audio-Element, ohne Checkbox Web Audio.
- Netzwerk: nur Anfragen an `localhost:8081`.

Die Prüfung auf den iPhones (P0-5) und der Netzwerk-Tab (Abnahmekriterium 4) stehen aus.
