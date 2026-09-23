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

const STORAGE_KEY = "scanner-test-r.cameraId";

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

export function loadCameraId(): string | undefined {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? undefined;
  } catch {
    return undefined;
  }
}

export function saveCameraId(deviceId: string | undefined): void {
  try {
    if (deviceId) {
      localStorage.setItem(STORAGE_KEY, deviceId);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    // Storage unavailable: the selection only lasts for this session.
  }
}
