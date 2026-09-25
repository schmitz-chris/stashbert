# P0: Protokoll Scanner-Test

| | |
|---|---|
| Stand | 23.09.2026 |
| Status | verkürzt ausgefüllt in P0-5, Ergebnis in 8.2 |
| Varianten | S: `poc/scanner-svelte/` (Port 8081), R: `poc/scanner-react/` (Port 8082) |
| Geräte | iPhone 15, iPhone 16 Pro |
| Grundlagen | [plan.md](../plan.md) (P0-4, P0-5, Spezifikation der Scanner-Testseite), [ADR-0007](../adr/0007-frontend-ab-vergleich.md), [research.md](../research.md) Kapitel 7 und 22 (POC-1 bis POC-3) |

Dieses Protokoll hält den Test beider Scanner-Testseiten auf beiden iPhones fest und ist die Grundlage für die Frontend-Entscheidung in P0-6. Die Testfälle decken POC-1 (Erkennung, Kamera, Zoom), POC-2 (Kamera-Rückfrage) und POC-3 (Ton) ab.

## 1. Anleitung: Vorschauen bereitstellen

Die Kamera gibt iOS nur in einem sicheren Kontext frei, also über HTTPS (research.md 7.3). Es gibt zwei Wege:

| Weg | Wann | Adressen auf dem iPhone | Zertifikatswarnung |
|---|---|---|---|
| A: selbst signiertes Zertifikat | jetzt, ohne Reverse Proxy | `https://<Mac-IP>:8081` (S), `https://<Mac-IP>:8082` (R) | ja, einmal pro Adresse bestätigen |
| B: eigener Reverse Proxy | später | `https://<Hostname S>` (S), `https://<Hostname R>` (R) | nein |

Tailscale `serve` (research.md 7.6) scheidet aus, weil der Mac in einem Firmen-Tailnet hängt.

Stand des bisherigen Tests: Weg A mit `https://<Mac-IP>:8081` (S) und `https://<Mac-IP>:8082` (R). Mit bestätigter Warnung gibt iOS die Kamera frei (belegt mit Variante R auf dem iPhone).

### 1.1 Voraussetzungen

- Mac und iPhones sind im selben WLAN.
- Node.js mit npm ist auf dem Mac installiert.
- Alle Befehle laufen im Wurzelverzeichnis des Repositorys.

### 1.2 Bauen

Vor dem ersten Start und nach jeder Änderung am Code:

```sh
(cd poc/scanner-svelte && npm install && npm run build)
(cd poc/scanner-react && npm install && npm run build)
```

`npm run build` legt vorher `public/zxing_reader.wasm` und `public/beep.wav` an (Skripte `copy:wasm` und `gen:beep`). Das Ergebnis liegt in `poc/scanner-svelte/build/` bzw. `poc/scanner-react/dist/`.

### 1.3 Mac-IP ermitteln

```sh
ipconfig getifaddr en0
```

Auf diesem Mac ist `en0` die Schnittstelle ins Heimnetz. Liefert der Befehl nichts, zeigt `route -n get default` in der Zeile `interface:` die aktive Schnittstelle; deren Namen statt `en0` einsetzen. Gesucht ist die Adresse im Heimnetz. Eine Adresse aus `100.64.0.0/10` gehört zum Tailnet und ist hier falsch.

Die IP kann sich ändern, weil der Router sie per DHCP vergibt. Dann gilt „Wenn sich die IP ändert" in 1.4 bzw. Schritt 5 in 1.5. Eine feste Adresse für den Mac (DHCP-Reservierung im Router) erspart diese Schritte.

### 1.4 Weg A: selbst signiertes Zertifikat (jetzt)

Die Vite-Konfiguration beider Projekte schaltet HTTPS ein, sobald `.cert/key.pem` und `.cert/cert.pem` im Projektordner liegen (`preview.https`). `.gitignore` schließt `poc/*/.cert/` aus; der Schlüssel gehört nie ins Repository.

