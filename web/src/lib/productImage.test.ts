import { describe, expect, it } from "vitest";
import {
  fitWithin,
  maxPhotoEdge,
  PhotoDecodeError,
  photoErrorText,
  productImageUrl,
} from "./productImage";

describe("fitWithin", () => {
  it("shrinks a portrait photo to a height of 1024", () => {
    expect(fitWithin(3000, 4000, maxPhotoEdge)).toEqual({ width: 768, height: 1024 });
  });

  it("shrinks a landscape photo to a width of 1024", () => {
    expect(fitWithin(3000, 2000, maxPhotoEdge)).toEqual({ width: 1024, height: 683 });
  });

  it("shrinks a square photo to 1024 × 1024", () => {
    expect(fitWithin(4032, 4032, maxPhotoEdge)).toEqual({ width: 1024, height: 1024 });
  });

  it("keeps the size of a smaller photo", () => {
    expect(fitWithin(800, 600, maxPhotoEdge)).toEqual({ width: 800, height: 600 });
    expect(fitWithin(300, 1000, maxPhotoEdge)).toEqual({ width: 300, height: 1000 });
  });

  it("keeps the size of a photo with a longest edge of exactly 1024", () => {
    expect(fitWithin(1024, 768, maxPhotoEdge)).toEqual({ width: 1024, height: 768 });
    expect(fitWithin(768, 1024, maxPhotoEdge)).toEqual({ width: 768, height: 1024 });
    expect(fitWithin(1024, 1024, maxPhotoEdge)).toEqual({ width: 1024, height: 1024 });
  });

  it("keeps a very narrow edge at least 1 pixel", () => {
    expect(fitWithin(5000, 2, maxPhotoEdge)).toEqual({ width: 1024, height: 1 });
  });
});

describe("productImageUrl", () => {
  const product = {
    id: "0192f0c4-7d1e-7c3a-9b1a-2f6d8e4a1b2c",
    has_image: true,
    updated_at: "2026-09-24T10:15:30.123Z",
  };

  it("returns null for a product without an image", () => {
    expect(productImageUrl({ ...product, has_image: false })).toBeNull();
  });

  it("adds updated_at as cache buster to the image URL", () => {
    expect(productImageUrl(product)).toBe(
      "/api/v1/products/0192f0c4-7d1e-7c3a-9b1a-2f6d8e4a1b2c/image?v=2026-09-24T10%3A15%3A30.123Z",
    );
  });

  it("changes the URL when updated_at changes", () => {
    const later = { ...product, updated_at: "2026-09-24T10:16:02.456Z" };
    expect(productImageUrl(later)).not.toBe(productImageUrl(product));
  });

  it("encodes the id", () => {
    expect(productImageUrl({ ...product, id: "a/b" })).toBe(
      "/api/v1/products/a%2Fb/image?v=2026-09-24T10%3A15%3A30.123Z",
    );
  });
});

describe("photoErrorText", () => {
  it("reports a photo the browser cannot decode as invalid", () => {
    expect(photoErrorText(new PhotoDecodeError())).toBe("Foto zu groß oder ungültig");
  });

  it("reports invalid_image of the server as invalid", () => {
    const problem = { status: 422, code: "invalid_image", title: "Ungültiges Bild" };
    expect(photoErrorText(problem)).toBe("Foto zu groß oder ungültig");
  });

  it("reports other errors as a failed upload", () => {
    expect(photoErrorText({ status: 404, code: "not_found" })).toBe("Hochladen fehlgeschlagen");
    expect(photoErrorText(new TypeError("Failed to fetch"))).toBe("Hochladen fehlgeschlagen");
    expect(photoErrorText(new DOMException("timeout", "TimeoutError"))).toBe(
      "Hochladen fehlgeschlagen",
    );
    expect(photoErrorText(new Error("PUT: status 502"))).toBe("Hochladen fehlgeschlagen");
  });
});
