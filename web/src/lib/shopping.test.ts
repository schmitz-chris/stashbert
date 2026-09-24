import { describe, expect, it } from "vitest";
import { shoppingText, type ShoppingItem } from "./shopping";

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
});
