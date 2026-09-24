// Automatic camera choice (docs/plan.md, F25): a virtual back camera that
// lets iOS switch between its lenses, down to macro, and its start zoom.
import type { CameraOption, ZoomRange } from "./camera";

/**
 * The virtual cameras worth choosing: "triple" (ultra wide, wide and tele)
 * and "dualWide" (ultra wide and wide). The plain dual camera (wide and
 * tele) has no ultra wide lens and so no macro.
 */
export type MultiLensKind = "triple" | "dualWide";

// iOS names cameras in the language of the system (AVCaptureDevice
// localizedName), for example "Back Triple Camera" or "Back Dual Wide
// Camera" in English. The German names are not documented, so the kind is
// found by keywords of both languages instead of whole names.
const front = /front|vorder/;
const triple = /triple|dreifach/;
const dual = /dual/;
const wide = /wide|weitwinkel/;
const tele = /tele/;

/** The kind of a virtual back camera with an ultra wide lens, or null. */
export function multiLensKind(label: string): MultiLensKind | null {
  const text = label.toLowerCase();
  if (front.test(text)) {
    return null;
  }
  if (triple.test(text)) {
    return "triple";
  }
  if (dual.test(text) && wide.test(text) && !tele.test(text)) {
    return "dualWide";
  }
  return null;
}

/**
 * The camera to use without a choice of the user: the triple camera, else
 * the dual wide camera, else null (the default back camera stays). Labels
 * are empty until the camera is allowed, so then there is no choice.
 */
export function preferredCamera<T extends CameraOption>(devices: readonly T[]): T | null {
  return (
    devices.find((device) => multiLensKind(device.label) === "triple") ??
    devices.find((device) => multiLensKind(device.label) === "dualWide") ??
    null
  );
}

/**
 * The camera to switch to after the default back camera started as
 * activeId: the preferred camera, or null if there is none or it already
 * runs.
 */
export function cameraToSwitchTo<T extends CameraOption>(
  devices: readonly T[],
  activeId: string | undefined,
): T | null {
  const preferred = preferredCamera(devices);
  return preferred !== null && preferred.deviceId !== activeId ? preferred : null;
}

/**
 * The zoom to set when a camera starts, or null to leave it. A virtual
 * camera whose zoom starts at the ultra wide lens (minimum 1) gets zoom 2,
 * the wide lens, like "1x" in the Camera app; iOS still switches to macro
 * up close. WebKit reports newer virtual cameras with a minimum of 0.5
 * instead; there zoom 1 is the wide lens already and stays.
 */
export function startZoom(range: ZoomRange | null, multiLens: boolean): number | null {
  if (!multiLens || range === null) {
    return null;
  }
  return range.min === 1 && range.max >= 2 ? 2 : null;
}
