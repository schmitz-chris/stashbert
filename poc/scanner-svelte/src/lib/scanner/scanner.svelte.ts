// Reactive state and flow of the scanner test page.
import {
	applyTorch,
	applyZoom,
	clearCameraId,
	currentZoom,
	hasTorch,
	isMissingCamera,
	listCameras,
	loadCameraId,
	openStream,
	saveCameraId,
	stopStream,
	zoomRange,
	type CameraOption,
	type ZoomRange
} from './camera';
import { createDetector, detectInStrip, type BarcodeDetector, type Detection } from './decoder';
import {
	playAudioElement,
	playTone,
	primeAudioElement,
	setPlaybackAudioSession
} from './feedback';
import { addRecent, isDuplicate, type Hit } from './hits';
import { ReadLoop } from './read-loop';

const FLASH_MS = 300;
const ERROR_LIMIT = 5;

/** idle: never started, starting: camera is opening, running: scanning, paused: stopped in the background. */
export type Phase = 'idle' | 'starting' | 'running' | 'paused';

export interface ErrorEntry {
	id: number;
	message: string;
}

export class Scanner {
	phase = $state<Phase>('idle');
	cameras = $state.raw<CameraOption[]>([]);
	cameraId = $state('');
	cameraLabel = $state('');
	zoom = $state.raw<ZoomRange | null>(null);
	zoomValue = $state(1);
	torchAvailable = $state(false);
	torchOn = $state(false);
	useAudioElement = $state(false);
	lastCode = $state('');
	lastDurationMs = $state(0);
	hitCount = $state(0);
	recent = $state.raw<Hit[]>([]);
	flash = $state(false);
	errors = $state.raw<ErrorEntry[]>([]);

	#wasmUrl: string;
	#video: HTMLVideoElement | null = null;
	#audio: HTMLAudioElement | null = null;
	#canvas: HTMLCanvasElement | null = null;
	#stream: MediaStream | null = null;
	#track: MediaStreamTrack | null = null;
	#audioContext: AudioContext | null = null;
	#detector: BarcodeDetector | null = null;
	#wakeLock: WakeLockSentinel | null = null;
	#flashTimer: ReturnType<typeof setTimeout> | undefined;
	#openAttempt = 0;
	#errorCount = 0;
	#loop = new ReadLoop(() => this.#scanOnce());

	constructor(wasmUrl: string) {
		this.#wasmUrl = wasmUrl;
	}

	attachVideo = (video: HTMLVideoElement) => {
		this.#video = video;
		return () => {
			this.#video = null;
		};
	};

	attachAudio = (audio: HTMLAudioElement) => {
		this.#audio = audio;
		return () => {
			this.#audio = null;
		};
	};

	/** Tap on "Scannen starten". */
	start = () => {
		if (this.phase !== 'idle') return;
		this.#unlockAudio();
		void this.#open(loadCameraId(), 'idle');
	};

	/** Tap on "Tippen zum Fortsetzen". */
	resume = () => {
		if (this.phase !== 'paused') return;
		this.#unlockAudio();
		void this.#open(loadCameraId(), 'paused');
	};

