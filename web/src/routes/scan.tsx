import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useReducer, useRef, useState } from "react";
import { CameraSettings } from "../components/CameraSettings";
import { CrateSheet } from "../components/CrateSheet";
import { Dialog } from "../components/Dialog";
import { FeedbackSymbol } from "../components/FeedbackSymbol";
import { ManualCodeDialog } from "../components/ManualCodeDialog";
import { MergeDialog } from "../components/MergeDialog";
import { PageHeading } from "../components/PageHeading";
import { MarkResultCard, ResultCard } from "../components/ResultCard";
import { ScanTimings } from "../components/ScanTimings";
import { TorchButton } from "../components/TorchButton";
import { useDocumentTitle } from "../hooks/useDocumentTitle";
import { useScanner } from "../hooks/useScanner";
import {
  markScanMutation,
  productListQuery,
  repeatMovementMutation,
  reversalMutation,
  scanMovementMutation,
  unmarkMutation,
  type ScanMovement,
} from "../lib/api/queries";
import { pageTitle } from "../lib/pageTitle";
import { findByBarcode, type Product } from "../lib/products";
import { hiddenCard, resultCardReducer } from "../lib/resultCard";
import {
  cameraErrorText,
  crateQuestion,
  feedbackFor,
  feedbackIcon,
  loadScanMode,
  saveScanMode,
  undoFeedbackFor,
  unmarkFeedbackFor,
  type BookingMode,
  type CrateQuestion,
  type Feedback,
  type FeedbackColor,
  type MovementResult,
  type ScanMode,
} from "../lib/scan";
import { ROI } from "../lib/scanner/decoder";
import { isSoundUnlocked, playSound, unlockSound } from "../lib/sound";

// How long the flash over the camera image and the message of a success
// stay visible. The message of a failure stays until the next feedback or a
// change of the mode (ADR-0016).
const flashDuration = 300;
const messageDuration = 2000;

const modes: { mode: ScanMode; label: string; active: string }[] = [
  { mode: "add", label: "Einlagern", active: "bg-accent text-white" },
  { mode: "consume", label: "Entnehmen", active: "bg-consume text-white" },
  { mode: "mark", label: "Einkaufen", active: "bg-marked text-white" },
];

// The flash tints the whole camera image in the color of the feedback. With
// reduced motion (HIG, Motion) it is only a frame of 6 px in that color
// along the edge of the image (docs/plan.md, F24).
const flashClass: Record<FeedbackColor, string> = {
  green: "motion-safe:bg-accent/70 motion-reduce:border-accent",
  blue: "motion-safe:bg-consume/70 motion-reduce:border-consume",
  orange: "motion-safe:bg-marked/70 motion-reduce:border-marked",
  yellow: "motion-safe:bg-warning/70 motion-reduce:border-warning",
  red: "motion-safe:bg-danger/70 motion-reduce:border-danger",
};

const messageClass: Record<FeedbackColor, string> = {
  green: "bg-accent text-white",
  blue: "bg-consume text-white",
  orange: "bg-marked text-white",
  yellow: "bg-warning text-ink",
  red: "bg-danger text-white",
};

// The strip the decoder reads (ROI), over the camera image.
const roiStyle = {
  left: `${ROI.left * 100}%`,
  top: `${ROI.top * 100}%`,
  width: `${ROI.width * 100}%`,
  height: `${ROI.height * 100}%`,
};

// Feedback on screen; id tells consecutive feedbacks apart, failed marks
// the feedback of a failed request, whose message has no time limit.
interface Shown {
  feedback: Feedback;
  id: number;
  flash: boolean;
  failed: boolean;
}

