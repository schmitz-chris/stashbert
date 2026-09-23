// Camera access: constraints, stream handling, capabilities (zoom, torch) and
// the persisted camera choice. Framework-independent.

const CAMERA_STORAGE_KEY = 'scanner-test.cameraId';

export interface CameraOption {
	deviceId: string;
	label: string;
}

export interface ZoomRange {
	min: number;
	max: number;
	step?: number;
}

// zoom and torch are not part of the TypeScript DOM typings yet.
type ExtendedCapabilities = MediaTrackCapabilities & {
	zoom?: ZoomRange;
	torch?: boolean | boolean[];
};
type ExtendedSettings = MediaTrackSettings & { zoom?: number };
type ExtendedConstraintSet = MediaTrackConstraintSet & { zoom?: number; torch?: boolean };

/** Default constraints, or a specific camera via `deviceId: { exact }` instead of `facingMode`. */
export function videoConstraints(deviceId: string | null): MediaTrackConstraints {
	const size = { width: { ideal: 1280 }, height: { ideal: 720 } };
	return deviceId
		? { deviceId: { exact: deviceId }, ...size }
		: { facingMode: 'environment', ...size };
}

export function openStream(deviceId: string | null): Promise<MediaStream> {
	return navigator.mediaDevices.getUserMedia({ video: videoConstraints(deviceId) });
}

/** True if getUserMedia failed because the requested camera does not exist (anymore). */
export function isMissingCamera(error: unknown): boolean {
	const name = (error as { name?: unknown } | null)?.name;
	return name === 'OverconstrainedError' || name === 'NotFoundError';
}

export function stopStream(stream: MediaStream): void {
	for (const track of stream.getTracks()) {
		track.stop();
	}
}

export async function listCameras(): Promise<CameraOption[]> {
	const devices = await navigator.mediaDevices.enumerateDevices();
	return devices
		.filter((device) => device.kind === 'videoinput')
		.map((device) => ({ deviceId: device.deviceId, label: device.label }));
}

function capabilities(track: MediaStreamTrack): ExtendedCapabilities {
	// getCapabilities is missing in some browsers.
	return typeof track.getCapabilities === 'function'
		? (track.getCapabilities() as ExtendedCapabilities)
		: {};
}

export function zoomRange(track: MediaStreamTrack): ZoomRange | null {
	const zoom = capabilities(track).zoom;
	return zoom ? { min: zoom.min, max: zoom.max, step: zoom.step } : null;
}

export function currentZoom(track: MediaStreamTrack): number | undefined {
	return (track.getSettings() as ExtendedSettings).zoom;
}

export function hasTorch(track: MediaStreamTrack): boolean {
	const torch = capabilities(track).torch;
	return torch === true || (Array.isArray(torch) && torch.includes(true));
}

export function applyZoom(track: MediaStreamTrack, zoom: number): Promise<void> {
	const advanced: ExtendedConstraintSet[] = [{ zoom }];
	return track.applyConstraints({ advanced });
}

export function applyTorch(track: MediaStreamTrack, torch: boolean): Promise<void> {
	const advanced: ExtendedConstraintSet[] = [{ torch }];
	return track.applyConstraints({ advanced });
}

export function loadCameraId(): string | null {
	try {
		return localStorage.getItem(CAMERA_STORAGE_KEY);
	} catch {
		return null;
	}
}

export function saveCameraId(deviceId: string): void {
	try {
		localStorage.setItem(CAMERA_STORAGE_KEY, deviceId);
	} catch {
		// Storage unavailable (e.g. private mode): the choice is simply not remembered.
	}
}

export function clearCameraId(): void {
	try {
		localStorage.removeItem(CAMERA_STORAGE_KEY);
	} catch {
		// See saveCameraId.
	}
}
