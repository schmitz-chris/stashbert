import { normalizeGtin } from "./gtin";

/**
 * The outcome of checking a code typed in by hand: the canonical code to
 * book, or the error text the input dialog shows instead of booking.
 */
export type ManualCodeResult =
  | { ok: true; code: string }
  | { ok: false; error: string };

/**
 * Checks a code typed in by hand in the scan view (docs/plan.md, F11).
 * All whitespace is removed first, also inside the code (for example
 * "4 006381 333931"), because the API accepts only digits. The rest is
 * checked with normalizeGtin; a valid code is returned in its canonical
 * form, an invalid one (also an empty input) as "Ungültiger Barcode".
 */
export function checkManualCode(input: string): ManualCodeResult {
  const code = normalizeGtin(input.replace(/\s/g, ""));
  if (code === null) {
    return { ok: false, error: "Ungültiger Barcode" };
  }
  return { ok: true, code };
}
