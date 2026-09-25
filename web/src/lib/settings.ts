import { problemCode } from "./api/client";
import type { components } from "./api/schema";
import { formatMovementTime } from "./movements";
import type { MqttStatus } from "./shoppingTarget";

/**
 * The state of the server for the settings (architecture.md, 6.2): version,
 * size of the database, backups and whether Open Food Facts is used.
 */
export type SystemStatus = components["schemas"]["SystemStatus"];

// The units of formatSize above bytes, each 1000 times the one before, with
// the number of decimals they are shown with.
const sizeUnits = [
  { unit: "KB", digits: 0 },
  { unit: "MB", digits: 1 },
  { unit: "GB", digits: 1 },
] as const;

/**
 * Formats a size in bytes readable in German, in steps of 1000 like iOS
 * and the Finder: "512 Byte", "128 KB", "1,4 MB", "2,1 GB". A number that
 * would round to 1000 goes up to the next unit ("1 MB", not "1000 KB").
 */
export function formatSize(bytes: number): string {
  if (bytes < 1000) {
    return `${bytes} Byte`;
  }
  let step = 0;
  while (
    step < sizeUnits.length - 1 &&
    roundTo(bytes / 1000 ** (step + 1), sizeUnits[step].digits) >= 1000
  ) {
    step++;
  }
  const { unit, digits } = sizeUnits[step];
  const number = new Intl.NumberFormat("de-DE", { maximumFractionDigits: digits });
  return `${number.format(bytes / 1000 ** (step + 1))} ${unit}`;
}

// Rounds value to digits decimals.
function roundTo(value: number, digits: number): number {
  const factor = 10 ** digits;
  return Math.round(value * factor) / factor;
}

/**
 * Returns the time of the newest backup (RFC 3339) in German, like
 * "25.09.2026, 11:40", in the same format as the times of movements, or
 * "Noch keines" if there is none. Without timeZone the time zone of the
 * device is used; tests pass one to get a fixed result.
 */
export function lastBackupText(lastBackupAt: string | null, timeZone?: string): string {
  return lastBackupAt === null ? "Noch keines" : formatMovementTime(lastBackupAt, timeZone);
}

/**
 * Returns how many backups there are of how many the server keeps
 * (BACKUP_KEEP), like "3 von höchstens 14".
 */
export function backupCountText(count: number, keep: number): string {
  return `${count} von höchstens ${keep}`;
}

const mqttStateTexts: Record<MqttStatus["status"], string> = {
  disabled: "Nicht eingerichtet",
  connecting: "Keine Verbindung zum Broker",
  connected: "Verbunden",
};

/** Returns the state of the connection to Home Assistant in German. */
export function mqttStateText(status: MqttStatus["status"]): string {
  return mqttStateTexts[status];
}

/**
 * Returns whether unknown barcodes are looked up at Open Food Facts
 * (OFF_CONTACT is set): "aktiv" or "nicht eingerichtet".
 */
export function openFoodFactsText(enabled: boolean): string {
  return enabled ? "aktiv" : "nicht eingerichtet";
}

/**
 * Returns the question before a backup is restored (docs/plan.md, F33),
 * with the name of the picked file.
 */
export function restoreQuestionText(fileName: string): string {
  return (
    `Alle Produkte, Bestände und Bilder werden durch das Backup „${fileName}“ ersetzt. ` +
    "Der jetzige Stand bleibt auf dem Server als Kopie erhalten."
  );
}

/**
 * Returns the message for a backup that StashBert did not take, by the
 * code of its Problem Details (architecture.md, 6.4), or "Einspielen
 * fehlgeschlagen" for every other failure, a network error included.
 */
export function restoreErrorText(error: unknown): string {
  switch (problemCode(error)) {
    case "invalid_backup":
      return "Das ist kein Backup von StashBert.";
    case "backup_too_new":
      return "Das Backup stammt von einer neueren Version. Bitte zuerst StashBert aktualisieren.";
    case "backup_too_large":
      return "Das Backup ist zu groß (höchstens 1 GB).";
    case "restore_in_progress":
      return "Es wird gerade schon ein Backup eingespielt.";
    default:
      return "Einspielen fehlgeschlagen";
  }
}

/** The state of the upload of a backup, like the status of a mutation. */
export type UploadStatus = "idle" | "pending" | "error" | "success";

/**
 * Whether StashBert answered again after the restart that follows an
 * uploaded backup: still waiting, answered, or silent for 60 s.
 */
export type RestartStatus = "waiting" | "answered" | "silent";

/** A notice below the buttons of the section "Backup". */
export interface RestoreNotice {
  text: string;
  tone: "progress" | "success" | "failure";
}

const restoringText = "Backup wird eingespielt …";

/**
 * Returns what the section "Backup" says about restoring a backup
 * (docs/plan.md, F33): nothing before the first try, "Backup wird
 * eingespielt …" during the upload and while StashBert restarts, then
 * "Backup eingespielt", the message of a refused upload, or that
 * StashBert did not answer within 60 s. error is the error of the upload.
 */
export function restoreNotice(
  upload: UploadStatus,
  error: unknown,
  restart: RestartStatus,
): RestoreNotice {
  switch (upload) {
    case "idle":
      return { text: "", tone: "progress" };
    case "pending":
      return { text: restoringText, tone: "progress" };
    case "error":
      return { text: restoreErrorText(error), tone: "failure" };
    case "success":
      switch (restart) {
        case "waiting":
          return { text: restoringText, tone: "progress" };
        case "answered":
          return { text: "Backup eingespielt", tone: "success" };
        case "silent":
          return {
            text: "StashBert antwortet nicht. Bitte die Seite später neu laden.",
            tone: "failure",
          };
      }
  }
}

// How often and how long the settings ask whether StashBert is back after
// the restart that follows an uploaded backup (docs/plan.md, F33).
const restartInterval = 500;
const restartTimeout = 60_000;

/**
 * Waits until StashBert answers again after the restart that follows an
 * uploaded backup (ADR-0020): calls ask every 500 ms, at most for 60 s.
 * ask gets a signal that aborts at the end of the 60 s or with signal, and
 * returns whether StashBert answered; its errors (no connection while
 * StashBert restarts) count as no answer. Resolves with true as soon as
 * ask returns true, otherwise with false after the 60 s or when signal
 * aborts. Its timers end with it.
 */
export async function waitForRestart(
  ask: (signal: AbortSignal) => Promise<boolean>,
  signal: AbortSignal,
): Promise<boolean> {
  const deadline = new AbortController();
  const timer = setTimeout(() => deadline.abort(), restartTimeout);
  const stop = AbortSignal.any([signal, deadline.signal]);
  try {
    for (;;) {
      await sleep(restartInterval, stop);
      if (stop.aborted) {
        return false;
      }
      try {
        if (await ask(stop)) {
          return true;
        }
      } catch {
        // StashBert is not back yet; ask again.
      }
    }
  } finally {
    clearTimeout(timer);
  }
}

// Resolves after ms, or at once when signal aborts.
function sleep(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    if (signal.aborted) {
      resolve();
      return;
    }
    const done = () => {
      clearTimeout(timer);
      signal.removeEventListener("abort", done);
      resolve();
    };
    const timer = setTimeout(done, ms);
    signal.addEventListener("abort", done);
  });
}
