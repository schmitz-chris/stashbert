import { describe, expect, it } from "vitest";
import { pageTitle } from "./pageTitle";

describe("pageTitle", () => {
  it.each([
    ["stock", "Vorrat · StashBert"],
    ["scan", "Scannen · StashBert"],
    ["shopping", "Einkauf · StashBert"],
    ["settings", "Einstellungen · StashBert"],
    ["not-found", "Nicht gefunden · StashBert"],
  ] as const)("names the view of the route %s", (route, title) => {
    expect(pageTitle(route)).toBe(title);
  });

  it("names the product page after its product", () => {
    expect(pageTitle("product", "Jever Pilsener")).toBe("Jever Pilsener · StashBert");
  });

  it.each([undefined, ""])(
    "names the product page after the app while the name is %o",
    (name) => {
      expect(pageTitle("product", name)).toBe("StashBert");
    },
  );

  it("ignores a product name for the other views", () => {
    expect(pageTitle("stock", "Jever Pilsener")).toBe("Vorrat · StashBert");
  });
});
