import type { Product } from "./products";

/** The origins of products whose data comes from Open Food Facts. */
const openFactsOrigins: ReadonlySet<Product["origin"]> = new Set([
  "openfoodfacts",
  "openbeautyfacts",
  "openpetfoodfacts",
  "openproductsfacts",
]);

/**
 * Decides the source note of product. It returns null if the data of
 * product does not come from Open Food Facts or a sister database
 * (architecture.md 7.2). Otherwise link is the page of the first barcode
 * on world.openfoodfacts.org, or null if product has no barcode.
 */
export function sourceNote(
  product: Pick<Product, "origin" | "barcodes">,
): { link: string | null } | null {
  if (!openFactsOrigins.has(product.origin)) {
    return null;
  }
  if (product.barcodes.length === 0) {
    return { link: null };
  }
  const code = encodeURIComponent(product.barcodes[0].code);
  return { link: `https://world.openfoodfacts.org/product/${code}` };
}
