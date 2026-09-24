# ADR-0016: Oberfläche nach Apples Human Interface Guidelines

- Status: angenommen
- Datum: 2026-09-24

## Kontext

- Eine Prüfung der Web-Oberfläche gegen Apples Human Interface Guidelines (HIG) ergab 24 Befunde: 4 hoch, 9 mittel, 11 niedrig. Die wichtigsten: Die App folgt nicht der iOS-Textgröße, der Produktname hat in der Vorrat-Zeile zu wenig Platz, Weiß auf dem Primärgrün erreicht nur 3,67:1 statt 4,5:1, es gibt keinen Druckzustand und keinen Dark Mode.
- StashBert ist eine Web-App (PWA). Haptik, SF Symbols als Systemschrift, Liquid Glass, eine direkte Steuerung der Statusleiste und dunkle Icon-Varianten sind im Web nicht oder nur angenähert möglich.
- Der Nutzer hat entschieden: kein Dark Mode, alle übrigen Pakete umsetzen.

## Entscheidung

- **Kein Dark Mode in M1.** Die App bleibt hell. `color-scheme` wird ausdrücklich auf `light` gesetzt, damit Formularelemente und Scrollleisten nicht dunkel werden.
- **Farbrollen statt einzelner Tailwind-Farben:** Akzent (Primärgrün mit mindestens 4,5:1 zu Weiß), Entnehmen (Blau), Vorgemerkt (Amber), Warnung (Gelb), Fehler und Zerstörendes (Rot) als Variablen in `web/src/index.css` (`@theme`). Jede Rolle hat genau eine Bedeutung. Meldungen tragen zusätzlich ein Symbol, damit Farbe nie das einzige Signal ist.
- **Druckzustand für alle Knöpfe und tippbaren Zeilen.**
- **Dynamic Type:** Die Wurzelschrift folgt auf iOS der Systemtextgröße über `font: -apple-system-body` (nicht standardisiert, nur Safari; greift nach dem Neuladen). Tab-Leiste und Modus-Schalter behalten feste Größen mit Obergrenze. Layouts werden so gebaut, dass sie bis etwa 200 % Schrift nutzbar bleiben.
- **Keine kurzen Zeitgrenzen für Wichtiges:** Fehlermeldungen bleiben bis zur nächsten Aktion; die Ergebniskarte mit Rückgängig bleibt bis zum nächsten Scan oder bis zum Schließen (ersetzt die 10 s aus F09).
- **Audio:** Die Scan-Töne nutzen weiter `navigator.audioSession.type = "playback"` (architecture.md 4.3). Ob `"transient"` oder `"ambient"` im Alltag besser passt (Stummschalter, laufende Musik), prüft der Nutzer auf dem Gerät; bis dahin keine Änderung.
- **App-Icon:** neues flächiges Motiv (Regalbrett mit unterschiedlich hohen Gläsern bzw. Dosen) statt der drei Balken, die wie ein Menü-Symbol wirken.

## Konsequenzen

- Sechs kleine Tasks F19 bis F24 im Plan (Phase 1g).
- Neue Oberflächen nutzen nur die Farbrollen und den gemeinsamen Druckzustand (AGENTS.md, Frontend-Regeln).
- Screenshots in 320, 390 und 402 px Breite und mit großer Schrift gehören zur Abnahme jedes Tasks, der Layout ändert. Die Grundschrift dieser Screenshots ist 17 px, die Standardgröße von iOS, nicht die 16 px eines Desktop-Browsers.

## Alternativen

- **Dark Mode jetzt mitbauen:** vom Nutzer abgelehnt; die Farbrollen machen ihn später einfacher.
- **Eigene Schriftgrößen-Einstellung in der App:** die HIG empfiehlt, der Systemeinstellung zu folgen.
