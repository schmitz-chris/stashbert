// Barcode decoding with the barcode-detector ponyfill and a self-hosted zxing WASM file.
import {
	BarcodeDetector,
	prepareZXingModule,
	type BarcodeFormat
} from 'barcode-detector/ponyfill';

export type { BarcodeDetector };

const FORMATS: BarcodeFormat[] = ['ean_13', 'ean_8', 'upc_a'];

/** Share of the frame that is decoded: a horizontal strip in the middle. */
export const STRIP_WIDTH = 0.8;
export const STRIP_HEIGHT = 0.3;

export interface Region {
	x: number;
	y: number;
	width: number;
	height: number;
}

export interface Detection {
	code: string;
	durationMs: number;
}

/**
 * Creates the detector. prepareZXingModule must run before the first
 * `new BarcodeDetector()`, because the constructor loads the module right away.
 */
export function createDetector(wasmUrl: string): BarcodeDetector {
	prepareZXingModule({
		overrides: {
			locateFile: (path: string, prefix: string) => (path.endsWith('.wasm') ? wasmUrl : prefix + path)
		}
	});
	return new BarcodeDetector({ formats: FORMATS });
}

/** The centered strip for a frame of the given size (works for pixels and percentages). */
export function stripRegion(width: number, height: number): Region {
	return {
		x: (width * (1 - STRIP_WIDTH)) / 2,
		y: (height * (1 - STRIP_HEIGHT)) / 2,
		width: width * STRIP_WIDTH,
		height: height * STRIP_HEIGHT
	};
}

/** Copies the strip of the current video frame to the canvas and decodes it. */
export async function detectInStrip(
	detector: BarcodeDetector,
	video: HTMLVideoElement,
	canvas: HTMLCanvasElement
): Promise<Detection | null> {
	const { videoWidth, videoHeight } = video;
	if (videoWidth === 0 || videoHeight === 0) {
		return null;
	}

	const strip = stripRegion(videoWidth, videoHeight);
	const width = Math.round(strip.width);
	const height = Math.round(strip.height);
	if (canvas.width !== width || canvas.height !== height) {
		canvas.width = width;
		canvas.height = height;
	}
	const context = canvas.getContext('2d', { willReadFrequently: true });
	if (!context) {
		throw new Error('Canvas 2D nicht verfügbar');
	}
	context.drawImage(video, strip.x, strip.y, strip.width, strip.height, 0, 0, width, height);

	const started = performance.now();
	const barcodes = await detector.detect(canvas);
	const durationMs = performance.now() - started;

	return barcodes.length > 0 ? { code: barcodes[0].rawValue, durationMs } : null;
}
