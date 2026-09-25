/**
 * The views with a title of their own, by the ids of their routes in
 * router.tsx.
 */
export type TitledRoute = "stock" | "scan" | "shopping" | "product" | "settings" | "not-found";

const appName = "StashBert";

const names: Record<Exclude<TitledRoute, "product">, string> = {
  stock: "Vorrat",
  scan: "Scannen",
  shopping: "Einkauf",
  settings: "Einstellungen",
  "not-found": "Nicht gefunden",
};

/**
 * Returns the document title of the view of route (docs/plan.md, F24):
 * the name of the view and the name of the app, like "Vorrat · StashBert".
 * The product page is named after its product; as long as the name is not
 * known (the product is loading or could not be loaded), the title is the
 * name of the app alone.
 */
export function pageTitle(route: TitledRoute, productName?: string): string {
  const name = route === "product" ? productName : names[route];
  return name === undefined || name === "" ? appName : `${name} · ${appName}`;
}
