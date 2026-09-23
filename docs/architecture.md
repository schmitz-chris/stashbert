# StashBert: Architektur (Revision 4)

| | |
|---|---|
| Stand | 23.09.2026 |
| Status | verbindlich für M1 (Erprobungsversion) |
| Entscheidungen | [docs/adr/](adr/) |
| Umsetzungsplan | [docs/plan.md](plan.md) |
| Recherche und Quellen | [docs/research.md](research.md) (Fakten gültig, Stack-Teile überholt) |

Dieses Dokument beschreibt den **aktuellen** Stand knapp und eindeutig. Es ist die Referenz für alle Tasks im Plan. Begründungen stehen in den ADRs, Fakten und Quellen in `research.md`.

## 1. Zweck und Umfang

StashBert ist ein selbst gehostetes Vorratsinventar für einen Haushalt. Es beantwortet: Was habe ich, wie viel, wie viel soll da sein, was muss nachgekauft werden.

**M1, die Erprobungsversion**, läuft im eigenen Homelab für zwei Personen. Ziel ist herauszufinden, ob der Ablauf im Alltag funktioniert, insbesondere ob Entnahmen zuverlässig gescannt werden.

**In M1 enthalten:**

- Produkte mit mehreren Barcodes, Bestand, Sollbestand und optionalem Mindestbestand (Mindestbestand nur über die API, nicht in der Oberfläche)
- Buchungen per Barcode oder Produkt: einlagern, entnehmen, Inventur, Storno
- Unbekannte Barcodes beim Einlagern: Produkt automatisch über Open Food Facts anlegen, sonst Platzhalter
- Platzhalter mit einem vorhandenen Produkt zusammenführen
- Berechnete Einkaufsliste
- Web-Oberfläche (PWA) mit Scanner, Vorrat, Einkauf und Produktdetail
- SQLite, tägliches Backup

