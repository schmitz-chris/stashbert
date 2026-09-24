import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState, type ChangeEvent } from "react";
import {
  productImageDeleteMutation,
  productImageUploadMutation,
} from "../lib/api/queries";
import { photoErrorText, productImageUrl } from "../lib/productImage";
import type { Product } from "../lib/products";
import { ConfirmActions, Dialog } from "./Dialog";

// Secondary buttons (docs/plan.md, F22); "Bild entfernen" in red text.
// Both are equally wide: side by side, each half of the row, or, when a
// half is narrower than 9rem (large text, narrow screen), one below the
// other at full width.
const buttonClass =
  "pressable min-h-11 rounded-lg border border-line-strong bg-surface px-4 py-2 font-medium disabled:opacity-40";

/**
 * The image of product on the product page, if it has one, and below it
 * "Foto aufnehmen", which opens the camera and uploads the photo in place
 * of the image, and, with an image, "Bild entfernen" with a confirmation
 * dialog (docs/plan.md, F17).
 */
export function ProductImageSection({ product }: { product: Product }) {
  const queryClient = useQueryClient();
  const upload = useMutation(productImageUploadMutation(queryClient));
  const remove = useMutation(productImageDeleteMutation(queryClient));
  const [confirmOpen, setConfirmOpen] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

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
      <p role="status" className="mt-1 text-sm font-medium text-danger">
        {upload.isError ? photoErrorText(upload.error) : ""}
      </p>
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
