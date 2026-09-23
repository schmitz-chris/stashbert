import { useParams } from "react-router";

export function ProductPage() {
  const { id } = useParams();

  return (
    <main className="px-4 py-6">
      <h1 className="text-2xl font-semibold">Produkt</h1>
      <p className="mt-2 font-mono text-sm break-all text-stone-500">{id}</p>
    </main>
  );
}
