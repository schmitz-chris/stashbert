import type { Product } from "./products";

/**
 * The level of a status in the stock list: empty, or short of its
 * threshold (missing > 0).
 */
export type StockLevel = "empty" | "missing";

/** The status of a row of the stock list (docs/plan.md, F26). */
export interface StockStatus {
  /** Decides the color and the symbol in front of the text. */
  level: StockLevel;
  /** The text of the status, "leer" or like "fehlt 2". */
  text: string;
}

/**
 * Returns the status of product for its row in the stock list, or null if
 * there is nothing to do. A stock of 0 gives "leer". Otherwise a shortfall
 * (missing > 0, computed by the server from target and min_stock,
 * architecture.md 5) gives "fehlt N" with N = missing. Any other stock
 * gives null: the stepper of the row already shows the number, and the
 * target is on the product page (user feedback of 2026-09-24). Whether the
 * product is marked (ADR-0015) is not part of the status either; the cart
 * button of the row shows it.
 */
export function stockStatus(
  product: Pick<Product, "stock" | "missing">,
): StockStatus | null {
  if (product.stock === 0) {
    return { level: "empty", text: "leer" };
  }
  if (product.missing > 0) {
    return { level: "missing", text: `fehlt ${product.missing}` };
  }
  return null;
}
