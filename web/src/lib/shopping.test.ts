import { describe, expect, it } from "vitest";
import {
  crateQuantity,
  cratesText,
  missingBottlesText,
  shoppingQuantity,
  shoppingText,
  type ShoppingItem,
} from "./shopping";

const kidneyBeans: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000001",
  name: "Kidneybohnen",
  brand: "Bonduelle",
  missing: 3,
  stock: 2,
  target: 5,
  marked: false,
  crate_size: null,
};

const flour: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000002",
  name: "Mehl",
  brand: null,
  missing: 4,
  stock: 0,
  target: 4,
  marked: false,
  crate_size: null,
};

const pasta: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000003",
  name: "Spaghetti",
  brand: "Barilla",
  missing: 1,
  stock: 4,
  target: 5,
  marked: false,
  crate_size: null,
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
  crate_size: null,
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
  crate_size: null,
};

// Bought in crates of 20: stock 3 of target 20, so 17 bottles are missing.
const beer: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000006",
  name: "Jever Pilsener",
  brand: "Jever",
  missing: 17,
  stock: 3,
  target: 20,
  marked: false,
  crate_size: 20,
};

// Bought in crates of 12, marked without a shortfall.
const water: ShoppingItem = {
  product_id: "0192a3b4-0000-7000-8000-000000000007",
  name: "Mineralwasser",
  brand: "Gerolsteiner",
  missing: 0,
  stock: 14,
  target: 12,
  marked: true,
  crate_size: 12,
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

describe("crateQuantity", () => {
  it("rounds a shortfall of 17 with a crate of 20 up to 1 crate", () => {
    expect(crateQuantity(beer)).toEqual({ crates: 1, bottles: 17 });
  });

  it("rounds a shortfall of 21 with a crate of 20 up to 2 crates", () => {
    expect(crateQuantity({ ...beer, missing: 21 })).toEqual({
      crates: 2,
      bottles: 21,
    });
  });

  it("gives exactly 1 crate for a shortfall of 20 with a crate of 20", () => {
    expect(crateQuantity({ ...beer, missing: 20 })).toEqual({
      crates: 1,
      bottles: 20,
    });
  });

  it("gives 1 crate for a shortfall of 1 bottle", () => {
    expect(crateQuantity({ ...beer, missing: 1 })).toEqual({
      crates: 1,
      bottles: 1,
    });
  });

  it("gives a marked item with a shortfall its crates", () => {
    expect(crateQuantity({ ...beer, marked: true, missing: 41 })).toEqual({
      crates: 3,
      bottles: 41,
    });
  });

  it("gives no crates without a crate size and leaves the quantity unchanged", () => {
    expect(crateQuantity(kidneyBeans)).toBeNull();
    expect(shoppingQuantity(kidneyBeans)).toBe(3);
  });

  it("gives no crates to a marked item without a shortfall", () => {
    expect(crateQuantity(water)).toBeNull();
    expect(shoppingQuantity(water)).toBeNull();
  });
});

describe("cratesText", () => {
  it("names a single crate in the singular", () => {
    expect(cratesText(1)).toBe("1 Kasten");
  });

  it("names several crates in the plural", () => {
    expect(cratesText(2)).toBe("2 Kästen");
    expect(cratesText(10)).toBe("10 Kästen");
  });
});

describe("missingBottlesText", () => {
  it("names a single missing bottle in the singular", () => {
    expect(missingBottlesText(1)).toBe("fehlt 1 Flasche");
  });

  it("names several missing bottles in the plural", () => {
    expect(missingBottlesText(17)).toBe("fehlen 17 Flaschen");
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

  it("writes an item with a crate size in crates", () => {
    expect(shoppingText([beer])).toBe("1 Kasten Jever Pilsener");
  });

  it("writes several crates in the plural", () => {
    expect(shoppingText([{ ...beer, missing: 21 }])).toBe(
      "2 Kästen Jever Pilsener",
    );
  });

  it("writes a marked item with a crate size and no shortfall as its name alone", () => {
    expect(shoppingText([water])).toBe("Mineralwasser");
  });

  it("mixes crates, quantities and names in the order of the list", () => {
    expect(shoppingText([kidneyBeans, beer, milk, water])).toBe(
      "3 × Kidneybohnen\n1 Kasten Jever Pilsener\nMilch\nMineralwasser",
    );
  });
});
