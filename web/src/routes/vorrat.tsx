import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router";
import { problemCode } from "../lib/api/client";
import {
  markProductMutation,
  productListQuery,
  productMovementMutation,
  unmarkMutation,
  type ProductMovement,
} from "../lib/api/queries";
import { cartToggle } from "../lib/cartToggle";
import { productImageUrl } from "../lib/productImage";
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

// How long the notice of a failed booking or mark stays visible.
const noticeDuration = 2000;

// The buttons of a row: the tap area is 44 × 44 px, the visible face only
// 40 × 40 px, so the three buttons leave more room for the name.
const rowButtonClass = "flex size-11 items-center justify-center disabled:opacity-40";
const rowButtonFaceClass = "flex size-10 items-center justify-center rounded-lg border";

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
  const mark = useMutation(markProductMutation(queryClient));
  const unmark = useMutation(unmarkMutation(queryClient));
  useResetAfterError(movement);
  useResetAfterError(mark);
  useResetAfterError(unmark);

  let notice = "";
  if (movement.isError) {
    notice =
      problemCode(movement.error) === "stock_already_zero"
        ? "War schon leer"
        : "Buchung fehlgeschlagen";
  } else if (mark.isError) {
    notice = "Vormerken fehlgeschlagen";
  } else if (unmark.isError) {
    notice = "Entfernen fehlgeschlagen";
  }

  function book(kind: ProductMovement["kind"]) {
    movement.mutate({ productId: product.id, kind });
  }

  const cart = cartToggle(product);

  function toggleCart() {
    if (cart.action === "mark") {
      mark.mutate(product.id);
    } else {
      unmark.mutate(product.id);
    }
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
      <div className="relative z-10 flex shrink-0">
        <button
          type="button"
          disabled={mark.isPending || unmark.isPending}
          onClick={toggleCart}
          aria-pressed={cart.pressed}
          aria-label={cart.label}
          className={rowButtonClass}
        >
          <span
            className={`${rowButtonFaceClass} ${cart.pressed ? "border-amber-700 bg-amber-700 text-white" : "border-stone-300 bg-white text-stone-700"}`}
          >
            <CartIcon checked={cart.pressed} />
          </span>
        </button>
        <button
          type="button"
          disabled={movement.isPending}
          onClick={() => book("consume")}
          aria-label={`Eins entnehmen: ${product.name}`}
          className={rowButtonClass}
        >
          <span className={`${rowButtonFaceClass} border-stone-300 bg-white text-xl`}>
            −
          </span>
        </button>
        <button
          type="button"
          disabled={movement.isPending}
          onClick={() => book("add")}
          aria-label={`Eins einlagern: ${product.name}`}
          className={rowButtonClass}
        >
          <span className={`${rowButtonFaceClass} border-stone-300 bg-white text-xl`}>
            +
          </span>
        </button>
      </div>
    </li>
  );
}

// Resets a failed mutation of a row after noticeDuration, which hides its
// notice.
function useResetAfterError({ isError, reset }: { isError: boolean; reset: () => void }) {
  useEffect(() => {
    if (!isError) {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [isError, reset]);
}

// The shopping cart of the cart button, drawn with the color of the text
// like the symbols of the navigation bar; checked, with a tick in the basket.
function CartIcon({ checked }: { checked: boolean }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      strokeLinejoin="round"
      className="size-6"
    >
      <path d="M2 3h2.5l2.6 12.2a1 1 0 0 0 1 .8h9.4a1 1 0 0 0 1-.76L21 7H5.6" />
      <circle cx="9" cy="20" r="1.5" />
      <circle cx="18" cy="20" r="1.5" />
      {checked && <path d="m10 11.5 2 2 4-4" />}
    </svg>
  );
}

function ProductImage({ product }: { product: Product }) {
  const imageUrl = productImageUrl(product);
  if (imageUrl !== null) {
    return (
      <img
        src={imageUrl}
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
