import type { components } from "./api/schema";

export type ShoppingItem = components["schemas"]["ShoppingItem"];

/** A shortfall in crates: the crates to buy and the bottles missing. */
export interface CrateQuantity {
  crates: number;
  bottles: number;
}

/**
 * Returns the quantity shown for item on the shopping list: missing, or
 * null for a marked item without a shortfall (missing 0), which is listed
 * by its name alone (ADR-0015).
 */
export function shoppingQuantity(item: ShoppingItem): number | null {
  return item.marked && item.missing <= 0 ? null : item.missing;
}

/**
 * Converts the shortfall of item into crates (ADR-0017): missing divided by
 * crate_size, rounded up, and missing as the bottles. Returns null for an
 * item without a crate size or without a shortfall; such an item keeps the
 * quantity of shoppingQuantity.
 */
export function crateQuantity(item: ShoppingItem): CrateQuantity | null {
  if (item.crate_size === null || item.missing <= 0) {
    return null;
  }
  return {
    crates: Math.ceil(item.missing / item.crate_size),
    bottles: item.missing,
  };
}

/** Returns "1 Kasten" or "N Kästen". */
export function cratesText(crates: number): string {
  return crates === 1 ? "1 Kasten" : `${crates} Kästen`;
}

/** Returns "fehlt 1 Flasche" or "fehlen N Flaschen". */
export function missingBottlesText(bottles: number): string {
  return bottles === 1 ? "fehlt 1 Flasche" : `fehlen ${bottles} Flaschen`;
}

/**
 * Returns the shopping list as plain text for sharing: one line
 * "3 × Kidneybohnen" per item, "1 Kasten Jever Pilsener" for an item that
 * is bought in crates (see crateQuantity), or just "Kidneybohnen" for an
 * item without a quantity (see shoppingQuantity), without the brand, in
 * the order of items, joined with "\n". An empty list gives an empty text.
 */
export function shoppingText(items: readonly ShoppingItem[]): string {
  return items
    .map((item) => {
      const crates = crateQuantity(item);
      if (crates !== null) {
        return `${cratesText(crates.crates)} ${item.name}`;
      }
      const quantity = shoppingQuantity(item);
      return quantity === null ? item.name : `${quantity} × ${item.name}`;
    })
    .join("\n");
}
