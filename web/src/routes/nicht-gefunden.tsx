import { Link } from "react-router";

export function NotFoundPage() {
  return (
    <main className="px-4 py-6">
      <h1 className="text-2xl font-semibold">Nicht gefunden</h1>
      <p className="mt-2 text-ink-secondary">Diese Seite gibt es nicht.</p>
      <Link
        to="/vorrat"
        className="pressable mt-6 inline-flex min-h-11 items-center rounded-lg bg-accent px-4 font-medium text-white"
      >
        Zum Vorrat
      </Link>
    </main>
  );
}
