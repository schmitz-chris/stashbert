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

  it("shows stock and target when the target is met", () => {
    expect(stockStatus(product(4, 4))).toEqual({
      level: "target",
      text: "4 von 4",
    });
  });

  it("shows stock and target above the target", () => {
    expect(stockStatus(product(5, 4))).toEqual({
      level: "target",
      text: "5 von 4",
    });
  });

  it.each([
    [product(2, 4, 2), "2 von 4"],
    [product(3, 4, 2), "3 von 4"],
  ])("shows stock and target without a shortfall when min_stock is met", (fields, text) => {
    const status = stockStatus(fields);
    expect(status).toEqual({ level: "target", text });
    expect(status.text).not.toContain("fehlt");
  });

  it("counts the stock without a target", () => {
    expect(stockStatus(product(3, 0))).toEqual({
      level: "untracked",
      text: "3 da",
    });
  });
});
