import { describe, expect, it } from "vitest";
import type { Product } from "./products";
import { sourceNote } from "./sourceNote";

const barcodes: Product["barcodes"] = [
  { code: "3017620422003", units: 1 },
  { code: "3017620425035", units: 1 },
];

describe("sourceNote", () => {
  it.each([
    "openfoodfacts",
    "openbeautyfacts",
    "openpetfoodfacts",
    "openproductsfacts",
  ] as const)("links %s data to the first barcode", (origin) => {
    expect(sourceNote({ origin, barcodes })).toEqual({
      link: "https://world.openfoodfacts.org/product/3017620422003",
    });
  });

  it("shows the note without a link if there is no barcode", () => {
    expect(sourceNote({ origin: "openfoodfacts", barcodes: [] })).toEqual({
      link: null,
    });
  });

  it.each(["manual", "placeholder"] as const)(
    "shows no note for %s",
    (origin) => {
      expect(sourceNote({ origin, barcodes })).toBeNull();
      expect(sourceNote({ origin, barcodes: [] })).toBeNull();
    },
  );
});
