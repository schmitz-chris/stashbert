import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useEffect, useReducer, useState } from "react";
import { CameraSettings } from "../components/CameraSettings";
import { Dialog } from "../components/Dialog";
import { ManualCodeDialog } from "../components/ManualCodeDialog";
import { MergeDialog } from "../components/MergeDialog";
import { ResultCard } from "../components/ResultCard";
import { useScanner } from "../hooks/useScanner";
import {
  repeatMovementMutation,
  reversalMutation,
  scanMovementMutation,
} from "../lib/api/queries";
import type { Product } from "../lib/products";
import { hiddenCard, resultCardReducer } from "../lib/resultCard";
import {
  cameraErrorText,
  feedbackFor,
  loadScanMode,
  saveScanMode,
  undoFeedbackFor,
  type Feedback,
  type FeedbackColor,
  type MovementResult,
  type ScanMode,
} from "../lib/scan";
import { ROI } from "../lib/scanner/decoder";
import { isSoundUnlocked, playSound, unlockSound } from "../lib/sound";

// How long the flash over the camera image and the message stay visible.
const flashDuration = 300;
const messageDuration = 2000;
// How often the result card checks whether its time is up.
const cardTickInterval = 200;

const modes: { mode: ScanMode; label: string; active: string }[] = [
  { mode: "add", label: "Einlagern", active: "bg-emerald-600 text-white" },
  { mode: "consume", label: "Entnehmen", active: "bg-sky-600 text-white" },
];

const flashClass: Record<FeedbackColor, string> = {
  green: "bg-emerald-500/70",
  blue: "bg-sky-500/70",
  yellow: "bg-amber-400/70",
  red: "bg-red-600/70",
};

const messageClass: Record<FeedbackColor, string> = {
  green: "bg-emerald-600 text-white",
  blue: "bg-sky-600 text-white",
  yellow: "bg-amber-400 text-stone-900",
  red: "bg-red-600 text-white",
};

// The strip the decoder reads (ROI), over the camera image.
const roiStyle = {
  left: `${ROI.left * 100}%`,
  top: `${ROI.top * 100}%`,
  width: `${ROI.width * 100}%`,
  height: `${ROI.height * 100}%`,
};

// Feedback on screen; id tells consecutive feedbacks apart.
interface Shown {
  feedback: Feedback;
  id: number;
  flash: boolean;
}

