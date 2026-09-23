/**
 * Validates code and returns its canonical form, with the same rules as
 * Normalize in internal/gtin (architecture.md 7.1). Returns null for an
 * invalid code.
 *
 * Only the digits 0 to 9 are accepted, the length must be 8, 12, 13 or 14
 * and the check digit must be correct. 12-digit codes (UPC-A) get a leading
 * "0", 14-digit codes with a leading "0" lose it; both result in 13 digits.
 * All other valid codes are returned unchanged. 8-digit codes are checked as
 * EAN-8; UPC-E is not supported. Like the server, it does not trim code, so
 * a code with spaces is invalid.
 */
export function normalizeGtin(code: string): string | null {
  if (!/^[0-9]*$/.test(code)) {
    return null;
  }
  if (![8, 12, 13, 14].includes(code.length)) {
    return null;
  }
  if (!validCheckDigit(code)) {
    return null;
  }
  if (code.length === 12) {
    return "0" + code;
  }
  if (code.length === 14 && code[0] === "0") {
    return code.slice(1);
  }
  return code;
}

// Reports whether the last digit of code (at least one digit) matches the
// Modulo 10 check digit of the others. Weights alternate 3 and 1, starting
// with 3 next to the check digit, so leading zeros do not change the result.
function validCheckDigit(code: string): boolean {
  const last = code.length - 1;
  let sum = 0;
  let weight = 3;
  for (let i = last - 1; i >= 0; i--) {
    sum += Number(code[i]) * weight;
    weight = 4 - weight;
  }
  return (10 - (sum % 10)) % 10 === Number(code[last]);
}
