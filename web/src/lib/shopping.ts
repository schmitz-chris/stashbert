import type { components } from "./api/schema";

export type ShoppingItem = components["schemas"]["ShoppingItem"];

/**
 * Returns the shopping list as plain text for sharing: one line
 * "3 × Kidneybohnen" per item, without the brand, in the order of items,
 * joined with "\n". An empty list gives an empty text.
 */
export function shoppingText(items: readonly ShoppingItem[]): string {
  return items.map((item) => `${item.missing} × ${item.name}`).join("\n");
}