	/** Tap on the checkbox "Ton über Audio-Element". */
	primeAudio = () => {
		if (this.#audio) primeAudioElement(this.#audio);
	};

	selectCamera = async (deviceId: string) => {
		this.cameraId = deviceId;
		saveCameraId(deviceId);
		if (this.phase !== 'running') return;
		// Stop the old stream before the new one is opened.
		this.#stopCamera();
		await this.#open(deviceId, 'idle');
	};

	setZoom = async (value: number) => {
		this.zoomValue = value;
		if (!this.#track) return;
		try {
			await applyZoom(this.#track, value);
		} catch (error) {
			this.#report(error);
		}
	};

	toggleTorch = async () => {
		if (!this.#track) return;
		const next = !this.torchOn;
		try {
			await applyTorch(this.#track, next);
			this.torchOn = next;
		} catch (error) {
			this.#report(error);
		}
	};

	handleVisibilityChange = () => {
		if (document.visibilityState !== 'hidden') return;
		if (this.phase === 'running' || this.phase === 'starting') {
			this.#stopCamera();
			this.phase = 'paused';
		}
	};

	/** Must run synchronously inside the tap handler. */
	#unlockAudio() {
		setPlaybackAudioSession();
		if (!this.#audioContext) {
			this.#audioContext = new AudioContext();
		}
		if (this.#audioContext.state !== 'running') {
			this.#audioContext.resume().catch((error: unknown) => this.#report(error));
		}
		this.primeAudio();
	}

	async #open(deviceId: string | null, phaseOnError: Phase) {
		const attempt = ++this.#openAttempt;
		this.phase = 'starting';
		try {
			let stream: MediaStream;
			try {
				stream = await openStream(deviceId);
			} catch (error) {
				if (!deviceId || !isMissingCamera(error)) throw error;
				// The saved camera is gone: forget it and use the defaults.
				clearCameraId();
				stream = await openStream(null);
			}
			if (attempt !== this.#openAttempt) {
				// Stopped (e.g. sent to the background) while opening.
				stopStream(stream);
				return;
			}

			this.#stream = stream;
			this.#track = stream.getVideoTracks()[0] ?? null;
			if (this.#video) {
				this.#video.srcObject = stream;
				await this.#video.play();
			}
			this.#readTrack();
			this.cameras = await listCameras();
			this.#detector ??= createDetector(this.#wasmUrl);
			if (attempt !== this.#openAttempt) return;

			this.phase = 'running';
			this.#loop.start();
			void this.#requestWakeLock();
		} catch (error) {
			if (attempt !== this.#openAttempt) return;
			this.#stopCamera();
			this.phase = phaseOnError;
			this.#report(error);
		}
	}

	#readTrack() {
		const track = this.#track;
		this.cameraLabel = track?.label ?? '';
		this.cameraId = track?.getSettings().deviceId ?? '';
		this.zoom = track ? zoomRange(track) : null;
		this.zoomValue = (track && currentZoom(track)) ?? this.zoom?.min ?? 1;
		this.torchAvailable = track ? hasTorch(track) : false;
		this.torchOn = false;
	}

	#stopCamera() {
		this.#openAttempt += 1;
		this.#loop.stop();
		if (this.#stream) stopStream(this.#stream);
		this.#stream = null;
		this.#track = null;
		if (this.#video) this.#video.srcObject = null;
		this.zoom = null;
		this.torchAvailable = false;
		this.torchOn = false;
		this.#releaseWakeLock();
	}

	async #scanOnce() {
		if (!this.#detector || !this.#video) return;
		this.#canvas ??= document.createElement('canvas');
		try {
			const detection = await detectInStrip(this.#detector, this.#video, this.#canvas);
			if (detection) this.#accept(detection);
		} catch (error) {
			this.#report(error);
		}
	}

	#accept({ code, durationMs }: Detection) {
		const now = Date.now();
		if (isDuplicate(code, now, this.recent[0])) return;

		this.hitCount += 1;
		this.lastCode = code;
		this.lastDurationMs = Math.round(durationMs);
		this.recent = addRecent(this.recent, { id: this.hitCount, code, at: now });
		this.#signal();
	}

	#signal() {
		this.flash = true;
		clearTimeout(this.#flashTimer);
		this.#flashTimer = setTimeout(() => {
			this.flash = false;
		}, FLASH_MS);

		if (this.useAudioElement) {
			if (this.#audio) {
				playAudioElement(this.#audio).catch((error: unknown) => this.#report(error));
			}
		} else if (this.#audioContext) {
			playTone(this.#audioContext);
		}
	}

	async #requestWakeLock() {
		if (!('wakeLock' in navigator)) return;
		try {
			const sentinel = await navigator.wakeLock.request('screen');
			if (this.phase === 'running') {
				this.#wakeLock = sentinel;
			} else {
				void sentinel.release();
			}
		} catch (error) {
			this.#report(error);
		}
	}

	#releaseWakeLock() {
		this.#wakeLock?.release().catch((error: unknown) => this.#report(error));
		this.#wakeLock = null;
	}

	#report(error: unknown) {
		const message = error instanceof Error ? `${error.name}: ${error.message}` : String(error);
		if (this.errors[0]?.message === message) return;
		this.#errorCount += 1;
		this.errors = [{ id: this.#errorCount, message }, ...this.errors].slice(0, ERROR_LIMIT);
	}
}
