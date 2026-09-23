import { useEffect, useRef, useState } from "react";
import type { ChangeEvent } from "react";
import {
  bookScan,
  feedbackFor,
  loadMode,
  loadShoppingList,
  reverseMovement,
  saveMode,
} from "./booking";
import type { Color, Feedback, Kind, Outcome, ShoppingItem } from "./booking";
import {
  currentZoom,
  hasTorch,
  listCameras,
  loadCameraId,
  saveCameraId,
  setTorch,
  setZoom,
  zoomRange,
} from "./scanner/camera";
import type { CameraOption, ZoomRange } from "./scanner/camera";
import { ROI } from "./scanner/decoder";
import { displayMode, errorMessage } from "./scanner/environment";
import { AudioFeedback, playAudioElement, primeAudioElement, TONES } from "./scanner/feedback";
import { Scanner } from "./scanner/scanner";

type Phase = "idle" | "starting" | "running" | "paused";

interface Booking {
  time: string;
  code: string;
  feedback: Feedback;
}

const FLASH_MS = 300;
const MESSAGE_MS = 2000;
const MAX_ERRORS = 5;
const MAX_BOOKINGS = 10;

const flashClass: Record<Color, string> = {
  green: "bg-green-500/70",
  blue: "bg-sky-500/70",
  yellow: "bg-amber-400/70",
  red: "bg-red-600/70",
};

const messageClass: Record<Color, string> = {
  green: "bg-green-600 text-white",
  blue: "bg-sky-600 text-white",
  yellow: "bg-amber-400 text-slate-900",
  red: "bg-red-600 text-white",
};

const roiStyle = {
  left: `${ROI.left * 100}%`,
  top: `${ROI.top * 100}%`,
  width: `${ROI.width * 100}%`,
  height: `${ROI.height * 100}%`,
};

function isMissingDevice(error: unknown): boolean {
  const name = (error as { name?: unknown } | null)?.name;
  return name === "OverconstrainedError" || name === "NotFoundError";
}

