import createClient from "openapi-fetch";
import type { paths } from "./schema";

/**
 * Creates a typed client for the StashBert API. Paths, parameters and
 * bodies are checked against the types generated from api/openapi.yaml.
 * baseUrl is the API prefix, for example "/api/v1".
 */
export function createApiClient(baseUrl: string) {
  return createClient<paths>({ baseUrl, credentials: "same-origin" });
}

/** The client for the API of the server that delivers the web UI. */
export const api = createApiClient("/api/v1");

/**
 * Returns the field code of a Problem Details object (RFC 9457), for
 * example "unknown_barcode". Returns undefined if error is not an object
 * or has no string field code.
 */
export function problemCode(error: unknown): string | undefined {
  if (typeof error !== "object" || error === null || !("code" in error)) {
    return undefined;
  }
  return typeof error.code === "string" ? error.code : undefined;
}
