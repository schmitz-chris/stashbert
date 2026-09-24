import type { components } from "./api/schema";
import { normalizeGtin } from "./gtin";

export type Product = components["schemas"]["Product"];
export type Barcode = components["schemas"]["Barcode"];

/** The filters of the stock list: Alle, Nachkaufen, Leer and Prüfen. */
export type ProductFilter = "all" | "restock" | "empty" | "review";

const collator = new Intl.Collator("de");

/** Returns a copy of products, sorted by name in German order. */
export function sortByName(products: readonly Product[]): Product[] {
  return [...products].sort((a, b) => collator.compare(a.name, b.name));
}

// Decomposes text (NFD), removes the combining diacritical marks and
// lowercases it, so "Käse" becomes "kase".
function fold(text: string): string {
  return text
    .normalize("NFD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase();
}

/**
 * Reports whether the name or the brand of product contains query,
 * ignoring case and diacritics. An empty query or one made of spaces
 * matches every product.
 */
export function matches(product: Product, query: string): boolean {
  const needle = fold(query.trim());
  if (needle === "") {
    return true;
  }
  return (
    fold(product.name).includes(needle) ||
    (product.brand !== null && fold(product.brand).includes(needle))
  );
}

/**
 * Returns the line below the name in the stock list: brand and package
 * size joined with " · ", like "Bonduelle · 400 g". A missing part is left
 * out; without both it returns null.
 */
export function brandAndSize(
  product: Pick<Product, "brand" | "package_size">,
): string | null {
  const parts = [product.brand, product.package_size].filter(
    (part): part is string => part !== null && part !== "",
  );
  return parts.length === 0 ? null : parts.join(" · ");
}

/**
 * Returns the products the product with ownId can be merged into: all
 * products except that one which match query (see matches), in their
 * order.
 */
export function mergeCandidates(
  products: readonly Product[],
  ownId: string,
  query: string,
): Product[] {
  return products.filter(
    (product) => product.id !== ownId && matches(product, query),
  );
}

const filterPredicates: Record<ProductFilter, (product: Product) => boolean> =
  {
    all: () => true,
    // A shortfall or a mark puts a product on the shopping list (ADR-0015).
    restock: (product) => product.missing > 0 || product.marked,
    empty: (product) => product.stock === 0,
    review: (product) => product.needs_review,
  };

/** Returns the products that pass filter, in their order. */
export function filterProducts(
  products: readonly Product[],
  filter: ProductFilter,
): Product[] {
  return products.filter(filterPredicates[filter]);
}

/**
 * Returns a copy of products in which the product with the id of
 * product is replaced by product. The other entries stay the same
 * objects. Without a list (nothing cached yet) it returns undefined.
 */
export function replaceProduct(
  products: readonly Product[] | undefined,
  product: Product,
): Product[] | undefined {
  return products?.map((entry) => (entry.id === product.id ? product : entry));
}

/**
 * Returns the product in products that has the barcode code. code is
 * compared in its canonical form (normalizeGtin), in which the API
 * delivers the barcodes of a product, so a UPC-A code finds its EAN-13
 * with a leading "0". Returns undefined for an invalid or unknown code and
 * without a list (nothing cached yet).
 */
export function findByBarcode(
  products: readonly Product[] | undefined,
  code: string,
): Product | undefined {
  const canonical = normalizeGtin(code);
  if (products === undefined || canonical === null) {
    return undefined;
  }
  return products.find((product) =>
    product.barcodes.some((barcode) => barcode.code === canonical),
  );
}

/**
 * Returns a copy of product with barcode added to its barcodes. They stay
 * sorted by code, as the API delivers them; an entry with the same code is
 * replaced.
 */
export function withBarcode(product: Product, barcode: Barcode): Product {
  const barcodes = product.barcodes.filter(
    (entry) => entry.code !== barcode.code,
  );
  barcodes.push(barcode);
  barcodes.sort((a, b) => (a.code < b.code ? -1 : a.code > b.code ? 1 : 0));
  return { ...product, barcodes };
}

/** Returns a copy of product without the barcode with code. */
export function withoutBarcode(product: Product, code: string): Product {
  return {
    ...product,
    barcodes: product.barcodes.filter((entry) => entry.code !== code),
  };
}
