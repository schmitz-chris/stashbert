# AGENTS.md

Anweisungen für KI-Agenten, die an StashBert arbeiten. Diese Datei ist verbindlich (ADR-0012).

## Projekt in einem Absatz

StashBert ist ein selbst gehostetes Vorratsinventar für einen Haushalt: Produkte mit Barcodes, Bestand, Sollbestand und einer berechneten Einkaufsliste. Das Go-Backend stellt eine REST-API bereit (Vertrag: `api/openapi.yaml`) und liefert eine eingebettete Web-Oberfläche (PWA) aus, die auf dem iPhone Barcodes per Kamera scannt. Aktueller Meilenstein: **M1, eine Erprobungsversion für zwei Personen im Homelab.**

## Verbindliche Dokumente

Lies vor jeder Arbeit in dieser Reihenfolge:

1. **Deinen Task** in [docs/plan.md](docs/plan.md). Er bestimmt, was zu tun ist und was nicht.
2. Die im Task genannten **ADRs** in [docs/adr/](docs/adr/).
3. Die im Task genannten Abschnitte in [docs/architecture.md](docs/architecture.md).
4. Fakten und Quellen, nur bei Bedarf: [docs/research.md](docs/research.md). Stack-Aussagen dort sind teilweise überholt; bei Widerspruch gelten architecture.md und die ADRs.

## Arbeitsregeln (nicht verhandelbar)

1. **Ein Task, nicht mehr.** Bearbeite genau den einen Task aus `docs/plan.md`, den du bekommen hast. Baue nichts, was dort nicht steht:
   - keine zusätzlichen Endpunkte, Felder, Ansichten, Optionen oder Konfigurationswerte,
   - keine „Verbesserungen" nebenbei,
   - keine Änderungen außerhalb der im Umfang genannten Dateien und Pakete und der Liste „Immer erlaubte Dateien".
2. **Nicht raten.** Wenn etwas unklar, widersprüchlich oder blockiert ist: Anhalten, das Problem konkret beschreiben und fragen. Erfinde keine Anforderungen, Namen, Felder oder Werte.
3. **Keine neuen Abhängigkeiten**, außer sie stehen in der Tabelle „Erlaubte Abhängigkeiten" oder ausdrücklich im Task.
4. **Contract-first.** API-Änderungen nur in dieser Reihenfolge:
   1. `api/openapi.yaml` ändern,
   2. `make generate`,
   3. implementieren,
   4. testen.
   Die Übersicht in `docs/architecture.md`, Kapitel 6, ist die Vorgabe für die Spec.
5. **Generierte Dateien nie von Hand ändern.** Das betrifft `internal/api/gen.go`, den sqlc-Code in `internal/store/db/` und `web/src/lib/api/schema.d.ts`. Sie werden eingecheckt.
6. **Bibliotheks-APIs nicht aus dem Gedächtnis.** Schlage sie in der aktuellen Doku nach (Context7, offizielle Doku oder `llms.txt`), passend zur Version in `go.mod` bzw. `web/package.json`. Die Quellen der gepinnten Version sind die genaueste Referenz: Go-Module unter `$(go env GOMODCACHE)/<modul>@<version>`, npm-Pakete unter `node_modules/`. Context7 zeigt oft den neuesten Stand statt der gepinnten Version.
7. **Tests gehören zum Task.** Jedes Abnahmekriterium ist durch einen automatischen Test abgedeckt, sofern der Task nichts anderes sagt. Go-Tests nur mit der Standardbibliothek (`testing`, `net/http/httptest`), kein testify.
8. **Fertig heißt:** `make check` ist grün und alle automatischen Abnahmekriterien sind erfüllt. Gibt es Kriterien mit „(Nutzer)", setzt du den Status auf `wartet auf Nutzer`, sonst auf `erledigt`. Dann genau ein Commit. Die mit „(Nutzer)" markierten Prüfungen führst du nicht selbst durch und behauptest sie nicht.
9. **Zusammenbau nur an einer Stelle.** Die Handler-Kette entsteht ausschließlich in `internal/app.NewHandler`. `cmd/stashbert/main.go` verdrahtet nur. Fehler aus Handlern folgen dem Muster in `docs/architecture.md`, 4.4.
10. **Parallel arbeiten** nur, wenn zwei Tasks keine gemeinsamen Dateien ändern (`api/openapi.yaml`, `internal/app`, `main.go` und `docs/plan.md` sind fast immer gemeinsam). Im Zweifel nacheinander. Commits bleiben lokal; gepusht wird nach Absprache mit dem Nutzer.

