import { describe, expect, it } from "vitest";
import {
  formatDelta,
  formatMovementTime,
  movementKindLabel,
  visibleMovements,
} from "./movements";

describe("movementKindLabel", () => {
  it.each([
    ["add", "Eingelagert"],
    ["consume", "Entnommen"],
    ["inventory", "Inventur"],
    ["reversal", "Storno"],
    ["merge", "Zusammengeführt"],
  ] as const)("labels %s as %s", (kind, label) => {
    expect(movementKindLabel(kind)).toBe(label);
  });
});

describe("formatDelta", () => {
  it.each([
    [2, "+2"],
    [1, "+1"],
    [0, "0"],
    [-1, "−1"],
    [-12, "−12"],
  ])("formats %d as %s", (delta, text) => {
    expect(formatDelta(delta)).toBe(text);
  });

  it("uses the minus sign, not a hyphen", () => {
    expect(formatDelta(-3)).not.toContain("-");
  });
});

describe("formatMovementTime", () => {
  it("formats date and time in German", () => {
    expect(formatMovementTime("2026-09-23T12:05:00.000Z", "UTC")).toBe(
      "23.09.2026, 12:05",
    );
  });

  it("converts to the given time zone", () => {
    // Summer time: UTC+2.
    expect(
      formatMovementTime("2026-09-23T12:05:00.000Z", "Europe/Berlin"),
    ).toBe("23.09.2026, 14:05");
    // Winter time, across midnight: UTC+1.
    expect(
      formatMovementTime("2026-12-31T23:30:00Z", "Europe/Berlin"),
    ).toBe("01.01.2027, 00:30");
  });

  it("pads day, month, hour and minute to two digits", () => {
    expect(formatMovementTime("2026-03-04T05:06:07.123Z", "UTC")).toBe(
      "04.03.2026, 05:06",
    );
  });
});

describe("visibleMovements", () => {
  const ten = Array.from({ length: 10 }, (_, i) => i);

  it("shows the first three while collapsed", () => {
    expect(visibleMovements(ten, false)).toEqual([0, 1, 2]);
  });

  it("shows all of them once expanded", () => {
    expect(visibleMovements(ten, true)).toEqual(ten);
  });

  it("shows fewer than three as they are", () => {
    expect(visibleMovements([0, 1], false)).toEqual([0, 1]);
    expect(visibleMovements([], false)).toEqual([]);
  });
});
