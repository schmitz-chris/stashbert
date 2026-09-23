// Hit handling: suppress repeats of the same code and keep a short history.

export const REPEAT_WINDOW_MS = 2000;
export const HISTORY_SIZE = 10;

export interface HistoryEntry {
  code: string;
  time: string;
}

// Remembers when each code was last seen. Every sighting renews the window,
// so a code that stays in view is booked once, not every windowMs. To book
// the same product again, it has to leave the view for windowMs.
export class RepeatFilter {
  private readonly lastSeen = new Map<string, number>();
  private readonly windowMs: number;

  constructor(windowMs = REPEAT_WINDOW_MS) {
    this.windowMs = windowMs;
  }

  // Returns false if the same code was seen less than windowMs ago.
  accept(code: string, now: number): boolean {
    const last = this.lastSeen.get(code);
    this.lastSeen.set(code, now);
    return last === undefined || now - last >= this.windowMs;
  }
}

export function addToHistory(
  history: readonly HistoryEntry[],
  entry: HistoryEntry,
  size = HISTORY_SIZE,
): HistoryEntry[] {
  return [entry, ...history].slice(0, size);
}
