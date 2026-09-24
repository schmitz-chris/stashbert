import type { Product } from "./products";

/** What a tap on the cart button of a stock row does. */
export type CartAction = "mark" | "unmark";

/** The state of the cart button of a stock row (docs/plan.md, F18). */
export interface CartToggle {
  /** Whether the button is shown as pressed (aria-pressed). */
  pressed: boolean;
  /** The accessible name of the button (aria-label). */
  label: string;
  /** What a tap does: mark the product for shopping or remove its mark. */
  action: CartAction;
}

/**
 * Returns the state of the cart button for product. The button shows the
 * mark alone (ADR-0015): a marked product gives a pressed button whose tap
 * removes the mark, any other product a button whose tap marks it. A
 * shortfall (missing > 0) does not change the button.
 */
export function cartToggle(product: Pick<Product, "name" | "marked">): CartToggle {
  if (product.marked) {
    return {
      pressed: true,
      label: `Von der Einkaufsliste nehmen: ${product.name}`,
      action: "unmark",
    };
  }
  return {
    pressed: false,
    label: `Auf die Einkaufsliste: ${product.name}`,
    action: "mark",
  };
}
