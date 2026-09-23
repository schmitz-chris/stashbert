// Hit handling: suppress repeats of the same code and keep a short history.

export const REPEAT_WINDOW_MS = 2000;
export const HISTORY_SIZE = 10;

export interface HistoryEntry {
  code: string;
  time: string;
}

// Remembers when each code was last accepted.
export class RepeatFilter {
  private readonly lastAccepted = new Map<string, number>();
  private readonly windowMs: number;

  constructor(windowMs = REPEAT_WINDOW_MS) {
    this.windowMs = windowMs;
  }

  // Returns false if the same code was accepted less than windowMs ago.
  accept(code: string, now: number): boolean {
    const last = this.lastAccepted.get(code);
    if (last !== undefined && now - last < this.windowMs) {
      return false;
    }
    this.lastAccepted.set(code, now);
    return true;
  }
}

export function addToHistory(
  history: readonly HistoryEntry[],
  entry: HistoryEntry,
  size = HISTORY_SIZE,
): HistoryEntry[] {
  return [entry, ...history].slice(0, size);
}
