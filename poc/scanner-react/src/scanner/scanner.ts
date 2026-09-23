// Scanner session: camera stream, read loop and wake lock.
import { openCamera, stopStream } from "./camera";
import { RoiDecoder } from "./decoder";
import { errorMessage } from "./environment";
import { RepeatFilter } from "./hits";
import { startLoop } from "./loop";

export interface ScannerEvents {
  onHit(code: string, ms: number): void;
  onError(message: string): void;
}

export class Scanner {
  private readonly video: HTMLVideoElement;
  private readonly events: ScannerEvents;
  private readonly repeats = new RepeatFilter();
  private decoder: RoiDecoder | null = null;
  private stream: MediaStream | null = null;
  private stopLoop: (() => void) | null = null;
  private wakeLock: WakeLockSentinel | null = null;
  // Incremented by stop(), so that late results of an old session are dropped.
  private session = 0;

  constructor(video: HTMLVideoElement, events: ScannerEvents) {
    this.video = video;
    this.events = events;
  }

  get track(): MediaStreamTrack | null {
    return this.stream?.getVideoTracks()[0] ?? null;
  }

  // Stops a running stream first, then opens the camera. Returns null if
  // stop() was called while the camera was opening.
  async start(deviceId?: string): Promise<MediaStreamTrack | null> {
    this.stop();
    const session = this.session;
    this.decoder ??= new RoiDecoder();

    const stream = await openCamera(deviceId);
    if (session !== this.session) {
      stopStream(stream);
      return null;
    }
    this.stream = stream;
    this.video.srcObject = stream;
    try {
      await this.video.play();
    } catch (error) {
      if (session !== this.session) {
        return null;
      }
      this.stop();
      throw error;
    }
    if (session !== this.session) {
      return null;
    }

    this.stopLoop = startLoop(() => this.tick(session));
    void this.acquireWakeLock(session);
    return this.track;
  }

  stop(): void {
    this.session++;
    this.stopLoop?.();
    this.stopLoop = null;
    if (this.stream) {
      stopStream(this.stream);
      this.stream = null;
    }
    this.video.srcObject = null;
    void this.wakeLock?.release().catch(() => undefined);
    this.wakeLock = null;
  }

  private async tick(session: number): Promise<void> {
    if (!this.decoder) {
      return;
    }
    try {
      const result = await this.decoder.decode(this.video);
      if (session !== this.session || !result) {
        return;
      }
      if (this.repeats.accept(result.code, performance.now())) {
        this.events.onHit(result.code, result.ms);
      }
    } catch (error) {
      if (session === this.session) {
        this.events.onError(errorMessage(error));
      }
    }
  }

  private async acquireWakeLock(session: number): Promise<void> {
    if (!("wakeLock" in navigator)) {
      return;
    }
    try {
      const sentinel = await navigator.wakeLock.request("screen");
      if (session === this.session) {
        this.wakeLock = sentinel;
      } else {
        await sentinel.release();
      }
    } catch (error) {
      this.events.onError(`Wake Lock: ${errorMessage(error)}`);
    }
  }
}
