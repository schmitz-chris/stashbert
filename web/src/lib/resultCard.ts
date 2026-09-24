import type { Movement } from "./movements";
import type { Product } from "./products";
import type { BookingMode, MarkResult, MovementResult } from "./scan";

/** How long the result card stays visible after a booking or mark, in ms. */
export const cardDuration = 10_000;

/**
 * The booking a result card shows, with the kind that [+1] books again.
 * merged is true once the product of the booking was merged into another
 * one; result then holds that product, which the booking belongs to now.
 */
export interface CardBooking {
  result: MovementResult;
  kind: BookingMode;
  merged: boolean;
}

/** The mark of a scanned code in mode mark (docs/plan.md, F15). */
export interface CardMark {
  result: MarkResult;
  kind: "mark";
}

/** What a result card shows: a booking or a mark, told apart by kind. */
export type CardContent = CardBooking | CardMark;

/**
 * The result card of the scan view (docs/plan.md, F09): hidden, running
 * (hidden at hideAt) or paused while a dialog of the card is open (hidden
 * remaining ms after it resumes). Times are in ms, as from Date.now().
 */
export type ResultCardState =
  | { status: "hidden" }
  | { status: "running"; content: CardContent; hideAt: number }
  | { status: "paused"; content: CardContent; remaining: number };

export type ResultCardAction =
  | { type: "show"; result: MovementResult; kind: BookingMode; now: number }
  | { type: "show"; result: MarkResult; kind: "mark"; now: number }
  | { type: "tick"; now: number }
  | { type: "pause"; now: number }
  | { type: "resume"; now: number }
  | { type: "hide"; movementId: string }
  | { type: "hideMark"; productId: string }
  | { type: "update"; product: Product }
  | { type: "merge"; sourceId: string; target: Product };

export const hiddenCard: ResultCardState = { status: "hidden" };

/**
 * Returns the next state of the result card:
 *
 *   - show: shows the booking result (booked with kind) or the mark
 *     result (kind mark) for cardDuration, replacing the card shown
 *     before, also a paused one.
 *   - tick: hides a running card whose time is up.
 *   - pause: stops the time of a running card.
 *   - resume: lets a paused card run for the time it had left.
 *   - hide: hides the card if it shows the booking movementId, so undoing
 *     an older booking leaves the card of a newer one.
 *   - hideMark: hides the card if it shows a mark of the product with
 *     productId, after its mark was undone.
 *   - update: replaces the product on the card with a changed version of
 *     it (same id), for example after its target was changed.
 *   - merge: after the product on the card (sourceId) was merged into
 *     target, the card shows target, to which the booking belongs now,
 *     and no longer as a new product.
 *
 * update and merge only change what the card of a booking shows, not its
 * time, and leave a card that shows a mark or another product unchanged.
 * Actions that do not apply to the state return it unchanged.
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
      return { status: "running", content, hideAt: action.now + cardDuration };
    }
    case "tick":
      return state.status === "running" && action.now >= state.hideAt ? hiddenCard : state;
    case "pause":
      if (state.status !== "running") {
        return state;
      }
      return {
        status: "paused",
        content: state.content,
        remaining: Math.max(0, state.hideAt - action.now),
      };
    case "resume":
      if (state.status !== "paused") {
        return state;
      }
      return {
        status: "running",
        content: state.content,
        hideAt: action.now + state.remaining,
      };
    case "hide":
      if (
        state.status === "hidden" ||
        state.content.kind === "mark" ||
        state.content.result.movement.id !== action.movementId
      ) {
        return state;
      }
      return hiddenCard;
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
      return withBooking(state, action.sourceId, ({ result, kind }) => ({
        result: {
          ...result,
          movement: { ...result.movement, product_id: action.target.id },
          product: action.target,
          product_created: false,
        },
        kind,
        merged: true,
      }));
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
 */
export function formatStockChange(movement: Movement): string {
  return `${movement.stock_after - movement.delta} → ${movement.stock_after}`;
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
 * needs_review; after a merge the target with its stock.
 */
export function cardView({ result, merged }: CardBooking): CardView {
  const { product, product_created: isNew } = result;
  return {
    title: isNew ? `Neu: ${product.name}` : product.name,
    stock: merged ? `Bestand: ${product.stock}` : formatStockChange(result.movement),
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
