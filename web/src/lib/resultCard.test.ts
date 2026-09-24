import { describe, expect, it } from "vitest";
import type { Movement } from "./movements";
import type { Product } from "./products";
import {
  cardDuration,
  cardView,
  formatStockChange,
  hiddenCard,
  resultCardReducer,
  targetChoices,
  type CardBooking,
  type ResultCardAction,
  type ResultCardState,
} from "./resultCard";
import type { MovementResult } from "./scan";

const product: Product = {
  id: "01a0ce63-0000-7000-8000-000000000000",
  name: "Kidneybohnen",
  brand: null,
  package_size: null,
  note: null,
  stock: 4,
  target: 3,
  min_stock: null,
  missing: 0,
  marked: false,
  needs_review: false,
  origin: "manual",
  lookup_state: "none",
  has_image: false,
  barcodes: [{ code: "4001234567890", units: 1 }],
  created_at: "2026-09-23T10:00:00.000Z",
  updated_at: "2026-09-23T10:00:00.000Z",
};

function movement(fields: Partial<Movement> = {}): Movement {
  return {
    id: "01a0ce63-0000-7000-8000-000000000001",
    product_id: product.id,
    kind: "add",
    delta: 1,
    stock_after: 4,
    barcode: "4001234567890",
    reverses_id: null,
    created_at: "2026-09-23T10:00:00.000Z",
    ...fields,
  };
}

function result(id: string): MovementResult {
  return {
    movement: movement({ id }),
    product,
    product_created: false,
    warnings: [],
    message: "Kidneybohnen 3 → 4",
  };
}

const first = result("01a0ce63-0000-7000-8000-000000000001");
const second = result("01a0ce63-0000-7000-8000-000000000002");

// Applies actions to state one after the other.
function run(state: ResultCardState, ...actions: ResultCardAction[]): ResultCardState {
  return actions.reduce(resultCardReducer, state);
}

// The card showing first, shown at time 1000.
const shown = run(hiddenCard, { type: "show", result: first, kind: "add", now: 1000 });

describe("resultCardReducer", () => {
  it("starts hidden", () => {
    expect(hiddenCard).toEqual({ status: "hidden" });
  });

  it("shows a booking with its kind for cardDuration", () => {
    expect(cardDuration).toBe(10_000);
    expect(shown).toEqual({
      status: "running",
      booking: { result: first, kind: "add", merged: false },
      hideAt: 11_000,
    });
  });

  it("stays visible until the time is up", () => {
    expect(run(shown, { type: "tick", now: 10_999 })).toBe(shown);
  });

  it("is hidden after 10 s", () => {
    expect(run(shown, { type: "tick", now: 11_000 })).toEqual(hiddenCard);
    expect(run(shown, { type: "tick", now: 30_000 })).toEqual(hiddenCard);
  });

  it("replaces the card on a new booking and starts the 10 s again", () => {
    const replaced = run(shown, {
      type: "show",
      result: second,
      kind: "consume",
      now: 9000,
    });
    expect(replaced).toEqual({
      status: "running",
      booking: { result: second, kind: "consume", merged: false },
      hideAt: 19_000,
    });
    expect(run(replaced, { type: "tick", now: 11_000 })).toBe(replaced);
    expect(run(replaced, { type: "tick", now: 19_000 })).toEqual(hiddenCard);
  });

  it("shows a booking again after the card was hidden", () => {
    const again = run(
      shown,
      { type: "tick", now: 11_000 },
      { type: "show", result: second, kind: "add", now: 12_000 },
    );
    expect(again).toEqual({
      status: "running",
      booking: { result: second, kind: "add", merged: false },
      hideAt: 22_000,
    });
  });

  it("stops the time while paused", () => {
    const paused = run(shown, { type: "pause", now: 4000 });
    expect(paused).toEqual({
      status: "paused",
      booking: { result: first, kind: "add", merged: false },
      remaining: 7000,
    });
    expect(run(paused, { type: "tick", now: 60_000 })).toBe(paused);
  });

  it("continues with the time left when resumed", () => {
    const resumed = run(
      shown,
      { type: "pause", now: 4000 },
      { type: "resume", now: 50_000 },
    );
    expect(resumed).toEqual({
      status: "running",
      booking: { result: first, kind: "add", merged: false },
      hideAt: 57_000,
    });
    expect(run(resumed, { type: "tick", now: 56_999 })).toBe(resumed);
    expect(run(resumed, { type: "tick", now: 57_000 })).toEqual(hiddenCard);
  });

  it("keeps no negative time left when paused after the time was up", () => {
    const paused = run(shown, { type: "pause", now: 12_000 });
    expect(paused).toMatchObject({ status: "paused", remaining: 0 });
    const resumed = run(paused, { type: "resume", now: 20_000 });
    expect(run(resumed, { type: "tick", now: 20_000 })).toEqual(hiddenCard);
  });

  it("ignores pause while paused and resume while running", () => {
    const paused = run(shown, { type: "pause", now: 4000 });
    expect(run(paused, { type: "pause", now: 8000 })).toBe(paused);
    expect(run(shown, { type: "resume", now: 8000 })).toBe(shown);
  });

  it("starts a new booking running, also while paused", () => {
    const replaced = run(
      shown,
      { type: "pause", now: 4000 },
      { type: "show", result: second, kind: "add", now: 5000 },
    );
    expect(replaced).toEqual({
      status: "running",
      booking: { result: second, kind: "add", merged: false },
      hideAt: 15_000,
    });
  });

  it("ignores tick, pause, resume and hide while hidden", () => {
    for (const action of [
      { type: "tick", now: 1000 },
      { type: "pause", now: 1000 },
      { type: "resume", now: 1000 },
      { type: "hide", movementId: first.movement.id },
    ] as const) {
      expect(resultCardReducer(hiddenCard, action)).toBe(hiddenCard);
    }
  });

  it("hides the card that shows the booking", () => {
    expect(run(shown, { type: "hide", movementId: first.movement.id })).toEqual(
      hiddenCard,
    );
  });

  it("hides a paused card that shows the booking", () => {
    const paused = run(shown, { type: "pause", now: 4000 });
    expect(run(paused, { type: "hide", movementId: first.movement.id })).toEqual(
      hiddenCard,
    );
  });

  it("keeps the card of another booking on hide", () => {
    expect(run(shown, { type: "hide", movementId: second.movement.id })).toBe(shown);
  });
});

