import type { Movement } from "./movements";
import type { MovementResult, ScanMode } from "./scan";

/** How long the result card stays visible after a booking, in ms. */
export const cardDuration = 10_000;

/** The booking a result card shows, with the kind that [+1] books again. */
export interface CardBooking {
  result: MovementResult;
  kind: ScanMode;
}

/**
 * The result card of the scan view (docs/plan.md, F09): hidden, running
 * (hidden at hideAt) or paused while a dialog of the card is open (hidden
 * remaining ms after it resumes). Times are in ms, as from Date.now().
 */
export type ResultCardState =
  | { status: "hidden" }
  | { status: "running"; booking: CardBooking; hideAt: number }
  | { status: "paused"; booking: CardBooking; remaining: number };

export type ResultCardAction =
  | { type: "show"; result: MovementResult; kind: ScanMode; now: number }
  | { type: "tick"; now: number }
  | { type: "pause"; now: number }
  | { type: "resume"; now: number }
  | { type: "hide"; movementId: string };

export const hiddenCard: ResultCardState = { status: "hidden" };

/**
 * Returns the next state of the result card:
 *
 *   - show: shows the booking result (booked with kind) for cardDuration,
 *     replacing the card shown before, also a paused one.
 *   - tick: hides a running card whose time is up.
 *   - pause: stops the time of a running card.
 *   - resume: lets a paused card run for the time it had left.
 *   - hide: hides the card if it shows the booking movementId, so undoing
 *     an older booking leaves the card of a newer one.
 *
 * Actions that do not apply to the state return it unchanged.
 */
export function resultCardReducer(
  state: ResultCardState,
  action: ResultCardAction,
): ResultCardState {
  switch (action.type) {
    case "show": {
      const booking = { result: action.result, kind: action.kind };
      return { status: "running", booking, hideAt: action.now + cardDuration };
    }
    case "tick":
      return state.status === "running" && action.now >= state.hideAt ? hiddenCard : state;
    case "pause":
      if (state.status !== "running") {
        return state;
      }
      return {
        status: "paused",
        booking: state.booking,
        remaining: Math.max(0, state.hideAt - action.now),
      };
    case "resume":
      if (state.status !== "paused") {
        return state;
      }
      return {
        status: "running",
        booking: state.booking,
        hideAt: action.now + state.remaining,
      };
    case "hide":
      if (state.status === "hidden" || state.booking.result.movement.id !== action.movementId) {
        return state;
      }
      return hiddenCard;
  }
}

/**
 * Formats the stock before and after a movement, for example "3 → 4".
 * The stock before is stock_after - delta, also for a new product (0).
 */
export function formatStockChange(movement: Movement): string {
  return `${movement.stock_after - movement.delta} → ${movement.stock_after}`;
}
