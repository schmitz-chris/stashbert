import type { components } from "./api/schema";

export type Product = components["schemas"]["Product"];

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

const filterPredicates: Record<ProductFilter, (product: Product) => boolean> =
  {
    all: () => true,
    restock: (product) => product.missing > 0,
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
