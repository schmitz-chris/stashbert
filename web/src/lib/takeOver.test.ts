import { describe, expect, it } from "vitest";
import type { Product } from "./products";
import {
  mergeOneByOne,
  selectedProducts,
  takeOverButtonLabel,
  takeOverCandidates,
  takeOverExplanation,
  takeOverNotice,
  takeOverQuestion,
  toggleSelected,
} from "./takeOver";

function product(id: string, name: string, brand: string | null = null): Product {
  return {
    id,
    name,
    brand,
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
    crate_size: null,
    barcodes: [],
    created_at: "2026-09-25T10:00:00.000Z",
    updated_at: "2026-09-25T10:00:00.000Z",
  };
}

const milk = product("m", "Milch");
const aldi = product("a", "Frische Milch", "Aldi");
const rewe = product("r", "Bio Milch", "Rewe");
const bread = product("b", "Brot");
// In the order of the product list (sorted by name).
const products = [rewe, bread, aldi, milk];

describe("toggleSelected", () => {
  it("adds an id at the end", () => {
    expect(toggleSelected(["a"], "r")).toEqual(["a", "r"]);
  });

  it("removes a selected id", () => {
    expect(toggleSelected(["a", "r", "b"], "r")).toEqual(["a", "b"]);
  });

  it("selects the first id", () => {
    expect(toggleSelected([], "a")).toEqual(["a"]);
  });

  it("does not change the given list", () => {
    const selected = ["a"];
    toggleSelected(selected, "r");
    toggleSelected(selected, "a");
    expect(selected).toEqual(["a"]);
  });
});

describe("takeOverCandidates", () => {
  it("lists all other products without a query", () => {
    expect(takeOverCandidates(products, "m", "", [])).toEqual([rewe, bread, aldi]);
  });

  it("filters by name and brand", () => {
    expect(takeOverCandidates(products, "m", "milch", [])).toEqual([rewe, aldi]);
    expect(takeOverCandidates(products, "m", "aldi", [])).toEqual([aldi]);
  });

  it("keeps selected products that do not match, in list order", () => {
    expect(takeOverCandidates(products, "m", "brot", ["a"])).toEqual([bread, aldi]);
  });

  it("never lists the own product, even when it is selected", () => {
    expect(takeOverCandidates(products, "m", "milch", ["m"])).toEqual([rewe, aldi]);
  });

  it("finds nothing for an unknown query", () => {
    expect(takeOverCandidates(products, "m", "käse", [])).toEqual([]);
  });
});

describe("selectedProducts", () => {
  it("returns the selected products in list order", () => {
    expect(selectedProducts(products, "m", ["a", "r"])).toEqual([rewe, aldi]);
  });

  it("leaves out the own product and products that are gone", () => {
    expect(selectedProducts(products, "m", ["m", "gone", "b"])).toEqual([bread]);
  });

  it("returns nothing without a selection", () => {
    expect(selectedProducts(products, "m", [])).toEqual([]);
  });
});

describe("texts", () => {
  it.each([
    [0, "Übernehmen (0)"],
    [1, "Übernehmen (1)"],
    [3, "Übernehmen (3)"],
  ])("labels the button for %i", (count, label) => {
    expect(takeOverButtonLabel(count)).toBe(label);
  });

  it("asks in the plural", () => {
    expect(takeOverQuestion(2, "Milch")).toBe("2 Produkte in „Milch“ übernehmen?");
    expect(takeOverExplanation(2)).toBe(
      "Ihre Barcodes, Bestände und Verläufe gehen auf dieses Produkt über, die Produkte selbst verschwinden.",
    );
  });

  it("asks in the singular", () => {
    expect(takeOverQuestion(1, "Milch")).toBe("1 Produkt in „Milch“ übernehmen?");
    expect(takeOverExplanation(1)).toBe(
      "Seine Barcodes, sein Bestand und sein Verlauf gehen auf dieses Produkt über, das gewählte Produkt verschwindet.",
    );
  });
});

describe("takeOverNotice", () => {
  it("counts the products after a success", () => {
    expect(takeOverNotice({ merged: [rewe, aldi, bread], failed: null, total: 3 })).toEqual({
      text: "3 Produkte übernommen",
      failed: false,
    });
    expect(takeOverNotice({ merged: [aldi], failed: null, total: 1 })).toEqual({
      text: "1 Produkt übernommen",
      failed: false,
    });
  });

  it("names the merged products and the failed one", () => {
    expect(takeOverNotice({ merged: [rewe, aldi], failed: bread, total: 3 })).toEqual({
      text: "2 von 3 übernommen: „Bio Milch“, „Frische Milch“. „Brot“ konnte nicht übernommen werden.",
      failed: true,
    });
  });

  it("names only the failed product when the first merge failed", () => {
    expect(takeOverNotice({ merged: [], failed: rewe, total: 2 })).toEqual({
      text: "„Bio Milch“ konnte nicht übernommen werden.",
      failed: true,
    });
  });
});

describe("mergeOneByOne", () => {
  it("merges all sources in their order, one after another", async () => {
    const log: string[] = [];
    let running = 0;
    const outcome = await mergeOneByOne([rewe, aldi, bread], async (source) => {
      running++;
      log.push(`start ${source.id} (${running} running)`);
      await Promise.resolve();
      await Promise.resolve();
      log.push(`end ${source.id}`);
      running--;
    });
    expect(log).toEqual([
      "start r (1 running)",
      "end r",
      "start a (1 running)",
      "end a",
      "start b (1 running)",
      "end b",
    ]);
    expect(outcome).toEqual({ merged: [rewe, aldi, bread], failed: null, total: 3 });
  });

  it("stops at the first error and does not try the rest", async () => {
    const tried: string[] = [];
    const outcome = await mergeOneByOne([rewe, aldi, bread], (source) => {
      tried.push(source.id);
      return source === aldi
        ? Promise.reject(new Error("not_found"))
        : Promise.resolve(milk);
    });
    expect(tried).toEqual(["r", "a"]);
    expect(outcome).toEqual({ merged: [rewe], failed: aldi, total: 3 });
  });

  it("reports a failure of the first merge", async () => {
    const outcome = await mergeOneByOne([rewe, aldi], () =>
      Promise.reject(new Error("offline")),
    );
    expect(outcome).toEqual({ merged: [], failed: rewe, total: 2 });
  });

  it("does nothing without sources", async () => {
    const outcome = await mergeOneByOne([], () => Promise.reject(new Error("never")));
    expect(outcome).toEqual({ merged: [], failed: null, total: 0 });
  });
});
