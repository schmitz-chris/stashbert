import { describe, expect, it } from "vitest";
import { cartToggle } from "./cartToggle";

describe("cartToggle", () => {
  it("offers to mark a product that is not marked", () => {
    expect(cartToggle({ name: "Mehl", marked: false })).toEqual({
      pressed: false,
      label: "Auf der Einkaufsliste: Mehl",
      action: "mark",
    });
  });

  it("shows a marked product as pressed with the same name and offers to remove the mark", () => {
    expect(cartToggle({ name: "Mehl", marked: true })).toEqual({
      pressed: true,
      label: "Auf der Einkaufsliste: Mehl",
      action: "unmark",
    });
  });

  it("ignores a shortfall without a mark", () => {
    const product = { name: "Kidneybohnen", marked: false, missing: 3 };
    expect(cartToggle(product)).toEqual({
      pressed: false,
      label: "Auf der Einkaufsliste: Kidneybohnen",
      action: "mark",
    });
  });

  it("ignores a shortfall with a mark", () => {
    const product = { name: "Kidneybohnen", marked: true, missing: 3 };
    expect(cartToggle(product)).toEqual({
      pressed: true,
      label: "Auf der Einkaufsliste: Kidneybohnen",
      action: "unmark",
    });
  });
});
