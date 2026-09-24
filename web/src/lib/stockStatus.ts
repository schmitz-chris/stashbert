import type { Product } from "./products";

/**
 * The level of the stock of a product in the stock list: empty, short of
 * its threshold (missing > 0), with a target, or without one.
 */
export type StockLevel = "empty" | "missing" | "target" | "untracked";

/** The status of a row of the stock list (docs/plan.md, F26). */
export interface StockStatus {
  /** Decides the color and the symbol in front of the text. */
  level: StockLevel;
  /** The text of the status, like "leer", "fehlt 2", "3 von 4" or "2 da". */
  text: string;
  /** Whether "· vorgemerkt" follows the text. */
  marked: boolean;
}

/**
 * Returns the status of product for its row in the stock list. A stock of
 * 0 gives "leer". Otherwise a shortfall (missing > 0, computed by the
 * server from target and min_stock, architecture.md 5) gives "fehlt N"
 * with N = missing. Otherwise a product with a target gives "N von T"
 * with N = stock and T = target, even when the stock is below the target
 * but not below min_stock; a product without a target gives "N da". A
 * marked product (ADR-0015) gets marked, whatever its level.
 */
export function stockStatus(
  product: Pick<Product, "stock" | "target" | "missing" | "marked">,
): StockStatus {
  const marked = product.marked;
  if (product.stock === 0) {
    return { level: "empty", text: "leer", marked };
  }
  if (product.missing > 0) {
    return { level: "missing", text: `fehlt ${product.missing}`, marked };
  }
  if (product.target > 0) {
    return {
      level: "target",
      text: `${product.stock} von ${product.target}`,
      marked,
    };
  }
  return { level: "untracked", text: `${product.stock} da`, marked };
}
