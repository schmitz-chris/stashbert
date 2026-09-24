import { problemCode } from "./api/client";
import type { components } from "./api/schema";
import type { Product } from "./products";
import type { TONES } from "./scanner/feedback";

export type MovementResult = components["schemas"]["MovementResult"];
export type MarkResult = components["schemas"]["MarkResult"];

/** A mode of the scan view that books every scanned code with its kind. */
export type BookingMode = "add" | "consume";

/**
 * The mode of the scan view: booking with a kind, or mark, which marks
 * every scanned code for shopping (docs/plan.md, F15).
 */
export type ScanMode = BookingMode | "mark";

const modeKey = "stashbert.scanMode";

/**
 * Returns the saved mode of the scan view, "add" if none or an unknown
 * value is saved.
 */
export function loadScanMode(): ScanMode {
  try {
    const saved = localStorage.getItem(modeKey);
    return saved === "consume" || saved === "mark" ? saved : "add";
  } catch {
    return "add";
  }
}

/** Saves the mode of the scan view. */
export function saveScanMode(mode: ScanMode): void {
  try {
    localStorage.setItem(modeKey, mode);
  } catch {
    // Storage unavailable: the mode only lasts while the view is open.
  }
}

/**
 * The outcome of a scanned code: the result of a booking in mode, the
 * result of marking it in mode mark, or the error the booking or the mark
 * failed with (see scanMovementMutation and markScanMutation).
 */
export type ScanResult =
  | { ok: true; mode: BookingMode; result: MovementResult }
  | { ok: true; mode: "mark"; result: MarkResult }
  | { ok: false; error: unknown; mode?: ScanMode };

export type FeedbackColor = "green" | "blue" | "orange" | "yellow" | "red";

/** How the scan view reports a scan: flash color, tone and text. */
export interface Feedback {
  color: FeedbackColor;
  sound: keyof typeof TONES;
  text: string;
}

/**
 * The symbol of a scan message, so that it does not rely on its color
 * alone (ADR-0016): a check for a booking, a cart for a mark, an
 * exclamation mark for a warning, a cross for an error.
 */
export type FeedbackIcon = "check" | "cart" | "warning" | "cross";

/** Returns the symbol of feedback, given by its color. */
export function feedbackIcon(feedback: Feedback): FeedbackIcon {
  switch (feedback.color) {
    case "green":
    case "blue":
      return "check";
    case "orange":
      return "cart";
    case "yellow":
      return "warning";
    case "red":
      return "cross";
  }
}

function failure(text: string): Feedback {
  return { color: "red", sound: "error", text };
}

// Reads the field status of an error (Problem Details or the Error of a
// response without them).
function errorStatus(error: unknown): number | undefined {
  if (typeof error !== "object" || error === null || !("status" in error)) {
    return undefined;
  }
  return typeof error.status === "number" ? error.status : undefined;
}

// Gateway errors: a reverse proxy in front of the server got no answer.
const gatewayStatuses = [502, 503, 504];

// Reports whether a booking failed without an answer of the server. fetch
// rejects with a TypeError on a network error and with a TimeoutError when
// the timeout of the booking expires; browsers without abort reasons
// reject with an AbortError instead.
function isUnreachable(error: unknown): boolean {
  const name = (error as { name?: unknown } | null)?.name;
  if (error instanceof TypeError || name === "TimeoutError" || name === "AbortError") {
    return true;
  }
  const status = errorStatus(error);
  return status !== undefined && gatewayStatuses.includes(status);
}

/**
 * Returns the feedback for a scan (docs/plan.md, F08 and F15). A booking
 * shows its message, in the color and with the tone of its mode, or yellow
 * with the warn tone if it has warnings. A mark shows its message orange
 * with the mark tone (see markFeedback).
 */
export function feedbackFor(scan: ScanResult): Feedback {
  if (scan.ok) {
    if (scan.mode === "mark") {
      return markFeedback(scan.result);
    }
    const text = scan.result.message;
    if (scan.result.warnings.length > 0) {
      return { color: "yellow", sound: "warn", text };
    }
    return scan.mode === "add"
      ? { color: "green", sound: "add", text }
      : { color: "blue", sound: "consume", text };
  }
  const { error } = scan;
  if (isUnreachable(error)) {
    return failure("Server nicht erreichbar");
  }
  switch (problemCode(error)) {
    case "unknown_barcode":
      return failure("Unbekannter Barcode");
    case "stock_already_zero":
      return { color: "yellow", sound: "warn", text: "War schon leer" };
  }
  if (errorStatus(error) === 422) {
    return failure("Ungültiger Barcode");
  }
  return failure(scan.mode === "mark" ? "Vormerken fehlgeschlagen" : "Buchung fehlgeschlagen");
}

// Returns the feedback for marking a scanned code: its message orange with
// the mark tone, also if the product was listed already ("Schon auf der
// Liste: <name>").
function markFeedback(result: MarkResult): Feedback {
  return { color: "orange", sound: "mark", text: result.message };
}

/**
 * The outcome of undoing the booking on the result card: the result of
 * the reversal, or the error it failed with (see reversalMutation).
 */
export type UndoResult =
  | { ok: true; result: MovementResult }
  | { ok: false; error: unknown };

/**
 * Returns the feedback for undoing a booking on the result card
 * (docs/plan.md, F09): the message of the reversal in yellow with the
 * warn tone, or a red error with the error tone.
 */
export function undoFeedbackFor(undo: UndoResult): Feedback {
  if (undo.ok) {
    return { color: "yellow", sound: "warn", text: undo.result.message };
  }
  if (isUnreachable(undo.error)) {
    return failure("Server nicht erreichbar");
  }
  if (problemCode(undo.error) === "already_reversed") {
    return failure("Schon rückgängig gemacht");
  }
  return failure("Rückgängig fehlgeschlagen");
}

/**
 * The outcome of undoing the mark on the result card: the product whose
 * mark was removed, or the error it failed with (see unmarkMutation).
 */
export type UnmarkResult =
  | { ok: true; product: Product }
  | { ok: false; error: unknown };

/**
 * Returns the feedback for undoing a mark on the result card
 * (docs/plan.md, F15): "Nicht mehr vorgemerkt: <name>" in yellow with the
 * warn tone, or a red error with the error tone.
 */
export function unmarkFeedbackFor(undo: UnmarkResult): Feedback {
  if (undo.ok) {
    return {
      color: "yellow",
      sound: "warn",
      text: `Nicht mehr vorgemerkt: ${undo.product.name}`,
    };
  }
  if (isUnreachable(undo.error)) {
    return failure("Server nicht erreichbar");
  }
  return failure("Rückgängig fehlgeschlagen");
}

/** Returns a German text for an error of getUserMedia. */
export function cameraErrorText(error: unknown): string {
  const name = (error as { name?: unknown } | null)?.name;
  switch (name) {
    case "NotAllowedError":
      // Why the view needs the camera and how to allow it (docs/plan.md, F24).
      return "Die Kamera liest nur Barcodes; Bilder verlassen das Gerät nicht. Kamera in den Safari-Einstellungen für diese Seite erlauben.";
    case "NotFoundError":
    case "OverconstrainedError":
      return "Keine Kamera gefunden.";
    case "NotReadableError":
      return "Die Kamera ist gerade belegt, vielleicht von einer anderen App.";
  }
  return "Die Kamera konnte nicht gestartet werden.";
}
