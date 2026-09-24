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
}

/**
 * Returns the status of product for its row in the stock list. A stock of
 * 0 gives "leer". Otherwise a shortfall (missing > 0, computed by the
 * server from target and min_stock, architecture.md 5) gives "fehlt N"
 * with N = missing. Otherwise a product with a target gives "N von T"
 * with N = stock and T = target, even when the stock is below the target
 * but not below min_stock; a product without a target gives "N da".
 * Whether the product is marked (ADR-0015) is not part of the status; the
 * cart button of the row shows it.
 */
export function stockStatus(
  product: Pick<Product, "stock" | "target" | "missing">,
): StockStatus {
  if (product.stock === 0) {
    return { level: "empty", text: "leer" };
  }
  if (product.missing > 0) {
    return { level: "missing", text: `fehlt ${product.missing}` };
  }
  if (product.target > 0) {
    return { level: "target", text: `${product.stock} von ${product.target}` };
  }
  return { level: "untracked", text: `${product.stock} da` };
}
