import { useEffect, useEffectEvent, useRef, useState } from "react";
import {
  isMissingDevice,
  listCameras,
  loadAutoCameraId,
  loadCameraId,
  saveAutoCameraId,
  saveCameraId,
  setZoom,
  zoomRange,
  type CameraOption,
} from "../lib/scanner/camera";
import { cameraToSwitchTo, multiLensKind, startZoom } from "../lib/scanner/cameraChoice";
import { Scanner, type ScanTimings } from "../lib/scanner/scanner";

/**
 * The state of the camera: starting, running, paused (stopped because
 * the page was hidden, a tap resumes it) or idle (stopped by an error).
 */
export type ScannerPhase = "idle" | "starting" | "running" | "paused";

interface ScannerState {
  phase: ScannerPhase;
  /** The error of the last start, in phase idle. */
  cameraError: unknown;
  /** The video track, in phase running. */
  track: MediaStreamTrack | null;
  /** The camera chosen in the menu, undefined while the app chooses. */
  chosenId: string | undefined;
}

/**
 * Runs a Scanner session (lib/scanner) on the video element of videoRef
 * while the component is mounted. The camera starts on mount and stops on
 * unmount and when the page is hidden. It starts with the camera chosen in
 * the menu; without one the app chooses a virtual camera with several
 * lenses, if the device has one (F25), else the default back camera.
 * onHit is called for every accepted code; it always sees the values of
 * the latest render, without restarting the session.
 */
export function useScanner(onHit: (code: string) => void) {
  const videoRef = useRef<HTMLVideoElement>(null);
  // Starts the camera of the running session; set by the effect.
  const startRef = useRef<(() => void) | null>(null);
  // The session of the effect, for timings().
  const scannerRef = useRef<Scanner | null>(null);
  const [state, setState] = useState<ScannerState>({
    phase: "starting",
    cameraError: null,
    track: null,
    chosenId: undefined,
  });
  const [cameras, setCameras] = useState<CameraOption[]>([]);
  const handleHit = useEffectEvent((code: string) => onHit(code));

  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return;
    }
    const scanner = new Scanner(video, {
      onHit: (code) => handleHit(code),
      onError: (message) => console.warn(`Scanner: ${message}`),
    });
    scannerRef.current = scanner;
    // Incremented by every start and stop, so that the result of an
    // older start is dropped.
    let attempt = 0;

    async function start() {
      const current = ++attempt;
      try {
        let chosenId = loadCameraId();
        const savedId = chosenId ?? loadAutoCameraId();
        let track: MediaStreamTrack | null;
        try {
          track = await scanner.start(savedId);
        } catch (error) {
          if (savedId === undefined || !isMissingDevice(error) || current !== attempt) {
            throw error;
          }
          // The saved camera no longer exists: forget it, open the default
          // one and choose again.
          chosenId = undefined;
          saveCameraId(undefined);
          saveAutoCameraId(undefined);
          track = await scanner.start();
        }
        // null: stopped while the camera was opening. The labels of the
        // cameras are known only now that the camera is allowed.
        const cameras = track === null ? [] : await listCameras().catch(() => []);
        if (track === null || current !== attempt) {
          return;
        }
        setCameras(cameras);

        // Without a chosen camera, switch to a better one, if the running
        // one is not the best already. Scanner.start stops the running
        // camera before it opens the next: iOS runs only one at a time.
        const better =
          chosenId === undefined ? cameraToSwitchTo(cameras, track.getSettings().deviceId) : null;
        if (better !== null) {
          saveAutoCameraId(better.deviceId);
          try {
            track = await scanner.start(better.deviceId);
          } catch (error) {
            if (current !== attempt) {
              throw error;
            }
            console.warn("Kamera:", error);
            saveAutoCameraId(undefined);
            track = await scanner.start();
          }
          if (track === null) {
            return;
          }
        }

        const zoom = startZoom(zoomRange(track), multiLensKind(track.label) !== null);
        if (zoom !== null) {
          await setZoom(track, zoom).catch((error: unknown) => console.warn("Zoom:", error));
        }
        if (current === attempt) {
          setState({ phase: "running", cameraError: null, track, chosenId });
        }
      } catch (error) {
        if (current === attempt) {
          setState({ phase: "idle", cameraError: error, track: null, chosenId: undefined });
        }
      }
    }

    function handleVisibilityChange() {
      if (document.visibilityState !== "hidden") {
        return;
      }
      attempt++;
      scanner.stop();
      setState((state) =>
        state.phase === "idle"
          ? state
          : { phase: "paused", cameraError: null, track: null, chosenId: state.chosenId },
      );
    }

    startRef.current = () => {
      setState((state) => ({ ...state, phase: "starting", cameraError: null, track: null }));
      void start();
    };
    void start();
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      attempt++;
      scanner.stop();
      startRef.current = null;
      scannerRef.current = null;
    };
  }, []);

  /** Starts the camera again, for example after a pause. */
  function restart() {
    startRef.current?.();
  }

  /** Saves deviceId as the chosen camera and starts it. */
  function selectCamera(deviceId: string) {
    saveCameraId(deviceId);
    restart();
  }

  /** Forgets the chosen camera and lets the app choose again from scratch. */
  function selectAutomatic() {
    saveCameraId(undefined);
    saveAutoCameraId(undefined);
    restart();
  }

  /** The timings of the running camera at this moment, null if none runs. */
  function timings(): ScanTimings | null {
    return scannerRef.current?.track ? scannerRef.current.timings(performance.now()) : null;
  }

  return { videoRef, ...state, cameras, restart, selectCamera, selectAutomatic, timings };
}
