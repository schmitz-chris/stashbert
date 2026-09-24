# M1: Inbetriebnahme und Abnahme

| | |
|---|---|
| Status | Vorlage aus R05, auszufüllen vom Nutzer |
| Grundlagen | [plan.md](plan.md) (R05 und alle Kriterien mit „(Nutzer)"), [betrieb.md](betrieb.md) |

Dieses Dokument sammelt alles, was nur der Nutzer prüfen kann. Ergebnisse bitte direkt in die Tabellen eintragen: `ok`, `nicht ok` oder eine kurze Beobachtung.

## 1. Inbetriebnahme im LXC

Ablauf nach [betrieb.md](betrieb.md), Abschnitte 2 bis 6.

| Schritt | Ergebnis | Datum | Notiz |
|---|---|---|---|
| Debian-13-LXC angelegt (unprivilegiert) | | | |
| `make release` auf dem Mac, Prüfsumme im Container ok | | | |
| `install.sh` führt zu einem laufenden Dienst (L02) | | | |
| `OFF_CONTACT` gesetzt, Dienst neu gestartet | | | |
| Zweiter Aufruf von `install.sh` mit neuem Binary aktualisiert, Env-Datei bleibt (L02) | | | |
| Reverse Proxy mit gültigem Zertifikat eingerichtet | | | |
| Anleitung vollständig und richtig (L03) | | | |

## 2. Abnahmekriterien M1 (R05)

| Nr. | Kriterium | Ergebnis | Datum | Notiz |
|---|---|---|---|---|
| 1 | Bekanntes Produkt entnehmen: vom Erkennen bis „3 → 2" unter 500 ms im WLAN; danach ohne Tap scanbereit | | | |
| 2 | 10 verschiedene bekannte Produkte nacheinander in unter 30 s | | | |
| 3 | Unbekanntes Produkt mit OFF-Treffer: keine Pflichteingabe, Name gesetzt, das Bild erscheint nachträglich | | | |
| 4 | Ohne Internet: Platzhalter, Bestand +1; nach dem Setzen des Solls über die Chips und der Rückkehr des Internets wird das Produkt trotzdem ergänzt | | | |
| 5 | Soll 5 und Ist 2 ergeben auf der Einkaufsseite „3 × Kidneybohnen" | | | |
| 6 | Ein Backup lässt sich zurückspielen (R04 ist grün; zusätzlich einmal manuell nach betrieb.md) | | | |
| 7 | Installation auf dem Home-Bildschirm beider iPhones | | | |
| 8 | Derselbe Code bleibt im Bild und wird nur einmal gebucht; [+1] und [Rückgängig] funktionieren | | | |
| 9 | Nach 2 bis 3 Wochen Nutzung: Liste und Regal verglichen, Ergebnis notiert (Kernfrage der Erprobung) | | | |

## 3. Offene Nutzer-Prüfungen aus dem Plan

| Task | Prüfung | Ergebnis | Notiz |
|---|---|---|---|
| F01 | Im Browser meldet die Konsole keine CSP-Verletzung | | |
| F05 | Vorrat: Bedienung auf dem iPhone | | |
| F06a | Produktseite: Formular, „Passt so", Löschen | | |
| F06c | Produktseite: Bestand setzen, Verlauf, Zusammenführen | | |
| F08 | Auf beiden iPhones 10 bekannte Produkte nacheinander ohne Tap | | |
| F09 | Ergebniskarte: [+1] und [Rückgängig] | | |
| F10 | Unbekanntes Produkt scannen, Soll wählen, zusammenführen | | |
| F13b | Installation auf dem Home-Bildschirm beider iPhones, Start im Standalone-Modus; Update-Hinweis nach einem neuen Build | | |
| F15 | Scan-Modus „Einkaufen": bekanntes und unbekanntes Produkt vormerken, Rückgängig; der Bestand ändert sich nie | | |
| F17 | Falsches Bild durch ein eigenes Foto ersetzen (kommt es aufrecht an?) und ein Bild entfernen | | |
| P0-2, P0-3 | Prototypen: keine Anfragen an fremde Hosts (nur noch der Vollständigkeit halber) | | |
| R02 | Optional: Docker-Anleitung führt zu einer laufenden Instanz | | |
| R03 | Optional: Nach dem Push ist die CI grün; ein Test-Tag `v…` erzeugt das Image in GHCR | | |

## 4. Offene Entscheidungen und Ideen

Keine davon ist Teil von M1. Sie stehen hier, damit sie nicht verloren gehen.

- **App-Icon:** Die drei Balken wirken wie ein Menü-Symbol. Mögliche Motive: Regal mit Seitenwänden oder Regal mit Dosen.
- **Selbstgemachte Produkte:** „Produkt anlegen" ohne Barcode im Vorrat; später eigene Barcode-Etiketten mit lokalen Codes (Präfix 2). Wiederverwendete Gläser: alten Barcode überkleben oder am Produkt entfernen.
- **Lange Produktnamen** von Open Food Facts brechen im Vorrat über mehrere Zeilen um; von Hand kürzen oder auf zwei Zeilen begrenzen.
- **M2:** MQTT, Home Assistant und Bring! (research.md, Kapitel 10 und 11).
