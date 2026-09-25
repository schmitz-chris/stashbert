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

- **Status:** wartet auf Nutzer
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

- **Status:** erledigt
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

- **Status:** wartet auf Nutzer
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

- **Status:** wartet auf Nutzer
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

- **Status:** wartet auf Nutzer
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

- **Status:** erledigt
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

- **Status:** erledigt
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

- **Status:** erledigt
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

- **Status:** wartet auf Nutzer
- **Abhängig von:** F12, F07, F13a
- **Referenzen:** research.md 7.5, architecture.md 4.3
- **Umfang:**
  - **Service Worker** mit `vite-plugin-pwa`, Strategie `generateSW`, `registerType: "prompt"` und `injectRegister: false`; registriert wird im App-Code über `useRegisterSW` aus `virtual:pwa-register/react` (gebündelt, also kein Inline-Skript wegen der CSP). `prompt` statt `autoUpdate`, damit eine neue Version nicht mitten im Scannen die Seite neu lädt:
    - Precache aller Build-Dateien inklusive `zxing_reader.wasm` (in `globPatterns` enthalten; das Standardlimit von 2 MiB reicht).
    - `navigateFallbackDenylist: [/^\/api\//]`. `/api/` wird nie gecacht.
  - **Manifest:** Name und Kurzname „StashBert", `display: standalone`, `start_url: /`, `scope: /`, `theme_color` wie die Hintergrundfarbe (seit F23, heute `#f4f3f1`), Icons aus F13a.
  - **`index.html`:** `apple-touch-icon`, `theme-color`, `viewport-fit=cover`.
  - **Update-Hinweis:** Bei einer neuen Service-Worker-Version (`needRefresh` aus `useRegisterSW`) erscheint ein Banner „Neue Version verfügbar, tippen zum Aktualisieren"; der Tap ruft `updateServiceWorker(true)`.
- **Nicht im Umfang:** Offline-Buchen, Push.
- **Abnahmekriterien:**
  1. Der Build enthält Manifest und Service Worker, und die Precache-Liste enthält `zxing_reader.wasm` (Test oder Skript prüft das Build-Ergebnis).
  2. `make check` ist grün.
  3. (Nutzer) Installation auf dem Home-Bildschirm beider iPhones, Start im Standalone-Modus.

### F14: Oberfläche nachbessern

