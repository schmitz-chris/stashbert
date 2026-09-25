import { describe, expect, it } from "vitest";
import type { Movement } from "./movements";
import type { Product } from "./products";
import {
  asksForName,
  cardView,
  formatStockChange,
  hiddenCard,
  markCardView,
  maxNameLength,
  nameToSave,
  openGtinDbUrl,
  placeholderCode,
  resultCardReducer,
  targetChoices,
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

describe("asksForName", () => {
  function booking(result: MovementResult): CardBooking {
    return { result, kind: "add", merged: false };
  }

  it("asks for the name of a new placeholder", () => {
    expect(asksForName(booking(created))).toBe(true);
  });

  it("asks for the name of a placeholder stored again, but not when consuming", () => {
    const again: MovementResult = {
      ...created,
      movement: movement({ id: "01a0ce63-0000-7000-8000-000000000004", stock_after: 2 }),
      product: { ...created.product, stock: 2, target: 3 },
      product_created: false,
      warnings: [],
    };
    expect(asksForName(booking(again))).toBe(true);
    expect(asksForName({ result: again, kind: "consume", merged: false })).toBe(false);
  });

  it("does not ask for the name of a product from Open Food Facts", () => {
    const found: MovementResult = {
      ...created,
      product: {
        ...created.product,
        name: "Kidneybohnen",
        origin: "openfoodfacts",
        lookup_state: "done",
      },
      warnings: [],
    };
    expect(found.product.needs_review).toBe(true);
    expect(asksForName(booking(found))).toBe(false);
  });

  it("does not ask again after the name was saved", () => {
    const saved = run(shownNew, {
      type: "update",
      product: { ...created.product, name: "Kaffeebohnen", needs_review: false },
    });
    if (saved.status === "hidden" || saved.content.kind === "mark") {
      throw new Error("no booking card");
    }
    expect(saved.content.result.product.origin).toBe("placeholder");
    expect(asksForName(saved.content)).toBe(false);
    expect(cardView(saved.content).title).toBe("Neu: Kaffeebohnen");
  });

  it("does not ask for the name of a known product", () => {
    expect(asksForName(booking(first))).toBe(false);
  });
});

describe("nameToSave", () => {
  it("trims the name", () => {
    expect(nameToSave("  Kaffeebohnen \t")).toBe("Kaffeebohnen");
  });

  it.each([
    ["an empty text", ""],
    ["a text of spaces", "   "],
    ["a text of a line break", "\n"],
  ])("returns null for %s", (_name, text) => {
    expect(nameToSave(text)).toBeNull();
  });

  it("accepts one character and 120 characters", () => {
    expect(nameToSave("X")).toBe("X");
    const longest = "a".repeat(maxNameLength);
    expect(nameToSave(` ${longest} `)).toBe(longest);
  });

  it("returns null for 121 characters", () => {
    expect(nameToSave("a".repeat(maxNameLength + 1))).toBeNull();
  });

  it("counts characters, not UTF-16 units", () => {
    const emoji = "🥫".repeat(maxNameLength);
    expect(emoji.length).toBe(2 * maxNameLength);
    expect(nameToSave(emoji)).toBe(emoji);
    expect(nameToSave(`${emoji}🥫`)).toBeNull();
  });

  it("has a limit of 120 characters", () => {
    expect(maxNameLength).toBe(120);
  });
});

describe("placeholderCode", () => {
  it("returns the code of the booking", () => {
    expect(placeholderCode({ result: created, kind: "add", merged: false })).toBe(
      "4001234567890",
    );
  });

  it("returns the barcode of the product for a booking without a code", () => {
    const plusOne: MovementResult = {
      ...created,
      movement: { ...created.movement, barcode: null },
      product_created: false,
    };
    expect(placeholderCode({ result: plusOne, kind: "add", merged: false })).toBe(
      "2000000000008",
    );
  });

  it("returns null without a code", () => {
    const none: MovementResult = {
      ...created,
      movement: { ...created.movement, barcode: null },
      product: { ...created.product, barcodes: [] },
    };
    expect(placeholderCode({ result: none, kind: "add", merged: false })).toBeNull();
  });
});

describe("openGtinDbUrl", () => {
  it("links to the page of a 13-digit code", () => {
    expect(openGtinDbUrl("4006381333931")).toBe(
      "https://opengtindb.org/index.php?cmd=ean1&ean=4006381333931",
    );
  });

  it("links to the page of an 8-digit code", () => {
    expect(openGtinDbUrl("96385074")).toBe(
      "https://opengtindb.org/index.php?cmd=ean1&ean=96385074",
    );
  });

  it("encodes the code", () => {
    expect(openGtinDbUrl("12 3&x=4")).toBe(
      "https://opengtindb.org/index.php?cmd=ean1&ean=12%203%26x%3D4",
    );
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