1. **Zertifikat erzeugen**, mit der Mac-IP als SAN, 30 Tage gültig, für beide Projekte:

   ```sh
   IP=$(ipconfig getifaddr en0)
   mkdir -p poc/scanner-svelte/.cert poc/scanner-react/.cert
   openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
     -subj "/CN=stashbert-poc" \
     -addext "subjectAltName=IP:$IP,DNS:localhost" \
     -keyout poc/scanner-svelte/.cert/key.pem \
     -out poc/scanner-svelte/.cert/cert.pem
   cp poc/scanner-svelte/.cert/key.pem poc/scanner-svelte/.cert/cert.pem poc/scanner-react/.cert/
   ```

   Prüfen:

   ```sh
   openssl x509 -in poc/scanner-svelte/.cert/cert.pem -noout -text | grep -A1 "Subject Alternative Name"
   openssl x509 -in poc/scanner-svelte/.cert/cert.pem -noout -enddate
   ```

   Erwartet: `IP Address:<Mac-IP>, DNS:localhost` und das Ablaufdatum. Die Befehle laufen mit OpenSSL 3 (Homebrew) und mit dem LibreSSL von macOS (`/usr/bin/openssl`). Das derzeitige Zertifikat gilt bis 23.10.2026. Der kürzere Befehl in `poc/scanner-svelte/NOTIZEN.md` setzt keinen SAN; maßgeblich ist der Befehl hier.

2. **Vorschauen starten**, je in einem eigenen Terminal:

   ```sh
   cd poc/scanner-svelte && npm run preview -- --host 0.0.0.0 --port 8081
   ```

   ```sh
   cd poc/scanner-react && npm run preview -- --host 0.0.0.0 --port 8082
   ```

   Vite liest das Zertifikat nur beim Start. Nach einem neuen Zertifikat beide Vorschauen mit Ctrl+C beenden und neu starten. Neu bauen ist nicht nötig.

3. **Am Mac prüfen:**

   ```sh
   curl -sk https://localhost:8081 | grep -o '<title>[^<]*</title>'
   curl -sk https://localhost:8082 | grep -o '<title>[^<]*</title>'
   ```

   Erwartet: `<title>Scanner-Test S</title>` bzw. `<title>Scanner-Test R</title>`. `-k` ist nötig, weil curl dem selbst signierten Zertifikat nicht vertraut.

4. **Auf dem iPhone** in Safari `https://<Mac-IP>:8081` und `https://<Mac-IP>:8082` öffnen. Safari warnt, dass die Verbindung nicht privat ist. Über die Details den Besuch der Website bestätigen, einmal pro Adresse. Danach fragt die Seite beim Tippen auf „Scannen starten" nach der Kamera.

   Ob eine Home-Bildschirm-App die bestätigte Ausnahme übernimmt, ist nicht dokumentiert (vgl. research.md 7.6). Was dabei passiert, in Testfall 3 festhalten. Ergebnisse aus Weg A sind in diesem Punkt nur eingeschränkt auf den späteren Betrieb hinter dem Proxy übertragbar.

**Wenn sich die IP ändert:**

1. Neue IP ermitteln (1.3).
2. Zertifikat neu erzeugen (Schritt 1). Der SAN des alten Zertifikats passt nicht mehr.
3. Beide Vorschauen neu starten (Schritt 2) und prüfen (Schritt 3).
4. Auf dem iPhone die neue Adresse öffnen und die Warnung erneut bestätigen (Schritt 4). Für iOS ist das eine neue Website: Kamera-Freigaben und Home-Bildschirm-Einträge der alten Adresse gelten nicht mehr. Home-Bildschirm-Einträge neu anlegen und einen angefangenen Testfall 3 neu beginnen.

Läuft das Zertifikat nach 30 Tagen ab, gelten die Schritte 2 bis 4 genauso, mit unveränderter IP.

### 1.5 Weg B: eigener Reverse Proxy (später)

Der Proxy terminiert TLS mit einem Zertifikat, dem das iPhone vertraut. Die Vorschauen sprechen dann HTTP.

1. **Selbst signiertes Zertifikat entfernen**, damit die Vorschauen HTTP sprechen:

   ```sh
   rm -r poc/scanner-svelte/.cert poc/scanner-react/.cert
   ```

   Den Ordner nicht umbenennen: `.gitignore` schließt nur `.cert/` aus.

2. **Vorschauen starten** mit denselben Befehlen wie in Weg A, Schritt 2, und am Mac prüfen:

   ```sh
   curl -s http://localhost:8081 | grep -o '<title>[^<]*</title>'
   curl -s http://localhost:8082 | grep -o '<title>[^<]*</title>'
   ```

