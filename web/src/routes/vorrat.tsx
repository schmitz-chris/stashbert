import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { Link } from "react-router";
import { CartIcon } from "../components/CartIcon";
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

// The buttons of a row, above the link of the row: 44 × 44 px, growing with
// the text size up to 48 px, so the stepper still fits into the row of a
// 320 px wide screen at 200 % text size.
const rowButtonClass =
  "pressable relative z-10 flex size-[min(2.75rem,48px)] shrink-0 items-center justify-center disabled:opacity-40";

export function StockPage() {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<ProductFilter>("all");
  const searchRef = useRef<HTMLInputElement>(null);
  const products = useQuery(productListQuery);

  // Empties the search and leaves the focus in the field, because the
  // button disappears with the text.
  function clearSearch() {
    setQuery("");
    searchRef.current?.focus();
  }

  return (
    <>
      <h1 className="text-2xl font-semibold">Vorrat</h1>
      {/* Search and filters stay at the top while the list scrolls, below the
          safe area. They lie above the buttons of the rows (z-10) and below
          the update banner and the navigation bar (z-20). Their minimum
          height grows only up to 48 px, so with a large text size they take
          up less than a third of the screen. */}
      <div className="sticky top-[env(safe-area-inset-top)] z-15 bg-canvas pt-4 pb-3">
        <div className="relative">
          <SearchIcon />
          <input
            ref={searchRef}
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Name oder Marke suchen"
            aria-label="Vorrat durchsuchen"
            className="min-h-[min(2.75rem,48px)] w-full rounded-lg border border-line-strong bg-surface pr-11 pl-10 text-base [&::-webkit-search-cancel-button]:appearance-none"
          />
          {query !== "" && (
            <button
              type="button"
              onClick={clearSearch}
              aria-label="Suche löschen"
              className="pressable absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-lg text-ink-tertiary"
            >
              <ClearIcon />
            </button>
          )}
        </div>
        {/* One row, which scrolls sideways when the filters do not fit (large
            text size, narrow screen), so the sticky header stays low. */}
        <div
          role="radiogroup"
          aria-label="Filter"
          className="-mx-4 mt-3 flex gap-2 overflow-x-auto px-4"
        >
          {filters.map(({ value, label }) => (
            <button
              key={value}
              type="button"
              role="radio"
              aria-checked={filter === value}
              onClick={() => setFilter(value)}
              className="pressable min-h-[min(2.75rem,48px)] shrink-0 rounded-full border border-line-strong bg-surface px-3 font-medium text-ink-secondary aria-checked:border-accent aria-checked:bg-accent aria-checked:text-white"
            >
              {label}
            </button>
          ))}
        </div>
      </div>
      <div className="mt-1">
        {products.data === undefined ? (
          products.isError && !products.isFetching ? (
            <div>
              <p className="text-ink-secondary">
                Der Vorrat konnte nicht geladen werden.
              </p>
              <button
                type="button"
                onClick={() => void products.refetch()}
                className="pressable mt-3 min-h-11 rounded-lg bg-accent px-4 font-medium text-white"
              >
                Erneut versuchen
              </button>
            </div>
          ) : (
            <p className="text-ink-tertiary">Vorrat wird geladen …</p>
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
      <p className="text-ink-tertiary">
        Noch keine Produkte. Scanne einen Barcode, um eines einzulagern.
      </p>
    );
  }
  if (products.length === 0) {
    return <p className="text-ink-tertiary">Keine Treffer.</p>;
  }
  return (
    <ul className="divide-y divide-line overflow-hidden rounded-xl border border-line bg-surface">
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

  // The notice of a failure has no time limit (ADR-0016): it stays until
  // the next action in the row.
  function clearNotice() {
    for (const mutation of [movement, mark, unmark]) {
      if (mutation.isError) {
        mutation.reset();
      }
    }
  }

  function book(kind: ProductMovement["kind"]) {
    clearNotice();
    movement.mutate({ productId: product.id, kind });
  }

  const cart = cartToggle(product);

  function toggleCart() {
    clearNotice();
    if (cart.action === "mark") {
      mark.mutate(product.id);
    } else {
      unmark.mutate(product.id);
    }
  }

  // Two lines: image, name and brand over the full width, below them on
  // the right the cart and, set apart by 16 px, the stepper [−] stock [+].
  // The link covers the whole row with its ::after box, so a tap anywhere
  // outside the buttons opens the product. The buttons lie above it.
  return (
    <li className="relative px-3 py-2">
      <div className="flex items-center gap-3">
        <ProductImage product={product} />
        <div className="min-w-0 flex-1">
          <Link
            to={`/produkt/${encodeURIComponent(product.id)}`}
            className="pressable-row font-medium break-words hyphens-auto after:absolute after:inset-0"
          >
            {product.name}
          </Link>
          {product.brand !== null && (
            <p className="text-sm break-words hyphens-auto text-ink-tertiary">{product.brand}</p>
          )}
          <p role="status" className="text-sm font-medium text-danger">
            {notice}
          </p>
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-4">
        <button
          type="button"
          disabled={mark.isPending || unmark.isPending}
          onClick={toggleCart}
          aria-pressed={cart.pressed}
          aria-label={cart.label}
          className={`${rowButtonClass} rounded-lg border ${cart.pressed ? "border-marked bg-marked text-white" : "border-line-strong bg-surface text-ink-secondary"}`}
        >
          <CartIcon checked={cart.pressed} />
        </button>
        {/* The stepper in one frame: the stock large, below it the target
            (without a target only the stock), between the buttons. */}
        <div className="flex items-center rounded-lg ring-1 ring-line-strong ring-inset">
          <button
            type="button"
            disabled={movement.isPending}
            onClick={() => book("consume")}
            aria-label={`Eins entnehmen: ${product.name}`}
            className={`${rowButtonClass} rounded-l-lg text-xl leading-none`}
          >
            −
          </button>
          <div className="min-w-12 px-1 text-center tabular-nums">
            <p className="text-xl leading-6 font-semibold text-ink">
              <span className="sr-only">Bestand </span>
              {product.stock}
            </p>
            {product.target > 0 && (
              <p className="text-xs leading-4 text-ink-tertiary">Soll {product.target}</p>
            )}
          </div>
          <button
            type="button"
            disabled={movement.isPending}
            onClick={() => book("add")}
            aria-label={`Eins einlagern: ${product.name}`}
            className={`${rowButtonClass} rounded-r-lg text-xl leading-none`}
          >
            +
          </button>
        </div>
      </div>
    </li>
  );
}

// The magnifier at the start of the search field.
function SearchIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2}
      strokeLinecap="round"
      className="pointer-events-none absolute top-1/2 left-3 size-5 -translate-y-1/2 text-ink-tertiary"
    >
      <circle cx="10.5" cy="10.5" r="6.5" />
      <path d="m15.5 15.5 5 5" />
    </svg>
  );
}

// The cross in a filled circle of the button that empties the search.
function ClearIcon() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="size-5">
      <circle cx="12" cy="12" r="10" fill="currentColor" />
      <path
        d="m8.5 8.5 7 7m0-7-7 7"
        fill="none"
        stroke="white"
        strokeWidth={2}
        strokeLinecap="round"
      />
    </svg>
  );
}

// The picture of a row: 40 px, growing with the text size only up to 48 px,
// so the name keeps the width.
const imageClass = "size-[min(2.5rem,48px)] shrink-0 rounded-lg";

function ProductImage({ product }: { product: Product }) {
  const imageUrl = productImageUrl(product);
  if (imageUrl !== null) {
    return (
      <img
        src={imageUrl}
        loading="lazy"
        alt=""
        className={`${imageClass} bg-fill object-cover`}
      />
    );
  }
  return (
    <div
      aria-hidden="true"
      className={`${imageClass} flex items-center justify-center bg-fill`}
    >
      <div className="size-1/2 rounded-md border-2 border-ink-tertiary" />
    </div>
  );
}
