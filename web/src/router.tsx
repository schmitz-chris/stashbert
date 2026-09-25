import { createBrowserRouter, type RouteObject } from "react-router";
import { StartRedirect } from "./components/StartRedirect";
import { TabLayout } from "./components/TabLayout";
import { ShoppingPage } from "./routes/einkauf";
import { SettingsPage } from "./routes/einstellungen";
import { NotFoundPage } from "./routes/nicht-gefunden";
import { ProductPage } from "./routes/produkt";
import { ScanPage } from "./routes/scan";
import { StockPage } from "./routes/vorrat";

/**
 * The route table. Every route has an id, so tests can check with
 * matchRoutes which view a path shows, without a DOM. Only the routes
 * below TabLayout show the bottom navigation bar; the product page and the
 * settings are among them, so the bar stays visible there (docs/plan.md,
 * F22 and F32). The start of the app (/) leads to the tab shown last
 * (F24).
 */
export const routes: RouteObject[] = [
  { id: "redirect", path: "/", Component: StartRedirect },
  {
    id: "tabs",
    Component: TabLayout,
    children: [
      { id: "stock", path: "/vorrat", Component: StockPage },
      { id: "scan", path: "/scan", Component: ScanPage },
      { id: "shopping", path: "/einkauf", Component: ShoppingPage },
      { id: "product", path: "/produkt/:id", Component: ProductPage },
      { id: "settings", path: "/einstellungen", Component: SettingsPage },
    ],
  },
  { id: "not-found", path: "*", Component: NotFoundPage },
];

/**
 * Creates the browser router from routes. It is a function, so that
 * importing routes in tests does not need a browser window.
 */
export function createAppRouter() {
  return createBrowserRouter(routes);
}
