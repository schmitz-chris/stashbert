import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useState } from "react";
import { inventoryMutation } from "../lib/api/queries";
import type { Product } from "../lib/products";

// How long the message of a stock count stays visible.
const noticeDuration = 2000;

/**
 * Shows the stock of product and sets it to a counted value with a
 * movement of kind inventory.
 */
export function StockSection({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const inventory = useMutation(inventoryMutation(queryClient));
  const [stock, setStock] = useState("");
  const headingId = useId();
  const inputId = useId();

  // Hides the message of a successful stock count after a short time.
  const { isSuccess, reset } = inventory;
  useEffect(() => {
    if (!isSuccess) {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [isSuccess, reset]);

  let notice = inventory.data?.message ?? "";
  if (inventory.isError) {
    notice = "Bestand setzen fehlgeschlagen";
  }

  return (
    <section aria-labelledby={headingId} className="mt-8">
      <h2 id={headingId} className="text-lg font-semibold">
        Bestand
      </h2>
      <p className="mt-2 text-ink-secondary">
        Aktuell: <span className="font-semibold tabular-nums">{product.stock}</span>
      </p>
      <form
        // The number input checks that the stock is a whole number from 0
        // to 100000; the form is only submitted if it is. An empty field
        // gets the hint of the browser instead of a disabled button.
        onSubmit={(event) => {
          event.preventDefault();
          inventory.mutate(
            { productId: product.id, stock: Number(stock) },
            { onSuccess: () => setStock("") },
          );
        }}
        className="mt-4"
      >
        <label htmlFor={inputId} className="block text-sm font-medium text-ink-secondary">
          Neuer Bestand
        </label>
        {/* The button moves below the field when the text is large. */}
        <div className="mt-1 flex flex-wrap gap-2">
          <input
            id={inputId}
            type="number"
            inputMode="numeric"
            required
            min={0}
            max={100000}
            step={1}
            value={stock}
            onChange={(event) => {
              setStock(event.target.value);
              if (inventory.isError) {
                inventory.reset();
              }
            }}
            className="min-h-11 min-w-[8rem] flex-1 rounded-lg border border-line-strong bg-surface px-3 py-2 text-base"
          />
          <button
            type="submit"
            disabled={inventory.isPending}
            className="pressable min-h-11 shrink-0 rounded-lg border border-line-strong bg-surface px-4 font-medium text-accent disabled:opacity-40"
          >
            Bestand setzen
          </button>
        </div>
        <p
          role="status"
          className={`mt-1 text-sm font-medium ${inventory.isError ? "text-danger" : "text-accent"}`}
        >
          {notice}
        </p>
      </form>
    </section>
  );
}
