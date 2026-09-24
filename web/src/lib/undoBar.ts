/** How long the bar stays visible after a removal, in ms. */
export const undoBarDuration = 6000;

/** The product the bar is about: its id for the undo, its name for the text. */
export interface RemovedItem {
  productId: string;
  name: string;
}

/**
 * The bar "Entfernt: <Name>" with [Rückgängig] below the shopping list
 * (docs/plan.md, F20 and F21): hidden, shown (hidden at hideAt), undoing
 * (the mark runs again, no time limit) or failed (the undo failed; no time
 * limit, ADR-0016). Times are in ms, as from Date.now().
 */
export type UndoBarState =
  | { status: "hidden" }
  | { status: "shown"; item: RemovedItem; hideAt: number }
  | { status: "undoing"; item: RemovedItem }
  | { status: "failed"; item: RemovedItem };

export type UndoBarAction =
  | { type: "removed"; item: RemovedItem; now: number }
  | { type: "tick"; now: number }
  | { type: "undo" }
  | { type: "undone"; productId: string }
  | { type: "undoFailed"; productId: string }
  | { type: "close" };

export const hiddenUndoBar: UndoBarState = { status: "hidden" };

/**
 * Returns the next state of the bar:
 *
 *   - removed: shows the removed item for undoBarDuration, replacing the
 *     bar shown before in any state.
 *   - tick: hides a shown bar whose time is up.
 *   - undo: a tap on [Rückgängig] of a shown or failed bar; the bar waits
 *     for the mark without a time limit.
 *   - undone: hides the bar if it waits for the mark of productId, which
 *     brought the item back to the list.
 *   - undoFailed: the mark of productId failed; the bar shows the failure
 *     and offers [Rückgängig] again, until the next removal, the next
 *     [Rückgängig] or close.
 *   - close: hides a failed bar (its button "Schließen").
 *
 * undone and undoFailed for another product (a removal replaced the bar in
 * the meantime) and actions that do not apply to the state return it
 * unchanged.
 */
export function undoBarReducer(state: UndoBarState, action: UndoBarAction): UndoBarState {
  switch (action.type) {
    case "removed":
      return { status: "shown", item: action.item, hideAt: action.now + undoBarDuration };
    case "tick":
      if (state.status === "shown" && action.now >= state.hideAt) {
        return hiddenUndoBar;
      }
      return state;
    case "undo":
      if (state.status !== "shown" && state.status !== "failed") {
        return state;
      }
      return { status: "undoing", item: state.item };
    case "undone":
      if (state.status !== "undoing" || state.item.productId !== action.productId) {
        return state;
      }
      return hiddenUndoBar;
    case "undoFailed":
      if (state.status !== "undoing" || state.item.productId !== action.productId) {
        return state;
      }
      return { status: "failed", item: state.item };
    case "close":
      return state.status === "failed" ? hiddenUndoBar : state;
  }
}