// A booking that created a placeholder, and the product it was merged into.
const created: MovementResult = {
  movement: movement({ id: "01a0ce63-0000-7000-8000-000000000003", delta: 1, stock_after: 1 }),
  product: {
    ...product,
    id: "01a0ce63-0000-7000-8000-00000000000a",
    name: "Neues Produkt 2000000000008",
    stock: 1,
    target: 0,
    missing: 0,
    needs_review: true,
    origin: "placeholder",
    barcodes: [{ code: "2000000000008", units: 1 }],
  },
  product_created: true,
  warnings: ["placeholder_created"],
  message: "Neu: Neues Produkt 2000000000008 0 → 1",
};
const mergeTarget: Product = {
  ...product,
  id: "01a0ce63-0000-7000-8000-00000000000b",
  name: "Nudeln",
  stock: 7,
  target: 4,
};

// The card of the new product, shown at time 1000.
const shownNew = run(hiddenCard, { type: "show", result: created, kind: "add", now: 1000 });

describe("resultCardReducer with a new product", () => {
  const changed = { ...created.product, target: 3 };

  it("shows the changed product and keeps the time", () => {
    const updated = run(shownNew, { type: "update", product: changed }, { type: "tick", now: 10_999 });
    expect(updated).toEqual({
      status: "running",
      booking: { result: { ...created, product: changed }, kind: "add", merged: false },
      hideAt: 11_000,
    });
    expect(run(updated, { type: "tick", now: 11_000 })).toEqual(hiddenCard);
  });

  it("updates a paused card and keeps it paused", () => {
    const paused = run(shownNew, { type: "pause", now: 4000 });
    expect(run(paused, { type: "update", product: changed })).toEqual({
      status: "paused",
      booking: { result: { ...created, product: changed }, kind: "add", merged: false },
      remaining: 7000,
    });
  });

  it("ignores an update of another product", () => {
    expect(run(shownNew, { type: "update", product: mergeTarget })).toBe(shownNew);
  });

  it("shows the target after a merge, with the booking, and keeps the time", () => {
    const merged = run(
      shownNew,
      { type: "pause", now: 4000 },
      { type: "merge", sourceId: created.product.id, target: mergeTarget },
      { type: "resume", now: 20_000 },
    );
    expect(merged).toEqual({
      status: "running",
      booking: {
        result: {
          ...created,
          movement: { ...created.movement, product_id: mergeTarget.id },
          product: mergeTarget,
          product_created: false,
        },
        kind: "add",
        merged: true,
      },
      hideAt: 27_000,
    });
  });

  it("keeps the booking for [Rückgängig] and books [+1] on the target after a merge", () => {
    const merged = run(shownNew, {
      type: "merge",
      sourceId: created.product.id,
      target: mergeTarget,
    });
    expect(merged).toMatchObject({ status: "running", hideAt: 11_000 });
    if (merged.status === "hidden") {
      throw new Error("card hidden");
    }
    expect(merged.booking.result.movement.id).toBe(created.movement.id);
    expect(merged.booking.result.product.id).toBe(mergeTarget.id);
    expect(run(merged, { type: "hide", movementId: created.movement.id })).toEqual(hiddenCard);
  });

  it("ignores a merge of another product", () => {
    expect(run(shownNew, { type: "merge", sourceId: mergeTarget.id, target: product })).toBe(
      shownNew,
    );
  });

  it("ignores update and merge while hidden", () => {
    expect(run(hiddenCard, { type: "update", product: changed })).toBe(hiddenCard);
    expect(
      run(hiddenCard, { type: "merge", sourceId: created.product.id, target: mergeTarget }),
    ).toBe(hiddenCard);
  });

  it("shows a new booking unmerged", () => {
    const merged = run(shownNew, {
      type: "merge",
      sourceId: created.product.id,
      target: mergeTarget,
    });
    expect(run(merged, { type: "show", result: first, kind: "add", now: 5000 })).toEqual({
      status: "running",
      booking: { result: first, kind: "add", merged: false },
      hideAt: 15_000,
    });
  });
});

