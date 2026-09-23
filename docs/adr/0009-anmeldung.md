# ADR-0009: Anmeldung mit Haushaltspasswort, OIDC später

- Status: angenommen
- Datum: 2026-09-23

## Kontext

- Die Erprobung läuft für zwei Personen im Homelab. Ein Identity Provider existiert nicht.
- Später sind Google- oder Pocket-ID-Anmeldung denkbar.

## Entscheidung

- **Haushaltspasswort:** eines für den ganzen Haushalt, gesetzt bei der Ersteinrichtung (`POST /api/v1/setup`).
  - Gespeichert als argon2id (m = 19 MiB, t = 2, p = 1) mit `golang.org/x/crypto/argon2`.
- **Sitzungen serverseitig:**
  - Zufälliges Token im Cookie `stashbert_session` (HttpOnly, SameSite=Lax, Secure).
  - In der DB steht nur der SHA-256-Hash. Gültig 365 Tage, bei Nutzung verlängert.
- **Haushaltsmitglieder:** Namen, ohne eigenes Passwort. Jede Sitzung wählt „Wer bist du?", und Buchungen speichern das Mitglied.
- **Keine Rollen.**
- **Später:**
  - API-Tokens für Maschinen (M2).
  - Generisches OIDC über `github.com/coreos/go-oidc/v3` als zusätzlicher Login-Weg. Die API bleibt dabei unverändert.

## Konsequenzen

- Handler sehen nur die Sitzung, nicht den Login-Weg.
- Details stehen in `architecture.md`, Kapitel 8.

## Alternativen

- **Keine Anmeldung:** Jedes Gerät im WLAN könnte buchen.
- **Konten pro Person:** mehr Aufwand ohne Nutzen in der Erprobung.
- **Google jetzt:** braucht ein Cloud-Projekt, eine HTTPS-Domain und Internet beim Login.
