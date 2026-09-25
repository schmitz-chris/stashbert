// Confirmation and repeat filter for scanned codes, as a pure function over an
// immutable state.
//
// Confirmation: a code counts only after CONFIRM_COUNT sightings of it with
// at most CONFIRM_GAP_MS between two of them. A single misread frame, for
// example on a curved bottle, is never accepted (user feedback of 2026-09-25:
// misreads created products that do not exist).
//
// Repeat: every sighting of a code renews its window, so a code that stays in
// view is accepted once, not every REPEAT_WINDOW_MS. To accept the same code
// again, it has to be out of view for REPEAT_WINDOW_MS and then be confirmed
// again.

export const REPEAT_WINDOW_MS = 2000;
export const CONFIRM_COUNT = 2;
export const CONFIRM_GAP_MS = 700;

// What is known about one code: its last sighting in ms, the number of
// sightings in a row with at most CONFIRM_GAP_MS between them, and whether it
// was accepted while in view.
export interface Sighting {
  last: number;
  streak: number;
  accepted: boolean;
}

export type DedupeState = ReadonlyMap<string, Sighting>;

export const EMPTY_DEDUPE: DedupeState = new Map();

export interface DedupeResult {
  state: DedupeState;
  accepted: boolean;
}

// Records a sighting of code at now. accepted is true only for the sighting
// that confirms the code while it is in view. The given state is left
// unchanged.
export function accept(state: DedupeState, code: string, now: number): DedupeResult {
  const previous = state.get(code);
  const inView = previous !== undefined && now - previous.last < REPEAT_WINDOW_MS;
  const streak = inView && now - previous.last <= CONFIRM_GAP_MS ? previous.streak + 1 : 1;
  const alreadyAccepted = inView && previous.accepted;
  const accepted = !alreadyAccepted && streak >= CONFIRM_COUNT;
  return {
    state: new Map(state).set(code, { last: now, streak, accepted: alreadyAccepted || accepted }),
    accepted,
  };
}
