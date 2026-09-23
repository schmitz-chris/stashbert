import { describe, expect, it } from "vitest";
import { normalizeGtin } from "./gtin";

// The same examples as the tests of internal/gtin (B06). Check digits:
// Modulo 10 with weights 3 and 1, starting with 3 at the digit left of the
// check digit. Check digit = (10 - sum mod 10) mod 10.

describe("normalizeGtin", () => {
  it.each([
    ["EAN-13", "3017620422003", "3017620422003"],
    ["EAN-13", "4001686301265", "4001686301265"],
    ["UPC-A gets leading zero", "034000470693", "0034000470693"],
    ["GTIN-14 loses leading zero", "00034000470693", "0034000470693"],
    ["EAN-8", "20004002", "20004002"],
    // Prefix 22, body 221234567890, digits from the right with weights
    // 3,1,3,1,...: 0*3 + 9*1 + 8*3 + 7*1 + 6*3 + 5*1 + 4*3 + 3*1 + 2*3
    // + 1*1 + 2*3 + 2*1 = 93, check digit = (10 - 93 mod 10) mod 10 = 7.
    ["EAN-13 prefix 22", "2212345678907", "2212345678907"],
    // Prefix 29, body 290000000000: 9*3 + 2*1 = 29, check digit 1.
    ["EAN-13 prefix 29", "2900000000001", "2900000000001"],
    // UPC-A number system 2, body 21234567890: 0*3 + 9*1 + 8*3 + 7*1
    // + 6*3 + 5*1 + 4*3 + 3*1 + 2*3 + 1*1 + 2*3 = 91, check digit 9.
    ["UPC-A prefix 02", "212345678909", "0212345678909"],
    // Indicator 1 in front of the body of 3017620422003 (sum 57) at
    // weight 3: 57 + 1*3 = 60, check digit 0.
    ["GTIN-14 without leading zero", "13017620422000", "13017620422000"],
  ])("%s %s becomes %s", (_name, code, want) => {
    expect(normalizeGtin(code)).toBe(want);
    // Normalizing the stored form again leaves it unchanged.
    expect(normalizeGtin(want)).toBe(want);
  });

  it.each([
    // Body 400040013015: 5*3 + 1*1 + 0*3 + 3*1 + 1*3 + 0*1 + 0*3 + 4*1
    // + 0*3 + 0*1 + 0*3 + 4*1 = 30, so the check digit must be 0, not 7.
    ["wrong check digit", "4000400130157"],
    ["letters", "abcdefghijklm"],
    ["letter in digits", "301762042200A"],
    ["10 digits", "3017620422"],
    ["empty", ""],
    ["7 digits", "2000400"],
    ["15 digits", "030176204220030"],
    ["space", " 3017620422003"],
    ["trailing space", "3017620422003 "],
    ["non-ASCII digits", "３０１７６２０４２２００３"],
  ])("rejects %s", (_name, code) => {
    expect(normalizeGtin(code)).toBeNull();
  });
});
