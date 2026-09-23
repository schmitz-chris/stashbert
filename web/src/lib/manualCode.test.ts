import { describe, expect, it } from "vitest";
import { checkManualCode } from "./manualCode";

const invalid = { ok: false, error: "Ungültiger Barcode" };

describe("checkManualCode", () => {
  it.each([
    ["EAN-13", "4006381333931", "4006381333931"],
    ["spaces inside", "4 006381 333931", "4006381333931"],
    ["spaces outside", "  4006381333931 ", "4006381333931"],
    ["spaces inside and outside", " 4 006381 333931  ", "4006381333931"],
    ["tab and non-breaking space", "\t4006381\u00a0333931", "4006381333931"],
    ["UPC-A gets leading zero", "034000470693", "0034000470693"],
    ["UPC-A with spaces", "0 34000 47069 3", "0034000470693"],
    ["EAN-8", "20004002", "20004002"],
  ])("accepts %s: %j becomes %s", (_name, input, code) => {
    expect(checkManualCode(input)).toEqual({ ok: true, code });
  });

  it.each([
    // 4006381333931 is valid, so the check digit 2 is wrong.
    ["wrong check digit", "4006381333932"],
    ["wrong check digit with spaces", "4 006381 333932"],
    ["letters", "abcdefghijklm"],
    ["letter in digits", "400638133393A"],
    ["empty", ""],
    ["only spaces", "   "],
    ["hyphen", "4006381-333931"],
    ["too short", "400638133"],
  ])("rejects %s: %j", (_name, input) => {
    expect(checkManualCode(input)).toEqual(invalid);
  });
});
