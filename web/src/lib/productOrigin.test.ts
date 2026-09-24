import { describe, expect, it } from "vitest";
import { productBack, productLinkState } from "./productOrigin";

describe("productBack", () => {
  it.each([
    ["vorrat", "Vorrat", "/vorrat"],
    ["einkauf", "Einkauf", "/einkauf"],
    ["scan", "Scan", "/scan"],
  ] as const)("goes back to %s when opened from there", (from, label, path) => {
    expect(productBack(productLinkState(from))).toEqual({ label, path });
  });

  it.each([
    ["no state (loaded directly)", undefined],
    ["null", null],
    ["a string", "einkauf"],
    ["an object without from", {}],
    ["an unknown view", { from: "produkt" }],
    ["a view that is no string", { from: 1 }],
  ])("falls back to Vorrat with %s", (_, state) => {
    expect(productBack(state)).toEqual({ label: "Vorrat", path: "/vorrat" });
  });
});
