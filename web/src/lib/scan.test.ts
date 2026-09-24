import { afterEach, describe, expect, it, vi } from "vitest";
import type { Product } from "./products";
import {
  cameraErrorText,
  feedbackFor,
  feedbackIcon,
  loadScanMode,
  saveScanMode,
  undoFeedbackFor,
  unmarkFeedbackFor,
  type MarkResult,
  type MovementResult,
} from "./scan";

const product: Product = {
  id: "01a0ce63-0000-7000-8000-000000000000",
  name: "Kidneybohnen",
  brand: null,
  package_size: null,
  note: null,
  stock: 2,
  target: 3,
  min_stock: null,
  missing: 1,
  marked: false,
  needs_review: false,
  origin: "manual",
  lookup_state: "none",
  has_image: false,
  crate_size: null,
  barcodes: [{ code: "4001234567890", units: 1 }],
  created_at: "2026-09-23T10:00:00.000Z",
  updated_at: "2026-09-23T10:00:00.000Z",
};

function result(
  kind: "add" | "consume",
  fields: Partial<MovementResult> = {},
): MovementResult {
  return {
    movement: {
      id: "01a0ce63-0000-7000-8000-000000000001",
      product_id: product.id,
      kind,
      delta: kind === "add" ? 1 : -1,
      stock_after: product.stock,
      barcode: "4001234567890",
      reverses_id: null,
      created_at: "2026-09-23T10:00:00.000Z",
    },
    product,
    product_created: false,
    warnings: [],
    message: kind === "add" ? "Kidneybohnen 1 → 2" : "Kidneybohnen 3 → 2",
    ...fields,
  };
}

function problem(status: number, code: string) {
  return { type: "about:blank", title: "Fehler", status, detail: "…", code };
}

// The error scanMovementMutation throws for a response without Problem
// Details, for example from a reverse proxy.
function statusError(status: number) {
  return Object.assign(new Error(`POST /movements: status ${status}`), { status });
}

// Replaces localStorage with a store in a Map that holds saved.
function stubStorage(saved: Record<string, string> = {}) {
  const items = new Map(Object.entries(saved));
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => items.set(key, value),
  });
}

describe("loadScanMode", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each(["add", "consume", "mark"] as const)("returns the saved mode %s", (mode) => {
    stubStorage({ "stashbert.scanMode": mode });
    expect(loadScanMode()).toBe(mode);
  });

  it.each(["", "Einkaufen", "reversal", "MARK"])(
    "returns add for the unknown saved value %o",
    (value) => {
      stubStorage({ "stashbert.scanMode": value });
      expect(loadScanMode()).toBe("add");
    },
  );

  it("returns add if no mode is saved", () => {
    stubStorage();
    expect(loadScanMode()).toBe("add");
  });

  it("returns add if the storage is unavailable", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => {
        throw new DOMException("", "SecurityError");
      },
    });
    expect(loadScanMode()).toBe("add");
  });

  it("returns the mode mark after saving it", () => {
    stubStorage();
    saveScanMode("mark");
    expect(loadScanMode()).toBe("mark");
  });
});

