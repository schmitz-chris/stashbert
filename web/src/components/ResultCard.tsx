import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useId } from "react";
import { Link } from "react-router";
import { productUpdateMutation } from "../lib/api/queries";
import { productLinkState } from "../lib/productOrigin";
import type { Product } from "../lib/products";
import {
  cardView,
  markCardView,
  offersCrate,
  targetChoices,
  type CardBooking,
  type CardMark,
} from "../lib/resultCard";
import { CloseIcon } from "./CloseIcon";

// How long the message of a saved target stays visible. A failure stays
// until the next choice (ADR-0016).
const noticeDuration = 2000;

const actionClass =
  "pressable inline-flex min-h-11 items-center rounded-lg border border-line-strong bg-surface px-3 font-medium hyphens-auto text-ink";

const undoClass =
  "pressable min-h-11 shrink-0 rounded-lg bg-fill px-3 font-medium text-ink disabled:opacity-40";

interface ResultCardProps {
  booking: CardBooking;
  /** Locks the buttons of the booking, while an action of the card runs. */
  disabled: boolean;
  onPlusOne: () => void;
  onUndo: () => void;
  /** Called by "War ein Kasten" (docs/plan.md, F28). */
  onCrate: () => void;
  /** Called with the product from the response after its target changed. */
  onProductChange: (product: Product) => void;
  /** Called by "Stattdessen zu vorhandenem Produkt". */
  onMerge: () => void;
  /** Called by "Karte schließen". */
  onClose: () => void;
}

/**
 * The result card of the scan view: the product of the booking, its
 * stock before and after, and the buttons [+1] and [Rückgängig]. For one
 * bottle of a product without a crate size it offers "War ein Kasten"
 * (offersCrate). For a booking that created its product it also offers
 * the target chips, a link to the product page and the merge into another
 * product. The card has no time limit; [×] closes it.
 */
export function ResultCard({
  booking,
  disabled,
  onPlusOne,
  onUndo,
  onCrate,
  onProductChange,
  onMerge,
  onClose,
}: ResultCardProps) {
  const nameId = useId();
  const view = cardView(booking);
  const plusOneClass = booking.kind === "add" ? "bg-accent" : "bg-consume";

  return (
    <section aria-labelledby={nameId} className="rounded-xl bg-surface p-3 shadow">
      <CardTitle id={nameId} title={view.title} onClose={onClose} />
      {view.review && (
        <p className="w-fit rounded bg-warning px-1.5 text-sm font-medium text-ink">
          Bitte prüfen
        </p>
      )}
      {/* The buttons move below the stock when the text is large. */}
      <div className="flex flex-wrap items-center justify-end gap-2">
        <p className="mr-auto text-xl font-bold tabular-nums">{view.stock}</p>
        <button
          type="button"
          disabled={disabled}
          onClick={onPlusOne}
          className={`pressable min-h-11 min-w-14 shrink-0 rounded-lg px-3 text-lg font-semibold text-white disabled:opacity-40 ${plusOneClass}`}
        >
          +1
        </button>
        <button type="button" disabled={disabled} onClick={onUndo} className={undoClass}>
          Rückgängig
        </button>
      </div>
      {offersCrate(booking) && (
        <button
          type="button"
          disabled={disabled}
          onClick={onCrate}
          className={`mt-2 ${actionClass} disabled:opacity-40`}
        >
          War ein Kasten
        </button>
      )}
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

interface MarkResultCardProps {
  mark: CardMark;
  /** Locks [Rückgängig], while it runs. */
  disabled: boolean;
  onUndo: () => void;
  /** Called by "Karte schließen". */
  onClose: () => void;
}

/**
 * The result card of the scan view in mode mark (docs/plan.md, F15): the
 * product, "vorgemerkt" or "schon auf der Liste", and [Rückgängig] only if
 * this scan marked the product. The card has no time limit; [×] closes it.
 */
export function MarkResultCard({ mark, disabled, onUndo, onClose }: MarkResultCardProps) {
  const nameId = useId();
  const view = markCardView(mark);

  return (
    <section aria-labelledby={nameId} className="rounded-xl bg-surface p-3 shadow">
      <CardTitle id={nameId} title={view.title} onClose={onClose} />
      <div className="flex flex-wrap items-center justify-end gap-2">
        <p className="mr-auto text-xl font-bold text-marked">{view.status}</p>
        {view.undo && (
          <button type="button" disabled={disabled} onClick={onUndo} className={undoClass}>
            Rückgängig
          </button>
        )}
      </div>
    </section>
  );
}

// The name of the product on a card, in full, and [×], which closes the
// card.
function CardTitle({ id, title, onClose }: { id: string; title: string; onClose: () => void }) {
  return (
    <div className="-mt-1.5 -mr-1.5 flex items-start gap-2">
      <h2 id={id} className="min-w-0 flex-1 self-center font-semibold break-words hyphens-auto">
        {title}
      </h2>
      <button
        type="button"
        aria-label="Karte schließen"
        onClick={onClose}
        className="pressable flex size-11 shrink-0 items-center justify-center rounded-full text-ink-tertiary"
      >
        <CloseIcon />
      </button>
    </div>
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

  // Hides the message of a saved target after a short time.
  const { status, reset } = update;
  useEffect(() => {
    if (status !== "success") {
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
    <div className="mt-3 border-t border-line pt-3">
      <div role="group" aria-label="Soll" className="flex flex-wrap items-center gap-2">
        <span className="mr-1 text-sm font-medium text-ink-secondary">Soll</span>
        {targetChoices.map((value) => {
          const pressed = product.target === value;
          return (
            <button
              key={value}
              type="button"
              aria-pressed={pressed}
              disabled={update.isPending}
              onClick={() => chooseTarget(value)}
              className={`pressable size-11 shrink-0 rounded-lg text-lg font-semibold tabular-nums disabled:opacity-40 ${pressed ? "bg-accent text-white" : "bg-fill text-ink"}`}
            >
              {value}
            </button>
          );
        })}
      </div>
      <p
        role="status"
        className={`mt-1 min-h-5 text-sm font-medium ${status === "error" ? "text-danger" : "text-accent"}`}
      >
        {notice}
      </p>
      <div className="mt-1 flex flex-wrap gap-2">
        <Link
          to={`/produkt/${encodeURIComponent(product.id)}`}
          state={productLinkState("scan")}
          className={actionClass}
        >
          Name ändern
        </Link>
        <button type="button" onClick={onMerge} className={actionClass}>
          Stattdessen zu vorhandenem Produkt
        </button>
      </div>
    </div>
  );
}
