import { afterEach, describe, expect, it, vi } from "vitest";
import { isTabPath, loadStartPath, saveLastTab, startPath } from "./lastTab";

// Replaces localStorage with a store in a Map that holds saved and returns
// the map.
function stubStorage(saved: Record<string, string> = {}) {
  const items = new Map(Object.entries(saved));
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => items.set(key, value),
  });
  return items;
}

// Replaces localStorage with one that throws, like Safari with blocked
// website data.
function stubBrokenStorage() {
  const fail = () => {
    throw new DOMException("", "SecurityError");
  };
  vi.stubGlobal("localStorage", { getItem: fail, setItem: fail });
}

describe("isTabPath", () => {
  it.each(["/vorrat", "/scan", "/einkauf"])("accepts the tab %s", (path) => {
    expect(isTabPath(path)).toBe(true);
  });

  it.each(["/", "/produkt/abc", "/vorrat/", "/Scan", "", null])("rejects %o", (path) => {
    expect(isTabPath(path)).toBe(false);
  });
});

describe("startPath", () => {
  it.each(["/vorrat", "/scan", "/einkauf"] as const)("leads to the saved tab %s", (path) => {
    expect(startPath(path)).toBe(path);
  });

  it.each(["", "/", "scan", "/produkt/abc", "/unbekannt", "/EINKAUF", "https://example.com/scan"])(
    "leads to /vorrat for the unknown saved value %o",
    (saved) => {
      expect(startPath(saved)).toBe("/vorrat");
    },
  );

  it("leads to /vorrat if nothing is saved", () => {
    expect(startPath(null)).toBe("/vorrat");
  });
});

describe("loadStartPath and saveLastTab", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("returns the saved tab", () => {
    stubStorage({ "stashbert.lastTab": "/einkauf" });
    expect(loadStartPath()).toBe("/einkauf");
  });

  it("returns /vorrat for an unknown saved value", () => {
    stubStorage({ "stashbert.lastTab": "/produkt/abc" });
    expect(loadStartPath()).toBe("/vorrat");
  });

  it("returns /vorrat if nothing is saved", () => {
    stubStorage();
    expect(loadStartPath()).toBe("/vorrat");
  });

  it("returns the tab saved last", () => {
    stubStorage();
    saveLastTab("/scan");
    saveLastTab("/einkauf");
    expect(loadStartPath()).toBe("/einkauf");
  });

  it.each(["/", "/produkt/abc", "/unbekannt"])("keeps the saved tab for %s", (path) => {
    const items = stubStorage({ "stashbert.lastTab": "/scan" });
    saveLastTab(path);
    expect(items.get("stashbert.lastTab")).toBe("/scan");
    expect(loadStartPath()).toBe("/scan");
  });

  it("leads to /vorrat without failing if the storage is unavailable", () => {
    stubBrokenStorage();
    expect(() => saveLastTab("/scan")).not.toThrow();
    expect(loadStartPath()).toBe("/vorrat");
  });
});
