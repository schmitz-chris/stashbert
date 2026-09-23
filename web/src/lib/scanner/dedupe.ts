// Repeat filter for scanned codes, as a pure function over an immutable state.
// Every sighting of a code renews its window, so a code that stays in view is
// accepted once, not every REPEAT_WINDOW_MS. To accept the same code again, it
// has to be out of view for REPEAT_WINDOW_MS.

export const REPEAT_WINDOW_MS = 2000;

// Time of the last sighting per code, in ms.
export type DedupeState = ReadonlyMap<string, number>;

export const EMPTY_DEDUPE: DedupeState = new Map();

export interface DedupeResult {
  state: DedupeState;
  accepted: boolean;
}

// Records a sighting of code at now. accepted is false if the same code was
// seen less than REPEAT_WINDOW_MS ago. The given state is left unchanged.
export function accept(state: DedupeState, code: string, now: number): DedupeResult {
  const last = state.get(code);
  return {
    state: new Map(state).set(code, now),
    accepted: last === undefined || now - last >= REPEAT_WINDOW_MS,
  };
}
