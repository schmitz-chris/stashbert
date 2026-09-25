import { describe, expect, it } from "vitest";
import {
  crateSizeMissing,
  diffPatch,
  recognizedFields,
  toForm,
  withCrate,
  withRecognition,
  type ProductForm,
  type ProductFormFields,
} from "./productForm";

const original: ProductFormFields = {
  name: "Nutella",
  brand: "Ferrero",
  package_size: "450 g",
  target: 2,
  crate_size: null,
  note: null,
};

// A product bought in crates of 20.
const crated: ProductFormFields = { ...original, crate_size: 20 };

function form(fields: Partial<ProductForm>): ProductForm {
  return { ...toForm(original), ...fields };
}

describe("toForm", () => {
  it("turns missing texts into empty strings and the target into text", () => {
    expect(toForm(original)).toEqual({
      name: "Nutella",
      brand: "Ferrero",
      package_size: "450 g",
      target: "2",
      crate: false,
      crate_size: "",
      note: "",
    });
  });

  it("starts a product with a crate size switched on, with its size", () => {
    expect(toForm(crated)).toMatchObject({ crate: true, crate_size: "20" });
  });
});

describe("withCrate", () => {
  it("switches the crate on with an empty size", () => {
    expect(withCrate(toForm(original), true)).toEqual({
      ...toForm(original),
      crate: true,
      crate_size: "",
    });
  });

  it("switches the crate off and empties the size", () => {
    expect(withCrate(toForm(crated), false)).toEqual({
      ...toForm(crated),
      crate: false,
      crate_size: "",
    });
  });

  it("starts with an empty size when switched on again", () => {
    const again = withCrate(withCrate(toForm(crated), false), true);
    expect(again).toMatchObject({ crate: true, crate_size: "" });
  });
});

describe("crateSizeMissing", () => {
  it("is false without a crate", () => {
    expect(crateSizeMissing(toForm(original))).toBe(false);
  });

  it("is false for a crate with a size from 2 to 100", () => {
    for (const size of ["2", "20", " 24 ", "100"]) {
      expect(crateSizeMissing({ ...toForm(original), crate: true, crate_size: size })).toBe(false);
    }
  });

  it("is true for a crate switched on without a size", () => {
    expect(crateSizeMissing(withCrate(toForm(original), true))).toBe(true);
  });

  it("is true for a size that is not a whole number from 2 to 100", () => {
    for (const size of ["1", "101", "0", "2.5", "-3", "abc", "  "]) {
      expect(crateSizeMissing({ ...toForm(original), crate: true, crate_size: size })).toBe(true);
    }
  });
});

