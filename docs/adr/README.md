# Architecture Decision Records

Format: Status, Kontext, Entscheidung, Konsequenzen, Alternativen (siehe ADR-0001).

| Nr. | Titel | Status |
|---|---|---|
| [0001](0001-adrs-verwenden.md) | Architekturentscheidungen als ADRs festhalten | angenommen |
| [0002](0002-backend-go.md) | Backend in Go | angenommen |
| [0003](0003-api-contract-first.md) | REST-API contract-first mit OpenAPI und oapi-codegen | angenommen |
| [0004](0004-buchungen-als-ressource.md) | Bestandsänderungen als Buchungen (Ressource `movements`) | angenommen |
| [0005](0005-datenhaltung-sqlite.md) | Datenhaltung mit SQLite, sqlc und goose | angenommen |
| [0006](0006-ids-uuidv7.md) | IDs als UUIDv7 | angenommen |
| [0007](0007-frontend-ab-vergleich.md) | Frontend als SPA, Framework per A/B-Vergleich | angenommen (React) |
| [0008](0008-barcode-scan-browser.md) | Barcode-Scan im Browser mit barcode-detector und zxing-wasm | angenommen |
| [0009](0009-anmeldung.md) | Anmeldung mit Haushaltspasswort, OIDC später | ersetzt durch ADR-0013 |
| [0010](0010-betrieb-ein-binary.md) | Betrieb als ein Binary mit einem HTTP-Port | angenommen, ergänzt durch ADR-0014 |
| [0011](0011-ereignisse-und-integrationen.md) | Interne Domänen-Ereignisse ab M1, Integrationen ab M2 | angenommen |
| [0012](0012-agentengetriebene-entwicklung.md) | Agentengetriebene Entwicklung mit kleinteiligem Plan | angenommen |
| [0013](0013-keine-anmeldung-in-m1.md) | Keine Anmeldung in M1 | angenommen |
| [0014](0014-betrieb-im-lxc.md) | Betrieb im Proxmox-LXC mit systemd | angenommen |
| [0015](0015-vormerken-fuer-den-einkauf.md) | Vormerken für den Einkauf | angenommen |
| [0016](0016-oberflaeche-nach-hig.md) | Oberfläche nach Apples Human Interface Guidelines, kein Dark Mode in M1 | angenommen |
