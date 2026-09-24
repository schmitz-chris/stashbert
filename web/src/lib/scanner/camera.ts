// Camera access: open and stop streams, list cameras, zoom and torch.

export interface ZoomRange {
  min: number;
  max: number;
  step: number;
}

export interface CameraOption {
  deviceId: string;
  label: string;
}

// zoom and torch are not part of the TypeScript DOM typings (Image Capture spec).
type ImageCapabilities = MediaTrackCapabilities & {
  zoom?: ZoomRange;
  torch?: boolean | boolean[];
};
type ImageConstraintSet = MediaTrackConstraintSet & {
  zoom?: number;
  torch?: boolean;
};
type ImageSettings = MediaTrackSettings & { zoom?: number };

// The camera chosen in the menu, and apart from it the camera the app chose
// itself (F25), so that the next start opens it at once.
const STORAGE_KEY = "stashbert.cameraId";
const AUTO_STORAGE_KEY = "stashbert.autoCameraId";

// Without a chosen camera, the default back camera is used.
export function buildConstraints(deviceId?: string): MediaStreamConstraints {
  const size = { width: { ideal: 1280 }, height: { ideal: 720 } };
  if (deviceId) {
    return { video: { deviceId: { exact: deviceId }, ...size } };
  }
  return { video: { facingMode: "environment", ...size } };
}

export function openCamera(deviceId?: string): Promise<MediaStream> {
  return navigator.mediaDevices.getUserMedia(buildConstraints(deviceId));
}

export function stopStream(stream: MediaStream): void {
  for (const track of stream.getTracks()) {
    track.stop();
  }
}

// True if openCamera failed because the chosen camera no longer exists. The
// caller then forgets the saved camera and opens the default one.
export function isMissingDevice(error: unknown): boolean {
  const name = (error as { name?: unknown } | null)?.name;
  return name === "OverconstrainedError" || name === "NotFoundError";
}

export async function listCameras(): Promise<CameraOption[]> {
  const devices = await navigator.mediaDevices.enumerateDevices();
  return devices
    .filter((device) => device.kind === "videoinput")
    .map((device, index) => ({
      deviceId: device.deviceId,
      label: device.label || `Kamera ${index + 1}`,
    }));
}

function capabilities(track: MediaStreamTrack): ImageCapabilities {
  return typeof track.getCapabilities === "function" ? track.getCapabilities() : {};
}

export function zoomRange(track: MediaStreamTrack): ZoomRange | null {
  const zoom = capabilities(track).zoom;
  return zoom ? { min: zoom.min, max: zoom.max, step: zoom.step } : null;
}

export function currentZoom(track: MediaStreamTrack): number | undefined {
  return (track.getSettings() as ImageSettings).zoom;
}

// WebKit and Chromium report a boolean, the Image Capture spec a list of booleans.
export function hasTorch(track: MediaStreamTrack): boolean {
  const torch = capabilities(track).torch;
  return Array.isArray(torch) ? torch.includes(true) : torch === true;
}

function applyImageConstraint(track: MediaStreamTrack, set: ImageConstraintSet): Promise<void> {
  return track.applyConstraints({ advanced: [set] });
}

export function setZoom(track: MediaStreamTrack, zoom: number): Promise<void> {
  return applyImageConstraint(track, { zoom });
}

export function setTorch(track: MediaStreamTrack, torch: boolean): Promise<void> {
  return applyImageConstraint(track, { torch });
}

function load(key: string): string | undefined {
  try {
    return localStorage.getItem(key) ?? undefined;
  } catch {
    return undefined;
  }
}

function save(key: string, deviceId: string | undefined): void {
  try {
    if (deviceId) {
      localStorage.setItem(key, deviceId);
    } else {
      localStorage.removeItem(key);
    }
  } catch {
    // Storage unavailable: the selection only lasts for this session.
  }
}

export function loadCameraId(): string | undefined {
  return load(STORAGE_KEY);
}

export function saveCameraId(deviceId: string | undefined): void {
  save(STORAGE_KEY, deviceId);
}

export function loadAutoCameraId(): string | undefined {
  return load(AUTO_STORAGE_KEY);
}

export function saveAutoCameraId(deviceId: string | undefined): void {
  save(AUTO_STORAGE_KEY, deviceId);
}
