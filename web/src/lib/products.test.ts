import { describe, expect, it } from "vitest";
import {
  filterProducts,
  matches,
  mergeCandidates,
  replaceProduct,
  sortByName,
  withBarcode,
  withoutBarcode,
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
    marked: false,
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

describe("mergeCandidates", () => {
  const own = product({ id: "own", name: "Neues Produkt 2000000000008" });
  const cheese = product({ id: "a", name: "Käse", brand: "Allgäuer Hof" });
  const beans = product({ id: "b", name: "Kidneybohnen", brand: "Bonduelle" });
  const flour = product({ id: "c", name: "Mehl" });
  const products = [cheese, own, beans, flour];

  it("excludes the own product and keeps the others in order", () => {
    expect(mergeCandidates(products, "own", "")).toEqual([
      cheese,
      beans,
      flour,
    ]);
  });

  it("excludes the own product even if it matches", () => {
    expect(mergeCandidates(products, "own", "neues")).toEqual([]);
  });

  it("filters with matches", () => {
    expect(mergeCandidates(products, "own", "kase")).toEqual([cheese]);
    expect(mergeCandidates(products, "own", "BONDU")).toEqual([beans]);
    expect(mergeCandidates(products, "own", "milch")).toEqual([]);
  });

  it("does not change the given list", () => {
    mergeCandidates(products, "own", "mehl");
    expect(products).toEqual([cheese, own, beans, flour]);
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

describe("withBarcode", () => {
  const nutella = { code: "3017620422003", units: 1 };
  const local = { code: "2212345678907", units: 1 };
  const knorr = { code: "4000400130150", units: 1 };

  it("adds the barcode sorted by code", () => {
    const original = product({ barcodes: [local, knorr] });

    const result = withBarcode(original, nutella);

    expect(result.barcodes).toEqual([local, nutella, knorr]);
    expect(original.barcodes).toEqual([local, knorr]);
  });

  it("adds the first barcode", () => {
    expect(withBarcode(product({}), nutella).barcodes).toEqual([nutella]);
  });

  it("replaces a barcode with the same code", () => {
    const original = product({ barcodes: [nutella, knorr] });
    const result = withBarcode(original, { code: nutella.code, units: 6 });
    expect(result.barcodes).toEqual([{ code: nutella.code, units: 6 }, knorr]);
  });

  it("keeps the other fields", () => {
    const original = product({ id: "a", name: "Nutella", stock: 2 });
    expect(withBarcode(original, nutella)).toEqual({
      ...original,
      barcodes: [nutella],
    });
  });
});

describe("withoutBarcode", () => {
  const nutella = { code: "3017620422003", units: 1 };
  const knorr = { code: "4000400130150", units: 1 };

  it("removes the barcode with the code", () => {
    const original = product({ barcodes: [nutella, knorr] });

    const result = withoutBarcode(original, nutella.code);

    expect(result).toEqual({ ...original, barcodes: [knorr] });
    expect(original.barcodes).toEqual([nutella, knorr]);
  });

  it("keeps the barcodes for an unknown code", () => {
    const original = product({ barcodes: [nutella] });
    expect(withoutBarcode(original, "20004002").barcodes).toEqual([nutella]);
  });
});
