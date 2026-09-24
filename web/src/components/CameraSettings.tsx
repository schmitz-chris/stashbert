import { useId, useState } from "react";
import {
  currentZoom,
  setZoom,
  zoomRange,
  type CameraOption,
} from "../lib/scanner/camera";

interface CameraSettingsProps {
  /** The video track of the running camera, null if it does not run. */
  track: MediaStreamTrack | null;
  cameras: CameraOption[];
  /** The camera chosen in the menu, undefined while the app chooses. */
  chosenId: string | undefined;
  onSelectCamera: (deviceId: string) => void;
  /** Forgets the chosen camera, so that the app chooses again. */
  onSelectAutomatic: () => void;
}

// The value of "Automatisch" in the camera choice; device ids are never empty
// once the camera runs.
const automatic = "";

function zoomText(zoom: number): string {
  return `${zoom.toFixed(1)}×`;
}

/**
 * Camera choice and zoom of the scanner. Each is shown only if the device
 * offers it. Below the choice the running camera and its zoom range are
 * named, which helps when testing on the device (F25). The light has its
 * own button on the camera image (TorchButton).
 */
export function CameraSettings({
  track,
  cameras,
  chosenId,
  onSelectCamera,
  onSelectAutomatic,
}: CameraSettingsProps) {
  // Zoom belongs to one track; a new track starts without it.
  const [zoom, setZoomState] = useState<{ track: MediaStreamTrack; value: number } | null>(null);
  const cameraId = useId();
  const zoomId = useId();

  if (track === null) {
    return <p className="mt-2 text-ink-secondary">Die Kamera läuft gerade nicht.</p>;
  }
  const range = zoomRange(track);
  const zoomValue = zoom?.track === track ? zoom.value : (currentZoom(track) ?? range?.min);

  function changeZoom(value: number) {
    if (track === null) {
      return;
    }
    setZoomState({ track, value });
    setZoom(track, value).catch((error: unknown) => console.warn("Zoom:", error));
  }

  function changeCamera(value: string) {
    if (value === automatic) {
      onSelectAutomatic();
    } else {
      onSelectCamera(value);
    }
  }

  return (
    <div className="mt-3 flex flex-col gap-4">
      <div>
        {cameras.length > 1 && (
          <>
            <label htmlFor={cameraId} className="block text-sm font-medium text-ink-secondary">
              Kamera
            </label>
            <select
              id={cameraId}
              value={chosenId ?? automatic}
              onChange={(event) => changeCamera(event.target.value)}
              className="mt-1 min-h-11 w-full rounded-lg border border-line-strong bg-surface px-2 text-base"
            >
              <option value={automatic}>Automatisch</option>
              {cameras.map((camera) => (
                <option key={camera.deviceId} value={camera.deviceId}>
                  {camera.label}
                </option>
              ))}
            </select>
          </>
        )}
        <p className="mt-1 text-sm text-ink-secondary">
          Aktive Kamera: {track.label || "unbekannt"}
        </p>
        {range !== null && (
          <p className="text-sm text-ink-secondary">
            Zoombereich: <span className="tabular-nums">{zoomText(range.min)} bis {zoomText(range.max)}</span>
          </p>
        )}
      </div>
      {range !== null && zoomValue !== undefined && (
        <div>
          <label htmlFor={zoomId} className="block text-sm font-medium text-ink-secondary">
            Zoom <span className="tabular-nums">{zoomText(zoomValue)}</span>
          </label>
          <input
            id={zoomId}
            type="range"
            min={range.min}
            max={range.max}
            step={range.step}
            value={zoomValue}
            onChange={(event) => changeZoom(Number(event.target.value))}
            className="mt-1 min-h-11 w-full"
          />
        </div>
      )}
    </div>
  );
}
