import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "react-router";
import { UpdateBanner } from "./components/UpdateBanner";
import "./index.css";
import { createAppRouter } from "./router";

// iOS Safari applies :active (the press state of pressable) only when a
// touchstart listener exists; an empty passive one is enough.
document.addEventListener("touchstart", () => {}, { passive: true });

const queryClient = new QueryClient();
const router = createAppRouter();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <UpdateBanner />
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  </StrictMode>,
);
