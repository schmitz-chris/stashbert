import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { CartButton, roundButtonClass } from "../components/CartButton";
import { PageHeading } from "../components/PageHeading";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import { problemCode } from "../lib/api/client";
import {
  markProductMutation,
  productListQuery,
  productMovementMutation,
  unmarkMutation,
  type ProductMovement,
} from "../lib/api/queries";
import { cartToggle } from "../lib/cartToggle";
import { pageTitle } from "../lib/pageTitle";
import { productImageUrl } from "../lib/productImage";
import { productLinkState } from "../lib/productOrigin";
import {
  brandAndSize,
  filterProducts,
  matches,
  parseProductFilter,
  stockSearchParams,
  type Product,
  type ProductFilter,
} from "../lib/products";
import { stockStatus, type StockLevel } from "../lib/stockStatus";

const filters: { value: ProductFilter; label: string }[] = [
  { value: "all", label: "Alle" },
  { value: "restock", label: "Nachkaufen" },
  { value: "empty", label: "Leer" },
  { value: "review", label: "Prüfen" },
];

export function StockPage() {
  // Filter and search live in the URL (replaced, not pushed), so going back
  // from a product page restores them. The search also has local state,
  // so typing never waits for the URL; when the URL changes from outside
  // (the tab bar opens /vorrat), the field follows it.
  const [searchParams, setSearchParams] = useSearchParams();
  const filter = parseProductFilter(searchParams.get("filter"));
  const urlQuery = searchParams.get("q") ?? "";
  const [query, setQueryState] = useState(urlQuery);
  const [shownUrlQuery, setShownUrlQuery] = useState(urlQuery);
  if (urlQuery !== shownUrlQuery) {
    setShownUrlQuery(urlQuery);
    setQueryState(urlQuery);
  }

  function setQuery(next: string) {
    setQueryState(next);
    setShownUrlQuery(next);
    setSearchParams(stockSearchParams(filter, next), { replace: true });
  }

  function setFilter(next: ProductFilter) {
    setSearchParams(stockSearchParams(next, query), { replace: true });
  }
  const searchRef = useRef<HTMLInputElement>(null);
  const products = useQuery(productListQuery);
  useDocumentTitle(pageTitle("stock"));

  // Empties the search and leaves the focus in the field, because the
  // button disappears with the text.
  function clearSearch() {
    setQuery("");
    searchRef.current?.focus();
  }

  return (
    <>
      {/* The title and, top right, the gear that opens the settings
          (docs/plan.md, F32). The gear is 44 px, and its symbol sits on
          the edge of the content. */}
      <div className="flex items-center justify-between gap-2">
        <PageHeading className="min-w-0 text-2xl font-semibold">Vorrat</PageHeading>
        <Link
          to="/einstellungen"
          aria-label="Einstellungen"
          className="pressable -mr-[10px] flex size-[44px] shrink-0 items-center justify-center rounded-full text-accent"
        >
          <GearIcon />
        </Link>
      </div>
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
            className="min-h-[min(2.75rem,48px)] w-full rounded-[10px] bg-fill pr-11 pl-10 text-base placeholder:text-ink-tertiary [&::-webkit-search-cancel-button]:appearance-none"
          />
          {query !== "" && (
            <button
              type="button"
              onClick={clearSearch}
              aria-label="Suche löschen"
              className="pressable absolute inset-y-0 right-0 flex w-11 items-center justify-center rounded-[10px] text-ink-tertiary"
            >
              <ClearIcon />
            </button>
          )}
        </div>
        {/* A segmented control: a gray track with the chosen segment on a
            white face. The track takes the full width and scrolls sideways
            when the filters do not fit (large text size, narrow screen), so
            the sticky header stays low. A transparent border of 2 px around
            each face belongs to its tap area of at least 44 px. */}
        <div
          role="radiogroup"
          aria-label="Filter"
          className="-mx-4 mt-3 overflow-x-auto px-4"
        >
          <div className="flex w-max min-w-full rounded-xl bg-fill">
            {filters.map(({ value, label }) => (
              <button
                key={value}
                type="button"
                role="radio"
                aria-checked={filter === value}
                onClick={() => setFilter(value)}
                className="pressable min-h-[min(2.75rem,48px)] flex-1 shrink-0 rounded-xl border-2 border-transparent bg-clip-padding px-2 text-sm font-medium text-ink-secondary aria-checked:bg-surface aria-checked:text-ink"
              >
                {label}
              </button>
            ))}
          </div>
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
    <ul className="overflow-hidden rounded-xl bg-surface">
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

  const details = brandAndSize(product);

  // Image, then name, brand and package size, and below them, where the
  // text starts, the status (if any) on the left and the cart and the stepper
  // [−] stock [+] on the right, set apart by 12 px. The separator above a
  // row starts where the text starts, not below the image. The side
  // padding and the gap grow with the text only up to 20 and 16 px, so the
  // stepper fits beside the image on a 320 px wide screen at 200 % text
  // size. The link covers the whole row with its ::after box, so a tap
  // anywhere outside the buttons opens the product. The buttons lie above
  // it.
  return (
    <li className="group relative flex items-start gap-[min(0.75rem,16px)] pl-[min(1rem,20px)]">
      <ProductImage product={product} />
      <div className="min-w-0 flex-1 border-t border-line py-3 pr-[min(1rem,20px)] group-first:border-t-0">
        <Link
          to={`/produkt/${encodeURIComponent(product.id)}`}
          state={productLinkState("vorrat")}
          className="pressable-row font-medium break-words hyphens-auto after:absolute after:inset-0"
        >
          {product.name}
        </Link>
        {details !== null && (
          <p className="text-sm break-words hyphens-auto text-ink-tertiary">{details}</p>
        )}
        <p role="status" className="text-sm font-medium text-danger">
          {notice}
        </p>
        {/* The status, if there is one, is as wide as its text and the
            buttons stand right of it; if both do not fit side by side (large text or a
            narrow screen), the buttons move to a line of their own,
            still on the right. */}
        <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-2">
          <StatusText product={product} />
          <div className="ml-auto flex flex-wrap items-center justify-end gap-3">
            <CartButton
              checked={cart.pressed}
              pressed={cart.pressed}
              label={cart.label}
              disabled={mark.isPending || unmark.isPending}
              onClick={toggleCart}
            />
            {/* The stepper: one gray capsule with round ends for [−] and
                [+] and the stock between them, in a box 3 digits wide, so
                the buttons stay in place when it changes. */}
            <div className="flex items-center rounded-full bg-fill">
              <button
                type="button"
                disabled={movement.isPending}
                onClick={() => book("consume")}
                aria-label={`Eins entnehmen: ${product.name}`}
                className={`${roundButtonClass} text-ink-secondary`}
              >
                <StepSymbol plus={false} />
              </button>
              <p className="box-content min-w-[3ch] px-[min(0.25rem,4px)] text-center text-[min(1.125rem,24px)] leading-none font-semibold text-ink tabular-nums">
                <span className="sr-only">Bestand </span>
                {product.stock}
              </p>
              <button
                type="button"
                disabled={movement.isPending}
                onClick={() => book("add")}
                aria-label={`Eins einlagern: ${product.name}`}
                className={`${roundButtonClass} text-ink-secondary`}
              >
                <StepSymbol plus />
              </button>
            </div>
          </div>
        </div>
      </div>
    </li>
  );
}

// The color and weight of the text of each level of the status.
const statusTextClass: Record<StockLevel, string> = {
  empty: "font-medium text-danger",
  missing: "font-medium text-ink",
};

// The status of a row (lib/stockStatus.ts): "leer" with a cross or
// "fehlt N" with a warning triangle, and nothing if there is nothing to do
// (user feedback of 2026-09-24). The text says everything the color and
// the symbol say.
function StatusText({ product }: { product: Product }) {
  const status = stockStatus(product);
  if (status === null) {
    return null;
  }
  return (
    <p
      className={`flex min-w-0 flex-auto items-center gap-1 text-sm ${statusTextClass[status.level]}`}
    >
      {status.level === "empty" && <EmptySymbol />}
      {status.level === "missing" && <WarningSymbol />}
      {status.text}
    </p>
  );
}

// A cross in a circle, in the color of the text (danger).
function EmptySymbol() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.25}
      strokeLinecap="round"
      className="size-[1.15em] shrink-0"
    >
      <circle cx="12" cy="12" r="9.5" />
      <path d="m8.75 8.75 6.5 6.5m0-6.5-6.5 6.5" />
    </svg>
  );
}

