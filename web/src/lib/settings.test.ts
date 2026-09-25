import { afterEach, describe, expect, it, vi } from "vitest";
import {
  backupCountText,
  formatSize,
  lastBackupText,
  mqttStateText,
  openFoodFactsText,
  restoreErrorText,
  restoreNotice,
  restoreQuestionText,
  waitForRestart,
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

  it("says Noch keines without a backup", () => {
    expect(lastBackupText(null, "Europe/Berlin")).toBe("Noch keines");
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

describe("restoreQuestionText", () => {
  it("names the picked file in the question", () => {
    expect(restoreQuestionText("stashbert-20260925-114000.tar.gz")).toBe(
      "Alle Produkte, Bestände und Bilder werden durch das Backup " +
        "„stashbert-20260925-114000.tar.gz“ ersetzt. " +
        "Der jetzige Stand bleibt auf dem Server als Kopie erhalten.",
    );
  });
});

// Problem Details (RFC 9457) with the given code, as the server sends them.
function problem(status: number, code: string) {
  return { type: "about:blank", title: code, status, detail: code, code };
}

describe("restoreErrorText", () => {
  it.each([
    [problem(422, "invalid_backup"), "Das ist kein Backup von StashBert."],
    [
      problem(422, "backup_too_new"),
      "Das Backup stammt von einer neueren Version. Bitte zuerst StashBert aktualisieren.",
    ],
    [problem(413, "backup_too_large"), "Das Backup ist zu groß (höchstens 1 GB)."],
    [problem(409, "restore_in_progress"), "Es wird gerade schon ein Backup eingespielt."],
  ])("names the problem %o", (error, text) => {
    expect(restoreErrorText(error)).toBe(text);
  });

  it.each([
    ["another code", problem(400, "invalid_request")],
    ["a server error", problem(500, "internal")],
    ["a code of Object", problem(400, "constructor")],
    ["a network error", new TypeError("Load failed")],
    ["an error without Problem Details", new Error("POST /backup/restore: status 502")],
    ["nothing", undefined],
  ])("says Einspielen fehlgeschlagen for %s", (_, error) => {
    expect(restoreErrorText(error)).toBe("Einspielen fehlgeschlagen");
  });
});

describe("restoreNotice", () => {
  it("says nothing before the first try", () => {
    expect(restoreNotice("idle", null, "waiting").text).toBe("");
  });

  it("says Backup wird eingespielt during the upload", () => {
    expect(restoreNotice("pending", null, "waiting")).toEqual({
      text: "Backup wird eingespielt …",
      tone: "progress",
    });
  });

  it("says Backup wird eingespielt while StashBert restarts", () => {
    expect(restoreNotice("success", null, "waiting")).toEqual({
      text: "Backup wird eingespielt …",
      tone: "progress",
    });
  });

  it("says Backup eingespielt when StashBert is back", () => {
    expect(restoreNotice("success", null, "answered")).toEqual({
      text: "Backup eingespielt",
      tone: "success",
    });
  });

  it("says that StashBert does not answer after 60 s", () => {
    expect(restoreNotice("success", null, "silent")).toEqual({
      text: "StashBert antwortet nicht. Bitte die Seite später neu laden.",
      tone: "failure",
    });
  });

  it("names a refused backup", () => {
    expect(restoreNotice("error", problem(422, "invalid_backup"), "waiting")).toEqual({
      text: "Das ist kein Backup von StashBert.",
      tone: "failure",
    });
  });

  it("says Einspielen fehlgeschlagen for a failed upload", () => {
    expect(restoreNotice("error", new TypeError("Load failed"), "waiting")).toEqual({
      text: "Einspielen fehlgeschlagen",
      tone: "failure",
    });
  });
});

describe("waitForRestart", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  // An ask that answers with the given results in turn: true or false, or
  // an Error to throw, like fetch while StashBert restarts.
  function answers(...results: (boolean | Error)[]) {
    return vi.fn(async (signal: AbortSignal) => {
      expect(signal).toBeInstanceOf(AbortSignal);
      const result = results.shift() ?? false;
      if (result instanceof Error) {
        throw result;
      }
      return result;
    });
  }

  it("asks every 500 ms and resolves with true when StashBert answers", async () => {
    vi.useFakeTimers();
    const ask = answers(false, false, true);
    const done = vi.fn();
    void waitForRestart(ask, new AbortController().signal).then(done);

    await vi.advanceTimersByTimeAsync(499);
    expect(ask).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(ask).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1000);
    expect(ask).toHaveBeenCalledTimes(3);
    expect(done).toHaveBeenCalledWith(true);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("keeps asking after network errors", async () => {
    vi.useFakeTimers();
    const ask = answers(new TypeError("Load failed"), new TypeError("Load failed"), true);
    const result = waitForRestart(ask, new AbortController().signal);

    await vi.advanceTimersByTimeAsync(1500);
    await expect(result).resolves.toBe(true);
    expect(ask).toHaveBeenCalledTimes(3);
  });

  it("resolves with false when StashBert does not answer within 60 s", async () => {
    vi.useFakeTimers();
    const ask = answers();
    const done = vi.fn();
    void waitForRestart(ask, new AbortController().signal).then(done);

    await vi.advanceTimersByTimeAsync(59_999);
    expect(done).not.toHaveBeenCalled();
    expect(ask).toHaveBeenCalledTimes(119);
    await vi.advanceTimersByTimeAsync(1);
    expect(done).toHaveBeenCalledWith(false);
    expect(ask).toHaveBeenCalledTimes(119);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("aborts a request that hangs when the 60 s are over", async () => {
    vi.useFakeTimers();
    // Like fetch: rejects only when its signal aborts.
    const ask = vi.fn(
      (signal: AbortSignal) =>
        new Promise<boolean>((_, reject) => {
          signal.addEventListener("abort", () => reject(signal.reason));
        }),
    );
    const done = vi.fn();
    void waitForRestart(ask, new AbortController().signal).then(done);

    await vi.advanceTimersByTimeAsync(59_999);
    expect(ask).toHaveBeenCalledTimes(1);
    expect(done).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(done).toHaveBeenCalledWith(false);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("stops at once when its signal aborts", async () => {
    vi.useFakeTimers();
    const ask = answers();
    const controller = new AbortController();
    const done = vi.fn();
    void waitForRestart(ask, controller.signal).then(done);

    await vi.advanceTimersByTimeAsync(1200);
    expect(ask).toHaveBeenCalledTimes(2);
    controller.abort();
    await vi.advanceTimersByTimeAsync(0);
    expect(done).toHaveBeenCalledWith(false);
    expect(vi.getTimerCount()).toBe(0);
    await vi.advanceTimersByTimeAsync(5000);
    expect(ask).toHaveBeenCalledTimes(2);
  });
});