describe("diffPatch", () => {
  it("returns an empty patch without changes", () => {
    expect(diffPatch(original, toForm(original))).toEqual({});
  });

  it("contains only a changed name", () => {
    expect(diffPatch(original, form({ name: "Nutella Hazelnut" }))).toEqual({
      name: "Nutella Hazelnut",
    });
  });

  it("sends a cleared brand as null", () => {
    expect(diffPatch(original, form({ brand: "" }))).toEqual({ brand: null });
  });

  it("sends a brand of only spaces as null", () => {
    expect(diffPatch(original, form({ brand: "   " }))).toEqual({
      brand: null,
    });
  });

  it("does not count spaces around a text as a change", () => {
    const spaced = form({ name: "  Nutella ", brand: " Ferrero", note: "  " });
    expect(diffPatch(original, spaced)).toEqual({});
  });

  it("sends a changed text trimmed", () => {
    expect(diffPatch(original, form({ package_size: " 750 g " }))).toEqual({
      package_size: "750 g",
    });
  });

  it("contains a changed target as a number", () => {
    expect(diffPatch(original, form({ target: "5" }))).toEqual({ target: 5 });
  });

  it("sends a target of 0", () => {
    expect(diffPatch(original, form({ target: "0" }))).toEqual({ target: 0 });
  });

  it("ignores a target that is not a number", () => {
    expect(diffPatch(original, form({ target: "" }))).toEqual({});
    expect(diffPatch(original, form({ target: " " }))).toEqual({});
  });

  it("contains the size of a crate switched on", () => {
    const switchedOn = { ...withCrate(toForm(original), true), crate_size: "20" };
    expect(diffPatch(original, switchedOn)).toEqual({ crate_size: 20 });
  });

  it("contains a changed crate size as a number", () => {
    const changed = { ...toForm(crated), crate_size: "24" };
    expect(diffPatch(crated, changed)).toEqual({ crate_size: 24 });
  });

  it("sends a crate switched off as null", () => {
    expect(diffPatch(crated, withCrate(toForm(crated), false))).toEqual({
      crate_size: null,
    });
    // The size in the hidden field does not count.
    expect(diffPatch(crated, { ...toForm(crated), crate: false })).toEqual({
      crate_size: null,
    });
  });

  it("does not count an unchanged crate as a change", () => {
    expect(diffPatch(crated, toForm(crated))).toEqual({});
    expect(diffPatch(crated, { ...toForm(crated), crate_size: " 20 " })).toEqual(
      {},
    );
    const onAndOff = withCrate(withCrate(toForm(original), true), false);
    expect(diffPatch(original, onAndOff)).toEqual({});
  });

  it("ignores an empty size of a crate switched on, which the form must catch", () => {
    expect(diffPatch(original, withCrate(toForm(original), true))).toEqual({});
    expect(diffPatch(crated, { ...toForm(crated), crate_size: " " })).toEqual(
      {},
    );
  });

  it("ignores a crate size that is not a number", () => {
    expect(diffPatch(crated, { ...toForm(crated), crate_size: "x" })).toEqual(
      {},
    );
  });

  it("sends a note that was null before", () => {
    expect(diffPatch(original, form({ note: "Im Keller" }))).toEqual({
      note: "Im Keller",
    });
  });

  it("keeps line breaks inside a note", () => {
    expect(diffPatch(original, form({ note: "Zeile 1\nZeile 2\n" }))).toEqual(
      { note: "Zeile 1\nZeile 2" },
    );
  });

  it("contains an empty name, which the form must catch", () => {
    expect(diffPatch(original, form({ name: "  " }))).toEqual({ name: "" });
  });

  it("contains all changed fields", () => {
    const changed = form({
      name: "Nuss-Nougat-Creme",
      brand: "",
      package_size: "1 kg",
      target: "3",
      crate: true,
      crate_size: "6",
      note: "Für das Frühstück",
    });
    expect(diffPatch(original, changed)).toEqual({
      name: "Nuss-Nougat-Creme",
      brand: null,
      package_size: "1 kg",
      target: 3,
      crate_size: 6,
      note: "Für das Frühstück",
    });
  });
});

describe("recognizedFields", () => {
  it("takes every field that was read", () => {
    expect(
      recognizedFields({ name: "Kidneybohnen", brand: "Bonduelle", package_size: "400 g" }),
    ).toEqual({ name: "Kidneybohnen", brand: "Bonduelle", package_size: "400 g" });
  });

  it("leaves out the fields that could not be read", () => {
    expect(recognizedFields({ name: "Kidneybohnen", brand: null, package_size: null })).toEqual({
      name: "Kidneybohnen",
    });
  });

  it("leaves out empty fields and trims the others", () => {
    expect(recognizedFields({ name: "  ", brand: " Bonduelle ", package_size: "" })).toEqual({
      brand: "Bonduelle",
    });
  });

  it("is empty when nothing could be read", () => {
    expect(recognizedFields({ name: null, brand: null, package_size: null })).toEqual({});
  });
});

describe("withRecognition", () => {
  // A form with changes the user typed before the recognition.
  const typed = form({
    name: "Bohnen",
    brand: "",
    package_size: "1 Dose",
    target: "4",
    note: "Keller",
  });

  it("puts every field that was read in place of the typed values", () => {
    expect(
      withRecognition(typed, { name: "Kidneybohnen", brand: "Bonduelle", package_size: "400 g" }),
    ).toEqual({
      ...typed,
      name: "Kidneybohnen",
      brand: "Bonduelle",
      package_size: "400 g",
    });
  });

  it("keeps the values of the fields that could not be read", () => {
    expect(withRecognition(typed, { name: null, brand: "Bonduelle", package_size: null })).toEqual(
      { ...typed, brand: "Bonduelle" },
    );
  });

  it("keeps target, crate and note", () => {
    const crate = { ...withCrate(typed, true), crate_size: "6" };
    const result = withRecognition(crate, {
      name: "Kidneybohnen",
      brand: "Bonduelle",
      package_size: "400 g",
    });
    expect(result).toMatchObject({ target: "4", crate: true, crate_size: "6", note: "Keller" });
  });

  it("keeps the form when nothing could be read", () => {
    expect(withRecognition(typed, { name: null, brand: null, package_size: null })).toEqual(typed);
  });

  it("gives a patch with the suggestion, which the form saves as usual", () => {
    const suggested = withRecognition(toForm(original), {
      name: "Nutella",
      brand: null,
      package_size: "750 g",
    });
    expect(diffPatch(original, suggested)).toEqual({ package_size: "750 g" });
  });
});
