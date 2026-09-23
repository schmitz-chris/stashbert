// Barcode decoding of the region of interest with the barcode-detector ponyfill.
import { BarcodeDetector, prepareZXingModule } from "barcode-detector/ponyfill";

// Horizontal strip in the middle of the frame, as fractions of width and height.
export const ROI = { left: 0.1, top: 0.35, width: 0.8, height: 0.3 } as const;

// Serve zxing_reader.wasm from our own origin (copied by `npm run copy:wasm`).
// This must run before the first BarcodeDetector is constructed, because the
// constructor starts loading the module with the overrides set at that time.
prepareZXingModule({
  overrides: {
    locateFile: (path: string, prefix: string) =>
      path.endsWith(".wasm") ? `${import.meta.env.BASE_URL}${path}` : prefix + path,
  },
});

export interface DecodeResult {
  code: string;
  ms: number;
}

export class RoiDecoder {
  private readonly detector = new BarcodeDetector({ formats: ["ean_13", "ean_8", "upc_a"] });
  private readonly canvas = document.createElement("canvas");
  private readonly context = this.canvas.getContext("2d", { willReadFrequently: true });

  async decode(video: HTMLVideoElement): Promise<DecodeResult | null> {
    const { videoWidth, videoHeight } = video;
    if (!this.context || videoWidth === 0 || videoHeight === 0) {
      return null;
    }
    const sx = Math.round(videoWidth * ROI.left);
    const sy = Math.round(videoHeight * ROI.top);
    const width = Math.round(videoWidth * ROI.width);
    const height = Math.round(videoHeight * ROI.height);
    if (this.canvas.width !== width || this.canvas.height !== height) {
      this.canvas.width = width;
      this.canvas.height = height;
    }
    this.context.drawImage(video, sx, sy, width, height, 0, 0, width, height);
    const image = this.context.getImageData(0, 0, width, height);

    const started = performance.now();
    const barcodes = await this.detector.detect(image);
    const ms = performance.now() - started;

    return barcodes.length > 0 ? { code: barcodes[0].rawValue, ms } : null;
  }
}
