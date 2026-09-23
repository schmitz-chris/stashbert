# ADR-0013: Keine Anmeldung in M1

- Status: angenommen
- Datum: 2026-09-23
- Ersetzt: ADR-0009

## Kontext

StashBert wird in M1 von zwei Personen im eigenen Heimnetz gemeinsam genutzt. Berechtigungen und die Frage, wer gebucht hat, spielen für die Erprobung keine Rolle (Entscheidung des Nutzers vom 23.09.2026). Das Haushaltspasswort mit Sitzungen, Login-Bremse und Mitgliederauswahl (ADR-0009) wäre Aufwand ohne Nutzen.

## Entscheidung

- M1 hat **keine Anmeldung**: kein Passwort, keine Sitzungen, keine Mitglieder, keine Zuordnung von Buchungen zu Personen.
- Der Zugriffsschutz ist das Netz: StashBert ist nur im Heimnetz erreichbar, später von unterwegs nur per VPN.
- Gegen Webseiten, die jemand im Heimnetz aufruft: keine CORS-Header, Request-Bodies nur als `application/json`.
- Die Endpunkte werden so gebaut, dass eine spätere Anmeldung nichts an ihnen ändert.

## Konsequenzen

- Jedes Gerät im Heimnetz kann lesen und buchen. Das ist akzeptiert.
- Der schon umgesetzte Task B07 (Setup-API mit Passwort) wird entfernt, die Tasks B08a, B08b, B09 und B10 entfallen. Die Tabelle `settings` bleibt für spätere Einstellungen.
- Später nachrüstbar, ohne API-Änderung: Forward-Auth am Reverse Proxy (z. B. mit einem Identity Provider) oder OIDC in StashBert; für Geräte (ESP32, Home Assistant) API-Tokens.

## Alternativen

- **Haushaltspasswort (ADR-0009):** schützt vor Gästen im WLAN, kostet aber fünf Tasks und macht Integrationen umständlicher.
- **Anmeldung am Reverse Proxy schon jetzt:** möglich, aber nicht nötig; bleibt als spätere Option.
