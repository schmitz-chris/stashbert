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
  onSelectCamera: (deviceId: string) => void;
}

/**
 * Camera choice and zoom of the scanner. Each is shown only if the device
 * offers it. The light has its own button on the camera image (TorchButton).
 */
export function CameraSettings({ track, cameras, onSelectCamera }: CameraSettingsProps) {
  // Zoom belongs to one track; a new track starts without it.
  const [zoom, setZoomState] = useState<{ track: MediaStreamTrack; value: number } | null>(null);
  const cameraId = useId();
  const zoomId = useId();

  if (track === null) {
    return <p className="mt-2 text-stone-700">Die Kamera läuft gerade nicht.</p>;
  }
  const range = zoomRange(track);
  const zoomValue = zoom?.track === track ? zoom.value : (currentZoom(track) ?? range?.min);

  if (cameras.length < 2 && range === null) {
    return <p className="mt-2 text-stone-700">Diese Kamera hat keine Einstellungen.</p>;
  }

  function changeZoom(value: number) {
    if (track === null) {
      return;
    }
    setZoomState({ track, value });
    setZoom(track, value).catch((error: unknown) => console.warn("Zoom:", error));
  }

  return (
    <div className="mt-3 flex flex-col gap-4">
      {cameras.length > 1 && (
        <div>
          <label htmlFor={cameraId} className="block text-sm font-medium text-stone-700">
            Kamera
          </label>
          <select
            id={cameraId}
            value={track.getSettings().deviceId}
            onChange={(event) => onSelectCamera(event.target.value)}
            className="mt-1 min-h-11 w-full rounded-lg border border-stone-300 bg-white px-2 text-base"
          >
            {cameras.map((camera) => (
              <option key={camera.deviceId} value={camera.deviceId}>
                {camera.label}
              </option>
            ))}
          </select>
        </div>
      )}
      {range !== null && zoomValue !== undefined && (
        <div>
          <label htmlFor={zoomId} className="block text-sm font-medium text-stone-700">
            Zoom <span className="tabular-nums">{zoomValue.toFixed(1)}×</span>
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
