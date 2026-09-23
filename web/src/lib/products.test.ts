import { describe, expect, it } from "vitest";
import {
  filterProducts,
  matches,
  replaceProduct,
  sortByName,
  type Product,
} from "./products";

function product(fields: Partial<Product>): Product {
  return {
    id: "01a0ce63-0000-7000-8000-000000000000",
    name: "Produkt",
    brand: null,
    package_size: null,
    note: null,
    stock: 1,
    target: 1,
    min_stock: null,
    missing: 0,
    needs_review: false,
    origin: "manual",
    lookup_state: "none",
    has_image: false,
    barcodes: [],
    created_at: "2026-09-23T10:00:00.000Z",
    updated_at: "2026-09-23T10:00:00.000Z",
    ...fields,
  };
}

describe("matches", () => {
  const cheese = product({ name: "Käse", brand: "Allgäuer Hof" });

  it.each([
    ["kase", true],
    ["Käse", true],
    ["KÄSE", true],
    ["KASE", true],
    ["as", true],
    ["allgauer", true],
    ["HOF", true],
    ["  kase  ", true],
    ["milch", false],
    ["käse hof", false],
  ])("matches %j: %s", (query, expected) => {
    expect(matches(cheese, query)).toBe(expected);
  });

  it("finds a query with diacritics in a name without them", () => {
    expect(matches(product({ name: "Creme fraiche" }), "Crème")).toBe(true);
  });

  it("searches the brand", () => {
    const beans = product({ name: "Kidneybohnen", brand: "Bonduelle" });
    expect(matches(beans, "bondu")).toBe(true);
    expect(matches(beans, "barilla")).toBe(false);
  });

  it("does not fail without a brand", () => {
    expect(matches(product({ name: "Mehl", brand: null }), "barilla")).toBe(
      false,
    );
  });

  it.each(["", " ", "   "])("matches every product for %j", (query) => {
    expect(matches(product({ name: "Mehl" }), query)).toBe(true);
  });
});

describe("filterProducts", () => {
  const restock = product({ id: "a", stock: 1, target: 3, missing: 2 });
  const empty = product({ id: "b", stock: 0, target: 0, missing: 0 });
  const emptyRestock = product({ id: "c", stock: 0, target: 2, missing: 2 });
  const review = product({ id: "d", needs_review: true });
  const full = product({ id: "e", stock: 4, target: 3, missing: 0 });
  const products = [restock, empty, emptyRestock, review, full];

  it("keeps all products for all", () => {
    expect(filterProducts(products, "all")).toEqual(products);
  });

  it("keeps products with missing > 0 for restock", () => {
    expect(filterProducts(products, "restock")).toEqual([
      restock,
      emptyRestock,
    ]);
  });

  it("keeps products with stock 0 for empty", () => {
    expect(filterProducts(products, "empty")).toEqual([empty, emptyRestock]);
  });

  it("keeps products that need a review for review", () => {
    expect(filterProducts(products, "review")).toEqual([review]);
  });
});

describe("sortByName", () => {
  it("sorts in German order, umlauts next to their base letter", () => {
    const names = ["Zucker", "Bohnen", "Äpfel", "apfelmus", "Öl", "Nudeln"];
    const sorted = sortByName(names.map((name) => product({ name })));
    expect(sorted.map((p) => p.name)).toEqual([
      "Äpfel",
      "apfelmus",
      "Bohnen",
      "Nudeln",
      "Öl",
      "Zucker",
    ]);
  });

  it("does not change the given list", () => {
    const products = [product({ name: "B" }), product({ name: "A" })];
    sortByName(products);
    expect(products.map((p) => p.name)).toEqual(["B", "A"]);
  });
});

describe("replaceProduct", () => {
  it("replaces the product with the same id and keeps the others", () => {
    const a = product({ id: "a", name: "A", stock: 1 });
    const b = product({ id: "b", name: "B", stock: 2 });
    const c = product({ id: "c", name: "C", stock: 3 });
    const updated = product({ id: "b", name: "B", stock: 1 });

    const result = replaceProduct([a, b, c], updated);

    expect(result).toEqual([a, updated, c]);
    expect(result?.[0]).toBe(a);
    expect(result?.[1]).toBe(updated);
    expect(result?.[2]).toBe(c);
  });

  it("keeps the list unchanged for an unknown id", () => {
    const a = product({ id: "a" });
    expect(replaceProduct([a], product({ id: "x" }))).toEqual([a]);
  });

  it("returns undefined without a cached list", () => {
    expect(replaceProduct(undefined, product({ id: "a" }))).toBeUndefined();
  });
});
