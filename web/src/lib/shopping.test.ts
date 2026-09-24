import { describe, expect, it } from "vitest";
import { shoppingQuantity, shoppingText, type ShoppingItem } from "./shopping";

const kidneyBeans: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000001",
  name: "Kidneybohnen",
  brand: "Bonduelle",
  missing: 3,
  stock: 2,
  target: 5,
  marked: false,
};

const flour: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000002",
  name: "Mehl",
  brand: null,
  missing: 4,
  stock: 0,
  target: 4,
  marked: false,
};

const pasta: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000003",
  name: "Spaghetti",
  brand: "Barilla",
  missing: 1,
  stock: 4,
  target: 5,
  marked: false,
};

// Marked without a shortfall: on the list only because of the mark.
const milk: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000004",
  name: "Milch",
  brand: "Weihenstephan",
  missing: 0,
  stock: 3,
  target: 2,
  marked: true,
};

// Marked with a shortfall.
const rice: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000005",
  name: "Reis",
  brand: null,
  missing: 2,
  stock: 0,
  target: 2,
  marked: true,
};

describe("shoppingQuantity", () => {
  it("gives missing for an item that is not marked", () => {
    expect(shoppingQuantity(kidneyBeans)).toBe(3);
  });

  it("gives missing for a marked item with a shortfall", () => {
    expect(shoppingQuantity(rice)).toBe(2);
  });

  it("gives no quantity for a marked item without a shortfall", () => {
    expect(shoppingQuantity(milk)).toBeNull();
  });
});

describe("shoppingText", () => {
  it("writes one line per item without the brand", () => {
    expect(shoppingText([kidneyBeans, flour, pasta])).toBe(
      "3 × Kidneybohnen\n4 × Mehl\n1 × Spaghetti",
    );
  });

  it("writes a single item without a line break", () => {
    expect(shoppingText([kidneyBeans])).toBe("3 × Kidneybohnen");
  });

  it("gives an empty text for an empty list", () => {
    expect(shoppingText([])).toBe("");
  });

  it("keeps the order of the list", () => {
    expect(shoppingText([pasta, kidneyBeans, flour])).toBe(
      "1 × Spaghetti\n3 × Kidneybohnen\n4 × Mehl",
    );
  });

  it("writes a marked item without a shortfall as its name alone", () => {
    expect(shoppingText([milk])).toBe("Milch");
  });

  it("writes a marked item with a shortfall with its quantity", () => {
    expect(shoppingText([rice])).toBe("2 × Reis");
  });

  it("mixes marked items and items with a shortfall in the order of the list", () => {
    expect(shoppingText([kidneyBeans, milk, rice, flour])).toBe(
      "3 × Kidneybohnen\nMilch\n2 × Reis\n4 × Mehl",
    );
  });
});
