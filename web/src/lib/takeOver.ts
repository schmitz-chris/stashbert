import { matches, type Product } from "./products";

// "Andere Produkte hierher übernehmen" (docs/plan.md, F36): the user picks
// several other products, and each is merged into the product of the page.

/**
 * Returns selected with id added at the end, or without id when it is
 * selected already.
 */
export function toggleSelected(selected: readonly string[], id: string): string[] {
  return selected.includes(id)
    ? selected.filter((entry) => entry !== id)
    : [...selected, id];
}

/**
 * Returns the products the user can take over into the product with
 * ownId, in their order: all others that match query (see matches), and
 * the selected ones even if they do not match, so a search never hides a
 * selection.
 */
export function takeOverCandidates(
  products: readonly Product[],
  ownId: string,
  query: string,
  selected: readonly string[],
): Product[] {
  return products.filter(
    (product) =>
      product.id !== ownId &&
      (selected.includes(product.id) || matches(product, query)),
  );
}

/**
 * Returns the selected products in the order of products, without the
 * product with ownId. A selected id that is not in products (the product
 * is gone) is left out.
 */
export function selectedProducts(
  products: readonly Product[],
  ownId: string,
  selected: readonly string[],
): Product[] {
  return products.filter(
    (product) => product.id !== ownId && selected.includes(product.id),
  );
}

// "1 Produkt" or "3 Produkte".
function productCount(count: number): string {
  return count === 1 ? "1 Produkt" : `${count} Produkte`;
}

/** Returns the title of the button at the end of the sheet. */
export function takeOverButtonLabel(count: number): string {
  return `Übernehmen (${count})`;
}

/** Returns the question of the confirmation. */
export function takeOverQuestion(count: number, targetName: string): string {
  return `${productCount(count)} in „${targetName}“ übernehmen?`;
}

/** Returns what the confirmation explains below its question. */
export function takeOverExplanation(count: number): string {
  return count === 1
    ? "Seine Barcodes, sein Bestand und sein Verlauf gehen auf dieses Produkt über, das gewählte Produkt verschwindet."
    : "Ihre Barcodes, Bestände und Verläufe gehen auf dieses Produkt über, die Produkte selbst verschwinden.";
}

/** What happened to the products the user chose to take over. */
export interface TakeOverOutcome {
  /** The products merged into the target, in the order they were tried. */
  merged: Product[];
  /** The product whose merge failed; the ones after it were not tried. */
  failed: Product | null;
  /** How many products the user chose. */
  total: number;
}

/**
 * Merges sources one after another with merge, which gets one source and
 * settles when its merge is done. The next merge only starts when the
 * last one succeeded; at the first error it stops. It never rejects.
 */
export async function mergeOneByOne(
  sources: readonly Product[],
  merge: (source: Product) => Promise<unknown>,
): Promise<TakeOverOutcome> {
  const merged: Product[] = [];
  for (const source of sources) {
    try {
      await merge(source);
    } catch {
      return { merged, failed: source, total: sources.length };
    }
    merged.push(source);
  }
  return { merged, failed: null, total: sources.length };
}

/** The message on the product page after taking over. */
export interface TakeOverNotice {
  text: string;
  failed: boolean;
}

/**
 * Returns the message for outcome: how many products were taken over or,
 * after an error, which ones were and which one failed.
 */
export function takeOverNotice({ merged, failed, total }: TakeOverOutcome): TakeOverNotice {
  if (failed === null) {
    return { text: `${productCount(merged.length)} übernommen`, failed: false };
  }
  const reason = `„${failed.name}“ konnte nicht übernommen werden.`;
  if (merged.length === 0) {
    return { text: reason, failed: true };
  }
  const names = merged.map((product) => `„${product.name}“`).join(", ");
  return {
    text: `${merged.length} von ${total} übernommen: ${names}. ${reason}`,
    failed: true,
  };
}