3. **Proxy einrichten**, mit zwei Hostnamen:

   | Hostname | Ziel |
   |---|---|
   | `<Hostname S>` | `http://<Mac-IP>:8081` |
   | `<Hostname R>` | `http://<Mac-IP>:8082` |

   Es gelten die Anforderungen an den Proxy vor StashBert (architecture.md 9.1, research.md 16.5), nur mit diesen Zielen:
   - HTTPS mit einem Zertifikat, dem das iPhone vertraut, also ohne Warnung.
   - Alle Pfade ohne Umschreibung weiterleiten. Beide Seiten erwarten, unter `/` zu liegen.
   - Die Vorschau nimmt jeden `Host`-Header an (`preview.allowedHosts: true`).
   - WebSockets werden nicht gebraucht.
   - Beide Hostnamen lösen im Heimnetz auf den Proxy auf. Zum Rebind-Schutz der FRITZ!Box siehe research.md 7.6.

4. **Prüfen:** am Mac ohne `-k`, das Zertifikat muss gültig sein:

   ```sh
   curl -s https://<Hostname S> | grep -o '<title>[^<]*</title>'
   curl -s https://<Hostname R> | grep -o '<title>[^<]*</title>'
   ```

   Auf dem iPhone beide Adressen in Safari öffnen. Es darf keine Warnung erscheinen.

5. Ändert sich die Mac-IP, die beiden Ziele im Proxy anpassen.

Zurück zu Weg A: Zertifikat nach 1.4, Schritt 1, erzeugen und die Vorschauen neu starten.

### 1.6 Bekannte Stolpersteine

| Beobachtung | Ursache | Abhilfe |
|---|---|---|
| Nach „Scannen starten" erscheint der Fehler „undefined is not an object (evaluating 'navigator.mediaDevices.getUserMedia')". | Die Seite ist über `http://` geöffnet. Ohne sicheren Kontext ist `navigator.mediaDevices` undefiniert. | Die Seite über `https://` öffnen (Weg A oder B). |
| `http://<Mac-IP>:8081` lädt nicht. | Liegt `.cert/` im Projekt, spricht der Port nur HTTPS. | `https://` verwenden. |
| Die bisherige Adresse ist nicht mehr erreichbar. | Die Mac-IP hat sich geändert. | 1.4, „Wenn sich die IP ändert", bzw. 1.5, Schritt 5. |
| Das Zertifikat ist abgelaufen (`-enddate` liegt in der Vergangenheit). | Es gilt 30 Tage. | Neu erzeugen, 1.4, Schritte 1 bis 4. |
| Das iPhone erreicht den Mac nicht. | Das iPhone ist nicht im selben WLAN, z. B. im Mobilfunk. | WLAN des Heimnetzes verwenden. |

## 2. Durchführung

Für beide Varianten dieselben 20 Artikel (Abschnitt 3) und möglichst dieselbe Umgebung (Ort, Licht) verwenden, damit S und R vergleichbar sind. Jeder Durchgang (Abschnitte 4 bis 7) beginnt mit Datum, iOS-Version, Weg und Adresse.

**Testfall 1: 20 reale Vorratsartikel.** Die Artikel aus Abschnitt 3 der Reihe nach scannen. Pro Artikel:

- **Treffer:** ja oder nein.
- **Fehlversuche:** Ansätze ohne Treffer. Ein falsch gelesener Code zählt als Fehlversuch und kommt mit dem gelesenen Code in die Bemerkung.
- **Dauer bis Treffer (grob):** vom Ausrichten bis zum Ton, geschätzt, z. B. „unter 1 s", „1 bis 3 s", „über 3 s". Die Dauer in ms auf der Seite misst nur den `detect`-Aufruf, nicht die Zeit bis zum Treffer.

Kamera und Zoom für diesen Testfall im Kopf des Durchgangs eintragen.

**Testfall 2: Kamera-Auswahl und Zoom.** Mit einem Artikel aus Abschnitt 3 jede Kamera aus der Auswahlliste „Kamera" durchprobieren, bei Bedarf mit mehreren Werten am Regler „Zoom" (erscheint nur, wenn die Kamera Zoom meldet). Das Label der aktiven Kamera steht im Statusbereich. Pro Einstellung in 5, 10 und 15 cm Abstand festhalten: „sofort", „langsam" oder „nein". Am Ende die beste Einstellung eintragen.

