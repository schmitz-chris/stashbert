import { isValidElement } from "react";
import { matchRoutes, Navigate, type NavigateProps } from "react-router";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StartRedirect } from "./components/StartRedirect";
import { TabLayout } from "./components/TabLayout";
import { routes } from "./router";
import { ShoppingPage } from "./routes/einkauf";
import { NotFoundPage } from "./routes/nicht-gefunden";
import { ProductPage } from "./routes/produkt";
import { ScanPage } from "./routes/scan";
import { StockPage } from "./routes/vorrat";

// Returns the matched routes for path, from the outermost to the innermost.
function match(path: string) {
  const matches = matchRoutes(routes, path);
  if (matches === null) {
    throw new Error(`no route matches ${path}`);
  }
  return matches;
}

// Replaces localStorage with a store that holds saved.
function stubStorage(saved: Record<string, string>) {
  vi.stubGlobal("localStorage", {
    getItem: (key: string) => saved[key] ?? null,
    setItem: () => {},
  });
}

// Returns the props of the Navigate element the route for / renders.
function startRedirect() {
  const matches = match("/");
  expect(matches.map((m) => m.route.id)).toEqual(["redirect"]);
  expect(matches[0].route.Component).toBe(StartRedirect);

  // StartRedirect uses no hooks, so it can be called without a DOM.
  const element = StartRedirect();
  if (!isValidElement<NavigateProps>(element)) {
    throw new Error("the route for / renders no element");
  }
  expect(element.type).toBe(Navigate);
  return element.props;
}

describe("routes", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it.each(["/vorrat", "/scan", "/einkauf"])(
    "redirects / to the tab %s shown last with replace",
    (path) => {
      stubStorage({ "stashbert.lastTab": path });
      expect(startRedirect()).toEqual({ to: path, replace: true });
    },
  );

  it.each<Record<string, string>>([{}, { "stashbert.lastTab": "/produkt/abc" }])(
    "redirects / to /vorrat with replace without a saved tab (%o)",
    (saved) => {
      stubStorage(saved);
      expect(startRedirect()).toEqual({ to: "/vorrat", replace: true });
    },
  );

  it.each([
    ["/vorrat", "stock", StockPage],
    ["/scan", "scan", ScanPage],
    ["/einkauf", "shopping", ShoppingPage],
    ["/produkt/abc", "product", ProductPage],
  ])("shows %s below the layout with the navigation bar", (path, id, page) => {
    const matches = match(path);
    expect(matches.map((m) => m.route.id)).toEqual(["tabs", id]);
    expect(matches[0].route.Component).toBe(TabLayout);
    expect(matches[1].route.Component).toBe(page);
  });

  it("passes the id of /produkt/abc to the product page", () => {
    const matches = match("/produkt/abc");
    expect(matches[1].params).toEqual({ id: "abc" });
  });

  it.each(["/unbekannt", "/produkt", "/vorrat/abc", "/produkt/abc/mehr"])(
    "shows the not found page for %s",
    (path) => {
      const matches = match(path);
      expect(matches.map((m) => m.route.id)).toEqual(["not-found"]);
      expect(matches[0].route.Component).toBe(NotFoundPage);
    },
  );
});
