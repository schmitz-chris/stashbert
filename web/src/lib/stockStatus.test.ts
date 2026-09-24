import { describe, expect, it } from "vitest";
import { stockStatus } from "./stockStatus";

// The fields stockStatus reads, with missing computed like the server does
// (architecture.md 5): target - stock when there is a target and the stock
// is below min_stock, or below the target without min_stock; else 0.
function product(
  stock: number,
  target: number,
  minStock: number | null = null,
) {
  const threshold = minStock ?? target;
  const missing = target > 0 && stock < threshold ? target - stock : 0;
  return { stock, target, missing };
}

describe("stockStatus", () => {
  it.each([
    ["without a target", product(0, 0)],
    ["with a target", product(0, 3)],
    ["with min_stock", product(0, 4, 2)],
    ["with min_stock 0", product(0, 4, 0)],
  ])("calls an empty stock leer, %s", (_, fields) => {
    expect(stockStatus(fields)).toEqual({ level: "empty", text: "leer" });
  });

  it("names the shortfall below the target without min_stock", () => {
    expect(stockStatus(product(1, 3))).toEqual({
      level: "missing",
      text: "fehlt 2",
    });
  });

  it("names the shortfall up to the target below min_stock", () => {
    expect(stockStatus(product(1, 6, 2))).toEqual({
      level: "missing",
      text: "fehlt 5",
    });
  });

  it.each([
    ["the target is met", product(4, 4)],
    ["the stock is above the target", product(5, 4)],
    ["min_stock is met below the target", product(2, 4, 2)],
    ["min_stock is met above it", product(3, 4, 2)],
    ["there is no target", product(3, 0)],
  ])("gives no status when %s", (_, fields) => {
    expect(stockStatus(fields)).toBeNull();
  });
});
