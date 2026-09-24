import { describe, expect, it } from "vitest";
import { addRead, READ_WINDOW, summarizeReads, type ReadSample } from "./timings";

// Reads every 100 ms from startedAt 0 on, with the given durations.
function reads(durations: number[]): ReadSample[] {
  let samples: ReadSample[] = [];
  durations.forEach((durationMs, index) => {
    samples = addRead(samples, { startedAt: index * 100, durationMs });
  });
  return samples;
}

describe("addRead", () => {
  it("keeps the last 20 reads, the newest last", () => {
    expect(READ_WINDOW).toBe(20);
    const samples = reads(Array.from({ length: 25 }, (_, index) => index + 1));
    expect(samples).toHaveLength(20);
    expect(samples[0].durationMs).toBe(6);
    expect(samples[19].durationMs).toBe(25);
  });

  it("does not change the given list", () => {
    const before: ReadSample[] = [{ startedAt: 0, durationMs: 10 }];
    const after = addRead(before, { startedAt: 100, durationMs: 20 });
    expect(before).toEqual([{ startedAt: 0, durationMs: 10 }]);
    expect(after).toHaveLength(2);
  });
});

describe("summarizeReads", () => {
  it("has no durations before the first read", () => {
    expect(summarizeReads([], 1000)).toEqual({ lastMs: null, averageMs: null, perSecond: 0 });
  });

  it("reports the last read and the mean of the last 20", () => {
    const samples = reads(Array.from({ length: 25 }, (_, index) => index + 1));
    const summary = summarizeReads(samples, 2500);
    expect(summary.lastMs).toBe(25);
    // The mean of 6 to 25.
    expect(summary.averageMs).toBe(15.5);
  });

  it("averages fewer reads than 20", () => {
    expect(summarizeReads(reads([30, 50]), 200).averageMs).toBe(40);
  });

  it("counts the reads started within the last second", () => {
    // 20 reads, started at 0, 100, ..., 1900.
    const samples = reads(Array.from({ length: 20 }, () => 40));
    expect(summarizeReads(samples, 1950).perSecond).toBe(10);
    expect(summarizeReads(samples, 2000).perSecond).toBe(9);
    expect(summarizeReads(samples, 2400).perSecond).toBe(5);
  });

  it("counts no reads per second once the loop stopped", () => {
    expect(summarizeReads(reads([40, 40, 40]), 5000).perSecond).toBe(0);
  });
});