export function ScanPage() {
  const queryClient = useQueryClient();
  const { mutateAsync: book } = useMutation(scanMovementMutation(queryClient));
  const repeat = useMutation(repeatMovementMutation(queryClient));
  const reversal = useMutation(reversalMutation(queryClient));
  const { mutateAsync: mark } = useMutation(markScanMutation(queryClient));
  const unmark = useMutation(unmarkMutation(queryClient));
  // The product list finds the product of a code before it is booked, to
  // ask "Flasche oder Kasten" (F28); without it every code is booked at once.
  const { data: products } = useQuery(productListQuery);
  const [card, dispatchCard] = useReducer(resultCardReducer, hiddenCard);
  const [mode, setMode] = useState(loadScanMode);
  // False when the view was loaded directly: iOS plays sound only after a tap.
  const [soundReady, setSoundReady] = useState(isSoundUnlocked);
  const [shown, setShown] = useState<Shown | null>(null);
  const [settingsOpen, setSettingsOpen] = useState(false);
  // The new product of the card while its merge dialog is open.
  const [mergeSource, setMergeSource] = useState<Product | null>(null);
  const [manualOpen, setManualOpen] = useState(false);
  // The open question "Flasche oder Kasten" (F28).
  const [question, setQuestion] = useState<CrateQuestion | null>(null);
  const [videoSize, setVideoSize] = useState({ width: 3, height: 4 });
  // From reading the last code to the answer of the server, for the camera
  // menu (F25); a ref, so that measuring renders nothing.
  const lastBookingMs = useRef<number | null>(null);
  // The name field on the card of a placeholder (F31).
  const nameInput = useRef<HTMLInputElement>(null);
  useDocumentTitle(pageTitle("scan"));

  function show(feedback: Feedback, failed = false) {
    playSound(feedback.sound);
    setShown((current) => ({ feedback, id: (current?.id ?? 0) + 1, flash: true, failed }));
  }

  // Reports a booking of kind and shows it on the result card.
  function showBooking(result: MovementResult, kind: BookingMode) {
    show(feedbackFor({ ok: true, mode: kind, result }));
    dispatchCard({ type: "show", result, kind });
  }

  // Returns request and notes how long it took since detectedAt, the time
  // the scanner read the code (undefined for a code typed in).
  function timed<T>(request: Promise<T>, detectedAt: number | undefined): Promise<T> {
    if (detectedAt !== undefined) {
      const note = () => {
        lastBookingMs.current = performance.now() - detectedAt;
      };
      request.then(note, note);
    }
    return request;
  }

  // Books code in the mode that is set now, or marks it for shopping in
  // mode mark, and reports the result. In mode add a code of a product
  // with a crate size asks "Flasche oder Kasten" first.
  function handleCode(code: string, detectedAt?: number) {
    const kind = mode;
    if (kind === "mark") {
      timed(mark(code), detectedAt).then(
        (result) => {
          show(feedbackFor({ ok: true, mode: kind, result }));
          dispatchCard({ type: "show", result, kind });
        },
        (error: unknown) => show(feedbackFor({ ok: false, error, mode: kind }), true),
      );
      return;
    }
    const crate = crateQuestion(kind, code, findByBarcode(products, code));
    if (crate !== null) {
      setQuestion(crate);
      return;
    }
    bookCode({ barcode: code, kind }, detectedAt);
  }

  // Books movement and reports the result.
  function bookCode(movement: ScanMovement, detectedAt?: number) {
    timed(book(movement), detectedAt).then(
      (result) => showBooking(result, movement.kind),
      (error: unknown) => show(feedbackFor({ ok: false, error }), true),
    );
  }

  // The answer to "Flasche oder Kasten": books quantity of the code.
  function answer(quantity: number) {
    if (question === null) {
      return;
    }
    setQuestion(null);
    bookCode({ barcode: question.code, kind: "add", quantity });
  }

  // Books or marks every accepted code. While a dialog or a sheet lies over
  // the camera (merge, typing in a code, "Flasche oder Kasten") or while the
  // name field on the card has the focus (F31), codes are ignored, so a scan
  // does not replace the card while the name is typed.
  const {
    videoRef,
    phase,
    cameraError,
    track,
    cameras,
    chosenId,
    restart,
    selectCamera,
    selectAutomatic,
    timings,
  } = useScanner((code) => {
    const naming = nameInput.current !== null && nameInput.current === document.activeElement;
    if (mergeSource !== null || manualOpen || question !== null || naming) {
      return;
    }
    handleCode(code, performance.now());
  });

  // [+1]: books one more unit of the product on the card, with its kind.
  function plusOne(productId: string, kind: BookingMode) {
    repeat.mutateAsync({ productId, kind }).then(
      (result) => showBooking(result, kind),
      (error: unknown) => show(feedbackFor({ ok: false, error }), true),
    );
  }

  // [Rückgängig]: reverses the booking on the card and hides the card.
  function undo(movementId: string) {
    reversal.mutateAsync(movementId).then(
      (result) => {
        dispatchCard({ type: "hide", movementId });
        show(undoFeedbackFor({ ok: true, result }));
      },
      (error: unknown) => show(undoFeedbackFor({ ok: false, error }), true),
    );
  }

  // [Rückgängig] on the card of a mark: removes the mark of product and
  // hides the card.
  function undoMark(product: Product) {
    unmark.mutateAsync(product.id).then(
      () => {
        dispatchCard({ type: "hideMark", productId: product.id });
        show(unmarkFeedbackFor({ ok: true, product }));
      },
      (error: unknown) => show(unmarkFeedbackFor({ ok: false, error }), true),
    );
  }

  // A code typed in by hand is booked or marked like a scanned one. The
  // repeat filter of the scanner does not apply: typing it in is deliberate.
  function submitManual(code: string) {
    setManualOpen(false);
    handleCode(code);
  }

  // After the merge the card shows the target, which the booking on the
  // card belongs to now.
  function merged(target: Product, source: Product) {
    dispatchCard({ type: "merge", sourceId: source.id, target });
    setMergeSource(null);
  }

  // Ends the flash and then hides the message of the latest feedback,
  // unless it reports a failure.
  const shownId = shown?.id;
  const shownFailed = shown?.failed;
  useEffect(() => {
    if (shownId === undefined) {
      return;
    }
    const flashTimer = setTimeout(
      () => setShown((current) => current && { ...current, flash: false }),
      flashDuration,
    );
    const messageTimer = shownFailed
      ? undefined
      : setTimeout(() => setShown(null), messageDuration);
    return () => {
      clearTimeout(flashTimer);
      clearTimeout(messageTimer);
    };
  }, [shownId, shownFailed]);

  // Every tap in the view unlocks the sound (iOS).
  function handleTap() {
    unlockSound();
    setSoundReady(true);
  }

  // A change of the mode also ends the message of the last feedback.
  function chooseMode(next: ScanMode) {
    setMode(next);
    saveScanMode(next);
    setShown(null);
  }

  function handleVideoSize(video: HTMLVideoElement) {
    if (video.videoWidth > 0 && video.videoHeight > 0) {
      setVideoSize({ width: video.videoWidth, height: video.videoHeight });
    }
  }

  const { width, height } = videoSize;
  const content = card.status === "hidden" ? null : card.content;

  return (
    <div className="flex flex-col gap-3" onClick={handleTap}>
      <PageHeading className="sr-only">Scannen</PageHeading>
      {/* The mode switch does not grow with the text size of the system
          (ADR-0016). Its text is at most 18 px and shrinks with the width of
          the switch (cqw), so "Entnehmen" fits into a third on every screen. */}
      <div role="group" aria-label="Modus" className="@container grid grid-cols-3 gap-[8px]">
        {modes.map(({ mode: value, label, active }) => (
          <button
            key={value}
            type="button"
            aria-pressed={mode === value}
            onClick={() => chooseMode(value)}
            className={`pressable min-h-[56px] rounded-[12px] text-[min(18px,5.5cqw)] leading-[24px] font-semibold ${mode === value ? active : "bg-fill text-ink-secondary"}`}
          >
            {label}
          </button>
        ))}
      </div>

      {/* Sized to the aspect ratio of the video, so the strip lies where the decoder reads.
          The buttons over the image keep their size in px; the texts keep
          64 px away from the top and the bottom, where the buttons lie. */}
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
            className={`pointer-events-none absolute inset-0 rounded-[inherit] motion-reduce:border-[6px] ${flashClass[shown.feedback.color]}`}
          />
        )}
        {phase === "starting" && (
          <p className="absolute inset-0 flex items-center justify-center px-4 py-[64px] text-center text-white">
            Kamera wird gestartet …
          </p>
        )}
        {phase === "paused" && (
          <button
            type="button"
            onClick={restart}
            className="pressable absolute inset-0 px-4 py-[64px] text-xl font-semibold text-white"
          >
            Tippen zum Fortsetzen
          </button>
        )}
        {phase === "idle" && (
          // Scrolls if the text does not fit (large text size).
          <div className="absolute inset-0 flex flex-col overflow-y-auto px-4 py-[64px] text-center text-white">
            <div className="my-auto flex flex-col items-center gap-4">
              <p>{cameraErrorText(cameraError)}</p>
              <button
                type="button"
                onClick={restart}
                className="pressable min-h-11 rounded-lg bg-surface px-4 font-medium text-ink"
              >
                Erneut versuchen
              </button>
            </div>
          </div>
        )}
        <TorchButton track={track} />
        <button
          type="button"
          aria-label="Kamera-Einstellungen"
          onClick={() => setSettingsOpen(true)}
          className="pressable absolute top-[8px] right-[8px] flex size-[48px] items-center justify-center rounded-full bg-black/60 text-white shadow-md ring-1 ring-white/40"
        >
          <GearIcon />
        </button>
      </div>
      <button
        type="button"
        onClick={() => setManualOpen(true)}
        className="pressable min-h-11 self-center rounded-lg border border-line-strong bg-surface px-4 font-medium text-ink-secondary"
      >
        Code eintippen
      </button>

      <p
        role="status"
        className={`flex min-h-16 items-center justify-center gap-2 rounded-xl px-3 text-center text-2xl font-bold ${shown ? messageClass[shown.feedback.color] : ""}`}
      >
        {shown && (
          <>
            <FeedbackSymbol icon={feedbackIcon(shown.feedback)} />
            <span className="min-w-0 break-words">{shown.feedback.text}</span>
          </>
        )}
      </p>
      {content?.kind === "mark" && (
        <MarkResultCard
          mark={content}
          disabled={unmark.isPending}
          onUndo={() => undoMark(content.result.product)}
          onClose={() => dispatchCard({ type: "close" })}
        />
      )}
      {content !== null && content.kind !== "mark" && (
        <ResultCard
          key={content.result.movement.id}
          booking={content}
          disabled={repeat.isPending || reversal.isPending}
          nameInputRef={nameInput}
          onPlusOne={() => plusOne(content.result.product.id, content.kind)}
          onUndo={() => undo(content.result.movement.id)}
          onProductChange={(product) => dispatchCard({ type: "update", product })}
          onMerge={() => setMergeSource(content.result.product)}
          onClose={() => dispatchCard({ type: "close" })}
        />
      )}
      {!soundReady && (
        <button type="button" className="pressable min-h-11 rounded-lg text-sm text-ink-tertiary">
          Für Ton einmal tippen
        </button>
      )}

      <MergeDialog
        source={mergeSource}
        onClose={() => setMergeSource(null)}
        onMerged={merged}
      />
      <ManualCodeDialog
        open={manualOpen}
        mode={mode}
        onClose={() => setManualOpen(false)}
        onSubmit={submitManual}
      />
      <CrateSheet
        question={question}
        onClose={() => setQuestion(null)}
        onAnswer={answer}
      />
      <Dialog open={settingsOpen} onClose={() => setSettingsOpen(false)} title="Kamera">
        <CameraSettings
          track={track}
          cameras={cameras}
          chosenId={chosenId}
          onSelectCamera={selectCamera}
          onSelectAutomatic={selectAutomatic}
        />
        {settingsOpen && (
          <ScanTimings read={() => ({ camera: timings(), bookingMs: lastBookingMs.current })} />
        )}
        {/* The settings apply at once, so the button only closes (HIG, Sheets). */}
        <div className="mt-4 flex justify-end">
          <button
            type="button"
            onClick={() => setSettingsOpen(false)}
            className="pressable min-h-11 rounded-lg bg-accent px-4 font-medium text-white"
          >
            Schließen
          </button>
        </div>
      </Dialog>
    </div>
  );
}

