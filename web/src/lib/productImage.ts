import { problemCode } from "./api/client";
import type { Product } from "./products";

/** The longest edge of an uploaded photo in pixels (architecture.md 7.3). */
export const maxPhotoEdge = 1024;

/**
 * Returns the size of an image of width × height pixels, scaled down so that
 * its longest edge is at most max, keeping the aspect ratio. Smaller images
 * keep their size; they are never enlarged. Each edge is rounded to whole
 * pixels and is at least 1.
 */
export function fitWithin(
  width: number,
  height: number,
  max: number,
): { width: number; height: number } {
  const longest = Math.max(width, height);
  if (longest <= max) {
    return { width, height };
  }
  const scale = max / longest;
  return {
    width: Math.max(1, Math.round(width * scale)),
    height: Math.max(1, Math.round(height * scale)),
  };
}

/**
 * Returns the URL of the image of product, or null if it has none. The
 * query parameter v comes from updated_at, so the URL changes when the
 * image is replaced and the browser does not show the old one from its
 * cache (the server sends Cache-Control: private, max-age=86400). The
 * server ignores the parameter.
 */
export function productImageUrl(
  product: Pick<Product, "id" | "has_image" | "updated_at">,
): string | null {
  if (!product.has_image) {
    return null;
  }
  const id = encodeURIComponent(product.id);
  return `/api/v1/products/${id}/image?v=${encodeURIComponent(product.updated_at)}`;
}

/** The browser cannot decode a photo, for example because of its format. */
export class PhotoDecodeError extends Error {
  constructor(options?: ErrorOptions) {
    super("photo cannot be decoded", options);
    this.name = "PhotoDecodeError";
  }
}

/**
 * Returns the message for a failed upload of a photo: "Foto zu groß oder
 * ungültig" if the browser cannot decode it (PhotoDecodeError) or the
 * server rejects it (invalid_image), otherwise "Hochladen fehlgeschlagen".
 */
export function photoErrorText(error: unknown): string {
  if (error instanceof PhotoDecodeError || problemCode(error) === "invalid_image") {
    return "Foto zu groß oder ungültig";
  }
  return "Hochladen fehlgeschlagen";
}
