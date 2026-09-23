# ADR-0002: Backend in Go

- Status: angenommen
- Datum: 2026-09-23

## Kontext

- Die API ist der wichtigste Teil. Der Code wird überwiegend von LLM-Agenten geschrieben.
- Der Nutzer schließt Java und Python aus. Rust ist beliebt, aber langsamer in der Entwicklung. TypeScript ist möglich.

## Entscheidung

- Backend in **Go**, aktuell Version 1.27 (`go 1.27` in `go.mod`), Modulpfad `github.com/schmitz-chris/stashbert`.
- Zuerst die Standardbibliothek. Zusätzliche Module nur nach ADR.

## Begründung

- Go ist seit dem Go-1-Kompatibilitätsversprechen stabil. Das Wissen von LLMs veraltet dadurch kaum.
- Ein Formatierungsstil (`gofmt`), wenig Magie, ein sehr schneller Compiler. Agenten bekommen so schnelle, eindeutige Rückmeldungen.
- Es entsteht ein statisches Binary ohne Laufzeit-Abhängigkeiten.
- Die Werkzeuge für OpenAPI (oapi-codegen) und SQL (sqlc) sind ausgereift.

## Konsequenzen

- Zwei Sprachen: Go im Backend, TypeScript im Frontend. Die Brücke ist der OpenAPI-Vertrag (ADR-0003).

## Alternativen

- **Rust** (axum, utoipa, sqlx): maximale Korrektheit, aber steilere Lernkurve und langsamere Entwicklung.
- **TypeScript/Node.js**: eine Sprache für alles, aber npm-Churn und Validierung zur Laufzeit.
- **Python, Java:** vom Nutzer ausgeschlossen.