describe("feedbackFor", () => {
  it("shows a booking in add mode green with the add tone", () => {
    expect(feedbackFor({ ok: true, mode: "add", result: result("add") })).toEqual({
      color: "green",
      sound: "add",
      text: "Kidneybohnen 1 → 2",
    });
  });

  it("shows a booking in consume mode blue with the consume tone", () => {
    expect(
      feedbackFor({ ok: true, mode: "consume", result: result("consume") }),
    ).toEqual({ color: "blue", sound: "consume", text: "Kidneybohnen 3 → 2" });
  });

  it("shows a new product with its message in the mode color", () => {
    const created = result("add", {
      product_created: true,
      message: "Neu: Kidneybohnen 0 → 1",
    });
    expect(feedbackFor({ ok: true, mode: "add", result: created })).toEqual({
      color: "green",
      sound: "add",
      text: "Neu: Kidneybohnen 0 → 1",
    });
  });

  it.each([
    ["consume", "clamped_to_zero", "Kidneybohnen 1 → 0"],
    ["add", "placeholder_created", "Neu: Produkt 4001234567890 0 → 1"],
  ] as const)(
    "shows a booking in %s mode with warning %s yellow with the warn tone",
    (mode, warning, message) => {
      const booked = result(mode, { warnings: [warning], message });
      expect(feedbackFor({ ok: true, mode, result: booked })).toEqual({
        color: "yellow",
        sound: "warn",
        text: message,
      });
    },
  );

  it.each([
    [problem(404, "unknown_barcode"), "red", "error", "Unbekannter Barcode"],
    [problem(409, "stock_already_zero"), "yellow", "warn", "War schon leer"],
    [problem(422, "invalid_barcode"), "red", "error", "Ungültiger Barcode"],
    [problem(422, "idempotency_key_mismatch"), "red", "error", "Ungültiger Barcode"],
    [new TypeError("Failed to fetch"), "red", "error", "Server nicht erreichbar"],
    [new DOMException("", "TimeoutError"), "red", "error", "Server nicht erreichbar"],
    [new DOMException("", "AbortError"), "red", "error", "Server nicht erreichbar"],
    [statusError(502), "red", "error", "Server nicht erreichbar"],
    [statusError(503), "red", "error", "Server nicht erreichbar"],
    [statusError(504), "red", "error", "Server nicht erreichbar"],
    [problem(500, "internal"), "red", "error", "Buchung fehlgeschlagen"],
    [statusError(500), "red", "error", "Buchung fehlgeschlagen"],
    [new Error("unexpected"), "red", "error", "Buchung fehlgeschlagen"],
  ])("shows the error %o as %s with the %s tone", (error, color, sound, text) => {
    expect(feedbackFor({ ok: false, error })).toEqual({ color, sound, text });
  });
});

describe("feedbackFor in mode mark", () => {
  function markResult(fields: Partial<MarkResult> = {}): MarkResult {
    return {
      product: { ...product, marked: true },
      product_created: false,
      already_listed: false,
      message: "Vorgemerkt: Kidneybohnen",
      ...fields,
    };
  }

  it("shows a new mark orange with the mark tone", () => {
    expect(feedbackFor({ ok: true, mode: "mark", result: markResult() })).toEqual({
      color: "orange",
      sound: "mark",
      text: "Vorgemerkt: Kidneybohnen",
    });
  });

  it("shows a product that was listed before orange with the mark tone", () => {
    const listed = markResult({
      already_listed: true,
      message: "Schon auf der Liste: Kidneybohnen",
    });
    expect(feedbackFor({ ok: true, mode: "mark", result: listed })).toEqual({
      color: "orange",
      sound: "mark",
      text: "Schon auf der Liste: Kidneybohnen",
    });
  });

  it("shows a product the mark created orange with the mark tone", () => {
    const created = markResult({
      product: { ...product, name: "Neues Produkt 2000000000008", stock: 0, marked: true },
      product_created: true,
      message: "Neu vorgemerkt: Neues Produkt 2000000000008",
    });
    expect(feedbackFor({ ok: true, mode: "mark", result: created })).toEqual({
      color: "orange",
      sound: "mark",
      text: "Neu vorgemerkt: Neues Produkt 2000000000008",
    });
  });

  it.each([
    [problem(422, "invalid_barcode"), "Ungültiger Barcode"],
    [new TypeError("Failed to fetch"), "Server nicht erreichbar"],
    [new DOMException("", "TimeoutError"), "Server nicht erreichbar"],
    [statusError(502), "Server nicht erreichbar"],
    [statusError(503), "Server nicht erreichbar"],
    [statusError(504), "Server nicht erreichbar"],
    [problem(500, "internal"), "Vormerken fehlgeschlagen"],
    [new Error("unexpected"), "Vormerken fehlgeschlagen"],
  ])("shows the error %o of a mark red with the error tone", (error, text) => {
    expect(feedbackFor({ ok: false, error, mode: "mark" })).toEqual({ color: "red", sound: "error", text });
  });
});

describe("unmarkFeedbackFor", () => {
  it("shows the removed mark yellow with the warn tone", () => {
    expect(unmarkFeedbackFor({ ok: true, product })).toEqual({
      color: "yellow",
      sound: "warn",
      text: "Nicht mehr vorgemerkt: Kidneybohnen",
    });
  });

  it.each([
    [problem(404, "not_found"), "Rückgängig fehlgeschlagen"],
    [problem(500, "internal"), "Rückgängig fehlgeschlagen"],
    [new TypeError("Failed to fetch"), "Server nicht erreichbar"],
    [new DOMException("", "TimeoutError"), "Server nicht erreichbar"],
    [statusError(503), "Server nicht erreichbar"],
  ])("shows the error %o red with the error tone", (error, text) => {
    expect(unmarkFeedbackFor({ ok: false, error })).toEqual({
      color: "red",
      sound: "error",
      text,
    });
  });
});

