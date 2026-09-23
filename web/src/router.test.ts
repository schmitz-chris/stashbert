import { isValidElement } from "react";
import { matchRoutes, Navigate, type NavigateProps } from "react-router";
import { describe, expect, it } from "vitest";
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

describe("routes", () => {
  it("redirects / to /vorrat with replace", () => {
    const matches = match("/");
    expect(matches.map((m) => m.route.id)).toEqual(["redirect"]);

    const element = matches[0].route.element;
    if (!isValidElement<NavigateProps>(element)) {
      throw new Error("the route for / has no element");
    }
    expect(element.type).toBe(Navigate);
    expect(element.props).toEqual({ to: "/vorrat", replace: true });
  });

  it.each([
    ["/vorrat", "stock", StockPage],
    ["/scan", "scan", ScanPage],
    ["/einkauf", "shopping", ShoppingPage],
  ])("shows %s below the layout with the navigation bar", (path, id, page) => {
    const matches = match(path);
    expect(matches.map((m) => m.route.id)).toEqual(["tabs", id]);
    expect(matches[0].route.Component).toBe(TabLayout);
    expect(matches[1].route.Component).toBe(page);
  });

  it("shows /produkt/abc without the navigation bar", () => {
    const matches = match("/produkt/abc");
    expect(matches.map((m) => m.route.id)).toEqual(["product"]);
    expect(matches[0].route.Component).toBe(ProductPage);
    expect(matches[0].params).toEqual({ id: "abc" });
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
