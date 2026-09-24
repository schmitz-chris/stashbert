import type { Movement } from "./movements";
import type { Product } from "./products";
import type { BookingMode, MarkResult, MovementResult } from "./scan";

/**
 * The booking a result card shows, with the kind that [+1] books again.
 * merged is true once the product of the booking was merged into another
 * one; result then holds that product, which the booking belongs to now.
 */
export interface CardBooking {
  result: MovementResult;
  kind: BookingMode;
  merged: boolean;
  /**
   * The booking of the rest of a crate after "War ein Kasten", whose first
   * bottle is result (docs/plan.md, F28). The card shows both as one
   * booking, and [Rückgängig] undoes both.
   */
  rest?: MovementResult;
}

/** The mark of a scanned code in mode mark (docs/plan.md, F15). */
export interface CardMark {
  result: MarkResult;
  kind: "mark";
}

/** What a result card shows: a booking or a mark, told apart by kind. */
export type CardContent = CardBooking | CardMark;

/**
 * The result card of the scan view (docs/plan.md, F09 and F21): hidden or
 * shown. A shown card has no time limit (ADR-0016); it stays until the next
 * booking or mark replaces it, until "Schließen" or until its booking or
 * mark is undone.
 */
export type ResultCardState =
  | { status: "hidden" }
  | { status: "shown"; content: CardContent };

export type ResultCardAction =
  | { type: "show"; result: MovementResult; kind: BookingMode }
  | { type: "show"; result: MarkResult; kind: "mark" }
  | { type: "close" }
  | { type: "hide"; movementId: string }
  | { type: "hideMark"; productId: string }
  | { type: "update"; product: Product }
  | { type: "merge"; sourceId: string; target: Product }
  | { type: "crate"; movementId: string; rest: MovementResult };

export const hiddenCard: ResultCardState = { status: "hidden" };

/**
 * Returns the next state of the result card:
 *
 *   - show: shows the booking result (booked with kind) or the mark
 *     result (kind mark), replacing the card shown before.
 *   - close: hides the card ("Schließen").
 *   - hide: hides the card if it shows the booking movementId, so undoing
 *     an older booking leaves the card of a newer one. If movementId is
 *     the rest of a crate on the card, the card shows its first bottle
 *     alone again, whose undoing is still to come.
 *   - hideMark: hides the card if it shows a mark of the product with
 *     productId, after its mark was undone.
 *   - update: replaces the product on the card with a changed version of
 *     it (same id), for example after its target was changed.
 *   - merge: after the product on the card (sourceId) was merged into
 *     target, the card shows target, to which the booking belongs now,
 *     and no longer as a new product.
 *   - crate: after "War ein Kasten" booked the rest of the crate whose
 *     first bottle is the booking movementId on the card, the card shows
 *     both as one booking, with the product from rest.
 *
 * update and merge only change what the card of a booking shows and leave
 * a card that shows a mark or another product unchanged. Actions that do
 * not apply to the state return it unchanged.
 */
export function resultCardReducer(
  state: ResultCardState,
  action: ResultCardAction,
): ResultCardState {
  switch (action.type) {
    case "show": {
      const content: CardContent =
        action.kind === "mark"
          ? { result: action.result, kind: "mark" }
          : { result: action.result, kind: action.kind, merged: false };
      return { status: "shown", content };
    }
    case "close":
      return state.status === "hidden" ? state : hiddenCard;
    case "hide":
      if (state.status === "hidden" || state.content.kind === "mark") {
        return state;
      }
      if (state.content.rest?.movement.id === action.movementId) {
        return { ...state, content: { ...state.content, rest: undefined } };
      }
      return state.content.result.movement.id === action.movementId ? hiddenCard : state;
    case "hideMark":
      if (
        state.status === "hidden" ||
        state.content.kind !== "mark" ||
        state.content.result.product.id !== action.productId
      ) {
        return state;
      }
      return hiddenCard;
    case "update":
      return withBooking(state, action.product.id, (booking) => ({
        ...booking,
        result: { ...booking.result, product: action.product },
      }));
    case "merge":
      return withBooking(state, action.sourceId, (booking) => ({
        ...booking,
        result: {
          ...booking.result,
          movement: { ...booking.result.movement, product_id: action.target.id },
          product: action.target,
          product_created: false,
        },
        merged: true,
      }));
    case "crate":
      if (
        state.status === "hidden" ||
        state.content.kind === "mark" ||
        state.content.result.movement.id !== action.movementId ||
        state.content.rest !== undefined
      ) {
        return state;
      }
      return {
        ...state,
        content: {
          ...state.content,
          result: { ...state.content.result, product: action.rest.product },
          rest: action.rest,
        },
      };
  }
}

