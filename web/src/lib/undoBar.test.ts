import { describe, expect, it } from "vitest";
import {
  hiddenUndoBar,
  undoBarDuration,
  undoBarReducer,
  type RemovedItem,
  type UndoBarState,
} from "./undoBar";

const flour: RemovedItem = { productId: "01a0ce63-0000-7000-8000-000000000001", name: "Mehl" };
const beans: RemovedItem = {
  productId: "01a0ce63-0000-7000-8000-000000000002",
  name: "Kidneybohnen",
};

const shown: UndoBarState = { status: "shown", item: flour, hideAt: 1000 + undoBarDuration };
const undoing: UndoBarState = { status: "undoing", item: flour };
const failed: UndoBarState = { status: "failed", item: flour };

describe("undoBarReducer", () => {
  it("shows a removed item for undoBarDuration", () => {
    expect(undoBarReducer(hiddenUndoBar, { type: "removed", item: flour, now: 1000 })).toEqual(
      shown,
    );
  });

  it("replaces the bar with a new removal in every state", () => {
    const next: UndoBarState = { status: "shown", item: beans, hideAt: 3000 + undoBarDuration };
    for (const state of [shown, undoing, failed]) {
      expect(undoBarReducer(state, { type: "removed", item: beans, now: 3000 })).toEqual(next);
    }
  });

  it("hides a shown bar once its time is up", () => {
    const hideAt = 1000 + undoBarDuration;
    expect(undoBarReducer(shown, { type: "tick", now: hideAt - 1 })).toBe(shown);
    expect(undoBarReducer(shown, { type: "tick", now: hideAt })).toEqual(hiddenUndoBar);
  });

  it("keeps a bar that waits for the undo, however long it takes", () => {
    expect(undoBarReducer(undoing, { type: "tick", now: 1_000_000 })).toBe(undoing);
  });

  it("waits for the mark after a tap on Rückgängig", () => {
    expect(undoBarReducer(shown, { type: "undo" })).toEqual(undoing);
  });

  it("hides the bar once the item is marked again", () => {
    expect(undoBarReducer(undoing, { type: "undone", productId: flour.productId })).toEqual(
      hiddenUndoBar,
    );
  });

  it("shows a failed undo without a time limit and offers it again", () => {
    const state = undoBarReducer(undoing, { type: "undoFailed", productId: flour.productId });
    expect(state).toEqual(failed);
    expect(undoBarReducer(state, { type: "tick", now: 1_000_000 })).toBe(state);
    expect(undoBarReducer(state, { type: "undo" })).toEqual(undoing);
  });

  it("hides a failed undo on close", () => {
    expect(undoBarReducer(failed, { type: "close" })).toEqual(hiddenUndoBar);
  });

  it("ignores the result of an undo whose bar a removal replaced", () => {
    const replaced = undoBarReducer(undoing, { type: "removed", item: beans, now: 3000 });
    expect(undoBarReducer(replaced, { type: "undone", productId: flour.productId })).toBe(
      replaced,
    );
    expect(undoBarReducer(replaced, { type: "undoFailed", productId: flour.productId })).toBe(
      replaced,
    );
  });

  it("ignores actions that do not apply", () => {
    expect(undoBarReducer(hiddenUndoBar, { type: "tick", now: 1000 })).toBe(hiddenUndoBar);
    expect(undoBarReducer(hiddenUndoBar, { type: "undo" })).toBe(hiddenUndoBar);
    expect(undoBarReducer(undoing, { type: "undo" })).toBe(undoing);
    expect(undoBarReducer(shown, { type: "undone", productId: flour.productId })).toBe(shown);
    expect(undoBarReducer(shown, { type: "undoFailed", productId: flour.productId })).toBe(
      shown,
    );
    for (const state of [hiddenUndoBar, shown, undoing]) {
      expect(undoBarReducer(state, { type: "close" })).toBe(state);
    }
  });
});
