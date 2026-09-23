import { describe, expect, it } from "vitest";
import {
  diffPatch,
  toForm,
  type ProductForm,
  type ProductFormFields,
} from "./productForm";

const original: ProductFormFields = {
  name: "Nutella",
  brand: "Ferrero",
  package_size: "450 g",
  target: 2,
  note: null,
};

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
      note: "",
    });
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
      note: "Für das Frühstück",
    });
    expect(diffPatch(original, changed)).toEqual({
      name: "Nuss-Nougat-Creme",
      brand: null,
      package_size: "1 kg",
      target: 3,
      note: "Für das Frühstück",
    });
  });
});
