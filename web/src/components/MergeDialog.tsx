import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { productListQuery, productMergeMutation } from "../lib/api/queries";
import { mergeCandidates, type Product } from "../lib/products";
import { Dialog } from "./Dialog";

const buttonClass = "min-h-11 rounded-lg px-4 font-medium disabled:opacity-40";
const cancelClass = `border border-stone-300 bg-white text-stone-700 ${buttonClass}`;

interface MergeDialogProps {
  /** The product to merge; the dialog is open while it is not null. */
  source: Product | null;
  /** Called when the user closes the dialog without merging. */
  onClose: () => void;
  /** Called after the merge with the target from the response and source. */
  onMerged: (target: Product, source: Product) => void;
}

/**
 * The dialog that merges source into another product: the user picks the
 * target from a list with search, confirms, and source is merged into it
 * with POST /products/{id}/merge.
 */
export function MergeDialog({ source, onClose, onMerged }: MergeDialogProps) {
  const queryClient = useQueryClient();
  const merge = useMutation(productMergeMutation(queryClient));
  // The picked target; while it is null the dialog shows the list.
  const [target, setTarget] = useState<Product | null>(null);

  function close() {
    setTarget(null);
    merge.reset();
    onClose();
  }

  function confirm(from: Product, picked: Product) {
    merge.mutate(
      { sourceId: from.id, targetId: picked.id },
      {
        onSuccess: (merged) => {
          setTarget(null);
          onMerged(merged, from);
        },
      },
    );
  }

  return (
    <Dialog
      open={source !== null}
      onClose={close}
      title={target === null ? "Zielprodukt wählen" : "Produkte zusammenführen"}
    >
      {source !== null && target === null && (
        <MergePicker ownId={source.id} onPick={setTarget} onCancel={close} />
      )}
      {source !== null && target !== null && (
        <>
          <p className="mt-2 text-stone-700">
            „{source.name}“ in „{target.name}“ zusammenführen? Barcodes,
            Verlauf und Bestand gehen auf „{target.name}“ über, „
            {source.name}“ wird gelöscht.
          </p>
          <p role="status" className="mt-2 text-sm font-medium text-red-700">
            {merge.isError ? "Zusammenführen fehlgeschlagen" : ""}
          </p>
          <div className="mt-4 flex justify-end gap-3">
            <button type="button" onClick={close} className={cancelClass}>
              Abbrechen
            </button>
            <button
              type="button"
              disabled={merge.isPending}
              onClick={() => confirm(source, target)}
              className={`bg-red-600 text-white ${buttonClass}`}
            >
              Zusammenführen
            </button>
          </div>
        </>
      )}
    </Dialog>
  );
}

// The search and the list of the products the product with ownId can be
// merged into. It loads the product list only while it is shown.
function MergePicker({
  ownId,
  onPick,
  onCancel,
}: {
  ownId: string;
  onPick: (product: Product) => void;
  onCancel: () => void;
}) {
  const products = useQuery(productListQuery);
  const [query, setQuery] = useState("");

  let content = <p className="mt-3 text-stone-500">Produkte werden geladen …</p>;
  if (products.data !== undefined) {
    const candidates = mergeCandidates(products.data, ownId, query);
    content =
      candidates.length === 0 ? (
        <p className="mt-3 text-stone-500">Keine Treffer.</p>
      ) : (
        <ul className="mt-3 max-h-[50dvh] divide-y divide-stone-200 overflow-y-auto rounded-xl border border-stone-200">
          {candidates.map((candidate) => (
            <li key={candidate.id}>
              <button
                type="button"
                onClick={() => onPick(candidate)}
                className="min-h-11 w-full px-3 py-2 text-left"
              >
                <span className="block font-medium break-words hyphens-auto">
                  {candidate.name}
                </span>
                {candidate.brand !== null && (
                  <span className="block text-sm break-words text-stone-500">
                    {candidate.brand}
                  </span>
                )}
              </button>
            </li>
          ))}
        </ul>
      );
  } else if (products.isError && !products.isFetching) {
    content = (
      <p className="mt-3 text-stone-700">
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
        aria-label="Zielprodukt suchen"
        className="mt-3 min-h-11 w-full rounded-lg border border-stone-300 bg-white px-3 text-base"
      />
      {content}
      <div className="mt-4 flex justify-end">
        <button type="button" onClick={onCancel} className={cancelClass}>
          Abbrechen
        </button>
      </div>
    </>
  );
}
