import { problemCode } from "./api/client";
import type { components } from "./api/schema";
import type { TONES } from "./scanner/feedback";

export type MovementResult = components["schemas"]["MovementResult"];

/** The mode of the scan view: the kind every scanned code is booked with. */
export type ScanMode = "add" | "consume";

const modeKey = "stashbert.scanMode";

/** Returns the saved mode of the scan view, "add" if none is saved. */
export function loadScanMode(): ScanMode {
  try {
    return localStorage.getItem(modeKey) === "consume" ? "consume" : "add";
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
 * The outcome of booking a scanned code: the result of a booking in mode,
 * or the error the booking failed with (see scanMovementMutation).
 */
export type ScanResult =
  | { ok: true; mode: ScanMode; result: MovementResult }
  | { ok: false; error: unknown };

export type FeedbackColor = "green" | "blue" | "yellow" | "red";

/** How the scan view reports a scan: flash color, tone and text. */
export interface Feedback {
  color: FeedbackColor;
  sound: keyof typeof TONES;
  text: string;
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
 * Returns the feedback for a scan (docs/plan.md, F08). A booking shows
 * its message, in the color and with the tone of its mode, or yellow
 * with the warn tone if it has warnings.
 */
export function feedbackFor(scan: ScanResult): Feedback {
  if (scan.ok) {
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
  return failure("Buchung fehlgeschlagen");
}

/** Returns a German text for an error of getUserMedia. */
export function cameraErrorText(error: unknown): string {
  const name = (error as { name?: unknown } | null)?.name;
  switch (name) {
    case "NotAllowedError":
      return "Kein Zugriff auf die Kamera. Bitte den Kamerazugriff für diese Seite erlauben.";
    case "NotFoundError":
    case "OverconstrainedError":
      return "Keine Kamera gefunden.";
    case "NotReadableError":
      return "Die Kamera ist gerade belegt, vielleicht von einer anderen App.";
  }
  return "Die Kamera konnte nicht gestartet werden.";
}