**Testfall 3: Kamera-Rückfrage.** Die Seite einmal als Safari-Tab und einmal als Home-Bildschirm-App testen (in Safari über Teilen „Zum Home-Bildschirm" hinzufügen). Der Statusbereich zeigt `display-mode`: `browser` im Tab, `standalone` in der App. Je 3 Durchgänge:

- **Kaltstart:** Safari bzw. die App im App-Umschalter schließen, neu öffnen, „Scannen starten" tippen.
- **Bildschirmsperre:** bei laufendem Scanner sperren, entsperren, „Tippen zum Fortsetzen" tippen.
- **App-Wechsel:** bei laufendem Scanner in eine andere App wechseln, zurückkehren, „Tippen zum Fortsetzen" tippen.

Pro Durchgang festhalten: Fragt iOS nach der Kamera (ja/nein)? Läuft die Kamera danach (ja/nein)?

**Testfall 4: Ton.** Vorher Musik in einer anderen App starten. Den Stummmodus je nach Gerät über den Schalter, die Aktionstaste oder das Kontrollzentrum umschalten. Standard ist Web Audio; mit der Checkbox „Ton über Audio-Element" spielt die Seite stattdessen `beep.wav` über ein Audio-Element. Pro Kombination festhalten: Ist der Ton hörbar? Läuft die Musik weiter?

**Testfall 5: Licht, falls vorhanden.** Der Button „Licht" erscheint nur, wenn die gewählte Kamera Licht meldet. Festhalten, bei welcher Kamera er erscheint und ob das Licht an- und ausgeht.

**Testfall 6: Fremd-Anfragen im Netzwerk-Tab.** Am iPhone in den Safari-Einstellungen unter „Erweitert" den Web-Inspektor einschalten, am Mac in Safari die Funktionen für Webentwickler aktivieren. Im Entwickler-Menü des Mac das iPhone und die Seite wählen und den Netzwerk-Tab öffnen. Dann die Seite neu laden und einmal scannen. Erlaubt sind nur Anfragen an die eigene Adresse (Weg A: `<Mac-IP>:<Port>`, Weg B: der jeweilige Hostname). Jeder andere Host, z. B. `cdn.jsdelivr.net`, wird eingetragen.

## 3. Artikelliste

Dieselben 20 Artikel für alle vier Durchgänge, möglichst gemischt, auch gewölbte Dosen und kleine Codes.

| Nr. | Artikel | Code (Ziffern) | Bemerkung (z. B. Dose, Folie, EAN-8) |
|---|---|---|---|
| 1 | | | |
| 2 | | | |
| 3 | | | |
| 4 | | | |
| 5 | | | |
| 6 | | | |
| 7 | | | |
| 8 | | | |
| 9 | | | |
| 10 | | | |
| 11 | | | |
| 12 | | | |
| 13 | | | |
| 14 | | | |
| 15 | | | |
| 16 | | | |
| 17 | | | |
| 18 | | | |
| 19 | | | |
| 20 | | | |

## 4. iPhone 15, Variante S

| | |
|---|---|
| Datum | |
| iOS-Version | |
| Weg (A oder B) | |
| Adresse (Port 8081 bzw. Hostname S) | |
| Kamera und Zoom in Testfall 1 | |

### 4.1 Testfall 1: 20 reale Vorratsartikel

| Nr. | Treffer (ja/nein) | Fehlversuche | Dauer bis Treffer (grob) | Bemerkung |
|---|---|---|---|---|
| 1 | | | | |
| 2 | | | | |
| 3 | | | | |
| 4 | | | | |
| 5 | | | | |
| 6 | | | | |
| 7 | | | | |
| 8 | | | | |
| 9 | | | | |
| 10 | | | | |
| 11 | | | | |
| 12 | | | | |
| 13 | | | | |
| 14 | | | | |
| 15 | | | | |
| 16 | | | | |
| 17 | | | | |
| 18 | | | | |
| 19 | | | | |
| 20 | | | | |
| **Summe** | Treffer: | Fehlversuche: | | |

### 4.2 Testfall 2: Kamera-Auswahl und Zoom (5 bis 15 cm)

Artikel Nr.:

| Kamera (Label) | Zoom | 5 cm | 10 cm | 15 cm | Bemerkung |
|---|---|---|---|---|---|
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |

| Beste Einstellung | |
|---|---|
| Kamera | |
| Zoom | |
| Abstand | |

### 4.3 Testfall 3: Kamera-Rückfrage

| Durchgang | Safari-Tab: Rückfrage | Safari-Tab: Kamera läuft | Home-Bildschirm: Rückfrage | Home-Bildschirm: Kamera läuft | Bemerkung |
|---|---|---|---|---|---|
| Kaltstart 1 | | | | | |
| Kaltstart 2 | | | | | |
| Kaltstart 3 | | | | | |
| Bildschirmsperre 1 | | | | | |
| Bildschirmsperre 2 | | | | | |
| Bildschirmsperre 3 | | | | | |
| App-Wechsel 1 | | | | | |
| App-Wechsel 2 | | | | | |
| App-Wechsel 3 | | | | | |

### 4.4 Testfall 4: Ton

| Ton über | Stummmodus | Ton hörbar (ja/nein) | Musik läuft weiter (ja/nein) | Bemerkung |
|---|---|---|---|---|
| Web Audio | aus | | | |
| Web Audio | an | | | |
| Audio-Element | aus | | | |
| Audio-Element | an | | | |

### 4.5 Testfall 5: Licht

| Button „Licht" erscheint (bei welcher Kamera) | Licht geht an und aus (ja/nein) | Bemerkung |
|---|---|---|
| | | |

### 4.6 Testfall 6: Fremd-Anfragen im Netzwerk-Tab

| Nur Anfragen an die eigene Adresse (ja/nein) | Fremde Hosts | Bemerkung |
|---|---|---|
| | | |

## 5. iPhone 15, Variante R

| | |
|---|---|
| Datum | |
| iOS-Version | |
| Weg (A oder B) | |
| Adresse (Port 8082 bzw. Hostname R) | |
| Kamera und Zoom in Testfall 1 | |

### 5.1 Testfall 1: 20 reale Vorratsartikel

| Nr. | Treffer (ja/nein) | Fehlversuche | Dauer bis Treffer (grob) | Bemerkung |
|---|---|---|---|---|
| 1 | | | | |
| 2 | | | | |
| 3 | | | | |
| 4 | | | | |
| 5 | | | | |
| 6 | | | | |
| 7 | | | | |
| 8 | | | | |
| 9 | | | | |
| 10 | | | | |
| 11 | | | | |
| 12 | | | | |
| 13 | | | | |
| 14 | | | | |
| 15 | | | | |
| 16 | | | | |
| 17 | | | | |
| 18 | | | | |
| 19 | | | | |
| 20 | | | | |
| **Summe** | Treffer: | Fehlversuche: | | |

### 5.2 Testfall 2: Kamera-Auswahl und Zoom (5 bis 15 cm)

Artikel Nr.:

| Kamera (Label) | Zoom | 5 cm | 10 cm | 15 cm | Bemerkung |
|---|---|---|---|---|---|
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |

| Beste Einstellung | |
|---|---|
| Kamera | |
| Zoom | |
| Abstand | |

### 5.3 Testfall 3: Kamera-Rückfrage

| Durchgang | Safari-Tab: Rückfrage | Safari-Tab: Kamera läuft | Home-Bildschirm: Rückfrage | Home-Bildschirm: Kamera läuft | Bemerkung |
|---|---|---|---|---|---|
| Kaltstart 1 | | | | | |
| Kaltstart 2 | | | | | |
| Kaltstart 3 | | | | | |
| Bildschirmsperre 1 | | | | | |
| Bildschirmsperre 2 | | | | | |
| Bildschirmsperre 3 | | | | | |
| App-Wechsel 1 | | | | | |
| App-Wechsel 2 | | | | | |
| App-Wechsel 3 | | | | | |

### 5.4 Testfall 4: Ton

| Ton über | Stummmodus | Ton hörbar (ja/nein) | Musik läuft weiter (ja/nein) | Bemerkung |
|---|---|---|---|---|
| Web Audio | aus | | | |
| Web Audio | an | | | |
| Audio-Element | aus | | | |
| Audio-Element | an | | | |

### 5.5 Testfall 5: Licht

| Button „Licht" erscheint (bei welcher Kamera) | Licht geht an und aus (ja/nein) | Bemerkung |
|---|---|---|
| | | |

### 5.6 Testfall 6: Fremd-Anfragen im Netzwerk-Tab

| Nur Anfragen an die eigene Adresse (ja/nein) | Fremde Hosts | Bemerkung |
|---|---|---|
| | | |

## 6. iPhone 16 Pro, Variante S

| | |
|---|---|
| Datum | |
| iOS-Version | |
| Weg (A oder B) | |
| Adresse (Port 8081 bzw. Hostname S) | |
| Kamera und Zoom in Testfall 1 | |

### 6.1 Testfall 1: 20 reale Vorratsartikel

| Nr. | Treffer (ja/nein) | Fehlversuche | Dauer bis Treffer (grob) | Bemerkung |
|---|---|---|---|---|
| 1 | | | | |
| 2 | | | | |
| 3 | | | | |
| 4 | | | | |
| 5 | | | | |
| 6 | | | | |
| 7 | | | | |
| 8 | | | | |
| 9 | | | | |
| 10 | | | | |
| 11 | | | | |
| 12 | | | | |
| 13 | | | | |
| 14 | | | | |
| 15 | | | | |
| 16 | | | | |
| 17 | | | | |
| 18 | | | | |
| 19 | | | | |
| 20 | | | | |
| **Summe** | Treffer: | Fehlversuche: | | |

### 6.2 Testfall 2: Kamera-Auswahl und Zoom (5 bis 15 cm)

Artikel Nr.:

| Kamera (Label) | Zoom | 5 cm | 10 cm | 15 cm | Bemerkung |
|---|---|---|---|---|---|
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |

| Beste Einstellung | |
|---|---|
| Kamera | |
| Zoom | |
| Abstand | |

### 6.3 Testfall 3: Kamera-Rückfrage

| Durchgang | Safari-Tab: Rückfrage | Safari-Tab: Kamera läuft | Home-Bildschirm: Rückfrage | Home-Bildschirm: Kamera läuft | Bemerkung |
|---|---|---|---|---|---|
| Kaltstart 1 | | | | | |
| Kaltstart 2 | | | | | |
| Kaltstart 3 | | | | | |
| Bildschirmsperre 1 | | | | | |
| Bildschirmsperre 2 | | | | | |
| Bildschirmsperre 3 | | | | | |
| App-Wechsel 1 | | | | | |
| App-Wechsel 2 | | | | | |
| App-Wechsel 3 | | | | | |

### 6.4 Testfall 4: Ton

| Ton über | Stummmodus | Ton hörbar (ja/nein) | Musik läuft weiter (ja/nein) | Bemerkung |
|---|---|---|---|---|
| Web Audio | aus | | | |
| Web Audio | an | | | |
| Audio-Element | aus | | | |
| Audio-Element | an | | | |

### 6.5 Testfall 5: Licht

| Button „Licht" erscheint (bei welcher Kamera) | Licht geht an und aus (ja/nein) | Bemerkung |
|---|---|---|
| | | |

### 6.6 Testfall 6: Fremd-Anfragen im Netzwerk-Tab

| Nur Anfragen an die eigene Adresse (ja/nein) | Fremde Hosts | Bemerkung |
|---|---|---|
| | | |

## 7. iPhone 16 Pro, Variante R

| | |
|---|---|
| Datum | |
| iOS-Version | |
| Weg (A oder B) | |
| Adresse (Port 8082 bzw. Hostname R) | |
| Kamera und Zoom in Testfall 1 | |

### 7.1 Testfall 1: 20 reale Vorratsartikel

| Nr. | Treffer (ja/nein) | Fehlversuche | Dauer bis Treffer (grob) | Bemerkung |
|---|---|---|---|---|
| 1 | | | | |
| 2 | | | | |
| 3 | | | | |
| 4 | | | | |
| 5 | | | | |
| 6 | | | | |
| 7 | | | | |
| 8 | | | | |
| 9 | | | | |
| 10 | | | | |
| 11 | | | | |
| 12 | | | | |
| 13 | | | | |
| 14 | | | | |
| 15 | | | | |
| 16 | | | | |
| 17 | | | | |
| 18 | | | | |
| 19 | | | | |
| 20 | | | | |
| **Summe** | Treffer: | Fehlversuche: | | |

### 7.2 Testfall 2: Kamera-Auswahl und Zoom (5 bis 15 cm)

Artikel Nr.:

| Kamera (Label) | Zoom | 5 cm | 10 cm | 15 cm | Bemerkung |
|---|---|---|---|---|---|
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |
| | | | | | |

| Beste Einstellung | |
|---|---|
| Kamera | |
| Zoom | |
| Abstand | |

### 7.3 Testfall 3: Kamera-Rückfrage

| Durchgang | Safari-Tab: Rückfrage | Safari-Tab: Kamera läuft | Home-Bildschirm: Rückfrage | Home-Bildschirm: Kamera läuft | Bemerkung |
|---|---|---|---|---|---|
| Kaltstart 1 | | | | | |
| Kaltstart 2 | | | | | |
| Kaltstart 3 | | | | | |
| Bildschirmsperre 1 | | | | | |
| Bildschirmsperre 2 | | | | | |
| Bildschirmsperre 3 | | | | | |
| App-Wechsel 1 | | | | | |
| App-Wechsel 2 | | | | | |
| App-Wechsel 3 | | | | | |

### 7.4 Testfall 4: Ton

| Ton über | Stummmodus | Ton hörbar (ja/nein) | Musik läuft weiter (ja/nein) | Bemerkung |
|---|---|---|---|---|
| Web Audio | aus | | | |
| Web Audio | an | | | |
| Audio-Element | aus | | | |
| Audio-Element | an | | | |

### 7.5 Testfall 5: Licht

| Button „Licht" erscheint (bei welcher Kamera) | Licht geht an und aus (ja/nein) | Bemerkung |
|---|---|---|
| | | |

### 7.6 Testfall 6: Fremd-Anfragen im Netzwerk-Tab

| Nur Anfragen an die eigene Adresse (ja/nein) | Fremde Hosts | Bemerkung |
|---|---|---|
| | | |

## 8. Vergleichsbewertung S gegen R

### 8.1 Werte aus NOTIZEN.md

Quelle: `poc/scanner-svelte/NOTIZEN.md` (P0-2) und `poc/scanner-react/NOTIZEN.md` (P0-3).

| Wert | S (Svelte) | R (React) |
|---|---|---|
| Korrekturschleifen bis zum grünen Build | 1 (Svelte-Autofixer: fehlender Schlüssel im `each`-Block); der erste `npm run build` war grün | 0; der erste `npm run build` war fehlerfrei |
| Build- und Typfehler | keine; Svelte-4-Syntax: keine | keine |
| Offene Befunde | `svelte-check` (einmalig ausgeführt) findet in `vite.config.ts` die Typen für `node:fs` nicht, weil `@types/node` nicht erlaubt ist; Build nicht betroffen | keine |
| Weitere Nacharbeit | Hash-Prüfung in `copy:wasm` vor dem Schreiben, Formatliste typisiert, Vorlagendateien entfernt | drei Layout-Korrekturen nach Sichtprüfung (Umbruch des Platzhaltertexts, Überlauf bei langen Kamera-Labels, unsichtbarer Rahmen auf weißem Grund) |
| Build-Ordner (`du -sk`) | 1268 KB (`build/`) | 1372 KB (`dist/`) |
| davon `zxing_reader.wasm` | 1068 KB | 1068 KB |
| JavaScript | 135 KB (gzip 49 KB) | 274 KB (gzip 87 KB) |

### 8.2 Bewertung

Je Kriterium 1 bis 5 Punkte, 5 ist am besten. Die Kriterien stammen aus ADR-0007.

| Kriterium | S | R | Begründung |
|---|---|---|---|
| Funktion auf beiden Geräten (Abschnitte 4 bis 7) | | | |
| Nacharbeit laut NOTIZEN.md (8.1) | | | |
| Verständlichkeit des Codes | | | |
| Gefühl | | | |
| **Summe** | | | |

| Ergebnis | |
|---|---|
| Gewählte Variante | R (React), entschieden vom Nutzer am 23.09.2026 |
| Begründung | Beide Varianten liefen auf dem iPhone (Standardkamera des 16 Pro reicht aus der Nähe, Kamera-Rückfrage nur einmal). React brauchte keine Nacharbeit bis zum grünen Build und hat das meiste Trainingsmaterial. Der React-Prototyp wurde zusätzlich ans Backend angeschlossen; Rückmeldung des Nutzers: „das Scannen fühlt sich gut an". Die Punktetabelle oben und die Abschnitte 4 bis 7 wurden nicht einzeln ausgefüllt. |

Bei Gleichstand gilt React (ADR-0007).