describe("cardView", () => {
  function booking(result: MovementResult, merged = false): CardBooking {
    return { result, kind: "add", merged };
  }

  it("shows the name and the stock change of a known product", () => {
    expect(cardView(booking(first))).toEqual({
      title: "Kidneybohnen",
      stock: "3 → 4",
      isNew: false,
      review: false,
    });
  });

  it("does not ask to review a known product", () => {
    const known = { ...first, product: { ...product, needs_review: true } };
    expect(cardView(booking(known))).toMatchObject({ isNew: false, review: false });
  });

  it("shows a new product with Neu: and asks to review it", () => {
    expect(cardView(booking(created))).toEqual({
      title: "Neu: Neues Produkt 2000000000008",
      stock: "0 → 1",
      isNew: true,
      review: true,
    });
  });

  it("does not ask to review a new product without needs_review", () => {
    const found = { ...created, product: { ...created.product, needs_review: false } };
    expect(cardView(booking(found))).toMatchObject({
      title: "Neu: Neues Produkt 2000000000008",
      isNew: true,
      review: false,
    });
  });

  it("shows the target with its stock after a merge", () => {
    const merged = run(shownNew, {
      type: "merge",
      sourceId: created.product.id,
      target: mergeTarget,
    });
    if (merged.status === "hidden") {
      throw new Error("card hidden");
    }
    expect(cardView(merged.booking)).toEqual({
      title: "Nudeln",
      stock: "Bestand: 7",
      isNew: false,
      review: false,
    });
  });
});

describe("targetChoices", () => {
  it("offers 1, 2, 3, 5 and 10", () => {
    expect(targetChoices).toEqual([1, 2, 3, 5, 10]);
  });
});

describe("formatStockChange", () => {
  it.each([
    ["an add", movement({ kind: "add", delta: 1, stock_after: 4 }), "3 → 4"],
    ["a consume", movement({ kind: "consume", delta: -1, stock_after: 2 }), "3 → 2"],
    ["a clamped consume", movement({ kind: "consume", delta: -1, stock_after: 0 }), "1 → 0"],
    ["a booking of a new product", movement({ kind: "add", delta: 2, stock_after: 2 }), "0 → 2"],
    ["a reversal", movement({ kind: "reversal", delta: -1, stock_after: 3 }), "4 → 3"],
  ])("shows the stock before and after %s", (_name, booked, text) => {
    expect(formatStockChange(booked)).toBe(text);
  });
});
