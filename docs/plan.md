# StashBert: Umsetzungsplan M1

| | |
|---|---|
| Stand | 23.09.2026, Revision 3 (nach zwei unabhängigen Prüfungen) |
| Ziel | M1, eine Erprobungsversion für zwei Personen im Homelab ([architecture.md](architecture.md), Kapitel 1) |
| Regeln | [AGENTS.md](../AGENTS.md), [ADR-0012](adr/0012-agentengetriebene-entwicklung.md) |

Dieser Plan steuert die Umsetzung. **Was nicht in einem Task steht, wird nicht gebaut.**

## Ablauf pro Task

1. `AGENTS.md` lesen.
2. Den Task lesen und prüfen, ob alle Tasks unter „Abhängig von" auf `erledigt` oder `wartet auf Nutzer` stehen. Sonst anhalten.
3. Die unter „Referenzen" genannten Abschnitte lesen.
4. Bei API-Änderungen: zuerst `api/openapi.yaml`, dann `make generate`.
5. Tests für die automatischen Abnahmekriterien schreiben, dann implementieren.
6. `make check` muss grün sein.
7. Status setzen (`erledigt`, oder `wartet auf Nutzer`, wenn Kriterien mit „(Nutzer)" offen sind) und genau einen Commit anlegen: `<Task-ID>: <Beschreibung>`.

Wird ein Task größer als etwa 400 geänderte Zeilen Produktivcode (ohne Tests, ohne generierten Code, ohne `poc/`), oder sind Angaben unklar oder widersprüchlich: anhalten und melden, nicht raten und nicht eigenmächtig aufteilen.

**Status-Werte:**

- `offen`
- `in Arbeit`
- `wartet auf Nutzer`
- `erledigt`
- `blockiert (Grund)`

**Wer arbeitet:** Standard ist „Agent". Tasks mit „Wer: Nutzer" und Kriterien mit „(Nutzer)" erledigt der Mensch.

**Muster für API-Tasks:**

- Handler kommen in `internal/api/handlers_<bereich>.go`, Fachlogik in `internal/domain`, SQL in `internal/store/queries/<bereich>.sql`.
- Verdrahtet wird in `internal/app`.
- Fehler nach architecture.md 4.4: in der Spec nur `default: Problem`, im Handler `*httpx.Error`.
- API-Tests laufen gegen `app.NewHandler` mit einer temporären Datenbank.

## Übersicht

| Phase | Tasks | Voraussetzung |
|---|---|---|
| 0: Fundament und Frontend-Vergleich | P0-1 bis P0-6 | keine |
| 1a: Backend | B01 bis B29 | P0-1 |
| 1b: Frontend | F01 bis F13b | P0-6 und die jeweils genannten B-Tasks |
| 1c: Auslieferung | R01 bis R05 | siehe Tasks |

Backend-Tasks können parallel zu P0-2 bis P0-6 laufen. Mehrere B-Tasks gleichzeitig nur nach AGENTS.md Regel 10.

---

## Phase 0: Fundament und Frontend-Vergleich

### P0-1: Repository-Grundgerüst

- **Status:** erledigt
- **Abhängig von:** keine
- **Referenzen:** AGENTS.md (Befehle, Struktur), ADR-0002
- **Umfang:**
  - `go.mod`: Modul `github.com/schmitz-chris/stashbert`, `go 1.27`, dazu die Direktiven `ignore ./web` und `ignore ./poc`, damit `./...` keine Go-Dateien aus `node_modules` findet.
  - `cmd/stashbert/main.go`: gibt `stashbert <version>` aus und endet mit Exit-Code 0. `version` ist eine Paketvariable mit Standardwert `dev`, die per `-ldflags "-X main.version=…"` gesetzt werden kann.
  - `Makefile` mit den Zielen aus AGENTS.md (nur GNU Make 3.81):
    - `generate`: `go generate ./...`
    - `check`: `gofmt -l` über die Paketverzeichnisse aus `go list -f '{{.Dir}}' ./...` muss leer sein (nicht `gofmt -l .`, das würde `node_modules` durchsuchen), dann `go vet ./...` und `go test ./...`
    - `test`: `go test ./...`
    - `run`: `DATA_DIR=./.data COOKIE_SECURE=false go run ./cmd/stashbert`
    - `build`: `go build -o bin/stashbert -ldflags "-X main.version=$(git describe --tags --always --dirty)" ./cmd/stashbert`
  - `.gitignore` mit mindestens diesen Einträgen:
    - `/bin/`, `/.data/`, `.env`
    - `web/node_modules/`, `web/dist/`, `web/public/zxing_reader.wasm`
    - `internal/webui/dist/*`, `!internal/webui/dist/.gitkeep`
    - `poc/*/node_modules/`, `poc/*/dist/`, `poc/*/build/`, `poc/*/public/zxing_reader.wasm`, `poc/*/public/beep.wav`
    - `**/.svelte-kit/`
  - `.github/workflows/ci.yml`: bei Push und Pull Request Go 1.27 einrichten und `make check` ausführen, mit `permissions: contents: read`. Die aktuellen Hauptversionen von `actions/checkout` und `actions/setup-go` vorher nachschlagen.
- **Nicht im Umfang:** HTTP-Server, Abhängigkeiten, Frontend, Dockerfile.
- **Abnahmekriterien:**
  1. `go run ./cmd/stashbert` gibt `stashbert dev` aus. (Seit B02 überholt: `main` startet den Server, die Version steht im Startlog.)
  2. `make check` ist grün.
  3. `make build` erzeugt `bin/stashbert`.
  4. (Nutzer) Nach dem Push läuft die CI grün (`gh run list`).

### Spezifikation der Scanner-Testseite (gilt für P0-2 und P0-3)

Beide Varianten setzen **exakt** diese Punkte um, nicht mehr:

1. **Aufbau:** eine einzige Seite, TypeScript, Tailwind CSS 4, ohne Backend und ohne API-Aufrufe. Der `<title>` ist „Scanner-Test S" bzw. „Scanner-Test R". Abhängigkeiten nur: Framework samt offizieller Adapter und Vite-Plugins (bei React zusätzlich `@types/react` und `@types/react-dom`), `vite`, `typescript`, `tailwindcss`, `@tailwindcss/vite`, `barcode-detector`. `zxing-wasm` **nicht** direkt installieren. Lint-Werkzeuge der Vorlage werden entfernt.
2. **Start:** Ein großer Button „Scannen starten" öffnet erst beim Tippen die Kamera, mit `getUserMedia({ video: { facingMode: "environment", width: { ideal: 1280 }, height: { ideal: 720 } } })`. Im selben Tap werden `navigator.audioSession.type = "playback"` (falls vorhanden) gesetzt und ein `AudioContext` erzeugt.
3. **Kamera-Auswahl:** Eine Auswahlliste „Kamera" zeigt die Einträge aus `enumerateDevices()` mit Typ `videoinput`. Die Auswahl wird in `localStorage` gespeichert und beim nächsten Start verwendet, dann mit `deviceId: { exact }` statt `facingMode`. Existiert die gespeicherte Kamera nicht mehr, wird die Auswahl gelöscht und mit den Vorgaben aus Punkt 2 gestartet. Beim Wechsel wird der alte Stream gestoppt, bevor ein neuer geöffnet wird.
4. **Zoom:** Ein Schieberegler „Zoom" erscheint nur, wenn `track.getCapabilities().zoom` existiert, und nutzt dessen min, max und step.
5. **Licht:** Ein Button „Licht" erscheint nur, wenn `track.getCapabilities().torch` `true` ist oder eine Liste mit `true` enthält.
6. **Dekodierung:**
   - Import aus `barcode-detector/ponyfill`, Formate `ean_13`, `ean_8`, `upc_a`.
   - Ein npm-Skript `copy:wasm` (nur Node, ohne Zusatzpakete) läuft vor `dev` und `build`:
     - Es löst `zxing-wasm` per `createRequire` relativ zu `barcode-detector` auf.
     - Es kopiert dessen `zxing_reader.wasm` nach `public/`.
     - Es vergleicht den SHA-256 der Datei mit dem vom Paket exportierten `ZXING_WASM_SHA256` und bricht bei Abweichung ab.
   - Eingebunden wird die Datei über `prepareZXingModule` mit `locateFile`. Der Aufruf muss **vor** dem ersten `new BarcodeDetector()` erfolgen, weil der Konstruktor das Modul sofort lädt. Es gibt keinen Abruf von jsDelivr.
7. **Leseschleife:**
   - Höchstens 10 Versuche pro Sekunde, nie zwei gleichzeitig.
   - Dekodiert wird nur ein waagerechter Streifen in der Bildmitte: 80 % der Breite und 30 % der Höhe, als Rahmen über dem Video eingezeichnet.
8. **Treffer:**
   - Derselbe Code innerhalb von 2000 ms wird ignoriert.
   - Sonst: den Code groß anzeigen, die Dauer des erfolgreichen `detect`-Aufrufs in ms und einen Zähler.
   - Eine Liste zeigt die letzten 10 Codes mit Uhrzeit.
9. **Rückmeldung:**
   - 300 ms grüne Fläche über dem Video und ein Ton mit 880 Hz für 120 ms über Web Audio.
   - Eine Checkbox „Ton über Audio-Element" spielt stattdessen `public/beep.wav` über ein `<audio>`-Element ab. Die Datei erzeugt ein npm-Skript mit Node ohne Zusatzpakete. Damit iOS das Element später ohne Tippen abspielt, wird es bei jedem Tippen auf Start, Fortsetzen und die Checkbox einmal stumm gestartet und sofort pausiert.
10. **Wach bleiben:** Während der Scanner läuft, wird ein Screen Wake Lock angefordert, falls verfügbar.
11. **Hintergrund:** Bei `visibilitychange` auf `hidden` werden alle Tracks gestoppt. Beim Zurückkehren erscheint ein Button „Tippen zum Fortsetzen", ohne automatischen Neustart.
12. **Statusbereich:** User-Agent, `display-mode` (`standalone` oder `browser`), Label der gewählten Kamera, Fehlermeldungen.
13. **Home-Bildschirm:** `manifest.webmanifest` (`display: standalone`, Name „Scanner-Test S" bzw. „Scanner-Test R") und ein `apple-touch-icon` (180 px). Das PNG wird per Node-Skript ohne Zusatzpakete als einfarbige Fläche erzeugt, oder es ist ein beliebiges vorhandenes PNG. Kein Service Worker.
14. **Auslieferung:** `npm run build`, dann `npm run preview -- --host 0.0.0.0 --port <Port>`, mit `preview.allowedHosts: true` in der Vite-Konfiguration, damit der Reverse Proxy mit eigenem Hostnamen zugreifen darf. Port 8081 für S, 8082 für R. Optional für Tests ohne Reverse Proxy: Liegen `.cert/key.pem` und `.cert/cert.pem` im Projekt (selbst signiert, per `openssl` erzeugt, von Git ausgeschlossen), liefert die Vorschau HTTPS aus (`preview.https`).

### P0-2: Scanner-Testseite, Variante S (Svelte)

- **Status:** wartet auf Nutzer
- **Abhängig von:** P0-1
- **Referenzen:** Spezifikation oben, ADR-0007, ADR-0008
- **Vorbereitung (Nutzer):** das offizielle Svelte-Plugin in Claude Code einrichten: `/plugin marketplace add sveltejs/ai-tools`, dann das Plugin `svelte` installieren.
- **Umfang:**
  - Projekt `poc/scanner-svelte/`: Svelte 5 + SvelteKit 2 im SPA-Modus (`adapter-static` mit Fallback-Seite, `ssr = false`), TypeScript, Tailwind CSS 4.
  - Die Spezifikation vollständig umsetzen.
  - `poc/scanner-svelte/NOTIZEN.md` mit den Werten für den Vergleich: Anzahl der Korrekturschleifen bis zum grünen Build, aufgetretene Fehler (z. B. Svelte-4-Syntax), Größe des Build-Ordners in KB.
- **Nicht im Umfang:** shadcn-svelte, Routing, Tests, Service Worker, alles außerhalb der Spezifikation.
- **Abnahmekriterien:**
  1. `npm run build` ist ohne Fehler.
  2. Die Vorschau läuft auf Port 8081: `curl -s -o /dev/null -w '%{http_code}' localhost:8081` liefert 200, und `curl -s localhost:8081` enthält `<title>Scanner-Test S</title>`.
  3. Alle 14 Punkte der Spezifikation sind in NOTIZEN.md abgehakt.
  4. (Nutzer) Im Browser werden keine Anfragen an fremde Hosts gestellt (Netzwerk-Tab).

### P0-3: Scanner-Testseite, Variante R (React)

- **Status:** wartet auf Nutzer
- **Abhängig von:** P0-1
- **Referenzen:** Spezifikation oben, ADR-0007, ADR-0008
- **Umfang:**
  - Projekt `poc/scanner-react/`: React 19 + Vite (Vorlage `react-ts`), TypeScript, Tailwind CSS 4. Die Scanner-Logik gehört in eigene Module außerhalb der Komponenten.
  - Die Spezifikation vollständig umsetzen.
  - `poc/scanner-react/NOTIZEN.md` wie bei P0-2.
- **Nicht im Umfang:** Router, TanStack Query, shadcn/ui, Tests, Service Worker.
- **Abnahmekriterien:** wie P0-2, mit Port 8082 und dem Titel „Scanner-Test R".

### P0-4: Protokollvorlage und Bereitstellungsanleitung

- **Status:** erledigt
- **Abhängig von:** P0-2, P0-3
- **Referenzen:** research.md, Kapitel 7 und 22 (POC-1 bis POC-3), ADR-0007
- **Umfang:** `docs/poc/p0-protokoll.md` mit:
  - Anleitung: beide Vorschauen starten und über den eigenen Reverse Proxy per HTTPS erreichbar machen (zwei Hostnamen, Ziel-Ports 8081 und 8082).
  - Pro Gerät (iPhone 15, iPhone 16 Pro) und pro Variante (S, R) eine Tabelle mit diesen Testfällen:
    1. 20 reale Vorratsartikel scannen: Treffer, Fehlversuche, Dauer bis Treffer (grob).
    2. Kamera-Auswahl und Zoom: Welche Einstellung liest am besten aus 5 bis 15 cm?
    3. Kamera-Rückfrage: Safari-Tab gegen Home-Bildschirm, je 3 Kaltstarts, Bildschirmsperre und App-Wechsel.
    4. Ton mit und ohne Stummschalter, Web Audio gegen Audio-Element, läuft Musik weiter?
    5. Licht, falls vorhanden.
    6. Fremd-Anfragen im Netzwerk-Tab.
  - Vergleichsbewertung S gegen R, je 1 bis 5 Punkte: Funktion auf beiden Geräten, Nacharbeit laut NOTIZEN.md, Verständlichkeit des Codes, Gefühl.
- **Nicht im Umfang:** Tests selbst durchführen.
- **Abnahmekriterien:** Die Vorlage enthält alle genannten Testfälle für beide Geräte und beide Varianten.

### P0-5: Test auf den iPhones

- **Status:** erledigt (verkürzt: Rückmeldung des Nutzers statt vollständigem Protokoll, siehe `docs/poc/p0-protokoll.md` 8.2)
- **Wer:** Nutzer
- **Abhängig von:** P0-4
- **Umfang:** Die Tests aus `docs/poc/p0-protokoll.md` durchführen und das Protokoll ausfüllen.
- **Abnahmekriterien:** Das Protokoll ist ausgefüllt und eingecheckt.

### P0-6: Frontend-Entscheidung festhalten

- **Status:** erledigt (React)
- **Wer:** Agent mit Nutzer
- **Abhängig von:** P0-5
- **Referenzen:** ADR-0007, `docs/poc/p0-protokoll.md`, architecture.md 8 (CSP)
- **Umfang:**
  - ADR-0007 auf „angenommen" setzen und die gewählte Variante samt Begründung aus dem Protokoll eintragen. Bei Gleichstand gilt React.
  - In `AGENTS.md` einen Abschnitt „Frontend-Regeln" ergänzen:
    - Framework, Version und die **exakte npm-Liste** der zusätzlich erlaubten Pakete (Router, Datenladen, Komponentenbibliothek, PWA-Integration, Test und Lint).
    - Konkrete Regeln, bei Svelte z. B. „nur Runes: `$state`, `$derived`, `$props`, `$effect`; kein `export let`, keine `$:`-Anweisungen".
    - Die Kamera-Einstellung, die sich im Protokoll bewährt hat.
  - In diesem Plan die Tasks F01 bis F13b um framework-spezifische Angaben ergänzen: Befehle zum Anlegen, Ordner, Routing-Datei. Dabei nur Angaben ergänzen, keinen Umfang hinzufügen.
  - Braucht das gewählte Framework Inline-Skripte oder andere Asset-Pfade als `/assets/`, das in F01 als Punkt festhalten (Anpassung von CSP und Cache-Headern).
- **Nicht im Umfang:** Frontend-Code.
- **Abnahmekriterien:**
  1. ADR-0007 ist angenommen.
  2. AGENTS.md hat Frontend-Regeln mit der npm-Liste.
  3. Die F-Tasks nennen das Framework.

---

## Phase 1a: Backend

### B01: OpenAPI-Grundgerüst, Codegenerierung, CI-Prüfung

- **Status:** erledigt
- **Abhängig von:** P0-1
- **Referenzen:** ADR-0003, architecture.md 4.4, 6.1, 6.2, 6.4
- **Umfang:**
  - `api/openapi.yaml`: `openapi: 3.1.0`, `info.title: StashBert API`, `info.version: 0.1.0`, `servers: [{url: /api/v1}]`.
    - Schema `Problem` mit `type`, `title`, `status`, `detail` und `code`. Pflichtfelder sind `title`, `status` und `code`.
    - Schema `Health` mit `status` (Enum `ok`) und `version`, beide Pflicht.
    - Pfad `GET /health`, `operationId: getHealth`: Antwort 200 `Health`, sonst `default` `Problem` als `application/problem+json`.
  - `internal/api/oapi-codegen.yaml`: Paket `api`, Ausgabe `gen.go`, `generate` mit `std-http-server`, `strict-server`, `models` und `embedded-spec`, dazu `output-options: nullable-type: true`.
  - `internal/api/generate.go` mit `//go:generate go tool oapi-codegen -config oapi-codegen.yaml ../../api/openapi.yaml`.
  - Tool-Abhängigkeit: `go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0`.
  - CI: **nach** `make check` ein zusätzlicher Schritt `make generate && git diff --exit-code && test -z "$(git status --porcelain)"` (erkennt auch neue, nicht eingecheckte generierte Dateien).
- **Nicht im Umfang:** Handler, Server, weitere Endpunkte.
- **Abnahmekriterien:**
  1. `make generate` erzeugt `internal/api/gen.go` mit `StrictServerInterface` und der Methode `GetHealth`.
  2. Ein Test lädt `api.GetSpec()` ohne Fehler.
  3. `make check` ist grün.

### B02: Konfiguration, Zusammenbau und Health

- **Status:** erledigt
- **Abhängig von:** B01
- **Referenzen:** architecture.md 4.1, 4.4, 9.2, AGENTS.md (Regel 9, Konventionen Go)
- **Umfang:**
  - **`internal/config`:** `Load(getenv func(string) string) (Config, error)` für alle Variablen aus architecture.md 9.2, mit Standardwerten. Prüfungen:
    - `PORT` 1 bis 65535
    - `BACKUP_KEEP` 1 bis 365
    - `LOG_LEVEL` aus den vier Werten
    - `COOKIE_SECURE` als Bool
    - `PUBLIC_URL`, wenn gesetzt: absolute `http`- oder `https`-URL ohne Pfad außer `/`
  - **`internal/api/server.go`:** `type ServerDeps struct{ Version string }`, `type Server struct`, `func NewServer(d ServerDeps) *Server`. `GetHealth` in `internal/api/handlers_health.go` liefert `{status: "ok", version}`.
  - **`internal/app/app.go`:** `type Deps struct{ Logger *slog.Logger; Version string }` und `func NewHandler(cfg config.Config, d Deps) (http.Handler, error)`.
    - Ein `http.ServeMux` mit `/api/v1/` über `http.StripPrefix("/api/v1", …)` auf den generierten std-http-Handler mit Strict-Handler.
    - Logging-Middleware aus `internal/httpx` außen herum.
  - **`internal/httpx/logging.go`:** Methode, Pfad, Status und Dauer in ms; `/api/v1/health` auf Level `debug`, alles andere auf `info`.
  - **`cmd/stashbert/main.go`:**
    - Konfiguration laden, `slog` mit JSON-Handler, `app.NewHandler`.
    - `http.Server` mit `ReadHeaderTimeout` 10 s, `ReadTimeout` 30 s, `WriteTimeout` 30 s, `IdleTimeout` 120 s.
    - Beenden bei SIGINT/SIGTERM mit 10 s Frist.
- **Nicht im Umfang:** Validierung, Problem Details, Datenbank.
- **Abnahmekriterien:**
  1. Tests für `config.Load`: Standardwerte und jeder ungültige Wert liefern einen Fehler.
  2. Ein `httptest`-Test über `app.NewHandler`: `GET /api/v1/health` liefert 200 und `{"status":"ok","version":"dev"}`.
  3. `make check` ist grün.

### B03: Problem Details, Validierung, Fehlerbehandlung

- **Status:** erledigt
- **Abhängig von:** B02
- **Referenzen:** architecture.md 4.4, 6.1, 6.4, ADR-0003
- **Umfang:**
  - **`internal/httpx/problem.go`:**
    - `WriteProblem(w http.ResponseWriter, status int, code, title, detail string)` mit `Content-Type: application/problem+json` und `type: about:blank`.
    - Typ `Error{Status int; Code, Title, Detail string}`, der das Interface `error` erfüllt.
    - Allgemeiner Konstruktor `NewError(status int, code, detail string) *Error`, der `Title` aus `http.StatusText(status)` setzt. Dazu die Kurzformen `BadRequest(detail)` (400 `invalid_request`) und `NotFound(detail)` (404 `not_found`).
    - Alle anderen Fälle nutzen `NewError`, z. B. `NewError(401, "invalid_credentials", …)` oder `NewError(404, "unknown_barcode", …)`. Spätere Tasks ändern `internal/httpx/problem.go` nicht.
  - **Handler-Kette in `internal/app`:** exakt nach architecture.md 4.4, ohne die Origin-Prüfung (die kommt in B09).
    - Validator mit `nethttp-middleware`: `OapiRequestValidatorWithOptions` mit `ErrorHandlerWithOpts`. Die Spec aus `api.GetSpec()`, vorher `Servers = nil` setzen.
    - `errors.Is(err, routers.ErrMethodNotAllowed)` ergibt 405 `method_not_allowed`, eine fehlende Route 404 `not_found`, sonst 400 `invalid_request` mit der Meldung als `detail`.
  - **Strict-Handler-Optionen:**
    - `RequestErrorHandlerFunc` ergibt 400 `invalid_request`.
    - `ResponseErrorHandlerFunc` wandelt `*httpx.Error` per `errors.As` um, sonst 500 `internal` mit Log.
    - Dazu der std-http-`ErrorHandlerFunc` mit 400 `invalid_request`.
  - **Recover-Middleware:** Ein Panic ergibt 500 `internal` und ein Log mit Stacktrace.
- **Nicht im Umfang:** Auth, Origin-Prüfung, neue Endpunkte.
- **Abnahmekriterien (Tests über `app.NewHandler`):**
  1. `GET /api/v1/unbekannt` liefert 404 mit `code: not_found` als `application/problem+json`.
  2. `POST /api/v1/health` liefert 405 mit `code: method_not_allowed`.
  3. Ein Test-Handler mit Panic liefert 500 `internal`.
  4. Ein Unit-Test: `ResponseErrorHandlerFunc` macht aus `httpx.NewError(409, "barcode_in_use", "…")` einen 409 Problem mit diesem `code`.
  5. `make check` ist grün.

### B04: SQLite öffnen und Migrationen

- **Status:** erledigt
- **Abhängig von:** B02
- **Referenzen:** ADR-0005, architecture.md 5 (`settings`, `members`, `sessions`, Zeitformat)
- **Umfang:**
  - **`internal/store/store.go`:**
    - `Open(ctx context.Context, path string) (*sql.DB, error)` mit `modernc.org/sqlite` (Treiber `sqlite`).
    - DSN mit `_pragma=journal_mode(WAL)`, `_pragma=foreign_keys(1)`, `_pragma=busy_timeout(5000)`, `_pragma=synchronous(NORMAL)` und `_txlock=immediate`.
    - `SetMaxOpenConns(4)`.
  - **`internal/store/time.go`:** `const TimeLayout = "2006-01-02T15:04:05.000Z"`, `FormatTime(time.Time) string` (immer UTC), `ParseTime(string) (time.Time, error)`.
  - **`internal/store/migrate.go`:**
    - `var Migrations fs.FS` (per `//go:embed migrations/*.sql`, als Unterverzeichnis `migrations`).
    - `Migrate(ctx context.Context, db *sql.DB, fsys fs.FS) error` mit dem goose-Provider (`goose.NewProvider(goose.DialectSQLite3, db, fsys)`). `main` übergibt `store.Migrations`, Tests können ein eigenes FS übergeben.
  - **`internal/store/migrations/0001_init.sql`:** Tabellen `settings`, `members` und `sessions` exakt nach architecture.md 5.
    - `STRICT`, dazu `members.name` mit `UNIQUE COLLATE NOCASE` und `CHECK(length(name) BETWEEN 1 AND 40)`.
    - goose-Up- und Down-Abschnitte.
  - **`main`:** `DATA_DIR` mit Rechten 0750 anlegen, falls es fehlt. `DATA_DIR/stashbert.db` öffnen und migrieren, **bevor** der Server startet. Bei einem Fehler beenden.
- **Nicht im Umfang:** sqlc, Abfragen, Backups.
- **Abnahmekriterien (Tests mit `t.TempDir()`):**
  1. `Open` und `Migrate` funktionieren, ein zweites `Migrate` ist wirkungslos.
  2. `PRAGMA foreign_keys` ist 1 und `PRAGMA journal_mode` ist `wal`.
  3. Die drei Tabellen existieren und sind laut `PRAGMA table_list` STRICT.
  4. `FormatTime` und `ParseTime` sind invers; die Ausgabe hat immer 3 Nachkommastellen und `Z`.
  5. `make check` ist grün.

### B05: sqlc einrichten

- **Status:** erledigt
- **Abhängig von:** B04
- **Referenzen:** ADR-0005
- **Umfang:**
  - Tool-Abhängigkeit: `go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`. Scheitert der Build des Tools, anhalten und melden.
  - `sqlc.yaml` (Version 2): Engine `sqlite`, Schema `internal/store/migrations`, Abfragen `internal/store/queries`, Go-Paket `db`, Ausgabe `internal/store/db`.
  - `internal/store/queries/settings.sql`: `GetSetting`, `UpsertSetting`.
  - `internal/store/queries/members.sql`: `ListMembers` (sortiert nach `name COLLATE NOCASE`), `GetMember`, `CreateMember`.
  - `make generate` ruft zusätzlich `go tool sqlc generate` auf.
- **Nicht im Umfang:** API-Endpunkte.
- **Abnahmekriterien:**
  1. Der generierte Code wird kompiliert.
  2. Tests: Einstellung schreiben und lesen; Mitglieder anlegen und sortiert auflisten; „Chris" und „chris" als Namen liefern einen Fehler.
  3. `make check` ist grün.

### B06: GTIN-Paket

- **Status:** erledigt
- **Abhängig von:** P0-1
- **Referenzen:** architecture.md 7.1, research.md 8.3
- **Umfang:** `internal/gtin` mit:
  - `Normalize(code string) (string, error)` mit Fehler `ErrInvalid`.
  - `IsLocal(normalized string) bool`.
  - Nur Standardbibliothek. 8-stellige Codes werden als EAN-8 geprüft.
- **Nicht im Umfang:** UPC-E, Datenbank.
- **Abnahmekriterien (tabellengetriebene Tests):**
  1. `3017620422003` und `4001686301265` sind gültig und bleiben unverändert.
  2. `4000400130157` liefert `ErrInvalid` (Prüfziffer).
  3. `034000470693` wird `0034000470693`; `00034000470693` wird `0034000470693`.
  4. `20004002` (8 Stellen) ist gültig und nicht lokal.
  5. Ein 13-stelliger Code mit Präfix `22` und korrekter Prüfziffer ist lokal. Den Code im Test selbst berechnen und die Rechnung kommentieren.
  6. Buchstaben, 10 Stellen und ein leerer String liefern `ErrInvalid`.
  7. `make check` ist grün.

### B07: Passwort-Hash und Setup-API

- **Status:** erledigt (durch B07r wieder zurückgebaut, ADR-0013)
- **Abhängig von:** B03, B05
- **Referenzen:** architecture.md 4.4, 6.2 (`getSetup`, `createSetup`), 6.5 (Setup-Request), 8, ADR-0009
- **Umfang:**
  - **`internal/auth/password.go`:**
    - `HashPassword(pw string) (string, error)` mit argon2id (m=19456, t=2, p=1), 16 Byte Salt aus `crypto/rand`, 32 Byte Schlüssel, PHC-Format.
    - `VerifyPassword(pw, encoded string) (bool, error)` mit Vergleich in konstanter Zeit.
  - **Spec:** Schemas `SetupStatus` (`configured`) und `SetupRequest` mit Grenzen laut 6.5; `GET /setup` (200) und `POST /setup` (204), jeweils mit `default: Problem`.
  - **Handler `createSetup`:**
    - Ist schon ein `password_hash` gesetzt, dann 409 `already_configured`.
    - Namen werden getrimmt; danach 1 bis 40 Zeichen, sonst 400.
    - Passwort und Mitglieder (UUIDv7) werden in **einer** Transaktion gespeichert.
    - Doppelte Namen erkennt die Datenbank (UNIQUE `NOCASE`): Ein Constraint-Fehler beim Einfügen ergibt 400 `invalid_request`, und die Transaktion wird zurückgerollt. Keine eigene Duplikatprüfung in Go.
- **Nicht im Umfang:** Login, Sitzungen.
- **Abnahmekriterien (Tests):**
  1. Hash und Prüfung funktionieren; ein falsches Passwort liefert `false`.
  2. Frische Instanz: `GET /setup` liefert `{"configured":false}`.
  3. `POST /setup` liefert 204, danach ist `configured` `true`. Ein zweiter `POST` liefert 409 `already_configured`.
  4. Je 400 `invalid_request` bei: Passwort mit 7 Zeichen, 0 Mitgliedern, „Chris" und „chris" in einer Anfrage (danach ist nichts gespeichert), einem Namen nur aus Leerzeichen, `Content-Type: text/plain`.
  5. `make check` ist grün.

### B07r: Anmeldung zurückbauen

- **Status:** erledigt
- **Abhängig von:** B07
- **Referenzen:** ADR-0013, architecture.md 5, 6.2, 8, 9.2
- **Umfang:**
  - `api/openapi.yaml`: Pfade `/setup` sowie die Schemas `SetupStatus` und `SetupRequest` entfernen, dann `make generate`.
  - Löschen: `internal/api/handlers_setup.go`, das Paket `internal/auth` samt Tests und die Setup-Tests in `internal/app`.
  - `internal/store/migrations/0001_init.sql`: Nur die Tabelle `settings` bleibt, `members` und `sessions` entfallen. Die Migration darf geändert werden, weil noch nichts ausgeliefert ist; eine lokale `.data/` muss danach gelöscht werden.
  - `internal/store/queries/members.sql` und die zugehörigen Tests entfernen, `make generate` (sqlc). `settings.sql` und `store.IsUniqueViolation` bleiben.
  - `internal/config`: `PUBLIC_URL` und `COOKIE_SECURE` samt Tests entfernen.
  - `Makefile`, Ziel `run`: `COOKIE_SECURE=false` entfernen.
  - `go mod tidy`, damit ungenutzte Module wie `golang.org/x/crypto` herausfallen.
- **Nicht im Umfang:** alles andere, insbesondere kein Umbau von `internal/app` über das Entfernen der Setup-Verdrahtung hinaus.
- **Abnahmekriterien:**
  1. `GET /api/v1/setup` liefert 404 `not_found`, `GET /api/v1/health` weiterhin 200 (Tests).
  2. Nach `Migrate` gibt es außer der goose-Tabelle nur `settings` (Test).
  3. Im Go-Code und in der Spec kommen die Wörter `password`, `session` und `member` (auch im Plural) nicht mehr vor: `grep -rinwE "passwords?|sessions?|members?" api cmd internal --exclude=gen.go --exclude-dir=db` ist leer.
  4. `make check` ist grün.

### B08a, B08b, B09, B10: entfallen

- **Status:** entfällt (ADR-0013, keine Anmeldung in M1)

### B11: Schema für Produkte, Barcodes, Buchungen und Cache

- **Status:** erledigt
- **Abhängig von:** B05
- **Referenzen:** architecture.md 5 (Tabellen und Formel `missing`)
- **Umfang:**
  - **`internal/store/migrations/0002_inventory.sql`:** `products`, `barcodes`, `movements` und `lookups` exakt nach architecture.md 5.
    - `CHECK`-Constraints für alle Wertebereiche, Aufzählungen **und** Längen (z. B. `length(name) BETWEEN 1 AND 120`).
    - Indizes `barcodes(product_id)` und `movements(product_id, id)`.
  - **`internal/domain/shopping.go`:** `Missing(stock, target int64, minStock *int64) int64`.
- **Nicht im Umfang:** sqlc-Abfragen (die kommen in den Tasks, die sie brauchen), API.
- **Abnahmekriterien (Tests):**
  1. Die Migration läuft auf einer DB nach `0001`.
  2. `stock = -1`, `min_stock > target`, `kind = 'foo'` und ein Name mit 121 Zeichen werden per direktem SQL-Insert abgelehnt.
  3. `Missing` als Tabelle (stock, target, min_stock, Ergebnis):
     - (2, 5, nil) ergibt 3
     - (6, 5, nil) ergibt 0
     - (0, 4, nil) ergibt 4
     - (2, 4, 1) ergibt 0
     - (0, 4, 1) ergibt 4
     - (1, 4, 1) ergibt 0
     - (0, 0, nil) ergibt 0
     - (5, 5, nil) ergibt 0
  4. `make check` ist grün.

### B12: Ereignis-Interface

- **Status:** erledigt
- **Abhängig von:** P0-1
- **Referenzen:** architecture.md 6.6, ADR-0011
- **Umfang:** `internal/events` mit:
  - `Event{ID, Type, Source, Time, Data}` und `New(typ string, data any) Event`. `New` setzt die ID (UUIDv7), `Source = "stashbert"` und die Zeit in UTC.
  - Die sechs Konstanten exakt nach der Tabelle in 6.6.
  - Für jeden Typ ein Data-Struct mit den Feldern aus 6.6: `ProductCreatedData`, `StockData` (für die drei `stock.*`-Typen), `ProductEmptyData`, `ShoppingChangedData`. JSON-Tags in `snake_case`.
  - Interface `Publisher { Publish(ctx context.Context, e Event) }`.
  - `Nop` (tut nichts) und `Recorder` (merkt sich Ereignisse, thread-sicher, für Tests anderer Pakete).
- **Nicht im Umfang:** Outbox, MQTT.
- **Abnahmekriterien:**
  1. Tests für `New` (Felder gesetzt) und `Recorder`.
  2. `make check` ist grün.

### B13a: Produkte lesen

- **Status:** erledigt
- **Abhängig von:** B06, B07r, B11
- **Referenzen:** architecture.md 6.2 (`listProducts`, `getProduct`), 6.5
- **Umfang:**
  - **Spec:** Schemas `Product`, `Barcode` und `ProductList` (`{items}`), Endpunkte `GET /products` und `GET /products/{id}`. Nullbare Felder als `type: [<typ>, "null"]`.
  - **Queries:**
    - Produkte auflisten, sortiert nach `name COLLATE NOCASE`, dann `id`.
    - Produkt lesen.
    - Barcodes eines Produkts und aller Produkte lesen, sortiert nach Code.
  - **Umwandlung in `internal/domain`:** `missing` mit `Missing`, `has_image = image_file != NULL`, Barcodes sortiert.
  - Unbekannte ID ergibt 404 `not_found`.
- **Nicht im Umfang:** Anlegen, Ändern, Buchungen.
- **Abnahmekriterien (Tests, Testdaten per direktem SQL):**
  1. Die Liste ist sortiert und `missing` berechnet.
  2. Die Barcodes hängen am richtigen Produkt.
  3. Unbekannte ID liefert 404.
  4. `make check` ist grün.

### B13b: Produkte anlegen

- **Status:** erledigt
- **Abhängig von:** B13a, B12
- **Referenzen:** architecture.md 6.2 (`createProduct`), 6.5, 6.6, ADR-0004
- **Umfang:**
  - **Spec:** Schemas `BarcodeInput` (`code`, `units` optional mit Standard 1) und `ProductCreate`, Endpunkt `POST /products` (201 mit `Location: /api/v1/products/{id}`).
  - **Fachlogik**, in **einer** Transaktion:
    - Barcodes über `gtin.Normalize`. Ungültig ergibt 422 `invalid_barcode`; doppelt in der Anfrage ergibt 400 `invalid_request`; schon vergeben ergibt 409 `barcode_in_use`.
    - `min_stock > target` ergibt 400 `invalid_request`. Namen und Texte werden getrimmt.
    - `origin = manual`, `lookup_state = none`, `needs_review = false`.
  - Nach dem Commit das Ereignis `product.created` über den `Publisher` aus `app.Deps`.
- **Nicht im Umfang:** Ändern, Löschen.
- **Abnahmekriterien (Tests):**
  1. Anlegen nur mit Name liefert die Standardwerte.
  2. Ein UPC-A-Barcode erscheint 13-stellig.
  3. Ungültiger Barcode liefert 422, doppelter 409, `min_stock > target` 400.
  4. Das Ereignis wird im `Recorder` gesehen.
  5. `make check` ist grün.

### B14: Produkte ändern und löschen

- **Status:** erledigt
- **Abhängig von:** B13b
- **Referenzen:** architecture.md 6.1 (PATCH), 6.2 (`updateProduct`, `deleteProduct`), 6.5, 6.6
- **Umfang:**
  - **Spec:** Schema `ProductPatch`: alle Felder optional, `brand`, `package_size`, `min_stock` und `note` als `type: [<typ>, "null"]`. Endpunkte `PATCH /products/{id}` und `DELETE /products/{id}`.
  - **`updateProduct`:**
    - Nur übergebene Felder ändern, `null` löscht.
    - `min_stock ≤ target` gegen den Endzustand prüfen, sonst 400 `invalid_request`.
    - `needs_review = false` nur, wenn der Patch `name`, `brand` oder `package_size` enthält.
    - `updated_at` setzen.
    - Ändert sich `missing`, nach dem Commit `shopping.changed`.
  - **`deleteProduct`:** löscht Produkt, Barcodes und Buchungen (Kaskade). 204.
- **Nicht im Umfang:** Bilddateien löschen (kommt in B22b).
- **Abnahmekriterien (Tests):**
  1. Der generierte Typ `ProductPatch` nutzt `nullable.Nullable` für die nullbaren Felder. Falls nicht, anhalten und melden, nicht auf OpenAPI 3.0 ausweichen.
  2. Nur `name` ändern lässt die anderen Felder unverändert und setzt `needs_review = false`.
  3. Nur `target` ändern lässt `needs_review` unverändert, ändert `missing` und löst `shopping.changed` aus.
  4. `brand: null` löscht die Marke.
  5. Löschen entfernt Barcodes und Buchungen; 404 bei unbekannter ID.
  6. `make check` ist grün.

### B15: Barcodes zuordnen und entfernen

- **Status:** erledigt
- **Abhängig von:** B13b
- **Referenzen:** architecture.md 6.2 (`addBarcode`, `removeBarcode`)
- **Umfang:**
  - Spec und Handler.
  - **`addBarcode`:** normalisieren. Ungültig ergibt 422 `invalid_barcode`, schon vergeben (auch am selben Produkt) ergibt 409 `barcode_in_use`, unbekanntes Produkt ergibt 404. Antwort 201 ohne `Location`.
  - **`removeBarcode`:** Code im Pfad normalisieren. Ungültig oder nicht an diesem Produkt ergibt 404 `not_found`. Antwort 204.
- **Nicht im Umfang:** Barcodes zwischen Produkten verschieben.
- **Abnahmekriterien:**
  1. Tests für alle genannten Fälle.
  2. `make check` ist grün.

### B16a: Buchungen per product_id

- **Status:** erledigt
- **Abhängig von:** B14
- **Referenzen:** architecture.md 6.3, 6.5, ADR-0004
- **Umfang:**
  - **Spec:**
    - Schema `MovementCreate` **vorerst nur** mit `product_id` (Pflicht), `kind` (`add`, `consume`, `inventory`), `quantity` (≥ 1, Standard 1) und `stock` (≥ 0).
    - Schemas `Movement` und `MovementResult`.
    - `POST /movements` (`createMovement`) mit 201 ohne `Location`.
  - **Prüfungen:** `inventory` ohne `stock` und `add`/`consume` mit `stock` ergeben 400 `invalid_request`.
  - **Fachlogik `domain.Book`**, in einer Transaktion: Produkt lesen, Regeln aus 6.3 anwenden (`delta` = tatsächliche Änderung), Buchung (UUIDv7) einfügen und Bestand am Produkt aktualisieren.
  - **Antwort:** `warnings` nach 6.3, `message` nach 6.3, z. B. `Kidneybohnen 3 → 2`.
- **Nicht im Umfang:** Ereignisse (B16b), Barcode, Idempotenz, Liste, Storno.
- **Abnahmekriterien (Tests):**
  1. `add` erhöht den Bestand.
  2. `consume` verringert ihn.
  3. Bestand 1 mit Menge 3 ergibt 0, `delta = -1` und `clamped_to_zero`.
  4. Bestand 0 ergibt 409 `stock_already_zero` ohne Buchung.
  5. `inventory` mit gleichem Wert ergibt `delta` 0.
  6. Unbekanntes Produkt ergibt 404.
  7. `message` hat das richtige Format.
  8. `make check` ist grün.

### B16b: Ereignisse für Buchungen

- **Status:** erledigt
- **Abhängig von:** B16a, B12
- **Referenzen:** architecture.md 6.6
- **Umfang:** `domain.Book` löst nach dem Commit die Ereignisse nach 6.6 aus (Typ nach Art, `product.empty`, `shopping.changed`, Reihenfolge, nur bei `delta ≠ 0`).
- **Nicht im Umfang:** andere Ereignisquellen.
- **Abnahmekriterien (Tests mit `Recorder`):**
  1. `consume` auf 0 liefert `stock.consumed`, dann `product.empty`, dann `shopping.changed` (bei `target > 0`).
  2. `inventory` ohne Änderung liefert kein Ereignis.
  3. `make check` ist grün.

### B17: Buchungen per bekanntem Barcode

- **Status:** erledigt
- **Abhängig von:** B15, B16b
- **Referenzen:** architecture.md 6.3
- **Umfang:**
  - **Spec:** `barcode` in `MovementCreate` ergänzen. `product_id` und `barcode` sind jetzt beide optional; genau eines muss gesetzt sein, sonst 400 `invalid_request`.
  - Barcode normalisieren, ungültig ergibt 422 `invalid_barcode`.
  - **Bekannter Barcode:** Menge = `quantity × units` für `add` und `consume`; `inventory` ignoriert `units`. In der Buchung steht der normalisierte Barcode.
  - **Unbekannter Barcode:** 404 `unknown_barcode` für **alle** Arten. Das ist vorläufig; B20 ersetzt das Verhalten für `add`.
- **Nicht im Umfang:** Produkte anlegen, Lookup.
- **Abnahmekriterien (Tests):**
  1. Buchung per Barcode; bei `units = 6` ergeben `add` +6 und `consume` -6.
  2. `inventory` per Barcode setzt `stock` unabhängig von `units`.
  3. Beide Felder oder keines ergeben 400.
  4. Ungültiger Barcode ergibt 422, unbekannter 404.
  5. `make check` ist grün.

### B18: Idempotency-Key für Buchungen

- **Status:** erledigt
- **Abhängig von:** B17
- **Referenzen:** architecture.md 5 (`idempotency_key`, `request_hash`), 6.1
- **Umfang:**
  - **Spec:** optionaler Header-Parameter `Idempotency-Key` (1 bis 255 Zeichen) bei `createMovement`.
  - `request_hash` = SHA-256 (hex) von `json.Marshal` des dekodierten Request-Bodys.
  - **Schlüssel mit gleichem Hash vorhanden:** 201 mit der rekonstruierten Antwort nach 6.1. Keine neue Buchung.
  - **Schlüssel mit anderem Hash vorhanden:** 422 `idempotency_key_mismatch`.
  - Gleichzeitige Anfragen sichert der UNIQUE-Index ab; bei Konflikt die vorhandene Buchung lesen und wie oben antworten.
- **Nicht im Umfang:** Idempotenz bei anderen Endpunkten (Storno folgt in B24).
- **Abnahmekriterien (Tests):**
  1. Zweimal gleich ergibt eine Buchung und zweimal 201 mit derselben `movement.id`.
  2. Gleicher Schlüssel mit anderem Body ergibt 422.
  3. `make check` ist grün.

### B19: Open-Food-Facts-Client

- **Status:** erledigt
- **Abhängig von:** B02, B06
- **Referenzen:** architecture.md 7.2 (Schritte 2 und 3), 7.3 (Limiter)
- **Umfang:**
  - **Testdaten zuerst:** Live-Antworten mit genau diesen Befehlen in `internal/lookup/testdata/` speichern (Code `4001686301265` = Haribo, `8710447445990` = Open Beauty Facts, `4009998877669` = nicht vorhanden). Danach nichts von Hand verändern.
    ```
    F='code,product_name,product_name_de,generic_name_de,brands,quantity,product_quantity,product_quantity_unit,image_front_url,product_type'
    UA='StashBert/dev (stashbert@example.org)'
    curl -sS -A "$UA" "https://world.openfoodfacts.org/api/v3.6/product/4001686301265?product_type=all&lc=de&fields=$F" -o internal/lookup/testdata/found_food.json
    curl -sSL -A "$UA" "https://world.openfoodfacts.org/api/v3.6/product/8710447445990?product_type=all&lc=de&fields=$F" -o internal/lookup/testdata/found_beauty.json
    curl -sS -A "$UA" "https://world.openfoodfacts.org/api/v3.6/product/4009998877669?product_type=all&lc=de&fields=$F" -o internal/lookup/testdata/not_found.json
    ```
  - **`internal/lookup/off.go`:**
    - `NewClient(baseURL, userAgent string, httpClient *http.Client, limiter *rate.Limiter) *Client`.
    - `Lookup(ctx, code) (Result, error)`: wartet mit `limiter.Wait(ctx)` einmal pro Aufruf und fragt exakt wie in 7.2 an. Redirects folgt der `http.Client` von selbst.
    - `Result{Found bool; Name, Brand, PackageSize, ImageURL, ProductType string}` mit der Zuordnung aus 7.2, Schritt 3 (ohne Kürzen, das macht B20).
    - HTTP 404 ergibt `Found = false` ohne Fehler; 429 ergibt `ErrRateLimited`; 5xx, Netzfehler und Kontext-Ende ergeben `ErrUnavailable`.
  - `NewDisabledClient() *Client`: derselbe Typ, dessen `Lookup` und `TryLookup` immer `ErrDisabled` zurückgeben. Genutzt wird er, wenn `OFF_CONTACT` leer ist.
  - Zusätzlich `TryLookup(ctx, code) (Result, error)`: wie `Lookup`, aber mit `limiter.Allow()`. Ist kein Token frei, gibt es `ErrRateLimited` ohne Anfrage.
  - `Finalize(r Result, code string) Result`: fehlender Name wird „Neues Produkt <code>", dann Kürzung nach Zeichen (`Name` und `Brand` 120, `PackageSize` 40). Genutzt von B20 und B21.
- **Nicht im Umfang:** Cache, Datenbank, Nutzung in Buchungen.
- **Abnahmekriterien (Tests mit `httptest`, der die Testdateien ausliefert):**
  1. `found_food.json` wird zu Name, Marke, Größe und `ProductType = food` gemappt.
  2. Der `httptest`-Server antwortet mit 302 auf einen zweiten Server, der `found_beauty.json` liefert; das Ergebnis hat `ProductType = beauty`.
  3. 404 mit `not_found.json` ergibt nicht gefunden.
  4. 429 mit HTML-Body ergibt `ErrRateLimited`; 503 ergibt `ErrUnavailable`.
  5. User-Agent und Query-Parameter werden geprüft.
  6. `TryLookup` ohne freien Token macht keine Anfrage.
  7. `Finalize` kürzt einen 200-Zeichen-Namen mit Umlauten korrekt auf 120 Zeichen und setzt den Fallback-Namen.
  8. `make check` ist grün.

### B20: Unbekannter Barcode beim Einlagern

- **Status:** erledigt
- **Abhängig von:** B18, B19
- **Referenzen:** architecture.md 5 (`lookups`), 6.3, 7.2, ADR-0004
- **Umfang:**
  - **Queries** für `lookups` (lesen, schreiben), `source = off`, `payload` = JSON von `lookup.Result`.
  - **Ablauf exakt nach 7.2:**
    - Die OFF-Anfrage läuft mit einem Kontext von 2,5 s.
    - Die Lookup-Quelle wird über das Interface `domain.Lookuper { Lookup(ctx context.Context, code string) (lookup.Result, error) }` injiziert.
    - Treffer werden mit `lookup.Finalize` aufbereitet.
  - Erst **nach** dem Lookup: Produkt, Barcode und Buchung in **einer** Transaktion.
  - Scheitert das Einfügen des Barcodes, weil ein paralleler Scan ihn gerade angelegt hat: als bekannten Barcode buchen.
  - **Antwort:** `product_created: true`, bei Platzhalter `warnings: ["placeholder_created"]`, `message` mit dem Präfix `Neu: `.
  - **Ereignisse:** `product.created`, dann die Buchungsereignisse.
  - `consume` und `inventory` mit unbekanntem Barcode bleiben 404 `unknown_barcode`.
- **Nicht im Umfang:** Hintergrund-Nachladen, Bilder.
- **Abnahmekriterien (Tests mit Fake-Lookuper):**
  1. Lokaler Code ergibt Platzhalter mit `lookup_state = none` und keinen Lookup-Aufruf.
  2. Cache-Treffer (gefunden und nicht gefunden) ohne Lookup-Aufruf.
  3. OFF-Treffer ergibt `origin` nach `product_type`.
  4. OFF 404 ergibt Platzhalter mit `not_found`.
  5. `ErrUnavailable`, `ErrDisabled` und Timeout ergeben Platzhalter mit `pending`.
  6. Ein Name mit 200 Zeichen wird auf 120 gekürzt; ein leerer Name ergibt „Neues Produkt <code>".
  7. Der Bestand ist danach 1.
  8. `make check` ist grün.

### B21: Hintergrund-Nachladen

- **Status:** erledigt
- **Abhängig von:** B20
- **Referenzen:** architecture.md 7.3
- **Umfang:**
  - `internal/lookup/enricher.go`: `NewEnricher(db *sql.DB, src TryLookuper, logger *slog.Logger) *Enricher`, `RunOnce(ctx)` und `Start(ctx, interval)`. `Start` blockiert bis zum Ende von `ctx` und ruft `RunOnce` bei jedem Tick eines `time.Ticker` auf (der erste Lauf nach einem Intervall).
  - Die Quelle wird über das Interface `lookup.TryLookuper { TryLookup(ctx context.Context, code string) (Result, error) }` injiziert.
  - Die Zuordnung `product_type` zu `origin` zieht als exportierte Funktion `lookup.Origin` aus `internal/domain` in `internal/lookup` um (sonst entsteht ein Import-Zyklus); `internal/domain` nutzt sie.
  - Pro Lauf höchstens 20 Produkte mit `lookup_state = pending`, älteste `created_at` zuerst. Nachgeschlagen wird der erste Barcode (nach Code sortiert) mit `TryLookup` und 10 s Timeout. `ErrRateLimited` und `ErrDisabled` beenden den Lauf. Ein Produkt ohne Barcode bekommt `lookup_state = none`.
  - **Treffer:** `lookup_state = done`; ist `needs_review` noch `true`, zusätzlich `name`, `brand`, `package_size`, `origin` und `image_source_url` (aufbereitet mit `lookup.Finalize`, leere Texte werden NULL). Den Cache schreiben wie in B20.
  - **404:** `lookup_state = not_found` und negativer Cache-Eintrag. Andere Fehler: unverändert lassen und mit `warn` loggen.
  - Jede Änderung setzt `updated_at` (architecture.md 5). Ereignisse gibt es keine (6.6 nennt keine für das Nachladen).
  - `main` startet den Job mit 60 s Intervall und dem Client aus B20 (gemeinsamer Limiter).
- **Nicht im Umfang:** Bilder.
- **Abnahmekriterien (Tests mit Fake und `RunOnce`):**
  1. Bei `needs_review = true` werden die Felder übernommen.
  2. Bei `needs_review = false` nur `lookup_state = done`.
  3. Nicht gefunden setzt `not_found`.
  4. Ein Fehler lässt den Zustand `pending`.
  5. `ErrRateLimited` beendet den Lauf nach dem ersten Produkt, `ErrDisabled` ebenso.
  6. Ein Produkt ohne Barcode bekommt `none` ohne Lookup-Aufruf.
  7. `make check` ist grün.

### B22a: Bild-Download

- **Status:** erledigt
- **Abhängig von:** B21
- **Referenzen:** architecture.md 7.3
- **Umfang:** `internal/lookup/images.go`:
  - Konstruktor mit Host-Allowlist (Standard aus 7.3), `http.Client` und Zielverzeichnis, dazu `RunOnce(ctx)` und `Start(ctx, interval)`.
  - Pro Lauf höchstens 10 Produkte mit `image_source_url` und ohne `image_file`.
    - Nur `https` und erlaubte Hosts; sonst `image_source_url = NULL`.
    - Timeout 30 s. Status 200 und ein erlaubter Content-Type sind Pflicht, maximal 2 MB.
    - Die Datei wird über eine temporäre Datei plus `rename` als `DATA_DIR/images/<id>.<jpg|png|webp>` geschrieben; danach `image_file` setzen.
    - Bei 4xx außer 408 und 429 (z. B. 403, 404, 410), falschem Typ, leerem Body oder zu groß: `image_source_url = NULL`. Andere Fehler (Netz, 408, 429, 5xx): im nächsten Lauf erneut versuchen.
  - `main` startet den Job mit 60 s Intervall.
- **Nicht im Umfang:** Endpunkt, Löschen, Bilder verkleinern.
- **Abnahmekriterien (Tests mit `httptest` und Test-Allowlist):**
  1. Ein fremder Host wird abgelehnt, eine zu große Datei ebenfalls.
  2. Erfolg schreibt die Datei und setzt `image_file`.
  3. `make check` ist grün.

### B22b: Bild-Endpunkt und Löschen der Bilddatei

- **Status:** erledigt
- **Abhängig von:** B22a
- **Referenzen:** architecture.md 6.2 (`getProductImage`)
- **Umfang:**
  - Spec und Handler `GET /products/{id}/image`: Antwort 200 mit Content-Type `image/*`, Bild-Bytes und `Cache-Control: private, max-age=86400`, sonst 404.
  - `deleteProduct` löscht nach dem Commit zusätzlich die Bilddatei, falls vorhanden, und löst `shopping.changed` mit `missing_after = 0` aus, wenn das Produkt vorher `missing > 0` hatte (architecture.md 6.6).
- **Nicht im Umfang:** Upload.
- **Abnahmekriterien (Tests):**
  1. Das Bild wird mit richtigem Content-Type geliefert; ohne Bild 404.
  2. Löschen entfernt die Datei.
  3. Löschen eines Produkts mit `missing > 0` löst `shopping.changed` aus, ohne fehlende Menge kein Ereignis.
  4. `make check` ist grün.

### B23: Buchungsliste

- **Status:** erledigt
- **Abhängig von:** B16a
- **Referenzen:** architecture.md 6.2 (`listMovements`)
- **Umfang:**
  - Spec und Handler: Parameter `product_id` (optional), `limit` (1 bis 200, Standard 50) und `cursor`. Antwort `{items, next_cursor}`.
  - Sortierung `id` absteigend. `cursor` ist die `id` des letzten Eintrags; die nächste Seite liefert `id < cursor`.
  - Ein Cursor, der keine UUID ist, ergibt 400 `invalid_request`.
- **Nicht im Umfang:** weitere Filter.
- **Abnahmekriterien (Tests):**
  1. 120 Buchungen ergeben mit `limit = 50` drei Seiten, die letzte mit `next_cursor: null`.
  2. Der Produktfilter wirkt.
  3. `make check` ist grün.

### B24: Storno

- **Status:** erledigt
- **Abhängig von:** B18, B23
- **Referenzen:** architecture.md 6.3 (Storno), 6.2 (`reverseMovement`), 6.6
- **Umfang:**
  - Spec und Handler `POST /movements/{id}/reversal` mit optionalem `Idempotency-Key` (gleiche Logik wie B18, Hash über die ID). Antwort 201 `MovementResult`.
  - Regeln aus 6.3: `reverses_id` setzen; ist schon storniert, 409 `already_reversed`; `reversal` und `merge` ergeben 409 `not_reversible`; ein negativer Bestand wird auf 0 begrenzt mit `clamped_to_zero`, und `delta` ist die tatsächliche Änderung.
  - Ereignisse nach 6.6 (`stock.adjusted` usw.).
- **Nicht im Umfang:** Storno eines Stornos.
- **Abnahmekriterien (Tests):**
  1. Storno von `consume` und von `add`.
  2. Doppeltes Storno ergibt 409.
  3. Ein Storno eines Stornos ergibt 409.
  4. Fall mit Begrenzung: `add` 3, `consume` 2, Storno des `add` ergibt Bestand 0 mit `delta = -1`.
  5. `make check` ist grün.

### B25: Produkte zusammenführen

- **Status:** erledigt
- **Abhängig von:** B22b, B24
- **Referenzen:** architecture.md 6.5 (Merge), 6.2 (`mergeProduct`), 6.6
- **Umfang:**
  - Spec und Handler `POST /products/{id}/merge` mit `{target_product_id}`. Antwort 200 mit dem Zielprodukt.
  - In einer Transaktion:
    1. Barcodes und Buchungen der Quelle auf das Ziel umhängen.
    2. Ist der Bestand der Quelle > 0: eine Buchung `merge` am Ziel mit `delta` = Bestand der Quelle und den Bestand des Ziels erhöhen.
    3. Die Quelle löschen.
  - Nach dem Commit: Bilddatei der Quelle löschen, Ereignisse nach 6.6.
  - Quelle = Ziel ergibt 400 `invalid_request`. Unbekannte IDs ergeben 404.
- **Nicht im Umfang:** Felder des Ziels verändern.
- **Abnahmekriterien (Tests):**
  1. Barcodes, Buchungen und Bestand sind danach am Ziel, die Quelle existiert nicht mehr.
  2. Die Fehlerfälle.
  3. `make check` ist grün.

### B26: Einkaufsliste

- **Status:** erledigt
- **Abhängig von:** B13a
- **Referenzen:** architecture.md 6.2 (`getShoppingList`), 6.5 (`ShoppingItem`), 5 (`missing`)
- **Umfang:** Spec und Handler: alle Produkte mit `missing > 0`, sortiert nach `name COLLATE NOCASE`. `missing` wird mit `domain.Missing` berechnet.
- **Nicht im Umfang:** Teilen, Export.
- **Abnahmekriterien (Tests):**
  1. Kidneybohnen (Bestand 2, Soll 5) erscheint mit `missing: 3`.
  2. Spaghetti (6, 5) erscheint nicht.
  3. Mehl (2, Soll 4, Mindestbestand 1) erscheint nicht.
  4. `make check` ist grün.

### B27: Spec ausliefern

- **Status:** erledigt
- **Abhängig von:** B07r
- **Referenzen:** architecture.md 6.2 (Zusatz unter der Tabelle)
- **Umfang:**
  - `api/embed.go` (Paket `apispec`) mit `//go:embed openapi.yaml`.
  - In `internal/app` die Route `GET /api/v1/openapi.yaml` **vor** dem generierten Handler registrieren (ohne Validator) mit `Content-Type: application/yaml`.
- **Nicht im Umfang:** Doku-Oberfläche (Scalar, Swagger UI).
- **Abnahmekriterien:**
  1. Ein Test vergleicht die ausgelieferten Bytes mit der Datei.
  2. `make check` ist grün.

### B28: Backups

- **Status:** erledigt
- **Abhängig von:** B04
- **Referenzen:** architecture.md 9.3, ADR-0005
- **Umfang:**
  - **`internal/backup`:**
    - `Run(ctx, db *sql.DB, dir string, keep int, now time.Time) (string, error)`: `VACUUM INTO` nach `dir/stashbert-<YYYYMMDD-HHMMSS>.db` (UTC aus `now`). Existiert die Datei schon, endet der Aufruf mit Fehler und legt nichts an. Danach nur die neuesten `keep` Dateien dieses Musters behalten.
    - `PreMigration(ctx, db, dir string, now time.Time) (string, error)`: schreibt `pre-migration-<YYYYMMDD-HHMMSS>.db`.
  - **`internal/app/db.go`:** `OpenAndMigrate(ctx, path, backupDir string, migrations fs.FS, now time.Time) (*sql.DB, error)`. Die Funktion liegt in `internal/app`, damit `store` nicht von `backup` abhängt.
    - War die DB-Datei vor dem Öffnen schon vorhanden und meldet der goose-Provider ausstehende Migrationen (`HasPending`), läuft vor `store.Migrate` `backup.PreMigration`.
    - `main` nutzt ab jetzt `app.OpenAndMigrate` mit `store.Migrations`.
  - **In `main`:** Nach dem Start `backup.Run` ausführen, danach alle 24 h.
- **Nicht im Umfang:** Restore-Befehl, Upload irgendwohin.
- **Abnahmekriterien (Tests):**
  1. Die Backup-Datei ist eine lesbare SQLite-DB mit denselben Zeilenzahlen.
  2. Bei `keep = 2` und 4 Läufen mit verschiedenen `now` bleiben 2 Dateien.
  3. `OpenAndMigrate`: Bei neuer DB gibt es kein Pre-Migration-Backup. Bei einer vorhandenen DB, die mit einem `fstest.MapFS` aus einer Migration angelegt wurde und dann mit einem FS aus zwei Migrationen geöffnet wird, gibt es eines.
  4. `make check` ist grün.

### B29: Web-Oberfläche einbetten und ausliefern

- **Status:** erledigt
- **Abhängig von:** B03
- **Referenzen:** architecture.md 4.2, 8 (Sicherheits-Header)
- **Umfang:**
  - `internal/webui/dist/.gitkeep` anlegen.
  - **`internal/webui`:** `//go:embed all:dist` und `Handler(fsys fs.FS) http.Handler`. Das `fs.FS` wird übergeben, damit Tests ein eigenes verwenden können. Kein `http.FileServer` (kein Verzeichnis-Listing).
    - Ist der Pfad eine Datei, wird sie ausgeliefert.
    - Existiert keine Datei und hat der Pfad keine Dateiendung, wird `index.html` ausgeliefert (SPA-Fallback). Verzeichnisse zählen als „keine Datei".
    - Sonst 404.
    - Ohne `index.html` im FS liefert `/` den Text „Web-Oberfläche nicht gebaut" (200, `text/plain`).
  - **Header:**
    - die Sicherheits-Header aus architecture.md 8. Beim Erzeugen des Handlers werden die Inline-`<script>`-Inhalte von `index.html` gesucht, per SHA-256 (base64) gehasht und als `'sha256-…'` an `script-src` angehängt. Skripte mit `src`-Attribut zählen nicht.
    - `Cache-Control: public, max-age=31536000, immutable` für `/assets/*` und `/_app/immutable/*`
    - `no-cache` für `index.html`, `sw.js` und `manifest.webmanifest`
  - **`internal/app`:** Handler unter `/` (die API bleibt unter `/api/v1/`).
  - **`make build`:** Existiert `web/package.json`:
    1. In `web/` `npm ci` und `npm run build`.
    2. In `internal/webui/dist/` alles außer `.gitkeep` löschen und `web/dist/.` dorthin kopieren.
    3. Go bauen.
    Sonst nur Go bauen.
- **Nicht im Umfang:** Frontend-Code.
- **Abnahmekriterien (Tests mit `fstest.MapFS`):**
  1. `/` liefert `index.html`, `/vorrat` den Fallback, `/assets/fehlt.js` 404, und der CSP-Header ist gesetzt.
  2. Eine `index.html` mit dem Inline-Skript `console.log(1)` ergibt genau einen passenden `'sha256-…'`-Eintrag in `script-src`; ohne Inline-Skript gibt es keinen.
  3. Ohne `index.html` kommt der Platzhaltertext.
  4. `/api/v1/health` funktioniert weiterhin.
  5. `make check` ist grün.

---

## Phase 1b: Frontend

Alle F-Tasks setzen P0-6 voraus. Gemeinsame Regeln: AGENTS.md (Abschnitte Frontend und Frontend-Regeln).

**React-Angaben aus P0-6** (gelten für alle F-Tasks):

- Framework: React 19 mit Vite, angelegt mit `npm create --yes vite@latest web -- --template react-ts --eslint --no-interactive --no-immediate` (ohne `--eslint` legt create-vite 9 Oxlint statt ESLint an).
- Ordner und Dateien nach AGENTS.md, Frontend-Regeln: Routentabelle in `web/src/router.tsx`, Ansichten in `web/src/routes/`, reine Funktionen in `web/src/lib/`.
- Router React Router 7 im Data-Modus, Daten mit TanStack Query 5, keine Komponentenbibliothek, Dialoge mit `<dialog>`.
- Vite legt Assets unter `/assets/` ab und erzeugt keine Inline-Skripte; `internal/webui` (B29) braucht keine Anpassung.

### F01: Frontend-Projekt anlegen

- **Status:** wartet auf Nutzer
- **Abhängig von:** P0-6, B29
- **Referenzen:** ADR-0007, AGENTS.md, architecture.md 8 (CSP)
- **Umfang:**
  - **Projekt in `web/`** mit `npm create --yes vite@latest web -- --template react-ts --eslint --no-interactive --no-immediate` (ohne `--eslint` legt create-vite 9 Oxlint statt ESLint an) (Vorlage behalten, auch ihre ESLint-Konfiguration; die Regeln von `eslint-plugin-react-hooks` auf Fehler stellen): TypeScript strict, Tailwind CSS 4 über `@tailwindcss/vite`, Vitest.
  - **npm-Skripte:** `dev`, `build` (Ausgabe `web/dist`), `check` (`tsc -b`), `lint` (`eslint .`), `test` (`vitest run`).
  - **Vite-Dev-Proxy:** `/api` wird an `http://localhost:8080` weitergeleitet.
  - **`make check`** ruft zusätzlich `npm ci`, `npm run check`, `npm run lint` und `npm test` in `web/` auf, sofern `web/package.json` existiert.
  - **CI:** Node 24 mit `actions/setup-node` einrichten (Version vorher nachschlagen). Reihenfolge: `setup-node`, `make check` (enthält `npm ci`), danach die Generierungsprüfung.
  - `internal/webui` bleibt unverändert: Vite nutzt `/assets/`, und die CSP-Hashes erledigt B29.
- **Nicht im Umfang:** Ansichten, API-Client.
- **Abnahmekriterien:**
  1. `make build` erzeugt ein Binary, das unter `/` die Startseite der Vorlage ausliefert.
  2. `make check` ist grün.
  3. (Nutzer) Im Browser meldet die Konsole keine CSP-Verletzung.

### F02: API-Client

- **Status:** erledigt
- **Abhängig von:** F01
- **Referenzen:** ADR-0003
- **Umfang:**
  - npm-Skript `generate:api`: `openapi-typescript ../api/openapi.yaml -o src/lib/api/schema.d.ts --default-non-nullable false` (sonst werden Felder mit `default` in Request-Schemas wie `quantity`, `target` und `units` zu Pflichtfeldern, obwohl der Server sie als optional führt).
  - `make generate` ruft es auf, sofern `web/package.json` existiert.
  - **`src/lib/api/client.ts`:**
    - `createApiClient(baseUrl: string)` auf Basis von `createClient<paths>` mit `credentials: "same-origin"`.
    - `export const api = createApiClient("/api/v1")`.
    - Die Hilfsfunktion `problemCode(error): string | undefined`.
- **Nicht im Umfang:** Ansichten.
- **Abnahmekriterien:**
  1. Die Typprüfung ist grün.
  2. Ein Vitest-Test mit gemocktem `fetch` und `createApiClient("http://test/api/v1")` prüft die URL und `problemCode`.
  3. `make check` ist grün.

### F03: App-Rahmen

- **Status:** erledigt
- **Abhängig von:** F02
- **Referenzen:** architecture.md 4.2
- **Umfang:**
  - **Routen:**
    - `/` leitet auf `/vorrat` weiter
    - `/vorrat`, `/scan`, `/einkauf`
    - `/produkt/:id`
  - **React:** Routentabelle als exportiertes Array `routes` in `web/src/router.tsx`, daraus `createBrowserRouter(routes)`. Die Weiterleitung mit `<Navigate to="/vorrat" replace />`. Eine Layout-Route mit `<Outlet />` trägt die Navigationsleiste. `main.tsx` setzt `QueryClientProvider` und `RouterProvider`.
  - Untere Navigationsleiste mit Vorrat, Scan (mittig, größer) und Einkauf (`NavLink`), nur auf den ersten drei Routen. Safe-Area-Abstände für das iPhone.
  - Alle Ansichten sind Platzhalter mit Überschrift, je eine Datei in `web/src/routes/`.
- **Nicht im Umfang:** Inhalte der Ansichten, Anmeldung (ADR-0013).
- **Abnahmekriterien:**
  1. Ein Vitest-Test prüft die Zuordnung von Pfad zu Ansicht mit `matchRoutes(routes, pfad)` aus `react-router`.
  2. `make check` ist grün.

### F04: entfällt

- **Status:** entfällt (ADR-0013, keine Anmeldung in M1)

### F05: Vorrat

- **Status:** wartet auf Nutzer
- **Abhängig von:** F03, B16b, B22b
- **Referenzen:** architecture.md 1, 6.2, 6.3
- **Umfang:**
  - `GET /products`, sortiert mit `Intl.Collator("de")`.
  - **Suche** als reine Funktion `matches(product, query)`: Name und Marke; beides mit `normalize("NFD")`, ohne Diakritika und in Kleinbuchstaben; Teilstring-Vergleich.
  - **Filter-Chips:** Alle, Nachkaufen (`missing > 0`), Leer (`stock = 0`), Prüfen (`needs_review`).
  - **Zeile:**
    - Bild (`/api/v1/products/{id}/image`, wenn `has_image`, sonst ein neutrales Platzhalter-Symbol)
    - Name, Marke klein
    - „Bestand / Soll"
    - Buttons [-] und [+]: `POST /movements` mit `product_id` und `consume` bzw. `add`
  - 409 `stock_already_zero` zeigt kurz „War schon leer". Die Zeile wird aus `product` der Antwort aktualisiert.
  - Ein Tap auf die Zeile öffnet `/produkt/:id`. Neu geladen wird beim Öffnen der Ansicht.
- **Nicht im Umfang:** Produkte manuell anlegen, Sortieroptionen, Pull-to-Refresh.
- **Abnahmekriterien:**
  1. Vitest-Tests für `matches`, z. B. „kase" findet „Käse".
  2. Vitest-Tests für die Filter.
  3. `make check` ist grün.
  4. (Nutzer) Bedienung auf dem iPhone.

### F06a: Produktdetail: Formular, Löschen, Quellenhinweis

- **Status:** wartet auf Nutzer
- **Abhängig von:** F05
- **Referenzen:** architecture.md 6.2, 6.5
- **Umfang:**
  - `GET /products/{id}`.
  - **Zurück:** oben ein Link „Zurück" zu `/vorrat` (die Seite hat keine Navigationsleiste; in der installierten App gibt es keinen Browser-Zurück-Knopf).
  - **Formular:** Name, Marke, Packungsgröße, Soll (Zahl mit Schritt 1), Notiz. „Speichern" sendet `PATCH`, nur mit den geänderten Feldern. Das Diff berechnet die reine Funktion `diffPatch(original, form)`.
  - **„Passt so"**: nur bei `needs_review`. Sendet `PATCH` mit dem unveränderten `name` und setzt damit `needs_review` zurück.
  - **Löschen** mit eigenem Bestätigungsdialog (kein `window.confirm`), danach `/vorrat`.
  - **Quellenhinweis:** Bei `origin` aus OFF steht dort „Daten und Bild: Open Food Facts (ODbL / CC BY-SA 3.0)" mit Link `https://world.openfoodfacts.org/product/<erster Barcode>`.
- **Nicht im Umfang:** Barcodes, Inventur, Verlauf, Mindestbestand.
- **Abnahmekriterien:**
  1. Vitest-Tests für `diffPatch`, inklusive „leeres Feld wird `null`".
  2. `make check` ist grün.
  3. (Nutzer) Bedienung.

### F06b: Produktdetail: Barcodes

- **Status:** erledigt
- **Abhängig von:** F06a, B15
- **Referenzen:** architecture.md 6.2, 7.1
- **Umfang:**
  - `src/lib/gtin.ts`: `normalizeGtin(code): string | null` mit denselben Regeln wie architecture.md 7.1.
  - Barcode-Liste mit Entfernen (Bestätigungsdialog) und Hinzufügen per Eingabefeld (`inputmode="numeric"`, Prüfung mit `normalizeGtin`). 409 zeigt „Barcode gehört schon zu einem anderen Produkt".
- **Nicht im Umfang:** `units` in der Oberfläche.
- **Abnahmekriterien:**
  1. Vitest-Tests für `gtin.ts` mit denselben Beispielen wie B06.
  2. `make check` ist grün.

### F06c: Produktdetail: Bestand, Verlauf, Zusammenführen

- **Status:** offen
- **Abhängig von:** F06b, B23, B25
- **Referenzen:** architecture.md 6.2, 6.3, 6.5 (Merge)
- **Umfang:**
  - **Bestand korrigieren:** Zahlfeld, sendet `POST /movements` mit `inventory`.
  - **Letzte Buchungen:** `GET /movements?product_id=…&limit=10`, mit Zeit und `delta`.
  - **Zusammenführen:** Bei `needs_review` gibt es den Button „Mit vorhandenem Produkt zusammenführen". Er öffnet eine Auswahlliste mit Suche (`matches` aus F05, ohne das eigene Produkt). Die Auswahl sendet `POST /products/{id}/merge` und öffnet danach das Zielprodukt.
- **Nicht im Umfang:** Storno aus dem Verlauf.
- **Abnahmekriterien:**
  1. Vitest-Test für die Auswahlliste: Das eigene Produkt ist ausgeschlossen.
  2. `make check` ist grün.
  3. (Nutzer) Bedienung.

### F07: Scanner-Modul

- **Status:** offen
- **Abhängig von:** F01
- **Referenzen:** ADR-0008, architecture.md 4.3, `docs/poc/p0-protokoll.md`, Code der gewählten POC-Variante
- **Umfang:** framework-unabhängiges TypeScript in `web/src/lib/scanner/`, übernommen aus `poc/scanner-react/src/scanner/` (dort schon mit Tonmustern und gleitendem Fenster):
  - `camera.ts`: Start und Stopp, Gerätewahl (gespeichert), Licht, Zoom.
  - `decoder.ts`: Ponyfill, Formate `ean_13`, `ean_8`, `upc_a`, und `prepareZXingModule` mit `locateFile` auf `/zxing_reader.wasm`. Der Aufruf erfolgt beim Laden des Moduls, **vor** dem ersten `new BarcodeDetector()`.
  - `loop.ts`: Leseschleife mit Streifen und höchstens einem laufenden Decode.
  - `dedupe.ts`: rein funktional, 2000 ms als gleitendes Fenster: Jede Erkennung eines Codes verlängert das Fenster, ein Code im Bild bucht also nur einmal. Erst nach 2000 ms ohne diesen Code zählt er wieder (im React-Prototyp mit Buchungen erprobt).
  - `feedback.ts`: Audio mit `audioSession` und `AudioContext`, freigeschaltet beim Start. Tonmuster als exportierte Tabelle:
    - `add`: 880 Hz, 100 ms
    - `consume`: 660 Hz, 100 ms
    - `warn`: 2 × 440 Hz, je 80 ms
    - `error`: 220 Hz, 300 ms
  - `wakelock.ts`.
  - npm-Skript `copy:wasm` wie in der POC-Spezifikation Punkt 6 (Auflösung relativ zu `barcode-detector`, SHA-256-Prüfung), läuft vor `build` und `dev`.
- **Nicht im Umfang:** Ansicht, API-Aufrufe.
- **Abnahmekriterien:**
  1. Vitest-Tests für `dedupe` (auch: ein Code, der dauerhaft im Bild bleibt, zählt nur einmal) und die Tonmuster-Tabelle.
  2. Der Build enthält `zxing_reader.wasm`.
  3. Ein manipulierter Hash lässt `copy:wasm` fehlschlagen (Test mit Umgebungsvariable oder Parameter für den erwarteten Hash).
  4. `make check` ist grün.

### F08: Scanner-Ansicht: Buchen und Rückmeldung

- **Status:** offen
- **Abhängig von:** F03, F07, B20
- **Referenzen:** architecture.md 6.3, ADR-0008
- **Umfang:**
  - **Modus-Schalter:** „Einlagern | Entnehmen", groß, in `localStorage` gespeichert, Farbe grün bzw. blau.
  - **Kamera:** startet beim Öffnen der Ansicht, mit Streifen-Overlay. Ein Zahnrad öffnet ein kleines Menü mit Kamera-Auswahl, Zoom und Licht (aus `camera.ts`, nur was das Gerät kann).
  - **Buchen pro Code:** `POST /movements` mit `{barcode, kind}` und Header `Idempotency-Key: crypto.randomUUID()`.
  - **Rückmeldung** über die reine Funktion `feedbackFor(result)`:

    | Ergebnis | Rückmeldung |
    |---|---|
    | 201 ohne `warnings` | Modusfarbe und Ton `add` bzw. `consume` |
    | 201 mit `warnings` | gelb, `warn` |
    | 404 `unknown_barcode` | rot, `error`, „Unbekannter Barcode" |
    | 409 `stock_already_zero` | gelb, `warn`, „War schon leer" |
    | 422 | rot, `error`, „Ungültiger Barcode" |
    | Netzfehler | rot, `error`, „Server nicht erreichbar" |

  - Der `message`-Text wird 2 s groß angezeigt, der Scanner läuft weiter.
  - Beim Verlassen und bei `hidden` die Kamera stoppen. Bei Rückkehr „Tippen zum Fortsetzen".
- **Nicht im Umfang:** Ergebniskarte mit Aktionen (F09), Karte für neue Produkte (F10), manuelle Eingabe (F11).
- **Abnahmekriterien:**
  1. Vitest-Tests für `feedbackFor`.
  2. `make check` ist grün.
  3. (Nutzer) Auf beiden iPhones 10 bekannte Produkte nacheinander ohne Tap.

### F09: Ergebniskarte: +1 und Rückgängig

- **Status:** offen
- **Abhängig von:** F08, B24
- **Referenzen:** architecture.md 6.3
- **Umfang:**
  - Nach erfolgreicher Buchung zeigt eine Karte Name, „vorher → nachher" und zwei Buttons:
    - [+1]: gleiche Art, `product_id`, Menge 1
    - [Rückgängig]: `POST /movements/{id}/reversal` auf die zuletzt angezeigte Buchung
  - Die Karte verschwindet beim nächsten Scan oder nach 10 s. Der Timer pausiert, solange ein Dialog aus der Karte offen ist (F10).
  - Der Zustand liegt in einem reinen Reducer.
- **Nicht im Umfang:** mehrstufiges Rückgängig.
- **Abnahmekriterien:**
  1. Vitest-Tests für den Reducer, inklusive Pause.
  2. `make check` ist grün.
  3. (Nutzer) Bedienung.

### F10: Neues Produkt: Soll-Schnellauswahl und Zusammenführen

- **Status:** offen
- **Abhängig von:** F09, F06c
- **Referenzen:** architecture.md 6.3, 6.5 (Merge und `needs_review`)
- **Umfang:**
  - Bei `product_created: true` zeigt die Karte „Neu: <Name>", bei `needs_review` mit dem Hinweis „Bitte prüfen".
  - Soll-Chips [1] [2] [3] [5] [10] senden `PATCH` nur mit `target`.
  - „Name ändern" öffnet `/produkt/:id`.
  - „Stattdessen zu vorhandenem Produkt" öffnet dieselbe Auswahlliste wie F06c; der Timer der Karte pausiert dabei. Die Auswahl sendet `POST /products/{neu}/merge`, danach zeigt die Karte das Zielprodukt.
- **Nicht im Umfang:** Produkt beim Scannen umbenennen.
- **Abnahmekriterien:**
  1. `make check` ist grün.
  2. (Nutzer) Unbekanntes Produkt scannen, Soll wählen, zusammenführen.

### F11: Manuelle Eingabe

- **Status:** offen
- **Abhängig von:** F08, F06b
- **Referenzen:** architecture.md 7.1
- **Umfang:**
  - Button „Code eintippen" in der Scanner-Ansicht öffnet ein Feld mit `inputmode="numeric"`.
  - Leerzeichen werden vor der Prüfung entfernt; die API selbst akzeptiert nur Ziffern. Prüfung mit `normalizeGtin`. Das Absenden verhält sich wie ein Scan.
- **Nicht im Umfang:** Suche nach Namen.
- **Abnahmekriterien:**
  1. Vitest-Test: Eine ungültige Prüfziffer wird vor dem Senden abgelehnt.
  2. `make check` ist grün.

### F12: Einkauf

- **Status:** offen
- **Abhängig von:** F03, B26
- **Referenzen:** architecture.md 6.2 (`getShoppingList`)
- **Umfang:**
  - `GET /shopping-list` als Liste „3 × Kidneybohnen" mit Marke klein. Ist die Liste leer: „Alles da".
  - Button „Als Text teilen": Text aus der reinen Funktion `shoppingText(items)`, Zeilen „3 × Kidneybohnen". Senden über `navigator.share`; ohne Share-API wird der Text in die Zwischenablage kopiert und ein kurzer Hinweis gezeigt.
- **Nicht im Umfang:** Abhaken, Bring!.
- **Abnahmekriterien:**
  1. Vitest-Test für `shoppingText`.
  2. `make check` ist grün.

### F13a: App-Icons

- **Status:** offen
- **Abhängig von:** F01
- **Referenzen:** keine
- **Umfang:** Go-Programm `tools/icongen/main.go` (nur Standardbibliothek):
  - Motiv: Hintergrund `#15803d`, drei weiße waagerechte Balken als Regal, mittig mit 15 % Rand.
  - Ausgabe als PNG in drei Größen: `web/public/apple-touch-icon.png` (180), `web/public/icon-192.png` und `web/public/icon-512.png`.
  - Aufruf über `go run ./tools/icongen -out web/public`.
- **Nicht im Umfang:** andere Motive.
- **Abnahmekriterien:**
  1. Ein Go-Test prüft Größe und Eckfarbe der drei PNGs.
  2. `make check` ist grün.

### F13b: PWA

- **Status:** offen
- **Abhängig von:** F12, F07, F13a
- **Referenzen:** research.md 7.5, architecture.md 4.3
- **Umfang:**
  - **Service Worker** mit `vite-plugin-pwa`, Strategie `generateSW`, `registerType: "autoUpdate"` und `injectRegister: "script"` (externe `registerSW.js`, kein Inline-Skript wegen der CSP):
    - Precache aller Build-Dateien inklusive `zxing_reader.wasm` (in `globPatterns` enthalten; das Standardlimit von 2 MiB reicht).
    - `navigateFallbackDenylist: [/^\/api\//]`. `/api/` wird nie gecacht.
  - **Manifest:** Name und Kurzname „StashBert", `display: standalone`, `start_url: /`, `scope: /`, `theme_color: #15803d`, Icons aus F13a.
  - **`index.html`:** `apple-touch-icon`, `theme-color`, `viewport-fit=cover`.
  - **Update-Hinweis:** Bei einer neuen Service-Worker-Version erscheint ein Banner „Neue Version verfügbar, tippen zum Aktualisieren".
- **Nicht im Umfang:** Offline-Buchen, Push.
- **Abnahmekriterien:**
  1. Der Build enthält Manifest und Service Worker, und die Precache-Liste enthält `zxing_reader.wasm` (Test oder Skript prüft das Build-Ergebnis).
  2. `make check` ist grün.
  3. (Nutzer) Installation auf dem Home-Bildschirm beider iPhones, Start im Standalone-Modus.

---

## Phase 1c: Auslieferung

### R01: Dockerfile und Healthcheck

- **Status:** offen
- **Abhängig von:** B29
- **Referenzen:** ADR-0010, architecture.md 9.1
- **Umfang:**
  - Flag `-healthcheck` in `cmd/stashbert`: ruft `http://127.0.0.1:$PORT/api/v1/health` auf und endet mit 0 bei 200, sonst mit 1.
  - **`Dockerfile`, mehrstufig:**
    1. `node:24-alpine`: Web bauen, falls `web/package.json` existiert.
    2. `golang:1.27-alpine`: mit `CGO_ENABLED=0` bauen, Web-Build nach `internal/webui/dist/`, Version per ldflags. Zusätzlich ein leeres Verzeichnis `/data` anlegen.
    3. `gcr.io/distroless/static-debian13:nonroot` mit dem Binary, dazu `COPY --chown=65532:65532` des leeren `/data`.
    - `EXPOSE 8080`, `ENV DATA_DIR=/data`, `ENTRYPOINT ["/stashbert"]`.
  - `make docker`.
- **Nicht im Umfang:** Push, Multi-Arch (R03).
- **Abnahmekriterien:**
  1. `make docker` ist erfolgreich.
  2. Ein Container mit neuem Named Volume auf `/data` startet und legt die DB an; `-healthcheck` liefert 0.
  3. Das Image ist kleiner als 40 MB.
  4. `make check` ist grün.

### R02: Compose und Betriebsanleitung

- **Status:** offen
- **Abhängig von:** R01
- **Referenzen:** architecture.md 9
- **Umfang:**
  - **`deploy/compose.yaml`:** ein Dienst, Image `ghcr.io/schmitz-chris/stashbert:${STASHBERT_VERSION:-latest}`, Port `8080:8080`, `env_file: .env`, Volume `./data:/data`, Healthcheck mit `["/stashbert", "-healthcheck"]`.
  - **`deploy/.env.example`** mit allen Variablen aus 9.2 und `STASHBERT_VERSION`.
  - **`docs/betrieb.md`:** Start, Rechte für `./data` (`chown 65532:65532`), Anforderungen an den Proxy (9.1), Update über eine neue `STASHBERT_VERSION`, Backup und Restore (9.3), Logs.
- **Nicht im Umfang:** Reverse-Proxy-Konfiguration.
- **Abnahmekriterien:**
  1. `docker compose -f deploy/compose.yaml config` ist gültig.
  2. (Nutzer) In einem leeren Verzeichnis führt die Anleitung zu einer laufenden Instanz.

### R03: CI: Image bauen

- **Status:** offen
- **Abhängig von:** R01, F01
- **Referenzen:** ADR-0003, ADR-0012
- **Umfang:**
  - Bei Tags `v*` ein Multi-Arch-Image (amd64, arm64) mit Buildx bauen und nach `ghcr.io/schmitz-chris/stashbert` pushen, mit den Tags `<version>` (ohne `v`) und `latest`.
  - Aktions-Versionen vorher nachschlagen.
- **Nicht im Umfang:** Breaking-Change-Prüfung der API (nach M1).
- **Abnahmekriterien:**
  1. (Nutzer) Nach dem Push ist die CI auf `main` grün.
  2. (Nutzer) Ein Test-Tag erzeugt das Image in GHCR.

### R04: Restore-Test

- **Status:** offen
- **Abhängig von:** B28, B16b
- **Referenzen:** architecture.md 9.3
- **Umfang:** Ein Go-Test in `internal/app/restore_test.go`:
  1. Daten über die Fachlogik anlegen (Produkt, Barcode, Buchungen).
  2. `backup.Run` ausführen.
  3. Die Backup-Datei als `stashbert.db` in ein neues Verzeichnis kopieren.
  4. `app.OpenAndMigrate` mit `store.Migrations` aufrufen, dann Produkte, Barcodes und Buchungen vergleichen.
- **Nicht im Umfang:** ein Restore-Befehl.
- **Abnahmekriterien:**
  1. Der Test ist grün.
  2. `make check` ist grün.

### R05: Inbetriebnahme und Abnahme M1

- **Status:** offen
- **Wer:** Nutzer, mit Unterstützung durch einen Agenten
- **Abhängig von:** alle anderen Tasks
- **Umfang:**
  - StashBert im LXC nach `docs/betrieb.md` starten, den eigenen Proxy einrichten und die PWA auf beiden iPhones installieren.
  - `docs/m1-abnahme.md` mit den Ergebnissen der Abnahmekriterien unten.
- **Abnahmekriterien M1:**
  1. Bekanntes Produkt entnehmen: vom Erkennen bis „3 → 2" unter 500 ms im WLAN; danach ohne Tap scanbereit.
  2. 10 verschiedene bekannte Produkte nacheinander in unter 30 s.
  3. Unbekanntes Produkt mit OFF-Treffer: keine Pflichteingabe, Name gesetzt, das Bild erscheint nachträglich.
  4. Ohne Internet: Platzhalter, Bestand +1; nach dem Setzen des Solls über die Chips und der Rückkehr des Internets wird das Produkt trotzdem ergänzt.
  5. Soll 5 und Ist 2 ergeben auf der Einkaufsseite „3 × Kidneybohnen".
  6. Ein Backup lässt sich zurückspielen (R04 plus einmal manuell).
  7. Installation auf dem Home-Bildschirm beider iPhones.
  8. Derselbe Code innerhalb von 2 s wird nur einmal gebucht; [+1] und [Rückgängig] funktionieren.
  9. Nach 2 bis 3 Wochen Nutzung: Liste und Regal verglichen, Ergebnis notiert. Das ist die Kernfrage der Erprobung.

---

## Später (bewusst nicht Teil dieses Plans)

Wird erst nach der M1-Abnahme geplant. Agenten bauen davon nichts vor.

- M2:
  - API-Tokens
  - Outbox und MQTT 5 mit HA-Discovery
  - Bring! über HA mit wählbarer Liste (research.md, Kapitel 10 und 11)
  - Summary-Endpunkt
  - Breaking-Change-Prüfung der API
- M3: ESP32-Scanner (research.md, Kapitel 9).
- Weitere Optionen:
  - OIDC
  - Kategorien, Lagerorte
  - MHD
  - Mindestbestand in der Oberfläche
  - Offline-Buchen
  - Kurzbefehl
  - Swift-Hülle
  - Doku-Oberfläche für die API
