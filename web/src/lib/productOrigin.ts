/**
 * The views a product page can be opened from (docs/plan.md, F22). A link
 * to a product page passes its view as navigation state; the page names it
 * on its back link, and the tab bar marks its tab.
 */
export type ProductOrigin = "vorrat" | "einkauf" | "scan";

/** The navigation state of a link to a product page. */
export interface ProductLinkState {
  from: ProductOrigin;
}

/** The view a product page goes back to: the label of the back link and its path. */
export interface ProductBack {
  label: string;
  path: string;
}

const backs: Record<ProductOrigin, ProductBack> = {
  vorrat: { label: "Vorrat", path: "/vorrat" },
  einkauf: { label: "Einkauf", path: "/einkauf" },
  scan: { label: "Scan", path: "/scan" },
};

function isOrigin(value: unknown): value is ProductOrigin {
  return value === "vorrat" || value === "einkauf" || value === "scan";
}

/** Returns the navigation state of a link from the view from to a product page. */
export function productLinkState(from: ProductOrigin): ProductLinkState {
  return { from };
}

/**
 * Returns the view the product page with the navigation state state goes
 * back to. Without a known origin (the page was loaded directly, or the
 * state is not one of productLinkState) it is Vorrat.
 */
export function productBack(state: unknown): ProductBack {
  if (typeof state === "object" && state !== null && "from" in state && isOrigin(state.from)) {
    return backs[state.from];
  }
  return backs.vorrat;
}
