// Read loop: at most one attempt per interval and never two at the same time.

export const MIN_INTERVAL_MS = 100; // at most 10 attempts per second

// Runs `step` repeatedly. The next run is scheduled only after the previous
// one has finished, and no earlier than `intervalMs` after it started.
// Returns a function that stops the loop.
export function startLoop(step: () => Promise<void>, intervalMs = MIN_INTERVAL_MS): () => void {
  let stopped = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  const run = async () => {
    if (stopped) {
      return;
    }
    const started = performance.now();
    await step();
    if (stopped) {
      return;
    }
    const elapsed = performance.now() - started;
    timer = setTimeout(run, Math.max(0, intervalMs - elapsed));
  };

  timer = setTimeout(run, 0);
  return () => {
    stopped = true;
    clearTimeout(timer);
  };
}
