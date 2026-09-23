// Screen Wake Lock, so the display stays on while the scanner runs.

// Returns null if the browser has no Screen Wake Lock API.
export async function requestWakeLock(): Promise<WakeLockSentinel | null> {
  if (!("wakeLock" in navigator)) {
    return null;
  }
  return navigator.wakeLock.request("screen");
}

export function releaseWakeLock(sentinel: WakeLockSentinel | null): void {
  void sentinel?.release().catch(() => undefined);
}
