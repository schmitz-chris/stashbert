# ADR-0007: Frontend als SPA, Framework per A/B-Vergleich

- Status: angenommen (React, P0-6)
- Datum: 2026-09-23

## Kontext

- Das Frontend wird überwiegend von Agenten geschrieben.
- React hat das meiste Trainingsmaterial.
- Svelte 5 ist moderner und knapper, aber LLMs mischen gern alte Svelte-4-Syntax hinein. Dagegen gibt es offizielle Hilfen: den Svelte-MCP-Server und das Claude-Code-Plugin `sveltejs/ai-tools`.

## Entscheidung

- **Architektur:** Die Web-Oberfläche ist eine statische SPA, gebaut mit TypeScript, Vite und Tailwind CSS 4 (CSS-first, keine `tailwind.config.js`). Sie wird ins Go-Binary eingebettet und spricht nur die öffentliche API über den generierten Client.
- **Framework:** wird im A/B-Vergleich in Phase 0 bestimmt. Ein Agent baut dieselbe Scanner-Testseite zweimal, jeweils nur mit der Basis (Framework, Vite, TypeScript, Tailwind CSS 4):
  - **Variante S:** Svelte 5 + SvelteKit 2 im SPA-Modus (`adapter-static`, `ssr = false`), mit dem offiziellen Svelte-MCP bzw. Plugin.
  - **Variante R:** React 19 + Vite.
- **Router und Komponenten:** Router, Datenlade-Bibliothek und Komponentenbibliothek werden erst in P0-6 festgelegt, als exakte npm-Liste in `AGENTS.md`. Naheliegend sind bei S shadcn-svelte, bei R TanStack Router, TanStack Query und shadcn/ui.
- **Kriterien:**
  1. Funktioniert es auf iPhone 15 und 16 Pro?
  2. Wie viel Nacharbeit brauchte der Agent?
  3. Wie gut ist der Code verständlich?
  4. Wie fühlt es sich an?
- **Unentschieden:** Dann gilt React.

## Ergebnis (P0-6)

**Gewählt: Variante R, React 19 mit Vite.**

- **Funktion:** Beide Varianten liefen auf dem iPhone (HTTPS mit selbst signiertem Zertifikat, Standardkamera des 16 Pro reicht auch aus der Nähe, Kamera-Rückfrage nur einmal).
- **Nacharbeit:** React brauchte keine Korrekturschleife bis zum grünen Build, Svelte eine (Svelte-Autofixer).
- **Praxis:** Der React-Prototyp wurde zusätzlich ans Backend angeschlossen und bucht per Scan. Der Nutzer hat den Ablauf auf dem iPhone getestet: „das Scannen fühlt sich gut an".
- **Agenten:** React hat das meiste Trainingsmaterial; das war das Hauptkriterium.
- Der Nutzer hat sich für React entschieden. Einen detaillierten Vergleich der Erkennung (P0-5) gab es nicht; er war für die Entscheidung nicht nötig.

**Bibliotheken** (exakte Liste in `AGENTS.md`, Abschnitt Frontend-Regeln):

- **Router:** React Router 7 (`react-router`) im Data-Modus mit `createBrowserRouter`, ohne Loader und Actions. Version 8 ist seit Juni 2026 erschienen und für Agenten noch zu neu; 7 wird weiter gepflegt.
- **Daten:** TanStack Query 5 mit dem generierten `openapi-fetch`-Client.
- **Komponenten:** keine Komponentenbibliothek. Tailwind direkt und native Elemente (`<dialog>`). Die wenigen Ansichten rechtfertigen den Einrichtungsaufwand von shadcn/ui nicht; das bleibt eine spätere Option.
- **Tests und Lint:** Vitest für reine Funktionen; ESLint wie in der Vite-Vorlage `react-ts`.
- **PWA:** `vite-plugin-pwa`.
- **Nicht gewählt:** TanStack Router (weniger Trainingsmaterial, Codegenerierung für dateibasierte Routen), globale Zustandsbibliotheken (Redux, Zustand), React Compiler (später möglich).

## Konsequenzen

- `AGENTS.md` enthält die Frontend-Regeln für React und die npm-Liste.
- Die F-Tasks im Plan nennen React; das Scanner-Modul (F07) wird aus `poc/scanner-react/` übernommen.
- Vite legt Assets unter `/assets/` ab und erzeugt keine Inline-Skripte. Die Einbettung aus B29 passt ohne Änderung.

## Alternativen

- **Next.js, Nuxt:** Ihre Server-Funktionen werden nicht gebraucht.
- **Vue:** gut, aber ohne Vorteil gegenüber den beiden Kandidaten.
- **htmx:** passt nicht zu einer Scanner-App.
