import { useState } from "react";
import { useLocation, useNavigate } from "react-router";
import type { Product } from "../lib/products";
import { MergeDialog } from "./MergeDialog";

/**
 * The button "Mit vorhandenem Produkt zusammenführen" with its dialog: the
 * user picks another product, confirms, and product is merged into it.
 * Then the page of the target opens in place of this one; it keeps the
 * navigation state, so its back link names the same view.
 */
export function MergeSection({ product }: { product: Product }) {
  const navigate = useNavigate();
  const { state } = useLocation();
  const [open, setOpen] = useState(false);

  function merged(target: Product) {
    setOpen(false);
    void navigate(`/produkt/${encodeURIComponent(target.id)}`, { replace: true, state });
  }

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="pressable min-h-11 rounded-lg border border-line-strong bg-surface px-4 font-medium text-accent"
      >
        Mit vorhandenem Produkt zusammenführen
      </button>
      <MergeDialog
        source={open ? product : null}
        onClose={() => setOpen(false)}
        onMerged={merged}
      />
    </>
  );
}
