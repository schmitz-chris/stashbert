import { describe, expect, it } from "vitest";
import type { CameraOption } from "./camera";
import { cameraToSwitchTo, multiLensKind, preferredCamera, startZoom } from "./cameraChoice";

// Device lists as enumerateDevices() returns them (videoinput only), with
// made up ids. The English names are those of AVCaptureDevice
// (localizedName), which Safari passes on as the label. The German names are
// guesses: Apple does not document them, so the tests use several spellings.
function devices(...labels: string[]): CameraOption[] {
  return labels.map((label, index) => ({ deviceId: `id-${index}`, label }));
}

const iPhone15 = devices(
  "Front Camera",
  "Back Camera",
  "Back Ultra Wide Camera",
  "Back Dual Wide Camera",
);

const iPhone16Pro = devices(
  "Front Camera",
  "Back Camera",
  "Back Ultra Wide Camera",
  "Back Telephoto Camera",
  "Back Dual Camera",
  "Back Dual Wide Camera",
  "Back Triple Camera",
);

const iPhone15German = devices(
  "Frontkamera",
  "Rückseitige Kamera",
  "Rückseitige Ultraweitwinkel-Kamera",
  "Rückseitige Dual-Weitwinkel-Kamera",
);

const iPhone16ProGerman = devices(
  "Frontkamera",
  "Rückseitige Kamera",
  "Rückseitige Ultraweitwinkel-Kamera",
  "Rückseitige Telekamera",
  "Rückseitige Dual-Kamera",
  "Rückseitige Dual-Weitwinkel-Kamera",
  "Rückseitige Triple-Kamera",
);

function chosenLabel(list: CameraOption[]): string | null {
  return preferredCamera(list)?.label ?? null;
}

describe("preferredCamera", () => {
  it("chooses the dual wide camera on an iPhone 15", () => {
    expect(chosenLabel(iPhone15)).toBe("Back Dual Wide Camera");
  });

  it("chooses the triple camera on an iPhone 16 Pro", () => {
    expect(chosenLabel(iPhone16Pro)).toBe("Back Triple Camera");
  });

  it("chooses the triple camera before the dual wide camera in any order", () => {
    expect(chosenLabel([...iPhone16Pro].reverse())).toBe("Back Triple Camera");
  });

  it("chooses by German names", () => {
    expect(chosenLabel(iPhone15German)).toBe("Rückseitige Dual-Weitwinkel-Kamera");
    expect(chosenLabel(iPhone16ProGerman)).toBe("Rückseitige Triple-Kamera");
  });

  it("returns the device with its id", () => {
    expect(preferredCamera(iPhone16Pro)).toEqual({ deviceId: "id-6", label: "Back Triple Camera" });
  });

  it("keeps the default camera on an iPad without virtual cameras", () => {
    expect(chosenLabel(devices("Front Camera", "Back Camera"))).toBeNull();
    expect(chosenLabel(devices("Front Ultra Wide Camera", "Back Camera", "Back Ultra Wide Camera"))).toBeNull();
    expect(chosenLabel(devices("Frontkamera", "Rückseitige Kamera"))).toBeNull();
  });

  it("keeps the default camera with Android names", () => {
    expect(
      chosenLabel(
        devices(
          "camera2 1, facing front",
          "camera2 0, facing back",
          "camera2 2, facing back",
          "camera2 3, facing back",
        ),
      ),
    ).toBeNull();
  });

  it("keeps the default camera while the labels are empty (camera not allowed yet)", () => {
    expect(chosenLabel(devices("", "", "", ""))).toBeNull();
    expect(preferredCamera([])).toBeNull();
  });

  it("never chooses a single lens or the plain dual camera", () => {
    expect(
      chosenLabel(
        devices("Back Camera", "Back Ultra Wide Camera", "Back Telephoto Camera", "Back Dual Camera"),
      ),
    ).toBeNull();
    expect(
      chosenLabel(
        devices(
          "Rückseitige Kamera",
          "Rückseitige Ultraweitwinkel-Kamera",
          "Rückseitige Telekamera",
          "Rückseitige Dual-Kamera",
        ),
      ),
    ).toBeNull();
  });
});

describe("multiLensKind", () => {
  it.each([
    ["Back Triple Camera", "triple"],
    ["Rückseitige Triple-Kamera", "triple"],
    ["Rückseitige Dreifachkamera", "triple"],
    ["Back Dual Wide Camera", "dualWide"],
    ["Rückseitige Dual-Weitwinkel-Kamera", "dualWide"],
    ["Rückseitige Dual-Weitwinkelkamera", "dualWide"],
    ["Dual-Kamera mit Ultraweitwinkel", "dualWide"],
  ])("finds %s as %s", (label, kind) => {
    expect(multiLensKind(label)).toBe(kind);
  });

  it.each([
    "Back Camera",
    "Back Ultra Wide Camera",
    "Back Telephoto Camera",
    "Back Dual Camera",
    "Rückseitige Dual-Kamera",
    "Dual-Kamera mit Weitwinkel und Tele",
    "Rückseitige Ultraweitwinkel-Kamera",
    "Front Camera",
    "Front TrueDepth Camera",
    "Front Triple Camera",
    "Vorderseitige Dual-Weitwinkel-Kamera",
    "Frontkamera (Dreifach)",
    "camera2 0, facing back",
    "",
  ])("finds no virtual back camera with an ultra wide lens in %j", (label) => {
    expect(multiLensKind(label)).toBeNull();
  });
});

describe("cameraToSwitchTo", () => {
  it("switches from the default back camera to the preferred one", () => {
    expect(cameraToSwitchTo(iPhone16Pro, "id-1")?.label).toBe("Back Triple Camera");
    expect(cameraToSwitchTo(iPhone15, "id-1")?.label).toBe("Back Dual Wide Camera");
  });

  it("does not switch if the default camera is the preferred one already", () => {
    expect(cameraToSwitchTo(iPhone16Pro, "id-6")).toBeNull();
    expect(cameraToSwitchTo(iPhone15, "id-3")).toBeNull();
  });

  it("does not switch without a preferred camera", () => {
    expect(cameraToSwitchTo(devices("Front Camera", "Back Camera"), "id-1")).toBeNull();
    expect(cameraToSwitchTo(devices("", ""), "")).toBeNull();
  });

  it("switches if the running camera is unknown", () => {
    expect(cameraToSwitchTo(iPhone15, undefined)?.label).toBe("Back Dual Wide Camera");
  });
});

describe("startZoom", () => {
  it("sets zoom 2 on a virtual camera that starts at the ultra wide lens", () => {
    expect(startZoom({ min: 1, max: 10, step: 0.1 }, true)).toBe(2);
    expect(startZoom({ min: 1, max: 2, step: 0.1 }, true)).toBe(2);
  });

  it("leaves the zoom if the range does not reach 2", () => {
    expect(startZoom({ min: 1, max: 1.5, step: 0.1 }, true)).toBeNull();
  });

  it("leaves the zoom at 1 if the minimum is below 1", () => {
    expect(startZoom({ min: 0.5, max: 5, step: 0.1 }, true)).toBeNull();
  });

  it("leaves the zoom of a camera that is not virtual", () => {
    expect(startZoom({ min: 1, max: 10, step: 0.1 }, false)).toBeNull();
  });

  it("leaves the zoom if the camera has none", () => {
    expect(startZoom(null, true)).toBeNull();
  });
});
