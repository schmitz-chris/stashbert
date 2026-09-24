import { fitWithin, maxPhotoEdge, PhotoDecodeError } from "./productImage";

// The JPEG quality of an uploaded photo.
const photoQuality = 0.8;

/**
 * Shrinks photo for the upload (architecture.md 7.3): decodes it with
 * createImageBitmap, draws it on a canvas whose longest edge is at most
 * maxPhotoEdge (see fitWithin; smaller photos keep their size) and encodes
 * it as JPEG with quality 0.8. Throws PhotoDecodeError if the browser
 * cannot decode photo.
 */
export async function shrinkPhoto(photo: Blob): Promise<Blob> {
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(photo);
  } catch (error) {
    throw new PhotoDecodeError({ cause: error });
  }
  try {
    const { width, height } = fitWithin(bitmap.width, bitmap.height, maxPhotoEdge);
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext("2d");
    if (context === null) {
      throw new Error("shrink photo: no 2d canvas context");
    }
    // JPEG has no transparency; transparent pixels would turn black.
    context.fillStyle = "#ffffff";
    context.fillRect(0, 0, width, height);
    context.drawImage(bitmap, 0, 0, width, height);
    return await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(
        (blob) => {
          if (blob === null) {
            reject(new Error("shrink photo: canvas cannot be encoded as JPEG"));
          } else {
            resolve(blob);
          }
        },
        "image/jpeg",
        photoQuality,
      );
    });
  } finally {
    bitmap.close();
  }
}
