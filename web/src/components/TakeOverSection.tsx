import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import {
  productListQuery,
  productMergeMutation,
  refetchAfterMerges,
} from "../lib/api/queries";
import type { Product } from "../lib/products";
import {
  mergeOneByOne,
  selectedProducts,
  takeOverButtonLabel,
  takeOverCandidates,
  takeOverExplanation,
  takeOverNotice,
  takeOverQuestion,
  toggleSelected,
  type TakeOverNotice,
} from "../lib/takeOver";
import { ConfirmActions, Dialog, Sheet } from "./Dialog";

// How long the message of a success stays visible. A failure stays until
// the next action (ADR-0016).
const noticeDuration = 2000;

/**
 * "Andere Produkte hierher übernehmen" (docs/plan.md, F36): the user picks
 * several other products in a sheet with search and list, confirms in a
 * dialog above the sheet, and each is merged into product with
 * POST /products/{id}/merge, one after another. At the first error it
 * stops; the message below the button says which products were taken
 * over. The opposite direction is MergeSection.
 */
export function TakeOverSection({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const merge = useMutation(productMergeMutation(queryClient));
  const [open, setOpen] = useState(false);
  // The ids of the chosen products, in the order they were tapped.
  const [selected, setSelected] = useState<string[]>([]);
  // The products the confirmation asks about; it is shown while this is
  // not null and the sheet is open. They are fixed when it opens, so the
  // question stays the same while the merges shrink the list.
  const [confirming, setConfirming] = useState<Product[] | null>(null);
  const [running, setRunning] = useState(false);
  const [notice, setNotice] = useState<TakeOverNotice | null>(null);
  // The product list is loaded only while the sheet is open.
  const products = useQuery({ ...productListQuery, enabled: open });
  const chosen = selectedProducts(products.data ?? [], product.id, selected);

  // Hides the message of a success after a short time.
  useEffect(() => {
    if (notice === null || notice.failed) {
      return;
    }
    const timer = setTimeout(() => setNotice(null), noticeDuration);
    return () => clearTimeout(timer);
  }, [notice]);

  function start() {
    setNotice(null);
    setSelected([]);
    setConfirming(null);
    setOpen(true);
  }

  async function takeOver(sources: Product[]) {
    setRunning(true);
    const outcome = await mergeOneByOne(sources, (source) =>
      merge.mutateAsync({ sourceId: source.id, targetId: product.id }),
    );
    // Each merge updated the caches; after an error it is unclear what
    // the server holds.
    if (outcome.failed !== null) {
      refetchAfterMerges(queryClient);
    }
    // Closing the sheet closes the confirmation too; confirming stays, so
    // its question does not change while it goes.
    setRunning(false);
    setOpen(false);
    setNotice(takeOverNotice(outcome));
  }

  return (
    <div className="mt-1 mb-4">
      <button
        type="button"
        onClick={start}
        className="pressable min-h-11 w-full rounded-lg border border-line-strong bg-surface px-4 py-2 font-medium text-accent"
      >
        Andere Produkte hierher übernehmen
      </button>
      <p
        role="status"
        className={`mt-1 text-sm font-medium ${notice?.failed ? "text-danger" : "text-accent"}`}
      >
        {notice?.text}
      </p>
      <Sheet open={open} onClose={() => setOpen(false)} title="Produkte wählen">
        {open && (
          <TakeOverPicker
            products={products.data}
            loadFailed={products.isError && !products.isFetching}
            ownId={product.id}
            selected={selected}
            onToggle={(id) => setSelected((current) => toggleSelected(current, id))}
          />
        )}
        <button
          type="button"
          disabled={chosen.length === 0}
          onClick={() => setConfirming(chosen)}
          className="pressable mt-4 min-h-11 w-full rounded-lg bg-accent px-4 py-2 font-medium text-white disabled:opacity-40"
        >
          {takeOverButtonLabel(chosen.length)}
        </button>
      </Sheet>
      <Dialog
        open={open && confirming !== null}
        onClose={() => setConfirming(null)}
        title={takeOverQuestion(confirming?.length ?? 0, product.name)}
      >
        <p className="mt-2 text-ink-secondary">
          {takeOverExplanation(confirming?.length ?? 0)}
        </p>
        <ConfirmActions
          label="Übernehmen"
          pending={running}
          onCancel={() => setConfirming(null)}
          onConfirm={() => {
            if (confirming !== null && !running) {
              void takeOver(confirming);
            }
          }}
        />
      </Dialog>
    </div>
  );
}

// The search and the list of the products that can be taken over into the
// product with ownId, each with a checkbox.
function TakeOverPicker({
  products,
  loadFailed,
  ownId,
  selected,
  onToggle,
}: {
  products: Product[] | undefined;
  loadFailed: boolean;
  ownId: string;
  selected: readonly string[];
  onToggle: (id: string) => void;
}) {
  const [query, setQuery] = useState("");

  let content = <p className="mt-3 text-ink-tertiary">Produkte werden geladen …</p>;
  if (products !== undefined) {
    const candidates = takeOverCandidates(products, ownId, query, selected);
    content =
      candidates.length === 0 ? (
        <p className="mt-3 text-ink-tertiary">Keine Treffer.</p>
      ) : (
        <ul className="mt-3 max-h-[50dvh] divide-y divide-line overflow-y-auto rounded-xl border border-line">
          {candidates.map((candidate) => (
            <li key={candidate.id}>
              {/* The whole row, at least 44 px high, toggles the box. */}
              <label className="pressable flex min-h-11 cursor-pointer items-center gap-3 px-3 py-2">
                <input
                  type="checkbox"
                  checked={selected.includes(candidate.id)}
                  onChange={() => onToggle(candidate.id)}
                  className="size-5 shrink-0 accent-accent"
                />
                <span className="min-w-0 flex-1">
                  <span className="block font-medium break-words hyphens-auto">
                    {candidate.name}
                  </span>
                  {candidate.brand !== null && (
                    <span className="block text-sm break-words text-ink-tertiary">
                      {candidate.brand}
                    </span>
                  )}
                </span>
              </label>
            </li>
          ))}
        </ul>
      );
  } else if (loadFailed) {
    content = (
      <p className="mt-3 text-ink-secondary">
        Die Produkte konnten nicht geladen werden.
      </p>
    );
  }

  return (
    <>
      <input
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder="Name oder Marke suchen"
        aria-label="Produkte suchen"
        className="mt-3 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-3 text-base"
      />
      {content}
    </>
  );
}
