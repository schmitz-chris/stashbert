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
 * "Noch keine" if there is none. Without timeZone the time zone of the
 * device is used; tests pass one to get a fixed result.
 */
export function lastBackupText(lastBackupAt: string | null, timeZone?: string): string {
  return lastBackupAt === null ? "Noch keine" : formatMovementTime(lastBackupAt, timeZone);
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
