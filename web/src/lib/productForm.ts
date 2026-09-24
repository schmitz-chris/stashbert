import type { components } from "./api/schema";
import type { Product } from "./products";

export type ProductPatch = components["schemas"]["ProductPatch"];

/** The values of the product form as its inputs hold them. */
export interface ProductForm {
  name: string;
  brand: string;
  package_size: string;
  /** The text of the number input for the target. */
  target: string;
  /** The text of the number input for the crate size; "" for no crate. */
  crate_size: string;
  note: string;
}

/** The fields of a product that the form edits. */
export type ProductFormFields = Pick<
  Product,
  "name" | "brand" | "package_size" | "target" | "crate_size" | "note"
>;

/**
 * Returns the form values for product. A missing text and a missing crate
 * size become "".
 */
export function toForm(product: ProductFormFields): ProductForm {
  return {
    name: product.name,
    brand: product.brand ?? "",
    package_size: product.package_size ?? "",
    target: String(product.target),
    crate_size: product.crate_size === null ? "" : String(product.crate_size),
    note: product.note ?? "",
  };
}

/**
 * Returns a JSON Merge Patch (architecture.md 6.1) with the fields of form
 * that differ from original. Texts are compared and sent trimmed; an
 * optional text that is empty after trimming becomes null. The target is
 * only compared if its text is a number; the number input checks that it
 * is a whole number from 0 to 100000. The crate size is compared like the
 * target, except that an empty text becomes null (no crate); the number
 * input checks that it is a whole number from 2 to 100. Without changes the
 * patch is empty. An empty name is part of the patch; the form must catch
 * it.
 */
export function diffPatch(
  original: ProductFormFields,
  form: ProductForm,
): ProductPatch {
  const patch: ProductPatch = {};
  const name = form.name.trim();
  if (name !== original.name) {
    patch.name = name;
  }
  for (const field of ["brand", "package_size", "note"] as const) {
    const value = form[field].trim();
    const next = value === "" ? null : value;
    if (next !== original[field]) {
      patch[field] = next;
    }
  }
  const target = form.target.trim() === "" ? NaN : Number(form.target);
  if (Number.isFinite(target) && target !== original.target) {
    patch.target = target;
  }
  const crateText = form.crate_size.trim();
  const crateSize = crateText === "" ? null : Number(crateText);
  if (
    (crateSize === null || Number.isFinite(crateSize)) &&
    crateSize !== original.crate_size
  ) {
    patch.crate_size = crateSize;
  }
  return patch;
}
