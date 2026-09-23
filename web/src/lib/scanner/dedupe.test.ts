import { describe, expect, it } from "vitest";
import { accept, EMPTY_DEDUPE, REPEAT_WINDOW_MS } from "./dedupe";
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

describe("accept", () => {
  it("uses a window of 2000 ms", () => {
    expect(REPEAT_WINDOW_MS).toBe(2000);
  });

  it("accepts the first sighting of a code", () => {
    expect(accept(EMPTY_DEDUPE, EAN, 0).accepted).toBe(true);
  });

  it("ignores the same code within 2000 ms", () => {
    expect(run([
      [EAN, 0],
      [EAN, 1999],
    ])).toEqual([[EAN, 0]]);
  });

  it("accepts a code that stays in view for 10 s only once", () => {
    const sightings: [string, number][] = [];
    for (let now = 0; now <= 10_000; now += 100) {
      sightings.push([EAN, now]);
    }
    expect(run(sightings)).toEqual([[EAN, 0]]);
  });

  it("accepts the code again after 2000 ms out of view", () => {
    expect(run([
      [EAN, 0],
      [EAN, 2000],
    ])).toEqual([
      [EAN, 0],
      [EAN, 2000],
    ]);
    // The window starts at the last sighting, not at the accepted one.
    expect(run([
      [EAN, 0],
      [EAN, 1500],
      [EAN, 3000],
      [EAN, 5000],
    ])).toEqual([
      [EAN, 0],
      [EAN, 5000],
    ]);
  });

  it("handles different codes independently", () => {
    expect(run([
      [EAN, 0],
      [OTHER, 100],
      [EAN, 200],
      [OTHER, 300],
      [OTHER, 2300],
    ])).toEqual([
      [EAN, 0],
      [OTHER, 100],
      [OTHER, 2300],
    ]);
  });

  it("leaves the given state unchanged", () => {
    const first = accept(EMPTY_DEDUPE, EAN, 0);
    const second = accept(first.state, EAN, 2500);
    expect(EMPTY_DEDUPE.size).toBe(0);
    expect(first.state.get(EAN)).toBe(0);
    expect(second.state.get(EAN)).toBe(2500);
    // The older state still decides as before.
    expect(accept(first.state, EAN, 2000).accepted).toBe(true);
  });
});
