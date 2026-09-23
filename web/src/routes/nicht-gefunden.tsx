import { Link } from "react-router";

export function NotFoundPage() {
  return (
    <main className="px-4 py-6">
      <h1 className="text-2xl font-semibold">Nicht gefunden</h1>
      <p className="mt-2 text-stone-600">Diese Seite gibt es nicht.</p>
      <Link
        to="/vorrat"
        className="mt-6 inline-flex min-h-11 items-center rounded-lg bg-emerald-600 px-4 font-medium text-white"
      >
        Zum Vorrat
      </Link>
    </main>
  );
}
