import { useEffect, useEffectEvent, useRef, useState } from "react";
import {
  isMissingDevice,
  listCameras,
  loadCameraId,
  saveCameraId,
  type CameraOption,
} from "../lib/scanner/camera";
import { Scanner } from "../lib/scanner/scanner";

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
}

/**
 * Runs a Scanner session (lib/scanner) on the video element of videoRef
 * while the component is mounted. The camera starts on mount, with the
 * saved camera or the default one, and stops on unmount and when the page
 * is hidden. onHit is called for every accepted code; it always sees the
 * values of the latest render, without restarting the session.
 */
export function useScanner(onHit: (code: string) => void) {
  const videoRef = useRef<HTMLVideoElement>(null);
  // Starts the camera of the running session; set by the effect.
  const startRef = useRef<(() => void) | null>(null);
  const [state, setState] = useState<ScannerState>({
    phase: "starting",
    cameraError: null,
    track: null,
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
    // Incremented by every start and stop, so that the result of an
    // older start is dropped.
    let attempt = 0;

    async function start() {
      const current = ++attempt;
      const deviceId = loadCameraId();
      try {
        let track: MediaStreamTrack | null;
        try {
          track = await scanner.start(deviceId);
        } catch (error) {
          if (deviceId === undefined || !isMissingDevice(error) || current !== attempt) {
            throw error;
          }
          // The saved camera no longer exists: forget it, use the default.
          saveCameraId(undefined);
          track = await scanner.start();
        }
        // null: stopped while the camera was opening.
        if (track !== null) {
          setState({ phase: "running", cameraError: null, track });
          listCameras().then(setCameras, () => undefined);
        }
      } catch (error) {
        if (current === attempt) {
          setState({ phase: "idle", cameraError: error, track: null });
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
        state.phase === "idle" ? state : { phase: "paused", cameraError: null, track: null },
      );
    }

    startRef.current = () => {
      setState({ phase: "starting", cameraError: null, track: null });
      void start();
    };
    void start();
    document.addEventListener("visibilitychange", handleVisibilityChange);
    return () => {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      attempt++;
      scanner.stop();
      startRef.current = null;
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

  return { videoRef, ...state, cameras, restart, selectCamera };
}