export default function App() {
  const videoRef = useRef<HTMLVideoElement>(null);
  const audioRef = useRef<HTMLAudioElement>(null);
  const scannerRef = useRef<Scanner | null>(null);
  const useAudioElementRef = useRef(false);
  const flashTimerRef = useRef<number | undefined>(undefined);
  const [audioFeedback] = useState(() => new AudioFeedback());
  const [mode] = useState(displayMode);

  const [phase, setPhase] = useState<Phase>("idle");
  const [cameras, setCameras] = useState<CameraOption[]>([]);
  const [cameraId, setCameraId] = useState<string | undefined>(loadCameraId);
  const [cameraLabel, setCameraLabel] = useState("");
  const [zoom, setZoomState] = useState<{ range: ZoomRange; value: number } | null>(null);
  const [torchAvailable, setTorchAvailable] = useState(false);
  const [torchOn, setTorchOn] = useState(false);
  const [useAudioElement, setUseAudioElement] = useState(false);
  const [lastHit, setLastHit] = useState<{ code: string; ms: number } | null>(null);
  const [hitCount, setHitCount] = useState(0);
  const [kind, setKind] = useState<Kind>(loadMode);
  const kindRef = useRef(kind);
  const messageTimerRef = useRef<number | undefined>(undefined);
  const [message, setMessage] = useState<Feedback | null>(null);
  const [bookings, setBookings] = useState<Booking[]>([]);
  const [lastMovementId, setLastMovementId] = useState<string | null>(null);
  const [shopping, setShopping] = useState<ShoppingItem[] | null>(null);
  const [flash, setFlash] = useState<Color | null>(null);
  const [errors, setErrors] = useState<string[]>([]);
  const [videoSize, setVideoSize] = useState({ width: 16, height: 9 });

  // addError and handleHit only use state setters and refs, so the versions
  // captured by the Scanner on the first start stay valid.
  function addError(message: string) {
    setErrors((current) =>
      current[0] === message ? current : [message, ...current].slice(0, MAX_ERRORS),
    );
  }

  // Shows feedback as flash, sound and a large message for MESSAGE_MS.
  function showFeedback(code: string, feedback: Feedback) {
    setFlash(feedback.color);
    window.clearTimeout(flashTimerRef.current);
    flashTimerRef.current = window.setTimeout(() => setFlash(null), FLASH_MS);
    setMessage(feedback);
    window.clearTimeout(messageTimerRef.current);
    messageTimerRef.current = window.setTimeout(() => setMessage(null), MESSAGE_MS);
    setBookings((entries) =>
      [{ time: new Date().toLocaleTimeString("de-DE"), code, feedback }, ...entries].slice(
        0,
        MAX_BOOKINGS,
      ),
    );
    if (useAudioElementRef.current && audioRef.current) {
      playAudioElement(audioRef.current).catch((error: unknown) =>
        addError(`Audio-Element: ${errorMessage(error)}`),
      );
    } else {
      audioFeedback.play(TONES[feedback.sound]);
    }
  }

  function refreshShopping() {
    loadShoppingList().then(setShopping, (error: unknown) =>
      addError(`Einkaufsliste: ${errorMessage(error)}`),
    );
  }

  function handleOutcome(code: string, outcome: Outcome, feedback: Feedback) {
    if (outcome.ok) {
      setLastMovementId(outcome.result.movementId);
      refreshShopping();
    } else if (outcome.code !== "unknown_barcode" && outcome.code !== "stock_already_zero") {
      addError(`${code}: ${outcome.status} ${outcome.code} ${outcome.detail}`);
    }
    showFeedback(code, feedback);
  }

  function handleHit(code: string, ms: number) {
    setLastHit({ code, ms });
    setHitCount((count) => count + 1);
    const bookedKind = kindRef.current;
    void bookScan(code, bookedKind).then((outcome) =>
      handleOutcome(code, outcome, feedbackFor(bookedKind, outcome)),
    );
  }

  function handleKind(next: Kind) {
    kindRef.current = next;
    setKind(next);
    saveMode(next);
  }

  function handleUndo() {
    const id = lastMovementId;
    if (!id) {
      return;
    }
    setLastMovementId(null);
    void reverseMovement(id).then((outcome) => {
      const feedback: Feedback = outcome.ok
        ? { color: "yellow", sound: "warn", text: `Rückgängig: ${outcome.result.message}` }
        : feedbackFor(kindRef.current, outcome);
      if (outcome.ok) {
        refreshShopping();
      } else {
        addError(`Rückgängig: ${outcome.status} ${outcome.code} ${outcome.detail}`);
      }
      showFeedback("Storno", feedback);
    });
  }

  function getScanner(): Scanner {
    scannerRef.current ??= new Scanner(videoRef.current!, {
      onHit: handleHit,
      onError: addError,
    });
    return scannerRef.current;
  }

  function showTrack(track: MediaStreamTrack) {
    setCameraLabel(track.label);
    setCameraId(track.getSettings().deviceId);
    const range = zoomRange(track);
    setZoomState(range ? { range, value: currentZoom(track) ?? range.min } : null);
    setTorchAvailable(hasTorch(track));
  }

  async function startScanner(deviceId: string | undefined) {
    setPhase("starting");
    setTorchOn(false);
    const scanner = getScanner();
    try {
      let track: MediaStreamTrack | null;
      try {
        track = await scanner.start(deviceId);
      } catch (error) {
        if (!deviceId || !isMissingDevice(error)) {
          throw error;
        }
        // The saved camera no longer exists: forget it and use the default.
        addError(`Gespeicherte Kamera nicht verfügbar: ${errorMessage(error)}`);
        saveCameraId(undefined);
        track = await scanner.start();
      }
      if (!track) {
        return;
      }
      if (document.visibilityState === "hidden") {
        scanner.stop();
        setPhase("paused");
        return;
      }
      showTrack(track);
      setPhase("running");
      setCameras(await listCameras());
    } catch (error) {
      addError(errorMessage(error));
      setPhase("idle");
    }
  }

  // Start and resume: audio has to be unlocked synchronously in the tap.
  function handleStart() {
    if (phase === "starting") {
      return;
    }
    audioFeedback.unlock();
    if (useAudioElementRef.current && audioRef.current) {
      primeAudioElement(audioRef.current);
    }
    void startScanner(cameraId);
  }

  function handleCameraChange(event: ChangeEvent<HTMLSelectElement>) {
    const id = event.target.value;
    saveCameraId(id);
    setCameraId(id);
    if (phase === "running") {
      scannerRef.current?.stop();
      void startScanner(id);
    }
  }

  function handleZoom(event: ChangeEvent<HTMLInputElement>) {
    const value = Number(event.target.value);
    setZoomState((current) => current && { ...current, value });
    const track = scannerRef.current?.track;
    if (track) {
      setZoom(track, value).catch((error: unknown) => addError(`Zoom: ${errorMessage(error)}`));
    }
  }

  function handleTorch() {
    const track = scannerRef.current?.track;
    if (!track) {
      return;
    }
    const next = !torchOn;
    setTorch(track, next).then(
      () => setTorchOn(next),
      (error: unknown) => addError(`Licht: ${errorMessage(error)}`),
    );
  }

  function handleAudioElementChange(event: ChangeEvent<HTMLInputElement>) {
    const checked = event.target.checked;
    useAudioElementRef.current = checked;
    setUseAudioElement(checked);
    if (checked && audioRef.current) {
      primeAudioElement(audioRef.current);
    }
  }

  function handleVideoSize() {
    const video = videoRef.current;
    if (video && video.videoWidth > 0 && video.videoHeight > 0) {
      setVideoSize({ width: video.videoWidth, height: video.videoHeight });
    }
  }

  useEffect(() => {
    function handleVisibilityChange() {
      const scanner = scannerRef.current;
      if (document.visibilityState === "hidden" && scanner?.track) {
        scanner.stop();
        setTorchOn(false);
        setPhase("paused");
      }
    }
    document.addEventListener("visibilitychange", handleVisibilityChange);
    refreshShopping();
    return () => {
      document.removeEventListener("visibilitychange", handleVisibilityChange);
      scannerRef.current?.stop();
      window.clearTimeout(flashTimerRef.current);
      window.clearTimeout(messageTimerRef.current);
    };
    // refreshShopping only uses state setters, so running it once is enough.
  }, []);

  const showVideo = phase === "starting" || phase === "running";
  const bigButton =
    "min-h-16 w-full rounded-xl bg-sky-600 px-6 text-xl font-semibold text-white active:bg-sky-700";

  return (
    <main className="mx-auto flex max-w-xl flex-col gap-4 p-4 text-slate-900">
      <h1 className="text-2xl font-bold">StashBert Scanner (Test)</h1>

      <div className="grid grid-cols-2 gap-2" role="group" aria-label="Modus">
        <button
          type="button"
          className={`min-h-14 rounded-xl text-lg font-semibold ${kind === "add" ? "bg-green-600 text-white" : "bg-slate-200"}`}
          aria-pressed={kind === "add"}
          onClick={() => handleKind("add")}
        >
          Einlagern
        </button>
        <button
          type="button"
          className={`min-h-14 rounded-xl text-lg font-semibold ${kind === "consume" ? "bg-sky-600 text-white" : "bg-slate-200"}`}
          aria-pressed={kind === "consume"}
          onClick={() => handleKind("consume")}
        >
          Entnehmen
        </button>
      </div>

      {phase === "idle" && (
        <button type="button" className={bigButton} onClick={handleStart}>
          Scannen starten
        </button>
      )}
      {phase === "paused" && (
        <button type="button" className={bigButton} onClick={handleStart}>
          Tippen zum Fortsetzen
        </button>
      )}

      <div
        className="relative mx-auto overflow-hidden rounded-xl bg-black"
        style={{ width: `min(100%, calc(60vh * ${videoSize.width} / ${videoSize.height}))` }}
        hidden={!showVideo}
      >
        <video
          ref={videoRef}
          className="block h-auto w-full"
          playsInline
          muted
          onLoadedMetadata={handleVideoSize}
          onResize={handleVideoSize}
        />
        <div
          className="pointer-events-none absolute rounded border-2 border-white shadow-[0_0_0_2px_rgb(0_0_0/0.6)]"
          style={roiStyle}
        />
        {flash && <div className={`pointer-events-none absolute inset-0 ${flashClass[flash]}`} />}
      </div>

      {message && (
        <p className={`rounded-xl p-4 text-center text-2xl font-bold ${messageClass[message.color]}`}>
          {message.text}
        </p>
      )}
      {lastMovementId && (
        <button
          type="button"
          className="min-h-11 rounded-lg bg-slate-200 px-4 font-semibold"
          onClick={handleUndo}
        >
          Letzte Buchung rückgängig
        </button>
      )}

      <section className="flex flex-col gap-3">
        {cameras.length > 0 && (
          <label className="flex min-h-11 items-center gap-3">
            <span className="w-16 shrink-0">Kamera</span>
            <select
              className="min-h-11 w-full rounded-lg border border-slate-300 bg-white px-2"
              value={cameraId ?? ""}
              onChange={handleCameraChange}
              disabled={phase === "starting"}
            >
              {cameras.map((camera) => (
                <option key={camera.deviceId} value={camera.deviceId}>
                  {camera.label}
                </option>
              ))}
            </select>
          </label>
        )}
        {phase === "running" && zoom && (
          <label className="flex min-h-11 items-center gap-3">
            <span className="w-16 shrink-0">Zoom</span>
            <input
              type="range"
              className="w-full"
              min={zoom.range.min}
              max={zoom.range.max}
              step={zoom.range.step}
              value={zoom.value}
              onChange={handleZoom}
            />
            <span className="w-12 text-right tabular-nums">{zoom.value.toFixed(1)}</span>
          </label>
        )}
        {phase === "running" && torchAvailable && (
          <button
            type="button"
            className={`min-h-11 rounded-lg px-4 font-semibold ${torchOn ? "bg-amber-400" : "bg-slate-200"}`}
            onClick={handleTorch}
            aria-pressed={torchOn}
          >
            Licht
          </button>
        )}
        <label className="flex min-h-11 items-center gap-3">
          <input
            type="checkbox"
            className="size-6"
            checked={useAudioElement}
            onChange={handleAudioElementChange}
          />
          Ton über Audio-Element
        </label>
        <audio ref={audioRef} src={`${import.meta.env.BASE_URL}beep.wav`} preload="auto" />
      </section>

      <section className="rounded-xl bg-slate-100 p-4">
        {lastHit ? (
          <p className="break-all font-mono text-4xl font-bold">{lastHit.code}</p>
        ) : (
          <p className="text-xl text-slate-500">Noch kein Treffer</p>
        )}
        <p className="mt-2 tabular-nums">
          Dauer: {lastHit ? `${Math.round(lastHit.ms)} ms` : "n/a"} · Treffer: {hitCount}
        </p>
      </section>

      <section>
        <h2 className="text-lg font-semibold">Letzte Buchungen</h2>
        <ol className="mt-2 flex flex-col gap-1">
          {bookings.map((entry, index) => (
            <li key={`${entry.time}-${entry.code}-${index}`} className="flex gap-2">
              <span className={`mt-1.5 size-3 shrink-0 rounded-full ${messageClass[entry.feedback.color]}`} />
              <span className="tabular-nums">{entry.time}</span>
              <span className="break-all">{entry.feedback.text}</span>
            </li>
          ))}
        </ol>
      </section>

      <section>
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">Einkaufsliste</h2>
          <button
            type="button"
            className="min-h-11 rounded-lg bg-slate-200 px-4"
            onClick={refreshShopping}
          >
            Neu laden
          </button>
        </div>
        {shopping === null ? (
          <p className="text-slate-500">Nicht geladen</p>
        ) : shopping.length === 0 ? (
          <p className="text-slate-500">Nichts fehlt</p>
        ) : (
          <ul className="mt-2 flex flex-col gap-1">
            {shopping.map((item) => (
              <li key={item.product_id}>
                {item.missing} × {item.name}
                {item.brand ? ` (${item.brand})` : ""}
                <span className="text-slate-500">
                  {" "}
                  · da {item.stock} von {item.target}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="rounded-xl border border-slate-300 p-4 text-sm">
        <h2 className="text-lg font-semibold">Status</h2>
        <dl className="mt-2 grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-1 [&_dd]:break-all">
          <dt className="font-semibold">User-Agent</dt>
          <dd>{navigator.userAgent}</dd>
          <dt className="font-semibold">display-mode</dt>
          <dd>{mode}</dd>
          <dt className="font-semibold">Kamera</dt>
          <dd>{cameraLabel || "n/a"}</dd>
          <dt className="font-semibold">Fehler</dt>
          <dd>
            {errors.length === 0 ? (
              "keine"
            ) : (
              <ul className="text-red-700">
                {errors.map((message, index) => (
                  <li key={`${index}-${message}`}>{message}</li>
                ))}
              </ul>
            )}
          </dd>
        </dl>
      </section>
    </main>
  );
}
