import { describe, expect, it } from "vitest";
import { accept, CONFIRM_COUNT, CONFIRM_GAP_MS, EMPTY_DEDUPE, REPEAT_WINDOW_MS } from "./dedupe";
import type { DedupeState } from "./dedupe";

const EAN = "4001686301265";
const OTHER = "3017620422003";

// Feeds sightings (code, time in ms) through accept and returns the codes that
// were accepted, with their times.
function run(sightings: [string, number][], start: DedupeState = EMPTY_DEDUPE): [string, number][] {
  let state = start;
  const accepted: [string, number][] = [];
  for (const [code, now] of sightings) {
    const result = accept(state, code, now);
    state = result.state;
    if (result.accepted) {
      accepted.push([code, now]);
    }
  }
  return accepted;
}

// Sightings of code every stepMs from start to end, both included.
function every(code: string, start: number, end: number, stepMs: number): [string, number][] {
  const sightings: [string, number][] = [];
  for (let now = start; now <= end; now += stepMs) {
    sightings.push([code, now]);
  }
  return sightings;
}

describe("accept", () => {
  it("uses a repeat window of 2000 ms and two sightings at most 700 ms apart", () => {
    expect(REPEAT_WINDOW_MS).toBe(2000);
    expect(CONFIRM_COUNT).toBe(2);
    expect(CONFIRM_GAP_MS).toBe(700);
  });

  it("does not accept a single sighting", () => {
    expect(accept(EMPTY_DEDUPE, EAN, 0).accepted).toBe(false);
    expect(run([[EAN, 0]])).toEqual([]);
  });

  it("accepts a code on its second sighting within 700 ms", () => {
    expect(run([
      [EAN, 0],
      [EAN, 80],
    ])).toEqual([[EAN, 80]]);
    expect(run([
      [EAN, 0],
      [EAN, 700],
    ])).toEqual([[EAN, 700]]);
  });

  it("does not confirm two sightings more than 700 ms apart", () => {
    expect(run([
      [EAN, 0],
      [EAN, 701],
    ])).toEqual([]);
    // The second sighting starts a new streak, which a third one confirms.
    expect(run([
      [EAN, 0],
      [EAN, 900],
      [EAN, 1000],
    ])).toEqual([[EAN, 1000]]);
  });

  it("never accepts misreads that each appear in one frame only", () => {
    // Ketjap Manis on 2026-09-24: three wrong codes around the right one.
    expect(run([
      ["0111220382309", 0],
      ["8711200382309", 100],
      ["0751020382309", 230],
      ["8711200382309", 300],
      ["1223083602711", 360],
    ])).toEqual([["8711200382309", 300]]);
  });

  it("accepts a code that stays in view for 10 s only once", () => {
    expect(run(every(EAN, 0, 10_000, 100))).toEqual([[EAN, 100]]);
  });

  it("accepts the code again after 2000 ms out of view, once confirmed again", () => {
    expect(run([
      [EAN, 0],
      [EAN, 100],
      [EAN, 2100],
      [EAN, 2200],
    ])).toEqual([
      [EAN, 100],
      [EAN, 2200],
    ]);
    // The window starts at the last sighting, not at the accepted one.
    expect(run([
      [EAN, 0],
      [EAN, 100],
      [EAN, 1500],
      [EAN, 3000],
      [EAN, 3100],
      [EAN, 5100],
      [EAN, 5200],
    ])).toEqual([
      [EAN, 100],
      [EAN, 5200],
    ]);
  });

  it("does not accept a code again while it is in view with gaps", () => {
    // Out of focus for 1 s, but never out of view for 2 s.
    expect(run([
      [EAN, 0],
      [EAN, 100],
      [EAN, 1100],
      [EAN, 1200],
    ])).toEqual([[EAN, 100]]);
  });

  it("handles different codes independently", () => {
    expect(run([
      [EAN, 0],
      [OTHER, 100],
      [EAN, 200],
      [OTHER, 300],
      [OTHER, 2400],
      [OTHER, 2500],
    ])).toEqual([
      [EAN, 200],
      [OTHER, 300],
      [OTHER, 2500],
    ]);
  });

  it("leaves the given state unchanged", () => {
    const first = accept(EMPTY_DEDUPE, EAN, 0);
    const second = accept(first.state, EAN, 100);
    expect(EMPTY_DEDUPE.size).toBe(0);
    expect(first.state.get(EAN)).toEqual({ last: 0, streak: 1, accepted: false });
    expect(second.state.get(EAN)).toEqual({ last: 100, streak: 2, accepted: true });
    // The older state still decides as before.
    expect(accept(first.state, EAN, 100).accepted).toBe(true);
  });
});
