import { describe, expect, it } from "vitest";
import type { Movement } from "./movements";
import type { Product } from "./products";
import {
  cardView,
  crateChoices,
  formatStockChange,
  hiddenCard,
  markCardView,
  offersCrate,
  parseCrateSize,
  resultCardReducer,
  targetChoices,
  undoOrder,
  type CardBooking,
  type ResultCardAction,
  type ResultCardState,
} from "./resultCard";
import type { MarkResult, MovementResult } from "./scan";

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
  crate_size: null,
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

// The card showing first.
const shown = run(hiddenCard, { type: "show", result: first, kind: "add" });

describe("resultCardReducer", () => {
  it("starts hidden", () => {
    expect(hiddenCard).toEqual({ status: "hidden" });
  });

  it("shows a booking with its kind, without a time limit", () => {
    expect(shown).toEqual({
      status: "shown",
      content: { result: first, kind: "add", merged: false },
    });
  });

  it("replaces the card on the next booking", () => {
    expect(run(shown, { type: "show", result: second, kind: "consume" })).toEqual({
      status: "shown",
      content: { result: second, kind: "consume", merged: false },
    });
  });

  it("hides the card on close", () => {
    expect(run(shown, { type: "close" })).toEqual(hiddenCard);
  });

  it("shows the next booking after the card was closed", () => {
    expect(
      run(shown, { type: "close" }, { type: "show", result: second, kind: "add" }),
    ).toEqual({
      status: "shown",
      content: { result: second, kind: "add", merged: false },
    });
  });

  it("ignores close and hide while hidden", () => {
    for (const action of [
      { type: "close" },
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

// The card of the new product.
const shownNew = run(hiddenCard, { type: "show", result: created, kind: "add" });

describe("resultCardReducer with a new product", () => {
  const changed = { ...created.product, target: 3 };

  it("shows the changed product", () => {
    expect(run(shownNew, { type: "update", product: changed })).toEqual({
      status: "shown",
      content: { result: { ...created, product: changed }, kind: "add", merged: false },
    });
  });

  it("ignores an update of another product", () => {
    expect(run(shownNew, { type: "update", product: mergeTarget })).toBe(shownNew);
  });

  it("shows the target after a merge, with the booking", () => {
    const merged = run(shownNew, {
      type: "merge",
      sourceId: created.product.id,
      target: mergeTarget,
    });
    expect(merged).toEqual({
      status: "shown",
      content: {
        result: {
          ...created,
          movement: { ...created.movement, product_id: mergeTarget.id },
          product: mergeTarget,
          product_created: false,
        },
        kind: "add",
        merged: true,
      },
    });
  });

  it("keeps the booking for [Rückgängig] and books [+1] on the target after a merge", () => {
    const merged = run(shownNew, {
      type: "merge",
      sourceId: created.product.id,
      target: mergeTarget,
    });
    if (merged.status === "hidden" || merged.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(merged.content.result.movement.id).toBe(created.movement.id);
    expect(merged.content.result.product.id).toBe(mergeTarget.id);
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
    expect(run(merged, { type: "show", result: first, kind: "add" })).toEqual({
      status: "shown",
      content: { result: first, kind: "add", merged: false },
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
    if (merged.status === "hidden" || merged.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(cardView(merged.content)).toEqual({
      title: "Nudeln",
      stock: "Bestand: 7",
      isNew: false,
      review: false,
    });
  });
});

// A mark of a known product by a scan, and one of a product that was
// listed before.
const marked: MarkResult = {
  product: { ...product, marked: true },
  product_created: false,
  already_listed: false,
  message: "Vorgemerkt: Kidneybohnen",
};
const listed: MarkResult = {
  product: { ...mergeTarget, marked: true },
  product_created: false,
  already_listed: true,
  message: "Schon auf der Liste: Nudeln",
};

// The card of the mark.
const shownMark = run(hiddenCard, { type: "show", result: marked, kind: "mark" });

describe("resultCardReducer with a mark", () => {
  it("shows a mark, without a time limit", () => {
    expect(shownMark).toEqual({
      status: "shown",
      content: { result: marked, kind: "mark" },
    });
  });

  it("replaces a booking card with a mark and a mark card with a booking", () => {
    expect(run(shown, { type: "show", result: listed, kind: "mark" })).toEqual({
      status: "shown",
      content: { result: listed, kind: "mark" },
    });
    expect(run(shownMark, { type: "show", result: first, kind: "add" })).toEqual({
      status: "shown",
      content: { result: first, kind: "add", merged: false },
    });
  });

  it("replaces a mark with the next mark", () => {
    expect(run(shownMark, { type: "show", result: listed, kind: "mark" })).toEqual({
      status: "shown",
      content: { result: listed, kind: "mark" },
    });
  });

  it("hides the card of a mark on close", () => {
    expect(run(shownMark, { type: "close" })).toEqual(hiddenCard);
  });

  it("hides the card of the mark after the mark of its product was undone", () => {
    expect(run(shownMark, { type: "hideMark", productId: product.id })).toEqual(hiddenCard);
  });

  it("keeps the card of a mark of another product on hideMark", () => {
    expect(run(shownMark, { type: "hideMark", productId: mergeTarget.id })).toBe(shownMark);
  });

  it("keeps a booking card on hideMark and a mark card on hide", () => {
    expect(run(shown, { type: "hideMark", productId: product.id })).toBe(shown);
    expect(run(shownMark, { type: "hide", movementId: first.movement.id })).toBe(shownMark);
  });

  it("ignores hideMark while hidden", () => {
    expect(run(hiddenCard, { type: "hideMark", productId: product.id })).toBe(hiddenCard);
  });

  it("ignores update and merge on a mark card", () => {
    expect(run(shownMark, { type: "update", product: { ...product, target: 5 } })).toBe(
      shownMark,
    );
    expect(
      run(shownMark, { type: "merge", sourceId: product.id, target: mergeTarget }),
    ).toBe(shownMark);
  });
});

describe("markCardView", () => {
  it("shows a product this scan marked as vorgemerkt, with [Rückgängig]", () => {
    expect(markCardView({ result: marked, kind: "mark" })).toEqual({
      title: "Kidneybohnen",
      status: "vorgemerkt",
      undo: true,
    });
  });

  it("shows a product that was listed before without [Rückgängig]", () => {
    expect(markCardView({ result: listed, kind: "mark" })).toEqual({
      title: "Nudeln",
      status: "schon auf der Liste",
      undo: false,
    });
  });

  it("shows a product the mark created with Neu: as vorgemerkt, with [Rückgängig]", () => {
    const createdMark: MarkResult = {
      product: { ...created.product, stock: 0, marked: true },
      product_created: true,
      already_listed: false,
      message: "Neu vorgemerkt: Neues Produkt 2000000000008",
    };
    expect(markCardView({ result: createdMark, kind: "mark" })).toEqual({
      title: "Neu: Neues Produkt 2000000000008",
      status: "vorgemerkt",
      undo: true,
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

  it("shows the stock before the first and after the last of two bookings", () => {
    const bottle = movement({ delta: 1, stock_after: 4 });
    const rest = movement({ delta: 19, stock_after: 23 });
    expect(formatStockChange(bottle, rest)).toBe("3 → 23");
  });
});

// The first bottle of a crate of beer (3 → 4) and, after "War ein Kasten"
// with 20, the rest of the crate (4 → 23).
const beer: Product = { ...product, name: "Jever Pilsener", stock: 4 };
const bottle: MovementResult = {
  ...result("01a0ce63-0000-7000-8000-000000000011"),
  product: beer,
  message: "Jever Pilsener 3 → 4",
};
const rest: MovementResult = {
  movement: movement({ id: "01a0ce63-0000-7000-8000-000000000012", delta: 19, stock_after: 23 }),
  product: { ...beer, stock: 23, crate_size: 20 },
  product_created: false,
  warnings: [],
  message: "Jever Pilsener 4 → 23",
};
const shownBottle = run(hiddenCard, { type: "show", result: bottle, kind: "add" });
const shownCrate = run(shownBottle, {
  type: "crate",
  movementId: bottle.movement.id,
  rest,
});

describe("resultCardReducer with a crate", () => {
  it("shows the bottle and the rest of the crate as one booking", () => {
    expect(shownCrate).toEqual({
      status: "shown",
      content: {
        result: { ...bottle, product: rest.product },
        kind: "add",
        merged: false,
        rest,
      },
    });
  });

  it("ignores the rest of a crate for another booking", () => {
    expect(run(shown, { type: "crate", movementId: bottle.movement.id, rest })).toBe(shown);
  });

  it("ignores a second rest and a rest while hidden or on a mark", () => {
    expect(run(shownCrate, { type: "crate", movementId: bottle.movement.id, rest })).toBe(
      shownCrate,
    );
    expect(run(hiddenCard, { type: "crate", movementId: bottle.movement.id, rest })).toBe(
      hiddenCard,
    );
    expect(run(shownMark, { type: "crate", movementId: bottle.movement.id, rest })).toBe(
      shownMark,
    );
  });

  it("shows the first bottle alone once the rest was undone", () => {
    const undone = run(shownCrate, { type: "hide", movementId: rest.movement.id });
    if (undone.status === "hidden" || undone.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(undone.content.result.movement.id).toBe(bottle.movement.id);
    expect(undone.content.rest).toBeUndefined();
    expect(cardView(undone.content).stock).toBe("3 → 4");
  });

  it("hides the card once the first bottle was undone as well", () => {
    expect(
      run(
        shownCrate,
        { type: "hide", movementId: rest.movement.id },
        { type: "hide", movementId: bottle.movement.id },
      ),
    ).toEqual(hiddenCard);
  });

  it("keeps the rest of the crate after a merge", () => {
    const merged = run(shownCrate, {
      type: "merge",
      sourceId: product.id,
      target: mergeTarget,
    });
    if (merged.status === "hidden" || merged.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(merged.content.rest).toBe(rest);
    expect(merged.content.merged).toBe(true);
  });
});

describe("cardView with a crate", () => {
  it("shows the stock before the bottle and after the rest", () => {
    if (shownCrate.status === "hidden" || shownCrate.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(cardView(shownCrate.content)).toEqual({
      title: "Jever Pilsener",
      stock: "3 → 23",
      isNew: false,
      review: false,
    });
  });

  it("keeps a new product new after the rest of its crate", () => {
    const shownNewCrate = run(
      shownNew,
      { type: "crate", movementId: created.movement.id, rest },
    );
    if (shownNewCrate.status === "hidden" || shownNewCrate.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(cardView(shownNewCrate.content)).toMatchObject({ isNew: true, stock: "0 → 23" });
  });
});

describe("undoOrder", () => {
  it("undoes a single booking alone", () => {
    expect(undoOrder({ result: bottle, kind: "add", merged: false })).toEqual([
      bottle.movement.id,
    ]);
  });

  it("undoes the rest of a crate first and then its first bottle", () => {
    expect(undoOrder({ result: bottle, kind: "add", merged: false, rest })).toEqual([
      rest.movement.id,
      bottle.movement.id,
    ]);
  });
});

describe("offersCrate", () => {
  const single: CardBooking = { result: bottle, kind: "add", merged: false };

  it("offers a crate for one bottle stored of a product without a crate size", () => {
    expect(offersCrate(single)).toBe(true);
  });

  it("offers a crate for a new product", () => {
    expect(offersCrate({ result: created, kind: "add", merged: false })).toBe(true);
  });

  it("does not offer a crate for a product with a crate size", () => {
    const known = { ...bottle, product: { ...beer, crate_size: 20 } };
    expect(offersCrate({ ...single, result: known })).toBe(false);
  });

  it("does not offer a crate once the rest was booked", () => {
    expect(offersCrate({ ...single, rest })).toBe(false);
  });

  it("does not offer a crate for a booking in mode consume", () => {
    const consumed = { ...bottle, movement: movement({ kind: "consume", delta: -1 }) };
    expect(offersCrate({ result: consumed, kind: "consume", merged: false })).toBe(false);
  });

  it("does not offer a crate for a booking of more than one unit", () => {
    const pack = { ...bottle, movement: movement({ delta: 6, stock_after: 9 }) };
    expect(offersCrate({ ...single, result: pack })).toBe(false);
  });
});

describe("crateChoices", () => {
  it("offers 6, 12, 20 and 24", () => {
    expect(crateChoices).toEqual([6, 12, 20, 24]);
  });
});

describe("parseCrateSize", () => {
  it.each([
    ["2", 2],
    ["11", 11],
    ["100", 100],
    [" 24 ", 24],
    ["024", 24],
  ])("accepts %j as %i", (input, size) => {
    expect(parseCrateSize(input)).toBe(size);
  });

  it.each(["", " ", "0", "1", "101", "1000", "2.5", "-6", "12a", "1e1"])(
    "rejects %j",
    (input) => {
      expect(parseCrateSize(input)).toBeNull();
    },
  );
});
