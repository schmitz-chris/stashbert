# ADR-0012: Agentengetriebene Entwicklung mit kleinteiligem Plan

- Status: angenommen
- Datum: 2026-09-23

## Kontext

- Der Code wird überwiegend von LLM-Agenten geschrieben.
- Die Risiken dabei: Halluzinationen, kreative Zusätze und Scope Creep.

## Entscheidung

- **Anweisungen:** `AGENTS.md` im Repo-Wurzelverzeichnis ist die einzige Anweisungsdatei für Agenten. `CLAUDE.md` importiert sie nur.
- **Tasks:** Gearbeitet wird ausschließlich an Tasks aus `docs/plan.md`, ein Task pro Sitzung und Commit.
  - Jeder Task hat Abhängigkeiten, Referenzen, Umfang (mit den betroffenen Dateien bzw. Paketen), Nicht-Umfang und Abnahmekriterien.
  - Welche Dateien zusätzlich immer geändert werden dürfen, steht in `AGENTS.md`.
  - Manuelle Prüfungen sind mit „(Nutzer)" markiert und werden vom Menschen abgenommen.
- **Prüfung:** `make check` muss grün sein. Er umfasst Formatierung, `go vet`, Tests und, sobald vorhanden, die Frontend-Prüfungen. Die CI prüft ab Task B01 zusätzlich mit `make generate && git diff --exit-code`, dass generierte Dateien aktuell sind.
- **Abhängigkeiten:** nur aus der Liste in `AGENTS.md` oder aus dem Task.
- **Unklarheiten:** Anhalten und fragen, nicht raten.
- **Doku:** Bibliotheks-APIs werden in der aktuellen Doku nachgeschlagen (Context7, offizielle Doku, `llms.txt`), nicht aus dem Gedächtnis übernommen.
- **Plan-Review:** Ein unabhängiger Agent ohne Vorwissen prüft den Plan, bevor die Umsetzung beginnt.

## Konsequenzen

- Der Plan ist die Steuerung. Was nicht im Plan steht, wird nicht gebaut.

## Alternativen

- Freies Arbeiten mit großen Aufträgen: schneller am Anfang, aber anfällig für Drift und Scope Creep.