// A gear with eight teeth, drawn with the color of the text.
function GearIcon() {
  return (
    <svg
      aria-hidden="true"
      viewBox="0 0 24 24"
      fill="currentColor"
      fillRule="evenodd"
      className="size-[24px]"
    >
      <path d="M10.31 4.69L10.44 2.12A10 10 0 0 1 13.56 2.12L13.69 4.69A7.5 7.5 0 0 1 15.97 5.64L17.88 3.91A10 10 0 0 1 20.09 6.12L18.36 8.03A7.5 7.5 0 0 1 19.31 10.31L21.88 10.44A10 10 0 0 1 21.88 13.56L19.31 13.69A7.5 7.5 0 0 1 18.36 15.97L20.09 17.88A10 10 0 0 1 17.88 20.09L15.97 18.36A7.5 7.5 0 0 1 13.69 19.31L13.56 21.88A10 10 0 0 1 10.44 21.88L10.31 19.31A7.5 7.5 0 0 1 8.03 18.36L6.12 20.09A10 10 0 0 1 3.91 17.88L5.64 15.97A7.5 7.5 0 0 1 4.69 13.69L2.12 13.56A10 10 0 0 1 2.12 10.44L4.69 10.31A7.5 7.5 0 0 1 5.64 8.03L3.91 6.12A10 10 0 0 1 6.12 3.91L8.03 5.64A7.5 7.5 0 0 1 10.31 4.69ZM15 12A3 3 0 1 0 9 12A3 3 0 1 0 15 12Z" />
    </svg>
  );
}