// Returns state with its booking replaced by change(booking), if the card
// shows a booking of the product with productId; otherwise state unchanged.
function withBooking(
  state: ResultCardState,
  productId: string,
  change: (booking: CardBooking) => CardBooking,
): ResultCardState {
  if (
    state.status === "hidden" ||
    state.content.kind === "mark" ||
    state.content.result.product.id !== productId
  ) {
    return state;
  }
  return { ...state, content: change(state.content) };
}

/**
 * Formats the stock before and after a movement, for example "3 → 4".
 * The stock before is stock_after - delta, also for a new product (0).
 * With last, the stock after is that of last, for bookings in a row.
 */
export function formatStockChange(movement: Movement, last: Movement = movement): string {
  return `${movement.stock_after - movement.delta} → ${last.stock_after}`;
}

/**
 * Returns the ids of the bookings that [Rückgängig] undoes on the card of
 * booking, the latest first: the rest of a crate and then its first
 * bottle, or the booking alone.
 */
export function undoOrder({ result, rest }: CardBooking): string[] {
  return rest === undefined ? [result.movement.id] : [rest.movement.id, result.movement.id];
}

/**
 * Reports whether the card of booking offers "War ein Kasten"
 * (docs/plan.md, F28): for one bottle booked in mode add of a product
 * without a crate size, so that the rest of the crate can follow.
 */
export function offersCrate({ result, kind, rest }: CardBooking): boolean {
  return (
    kind === "add" &&
    rest === undefined &&
    result.movement.delta === 1 &&
    result.product.crate_size === null
  );
}

/** The crate sizes "War ein Kasten" offers; any other is typed in. */
export const crateChoices = [6, 12, 20, 24] as const;

/**
 * Returns the crate size typed in for "War ein Kasten": a whole number
 * from 2 to 100 (ADR-0017), spaces around it allowed. Returns null for any
 * other input.
 */
export function parseCrateSize(input: string): number | null {
  const text = input.trim();
  if (!/^[0-9]{1,3}$/.test(text)) {
    return null;
  }
  const size = Number(text);
  return size >= 2 && size <= 100 ? size : null;
}

/** The values the result card of a new product offers as its target. */
export const targetChoices = [1, 2, 3, 5, 10] as const;

/** What the result card shows for its booking. */
export interface CardView {
  /** The name of the product, as "Neu: <name>" for a new product. */
  title: string;
  /** The stock before and after the booking, or the stock after a merge. */
  stock: string;
  /** Whether the booking created the product (docs/plan.md, F10). */
  isNew: boolean;
  /** Whether a new product needs a review ("Bitte prüfen"). */
  review: boolean;
}

/**
 * Returns what the result card shows for booking: for a booking that
 * created its product "Neu: <name>", with the hint to review it if it
 * needs_review; after a merge the target with its stock; for a crate the
 * stock before its first bottle and after its rest.
 */
export function cardView({ result, merged, rest }: CardBooking): CardView {
  const { product, product_created: isNew } = result;
  return {
    title: isNew ? `Neu: ${product.name}` : product.name,
    stock: merged
      ? `Bestand: ${product.stock}`
      : formatStockChange(result.movement, rest?.movement),
    isNew,
    review: isNew && product.needs_review,
  };
}

/** What the result card shows for a mark (docs/plan.md, F15). */
export interface MarkCardView {
  /** The name of the product, as "Neu: <name>" if the mark created it. */
  title: string;
  /** "vorgemerkt", or "schon auf der Liste" if it was listed before. */
  status: string;
  /** Whether [Rückgängig] is offered: only if this scan marked it. */
  undo: boolean;
}

/**
 * Returns what the result card shows for mark: the name of the product,
 * as "Neu: <name>" if the mark created it, and "vorgemerkt", or "schon auf
 * der Liste" without [Rückgängig] if the product was on the shopping list
 * before (already_listed), so the mark did not come from this scan.
 */
export function markCardView({ result }: CardMark): MarkCardView {
  const { product, product_created: isNew, already_listed: listed } = result;
  return {
    title: isNew ? `Neu: ${product.name}` : product.name,
    status: listed ? "schon auf der Liste" : "vorgemerkt",
    undo: !listed,
  };
}
