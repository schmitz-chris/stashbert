import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { productListQuery, productMergeMutation } from "../lib/api/queries";
import { mergeCandidates, type Product } from "../lib/products";
import { ConfirmActions, Dialog, Sheet } from "./Dialog";

interface MergeDialogProps {
  /** The product to merge; the dialog is open while it is not null. */
  source: Product | null;
  /** Called when the user closes the dialog without merging. */
  onClose: () => void;
  /** Called after the merge with the target from the response and source. */
  onMerged: (target: Product, source: Product) => void;
}

/**
 * Merges source into another product: the user picks the target in a
 * sheet with search and list, confirms in a dialog above the sheet, and
 * source is merged into it with POST /products/{id}/merge. "Abbrechen" in
 * the confirmation goes back to the list.
 */
export function MergeDialog({ source, onClose, onMerged }: MergeDialogProps) {
  const queryClient = useQueryClient();
  const merge = useMutation(productMergeMutation(queryClient));
  // The picked target; while it is not null the confirmation is shown.
  const [target, setTarget] = useState<Product | null>(null);

  function close() {
    setTarget(null);
    merge.reset();
    onClose();
  }

  function cancelConfirm() {
    setTarget(null);
    merge.reset();
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
    <>
      <Sheet open={source !== null} onClose={close} title="Zielprodukt wählen">
        {source !== null && <MergePicker ownId={source.id} onPick={setTarget} />}
      </Sheet>
      <Dialog
        open={source !== null && target !== null}
        onClose={cancelConfirm}
        title="Produkte zusammenführen"
      >
        {source !== null && target !== null && (
          <>
            <p className="mt-2 text-ink-secondary">
              „{source.name}“ in „{target.name}“ zusammenführen? Barcodes,
              Verlauf und Bestand gehen auf „{target.name}“ über, „
              {source.name}“ wird gelöscht.
            </p>
            <p role="status" className="mt-2 text-sm font-medium text-danger">
              {merge.isError ? "Zusammenführen fehlgeschlagen" : ""}
            </p>
            <ConfirmActions
              label="Zusammenführen"
              pending={merge.isPending}
              onCancel={cancelConfirm}
              onConfirm={() => confirm(source, target)}
            />
          </>
        )}
      </Dialog>
    </>
  );
}

// The search and the list of the products the product with ownId can be
// merged into. It loads the product list only while it is shown.
function MergePicker({
  ownId,
  onPick,
}: {
  ownId: string;
  onPick: (product: Product) => void;
}) {
  const products = useQuery(productListQuery);
  const [query, setQuery] = useState("");

  let content = <p className="mt-3 text-ink-tertiary">Produkte werden geladen …</p>;
  if (products.data !== undefined) {
    const candidates = mergeCandidates(products.data, ownId, query);
    content =
      candidates.length === 0 ? (
        <p className="mt-3 text-ink-tertiary">Keine Treffer.</p>
      ) : (
        <ul className="mt-3 max-h-[50dvh] divide-y divide-line overflow-y-auto rounded-xl border border-line">
          {candidates.map((candidate) => (
            <li key={candidate.id}>
              <button
                type="button"
                onClick={() => onPick(candidate)}
                className="pressable min-h-11 w-full px-3 py-2 text-left"
              >
                <span className="block font-medium break-words hyphens-auto">
                  {candidate.name}
                </span>
                {candidate.brand !== null && (
                  <span className="block text-sm break-words text-ink-tertiary">
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
        aria-label="Zielprodukt suchen"
        className="mt-3 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-3 text-base"
      />
      {content}
    </>
  );
}
