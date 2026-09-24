import type { components } from "./api/schema";

export type Movement = components["schemas"]["Movement"];

const kindLabels: Record<Movement["kind"], string> = {
  add: "Eingelagert",
  consume: "Entnommen",
  inventory: "Inventur",
  reversal: "Storno",
  merge: "Zusammengeführt",
};

/** Returns the German label of a movement kind, for example "Entnommen". */
export function movementKindLabel(kind: Movement["kind"]): string {
  return kindLabels[kind];
}

/**
 * Formats the delta of a movement with its sign: "+2", "−1" (with the
 * minus sign U+2212) or "0".
 */
export function formatDelta(delta: number): string {
  if (delta > 0) {
    return `+${delta}`;
  }
  if (delta < 0) {
    return `−${-delta}`;
  }
  return "0";
}

/**
 * Formats the time of a movement (RFC 3339) in German, for example
 * "23.09.2026, 14:05". Without timeZone the time zone of the device is
 * used; tests pass one to get a fixed result.
 */
export function formatMovementTime(createdAt: string, timeZone?: string): string {
  return new Intl.DateTimeFormat("de-DE", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    timeZone,
  }).format(new Date(createdAt));
}

/** How many movements the product page shows before "Alle anzeigen". */
export const collapsedMovementCount = 3;

/**
 * Returns the movements the history of the product page shows: the first
 * collapsedMovementCount, or all of them (at most the ten of the query)
 * once the user chose "Alle anzeigen" (docs/plan.md, F22).
 */
export function visibleMovements<T>(movements: T[], expanded: boolean): T[] {
  return expanded ? movements : movements.slice(0, collapsedMovementCount);
}
