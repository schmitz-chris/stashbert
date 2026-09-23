import { createBrowserRouter, Navigate, type RouteObject } from "react-router";
import { TabLayout } from "./components/TabLayout";
import { ShoppingPage } from "./routes/einkauf";
import { NotFoundPage } from "./routes/nicht-gefunden";
import { ProductPage } from "./routes/produkt";
import { ScanPage } from "./routes/scan";
import { StockPage } from "./routes/vorrat";

/**
 * The route table. Every route has an id, so tests can check with
 * matchRoutes which view a path shows, without a DOM. Only the routes
 * below TabLayout show the bottom navigation bar.
 */
export const routes: RouteObject[] = [
  { id: "redirect", path: "/", element: <Navigate to="/vorrat" replace /> },
  {
    id: "tabs",
    Component: TabLayout,
    children: [
      { id: "stock", path: "/vorrat", Component: StockPage },
      { id: "scan", path: "/scan", Component: ScanPage },
      { id: "shopping", path: "/einkauf", Component: ShoppingPage },
    ],
  },
  { id: "product", path: "/produkt/:id", Component: ProductPage },
  { id: "not-found", path: "*", Component: NotFoundPage },
];

/**
 * Creates the browser router from routes. It is a function, so that
 * importing routes in tests does not need a browser window.
 */
export function createAppRouter() {
  return createBrowserRouter(routes);
}
