// Scanner session: camera stream, read loop, repeat filter and wake lock.
// Framework independent; a view drives it with start() and stop().
import { openCamera, stopStream } from "./camera";
import { RoiDecoder } from "./decoder";
import { accept, EMPTY_DEDUPE } from "./dedupe";
import type { DedupeState } from "./dedupe";
import { startLoop } from "./loop";
import { addRead, summarizeReads } from "./timings";
import type { ReadSample, ReadSummary } from "./timings";
import { releaseWakeLock, requestWakeLock } from "./wakelock";

export interface ScannerEvents {
  onHit(code: string): void;
  onError(message: string): void;
}

/** What the camera menu shows about the running session (F25). */
export interface ScanTimings extends ReadSummary {
  /** Size of the camera image in px, 0 before the first frame. */
  width: number;
  height: number;
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.name && error.name !== "Error" ? `${error.name}: ${error.message}` : error.message;
  }
  return String(error);
}

export class Scanner {
  private readonly video: HTMLVideoElement;
  private readonly events: ScannerEvents;
  private seen: DedupeState = EMPTY_DEDUPE;
  private decoder: RoiDecoder | null = null;
  private stream: MediaStream | null = null;
  private stopLoop: (() => void) | null = null;
  private wakeLock: WakeLockSentinel | null = null;
  // The last reads of the running session, for timings().
  private reads: readonly ReadSample[] = [];
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
    this.reads = [];

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
    releaseWakeLock(this.wakeLock);
    this.wakeLock = null;
  }

  /** The image size and read timings of the running session at now. */
  timings(now: number): ScanTimings {
    return {
      width: this.video.videoWidth,
      height: this.video.videoHeight,
      ...summarizeReads(this.reads, now),
    };
  }

  private async tick(session: number): Promise<void> {
    if (!this.decoder) {
      return;
    }
    const startedAt = performance.now();
    try {
      const code = await this.decoder.decode(this.video);
      if (session !== this.session) {
        return;
      }
      this.reads = addRead(this.reads, { startedAt, durationMs: performance.now() - startedAt });
      if (code === null) {
        return;
      }
      const { state, accepted } = accept(this.seen, code, performance.now());
      this.seen = state;
      if (accepted) {
        this.events.onHit(code);
      }
    } catch (error) {
      if (session === this.session) {
        this.events.onError(errorMessage(error));
      }
    }
  }

  private async acquireWakeLock(session: number): Promise<void> {
    try {
      const sentinel = await requestWakeLock();
      if (session === this.session) {
        this.wakeLock = sentinel;
      } else {
        await sentinel?.release();
      }
    } catch (error) {
      this.events.onError(`Wake Lock: ${errorMessage(error)}`);
    }
  }
}
