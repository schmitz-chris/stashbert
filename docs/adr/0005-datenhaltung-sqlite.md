# ADR-0005: Datenhaltung mit SQLite, sqlc und goose

- Status: angenommen
- Datum: 2026-09-23

## Kontext

- Ein Haushalt, bis ca. 1000 Produkte, wenige Schreibzugriffe.
- Backup soll trivial sein. Ein späterer Wechsel zu PostgreSQL soll möglich bleiben.

## Entscheidung

- **Treiber:** SQLite über `modernc.org/sqlite` (reines Go, `CGO_ENABLED=0`).
- **Pragmas:** `journal_mode=WAL`, `foreign_keys=ON`, `busy_timeout=5000`, `synchronous=NORMAL`.
- **Verbindungen:** höchstens 4 (`SetMaxOpenConns(4)`), Transaktionen mit `_txlock=immediate`. Schreibende Transaktionen reihen sich so geordnet ein. Wer versehentlich innerhalb einer Transaktion über `*sql.DB` statt über die Transaktion schreibt, bekommt nach 5 s einen Fehler statt eines Hängers.
- **Tabellen:** STRICT.
- **Datenzugriff:** mit **sqlc** v1.31.1.
  - Die Abfragen stehen als SQL in `internal/store/queries/`, der generierte Code wird eingecheckt.
  - Das Werkzeug wird als Go-Tool-Abhängigkeit geführt: `go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`.
- **Migrationen:** mit **goose v3**, eingebettet per `embed`, beim Start ausgeführt. Dateien heißen `internal/store/migrations/NNNN_name.sql`.
- **SQL portabel halten:** keine SQLite-Spezialfunktionen, wenn es eine Standard-Variante gibt.

## Konsequenzen

- Eine Datei `DATA_DIR/stashbert.db` enthält alle Daten.
- Backup läuft über `VACUUM INTO`, siehe `architecture.md`, 9.3.

## Alternativen

- **PostgreSQL:** zusätzlicher Dienst, Major-Upgrades, aufwendigeres Backup.
- **GORM, ent:** viel Magie, weniger explizit.
- **mattn/go-sqlite3:** braucht cgo.
- **golang-migrate, Atlas:** mehr Werkzeug als nötig.