**Nicht in M1** (siehe Plan, Abschnitt „Später"): Anmeldung und Benutzer (ADR-0013), MQTT, Home Assistant, Bring!, API-Tokens, ESP32, Kurzbefehl, OIDC, Kategorien, Lagerorte, MHD, Statistiken, Offline-Buchen, Mehrsprachigkeit.

## 2. Leitplanken

Diese Regeln gelten ab dem ersten Task, weil sie später teuer zu ändern wären:

1. **API zuerst:** `api/openapi.yaml` ist die Quelle der Wahrheit (ADR-0003). Die Web-Oberfläche nutzt ausschließlich die öffentliche API.
2. **Buchungen als Ressource:** Bestand ändert sich nur über `movements` (ADR-0004).
3. **IDs sind UUIDv7** (ADR-0006).
4. **Barcodes als eigene Tabelle**, mehrere pro Produkt.
5. **Migrationen ab Tag 1** mit goose (ADR-0005).
6. **Domänen-Ereignisse intern ab Tag 1**, zunächst ohne Empfänger (ADR-0011).
7. **Keine Anmeldung in M1, aber nachrüstbar** (ADR-0013): Die Endpunkte enthalten nichts, was eine spätere Anmeldung am Reverse Proxy oder per OIDC verhindert.
8. **Keine Mandantenfähigkeit:** Eine Instanz ist ein Haushalt (ADR-0010).
9. **Zustand nur in `DATA_DIR`, Konfiguration nur über Umgebungsvariablen** (ADR-0010).

## 3. Systemübersicht

```
┌────────────────────────────┐
│ iPhone / Browser (PWA)     │
│ Scanner (zxing-wasm), UI   │
└──────────────┬─────────────┘
               │ HTTPS
               ▼
┌────────────────────────────┐
│ Reverse Proxy, TLS         │   vorhandene Infrastruktur,
│ (nicht Teil von StashBert) │   nicht Teil des Projekts
└──────────────┬─────────────┘
               │ HTTP :8080
               ▼
┌──────────────────────────────────────────────────────────┐
│ stashbert (ein Go-Binary, ein Container)                 │
│                                                          │
│  /api/v1/...   API nach api/openapi.yaml                 │
│  /             eingebettete Web-Oberfläche (SPA)         │
│                                                          │
│  Fachlogik ─── SQLite (DATA_DIR/stashbert.db)            │
│      │                                                   │
│      ├── Lookup-Client ──────────────► Open Food Facts   │
│      ├── Hintergrund-Jobs (Nachladen, Bilder, Backup)    │
│      └── interne Ereignisse (M1 ohne Empfänger)          │
└──────────────────────────────────────────────────────────┘
```

## 4. Komponenten

### 4.1 Backend (Go)

| Paket | Aufgabe |
|---|---|
| `cmd/stashbert` | Einstieg: Konfiguration laden, DB öffnen und migrieren, Dienste erzeugen, Jobs starten, HTTP-Server. Nur Verdrahtung, keine Logik |
| `internal/app` | **Zusammenbau (Composition Root):** `NewHandler(cfg config.Config, deps Deps) (http.Handler, error)` baut die komplette Handler-Kette (4.4). Tests der API nutzen genau diese Funktion |
| `internal/config` | Umgebungsvariablen lesen und prüfen (Tabelle in 9.2) |
| `internal/api` | generierter Code aus `api/openapi.yaml` (`gen.go`) und die Handler-Implementierung: `type Server struct`, `func NewServer(d ServerDeps) *Server`, Handler-Methoden in `handlers_<bereich>.go` |
| `internal/httpx` | Middleware: Problem Details, Request-Validierung, Logging, statische Auslieferung |
| `internal/store` | SQLite öffnen, Pragmas, Migrationen (`migrations/`), sqlc-Abfragen (`queries/`) und generierter Code |
| `internal/domain` | Fachlogik: Produkte, Barcodes, Buchungen, Einkauf, Zusammenführen |
| `internal/gtin` | Barcode-Normalisierung und Prüfziffer |
| `internal/lookup` | Open-Food-Facts-Client, Rate-Limiter, Hintergrund-Nachladen, Bild-Download |
| `internal/events` | Interface für Domänen-Ereignisse, in M1 eine No-op-Implementierung |
| `internal/backup` | tägliches Backup und Backup vor Migrationen |
| `internal/webui` | `go:embed` der gebauten Web-Oberfläche |

### 4.2 Web-Oberfläche

- Statische SPA in `web/`, TypeScript, Vite, Tailwind CSS 4. Das Framework wird per A/B-Vergleich in Phase 0 festgelegt (ADR-0007).
- API-Zugriff nur über den aus `api/openapi.yaml` generierten Client (`openapi-typescript` + `openapi-fetch`).
- Das Build-Ergebnis `web/dist` wird von `make build` nach `internal/webui/dist/` kopiert und von dort ins Go-Binary eingebettet (`//go:embed all:dist`). Im Repository liegt dort nur `.gitkeep`. Fehlt der Build, liefert `/` eine kurze Textseite „Web-Oberfläche nicht gebaut".
- Ansichten in M1:
  - Vorrat
  - Scanner
  - Einkauf
  - Produktdetail
- Navigation: untere Leiste mit Vorrat, Scan (mittig, hervorgehoben) und Einkauf.

### 4.3 Scanner im Browser

Festgelegt in ADR-0008:

- Import `barcode-detector/ponyfill`, nie das Polyfill.
- Das WASM (zxing-wasm) wird selbst gehostet und vom Service Worker vorab gecacht.
- Formate: `ean_13`, `ean_8`, `upc_a`. UPC-E wird nicht unterstützt (7.1).
- Der Kamera-Stream bleibt offen, solange die Scanner-Ansicht offen ist. Bei `visibilitychange` auf `hidden` werden die Tracks gestoppt.
- Derselbe Code wird innerhalb von 2 s nur einmal verarbeitet.
- Rückmeldung: farbige Fläche und Ton über Web Audio. `navigator.audioSession.type = "playback"` wird gesetzt, falls verfügbar.

### 4.4 Handler-Kette und Fehler-Muster

Kette für `/api/v1/` von außen nach innen:

1. Recover
2. Logging
3. `http.StripPrefix("/api/v1")`
4. Request-Validator (`nethttp-middleware`, umschließt den gesamten generierten Mux)
5. generierter std-http-Handler mit Strict-Handler

Eine Anmeldung gibt es in M1 nicht (ADR-0013). Hinweis für eine spätere Strict-Middleware: oapi-codegen übergibt ihr die Go-Namen (`GetHealth`), nicht die `operationId` aus der Spec (`getHealth`).

**Fehler-Muster (verbindlich für alle Handler):**

- In der Spec hat jede Operation ihre Erfolgsantwort(en) und **`default`** mit `Problem` (`application/problem+json`). Die möglichen `code`-Werte stehen in der `description` der Operation. Einzelne Fehlerstatus werden nicht als eigene Responses angelegt.
- Handler geben Fachfehler als Go-Fehler `*httpx.Error{Status, Code, Title, Detail}` zurück, z. B. `return nil, httpx.NotFound("Produkt nicht gefunden")`.
- Der `ResponseErrorHandlerFunc` des Strict-Handlers wandelt `*httpx.Error` per `errors.As` in Problem Details um. Alle anderen Fehler werden 500 `internal` (mit Log).
- `RequestErrorHandlerFunc` (Strict) und `ErrorHandlerFunc` (std-http, Parameter-Bindung) ergeben 400 `invalid_request`.
- Validator:
  - Fehlerbehandlung über `ErrorHandlerWithOpts`.
  - Der Fehlerwert `routers.ErrMethodNotAllowed` ergibt 405 `method_not_allowed`.
  - Keine passende Route ergibt 404 `not_found`.
  - Alle anderen Validierungsfehler ergeben 400 `invalid_request`.

## 5. Datenmodell (SQLite, STRICT-Tabellen)

Alle Spalten sind `NOT NULL`, sofern hier nicht ausdrücklich „NULL" steht. Alle Zeitstempel: TEXT im festen Format RFC 3339 UTC mit Millisekunden, Go-Layout `2006-01-02T15:04:05.000Z` (Hilfsfunktion in `internal/store`). Alle IDs: TEXT, UUIDv7 in Kleinbuchstaben.

**`settings`**: `key` TEXT PK, `value` TEXT NOT NULL. In M1 ohne Einträge; vorgesehen für spätere Einstellungen (z. B. die Ziel-Einkaufsliste in M2).

**`products`**:

| Spalte | Typ | Regel |
|---|---|---|
| `id` | TEXT PK | UUIDv7 |
| `name` | TEXT | NOT NULL, 1 bis 120 Zeichen |
| `brand` | TEXT | NULL, bis 120 |
| `package_size` | TEXT | NULL, bis 40 (z. B. „400 g") |
| `stock` | INTEGER | NOT NULL, ≥ 0, Standard 0 |
| `target` | INTEGER | NOT NULL, 0 bis 100000, Standard 0 |
| `min_stock` | INTEGER | NULL, 0 bis 100000 und ≤ `target` |
| `note` | TEXT | NULL, bis 500 |
| `origin` | TEXT | `openfoodfacts`, `openbeautyfacts`, `openpetfoodfacts`, `openproductsfacts`, `manual`, `placeholder` |
| `lookup_state` | TEXT | `none`, `pending`, `done`, `not_found` |
| `needs_review` | INTEGER | 0/1 |
| `image_source_url` | TEXT | NULL, Bild-URL von OFF |
| `image_file` | TEXT | NULL, Dateiname in `DATA_DIR/images/` |
| `created_at`, `updated_at` | TEXT | `updated_at` ändert sich bei jeder Änderung der Zeile `products`, also bei Stammdaten (PATCH), jeder Buchung (auch `inventory` mit `delta` 0), Storno, Zusammenführen und Nachladen. Barcodes zuordnen oder entfernen ändert es nicht. |

**`barcodes`**: `code` TEXT PK (normalisiert, siehe 7.1), `product_id` FK ON DELETE CASCADE, `units` INTEGER NOT NULL 1 bis 1000, Standard 1, `created_at`.

**`movements`** (nur anhängen; einzige Ausnahme: Zusammenführen hängt Buchungen auf das Zielprodukt um, siehe 6.5):

| Spalte | Typ | Regel |
|---|---|---|
| `id` | TEXT PK | UUIDv7 |
| `product_id` | TEXT | FK ON DELETE CASCADE |
| `kind` | TEXT | `add`, `consume`, `inventory`, `reversal`, `merge` |
| `delta` | INTEGER | Änderung des Bestands, darf 0 sein (Inventur ohne Änderung) |
| `stock_after` | INTEGER | ≥ 0 |
| `barcode` | TEXT | NULL |
| `reverses_id` | TEXT | NULL, UNIQUE, FK auf `movements` |
| `idempotency_key` | TEXT | NULL, UNIQUE |
| `request_hash` | TEXT | NULL, SHA-256 des Request-Bodys (für Idempotenz) |
| `created_at` | TEXT | |

**`lookups`** (Cache): `code`, `source` (in M1 nur `off`), `found` 0/1, `payload` TEXT (JSON des gemappten Ergebnisses `lookup.Result`; bei nicht gefunden SQL-`NULL`), `fetched_at`; PK (`code`, `source`).

**Abgeleitete Werte:**

- `missing = (target > 0 und stock < coalesce(min_stock, target)) ? target - stock : 0`
- Ein Produkt steht auf der Einkaufsliste, wenn `missing > 0`.

## 6. API v1

Vollständiger Vertrag: `api/openapi.yaml` (OpenAPI 3.1). Diese Übersicht ist die Vorgabe, nach der die Spec in den Tasks geschrieben wird. Weicht die Spec ab, gilt diese Übersicht, bis ein ADR sie ändert.

### 6.1 Allgemeine Regeln

- Basis-Pfad `/api/v1`. JSON-Felder in `snake_case`. Zeitstempel sind RFC 3339 in UTC (`format: date-time`); die Zahl der Nachkommastellen ist in der API nicht fest, das feste Format gilt nur in der Datenbank.
- Fehler: RFC 9457, `application/problem+json`, mit den Feldern `type` (`about:blank`), `title`, `status`, `detail`, `code`. Die Liste der `code`-Werte steht in 6.4.
- Ändernde Aufrufe (`POST`, `PATCH`, `DELETE`) verlangen `Content-Type: application/json`, sofern sie einen Body haben.
- Keine Anmeldung (ADR-0013). Es werden keine CORS-Header gesetzt; zusammen mit der JSON-Pflicht für Bodies verhindert der Browser, dass fremde Webseiten Buchungen auslösen.
- Idempotenz: optionaler Header `Idempotency-Key` bei `POST /movements` und `POST /movements/{id}/reversal`.
  - Gleicher Schlüssel mit gleichem Body: keine neue Buchung, sondern 201 mit einer aus der gespeicherten Buchung **rekonstruierten** Antwort (aktuelles Produkt, `product_created: false`, `warnings: []`, `message` aus der Buchung).
  - Gleicher Schlüssel mit anderem Body liefert 422 mit `idempotency_key_mismatch`.
- Neu angelegte Ressourcen: `201`. Einen `Location`-Header gibt es nur, wenn für die Ressource ein `GET`-Endpunkt existiert (Produkte).
- `PATCH` nutzt JSON-Merge-Patch-Semantik mit `Content-Type: application/json`: Ein fehlendes Feld bleibt unverändert, `null` löscht ein nullbares Feld. Im Go-Code wird das über `nullable.Nullable` abgebildet (oapi-codegen, Option `nullable-type`).
- Keine Paginierung bei Produkten (höchstens ca. 1000). Cursor-Paginierung bei Buchungen.

### 6.2 Endpunkte

`operationId` ist verbindlich, weil daraus die Namen im generierten Go- und TypeScript-Code entstehen. Pfade in der Spec stehen ohne `/api/v1`, das Präfix kommt aus `servers`.

| Methode und Pfad | `operationId` | Zweck | Erfolg | Fehler-`code` |
|---|---|---|---|---|
| `GET /health` | `getHealth` | Status | 200 `{status, version}` | |
| `GET /products` | `listProducts` | alle Produkte, sortiert nach Name | 200 `{items: Product[]}` | |
| `POST /products` | `createProduct` | Produkt manuell anlegen | 201 `Product` | `barcode_in_use` (409), `invalid_barcode` (422) |
| `GET /products/{id}` | `getProduct` | ein Produkt | 200 `Product` | `not_found` |
| `PATCH /products/{id}` | `updateProduct` | ändern (JSON Merge Patch) | 200 `Product` | `not_found`, `invalid_request` |
| `DELETE /products/{id}` | `deleteProduct` | löschen inkl. Barcodes und Buchungen | 204 | `not_found` |
| `POST /products/{id}/barcodes` | `addBarcode` | Barcode zuordnen `{code, units?}` | 201 `Barcode` | `barcode_in_use` (409), `invalid_barcode` (422), `not_found` |
| `DELETE /products/{id}/barcodes/{code}` | `removeBarcode` | Barcode entfernen | 204 | `not_found` |
| `POST /products/{id}/merge` | `mergeProduct` | in anderes Produkt überführen `{target_product_id}` | 200 `Product` (Ziel) | `not_found`, `invalid_request` (Ziel = Quelle) |
| `GET /products/{id}/image` | `getProductImage` | Produktbild | 200 Bild | `not_found` |
| `POST /movements` | `createMovement` | Buchung anlegen | 201 `MovementResult` | siehe 6.3 |
| `GET /movements` | `listMovements` | Buchungen, neueste zuerst, `?product_id=&limit=&cursor=` (`limit` 1 bis 200, Standard 50) | 200 `{items: Movement[], next_cursor}` | |
| `POST /movements/{id}/reversal` | `reverseMovement` | Buchung stornieren | 201 `MovementResult` | `already_reversed` (409), `not_reversible` (409), `not_found` |
| `GET /shopping-list` | `getShoppingList` | Einkaufsliste, sortiert nach Name | 200 `{items: ShoppingItem[]}` | |

Zusätzlich, **nicht** in der Spec beschrieben: `GET /api/v1/openapi.yaml` liefert die eingebettete Spec aus (öffentlich).

### 6.3 Buchungsregeln (`POST /movements`)

Request: `{product_id?, barcode?, kind, quantity?, stock?}`. Genau eines von `product_id` und `barcode` ist gesetzt.

- `kind` ist `add`, `consume` oder `inventory`.
- `quantity` (1 bis 1000, Standard 1) gilt für `add` und `consume`.
- `stock` (0 bis 100000, Pflicht) gilt für `inventory`.
- Die Obergrenzen stehen als `maximum` in der Spec und schließen Überläufe bei `quantity × units` aus. Überschreitungen ergeben 400 `invalid_request`.

| Fall | Ergebnis |
|---|---|
| `add` mit bekanntem Produkt | Bestand + `quantity × units` (per `product_id` ist `units` = 1) |
| `consume`, Bestand ≥ Menge | Bestand - Menge; Menge = `quantity × units` (per `product_id` ist `units` = 1) |
| `consume`, 0 < Bestand < Menge | Bestand wird 0, `warnings: ["clamped_to_zero"]` |
| `consume`, Bestand = 0 | 409 `stock_already_zero`, keine Buchung |
| `inventory` | Bestand = `stock`, `delta` = Differenz (darf 0 sein); `units` spielt keine Rolle |
| `barcode` ungültig (Länge, Ziffern, Prüfziffer) | 422 `invalid_barcode` |
| `barcode` unbekannt, `add` | Produkt anlegen (7.2), dann buchen; `product_created: true`, bei Platzhalter zusätzlich `warnings: ["placeholder_created"]` |
| `barcode` unbekannt, `consume` oder `inventory` | 404 `unknown_barcode` |
| `product_id` unbekannt | 404 `not_found` |

**`delta` ist immer die tatsächliche Änderung** (`stock_after - stock_vorher`), auch bei Begrenzung auf 0. Beispiel: Bestand 1, `consume` 3 ergibt `delta = -1`.

Storno (`POST /movements/{id}/reversal`):

- Legt eine Buchung `kind: reversal` an, Ziel ist `-original.delta`.
- Würde der Bestand negativ, wird er 0 mit `warnings: ["clamped_to_zero"]`; `delta` ist dann die tatsächliche Änderung.
- Buchungen der Arten `reversal` und `merge` sind nicht stornierbar: 409 `not_reversible`.

Produkt und Buchung werden in **einer** Transaktion gespeichert. Nach dem Commit wird ein Domänen-Ereignis ausgelöst (ADR-0011).

`MovementResult`:

```json
{
  "movement": { "id": "…", "product_id": "…", "kind": "consume", "delta": -1, "stock_after": 2,
                "barcode": "4001234567890", "reverses_id": null, "created_at": "…" },
  "product": { "…": "Product, siehe 6.5" },
  "product_created": false,
  "warnings": [],
  "message": "Kidneybohnen 3 → 2"
}
```

`message` ist ein fertiger deutscher Anzeigetext. Format: `"<name> <stock_vorher> → <stock_nachher>"` mit `stock_vorher = stock_after - delta`; bei neu angelegtem Produkt `"Neu: <name> 0 → <stock_nachher>"`.

### 6.4 Fehlercodes

| `code` | Status |
|---|---|
| `invalid_request` | 400 |
| `not_found` | 404 |
| `unknown_barcode` | 404 |
| `barcode_in_use` | 409 |
| `stock_already_zero` | 409 |
| `already_reversed` | 409 |
| `not_reversible` | 409 |
| `method_not_allowed` | 405 |
| `invalid_barcode` | 422 |
| `idempotency_key_mismatch` | 422 |
| `internal` | 500 |

### 6.5 Schemas (Kurzform)

- **Product:**
  - `id`, `name`, `brand|null`, `package_size|null`, `note|null`
  - `stock` (nur lesen), `target`, `min_stock|null`, `missing` (nur lesen)
  - `needs_review`, `origin`, `lookup_state`, `has_image`
  - `barcodes: Barcode[]`, `created_at`, `updated_at`
- **Barcode:** `code`, `units`
- **Leere Texte:** `brand`, `package_size` und `note` werden getrimmt; ist das Ergebnis leer, wird `NULL` gespeichert (bei Anlegen und Ändern).
- **ProductCreate:** `name` (Pflicht), `brand?`, `package_size?`, `target?` (Standard 0), `min_stock?`, `note?`, `barcodes?: [{code, units?}]`
- **ProductPatch (JSON Merge Patch):**
  - Felder: `name`, `brand`, `package_size`, `target`, `min_stock`, `note`.
  - `null` löscht ein nullbares Feld.
  - `needs_review` wird nur dann `false`, wenn der Patch mindestens eines der Felder `name`, `brand` oder `package_size` enthält. Ein Patch nur mit `target` ändert `needs_review` nicht.
  - Nullbare Felder werden in der Spec als `type: [<typ>, "null"]` geschrieben (OpenAPI 3.1), nicht mit `nullable: true`.
- **ShoppingItem:** `product_id`, `name`, `brand|null`, `missing`, `stock`, `target`
- **Merge:**
  - Barcodes und Buchungen der Quelle gehen auf das Ziel über.
  - Das Ziel bekommt eine Buchung `kind: merge` mit `delta` = Bestand der Quelle.
  - Danach wird die Quelle gelöscht.

### 6.6 Domänen-Ereignisse (intern, M1 ohne Empfänger)

| Typ | Go-Konstante | Wann | `data` |
|---|---|---|---|
| `product.created` | `TypeProductCreated` | Produkt angelegt (manuell oder per Scan) | `product_id`, `name`, `origin` |
| `stock.added` | `TypeStockAdded` | Buchung `add` | `product_id`, `movement_id`, `delta`, `stock_after` |
| `stock.consumed` | `TypeStockConsumed` | Buchung `consume` | wie `stock.added` |
| `stock.adjusted` | `TypeStockAdjusted` | Buchung `inventory`, `reversal` oder `merge` | wie `stock.added` |
| `product.empty` | `TypeProductEmpty` | Bestand wechselt von > 0 auf 0 | `product_id`, `name` |
| `shopping.changed` | `TypeShoppingChanged` | `missing` ändert sich, auch beim Löschen oder Zusammenführen eines Produkts mit `missing > 0` (dann `missing_after = 0`) | `product_id`, `name`, `missing_before`, `missing_after` |

Ereignisse werden erst **nach** dem Commit ausgelöst, in dieser Reihenfolge: `product.created`, Buchungsereignis, `product.empty`, `shopping.changed`. Die Buchungsereignisse gibt es nur, wenn `delta ≠ 0`.

## 7. Barcodes und Open Food Facts

### 7.1 Normalisierung (`internal/gtin`)

- Nur Ziffern. Erlaubte Längen: 8, 12, 13, 14.
- Die Prüfziffer (Modulo 10, Gewichte 3/1 von rechts) muss stimmen.
- 12 Stellen (UPC-A): eine `0` voranstellen, ergibt 13 Stellen.
- 14 Stellen mit führender `0`: die `0` entfernen, ergibt 13. Andere 14-stellige Codes bleiben unverändert.
- 8 Stellen bleiben unverändert und werden als EAN-8 geprüft. **UPC-E wird nicht unterstützt**, weil dessen Prüfziffer über die expandierte Form berechnet wird; der Scanner liest UPC-E deshalb gar nicht erst.
- **Lokale Codes:** 13-stellige Codes mit Präfix `20` bis `29` oder `02` werden nie extern nachgeschlagen.

### 7.2 Unbekannter Barcode beim Einlagern

1. Cache (`lookups`) prüfen. Ein Treffer jünger als 30 Tage wird verwendet, auch ein negativer.
2. Sonst Open Food Facts mit einem **Zeitbudget von 2,5 s**:
   - Endpunkt: `GET https://world.openfoodfacts.org/api/v3.6/product/{code}?product_type=all&lc=de&fields=code,product_name,product_name_de,generic_name_de,brands,quantity,product_quantity,product_quantity_unit,image_front_url,product_type`
   - Redirects auf Schwester-Datenbanken folgen.
   - User-Agent: `StashBert/<version> (<OFF_CONTACT>)`
3. **Treffer:**
   - `name` = `product_name_de`, sonst `product_name`, sonst `generic_name_de`.
   - `brand` = erster Eintrag aus `brands`.
   - `package_size` = `quantity`, sonst `product_quantity` und `product_quantity_unit` mit Leerzeichen dazwischen (`300 ml`).
   - `origin` aus `product_type`: `food` wird `openfoodfacts`, `beauty` wird `openbeautyfacts`, `petfood` wird `openpetfoodfacts`, `product` wird `openproductsfacts`. Ein leerer oder unbekannter `product_type` wird `openfoodfacts`.
   - `image_source_url` = `image_front_url`, `lookup_state = done`, `needs_review = true`.
   - Ist kein Name vorhanden, ist `name = "Neues Produkt <code>"`.
   - Werte werden auf die Längen aus Kapitel 5 gekürzt (nach Zeichen, nicht nach Bytes): `name` und `brand` 120, `package_size` 40. Leerzeichen am Ende nach dem Kürzen fallen weg.
   - Der Cache-Eintrag hat `source = off` und als `payload` das gemappte Ergebnis (`lookup.Result` als JSON mit Feldern in `snake_case`).
4. **Kein Treffer (404):** Platzhalter mit `name = "Neues Produkt <code>"`, `origin = placeholder`, `lookup_state = not_found`, `needs_review = true`.
5. **Timeout, 429, 503, Netzfehler oder fehlendes `OFF_CONTACT`:** Platzhalter wie in 4, aber `lookup_state = pending`.
6. Bei lokalen Codes (7.1) gibt es keinen Lookup: Platzhalter mit `lookup_state = none`.

Danach wird mit `target = 0` gebucht.

### 7.3 Hintergrund-Jobs

- **Nachladen:**
  - Alle 60 s werden Produkte mit `lookup_state = pending` erneut nachgeschlagen.
  - Bei einem Treffer wird `lookup_state` **immer** auf `done` gesetzt. `name`, `brand`, `package_size`, `origin` und `image_source_url` werden **nur übernommen, wenn `needs_review` noch `true` ist**.
  - Nicht gefunden ergibt `not_found`, andere Fehler lassen `pending` stehen. Ein Produkt ohne Barcode (alle entfernt) bekommt `none`. Ohne `OFF_CONTACT` endet jeder Lauf sofort.
  - Gemeinsamer Rate-Limiter für alle OFF-Anfragen: höchstens 10 pro Minute (`golang.org/x/time/rate`, 1 Token alle 6 s, Burst 1). Er wird dem Client übergeben und gilt pro `Lookup`-Aufruf, nicht pro HTTP-Anfrage (Redirects zählen nicht extra).
  - **Vorrang für Scans:** Interaktive Lookups warten mit `Wait(ctx)` innerhalb des Budgets von 2,5 s. Das Nachladen im Hintergrund nimmt nur mit `Allow()` einen Token und beendet den Lauf, wenn keiner frei ist. Timeout pro Nachlade-Anfrage: 10 s.
  - **Gewollte Folgen:**
    - Mit Burst 1 bearbeitet ein Nachlade-Lauf meist nur ein Produkt pro Minute. Bei den erwarteten Mengen reicht das.
    - Wird ein unbekanntes Produkt kurz nach einem Nachlade-Token gescannt, kann es zum Platzhalter werden und wird dann im Hintergrund ergänzt.
- **Bilder:**
  - Für Produkte mit `image_source_url` und ohne `image_file` wird das Bild geladen.
  - Nur von `images.openfoodfacts.org`, `images.openbeautyfacts.org`, `images.openpetfoodfacts.org` und `images.openproductsfacts.org`.
  - Höchstens 2 MB, Typen `image/jpeg`, `image/png` oder `image/webp`. Auch Redirect-Ziele müssen erlaubte Hosts sein.
  - Dauerhafte Fehler (4xx außer 408 und 429, falscher Typ, leer oder zu groß) setzen `image_source_url = NULL`; vorübergehende werden im nächsten Lauf wiederholt.
  - Ablage unter `DATA_DIR/images/<product_id>.<ext>`. Nie im Scan-Pfad.
- **Backup:** siehe 9.3.

## 8. Zugriffsschutz und Sicherheits-Header

- **Keine Anmeldung in M1** (ADR-0013). Wer die Web-Oberfläche oder die API erreicht, darf alles. Schutz bietet allein das Netz: nur im Heimnetz erreichbar, später von unterwegs nur per VPN.
- **Schutz vor fremden Webseiten:** keine CORS-Header, Bodies nur als `application/json`. Damit können Webseiten, die jemand im Heimnetz aufruft, keine Buchungen auslösen.
- **Nachrüsten:** Eine Anmeldung lässt sich später ohne Änderung der Endpunkte ergänzen, entweder vorgelagert am Reverse Proxy (Forward-Auth) oder per OIDC in StashBert.
- **Sicherheits-Header** für alle Antworten der Web-Oberfläche. Inline-Skripte in `index.html` (SvelteKit erzeugt welche) werden erlaubt, indem `internal/webui` beim Start den SHA-256 jedes Inline-`<script>` der eingebetteten `index.html` berechnet und als `'sha256-…'` an `script-src` anhängt. `'unsafe-inline'` für Skripte ist nicht erlaubt.
  - `Content-Security-Policy: default-src 'self'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self'; script-src 'self' 'wasm-unsafe-eval' <Hashes der Inline-Skripte>; style-src 'self' 'unsafe-inline'; worker-src 'self'; manifest-src 'self'; frame-ancestors 'none'`
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: same-origin`


## 9. Betrieb

### 9.1 Artefakt

- Ein statisch gelinktes Go-Binary (`CGO_ENABLED=0`). Es enthält API, Web-Oberfläche, Migrationen und `openapi.yaml` (ausgeliefert unter `GET /api/v1/openapi.yaml`, öffentlich).
- Container-Image `gcr.io/distroless/static-debian13:nonroot` plus Binary. Multi-Arch: amd64 und arm64. Das Verzeichnis `/data` wird im Image mit Besitzer 65532 angelegt, damit ein neues Volume beschreibbar ist.
- Docker Compose mit genau einem Dienst. TLS, DNS und Reverse Proxy stellt der Betreiber.
- **Anforderungen an den vorgelagerten Proxy:**
  - HTTPS mit einem Zertifikat, dem das iPhone vertraut. Ohne sichere Verbindung gibt iOS die Kamera nicht frei.
  - Weiterleitung aller Pfade an Port 8080, ohne Umschreibung auf einen Unterpfad.
  - Die Header `X-Forwarded-For` und `X-Forwarded-Proto` werden gesetzt.

### 9.2 Konfiguration

| Variable | Standard | Bedeutung |
|---|---|---|
| `PORT` | `8080` | HTTP-Port |
| `DATA_DIR` | `/data` | Datenbank, Bilder, Backups |
| `OFF_CONTACT` | leer | Kontakt für den OFF-User-Agent; leer bedeutet keine OFF-Lookups (nur Platzhalter) |
| `BACKUP_KEEP` | `14` | Anzahl aufbewahrter täglicher Backups |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |

### 9.3 Backup

- **Beim Start und danach alle 24 h:** `VACUUM INTO 'DATA_DIR/backups/stashbert-<YYYYMMDD-HHMMSS>.db'`. Es bleiben die neuesten `BACKUP_KEEP` Dateien erhalten.
- **Vor Migrationen:** Stehen Migrationen an und existiert die DB schon, wird vorher `pre-migration-<YYYYMMDD-HHMMSS>.db` geschrieben (wird nicht automatisch gelöscht).
- **Restore:** Container stoppen, `stashbert.db`, `-wal` und `-shm` entfernen, Backup als `stashbert.db` ablegen, Container starten.

## 10. Später (nicht M1)

Nur als Orientierung; nichts davon wird in M1 vorbereitet, außer den Leitplanken aus Kapitel 2.

- **M2 Integrationen:** API-Tokens mit Scopes, Outbox und MQTT 5 (Mosquitto) mit HA-Discovery, Bring! über HA `todo.*` mit wählbarer Ziel-Liste, Summary-Endpunkt. Details in `research.md`, Kapitel 10 und 11.
- **M3 Hardware:** ESP32 mit ESPHome, bucht über `POST /api/v1/movements` mit Barcode (research.md, Kapitel 9).
- **Weitere Optionen:**
  - OIDC-Anmeldung (Google oder Pocket ID)
  - Kategorien, Lagerorte
  - MHD
  - Mindestbestand in der Oberfläche
  - Offline-Buchen mit client-seitigen UUIDv7
  - Kurzbefehl
  - Swift-Hülle (research.md, 7.9)
