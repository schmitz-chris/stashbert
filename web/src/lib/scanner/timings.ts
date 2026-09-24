// Timings of the read loop for the camera menu (docs/plan.md, F25). They
// stay on the device; nothing is sent to the server.

/** How many reads the average covers. */
export const READ_WINDOW = 20;

/** One read of the strip: when it started and how long it took, in ms. */
export interface ReadSample {
  startedAt: number;
  durationMs: number;
}

export interface ReadSummary {
  /** Duration of the last read, null before the first one. */
  lastMs: number | null;
  /** Mean duration of the last READ_WINDOW reads, null before the first one. */
  averageMs: number | null;
  /** Reads started within the last second. */
  perSecond: number;
}

/** The last READ_WINDOW reads after sample. */
export function addRead(samples: readonly ReadSample[], sample: ReadSample): ReadSample[] {
  return [...samples.slice(Math.max(0, samples.length - READ_WINDOW + 1)), sample];
}

/**
 * The summary of samples at the time now (ms, like performance.now()). The
 * read loop runs at most 10 times per second (loop.ts), so the window holds
 * every read of the last second.
 */
export function summarizeReads(samples: readonly ReadSample[], now: number): ReadSummary {
  if (samples.length === 0) {
    return { lastMs: null, averageMs: null, perSecond: 0 };
  }
  const total = samples.reduce((sum, sample) => sum + sample.durationMs, 0);
  return {
    lastMs: samples[samples.length - 1].durationMs,
    averageMs: total / samples.length,
    perSecond: samples.filter((sample) => now - sample.startedAt < 1000).length,
  };
}