describe("undoFeedbackFor", () => {
  it("shows the message of the reversal yellow with the warn tone", () => {
    const reversal = result("consume", { message: "Kidneybohnen 3 → 2" });
    expect(undoFeedbackFor({ ok: true, result: reversal })).toEqual({
      color: "yellow",
      sound: "warn",
      text: "Kidneybohnen 3 → 2",
    });
  });

  it("shows a clamped reversal yellow with the warn tone", () => {
    const reversal = result("consume", {
      warnings: ["clamped_to_zero"],
      message: "Kidneybohnen 1 → 0",
    });
    expect(undoFeedbackFor({ ok: true, result: reversal })).toEqual({
      color: "yellow",
      sound: "warn",
      text: "Kidneybohnen 1 → 0",
    });
  });

  it.each([
    [problem(409, "already_reversed"), "Schon rückgängig gemacht"],
    [problem(409, "not_reversible"), "Rückgängig fehlgeschlagen"],
    [problem(404, "not_found"), "Rückgängig fehlgeschlagen"],
    [problem(500, "internal"), "Rückgängig fehlgeschlagen"],
    [statusError(500), "Rückgängig fehlgeschlagen"],
    [new TypeError("Failed to fetch"), "Server nicht erreichbar"],
    [new DOMException("", "TimeoutError"), "Server nicht erreichbar"],
    [statusError(502), "Server nicht erreichbar"],
  ])("shows the error %o red with the error tone", (error, text) => {
    expect(undoFeedbackFor({ ok: false, error })).toEqual({
      color: "red",
      sound: "error",
      text,
    });
  });
});

describe("feedbackIcon", () => {
  const marked: MarkResult = {
    product: { ...product, marked: true },
    product_created: false,
    already_listed: false,
    message: "Vorgemerkt: Kidneybohnen",
  };

  it.each([
    ["a booking in add mode", "check", feedbackFor({ ok: true, mode: "add", result: result("add") })],
    ["a booking in consume mode", "check", feedbackFor({ ok: true, mode: "consume", result: result("consume") })],
    ["a mark", "cart", feedbackFor({ ok: true, mode: "mark", result: marked })],
    [
      "a booking with a warning",
      "warning",
      feedbackFor({ ok: true, mode: "consume", result: result("consume", { warnings: ["clamped_to_zero"] }) }),
    ],
    ["an empty stock", "warning", feedbackFor({ ok: false, error: problem(409, "stock_already_zero") })],
    ["an undone booking", "warning", undoFeedbackFor({ ok: true, result: result("consume") })],
    ["an undone mark", "warning", unmarkFeedbackFor({ ok: true, product })],
    ["an unknown barcode", "cross", feedbackFor({ ok: false, error: problem(404, "unknown_barcode") })],
    ["a failed mark", "cross", feedbackFor({ ok: false, error: new TypeError("Failed to fetch"), mode: "mark" })],
    ["a failed undo", "cross", undoFeedbackFor({ ok: false, error: problem(409, "already_reversed") })],
  ] as const)("shows %s with the symbol %s", (_, icon, feedback) => {
    expect(feedbackIcon(feedback)).toBe(icon);
  });

  it.each([
    ["green", "check"],
    ["blue", "check"],
    ["orange", "cart"],
    ["yellow", "warning"],
    ["red", "cross"],
  ] as const)("gives the color %s the symbol %s", (color, icon) => {
    expect(feedbackIcon({ color, sound: "add", text: "" })).toBe(icon);
  });
});

describe("cameraErrorText", () => {
  it.each([
    [
      "NotAllowedError",
      "Die Kamera liest nur Barcodes; Bilder verlassen das Gerät nicht. Kamera in den Safari-Einstellungen für diese Seite erlauben.",
    ],
    ["NotFoundError", "Keine Kamera gefunden."],
    ["OverconstrainedError", "Keine Kamera gefunden."],
    ["NotReadableError", "Die Kamera ist gerade belegt, vielleicht von einer anderen App."],
    ["AbortError", "Die Kamera konnte nicht gestartet werden."],
  ])("explains %s", (name, text) => {
    expect(cameraErrorText(new DOMException("", name))).toBe(text);
  });

  it("explains an error without a name", () => {
    expect(cameraErrorText(null)).toBe("Die Kamera konnte nicht gestartet werden.");
  });
});