## Immer erlaubte Dateien

Zusätzlich zu den im Task genannten Dateien darfst du ändern, soweit der Task es braucht:

- `api/openapi.yaml` (nur Operationen und Schemas, die der Task nennt)
- generierte Dateien, ausschließlich über `make generate`
- `internal/api/server.go` (`ServerDeps`, Konstruktor) und `internal/api/handlers_*.go`
- `internal/app/*.go` (Verdrahtung neuer Dienste und Middleware)
- `cmd/stashbert/main.go` (nur Verdrahtung)
- Testdateien (`*_test.go`, `testdata/`, `web/**/*.test.ts`)
- `go.mod` und `go.sum` sowie `web/package.json` und `web/package-lock.json`, nur für erlaubte Abhängigkeiten
- `docs/plan.md`, nur die Statuszeile des eigenen Tasks

Ausnahme für die Größenregel: Die Wegwerf-Prototypen in `poc/` (P0-2, P0-3) dürfen größer als 400 Zeilen sein.

## Befehle

Diese Befehle gibt es ab Task P0-1:

| Befehl | Wirkung |
|---|---|
| `make generate` | oapi-codegen, sqlc und API-Typen für das Frontend erzeugen |
| `make check` | Formatierung prüfen (`gofmt`), `go vet`, Go-Tests, Frontend-Prüfungen (sobald `web/` existiert). Die CI prüft zusätzlich, dass die generierten Dateien aktuell sind |
| `make test` | nur die Tests |
| `make run` | Server lokal starten (`DATA_DIR=./.data`, `COOKIE_SECURE=false`) |
| `make build` | Web-Oberfläche bauen und Go-Binary nach `bin/stashbert` erzeugen |

`make` auf macOS ist GNU Make 3.81. Nutze keine Features neuerer Make-Versionen.

## Stack und Versionen

| Bereich | Festlegung |
|---|---|
| Go | 1.27 (`go 1.27` in `go.mod`), Modul `github.com/schmitz-chris/stashbert` |
| API-Vertrag | OpenAPI 3.1 in `api/openapi.yaml` |
| Servercode | oapi-codegen v2 (`std-http-server`, `strict-server`) als Go-Tool: `go tool oapi-codegen` |
| Router | `net/http` der Standardbibliothek |
| Datenbank | SQLite über `modernc.org/sqlite`, Treibername `sqlite` |
| SQL-Code | sqlc v1.31.1 als Go-Tool: `go tool sqlc` |
| Migrationen | goose v3, eingebettet |
| Logging | `log/slog`, JSON auf stdout |
| Frontend | TypeScript, Vite, Tailwind CSS 4 (CSS-first, **keine** `tailwind.config.js`). Framework nach ADR-0007; bis zur Entscheidung keine Framework-Annahmen im Hauptprojekt |
| API-Client (Frontend) | `openapi-typescript` + `openapi-fetch` |
| Barcode (Frontend) | `barcode-detector` v3, Import **nur** `barcode-detector/ponyfill`; WASM selbst gehostet |

## Erlaubte Abhängigkeiten

**Go:**

- `github.com/oapi-codegen/oapi-codegen/v2` (Tool)
- `github.com/oapi-codegen/runtime`
- `github.com/oapi-codegen/nethttp-middleware`
- `github.com/oapi-codegen/nullable` (für PATCH-Felder, Option `nullable-type`)
- `github.com/getkin/kin-openapi` (von der Middleware benötigt)
- `github.com/sqlc-dev/sqlc` (Tool)
- `github.com/pressly/goose/v3`
- `modernc.org/sqlite`
- `github.com/google/uuid`
- `golang.org/x/crypto` (nur `argon2`)
- `golang.org/x/time` (nur `rate`)

**Frontend (npm):**

- `vite`, `typescript`
- `tailwindcss` mit `@tailwindcss/vite`
- `openapi-typescript`, `openapi-fetch`
- `barcode-detector` (bringt `zxing-wasm` als eigene, gepinnte Abhängigkeit mit; `zxing-wasm` **nicht** direkt installieren)
- `vite-plugin-pwa` bzw. die in P0-6 festgelegte offizielle PWA-Integration des Frameworks (bei SvelteKit `@vite-pwa/sveltekit`)
- das Framework nach ADR-0007 mit seinen offiziellen Vite-Plugins
- die im jeweiligen Task genannten Test- und Lint-Werkzeuge

