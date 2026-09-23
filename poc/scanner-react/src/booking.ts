// Booking against the StashBert API (POC only, no generated client).
// The vite preview proxies /api to the Go server, so requests stay same-origin.

export type Kind = "add" | "consume";

export interface BookingResult {
  movementId: string;
  message: string;
  warnings: string[];
  productCreated: boolean;
}

// Outcome of one scan: either a booking or a problem from the API or network.
export type Outcome =
  | { ok: true; result: BookingResult }
  | { ok: false; status: number; code: string; detail: string };

export type Color = "green" | "blue" | "yellow" | "red";
export type Sound = "add" | "consume" | "warn" | "error";

export interface Feedback {
  color: Color;
  sound: Sound;
  text: string;
}

const TIMEOUT_MS = 5000;
const MODE_KEY = "stashbert-poc-mode";

// Planned for F08 as feedbackFor(result), see docs/plan.md.
export function feedbackFor(kind: Kind, outcome: Outcome): Feedback {
  if (outcome.ok) {
    const { message, warnings } = outcome.result;
    if (warnings.length > 0) {
      return { color: "yellow", sound: "warn", text: message };
    }
    return kind === "add"
      ? { color: "green", sound: "add", text: message }
      : { color: "blue", sound: "consume", text: message };
  }
  switch (outcome.code) {
    case "unknown_barcode":
      return { color: "red", sound: "error", text: "Unbekannter Barcode" };
    case "stock_already_zero":
      return { color: "yellow", sound: "warn", text: "War schon leer" };
    case "invalid_barcode":
      return { color: "red", sound: "error", text: "Ungültiger Barcode" };
    case "network":
      return { color: "red", sound: "error", text: "Server nicht erreichbar" };
  }
  return { color: "red", sound: "error", text: outcome.detail || `Fehler ${outcome.status}` };
}

export function bookScan(barcode: string, kind: Kind): Promise<Outcome> {
  return post("/api/v1/movements", { barcode, kind });
}

export function reverseMovement(id: string): Promise<Outcome> {
  return post(`/api/v1/movements/${encodeURIComponent(id)}/reversal`);
}

async function post(url: string, body?: unknown): Promise<Outcome> {
  let response: Response;
  try {
    response = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Idempotency-Key": crypto.randomUUID(),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch (error) {
    return { ok: false, status: 0, code: "network", detail: String(error) };
  }
  const data: unknown = await response.json().catch(() => null);
  if (response.status === 201 && isResult(data)) {
    return {
      ok: true,
      result: {
        movementId: data.movement.id,
        message: data.message,
        warnings: data.warnings,
        productCreated: data.product_created,
      },
    };
  }
  const problem = (data ?? {}) as { code?: unknown; detail?: unknown };
  return {
    ok: false,
    status: response.status,
    // A 502 from the proxy means the Go server is not running.
    code: response.status === 502 ? "network" : String(problem.code ?? ""),
    detail: String(problem.detail ?? ""),
  };
}

function isResult(data: unknown): data is {
  movement: { id: string };
  message: string;
  warnings: string[];
  product_created: boolean;
} {
  const d = data as { movement?: { id?: unknown }; message?: unknown } | null;
  return typeof d?.movement?.id === "string" && typeof d.message === "string";
}

export function loadMode(): Kind {
  try {
    return localStorage.getItem(MODE_KEY) === "consume" ? "consume" : "add";
  } catch {
    return "add";
  }
}

export function saveMode(kind: Kind): void {
  try {
    localStorage.setItem(MODE_KEY, kind);
  } catch {
    // Private mode: the mode is simply not remembered.
  }
}

export interface ShoppingItem {
  product_id: string;
  name: string;
  brand: string | null;
  missing: number;
  stock: number;
  target: number;
}

export async function loadShoppingList(): Promise<ShoppingItem[]> {
  const response = await fetch("/api/v1/shopping-list", { signal: AbortSignal.timeout(TIMEOUT_MS) });
  if (!response.ok) {
    throw new Error(`Status ${response.status}`);
  }
  const data = (await response.json()) as { items: ShoppingItem[] };
  return data.items;
}
