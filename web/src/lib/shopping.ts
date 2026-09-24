import type { components } from "./api/schema";

export type ShoppingItem = components["schemas"]["ShoppingItem"];

/**
 * Returns the quantity shown for item on the shopping list: missing, or
 * null for a marked item without a shortfall (missing 0), which is listed
 * by its name alone (ADR-0015).
 */
export function shoppingQuantity(item: ShoppingItem): number | null {
  return item.marked && item.missing <= 0 ? null : item.missing;
}

/**
 * Returns the shopping list as plain text for sharing: one line
 * "3 × Kidneybohnen" per item, or just "Kidneybohnen" for an item without
 * a quantity (see shoppingQuantity), without the brand, in the order of
 * items, joined with "\n". An empty list gives an empty text.
 */
export function shoppingText(items: readonly ShoppingItem[]): string {
  return items
    .map((item) => {
      const quantity = shoppingQuantity(item);
      return quantity === null ? item.name : `${quantity} × ${item.name}`;
    })
    .join("\n");
}