export function ScanPage() {
  const queryClient = useQueryClient();
  const { mutateAsync: book } = useMutation(scanMovementMutation(queryClient));
  const repeat = useMutation(repeatMovementMutation(queryClient));
  const reversal = useMutation(reversalMutation(queryClient));
  const [card, dispatchCard] = useReducer(resultCardReducer, hiddenCard);
  const [mode, setMode] = useState(loadScanMode);
  // False when the view was loaded directly: iOS plays sound only after a tap.
  const [soundReady, setSoundReady] = useState(isSoundUnlocked);
  const [shown, setShown] = useState<Shown | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // The new product of the card while its merge dialog is open.
  const [mergeSource, setMergeSource] = useState<Product | null>(null);
  const [manualOpen, setManualOpen] = useState(false);
  const [videoSize, setVideoSize] = useState({ width: 3, height: 4 });

  function show(feedback: Feedback) {
    playSound(feedback.sound);
    setShown((current) => ({ feedback, id: (current?.id ?? 0) + 1, flash: true }));
  }

  // Reports a booking of kind and shows it on the result card.
  function showBooking(result: MovementResult, kind: ScanMode) {
    show(feedbackFor({ ok: true, mode: kind, result }));
    dispatchCard({ type: "show", result, kind, now: Date.now() });
  }

  // Books code in the mode that is set now and reports the result.
  function bookCode(code: string) {
    const kind = mode;
    book({ barcode: code, kind }).then(
      (result) => showBooking(result, kind),
      (error: unknown) => show(feedbackFor({ ok: false, error })),
    );
  }

  // Books every accepted code. While the merge dialog or the dialog for
  // typing in a code lies over the camera, codes are ignored.
  const { videoRef, phase, cameraError, track, cameras, restart, selectCamera } =
    useScanner((code) => {
      if (mergeSource !== null || manualOpen) {
        return;
      }
      bookCode(code);
    });

  // [+1]: books one more unit of the product on the card, with its kind.
  function plusOne(productId: string, kind: ScanMode) {
    repeat.mutateAsync({ productId, kind }).then(
      (result) => showBooking(result, kind),
      (error: unknown) => show(feedbackFor({ ok: false, error })),
    );
  }

  // [Rückgängig]: reverses the booking on the card and hides the card.
  function undo(movementId: string) {
    reversal.mutateAsync(movementId).then(
      (result) => {
        dispatchCard({ type: "hide", movementId });
        show(undoFeedbackFor({ ok: true, result }));
      },
      (error: unknown) => show(undoFeedbackFor({ ok: false, error })),
    );
  }

  // "Stattdessen zu vorhandenem Produkt": the card waits while the dialog
  // is open.
  function openMerge(product: Product) {
    dispatchCard({ type: "pause", now: Date.now() });
    setMergeSource(product);
  }

  function closeMerge() {
    setMergeSource(null);
    dispatchCard({ type: "resume", now: Date.now() });
  }

  // "Code eintippen": the card waits while the dialog is open.
  function openManual() {
    dispatchCard({ type: "pause", now: Date.now() });
    setManualOpen(true);
  }

  function closeManual() {
    setManualOpen(false);
    dispatchCard({ type: "resume", now: Date.now() });
  }

  // A code typed in by hand is booked like a scanned one. The repeat
  // filter of the scanner does not apply: typing it in is deliberate.
  function submitManual(code: string) {
    closeManual();
    bookCode(code);
  }

  // After the merge the card shows the target, which the booking on the
  // card belongs to now.
  function merged(target: Product, source: Product) {
    dispatchCard({ type: "merge", sourceId: source.id, target });
    closeMerge();
  }

  // While the card runs, reports the time to the reducer, which hides the
  // card when its time is up.
  const cardRunning = card.status === "running";
  useEffect(() => {
    if (!cardRunning) {
      return;
    }
    const timer = setInterval(
      () => dispatchCard({ type: "tick", now: Date.now() }),
      cardTickInterval,
    );
    return () => clearInterval(timer);
  }, [cardRunning]);

  // Ends the flash and then hides the message of the latest feedback.
  const shownId = shown?.id;
  useEffect(() => {
    if (shownId === undefined) {
      return;
    }
    const flashTimer = setTimeout(
      () => setShown((current) => current && { ...current, flash: false }),
      flashDuration,
    );
    const messageTimer = setTimeout(() => setShown(null), messageDuration);
    return () => {
      clearTimeout(flashTimer);
      clearTimeout(messageTimer);
    };
  }, [shownId]);

  // Every tap in the view unlocks the sound (iOS).
  function handleTap() {
    unlockSound();
    setSoundReady(true);
  }

  function chooseMode(next: ScanMode) {
    setMode(next);
    saveScanMode(next);
  }

  function handleVideoSize(video: HTMLVideoElement) {
    if (video.videoWidth > 0 && video.videoHeight > 0) {
      setVideoSize({ width: video.videoWidth, height: video.videoHeight });
    }
  }

  const { width, height } = videoSize;

  return (
    <div className="flex flex-col gap-3" onClick={handleTap}>
      <h1 className="sr-only">Scannen</h1>
      <div role="group" aria-label="Modus" className="grid grid-cols-2 gap-2">
        {modes.map(({ mode: value, label, active }) => (
          <button
            key={value}
            type="button"
            aria-pressed={mode === value}
            onClick={() => chooseMode(value)}
            className={`min-h-14 rounded-xl text-lg font-semibold ${mode === value ? active : "bg-stone-200 text-stone-700"}`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Sized to the aspect ratio of the video, so the strip lies where the decoder reads. */}
      <div
        className="relative mx-auto overflow-hidden rounded-xl bg-black"
        style={{
          width: `min(100%, calc(55dvh * ${width} / ${height}))`,
          aspectRatio: `${width} / ${height}`,
        }}
      >
        <video
          ref={videoRef}
          playsInline
          muted
          onLoadedMetadata={(event) => handleVideoSize(event.currentTarget)}
          onResize={(event) => handleVideoSize(event.currentTarget)}
          className="absolute inset-0 size-full"
        />
        {phase === "running" && (
          <div
            className="pointer-events-none absolute rounded border-2 border-white shadow-[0_0_0_2px_rgb(0_0_0/0.6)]"
            style={roiStyle}
          />
        )}
        {shown?.flash && (
          <div
            className={`pointer-events-none absolute inset-0 ${flashClass[shown.feedback.color]}`}
          />
        )}
        {phase === "starting" && (
          <p className="absolute inset-0 flex items-center justify-center p-4 text-center text-white">
            Kamera wird gestartet …
          </p>
        )}
        {phase === "paused" && (
          <button
            type="button"
            onClick={restart}
            className="absolute inset-0 p-4 text-xl font-semibold text-white"
          >
            Tippen zum Fortsetzen
          </button>
        )}
        {phase === "idle" && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-4 p-4 text-center text-white">
            <p>{cameraErrorText(cameraError)}</p>
            <button
              type="button"
              onClick={restart}
              className="min-h-11 rounded-lg bg-white px-4 font-medium text-stone-900"
            >
              Erneut versuchen
            </button>
          </div>
        )}
        <button
          type="button"
          aria-label="Kamera-Einstellungen"
          onClick={() => setSettingsOpen(true)}
          className="absolute top-2 right-2 flex size-11 items-center justify-center rounded-full bg-black/50 text-2xl text-white"
        >
          {"\u2699\uFE0E"}
        </button>
      </div>
      <button
        type="button"
        onClick={openManual}
        className="min-h-11 self-center rounded-lg border border-stone-300 bg-white px-4 font-medium text-stone-700"
      >
        Code eintippen
      </button>

      <p
        role="status"
        className={`flex min-h-16 items-center justify-center rounded-xl px-3 text-center text-2xl font-bold ${shown ? messageClass[shown.feedback.color] : ""}`}
      >
        {shown?.feedback.text}
      </p>
      {card.status !== "hidden" && (
        <ResultCard
          key={card.booking.result.movement.id}
          booking={card.booking}
          disabled={repeat.isPending || reversal.isPending}
          onPlusOne={() => plusOne(card.booking.result.product.id, card.booking.kind)}
          onUndo={() => undo(card.booking.result.movement.id)}
          onProductChange={(product) => dispatchCard({ type: "update", product })}
          onMerge={() => openMerge(card.booking.result.product)}
        />
      )}
      {!soundReady && (
        <button type="button" className="min-h-11 text-sm text-stone-500">
          Für Ton einmal tippen
        </button>
      )}

      <MergeDialog
        source={mergeSource}
        onClose={closeMerge}
        onMerged={merged}
      />
      <ManualCodeDialog
        open={manualOpen}
        mode={mode}
        onClose={closeManual}
        onSubmit={submitManual}
      />
      <Dialog open={settingsOpen} onClose={() => setSettingsOpen(false)} title="Kamera">
        <CameraSettings
          track={track}
          cameras={cameras}
          onSelectCamera={selectCamera}
        />
        <div className="mt-4 flex justify-end">
          <button
            type="button"
            onClick={() => setSettingsOpen(false)}
            className="min-h-11 rounded-lg bg-emerald-600 px-4 font-medium text-white"
          >
            Fertig
          </button>
        </div>
      </Dialog>
    </div>
  );
}
