import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { problemCode } from "../lib/api/client";
import {
  productListQuery,
  productMovementMutation,
  type ProductMovement,
} from "../lib/api/queries";
import {
  filterProducts,
  matches,
  type Product,
  type ProductFilter,
} from "../lib/products";

const filters: { value: ProductFilter; label: string }[] = [
  { value: "all", label: "Alle" },
  { value: "restock", label: "Nachkaufen" },
  { value: "empty", label: "Leer" },
  { value: "review", label: "Prüfen" },
];

// How long the notice of a failed booking stays visible.
const noticeDuration = 2000;

export function StockPage() {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ProductFilter>("all");
  const products = useQuery(productListQuery);

  return (
    <>
      <h1 className="text-2xl font-semibold">Vorrat</h1>
      <input
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Name oder Marke suchen"
        aria-label="Vorrat durchsuchen"
        className="mt-4 min-h-11 w-full rounded-lg border border-stone-300 bg-white px-3 text-base"
      />
      <div role="group" aria-label="Filter" className="mt-3 flex flex-wrap gap-2">
        {filters.map(({ value, label }) => (
          <button
            key={value}
            type="button"
            aria-pressed={filter === value}
            onClick={() => setFilter(value)}
            className="min-h-11 rounded-full border border-stone-300 bg-white px-3 font-medium text-stone-700 aria-pressed:border-emerald-600 aria-pressed:bg-emerald-600 aria-pressed:text-white"
          >
            {label}
          </button>
        ))}
      </div>
      <div className="mt-4">
        {products.data === undefined ? (
          products.isError && !products.isFetching ? (
            <div>
              <p className="text-stone-700">
                Der Vorrat konnte nicht geladen werden.
              </p>
              <button
                type="button"
                onClick={() => void products.refetch()}
                className="mt-3 min-h-11 rounded-lg bg-emerald-600 px-4 font-medium text-white"
              >
                Erneut versuchen
              </button>
            </div>
          ) : (
            <p className="text-stone-500">Vorrat wird geladen …</p>
          )
        ) : (
          <StockList
            products={filterProducts(products.data, filter).filter((product) =>
              matches(product, query),
            )}
            isEmpty={products.data.length === 0}
          />
        )}
      </div>
    </>
  );
}

function StockList({
  products,
  isEmpty,
}: {
  products: Product[];
  isEmpty: boolean;
}) {
  if (isEmpty) {
    return (
      <p className="text-stone-500">
        Noch keine Produkte. Scanne einen Barcode, um eines einzulagern.
      </p>
    );
  }
  if (products.length === 0) {
    return <p className="text-stone-500">Keine Treffer.</p>;
  }
  return (
    <ul className="divide-y divide-stone-200 rounded-xl border border-stone-200 bg-white">
      {products.map((product) => (
        <StockRow key={product.id} product={product} />
      ))}
    </ul>
  );
}

function StockRow({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const movement = useMutation(productMovementMutation(queryClient));
  const { isError, reset } = movement;

  // Hides the notice of a failed booking after a short time.
  useEffect(() => {
    if (!isError) {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [isError, reset]);

  let notice = "";
  if (isError) {
    notice =
      problemCode(movement.error) === "stock_already_zero"
        ? "War schon leer"
        : "Buchung fehlgeschlagen";
  }

  function book(kind: ProductMovement["kind"]) {
    movement.mutate({ productId: product.id, kind });
  }

  // The link covers the whole row with its ::after box, so a tap anywhere
  // outside the buttons opens the product. The buttons lie above it.
  return (
    <li className="relative flex items-center gap-2 px-3 py-2">
      <ProductImage product={product} />
      <div className="min-w-0 flex-1">
        <Link
          to={`/produkt/${encodeURIComponent(product.id)}`}
          className="font-medium break-words hyphens-auto after:absolute after:inset-0"
        >
          {product.name}
        </Link>
        {product.brand !== null && (
          <p className="text-sm break-words hyphens-auto text-stone-500">
            {product.brand}
          </p>
        )}
        {product.marked && (
          <p className="text-xs font-medium text-amber-700">vorgemerkt</p>
        )}
        <p role="status" className="text-sm font-medium text-amber-700">
          {notice}
        </p>
      </div>
      {/* The stock large, below it the target; without a target only the stock. */}
      <div className="min-w-12 shrink-0 text-center tabular-nums">
        <p className="text-xl font-semibold text-stone-900">
          <span className="sr-only">Bestand </span>
          {product.stock}
        </p>
        {product.target > 0 && (
          <p className="text-xs text-stone-500">Soll {product.target}</p>
        )}
      </div>
      <div className="relative z-10 flex shrink-0 gap-2">
        <button
          type="button"
          disabled={movement.isPending}
          onClick={() => book("consume")}
          aria-label={`Eins entnehmen: ${product.name}`}
          className="size-11 rounded-lg border border-stone-300 bg-white text-xl disabled:opacity-40"
        >
          −
        </button>
        <button
          type="button"
          disabled={movement.isPending}
          onClick={() => book("add")}
          aria-label={`Eins einlagern: ${product.name}`}
          className="size-11 rounded-lg border border-stone-300 bg-white text-xl disabled:opacity-40"
        >
          +
        </button>
      </div>
    </li>
  );
}

function ProductImage({ product }: { product: Product }) {
  if (product.has_image) {
    return (
      <img
        src={`/api/v1/products/${encodeURIComponent(product.id)}/image`}
        loading="lazy"
        alt=""
        className="size-10 shrink-0 rounded-lg bg-stone-100 object-cover"
      />
    );
  }
  return (
    <div
      aria-hidden="true"
      className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-stone-100"
    >
      <div className="size-5 rounded-md border-2 border-stone-300" />
    </div>
  );
}