- **Status:** erledigt (Nutzer: „sieht besser aus", 24.09.2026)
- **Abhängig von:** F13b
- **Referenzen:** Rückmeldung des Nutzers vom 24.09.2026 („sieht nicht schön aus, der Knopf für das Licht ist weg")
- **Umfang:**
  - **Scan, Licht:** Ein runder Licht-Knopf (mindestens 48 px) liegt sichtbar unten links auf dem Kamerabild, nur wenn das Gerät Licht kann; an/aus ist klar erkennbar (z. B. gefüllt gelb bzw. dunkel halbtransparent) und hat `aria-pressed`. Im Zahnrad-Menü bleiben Kamerawahl und Zoom; das Licht ist dort nicht mehr doppelt.
  - **Scan, Zahnrad:** Der Knopf ist mindestens 44 × 44 px groß und auf jedem Kamerabild gut sichtbar (runder, dunkler halbtransparenter Hintergrund).
  - **Navigationsleiste:** Über den Beschriftungen Vorrat, Scan und Einkauf je ein einfaches Symbol als Inline-SVG (keine neue Abhängigkeit): Regal bzw. Kiste, Barcode, Einkaufswagen. Der aktive Reiter ist farbig; der Scan-Knopf bleibt mittig und hervorgehoben.
  - **Vorrat-Zeile:** Statt „3 / 0" steht der Bestand groß, darunter klein „Soll 5"; bei Soll 0 entfällt die Zeile.
- **Nicht im Umfang:** Farben, Schriften oder Abstände grundsätzlich ändern, neue Funktionen, App-Icon.
- **Abnahmekriterien:**
  1. `make check` ist grün.
  2. Bildschirmfotos der drei Ansichten und der Produktseite in 390 × 844 zeigen die Änderungen.
  3. (Nutzer) Sieht auf dem iPhone gut aus.

---

## Phase 1c: Auslieferung

### R01: Dockerfile und Healthcheck

- **Status:** erledigt (optionaler Weg nach ADR-0014)
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

- **Status:** wartet auf Nutzer (optionaler Weg nach ADR-0014)
- **Abhängig von:** R01
- **Referenzen:** architecture.md 9
- **Umfang:**
  - **`deploy/compose.yaml`:** ein Dienst, Image `ghcr.io/schmitz-chris/stashbert:${STASHBERT_VERSION:-latest}`, Port `8080:8080`, `env_file: .env`, Volume `./data:/data`, Healthcheck mit `["/stashbert", "-healthcheck"]`.
  - **`deploy/.env.example`** mit allen Variablen aus 9.2 und `STASHBERT_VERSION`.
  - **`docs/betrieb.md`:** Die Anleitung für den LXC (L03) bleibt Hauptteil. Am Ende kommt ein eigener Abschnitt „Alternative: Docker" dazu: Start, Rechte für `./data` (`chown 65532:65532`), Update über eine neue `STASHBERT_VERSION`, Backup und Restore mit dem Container statt systemd, Logs mit `docker compose logs`. Proxy-Anforderungen nur verweisen, nicht wiederholen. Das Image liegt in GHCR eines privaten Repositorys; der Abschnitt nennt `docker login ghcr.io` bzw. alternativ `make docker` für ein lokal gebautes Image.
- **Nicht im Umfang:** Reverse-Proxy-Konfiguration.
- **Abnahmekriterien:**
  1. `docker compose -f deploy/compose.yaml config` ist gültig.
  2. (Nutzer) In einem leeren Verzeichnis führt die Anleitung zu einer laufenden Instanz.

### R03: CI: Image bauen

- **Status:** wartet auf Nutzer (Workflow `image.yml` steht, `actionlint` ohne Befund, Multi-Arch-Build lokal geprüft; Tag und GHCR prüft der Nutzer)
- **Abhängig von:** R01, F01
- **Referenzen:** ADR-0003, ADR-0012
- **Umfang:**
  - Bei Tags `v*` ein Multi-Arch-Image (amd64, arm64) mit Buildx bauen und nach `ghcr.io/schmitz-chris/stashbert` pushen, mit den Tags `<version>` (ohne `v`) und `latest`.
  - Aktions-Versionen vorher nachschlagen.
- **Nicht im Umfang:** Breaking-Change-Prüfung der API (nach M1).
- **Abnahmekriterien:**
  1. (Nutzer) Nach dem Push ist die CI auf `main` grün.
  2. (Nutzer) Ein Test-Tag erzeugt das Image in GHCR.

### L01: Release-Binary für den LXC

- **Status:** erledigt
- **Abhängig von:** B29, F13b
- **Referenzen:** ADR-0014, architecture.md 9.1
- **Umfang:**
  - Flag `-version` in `cmd/stashbert`: gibt die Version aus (`main.version`) und endet mit 0, ohne Konfiguration zu laden oder etwas zu starten.
  - `make release`: baut wie `make build` zuerst die Web-Oberfläche (falls `web/package.json` existiert) und kopiert sie nach `internal/webui/dist/`, dann `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=<git describe --tags --always --dirty>"` nach `bin/release/stashbert-linux-amd64`, dazu `bin/release/SHA256SUMS` (mit `sha256sum`, auf macOS ersatzweise `shasum -a 256`). `bin/` ist schon ignoriert.
  - Nach dem Build bleibt `internal/webui/dist/` bis auf `.gitkeep` leer (anders als `make build`, das die Oberfläche dort liegen lässt; der Ordner ist ignoriert).
- **Nicht im Umfang:** arm64, GitHub-Releases, Signaturen.
- **Abnahmekriterien:**
  1. Ein Go-Test prüft `-version` (z. B. über eine testbare Funktion statt `os.Exit`).
  2. `make release` erzeugt ein statisch gelinktes ELF-Binary für x86-64 (`file` zeigt es) mit eingebetteter Web-Oberfläche; die Prüfsumme stimmt (`shasum -c` bzw. `sha256sum -c`).
  3. `make check` ist grün.

### L02: systemd-Unit und Installationsskript

- **Status:** wartet auf Nutzer
- **Abhängig von:** L01
- **Referenzen:** ADR-0014, architecture.md 9
- **Umfang:**
  - **`deploy/stashbert.service`:** `User=stashbert`, `Group=stashbert`, `StateDirectory=stashbert`, `Environment=DATA_DIR=/var/lib/stashbert`, `EnvironmentFile=/etc/stashbert/stashbert.env`, `ExecStart=/usr/local/bin/stashbert`, `Restart=on-failure`, `RestartSec=5`, `NoNewPrivileges=yes`, `After=network-online.target`, `Wants=network-online.target`, `WantedBy=multi-user.target`. Weitergehende Schutzoptionen, die Namensräume brauchen (`ProtectSystem`, `PrivateTmp`, `ProtectHome` usw.), nur **auskommentiert** mit dem Hinweis, dass sie im LXC Nesting brauchen.
  - **`deploy/stashbert.env.example`:** alle Variablen aus 9.2 außer `DATA_DIR`, mit Kommentaren; `OFF_CONTACT` leer.
  - **`deploy/install.sh`** (POSIX `sh`, als root im Container, idempotent, `set -eu`): Aufruf `sh install.sh <pfad-zum-binary>`. Prüft root und dass das Binary mit `-version` läuft; legt den Systembenutzer `stashbert` an, falls er fehlt (`useradd --system --home-dir /var/lib/stashbert --shell /usr/sbin/nologin`); installiert das Binary mit Modus 0755 nach `/usr/local/bin/stashbert`; installiert die Unit; legt `/etc/stashbert/stashbert.env` aus dem Beispiel an, **nur wenn sie fehlt** (Modus 0640, Gruppe `stashbert`); `systemctl daemon-reload`; beim ersten Mal `systemctl enable --now stashbert`, sonst `systemctl restart stashbert`; zeigt am Ende Version und `systemctl status` kurz an. Unit und Beispiel liegen neben dem Skript (Pfade relativ zum Skript).
  - `make check` prüft die Syntax des Skripts mit `sh -n`.
- **Nicht im Umfang:** Container anlegen, Reverse Proxy, Deinstallationsskript.
- **Abnahmekriterien:**
  1. `make check` ist grün (inklusive `sh -n`).
  2. (Nutzer) In einem frischen Debian-13-LXC führt `install.sh` zu einem laufenden Dienst; ein zweiter Aufruf mit neuem Binary aktualisiert ihn, ohne die Env-Datei zu überschreiben.

### L03: Betriebsanleitung für den LXC

- **Status:** wartet auf Nutzer
- **Abhängig von:** L02
- **Referenzen:** ADR-0014, architecture.md 8, 9
- **Umfang:** `docs/betrieb.md` auf Deutsch:
  - Container anlegen (Empfehlung: Debian-13-Vorlage, unprivilegiert, 1 Kern, 512 MB RAM, 8 GB Platte, feste IP bzw. DHCP-Reservierung), ohne Proxmox-Klickanleitung im Detail.
  - Binary bauen (`make release`), per `scp` samt `deploy/` in den Container kopieren, `install.sh` ausführen, Env-Datei anpassen (`OFF_CONTACT`), Dienst neu starten.
  - Anforderungen an den Reverse Proxy (9.1), Kamera nur über HTTPS mit vertrauenswürdigem Zertifikat, Kamera-Freigabe in Safari dauerhaft erlauben.
  - Update, Logs (`journalctl -u stashbert`), Backup (eigene Backups in `DATA_DIR/backups`, zusätzlich Proxmox-Snapshots), Restore nach 9.3, Deinstallation in wenigen Befehlen.
- **Nicht im Umfang:** Docker, Reverse-Proxy-Konfiguration im Detail.
- **Abnahmekriterien:**
  1. Alle Befehle in der Anleitung passen zu `install.sh`, Unit und Makefile (gegenlesen).
  2. (Nutzer) Die Anleitung führt zu einer laufenden Instanz.

### R04: Restore-Test

- **Status:** erledigt
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

- **Status:** wartet auf Nutzer (Vorlage `docs/m1-abnahme.md` liegt bereit)
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

## Phase 1d: Vormerken für den Einkauf (ADR-0015)

### B30: Vormerkung im Datenmodell und in der Einkaufsliste

- **Status:** erledigt
- **Abhängig von:** B26, B25
- **Referenzen:** ADR-0015, architecture.md 5, 6.2, 6.3, 6.5
- **Umfang:**
  - Migration `0003_marked.sql`: Spalte `products.marked INTEGER NOT NULL DEFAULT 0 CHECK (marked IN (0, 1))`, mit Down-Migration.
  - `marked` als Pflichtfeld (boolean, nur lesen) in `Product` und `ShoppingItem` der Spec.
  - `getShoppingList`: Produkte mit `missing > 0` oder `marked`; `missing` bleibt die berechnete Menge (bei nur vorgemerkten Produkten 0).
  - Eine Buchung `add` setzt `marked = 0` in derselben Transaktion (per Barcode, per `product_id`, bei neu angelegten Produkten, bei +1). Inventur, Entnehmen, Storno und Zusammenführen ändern die Vormerkung nicht, außer: beim Zusammenführen ist das Ziel vorgemerkt, wenn Quelle oder Ziel es war.
  - Keine neuen Ereignisse (ADR-0015).
- **Nicht im Umfang:** Endpunkte zum Vormerken (B31), Oberfläche.
- **Abnahmekriterien (Tests):**
  1. Ein vorgemerktes Produkt ohne Fehlbestand steht mit `missing: 0` und `marked: true` auf der Liste; ein nicht vorgemerktes ohne Fehlbestand nicht.
  2. `add` beendet die Vormerkung, `consume` und `inventory` nicht.
  3. Zusammenführen übernimmt die Vormerkung der Quelle.
  4. Migration hin und zurück funktioniert; `make check` ist grün.

### B31: Endpunkte zum Vormerken

- **Status:** erledigt
- **Abhängig von:** B30, B20
- **Referenzen:** ADR-0015, architecture.md 6.2 (`markShoppingItem`, `unmarkShoppingItem`), 6.5 (`MarkResult`), 7.2
- **Umfang:**
  - `POST /shopping-list/items` mit `{product_id}` oder `{barcode}` (genau eines, sonst 400 `invalid_request`; Barcode normalisieren, ungültig 422 `invalid_barcode`; unbekanntes `product_id` 404). Setzt `marked = 1`, ändert nie `stock` und legt keine Buchung an. `updated_at` ändert sich nur, wenn sich `marked` ändert.
  - Unbekannter Barcode: Produkt anlegen wie in B20 (Lookup außerhalb der Schreibtransaktion, Cache, Platzhalter, `product.created`), aber **ohne Buchung**, Bestand 0, `marked = 1`. Die Anlege-Logik aus B20 wiederverwenden, nicht kopieren.
  - Antwort 200 `MarkResult` mit `message` nach 6.5.
  - `DELETE /shopping-list/items/{product_id}`: setzt `marked = 0`, 204; unbekanntes Produkt 404; nicht vorgemerktes Produkt ebenfalls 204.
- **Nicht im Umfang:** Idempotency-Key (Vormerken ist von sich aus wiederholbar), Ereignisse, Oberfläche.
- **Abnahmekriterien (Tests):**
  1. Vormerken per Barcode und per `product_id` ändert weder Bestand noch Buchungen.
  2. Unbekannter Barcode legt ein Produkt mit Bestand 0 und `marked` an, ohne Buchung; lokaler Code ohne Lookup.
  3. `already_listed` und `message` für die drei Fälle.
  4. Entfernen; Fehlerfälle.
  5. `make check` ist grün.

### F15: Scan-Modus „Einkaufen"

- **Status:** wartet auf Nutzer
- **Abhängig von:** B31, F10
- **Referenzen:** ADR-0015, F08 bis F11
- **Umfang:**
  - Dritter Modus im Schalter: „Einlagern | Entnehmen | Einkaufen", Farbe für Einkaufen orange (Tailwind `amber`), gespeichert wie bisher.
  - Scan und „Code eintippen" im Modus Einkaufen rufen `markShoppingItem` per Barcode auf. Rückmeldung über `feedbackFor` erweitert: Erfolg orange mit eigenem Ton `mark` (in `TONES`: 2 × 1320 Hz, je 60 ms), Meldung `message` der Antwort; Fehler wie bisher.
  - Ergebniskarte im Modus Einkaufen: Name und „vorgemerkt" bzw. „schon auf der Liste", nur ein Button [Rückgängig] (entfernt die Vormerkung per `unmarkShoppingItem`, nur wenn die Vormerkung durch diesen Scan entstand, sonst kein Button). Kein [+1], keine Soll-Chips.
  - Caches aktualisieren (Produkt, Liste, Einkaufsliste).
- **Nicht im Umfang:** Einkaufsansicht (F16).
- **Abnahmekriterien:**
  1. Vitest-Tests für die erweiterte `feedbackFor`, den Ton und die Kartenlogik im Modus Einkaufen.
  2. `make check` ist grün.
  3. (Nutzer) Bedienung auf dem iPhone.

### F16: Vormerkungen in Einkauf und Vorrat

- **Status:** erledigt
- **Abhängig von:** B31, F12
- **Referenzen:** ADR-0015
- **Umfang:**
  - Einkaufsansicht: vorgemerkte Produkte ohne Fehlbestand erscheinen ohne Mengenangabe (der Hinweis „vorgemerkt" entfiel am 24.09.2026, weil der orange Wagen-Knopf die Vormerkung zeigt); vorgemerkte mit Fehlbestand wie bisher mit Menge. Jeder vorgemerkte Eintrag hat „Von der Liste nehmen" (entfernt nur die Vormerkung). `shoppingText` nimmt vorgemerkte Einträge ohne Menge als reinen Namen auf.
  - Vorrat: der Filter „Nachkaufen" zeigt `missing > 0` oder `marked`; vorgemerkte Zeilen haben einen kleinen Hinweis.
  - Produktseite: Hinweis „vorgemerkt" mit „Von der Liste nehmen" bzw. Button „Vormerken" (per `product_id`).
- **Nicht im Umfang:** Mengen für Vormerkungen.
- **Abnahmekriterien:**
  1. Vitest-Tests für `shoppingText` und den Filter mit Vormerkungen.
  2. `make check` ist grün.

## Phase 1e: Falsche Produktbilder

Anlass: Open Food Facts lieferte zu einem Barcode ein falsches Produkt samt Bild (Bruschetta statt Blueberry Jam). Name und Marke lassen sich schon ändern, das Bild nicht.

### B32: Produktbild hochladen und entfernen

- **Status:** erledigt
- **Abhängig von:** B22b
- **Referenzen:** architecture.md 6.1, 6.2 (`uploadProductImage`, `deleteProductImage`), 6.4 (`invalid_image`), 7.3
- **Umfang:**
  - `PUT /products/{id}/image`: Body ist das Bild; in der Spec als `image/*` (oapi-codegen erzeugt für mehrere Binär-Typen doppelte Felder), die erlaubten Typen `image/jpeg`, `image/png`, `image/webp` und die Grenze von 2 MB stehen in der `description`. Die 2 MB begrenzt ein `http.MaxBytesReader` nur für diese Route vor dem Validator (architecture.md 4.4); `*http.MaxBytesError` wird 422 `invalid_image`. Die Fachlogik prüft zusätzlich mit Begrenzung (höchstens 2 MB + 1 Byte). Der Inhalt wird per `http.DetectContentType` geprüft und muss zu einem der drei Typen passen (sonst 422 `invalid_image`); die Dateiendung folgt dem erkannten Typ. Speichern über eine temporäre Datei plus `rename` unter dem eindeutigen Namen `<id>-upload-<unix-millis>.<ext>` im Bildverzeichnis (so kann der Bild-Job, der nur `<id>.<ext>` schreibt, ein Foto nicht überschreiben); die vorherige Datei wird nach dem Commit entfernt. In der Datenbank `image_file` setzen, `image_source_url` auf `NULL`, `updated_at`. Antwort 200 `Product`. Unbekanntes Produkt 404 (dann keine Datei zurücklassen).
  - Der Request-Validator bleibt aktiv; ohne `schema` prüft kin-openapi nur den Content-Type gegen `image/*`.
  - `DELETE /products/{id}/image`: Datei löschen (fehlende Datei ist kein Fehler), `image_file` und `image_source_url` auf `NULL`, `updated_at`. 204; unbekanntes Produkt 404; Produkt ohne Bild ebenfalls 204.
  - Der Bild-Job aus B22a überschreibt ein hochgeladenes Foto nicht (er lädt nur bei `image_source_url` ohne `image_file`); prüfen und testen.
- **Nicht im Umfang:** Verkleinern auf dem Server, weitere Formate (HEIC), mehrere Bilder pro Produkt.
- **Abnahmekriterien (Tests):**
  1. Hochladen von JPEG, PNG und WebP; danach liefert `getProductImage` das neue Bild mit richtigem Typ; `has_image` ist true.
  2. Falscher Inhalt (z. B. Text mit `Content-Type: image/jpeg`) und zu große Dateien ergeben 422 ohne Änderung.
  3. Ersetzen eines Bildes (auch eines OFF-Bildes `<id>.<ext>`) hinterlässt nur die neue Datei; ein Body über 2 MB ergibt 422, ohne dass der Handler ihn einliest.
  4. Entfernen löscht Datei und Bildquelle; der Bild-Job lädt danach nichts nach.
  5. `make check` ist grün.

### F17: Bild auf der Produktseite ersetzen oder entfernen

- **Status:** wartet auf Nutzer
- **Abhängig von:** B32, F06a
- **Referenzen:** architecture.md 7.3 (Verkleinern im Frontend)
- **Umfang:**
  - Unter dem Bild bzw. dem Platzhalter der Produktseite zwei Knöpfe: „Foto aufnehmen" (`<input type="file" accept="image/*" capture="environment">`, versteckt hinter einem Knopf) und, wenn ein Bild da ist, „Bild entfernen" (mit Bestätigungsdialog über `components/Dialog.tsx`).
  - Vor dem Hochladen verkleinert das Frontend das Foto: längste Kante höchstens 1024 px, JPEG mit Qualität 0,8, über `createImageBitmap` und ein Canvas (`toBlob`). Die Berechnung der Zielgröße als reine Funktion in `web/src/lib/`. Hochladen per `PUT` über den generierten Client (Body als `Blob`, Content-Type `image/jpeg`), mit Zeitlimit 30 s.
  - Nach Erfolg Produkt-Caches aktualisieren; das Bild wird neu geladen (z. B. Cache-Buster aus `updated_at` an der Bild-URL, auch in Vorrat und Ergebniskarte, wo das Bild erscheint).
  - Fehler als kurze Meldung („Foto zu groß oder ungültig", „Hochladen fehlgeschlagen").
- **Nicht im Umfang:** Zuschneiden, Drehen, Galerie mehrerer Bilder.
- **Abnahmekriterien:**
  1. Vitest-Tests für die Zielgröße (Hochformat, Querformat, kleiner als 1024 bleibt gleich) und den Cache-Buster.
  2. `make check` ist grün.
  3. (Nutzer) Auf dem iPhone ein falsches Bild durch ein eigenes Foto ersetzen und ein Bild entfernen.

## Phase 1f: Vormerken direkt im Vorrat

### F18: Einkaufswagen-Knopf in der Vorrat-Liste

- **Status:** wartet auf Nutzer
- **Abhängig von:** F16
- **Referenzen:** ADR-0015; Wunsch des Nutzers vom 24.09.2026 (Variante A: Knopf in jeder Zeile)
- **Umfang:**
  - In jeder Zeile der Vorrat-Liste links neben [−] und [+] ein Knopf mit Einkaufswagen-Symbol (Inline-SVG, keine neue Abhängigkeit), Tippfläche mindestens 44 × 44 px, `aria-pressed` = `marked`, `aria-label` „Auf die Einkaufsliste: <Name>" bzw. „Von der Einkaufsliste nehmen: <Name>".
  - Der Knopf zeigt nur die Vormerkung: nicht vorgemerkt als Umriss-Symbol, vorgemerkt orange (`amber`, wie der Modus Einkaufen) mit Häkchen im Symbol. Ein Tipp merkt vor (`markShoppingItem` mit `product_id`), ein zweiter entfernt die Vormerkung (`unmarkShoppingItem`). Ein Fehlbestand nach Soll ändert den Knopf nicht.
  - Der Tipp löst keine Navigation aus; während die Anfrage läuft, ist der Knopf der Zeile gesperrt; Fehler als kurze Meldung an der Zeile wie bei −/+. Caches wie in F16 aktualisieren (Liste, Detail, Einkaufsliste).
  - Der Hinweis „vorgemerkt" in der Zeile (aus F16) entfällt, weil der Knopf den Zustand zeigt.
  - Die drei Knöpfe dürfen optisch etwas schmaler werden (z. B. 40 px Symbolfläche), die Tippfläche bleibt mindestens 44 px (z. B. über Padding oder Überlappung der Klickfläche); Name und Marke bekommen den restlichen Platz.
- **Nicht im Umfang:** Wischgesten, Auswahlmodus, Änderungen am Backend.
- **Abnahmekriterien:**
  1. Vitest-Test für die Beschriftungs- bzw. Zustandslogik des Knopfes (reine Funktion).
  2. `make check` ist grün.
  3. (Nutzer) Auf dem iPhone vormerken und wieder entfernen, ohne die Zeile zu öffnen.

## Phase 1g: Oberfläche nach Apples HIG (ADR-0016)

Grundlage: `docs/hig-pruefung.md` (Befunde H1 bis N11). Kein Dark Mode. Jeder Task, der Layout ändert, prüft mit Bildschirmfotos in 320, 390 und 402 px Breite und mit großer Schrift (z. B. per CDP `Page.setFontSizes`).

### F19: Farbrollen, Kontrast und Druckzustand

- **Status:** erledigt
- **Abhängig von:** F18
- **Referenzen:** ADR-0016; hig-pruefung.md H3, M2, M7, N8
- **Umfang:**
  - Farbrollen als Variablen in `web/src/index.css` (`@theme`): Akzent (Primärgrün, Weiß darauf mindestens 4,5:1, z. B. `emerald-700`), Entnehmen (Blau, Weiß darauf mindestens 4,5:1, z. B. `sky-700`), Vorgemerkt (Amber), Warnung (Gelb), Fehler und Zerstörendes (Rot). Alle Stellen im Frontend auf diese Rollen umstellen; Amber nur noch für „vorgemerkt", Fehler überall rot, „Bitte prüfen" als Warnung.
  - Rahmen von Eingabefeldern und Rahmenknöpfen mit mindestens 3:1 Kontrast zum Hintergrund.
  - Einheitlicher Druckzustand (`active:`) für alle Knöpfe und tippbaren Zeilen, zentral als Utility in `index.css`.
  - Scan-Meldung zusätzlich mit Symbol je Rückmeldung (Haken, Warenkorb, Ausrufezeichen, Kreuz) als Inline-SVG.
  - `color-scheme: light` bzw. `<meta name="color-scheme" content="light">`.
  - AGENTS.md, Frontend-Regeln: Farben nur über die Rollen, Druckzustand für Tippbares.
- **Nicht im Umfang:** Dark Mode, Layout der Zeilen (F20).
- **Abnahmekriterien:**
  1. Ein Vitest-Test prüft die Kontraste der Rollenfarben gegen Weiß bzw. Hintergrund (Farbwerte als Konstanten, Formel nach WCAG).
  2. `make check` ist grün; Bildschirmfotos zeigen die neuen Farben.

### F20: Zeilen neu ordnen

- **Status:** wartet auf Nutzer
- **Abhängig von:** F19
- **Referenzen:** hig-pruefung.md H2, M1, M9, N5
- **Umfang:**
  - Vorrat-Zeile zweistufig: oben Bild, Name (bis zwei Zeilen) und Marke über die volle Breite; darunter rechtsbündig die Gruppe [−] Bestand/Soll [+] und, mit mindestens 12 px Abstand abgesetzt, der Einkaufswagen. Tippflächen mindestens 44 px, keine aneinanderstoßenden Tippflächen verschiedener Aktionen.
  - Einkaufsliste: „Von der Liste nehmen" als kompakter Symbolknopf (Warenkorb mit Haken, `aria-label`); nach dem Entfernen einige Sekunden eine Leiste „Entfernt: <Name>" mit „Rückgängig" (vormerken erneut).
  - Suchfeld im Vorrat mit Lupe links und Löschen-Knopf rechts; Suche und Filter bleiben beim Scrollen oben sichtbar; Filter als `role="radiogroup"` mit `aria-checked`.
- **Nicht im Umfang:** Produktseite (F22), Schriftgrößen (F21).
- **Abnahmekriterien:**
  1. Vitest-Tests für ausgelagerte Logik (z. B. Rückgängig-Leiste).
  2. `make check` ist grün; Bildschirmfotos in 320, 390 und 402 px zeigen vollständig lesbare Namen (bis zwei Zeilen).
  3. (Nutzer) Bedienung auf dem iPhone.

### F21: Dynamic Type und Meldungen ohne Zeitgrenze

- **Status:** wartet auf Nutzer
- **Abhängig von:** F20
- **Referenzen:** ADR-0016; hig-pruefung.md H1, M6
- **Umfang:**
  - `html { font: -apple-system-body }` in einem `@supports`-Block, sodass die Wurzelschrift auf iOS der Systemtextgröße folgt; Desktop-Safari (13 px) ausnehmen, z. B. per Medienabfrage auf Zeigegeräte. Alle Größen in `rem`.
  - Tab-Leiste und Modus-Schalter mit festen Größen bzw. Obergrenze, damit sie bei großer Schrift nicht umbrechen; Kamera-Overlays (Zahnrad, Licht) ebenfalls fest.
  - Layouts von Vorrat, Einkauf, Scan und Produktseite bis etwa 200 % Schrift nutzbar (umbrechen statt abschneiden).
  - Fehlermeldungen bleiben stehen bis zur nächsten Aktion in derselben Zeile bzw. Ansicht (Erfolgsmeldungen dürfen weiter nach 2 s verschwinden). Die Ergebniskarte bleibt bis zum nächsten Scan oder bis „Schließen" (neuer Knopf „×" mit `aria-label`); der 10-s-Timer aus F09 entfällt, Pause-Logik nur, soweit noch nötig.
- **Nicht im Umfang:** eigene Schriftgrößen-Einstellung.
- **Abnahmekriterien:**
  1. Vitest-Tests für den geänderten Karten-Reducer.
  2. `make check` ist grün; Bildschirmfotos mit 22 px und 32 px Grundschrift zeigen keine abgeschnittenen Bedienelemente.
  3. (Nutzer) Auf dem iPhone mit großer Textgröße (Einstellungen, Anzeige und Helligkeit, Textgröße) nutzbar.

### F22: Produktseite aufräumen

- **Status:** wartet auf Nutzer
- **Abhängig von:** F21
- **Referenzen:** hig-pruefung.md M3, M4, N6
- **Umfang:**
  - Die Produktseite liegt im Tab-Layout (Tab-Leiste sichtbar). Oben eine fixierte schmale Navigationszeile mit Zurück samt Herkunft („‹ Vorrat", „‹ Einkauf", „‹ Scan", Rückfall „‹ Vorrat") und dem Produktnamen; Name über dem Bild.
  - Nur „Speichern" als gefüllter Hauptknopf; „Hinzufügen", „Bestand setzen", „Vormerken" als Sekundärknöpfe. Verlauf zeigt drei Einträge mit „Alle anzeigen" (bis zehn). Barcodes und „Produkt löschen" in einem aufklappbaren Bereich (`details`/`summary`).
  - Bestätigungsdialoge: zerstörende Aktion als rote Schrift ohne Füllung, „Abbrechen" gleichwertig. Die Auswahl beim Zusammenführen als Sheet von unten mit „Abbrechen" oben links (weiter `<dialog>`). Im Kamera-Dialog „Schließen" statt „Fertig".
- **Nicht im Umfang:** Wischen zum Schließen von Sheets.
- **Abnahmekriterien:**
  1. Router-Test angepasst (Produktseite unter der Layout-Route).
  2. `make check` ist grün; Bildschirmfotos der Produktseite und der Dialoge.
  3. (Nutzer) Bedienung.

### F23: App-Icon, Favicon, Startbild und Statusleiste

- **Status:** wartet auf Nutzer
- **Abhängig von:** F13a
- **Referenzen:** ADR-0016; hig-pruefung.md M8, N4, N3, N2
- **Umfang:**
  - `tools/icongen`: neues Motiv, vollflächiges Grün, ein Regalbrett mit zwei bis drei unterschiedlich hohen, vereinfachten Gläsern bzw. Dosen in Weiß (klar lesbar bei 180 px, kein Text), Maße als benannte Konstanten; zusätzlich ein Favicon (32 px PNG) statt des Vite-Logos. Die Tests aus F13a an das Motiv anpassen (Eckfarbe, einige Motivpixel, eingecheckte Dateien entsprechen dem Generator).
  - Startbilder (`apple-touch-startup-image`) als einfarbige Fläche in der Hintergrundfarbe für iPhone 15 (393 × 852 pt, @3x) und iPhone 16 Pro (402 × 874 pt, @3x), erzeugt von icongen.
  - `theme-color` auf die Hintergrundfarbe der App; `html` und `body` mit derselben Hintergrundfarbe.
- **Nicht im Umfang:** Icon-Varianten für dunkel oder getönt (im Web nicht möglich).
- **Abnahmekriterien:**
  1. Go-Tests für Icon und Startbilder.
  2. `make check` ist grün.
  3. (Nutzer) Das Icon auf dem Home-Bildschirm gefällt.

### F24: Barrierefreiheit und Feinschliff

- **Status:** erledigt
- **Abhängig von:** F22
- **Referenzen:** hig-pruefung.md N1, N3, N7, N9, N10
- **Umfang:**
  - Seitentitel je Route (`document.title`, z. B. „Vorrat · StashBert"); nach einer Navigation liegt der Fokus auf der `h1`.
  - Einkaufswagen im Vorrat: feste Beschriftung „Auf der Einkaufsliste: <Name>" plus `aria-pressed` (ersetzt die wechselnde Beschriftung aus F18).
  - Tab-Leiste: gefüllte Symbole für den aktiven Reiter, deutlicherer Auswahlzustand beim Scan-Knopf, leicht transluzenter Hintergrund mit `backdrop-filter`.
  - Kamera abgelehnt: Zweck und Weg nennen („Die Kamera liest nur Barcodes; Bilder verlassen das Gerät nicht. Kamera in den Safari-Einstellungen für diese Seite erlauben.").
  - `prefers-reduced-motion: reduce`: statt Vollflächen-Blitz ein farbiger Rahmen um das Kamerabild.
  - Zuletzt genutzten Reiter merken und beim Start (`/`) dorthin leiten.
- **Nicht im Umfang:** Haptik (im Web nicht möglich), Audio-Kategorie (Gerätetest durch den Nutzer).
- **Abnahmekriterien:**
  1. Vitest-Tests für Titel-Zuordnung und Reiter-Merken.
  2. `make check` ist grün.

## Phase 1h: Bessere Kamera für nahe Barcodes

### F25: Automatische Kamerawahl

- **Status:** wartet auf Nutzer
- **Abhängig von:** F24
- **Referenzen:** AGENTS.md (Kamera), architecture.md 4.3; Quellen: dominikschilling.de/notes/ios-access-all-back-cameras-mediadevices-api/, developer.apple.com/forums/thread/772553 und /thread/776460
- **Umfang:**
  - Reine Funktion `preferredCamera(devices)` in `web/src/lib/scanner/` (Eingabe: `videoinput`-Geräte mit `deviceId` und `label`): wählt eine Rückkamera in der Reihenfolge Triple, Dual-Weitwinkel; sonst keine (dann bleibt die Standard-Rückkamera). Erkennung über Schlüsselwörter in deutschen und englischen Bezeichnungen, ohne Frontkameras, ohne Ultraweitwinkel und Tele allein, ohne die reine Dual-Kamera.
  - Ablauf im Scanner ohne manuelle Wahl: Kamera wie bisher mit `facingMode: "environment"` öffnen (nötig für Erlaubnis und Bezeichnungen), dann `enumerateDevices()`, `preferredCamera` anwenden; gibt es eine bessere Kamera, sauber auf sie umschalten (alte Spur stoppen, nur ein Stream gleichzeitig). Die automatisch gewählte `deviceId` wird getrennt von der manuellen Wahl gemerkt (eigener Schlüssel), damit der nächste Start direkt mit ihr beginnt; scheitert das (Gerät fehlt), zurück auf den Standard und neu wählen.
  - Zoom: Bietet die gewählte virtuelle Kamera laut `getCapabilities().zoom` ein Minimum von 1 und ein Maximum von mindestens 2, wird beim Start Zoom 2 gesetzt (entspricht „1x" der Kamera-App mit Weitwinkel als Ausgangsbild; iOS wechselt beim Annähern trotzdem ins Makro). Liegt das Minimum unter 1, bleibt der Zoom bei 1. Die Entscheidung als reine Funktion mit Tests.
  - Kamera-Menü: Eintrag „Automatisch" oben in der Auswahl (Standard, solange nichts manuell gewählt ist); die Auswahl einer Kamera speichert sie wie bisher als manuelle Wahl, „Automatisch" löscht sie. Unter der Auswahl klein die Bezeichnung der aktiven Kamera und der Zoombereich (hilft beim Test auf dem Gerät).
  - Messwerte im Kamera-Menü (Nutzer-Rückmeldung: Erkennung zum Teil langsam), klein und schlicht: Auflösung des Kamerabilds, Dauer des letzten Lesevorgangs in ms und Mittelwert der letzten 20, Leseversuche pro Sekunde, Dauer der letzten Buchung (vom Erkennen bis zur Serverantwort) in ms. Die Messung selbst (gleitender Mittelwert, Versuche pro Sekunde) als reine Funktion mit Tests; keine Messwerte an den Server.
- **Nicht im Umfang:** Objektivwechsel verhindern, eigene Fokus-Steuerung, Backend.
- **Abnahmekriterien:**
  1. Vitest-Tests für `preferredCamera` mit realistischen deutschen und englischen Bezeichnungen (iPhone 15, iPhone 16 Pro, iPad ohne virtuelle Kameras, Android-ähnliche Bezeichnungen), für die Zoom-Entscheidung und für die Messwerte.
  2. `make check` ist grün.
  3. (Nutzer) Nahe, kleine Barcodes werden auf beiden iPhones scharf gelesen; das Menü zeigt die gewählte Kamera.

## Phase 1i: Vorrat-Liste ruhiger gestalten

### F26: Vorrat-Liste neu gestalten

- **Status:** wartet auf Nutzer
- **Abhängig von:** F25
- **Referenzen:** Nutzer-Rückmeldung vom 24.09.2026 („nicht stimmig"), Entwurf im Chat (Vorschlag rechts), ADR-0016; Quellen: learnui.design/blog/ios-design-guidelines-templates.html, dev.to/flownato/quantity-stepper-ux-zero-stock-limits-and-failed-updates-4emj, mobbin.com/glossary/stepper
- **Umfang:**
  - **Liste:** weiße Karte ohne Rahmen auf dem Hintergrund, Trennlinien eingerückt ab dem Textbeginn (nicht unter dem Bild), großzügigere Innenabstände.
  - **Zeile:** Bild (48 px, runde Ecken), Name, darunter Marke und Packungsgröße („Bonduelle · 400 g", fehlende Teile weglassen). Darunter, auf der Höhe des Textes beginnend, eine Zeile mit links dem Status und rechts den Knöpfen. Der Status ist so breit wie sein Text; nur wenn Status und Knöpfe nicht nebeneinander passen, rücken die Knöpfe in eine eigene Zeile.
  - **Status** (reine Funktion `stockStatus(product)` mit Tests, kein Balken): Bestand 0 ergibt „leer" (Fehlerfarbe, Kreuz-Symbol); `missing > 0` ergibt „fehlt N" (Text in `ink`, kleines Warnsymbol in Warnfarbe); sonst kein Status, weil der Stepper die Zahl schon zeigt und das Soll auf der Produktseite steht (Nutzer-Rückmeldung vom 24.09.2026; ursprünglich „N von Soll" bzw. „N da"). Der Status ist nie nur Farbe. Ob ein Produkt vorgemerkt ist, zeigt nur der Wagen-Knopf (Nutzer-Rückmeldung vom 24.09.2026; ursprünglich folgte „· vorgemerkt").
  - **Knöpfe:** Einkaufswagen und Stepper [−] Zahl [+] als runde, hellgraue Flächen ohne Rahmen (Rolle `fill` o. ä.); sichtbar etwa 34 bis 36 px hoch, Tippfläche weiterhin mindestens 44 × 44 px; die Zahl mit fester Breite (`tabular-nums`), damit nichts springt. Vorgemerkt: hellorange Fläche mit Symbol in der Vorgemerkt-Farbe statt vollflächigem Block. Kontraste der Symbole mindestens 3:1, der Zahl mindestens 4,5:1.
  - **Suche und Filter:** Suchfeld als graue Fläche ohne Rahmen mit Lupe und Löschen-Knopf (wie bisher, nur Stil); Filter als Segment-Schalter (graue Leiste, gewähltes Segment weiß hinterlegt), weiter mit `radiogroup`/`radio`; bei großer Schrift darf er horizontal scrollen.
  - Die Einkaufsliste bekommt denselben Stil für ihren Wagen-Knopf und dieselbe Karten- und Trennlinien-Optik, sonst keine Änderungen.
  - Alle Regeln aus F19 bis F24 bleiben: Farbrollen, `pressable`, Dynamic Type, Fokus, Tippflächen.
- **Nicht im Umfang:** Bestandsbalken (vom Nutzer nicht gewünscht), Gruppierung nach Status, Wischgesten, Produktseite.
- **Abnahmekriterien:**
  1. Vitest-Tests für `stockStatus` (leer, fehlt, kein Status mit und ohne Soll, Mindestbestand-Fälle) und für die Kontraste neuer Farbrollen.
  2. `make check` ist grün; Bildschirmfotos in 320, 390 und 402 px sowie mit großer Schrift.
  3. (Nutzer) Die Liste wirkt stimmig.

## Phase 1j: Kästen (ADR-0017)

### B33: Kastengröße im Datenmodell und in der API

- **Status:** erledigt
- **Abhängig von:** B31
- **Referenzen:** ADR-0017; architecture.md 5, 6.5
- **Umfang:**
  - Migration `0004_crate_size.sql`: `products.crate_size INTEGER NULL CHECK (crate_size IS NULL OR crate_size BETWEEN 2 AND 100)`, mit Down-Migration.
  - `crate_size` (nullbar im 3.1-Stil, `minimum: 2`, `maximum: 100`) in `Product` (Pflichtfeld, Wert oder null), `ProductCreate` (optional), `ProductPatch` (optional, `null` löscht) und `ShoppingItem` (Pflichtfeld).
  - Fachlogik und Handler: anlegen, ändern (PATCH setzt `needs_review` wegen `crate_size` nicht zurück), lesen, Einkaufsliste; Zusammenführen übernimmt die Kastengröße der Quelle nur, wenn das Ziel keine hat.
  - `make generate` erzeugt auch `web/src/lib/api/schema.d.ts`; Frontend-Test-Fixtures bekommen `crate_size: null`, sonst keine Frontend-Änderung.
- **Nicht im Umfang:** Oberfläche (F27, F28), Umrechnung in Kästen (rein im Frontend).
- **Abnahmekriterien (Tests):**
  1. Anlegen, Ändern, Löschen (`null`) der Kastengröße; Grenzen 1 und 101 ergeben 400.
  2. Einkaufsliste und Produktliste liefern `crate_size`.
  3. Zusammenführen nach der Regel oben; Migration hin und zurück; `make check` ist grün.

### F27: Kastengröße auf der Produktseite und Kästen in der Einkaufsliste

- **Status:** erledigt
- **Abhängig von:** B33, F26
- **Referenzen:** ADR-0017
- **Umfang:**
  - Produktseite, Formular: Feld „Kastengröße" (Zahl, 2 bis 100, leer bedeutet kein Kasten, Hilfetext „Flaschen pro Kasten"), gespeichert über den bestehenden PATCH mit `diffPatch`.
  - Einkaufsliste: für Produkte mit Kastengröße und Fehlbestand „N Kasten"/„N Kästen" (aufgerundet) mit dem Zusatz „fehlen M Flaschen"; geteilter Text „N Kasten <Name>" bzw. „N Kästen <Name>". Die Umrechnung als reine Funktion mit Tests.
  - Vorrat-Status aus F26: bei Kastengröße zusätzlich „(Kasten zu N)" nicht nötig; unverändert lassen.
- **Nicht im Umfang:** Scan-Frage (F28).
- **Abnahmekriterien:**
  1. Vitest-Tests für die Umrechnung (17 von 20 ergibt 1 Kasten, 21 ergibt 2 Kästen, genau 20 ergibt 1 Kasten, ohne Kastengröße unverändert) und den geteilten Text.
  2. `make check` ist grün.

### F28: Frage „Flasche oder Kasten" beim Einlagern

- **Status:** erledigt (vom Nutzer am 24.09.2026 geprüft: Kastenware bei Flensburger eingeschaltet, der Scan fragte und buchte einen Kasten)
- **Abhängig von:** F27
- **Referenzen:** ADR-0017; F08 bis F11
- **Umfang:**
  - Im Modus Einlagern sucht die Scan-Ansicht das Produkt zum (normalisierten) Barcode in der zwischengespeicherten Produktliste (reine Funktion). Hat es eine Kastengröße, erscheint vor dem Buchen ein Sheet mit zwei großen Knöpfen „Flasche" und „Kasten (N Flaschen)"; gebucht wird `quantity` 1 bzw. N. Solange das Sheet offen ist, werden Scans ignoriert; „Abbrechen" bucht nichts. „Code eintippen" verhält sich gleich.
  - Ohne Kastengröße (oder Produkt nicht im Cache, oder neu angelegt) wird wie bisher sofort 1 gebucht. (Mit F29 entfallen:) Die Ergebniskarte im Modus Einlagern zeigt dann „War ein Kasten": Auswahl 6, 12, 20, 24 oder eine andere Zahl (2 bis 100); danach wird die Kastengröße per PATCH gespeichert und `N − 1` per `product_id` nachgebucht (mit Idempotency-Key). Die Karte zeigt danach das neue Ergebnis.
  - Entnehmen und Einkaufen fragen nicht.
- **Nicht im Umfang:** Kästen beim Entnehmen, Inventur-Ablauf (bleibt „Bestand setzen").
- **Abnahmekriterien:**
  1. Vitest-Tests für die Produktsuche per Barcode im Cache und die Entscheidung Frage ja/nein.
  2. `make check` ist grün.
  3. (Nutzer) Kasten Bier einlagern: erst „War ein Kasten" mit Größe, beim nächsten Kasten die Frage vorab. Ersetzt durch Kriterium 4 von F29.

### F29: Schalter „Kastenware" am Produkt, „War ein Kasten" entfällt

- **Status:** erledigt (vom Nutzer am 24.09.2026 geprüft: Kastenware bei Flensburger eingeschaltet, der Scan fragte und buchte einen Kasten)
- **Abhängig von:** F28
- **Referenzen:** ADR-0017 (geändert am 24.09.2026); Nutzer-Rückmeldung vom 24.09.2026 („Das sollte man im Produkt selbst aktivieren können")
- **Umfang:**
  - Produktseite: das Zahlenfeld „Kastengröße" aus F27 wird ein Schalter (Checkbox) „Kastenware". Eingeschaltet erscheint darunter das Pflichtfeld „Flaschen pro Kasten" (Zahl, 2 bis 100, beim Einschalten leer); ausgeschaltet wird `crate_size: null` gespeichert und das Feld verschwindet. Gespeichert wird weiter über den bestehenden PATCH mit `diffPatch`. Nachtrag vom 24.09.2026: Fehlt die Zahl oder liegt sie nicht zwischen 2 und 100, steht unter dem Feld rot „Bitte Flaschen pro Kasten eintragen (2 bis 100)." und neben „Speichern" „Nicht gespeichert"; die native Prüfung des Browsers entfällt für dieses Feld, weil ihre Blase auf iOS leicht zu übersehen ist.
  - Ergebniskarte der Scan-Ansicht: „War ein Kasten" entfällt mit dem Größen-Dialog, dem Speichern der Kastengröße von dort, dem Nachbuchen von N − 1 und dem Rückgängig über zwei Buchungen. Code, der dadurch nicht mehr gebraucht wird, wird entfernt.
  - Unverändert bleiben: die Frage „Flasche oder Kasten" für Produkte mit Kastengröße (F28), das Laden der Produktliste in der Scan-Ansicht, die Einkaufsliste (F27).
- **Nicht im Umfang:** API und Backend, Einkaufsliste, Vorrat-Liste.
- **Abnahmekriterien:**
  1. Vitest-Tests für das Formular: einschalten mit Größe ergibt die Größe im Patch, ausschalten ergibt `null`, unverändert ergibt keinen Patch, ein Produkt mit Kastengröße startet eingeschaltet.
  2. Kein „War ein Kasten" mehr in Code und Tests; die Tests der Ergebniskarte sind entsprechend angepasst.
  3. `make check` ist grün.
  4. (Nutzer) Bei Jever „Kastenware" mit 20 einschalten: der nächste Scan im Modus Einlagern fragt „Flasche oder Kasten". Andere Produkte fragen nie, und ihre Karte zeigt keinen Kasten-Knopf.

---

## Phase 1k: Unbekannte Barcodes beim Scannen benennen

### F31: Namensfeld auf der Ergebniskarte

- **Status:** wartet auf Nutzer
- **Abhängig von:** F10, B36a
- **Referenzen:** Nutzer-Rückmeldung vom 25.09.2026 (keine kostenpflichtigen Quellen; unbekannte Produkte sollen sich beim Scannen benennen lassen); architecture.md 6.5 (`needs_review` nach PATCH des Namens), 7.2, 7.3 (Nachladen überschreibt nur bei `needs_review`); ADR-0016
- **Umfang:**
  - Zeigt die Ergebniskarte im Modus Einlagern ein Platzhalter-Produkt (`origin = placeholder` mit `needs_review`, beim ersten Scan und bei jedem späteren Einlagern, solange es Platzhalter ist; beim Entnehmen und Einkaufen nicht, Nutzer-Rückmeldung vom 25.09.2026), steht statt „Name ändern" ein Textfeld „Name" (leer, Platzhaltertext „Wie heißt das Produkt?") mit dem Knopf „Speichern". Absenden auch mit der Eingabetaste.
  - Speichern sendet `PATCH /products/{id}` nur mit `name` (getrimmt, 1 bis 120 Zeichen; leer lässt sich nicht speichern). Danach zeigt die Karte den neuen Namen; der Cache wird aus der Antwort aktualisiert. Fehler: „Speichern fehlgeschlagen" unter dem Feld bis zur nächsten Aktion.
  - Solange das Feld den Fokus hat, ignoriert die Scan-Ansicht erkannte Codes (wie bei den Dialogen), damit die Karte beim Tippen nicht durch einen neuen Scan ersetzt wird.
  - Soll-Schnellauswahl und „Stattdessen zu vorhandenem Produkt" bleiben unverändert. Bei Produkten mit Namen aus Open Food Facts bleibt „Name ändern".
  - Nachtrag (Nutzer, 25.09.2026): Neben dem Namensfeld ein Link „Bei OpenGTINDB nachsehen", der `https://opengtindb.org/index.php?cmd=ean1&ean=<code>` (Code des Platzhalters) in einem neuen Tab bzw. im Browser öffnet (`target="_blank"`, `rel="noopener noreferrer"`). StashBert selbst fragt OpenGTINDB nie ab (keine automatische Abfrage der Webseite, die Schnittstelle braucht eine kostenpflichtige Kennung); ist die Seite nicht erreichbar, betrifft das nur den geöffneten Tab. Die URL entsteht in einer reinen Funktion mit Test.
  - Die Entscheidung, ob das Feld erscheint, ist eine reine Funktion mit Vitest-Tests.
- **Nicht im Umfang:** Marke oder Packungsgröße auf der Karte, weitere Datenquellen, Änderungen am Backend.
- **Abnahmekriterien:**
  1. Vitest-Tests für die Entscheidung (Platzhalter neu, Platzhalter erneut gescannt, Produkt aus Open Food Facts, nach dem Speichern) und für das Trimmen und Prüfen des Namens.
  2. `make check` ist grün; Bildschirmfotos in 17 px (393 und 320 px Breite) und mit großer Schrift.
  3. (Nutzer) Unbekannten Barcode scannen, Namen auf der Karte eintragen, Produkt erscheint mit diesem Namen im Vorrat.

## Phase 1l: Einstellungen

Nutzerentscheidung vom 25.09.2026: SQLite bleibt (ADR-0005); eine Einstellungsseite hinter einem Zahnrad oben rechts in der Vorrat-Ansicht bündelt Home Assistant, Sicherung und Informationen. Verbindungsdaten bleiben in der Konfigurationsdatei (architecture.md 4.2).

### B40: Systemstatus und Sicherung herunterladen

- **Status:** erledigt
- **Abhängig von:** B39
- **Referenzen:** architecture.md 6.2 (`GET /system`, `GET /backup`), 9.3, 4.2; ADR-0005, ADR-0013
- **Umfang:**
  - `GET /system` (`getSystemStatus`, Schema `SystemStatus` wie in 6.2), contract-first. Die Zahlen kommen aus der Datenbankdatei und dem Ordner `DATA_DIR/backups/` (nur die regelmäßigen Sicherungen `stashbert-*.db`, nicht `pre-migration-*`).
  - `GET /backup` (`downloadBackup`, Antwort `application/gzip`), contract-first: erzeugt mit der vorhandenen Logik aus `internal/backup` eine frische Kopie per `VACUUM INTO` in ein temporäres Verzeichnis, streamt ein tar.gz mit `stashbert.db` und allen Dateien aus `DATA_DIR/images/` (Ordner `images/`) und löscht die temporäre Kopie danach. Nur Standardbibliothek (`archive/tar`, `compress/gzip`). Gleichzeitige Downloads laufen nacheinander. Die Schreibfrist des HTTP-Servers wird für diese Antwort verlängert (`http.ResponseController`), damit größere Sicherungen nicht abbrechen.
  - `docs/betrieb.md`: kurzer Abschnitt „Sicherung herunterladen und wiederherstellen" (Archiv entpacken, Dienst stoppen, Dateien ablegen, Besitzer setzen, Dienst starten).
- **Nicht im Umfang:** Wiederherstellen per API, Oberfläche (F32).
- **Abnahmekriterien:**
  1. Tests: Status mit und ohne Sicherungen, mit und ohne `OFF_CONTACT`; das Archiv lässt sich entpacken, die Datenbank darin ist gültig und enthält die Daten, die Bilder sind vollständig, die temporäre Kopie ist danach weg; Dateiname und Content-Type.
  2. `make check` ist grün.

### F32: Einstellungsseite mit Zahnrad

- **Status:** wartet auf Nutzer
- **Abhängig von:** B40, F30
- **Referenzen:** architecture.md 4.2, 6.2, 11.7; ADR-0016; F30 (Abschnitt Home Assistant)
- **Umfang:**
  - Neue Ansicht `/einstellungen` („Einstellungen") mit Zurück zu „Vorrat" wie die Produktseite; in der Vorrat-Ansicht oben rechts ein Zahnrad-Knopf (44 × 44 px, `aria-label` „Einstellungen").
  - Abschnitt **Home Assistant**: der Abschnitt aus F30 zieht aus der Einkaufsansicht hierher um (dort entfällt er). Ohne MQTT steht dort „Nicht eingerichtet" mit dem Hinweis auf `docs/home-assistant.md`.
  - Abschnitt **Sicherung**: letzte Sicherung (Datum und Uhrzeit auf Deutsch, oder „Noch keine"), Anzahl und aufbewahrte Anzahl, Knopf „Sicherung herunterladen" als Link auf `/api/v1/backup` (ein Download ist Navigation, kein API-Aufruf; Ausnahme von „nur über den generierten Client").
  - Abschnitt **Über StashBert**: Version, Größe der Datenbank (lesbar, z. B. „128 KB"), Open Food Facts „aktiv" bzw. „nicht eingerichtet", Hinweis auf die Datenquelle Open Food Facts (ODbL).
  - Formatierungen (Größe, Datum, Texte der Abschnitte) als reine Funktionen mit Vitest-Tests.
- **Nicht im Umfang:** Ändern von Verbindungsdaten, Kamera-Einstellungen (bleiben im Scan-Bereich), weitere Tabs.
- **Abnahmekriterien:**
  1. Vitest-Tests für die Formatierungen.
  2. `make check` ist grün; Bildschirmfotos in 17 px (393 und 320 px Breite) und mit großer Schrift von Vorrat (Zahnrad), Einstellungen und Einkauf (ohne Home-Assistant-Abschnitt).
  3. (Nutzer) Auf dem iPhone die Einstellungen öffnen, die Liste in Home Assistant wählen und eine Sicherung herunterladen.

## Phase 1m: Installation per Proxmox-Skript (ADR-0019)

Nutzerentscheidung vom 25.09.2026: Das Repository ist öffentlich (MIT). Releases entstehen per Tag auf GitHub; ein eigenes Skript im Stil der community-scripts legt auf dem Proxmox-Host den Container an und installiert daraus. Kein Debian-Paket. Gemeinsame Referenzen: ADR-0019, ADR-0014, architecture.md 9.

### L04: Release-Paket mit Update und Restore

- **Status:** wartet auf Nutzer (Skripte, `make release` und Workflow stehen, shellcheck und `actionlint` ohne Befund, Durchlauf im Debian-13-Container bestanden; das erste Release auf GitHub prüft der Nutzer)
- **Abhängig von:** L02, B40
- **Referenzen:** ADR-0019, ADR-0014, architecture.md 9.1 und 9.3, docs/betrieb.md 10
- **Umfang:**
  - **`deploy/stashbert-update`** (POSIX `sh`, als root im Container, `set -eu`, Meldungen auf Deutsch):
    - Basis `https://github.com/schmitz-chris/stashbert/releases`, für Tests überschreibbar mit `STASHBERT_RELEASES_URL`.
    - Prüft root, `uname -m` = `x86_64`, `curl`, `sha256sum` und `tar`.
    - Neueste Version: die Weiterleitung von `<basis>/latest` (`curl -fsSLI -o /dev/null -w '%{url_effective}'`); der letzte Pfadteil ist der Tag und beginnt mit `v`.
    - Installierte Version über `/usr/local/bin/stashbert -version`; fehlt das Binary, ist es eine Erstinstallation. Ist die Version gleich: „StashBert ist aktuell (<version>)." und Ende mit 0.
    - Lädt `stashbert_<version ohne v>_linux_amd64.tar.gz` und `SHA256SUMS` aus `<basis>/download/<tag>/` in ein temporäres Verzeichnis, das in jedem Fall gelöscht wird. Prüft die Prüfsumme des Archivs, entpackt und ruft `sh install.sh` im entpackten Ordner auf.
    - Jeder Fehler vor `install.sh` lässt die laufende Installation unberührt und endet mit einer Meldung und Exit-Code ungleich 0.
  - **`deploy/stashbert-restore`** (POSIX `sh`, als root im Container, `set -eu`): Aufruf `stashbert-restore <archiv.tar.gz>` mit einem Archiv aus `GET /backup`.
    - Entpackt in ein temporäres Verzeichnis und bricht ohne Änderung ab, wenn `stashbert.db` fehlt.
    - Stoppt den Dienst und verschiebt, was davon vorhanden ist (`stashbert.db`, `-wal`, `-shm`, `images/`), aus `/var/lib/stashbert` nach `/var/lib/stashbert/vor-restore-<YYYYMMDD-HHMMSS>/`.
    - Legt Datenbank und Bilder aus dem Archiv ab, setzt den Besitzer `stashbert:stashbert`, startet den Dienst und prüft nach einigen Sekunden, dass er läuft.
    - Nennt am Ende den Ordner mit dem alten Stand.
  - **`deploy/install.sh`:**
    - Das Argument wird optional; ohne Argument nimmt das Skript `stashbert` neben sich.
    - Installiert zusätzlich `stashbert-update` und `stashbert-restore` (müssen neben dem Skript liegen) mit Modus 0755 nach `/usr/local/bin/`.
    - Die Meldung bei einem falschen Binary nennt das Release-Archiv statt `stashbert-linux-amd64`.
  - **`Makefile`**, `make release`:
    - `VERSION ?=` `git describe --tags --always --dirty`; der Workflow übergibt den Tag.
    - `bin/release/` wird vorher geleert. Darin entstehen `stashbert_<VERSION ohne führendes v>_linux_amd64.tar.gz`, `stashbert-update` einzeln und `SHA256SUMS` über alle Dateien in `bin/release/`.
    - Das Archiv enthält einen Ordner gleichen Namens mit `stashbert`, `install.sh`, `stashbert.service`, `stashbert.env.example`, `stashbert-update` und `stashbert-restore`; Skripte und Binary mit Modus 0755. Auf macOS ohne `._`-Dateien (`COPYFILE_DISABLE=1`).
    - Das lose Binary `stashbert-linux-amd64` entfällt.
  - **`.github/workflows/release.yml`:** bei Tags `v*`, `permissions: contents: write`; Go und Node wie in `ci.yml`; `make check`; `make release VERSION=<tag>`; `gh release create <tag> bin/release/* --verify-tag --generate-notes`, mit `--prerelease`, wenn der Tag einen Bindestrich enthält.
  - **`make check`:** `sh -n` für die drei Skripte; zusätzlich `shellcheck`, wenn es installiert ist.
- **Nicht im Umfang:** Proxmox-Skript (L05), Anleitung (L06), arm64, Signaturen, `.deb`, Wahl einer bestimmten Version oder Downgrade im Updater.
- **Abnahmekriterien:**
  1. `make check` ist grün; shellcheck ohne Befund.
  2. `make release` erzeugt Archiv, `stashbert-update` und `SHA256SUMS`; die Prüfsummen stimmen; das Archiv enthält genau die sechs Dateien im Ordner, ohne `._`-Dateien.
  3. Durchlauf in einem Debian-13-Container mit systemd (Docker, `linux/amd64`) gegen einen lokalen Server, der die Release-URLs nachbildet. Geprüft wird:
     - Die Erstinstallation mit `stashbert-update` führt zu einem laufenden Dienst.
     - Ein zweiter Aufruf meldet „aktuell".
     - Ein neueres Release wird installiert, die Env-Datei bleibt.
     - Eine falsche Prüfsumme bricht ohne Änderung ab.
     - `stashbert-restore` mit einem Archiv aus `GET /backup` einer anderen Instanz stellt Daten und Bilder her und legt den alten Stand ab.
     Das Protokoll steht im Bericht; der Durchlauf ist nicht Teil von `make check`.
  4. (Nutzer) Nach dem ersten Tag steht das Release mit allen Assets auf GitHub.

### L05: Proxmox-Skript

- **Status:** wartet auf Nutzer (Skript und Makefile stehen, shellcheck ohne Befund, Durchlauf mit Attrappen für `pct`, `pveam`, `pvesm` und `pvesh` im Debian-13-Container bestanden; den Einzeiler auf dem Proxmox-Host prüft der Nutzer)
- **Abhängig von:** L04
- **Referenzen:** ADR-0019, ADR-0014, docs/betrieb.md 2, pct(1), pveam(1), pvesm(1) und pvesh(1) der Proxmox-Doku
- **Umfang:** `deploy/proxmox.sh` (bash, `set -Eeuo pipefail`), läuft als root auf dem Proxmox-Host, Texte auf Deutsch.
  - **Aufruf:** `bash -c "$(curl -fsSL https://github.com/schmitz-chris/stashbert/releases/latest/download/proxmox.sh)"`; Argumente nach einem Platzhalter, z. B. `bash -c "$(curl …)" _ --dry-run`. Es gibt `--dry-run` und `--help`.
  - **Prüfungen:** root und `pct`, `pveam`, `pvesm`, `pvesh`. Fehlt etwas, sagt das Skript, dass es auf den Proxmox-Host gehört.
  - **Fragen** (von der Standardeingabe, beim Einzeiler also vom Terminal; Enter übernimmt die Vorgabe, eine ungültige Eingabe wird erneut abgefragt):
    1. „Standardeinstellungen verwenden?" Ja heißt: nächste freie ID (`pvesh get /cluster/nextid`), Name `stashbert`, Speicher `local-lvm` bzw. der erste Speicher für Container, Bridge `vmbr0`, DHCP. Nein heißt: diese fünf Werte einzeln, bei fester IP (CIDR) auch das Gateway. Eingaben werden geprüft: ID frei (`pvesh get /cluster/nextid --vmid <id>`), Speicher aktiv und für Container (`pvesm status --content rootdir`), Bridge vorhanden (`ip link show dev <bridge>`), IP im CIDR-Format, Gateway als IPv4-Adresse.
    2. Optional: Kontakt für Open Food Facts; MQTT-Broker-URL (`mqtt://host:port` oder `mqtts://host:port`) mit Benutzer und Passwort (Passwort ohne Echo, `read -s`).
    3. Optional: ein Backup übernehmen, als Pfad zu einer Datei auf dem Host oder als Adresse einer laufenden Instanz (`http://…` oder `https://…`; das Skript lädt dann auf dem Host `<adresse>/api/v1/backup`, eine Adresse mit `/api/v1/backup` am Ende unverändert). Es prüft mit `tar -tzf`, dass das Archiv `stashbert.db` enthält.
    4. Zusammenfassung und Bestätigung.
  - Alle Eingaben sind geprüft, bevor etwas angelegt wird.
  - Werte mit `"`, `'`, `\`, `$`, Backtick, Leerzeichen oder Zeilenumbruch lehnt das Skript ab, mit dem Hinweis, sie später in `/etc/stashbert/stashbert.env` einzutragen.
  - **Ablauf:**
    1. Neueste Vorlage `debian-13-standard_*_amd64` über `pveam` auf einem Speicher mit Inhalt `vztmpl`; Download nur, wenn sie fehlt.
    2. `pct create`: unprivilegiert, `--features nesting=1`, 1 Kern, 512 MB RAM, 512 MB Swap, 8 GB Platte, `--onboot 1`, `--tags stashbert`. Dann starten und höchstens 60 s auf das Netz warten.
    3. Im Container `apt-get update`, Upgrade, `curl` und `ca-certificates` installieren.
    4. `stashbert-update` aus `<basis>/latest/download/` laden und ausführen. `<basis>` ist `https://github.com/schmitz-chris/stashbert/releases` oder, falls auf dem Host gesetzt, `STASHBERT_RELEASES_URL`; es wird an `stashbert-update` durchgereicht.
    5. Die Angaben in `/etc/stashbert/stashbert.env` eintragen: vorhandene Zeile `KEY=` ersetzen; Schlüssel und Werte über `pct exec <id> -- env …` in der Umgebung (awk liest sie aus `ENVIRON`), nie in eine Shell-Zeile eingesetzt.
    6. Falls gewählt: Backup per `pct push` in den Container kopieren und `stashbert-restore` ausführen. Dann den Dienst neu starten.
    7. `/usr/bin/update` als Aufruf von `stashbert-update` anlegen und die automatische Anmeldung auf der Konsole einrichten (getty-Drop-in wie bei den community-scripts).
  - **Am Ende** zeigt das Skript:
    - die Adresse `http://<ip>:<port>`;
    - die MAC-Adresse für eine DHCP-Reservierung;
    - den Update-Befehl: `update` in der Konsole oder `pct exec <id> -- update` auf dem Host;
    - den Hinweis auf den HTTPS-Proxy für die Kamera (docs/betrieb.md 5).
  - Die Ausgabe bleibt ruhig: eine Statuszeile pro Schritt. Die ausführliche Ausgabe (pveam, apt, `stashbert-update`) geht in ein Protokoll auf dem Host, das die Schlussmeldung und jede Fehlermeldung nennen.
  - Schlägt ein Schritt nach dem Anlegen fehl, bleibt der Container zur Fehlersuche bestehen; die Meldung nennt `pct stop <id>; pct destroy <id>`.
  - `--dry-run` fragt wie sonst, zeigt aber alle ändernden Befehle nur an.
  - `make release` legt `proxmox.sh` nach `bin/release/` (mit Eintrag in `SHA256SUMS`); `make check` prüft es mit `bash -n` und, wenn installiert, mit shellcheck.
- **Nicht im Umfang:** Aufnahme in die community-scripts, Whiptail-Dialoge, Abfrage der Ressourcen, SSH-Schlüssel, root-Passwort, arm64, Einrichtung des Proxys.
- **Abnahmekriterien:**
  1. `make check` ist grün; shellcheck ohne Befund.
  2. Durchlauf mit Attrappen für `pct`, `pveam`, `pvesm` und `pvesh`: Container-Befehle laufen in einem Debian-13-Container mit systemd (Docker), die Release-URLs kommen von einem lokalen Server. Geprüft werden Standard und feste IP, mit OFF-Kontakt, MQTT und einem Backup von einer laufenden Instanz. Am Ende läuft der Dienst mit diesen Angaben und Daten; `--dry-run` ändert nichts. Das Protokoll steht im Bericht.
  3. (Nutzer) Auf dem Proxmox-Host legt der Einzeiler einen laufenden StashBert-Container an; `update` in dessen Konsole meldet „aktuell".

### L06: Betriebsanleitung für das Skript

- **Status:** wartet auf Nutzer
- **Abhängig von:** L05
- **Referenzen:** ADR-0019, docs/betrieb.md
- **Umfang:**
  - **`docs/betrieb.md`:**
    - Hauptweg „Einrichten mit dem Proxmox-Skript": Einzeiler, die Fragen, was entsteht, danach Proxy und iPhone.
    - Update mit `update`, Restore mit `stashbert-restore`.
    - Der bisherige Weg wird „Ohne Skript" mit dem Release-Archiv (Download von GitHub oder `make release`).
    - Container-Empfehlung mit `nesting=1` (Debian 13 braucht es).
    - Deinstallation und Fehlersuche um die neuen Befehle ergänzt.
  - **`README.md`:** Abschnitt „Installation" mit dem Einzeiler und Verweis auf `docs/betrieb.md`.
  - **`docs/architecture.md` 9.1:** Artefakt und Übertragung nach ADR-0019.
  - **`docs/m2-pruefung.md` 4:** verweist auf das Skript.
- **Nicht im Umfang:** Proxy-Konfiguration im Detail.
- **Abnahmekriterien:**
  1. Alle Befehle passen zu den Skripten, zum Makefile und zum Workflow (gegenlesen).
  2. (Nutzer) Die Anleitung führt zu einer laufenden Instanz.

## Phase 1n: Backup einspielen (ADR-0020)

Nutzerentscheidung vom 25.09.2026: Ein Backup lässt sich in den Einstellungen einspielen. Die Oberfläche sagt „Backup", nicht „Sicherung". Gemeinsame Referenzen: ADR-0020, architecture.md 4.4, 6.2, 6.4 und 9.3.

### B41: Backup einspielen per API

- **Status:** erledigt
- **Abhängig von:** B40
- **Referenzen:** ADR-0020, architecture.md 4.4, 6.2 (`POST /backup/restore`), 6.4, 9.3; `internal/backup/archive.go` (Aufbau des Archivs)
- **Umfang:**
  - **Spec** (contract-first): `POST /backup/restore` (`restoreBackup`), Body `application/gzip` (binär), Antwort 202 ohne Body, `default` mit `Problem`; die Codes in der `description`.
  - **Prüfen und bereitlegen** (`internal/backup`, z. B. `restore.go`):
    - Das Archiv wird beim Lesen nach `DATA_DIR/restore/incoming-<zufall>/` entpackt (`archive/tar`, `compress/gzip`), nie ganz in den Speicher.
    - Erlaubt sind nur die reguläre Datei `stashbert.db`, der Ordner `images/` und reguläre Dateien darin, deren Namen dem Muster der gespeicherten Produktbilder folgen. Alles andere (andere Pfade, `..`, absolute Pfade, Links, Geräte) ergibt `invalid_backup`. Entpackt höchstens 4 GB; mehr ergibt `backup_too_large`.
    - Fehlt `stashbert.db`, ist es kein gültiges gzip-tar oder besteht die Datenbank `PRAGMA integrity_check` nicht: `invalid_backup`.
    - Enthält `goose_db_version` eine Version, die neuer ist als die neueste eingebettete Migration: `backup_too_new`.
    - Bei Erfolg wird `incoming-…` in `DATA_DIR/restore/pending/` umbenannt. Liegt dort schon ein Backup oder läuft gerade ein Einspielen: `restore_in_progress`. Bei jedem Fehler wird `incoming-…` gelöscht, der laufende Betrieb bleibt unberührt.
  - **Einspielen beim Start** (`internal/backup`, aufgerufen in `main` vor `app.OpenAndMigrate`):
    - Liegt `DATA_DIR/restore/pending/` bereit, verschiebt StashBert `stashbert.db`, `-wal`, `-shm` und `images/` (was davon vorhanden ist) nach `DATA_DIR/vor-restore-<YYYYMMDD-HHMMSS>/` (UTC) und legt Datenbank und Bilder aus `pending/` ab.
    - Danach wird `DATA_DIR/restore/` entfernt, auch Reste von `incoming-…`.
    - Jeder Schritt steht im Log. Scheitert ein Schritt, bricht der Start mit Fehler ab, statt mit halbem Stand zu laufen.
  - **Neustart im Prozess** (`cmd/stashbert`, nur Verdrahtung):
    - Der Handler meldet nach dem Bereitlegen über eine Abhängigkeit in `ServerDeps` einen Neustart an und antwortet 202.
    - `main` fährt dann herunter wie bei SIGTERM (HTTP-Server mit `Shutdown`, damit die 202 noch ankommt; Hintergrundjobs; MQTT; Datenbank) und durchläuft den Start erneut im selben Prozess: Backup einspielen, Datenbank öffnen und migrieren, Dienste starten, Port öffnen.
    - SIGINT und SIGTERM beenden weiterhin ganz.
  - **Route** in `internal/app`: `POST /api/v1/backup/restore` geht am Validator vorbei zum generierten Handler, mit `http.MaxBytesReader` (1 GB, Überschreitung: 413 `backup_too_large`) und Lese- und Schreibfrist 10 min per `http.ResponseController`. Ein anderer Content-Type als `application/gzip` oder `application/x-gzip` ergibt `invalid_request`.
  - Neue Codes in `internal/httpx` bzw. dort, wo die vorhandenen Codes stehen, nach architecture.md 6.4.
- **Nicht im Umfang:** Oberfläche (F33), automatisches Löschen alter `vor-restore-…`-Ordner, Einspielen einzelner `.db`-Dateien ohne Archiv, Anmeldung.
- **Abnahmekriterien:**
  1. Tests für das Prüfen:
     - Ein Archiv aus `GET /backup` wird bereitgelegt.
     - Diese Archive werden abgelehnt und hinterlassen keine Reste: ohne `stashbert.db`, mit `../`, mit absolutem Pfad, mit Link, mit fremder Datei, kein gzip, kaputte Datenbank, zu neue Migrationsversion, zu groß (mit kleinem Limit im Test).
     - Ein zweites Bereitlegen ergibt `restore_in_progress`.
  2. Tests für das Einspielen beim Start: Der alte Stand liegt in `vor-restore-…`, die neuen Daten und Bilder sind da, `restore/` ist weg; ohne `pending/` ändert sich nichts.
  3. API-Test gegen `app.NewHandler`: 202 und der Neustart ist angemeldet; 413 bei Überschreitung; `invalid_request` bei falschem Content-Type. Ein Durchlauf „bereitlegen, einspielen, `OpenAndMigrate`" zeigt die Produkte, Barcodes und Buchungen des Backups (wie `restore_test.go`).
  4. Von Hand gegen einen laufenden Server (Bericht): Einspielen per `curl`, StashBert ist nach wenigen Sekunden mit den neuen Daten wieder da, derselbe Prozess läuft weiter.
  5. `make check` ist grün.

### F33: Backup einspielen in den Einstellungen

- **Status:** wartet auf Nutzer
- **Abhängig von:** B41, F32
- **Referenzen:** ADR-0020, ADR-0016, architecture.md 4.2, 6.2; `components/Dialog.tsx`
- **Umfang:**
  - **Umbenennen:** Der Abschnitt heißt „Backup". Aus „Letzte Sicherung" wird „Letztes Backup", aus „Sicherung herunterladen" wird „Backup herunterladen"; die übrigen Texte des Abschnitts entsprechend, ohne „Sicherung".
  - **Knopf „Backup einspielen"** unter „Backup herunterladen", als zweitrangiger Knopf. Er öffnet eine Dateiauswahl (`<input type="file">`, `.tar.gz` und gzip).
  - **Rückfrage** im Dialog nach der Auswahl: Titel „Backup einspielen?", Text „Alle Produkte, Bestände und Bilder werden durch das Backup „<Dateiname>" ersetzt. Der jetzige Stand bleibt auf dem Server als Kopie erhalten.", Knöpfe „Abbrechen" und „Einspielen" (Rolle `danger`).
  - **Hochladen** über den generierten Client mit der Datei als Body und `Content-Type: application/gzip`. Währenddessen „Backup wird eingespielt …", Knöpfe gesperrt.
  - **Nach 202:** `GET /health` etwa alle 500 ms abfragen, höchstens 60 s. Sobald StashBert antwortet, alle Queries neu laden und „Backup eingespielt" zeigen.
  - **Fehler** über `problemCode`:
    - `invalid_backup`: „Das ist kein Backup von StashBert."
    - `backup_too_new`: „Das Backup stammt von einer neueren Version. Bitte zuerst StashBert aktualisieren."
    - `backup_too_large`: „Das Backup ist zu groß (höchstens 1 GB)."
    - `restore_in_progress`: „Es wird gerade schon ein Backup eingespielt."
    - Sonst: „Einspielen fehlgeschlagen".
    - Antwortet StashBert nach 60 s nicht: „StashBert antwortet nicht. Bitte die Seite später neu laden."
  - Texte und die Zuordnung der Fehler als reine Funktionen in `lib/settings.ts` mit Vitest-Tests.
- **Nicht im Umfang:** Fortschrittsbalken beim Hochladen, Auswahl älterer Backups vom Server, Löschen der `vor-restore-…`-Ordner.
- **Abnahmekriterien:**
  1. Vitest-Tests für Texte und Fehlerzuordnung.
  2. `make check` ist grün; Bildschirmfotos in 17 px (393 und 320 px Breite) vom Abschnitt „Backup", vom Dialog und vom Zustand nach dem Einspielen.
  3. Von Hand gegen einen laufenden Server (Bericht): ein Backup einer anderen Instanz über die Oberfläche einspielen; danach zeigt die Vorrat-Ansicht deren Produkte.
  4. (Nutzer) Auf dem iPhone ein Backup aus der Dateien-App einspielen.

## Phase 2a: Home Assistant über MQTT (M2, ADR-0018)

Gemeinsame Referenzen aller Tasks dieser Phase: ADR-0018, architecture.md Kapitel 11 und 9.2. Topics und Nachrichten sind immer Englisch (AGENTS.md). Der Broker im Heimnetz ist das Mosquitto-Add-on von HA; Tests laufen nur gegen den eingebetteten Test-Broker (`mochi-mqtt`), nie gegen den echten.

### B34: MQTT-Konfiguration und Verbindung

- **Status:** erledigt
- **Abhängig von:** B33
- **Referenzen:** ADR-0018; architecture.md 11.1, 11.2, 9.2
- **Umfang:**
  - `internal/config`: die Variablen `MQTT_*` aus 9.2 mit Prüfung (`MQTT_URL` nur `mqtt://` oder `mqtts://` mit Host und Port; Präfixe nach den Regeln in 9.2; `MQTT_HA_DISCOVERY` nur `true` oder `false`).
  - Neues Paket `internal/mqtt` über `autopaho` (paho.golang v0.23.0): Verbindung, Wiederverbinden, Last Will und Status nach 11.1, Abonnements `<p>/in/#` und (nur mit Discovery) `<ha>/status`, Weitergabe eingehender Nachrichten samt Retain-Flag an registrierte Empfänger, Rückruf nach jeder Verbindung für spätere Tasks, Zustand `disabled`/`connecting`/`connected`, `Publish` mit QoS 1 (wartet auf PUBACK, Fehler bei Ablehnung), sauberes Beenden mit `offline`.
  - `cmd/stashbert/main.go`: Client nur starten, wenn `MQTT_URL` gesetzt ist; beim Herunterfahren nach dem HTTP-Server beenden.
- **Nicht im Umfang:** Outbox, Ereignisse, Zusammenfassung, Discovery, Endpunkte.
- **Abnahmekriterien:**
  1. Tests für die Konfiguration (Standardwerte, gültige und ungültige Werte je Variable).
  2. Test gegen `mochi-mqtt`: nach dem Start liegt `<p>/status` = `online` retained vor; nach dem Beenden `offline`; eine Nachricht auf `<p>/in/snapshot` erreicht den Empfänger mit korrektem Retain-Flag; das Passwort taucht in keinem Log auf.
  3. `make check` ist grün.

### B35: Outbox und Zustellung

- **Status:** erledigt
- **Abhängig von:** B34
- **Referenzen:** ADR-0018; architecture.md 11.3, 11.4, 6.6
- **Umfang:**
  - Migration `0005_outbox.sql` und sqlc-Queries für die Tabelle `outbox` (11.4).
  - Reine Funktion für `quantity` und `unit` (11.3) in `internal/domain`, damit die Zusammenfassung sie wiederverwendet.
  - `outbox.Writer` als `events.Publisher`: ergänzt `shopping.changed` nach 11.3 (liest Produkt und die Einstellung `shopping_target_id`; fehlt sie, ist `list` `""`), schreibt die Zeile, weckt den Zusteller.
  - Zusteller nach 11.4 (Reihenfolge, Löschen nach PUBACK, Backoff, Aufräumen nach 7 Tagen) über ein Interface, das `internal/mqtt` erfüllt.
  - `main.go`: mit `MQTT_URL` ist der Publisher der Writer, und der Zusteller läuft; sonst bleibt `events.Nop`.
- **Nicht im Umfang:** neue Ereignisauslöser, `shopping.snapshot`, Zusammenfassung, Discovery.
- **Abnahmekriterien:**
  1. Tests: `quantity` und `unit` (ohne Kastengröße, mit Kastengröße aufgerundet, ohne Fehlbestand); Anreicherung von `shopping.changed` (vorhandenes, gelöschtes Produkt, mit Kastengröße, mit gespeicherter Zielliste); Reihenfolge; Löschen nach Erfolg; Behalten und erneuter Versuch nach Fehler; Aufräumen alter Einträge.
  2. Ende-zu-Ende-Test mit `mochi-mqtt`: eine Buchung über die HTTP-API erscheint als `<p>/events/stock.added` mit dem JSON aus 11.3.
  3. `make check` ist grün.

### B36: Einkaufsereignisse beim Vormerken und Abgleich

- **Status:** erledigt
- **Abhängig von:** B35
- **Referenzen:** ADR-0018; architecture.md 11.3, 6.2, 6.4, 6.6; ADR-0015
- **Umfang:**
  - Fachlogik: Vormerken und Entfernen der Vormerkung lösen `shopping.changed` aus, wenn sich dadurch ändert, ob das Produkt auf der Liste steht (6.6).
  - `shopping.snapshot` nach 11.3: Aufbau der Einträge (sqlc-Query), Auslösen über `<p>/in/snapshot` (Nachrichten mit Retain-Flag ignorieren, 2 s zusammenfassen) und über `POST /shopping-list/snapshot` (`sendShoppingSnapshot`, 202; ohne MQTT 409 `mqtt_disabled`). Contract-first.
- **Nicht im Umfang:** Zielliste wählen (B39), Zusammenfassung, Discovery, Oberfläche.
- **Abnahmekriterien:**
  1. Tests: Vormerken eines Produkts ohne Fehlbestand löst `shopping.changed` aus, Vormerken eines Produkts mit Fehlbestand nicht; Entfernen entsprechend; Inhalt und Sortierung des Snapshots; Retain-Flag wird ignoriert; mehrere Anfragen in 2 s ergeben einen Snapshot; API 202 und 409.
  2. `make check` ist grün.

### B36a: Einkaufsereignis bei jeder Änderung der Listenzugehörigkeit

- **Status:** erledigt
- **Abhängig von:** B36
- **Referenzen:** ADR-0018; architecture.md 6.6, 11.3; ADR-0015
- **Umfang:**
  - Befund aus B36: Wenn eine Buchung die Vormerkung beendet, ein vorgemerktes Produkt gelöscht oder beim Zusammenführen die Vormerkung übertragen wird, ändert sich die Zugehörigkeit zur Einkaufsliste ohne `shopping.changed`. In HA bliebe der Eintrag dann bis zum nächsten Abgleich stehen.
  - Fachlogik nach der Regel in 6.6: `shopping.changed` bei jeder Änderung von `missing` oder der Zugehörigkeit, höchstens eines pro Produkt und Vorgang, nach dem Commit, in der Reihenfolge aus 6.6.
- **Nicht im Umfang:** alles andere.
- **Abnahmekriterien:**
  1. Tests: `add` auf ein vorgemerktes Produkt ohne Fehlbestand (Vormerkung endet) löst `shopping.changed` aus; Löschen eines vorgemerkten Produkts ohne Fehlbestand; Zusammenführen, bei dem das Ziel die Vormerkung der Quelle übernimmt, und die gelöschte vorgemerkte Quelle; kein doppeltes Ereignis, wenn sich `missing` und Zugehörigkeit zugleich ändern; bestehende Fälle unverändert.
  2. `make check` ist grün.

### B37: Zusammenfassung

- **Status:** erledigt
- **Abhängig von:** B36a
- **Referenzen:** ADR-0018; architecture.md 11.5, 6.2, 6.5
- **Umfang:**
  - `GET /summary` (`getSummary`, Schema `Summary`), contract-first; Berechnung in `internal/domain` mit sqlc.
  - Retained Veröffentlichung auf `<p>/state/summary` nach jeder Verbindung und 1 s nach dem letzten von schnell aufeinander folgenden Ereignissen (der Writer meldet Ereignisse); ohne Verbindung überspringen.
- **Nicht im Umfang:** Discovery und die Reaktion auf die Birth-Nachricht von HA (B38).
- **Abnahmekriterien:**
  1. Tests: Zählungen (Einkauf mit Vormerkungen, leer, zu prüfen, alle), `quantity` und `unit` in `shopping`, Begrenzung auf 100 mit `shopping_truncated`; mehrere Ereignisse ergeben eine Veröffentlichung; Veröffentlichung nach dem Verbinden (mit `mochi-mqtt`).
  2. `make check` ist grün.

### B38: HA-Discovery

- **Status:** erledigt
- **Abhängig von:** B36, B37
- **Referenzen:** ADR-0018; architecture.md 11.6
- **Umfang:**
  - Reine Funktion für das Discovery-Payload nach 11.6 (Präfixe und Version eingesetzt).
  - Veröffentlichung nach jeder Verbindung; nach `online` auf `<ha>/status` nach 1 bis 5 s Zufallsverzögerung Discovery, `<p>/status` = `online` und Zusammenfassung erneut; mit `MQTT_HA_DISCOVERY=false` stattdessen leeres retained Payload.
  - Nachtrag aus B37: Die Zusammenfassung wird auch nach jeder erfolgreichen ändernden API-Anfrage (Middleware in `internal/app.NewHandler`, nur mit MQTT) und alle 5 min neu veröffentlicht (11.5).
- **Nicht im Umfang:** Zielliste, Blueprints.
- **Abnahmekriterien:**
  1. Test des Payloads gegen das JSON aus 11.6 (mit Standardpräfixen und mit eigenem Präfix); gültiges JSON, keine Schlüssel `object_id`.
  2. Tests mit `mochi-mqtt`: Discovery retained nach dem Verbinden; erneute Veröffentlichung nach `online` (Verzögerung im Test verkürzbar); leeres Payload bei `false`.
  3. Tests: eine ändernde API-Anfrage ohne Ereignis (z. B. PATCH des Namens) führt zu einer neuen Zusammenfassung; GET-Anfragen und fehlgeschlagene Anfragen nicht; die periodische Veröffentlichung (Intervall im Test verkürzbar).
  4. `make check` ist grün.

### B39: Zielliste wählen

- **Status:** erledigt
- **Abhängig von:** B38
- **Referenzen:** ADR-0018; architecture.md 11.7, 6.2, 6.4, 6.5
- **Umfang:**
  - Auswertung von `<p>/in/targets` (reine Funktion mit den Regeln aus 11.7, Angebot im Speicher).
  - `GET /integrations/mqtt` (`getMqttStatus`) und `PUT /integrations/mqtt/target` (`setShoppingTarget`) mit Schema `MqttStatus`, contract-first; Speichern in `settings`.
  - Beim Wechsel die beiden Snapshots aus 11.7 über die Outbox.
- **Nicht im Umfang:** Oberfläche (F30), Blueprints.
- **Abnahmekriterien:**
  1. Tests: gültige und ungültige Angebote; Setzen, Aufheben, unbekannte ID (422), ohne MQTT (409); Snapshots beim Wechsel (alte Liste abgeräumt, neue befüllt, Reihenfolge); `list` in `shopping.changed` nach dem Setzen.
  2. `make check` ist grün.

### F30: Home Assistant in der Einkaufsansicht

- **Status:** erledigt
- **Abhängig von:** B39
- **Referenzen:** ADR-0018; architecture.md 11.7; ADR-0016
- **Umfang:**
  - Unter der Einkaufsliste (auch bei leerer Liste) ein Abschnitt „Home Assistant", nur wenn `status` nicht `disabled` ist: Zustand („Verbunden" bzw. „Keine Verbindung zum Broker"), Auswahl „Liste in Home Assistant" mit den angebotenen Listen und „Keine" (eine gespeicherte, gerade nicht angebotene Liste erscheint als „<Name> (nicht verfügbar)"), Knopf „Liste neu senden" mit Rückmeldung.
  - Logik für die Auswahl als reine Funktion mit Tests; Zugriff nur über den generierten Client.
- **Nicht im Umfang:** eigene Einstellungsseite, weitere Integrationen.
- **Abnahmekriterien:**
  1. Vitest-Tests für die Auswahl-Logik.
  2. `make check` ist grün; Bildschirmfotos in 17 px (393 und 320 px Breite) und mit großer Schrift.

### H01: Blueprints und Anleitung für Home Assistant

- **Status:** erledigt (beide Blueprints am 24.09.2026 im HA des Nutzers gespeichert und gegen eine Testliste und eine Bring!-Liste geprüft)
- **Abhängig von:** B39
- **Referenzen:** ADR-0018; architecture.md 11.8, 11.2, 11.3
- **Umfang:**
  - Die beiden Blueprints aus 11.8 unter `deploy/homeassistant/` (Einkauf nach To-do-Liste mit Snapshot-Verarbeitung per `repeat`/`for_each`; Listen melden).
  - `docs/home-assistant.md` (Deutsch): Voraussetzungen, Konfiguration von StashBert, ACL-Vorschlag (mit Hinweis, dass die Wirkung im Add-on nicht bestätigt ist), Import der Blueprints, Prüfung in HA, Fehlersuche mit `mosquitto_sub`, Entfernen.
  - `docs/betrieb.md` und die Beispielkonfiguration ergänzen.
- **Nicht im Umfang:** Code in StashBert.
- **Abnahmekriterien:**
  1. Beide Blueprints sind gültiges YAML mit `blueprint:`-Kopf, `min_version` und den Eingaben aus 11.8.
  2. Die Anleitung deckt alle Topics und Variablen aus 11.2 und 9.2 ab.

### H02: Prüfung gegen den Broker im Heimnetz

- **Status:** wartet auf Nutzer (Lead-Prüfung am 24.09.2026 bestanden, siehe `docs/m2-pruefung.md`)
- **Abhängig von:** F30, H01
- **Referenzen:** ADR-0018; architecture.md Kapitel 11
- **Umfang:**
  - Der Lead startet StashBert mit dem Broker im Heimnetz und prüft mit einem Beobachter-Skript: Status, Zusammenfassung, Discovery, Ereignisse einer Buchung, Snapshot über die API und den Knopf-Topic, Zielliste mit einem Testangebot (danach wieder gelöscht).
  - Ergebnisse und offene Nutzerprüfungen in `docs/m2-pruefung.md`.
- **Abnahmekriterien:**
  1. Alle Topics aus 11.2 wurden gegen den echten Broker beobachtet.
  2. (Nutzer) In HA erscheint das Gerät „StashBert" mit vier Sensoren, der Ereignis-Entität und dem Knopf.
  3. (Nutzer) Eine Buchung ändert die Sensoren; ein Artikel erscheint über den Blueprint in der gewählten Bring!-Liste und verschwindet nach dem Einlagern.

---

## Später (bewusst nicht Teil dieses Plans)

Wird erst nach der M1-Abnahme geplant. Agenten bauen davon nichts vor.

- M2 (Rest, Phase 2a deckt MQTT, HA und Bring! ab):
  - API-Tokens
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
