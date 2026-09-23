# ADR-0001: Architekturentscheidungen als ADRs festhalten

- Status: angenommen
- Datum: 2026-09-23

## Kontext

Entscheidungen wurden bisher im Chat besprochen und in einem großen Dokument mehrfach umgeschrieben. Agenten, die den Code schreiben, brauchen kurze, stabile und eindeutige Entscheidungen.

## Entscheidung

- Jede architekturrelevante Entscheidung bekommt ein nummeriertes ADR in `docs/adr/`.
- Gliederung: Status, Kontext, Entscheidung, Konsequenzen, Alternativen.
- Ein angenommenes ADR wird nicht mehr inhaltlich geändert, nur noch der Status. Eine Änderung braucht ein neues ADR, das das alte ersetzt („ersetzt durch ADR-00xx").
- `docs/architecture.md` beschreibt den aktuellen Stand, `docs/research.md` enthält Fakten und Quellen.

## Konsequenzen

- Neue Abhängigkeiten, neue Technologien und Abweichungen von der Architektur brauchen vorher ein ADR.
- Agenten lesen die im Task genannten ADRs, bevor sie arbeiten.

## Alternativen

- Nur ein großes Architektur-Dokument: wird unübersichtlich und veraltet unbemerkt.
