import { useEffect, useEffectEvent, useState, type ReactNode } from "react";
import type { ScanTimings as CameraTimings } from "../lib/scanner/scanner";
import { READ_WINDOW } from "../lib/scanner/timings";

export interface TimingValues {
  /** The running camera, null if none runs. */
  camera: CameraTimings | null;
  /** From reading a code to the answer of the server, null before the first. */
  bookingMs: number | null;
}

interface ScanTimingsProps {
  /** Returns the current values; called once per second while shown. */
  read: () => TimingValues;
}

// Taking the values once per second, and only while the menu shows them,
// keeps the read loop free of renders (F25).
const refreshMs = 1000;

function Row({ name, children }: { name: string; children: ReactNode }) {
  return (
    <div className="flex flex-wrap gap-x-2">
      <dt>{name}:</dt>
      <dd>{children}</dd>
    </div>
  );
}

function ms(value: number | null): string {
  return value === null ? "noch keine" : `${Math.round(value)} ms`;
}

/**
 * Timings of the scanner for the camera menu, small and plain: the size of
 * the camera image, the last read and the mean of the last reads, reads per
 * second and the last booking. They help to find out why a code is read
 * slowly; they stay on the device.
 */
export function ScanTimings({ read }: ScanTimingsProps) {
  const [values, setValues] = useState<TimingValues | null>(null);
  const update = useEffectEvent(() => setValues(read()));

  useEffect(() => {
    const first = setTimeout(() => update(), 0);
    const timer = setInterval(() => update(), refreshMs);
    return () => {
      clearTimeout(first);
      clearInterval(timer);
    };
  }, []);

  if (values === null) {
    return null;
  }
  const { camera, bookingMs } = values;
  return (
    <section aria-label="Messwerte" className="mt-4 text-sm text-ink-secondary">
      <h3 className="font-medium">Messwerte</h3>
      {/* One row per value; the value moves below its name when the text is large. */}
      <dl className="mt-1 tabular-nums">
        {camera !== null && (
          <>
            <Row name="Bild">
              {camera.width > 0 ? `${camera.width} × ${camera.height} px` : "noch keins"}
            </Row>
            <Row name="Lesen">
              {camera.lastMs === null
                ? "noch keins"
                : `zuletzt ${ms(camera.lastMs)}, Mittel ${ms(camera.averageMs)} (letzte ${READ_WINDOW})`}
            </Row>
            <Row name="Versuche">{camera.perSecond} pro Sekunde</Row>
          </>
        )}
        <Row name="Letzte Buchung">{ms(bookingMs)}</Row>
      </dl>
    </section>
  );
}
