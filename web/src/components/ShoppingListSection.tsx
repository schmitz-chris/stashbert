import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { markProductMutation, unmarkMutation } from "../lib/api/queries";
import type { Product } from "../lib/products";

// How long the message of a mark or its removal stays visible. A failure
// stays until the next action (ADR-0016).
const noticeDuration = 2000;

// The message after a mark or its removal; failed ones are shown in red.
interface Notice {
  text: string;
  failed: boolean;
}

const buttonClass = "pressable min-h-11 rounded-lg px-4 font-medium disabled:opacity-40";

/**
 * Shows whether product is marked for shopping (ADR-0015): marked, the
 * hint "vorgemerkt" and "Von der Liste nehmen", which removes the mark;
 * otherwise "Vormerken", which marks it by product_id.
 */
export function ShoppingListSection({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const mark = useMutation(markProductMutation(queryClient));
  const unmark = useMutation(unmarkMutation(queryClient));
  const [notice, setNotice] = useState<Notice | null>(null);
  const headingId = useId();

  // Hides the message of a success after a short time.
  useEffect(() => {
    if (notice === null || notice.failed) {
      return;
    }
    const timer = setTimeout(() => setNotice(null), noticeDuration);
    return () => clearTimeout(timer);
  }, [notice]);

  function markProduct() {
    setNotice(null);
    mark.mutate(product.id, {
      onSuccess: (result) => setNotice({ text: result.message, failed: false }),
      onError: () => setNotice({ text: "Vormerken fehlgeschlagen", failed: true }),
    });
  }

  function unmarkProduct() {
    setNotice(null);
    unmark.mutate(product.id, {
      onSuccess: () => setNotice({ text: "Nicht mehr vorgemerkt", failed: false }),
      onError: () => setNotice({ text: "Entfernen fehlgeschlagen", failed: true }),
    });
  }

  const pending = mark.isPending || unmark.isPending;

  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="text-lg font-semibold">
        Einkaufsliste
      </h2>
      {product.marked ? (
        <div className="mt-2 flex flex-wrap items-center gap-3">
          <p className="font-medium text-marked">vorgemerkt</p>
          <button
            type="button"
            disabled={pending}
            onClick={unmarkProduct}
            className={`border border-line-strong bg-surface text-ink-secondary ${buttonClass}`}
          >
            Von der Liste nehmen
          </button>
        </div>
      ) : (
        <button
          type="button"
          disabled={pending}
          onClick={markProduct}
          className={`mt-2 bg-marked text-white ${buttonClass}`}
        >
          Vormerken
        </button>
      )}
      <p
        role="status"
        className={`mt-1 text-sm font-medium ${notice?.failed ? "text-danger" : "text-marked"}`}
      >
        {notice?.text}
      </p>
    </section>
  );
}
