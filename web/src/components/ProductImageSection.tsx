import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef, useState, type ChangeEvent } from "react";
import {
  productImageDeleteMutation,
  productImageUploadMutation,
} from "../lib/api/queries";
import { photoErrorText, productImageUrl } from "../lib/productImage";
import type { Product } from "../lib/products";
import { Dialog } from "./Dialog";

const buttonClass = "min-h-11 rounded-lg px-4 font-medium disabled:opacity-40";

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
    <>
      {imageUrl !== null && (
        <img
          src={imageUrl}
          alt=""
          className="mt-2 h-48 w-full rounded-xl bg-white object-contain"
        />
      )}
      <div className="mt-2 flex flex-wrap items-center gap-3">
        <button
          type="button"
          disabled={pending}
          onClick={() => inputRef.current?.click()}
          className={`border border-stone-300 bg-white text-stone-700 ${buttonClass}`}
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
            className={`border border-red-300 bg-white text-red-700 ${buttonClass}`}
          >
            Bild entfernen
          </button>
        )}
      </div>
      <p role="status" className="mt-1 text-sm font-medium text-red-700">
        {upload.isError ? photoErrorText(upload.error) : ""}
      </p>
      <Dialog
        open={confirmOpen}
        onClose={() => setConfirmOpen(false)}
        title="Bild entfernen?"
      >
        <p className="mt-2 text-stone-700">
          Das Bild von „{product.name}“ wird entfernt.
        </p>
        <p role="status" className="mt-2 text-sm font-medium text-red-700">
          {remove.isError ? "Entfernen fehlgeschlagen" : ""}
        </p>
        <div className="mt-4 flex justify-end gap-3">
          <button
            type="button"
            onClick={() => setConfirmOpen(false)}
            className={`border border-stone-300 bg-white text-stone-700 ${buttonClass}`}
          >
            Abbrechen
          </button>
          <button
            type="button"
            disabled={remove.isPending}
            onClick={() =>
              remove.mutate(product.id, {
                onSuccess: () => setConfirmOpen(false),
              })
            }
            className={`bg-red-600 text-white ${buttonClass}`}
          >
            Entfernen
          </button>
        </div>
      </Dialog>
    </>
  );
}
