import { useId } from "react";
import { formatStockChange, type CardBooking } from "../lib/resultCard";

interface ResultCardProps {
  booking: CardBooking;
  /** Locks both buttons, while an action of the card runs. */
  disabled: boolean;
  onPlusOne: () => void;
  onUndo: () => void;
}

/**
 * The result card of the scan view: the product of the booking, its
 * stock before and after, and the buttons [+1] and [Rückgängig].
 */
export function ResultCard({ booking, disabled, onPlusOne, onUndo }: ResultCardProps) {
  const nameId = useId();
  const { product, movement } = booking.result;
  const plusOneClass = booking.kind === "add" ? "bg-emerald-600" : "bg-sky-600";

  return (
    <section
      aria-labelledby={nameId}
      className="flex items-center gap-2 rounded-xl bg-white p-3 shadow"
    >
      <div className="min-w-0 flex-1">
        <h2 id={nameId} className="truncate font-semibold">
          {product.name}
        </h2>
        <p className="text-xl font-bold tabular-nums">{formatStockChange(movement)}</p>
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
    </section>
  );
}