Alles andere braucht einen ausdrücklichen Task oder ein ADR.

## Repository-Struktur (Ziel)

```
api/openapi.yaml               API-Vertrag (Quelle der Wahrheit)
cmd/stashbert/                 main
internal/api/                  gen.go (generiert), Handler
internal/httpx/                Middleware, Problem Details
internal/store/                DB öffnen, migrations/, queries/, db/ (sqlc, generiert)
internal/domain/               Fachlogik
internal/gtin/                 Barcode-Normalisierung
internal/lookup/               Open Food Facts, Hintergrund-Jobs
internal/auth/                 Passwort, Sitzungen, Mitglieder
internal/events/               Ereignis-Interface (M1: No-op)
internal/backup/               Backups
internal/config/               Umgebungsvariablen
internal/webui/                go:embed der Web-Oberfläche; dist/ wird von make build befüllt (im Repo nur .gitkeep)
web/                           Frontend-Projekt, Build nach web/dist
poc/                           Wegwerf-Prototypen aus Phase 0, nicht Teil von make check
deploy/                        compose.yaml
docs/                          Architektur, ADRs, Plan, Recherche
```

## Konventionen

**Go:**

- Standardbibliothek zuerst.
- Fehler mit `fmt.Errorf("…: %w", err)` einpacken. Kein `panic` außer in `main` beim Start.
- `context.Context` als erster Parameter bei allem, was I/O macht.
- Kein globaler Zustand außer in `main`. Abhängigkeiten werden per Konstruktor übergeben.
- Zeit immer UTC, gespeichert im festen Format `2006-01-02T15:04:05.000Z` (Hilfsfunktion in `internal/store`). IDs sind UUIDv7 als String in Kleinschreibung.
- SQL nur in `internal/store/queries/*.sql` (sqlc). Ausnahmen: Migrationen, PRAGMA-Abfragen, `VACUUM INTO` in `internal/backup`, direkte Inserts in Tests.
- Schreibende Abläufe mit mehreren Statements laufen in einer Transaktion. **Innerhalb einer Transaktion nur über die Transaktion zugreifen** (`queries.WithTx(tx)`), nie über `*sql.DB`.
- Tests mit Datenbank nutzen eine temporäre Datei in `t.TempDir()`, kein `:memory:` (mehrere Verbindungen). Tests, die blockieren könnten, bekommen einen Kontext mit Deadline.
- Texteingaben (Namen) werden vor der Längenprüfung getrimmt.

**API:**

- Pfade im Plural, JSON-Felder in `snake_case`.
- Fehler als RFC 9457 mit `code` aus `architecture.md`, 6.4. Keine neuen Codes ohne Task.
- 201 bei neu angelegten Ressourcen, `Location` nur, wenn es einen `GET`-Endpunkt dafür gibt (`architecture.md`, 6.1).

**Frontend:**

- UI-Texte auf Deutsch, Code und Bezeichner auf Englisch.
- Logik, die testbar sein soll (Filter, Zuordnungen, Reducer), steht in reinen Funktionen außerhalb der Komponenten.
- API-Zugriff nur über den generierten Client, kein direktes `fetch` auf `/api`.
- Mobile first, Touch-Ziele mindestens 44 × 44 px.

**Sprache und Stil:**

- Code, Bezeichner und Commit-Messages auf Englisch. Dokumentation und UI-Texte auf Deutsch.
- In Dokumentation, Kommentaren und Commit-Messages **keine Gedankenstriche** (weder Em- noch En-Dash). Stattdessen Komma, Doppelpunkt, Klammern oder einen neuen Satz.

**Commits:**

- Ein Commit pro Task. Message: `<Task-ID>: <kurze Beschreibung im Imperativ>`, z. B. `B07: add session login and logout`.
- Keine `Co-Authored-By`-Zeilen und keine „Generated with"-Hinweise.
- Die Git-Identität ist lokal konfiguriert und wird nicht geändert.
- Nie `--no-verify`.

## Sicherheit

- Keine Geheimnisse im Repository. `.env` und `.data/` stehen in `.gitignore`.
- Passwörter, Tokens und Cookies nie loggen.
- Bilder nur von den Hosts laden, die in `architecture.md`, 7.3, erlaubt sind.
