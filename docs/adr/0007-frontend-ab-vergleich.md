# ADR-0007: Frontend als SPA, Framework per A/B-Vergleich

- Status: vorgeschlagen (Entscheidung in Plan-Task P0-6)
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

## Konsequenzen

- Die Frontend-Tasks im Plan sind bis zur Entscheidung framework-neutral formuliert.
- Nach der Entscheidung wird dieses ADR auf „angenommen" gesetzt. `AGENTS.md` bekommt dann die framework-spezifischen Regeln, z. B. „nur Svelte-5-Runes".

## Alternativen

- **Next.js, Nuxt:** Ihre Server-Funktionen werden nicht gebraucht.
- **Vue:** gut, aber ohne Vorteil gegenüber den beiden Kandidaten.
- **htmx:** passt nicht zu einer Scanner-App.