// A warning triangle in the warning color with an exclamation mark in ink
// (the warning role carries ink).
function WarningSymbol() {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" className="size-[1.15em] shrink-0">
      <path
        d="M10.3 3.6a2 2 0 0 1 3.4 0l8.3 14.4a2 2 0 0 1-1.7 3H3.7a2 2 0 0 1-1.7-3z"
        className="fill-warning"
      />
      <path
        d="M12 8.5v5.5m0 3.25v.01"
        fill="none"
        strokeWidth={2.25}
        strokeLinecap="round"
        className="stroke-ink"
      />
    </svg>
  );
}

// The minus or plus of the stepper, in the color of the text.
function StepSymbol({ plus }: { plus: boolean }) {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={2.25}
      strokeLinecap="round"
      className="size-[min(1.25rem,24px)] shrink-0"
    >
      <path d={plus ? "M5 12h14M12 5v14" : "M5 12h14"} />
    </svg>
  );
}

// A gear with eight teeth, the symbol of the settings, in the color of the
// text.
function GearIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.75}
      strokeLinejoin="round"
      className="size-[24px]"
    >
      <path d="M10.36 4.89L10.62 2.2A9.9 9.9 0 0 1 13.38 2.2L13.64 4.89A7.3 7.3 0 0 1 15.87 5.81L17.96 4.09A9.9 9.9 0 0 1 19.91 6.04L18.19 8.13A7.3 7.3 0 0 1 19.11 10.36L21.8 10.62A9.9 9.9 0 0 1 21.8 13.38L19.11 13.64A7.3 7.3 0 0 1 18.19 15.87L19.91 17.96A9.9 9.9 0 0 1 17.96 19.91L15.87 18.19A7.3 7.3 0 0 1 13.64 19.11L13.38 21.8A9.9 9.9 0 0 1 10.62 21.8L10.36 19.11A7.3 7.3 0 0 1 8.13 18.19L6.04 19.91A9.9 9.9 0 0 1 4.09 17.96L5.81 15.87A7.3 7.3 0 0 1 4.89 13.64L2.2 13.38A9.9 9.9 0 0 1 2.2 10.62L4.89 10.36A7.3 7.3 0 0 1 5.81 8.13L4.09 6.04A9.9 9.9 0 0 1 6.04 4.09L8.13 5.81A7.3 7.3 0 0 1 10.36 4.89z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
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

// The picture of a row: 48 px (smaller only below a text size of 16 px),
// level with the name, so the name keeps the width at a large text size.
const imageClass = "mt-3 size-[min(3rem,48px)] shrink-0 rounded-[10px]";

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
