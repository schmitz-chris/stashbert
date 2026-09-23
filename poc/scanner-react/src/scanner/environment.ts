// Environment details for the status area.

// navigator.standalone is Safari only and missing from the TypeScript DOM typings.
type StandaloneNavigator = Navigator & { standalone?: boolean };

export function displayMode(): "standalone" | "browser" {
  const standalone =
    window.matchMedia("(display-mode: standalone)").matches ||
    (navigator as StandaloneNavigator).standalone === true;
  return standalone ? "standalone" : "browser";
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.name && error.name !== "Error" ? `${error.name}: ${error.message}` : error.message;
  }
  return String(error);
}
