import { describe, expect, it } from "vitest";

// The color roles of the @theme block in index.css (ADR-0016). The last
// test checks that index.css has exactly these values, so both change
// together.
const roles = {
  accent: "#007a55",
  "accent-strong": "#006045",
  "accent-soft": "#ecfdf5",
  consume: "#0069a8",
  marked: "#bb4d00",
  "marked-soft": "#ffedd4",
  warning: "#fdc700",
  "warning-soft": "#fefce8",
  danger: "#c10007",
  canvas: "#fafaf9",
  surface: "#ffffff",
  fill: "#e7e5e4",
  ink: "#1c1917",
  "ink-secondary": "#44403b",
  "ink-tertiary": "#79716b",
  line: "#e7e5e4",
  "line-strong": "#79716b",
} as const;

type Role = keyof typeof roles;

const white = "#ffffff";

// The relative luminance of an sRGB color "#rrggbb" (WCAG 2.2).
function luminance(color: string): number {
  const [r, g, b] = [1, 3, 5].map((start) => {
    const channel = Number.parseInt(color.slice(start, start + 2), 16) / 255;
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

// The contrast ratio of two colors (WCAG 2.2), from 1 to 21.
function contrast(a: string, b: string): number {
  const [lighter, darker] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (lighter + 0.05) / (darker + 0.05);
}

// Reads index.css next to this test. The project has no Node typings, so
// the one function of node:fs it needs is typed here.
function readIndexCss(): string {
  const { process } = globalThis as unknown as {
    process: {
      getBuiltinModule(id: "node:fs"): {
        readFileSync(path: URL, encoding: "utf8"): string;
      };
    };
  };
  const fs = process.getBuiltinModule("node:fs");
  return fs.readFileSync(new URL("./index.css", import.meta.url), "utf8");
}

describe("contrast", () => {
  it("follows the WCAG formula", () => {
    expect(contrast("#000000", white)).toBeCloseTo(21, 5);
    expect(contrast(white, "#000000")).toBeCloseTo(21, 5);
    expect(contrast("#777777", "#777777")).toBe(1);
    expect(contrast("#767676", white)).toBeCloseTo(4.54, 2);
  });
});

describe("color roles", () => {
  it.each<Role>(["accent", "accent-strong", "consume", "marked", "danger"])(
    "carry white text on %s with at least 4.5:1",
    (role) => {
      expect(contrast(white, roles[role])).toBeGreaterThanOrEqual(4.5);
    },
  );

  it("carry ink on warning with at least 4.5:1", () => {
    expect(contrast(roles.ink, roles.warning)).toBeGreaterThanOrEqual(4.5);
  });

  it.each<[Role, Role]>([
    ["ink", "surface"],
    ["ink", "canvas"],
    ["ink", "fill"],
    ["ink", "warning-soft"],
    ["ink-secondary", "surface"],
    ["ink-secondary", "canvas"],
    ["ink-secondary", "fill"],
    ["ink-tertiary", "surface"],
    ["ink-tertiary", "canvas"],
    ["accent", "surface"],
    ["accent", "canvas"],
    ["accent", "accent-soft"],
    ["marked", "surface"],
    ["marked", "canvas"],
    ["danger", "surface"],
    ["danger", "canvas"],
  ])("carry text in %s on %s with at least 4.5:1", (text, background) => {
    expect(contrast(roles[text], roles[background])).toBeGreaterThanOrEqual(4.5);
  });

  // Symbols of buttons and fields (F26): the cart on a marked cart button,
  // the cart, minus and plus on the gray buttons, and the magnifier and
  // the clear button of the search field.
  it.each<[Role, Role]>([
    ["marked", "marked-soft"],
    ["ink-secondary", "fill"],
    ["ink-tertiary", "fill"],
  ])("draw symbols in %s on %s with at least 3:1", (symbol, background) => {
    expect(contrast(roles[symbol], roles[background])).toBeGreaterThanOrEqual(3);
  });

  it.each<Role>(["surface", "canvas"])(
    "draw borders of fields and buttons on %s with at least 3:1",
    (background) => {
      expect(contrast(roles["line-strong"], roles[background])).toBeGreaterThanOrEqual(3);
    },
  );

  it("are the colors of @theme in index.css", () => {
    const theme = /@theme\s*\{([^}]*)\}/.exec(readIndexCss())?.[1] ?? "";
    const declared = Object.fromEntries(
      [...theme.matchAll(/--color-([a-z-]+):\s*(#[0-9a-f]{6});/g)].map(([, role, color]) => [
        role,
        color,
      ]),
    );
    expect(declared).toEqual(roles);
  });
});
