import { Link } from "react-router";
import { PageHeading } from "../components/PageHeading";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import { pageTitle } from "../lib/pageTitle";

export function NotFoundPage() {
  useDocumentTitle(pageTitle("not-found"));
  return (
    <main className="px-4 py-6">
      <PageHeading className="text-2xl font-semibold">Nicht gefunden</PageHeading>
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
