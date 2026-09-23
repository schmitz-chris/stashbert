import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId } from "react";
import { Link } from "react-router";
import { productUpdateMutation } from "../lib/api/queries";
import type { Product } from "../lib/products";
import { cardView, targetChoices, type CardBooking } from "../lib/resultCard";

// How long the message of a target change stays visible.
const noticeDuration = 2000;

const actionClass =
  "inline-flex min-h-11 items-center rounded-lg border border-stone-300 bg-white px-3 font-medium text-stone-800";

interface ResultCardProps {
  booking: CardBooking;
  /** Locks both buttons, while an action of the card runs. */
  disabled: boolean;
  onPlusOne: () => void;
  onUndo: () => void;
  /** Called with the product from the response after its target changed. */
  onProductChange: (product: Product) => void;
  /** Called by "Stattdessen zu vorhandenem Produkt". */
  onMerge: () => void;
}

/**
 * The result card of the scan view: the product of the booking, its
 * stock before and after, and the buttons [+1] and [Rückgängig]. For a
 * booking that created its product it also offers the target chips, a
 * link to the product page and the merge into another product.
 */
export function ResultCard({
  booking,
  disabled,
  onPlusOne,
  onUndo,
  onProductChange,
  onMerge,
}: ResultCardProps) {
  const nameId = useId();
  const view = cardView(booking);
  const plusOneClass = booking.kind === "add" ? "bg-emerald-600" : "bg-sky-600";

  return (
    <section aria-labelledby={nameId} className="rounded-xl bg-white p-3 shadow">
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">
          <h2 id={nameId} className="truncate font-semibold">
            {view.title}
          </h2>
          {view.review && (
            <p className="text-sm font-medium text-amber-700">Bitte prüfen</p>
          )}
          <p className="text-xl font-bold tabular-nums">{view.stock}</p>
        </div>
        <button
          type="button"
          disabled={disabled}
          onClick={onPlusOne}
          className={`min-h-11 min-w-14 shrink-0 rounded-lg px-3 text-lg font-semibold text-white disabled:opacity-40 ${plusOneClass}`}
        >
          +1
        </button>
        <button
          type="button"
          disabled={disabled}
          onClick={onUndo}
          className="min-h-11 shrink-0 rounded-lg bg-stone-200 px-3 font-medium text-stone-800 disabled:opacity-40"
        >
          Rückgängig
        </button>
      </div>
      {view.isNew && (
        <NewProductActions
          product={booking.result.product}
          onProductChange={onProductChange}
          onMerge={onMerge}
        />
      )}
    </section>
  );
}

// The actions for a new product: the target chips, which send a PATCH
// with only the target, "Name ändern" and the merge.
function NewProductActions({
  product,
  onProductChange,
  onMerge,
}: {
  product: Product;
  onProductChange: (product: Product) => void;
  onMerge: () => void;
}) {
  const queryClient = useQueryClient();
  const update = useMutation(productUpdateMutation(queryClient));

  // Hides the message of a target change after a short time.
  const { status, reset } = update;
  useEffect(() => {
    if (status !== "success" && status !== "error") {
      return;
    }
    const timer = setTimeout(reset, noticeDuration);
    return () => clearTimeout(timer);
  }, [status, reset]);

  let notice = "";
  if (status === "success") {
    notice = "Soll gespeichert";
  } else if (status === "error") {
    notice = "Soll nicht gespeichert";
  }

  function chooseTarget(target: number) {
    update.mutate(
      { id: product.id, patch: { target } },
      { onSuccess: onProductChange },
    );
  }

  return (
    <div className="mt-3 border-t border-stone-200 pt-3">
      <div role="group" aria-label="Soll" className="flex items-center gap-2">
        <span className="mr-1 text-sm font-medium text-stone-700">Soll</span>
        {targetChoices.map((value) => {
          const pressed = product.target === value;
          return (
            <button
              key={value}
              type="button"
              aria-pressed={pressed}
              disabled={update.isPending}
              onClick={() => chooseTarget(value)}
              className={`size-11 shrink-0 rounded-lg text-lg font-semibold tabular-nums disabled:opacity-40 ${pressed ? "bg-emerald-600 text-white" : "bg-stone-200 text-stone-800"}`}
            >
              {value}
            </button>
          );
        })}
      </div>
      <p
        role="status"
        className={`mt-1 min-h-5 text-sm font-medium ${status === "error" ? "text-red-700" : "text-emerald-700"}`}
      >
        {notice}
      </p>
      <div className="mt-1 flex flex-wrap gap-2">
        <Link to={`/produkt/${encodeURIComponent(product.id)}`} className={actionClass}>
          Name ändern
        </Link>
        <button type="button" onClick={onMerge} className={actionClass}>
          Stattdessen zu vorhandenem Produkt
        </button>
      </div>
    </div>
  );
}
