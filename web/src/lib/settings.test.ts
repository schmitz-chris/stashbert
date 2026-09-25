import { describe, expect, it } from "vitest";
import {
  backupCountText,
  formatSize,
  lastBackupText,
  mqttStateText,
  openFoodFactsText,
} from "./settings";

describe("formatSize", () => {
  it.each([
    [0, "0 Byte"],
    [999, "999 Byte"],
    [1000, "1 KB"],
    [128_000, "128 KB"],
    [131_072, "131 KB"],
    [999_499, "999 KB"],
    [1_400_000, "1,4 MB"],
    [1_449_999, "1,4 MB"],
    [1_450_000, "1,5 MB"],
    [12_300_000, "12,3 MB"],
    [2_000_000, "2 MB"],
    [2_100_000_000, "2,1 GB"],
  ])("formats %d bytes as %s", (bytes, text) => {
    expect(formatSize(bytes)).toBe(text);
  });

  it.each([
    [999_500, "1 MB"],
    [999_999, "1 MB"],
    [999_950_000, "1 GB"],
  ])("goes up to the next unit for %d bytes, which would round to 1000", (bytes, text) => {
    expect(formatSize(bytes)).toBe(text);
  });

  it("stays with GB above 1000 GB", () => {
    expect(formatSize(1_234_500_000_000)).toBe("1.234,5 GB");
  });
});

describe("lastBackupText", () => {
  it("formats the time of the newest backup in German", () => {
    expect(lastBackupText("2026-09-25T09:40:12Z", "Europe/Berlin")).toBe("25.09.2026, 11:40");
  });

  it("uses the time zone it gets", () => {
    expect(lastBackupText("2026-09-25T09:40:12Z", "UTC")).toBe("25.09.2026, 09:40");
  });

  it("says Noch keine without a backup", () => {
    expect(lastBackupText(null, "Europe/Berlin")).toBe("Noch keine");
  });
});

describe("backupCountText", () => {
  it.each([
    [3, 14, "3 von höchstens 14"],
    [1, 14, "1 von höchstens 14"],
    [0, 14, "0 von höchstens 14"],
    [14, 14, "14 von höchstens 14"],
  ])("says %d backups of at most %d as %s", (count, keep, text) => {
    expect(backupCountText(count, keep)).toBe(text);
  });
});

describe("mqttStateText", () => {
  it.each([
    ["disabled", "Nicht eingerichtet"],
    ["connecting", "Keine Verbindung zum Broker"],
    ["connected", "Verbunden"],
  ] as const)("names the state %s as %s", (status, text) => {
    expect(mqttStateText(status)).toBe(text);
  });
});

describe("openFoodFactsText", () => {
  it.each([
    [true, "aktiv"],
    [false, "nicht eingerichtet"],
  ])("says whether Open Food Facts is used (%s) as %s", (enabled, text) => {
    expect(openFoodFactsText(enabled)).toBe(text);
  });
});
