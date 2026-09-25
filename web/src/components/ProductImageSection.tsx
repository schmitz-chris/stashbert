import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState, type ChangeEvent } from "react";
import {
  productImageDeleteMutation,
  productImageUploadMutation,
} from "../lib/api/queries";
import { photoErrorText, productImageUrl } from "../lib/productImage";
import type { Product } from "../lib/products";
import { noImageText, type RecognizeNotice } from "../lib/recognition";
import { ConfirmActions, Dialog } from "./Dialog";

// Secondary buttons (docs/plan.md, F22); "Bild entfernen" in red text.
// All are equally wide: side by side in columns of at least 9rem, or, when
// two columns do not fit (large text, narrow screen), one below the other
// at full width.
const buttonClass =
  "pressable min-h-11 rounded-lg border border-line-strong bg-surface px-4 py-2 font-medium disabled:opacity-40";

// The color of the message about the last recognition.
const noticeToneClass: Record<RecognizeNotice["tone"], string> = {
  neutral: "text-ink-secondary",
  success: "text-accent",
  failure: "text-danger",
};

/**
 * "Mit KI erkennen" (docs/plan.md, F35). The product page runs the
 * recognition, because its result goes into the form of the page.
 */
export interface RecognizeControl {
  /** Whether the recognition runs. */
  pending: boolean;
  /** The message about the last recognition. */
  notice: RecognizeNotice;
  /** Starts the recognition of the stored photo. */
  onRecognize: () => void;
}

/**
 * The image of product on the product page, if it has one, and below it
 * "Foto aufnehmen", which opens the camera and uploads the photo in place
 * of the image, and, with an image, "Bild entfernen" with a confirmation
 * dialog (docs/plan.md, F17). With recognize (a provider is set up) there
 * is also "Mit KI erkennen"; without an image it is locked, with the hint
 * "Erst ein Foto aufnehmen" below the buttons.
 */
export function ProductImageSection({
  product,
  recognize,
}: {
  product: Product;
  recognize: RecognizeControl | null;
}) {
  const queryClient = useQueryClient();
  const upload = useMutation(productImageUploadMutation(queryClient));
  const remove = useMutation(productImageDeleteMutation(queryClient));
  const [confirmOpen, setConfirmOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const hintId = useId();

  const imageUrl = productImageUrl(product);
  const pending = upload.isPending || remove.isPending;

  function pickPhoto(event: ChangeEvent<HTMLInputElement>) {
    const photo = event.target.files?.[0];
    // Lets the same file be picked again next time.
    event.target.value = "";
    if (photo !== undefined) {
      upload.mutate({ productId: product.id, photo });
    }
  }

  return (
    <div className="mt-4">
      {imageUrl !== null && (
        <img
          src={imageUrl}
          alt=""
          className="mb-2 h-48 w-full rounded-xl bg-surface object-contain"
        />
      )}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(min(100%,9rem),1fr))] gap-3">
        <button
          type="button"
          disabled={pending}
          onClick={() => inputRef.current?.click()}
          className={`text-accent ${buttonClass}`}
        >
          {upload.isPending ? "Wird hochgeladen …" : "Foto aufnehmen"}
        </button>
        <input
          ref={inputRef}
          type="file"
          accept="image/*"
          capture="environment"
          hidden
          onChange={pickPhoto}
        />
        {/* Next to "Foto aufnehmen" with and without an image. It waits
            for a new image to be uploaded or the old one removed. */}
        {recognize !== null && (
          <button
            type="button"
            disabled={pending || recognize.pending || !product.has_image}
            onClick={recognize.onRecognize}
            aria-describedby={product.has_image ? undefined : hintId}
            className={`text-accent ${buttonClass}`}
          >
            {recognize.pending ? "Wird erkannt …" : "Mit KI erkennen"}
          </button>
        )}
        {product.has_image && (
          <button
            type="button"
            disabled={pending}
            onClick={() => {
              upload.reset();
              remove.reset();
              setConfirmOpen(true);
            }}
            className={`text-danger ${buttonClass}`}
          >
            Bild entfernen
          </button>
        )}
      </div>
      {recognize !== null && !product.has_image && (
        <p id={hintId} className="mt-1 text-sm text-ink-tertiary">
          {noImageText}
        </p>
      )}
      <p role="status" className="mt-1 text-sm font-medium text-danger">
        {upload.isError ? photoErrorText(upload.error) : ""}
      </p>
      {recognize !== null && (
        <p
          role="status"
          className={`mt-1 text-sm font-medium ${noticeToneClass[recognize.notice.tone]}`}
        >
          {recognize.notice.text}
        </p>
      )}
      <Dialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        title="Bild entfernen?"
      >
        <p className="mt-2 text-ink-secondary">
          Das Bild von „{product.name}“ wird entfernt.
        </p>
        <p role="status" className="mt-2 text-sm font-medium text-danger">
          {remove.isError ? "Entfernen fehlgeschlagen" : ""}
        </p>
        <ConfirmActions
          label="Entfernen"
          pending={remove.isPending}
          onCancel={() => setConfirmOpen(false)}
          onConfirm={() =>
            remove.mutate(product.id, {
              onSuccess: () => setConfirmOpen(false),
            })
          }
        />
      </Dialog>
    </div>
  );
}
