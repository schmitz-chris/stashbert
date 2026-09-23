import { afterEach, describe, expect, it, vi } from "vitest";
import { createApiClient, problemCode } from "./client";

// openapi-fetch reads globalThis.fetch and globalThis.Request when the
// client is created, so both are stubbed before createApiClient. The
// Request subclass records the options passed by the client, because a
// Request reports credentials "same-origin" even when none were given.
function stubNetwork(response: Response) {
  const inits: (RequestInit | undefined)[] = [];
  class RecordingRequest extends Request {
    constructor(input: RequestInfo | URL, init?: RequestInit) {
      super(input, init);
      inits.push(init);
    }
  }
  const fetchMock = vi.fn<(request: Request) => Promise<Response>>(
    async () => response,
  );
  vi.stubGlobal("Request", RecordingRequest);
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, inits };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createApiClient", () => {
  it("sends requests below the base URL with same-origin credentials", async () => {
    const { fetchMock, inits } = stubNetwork(Response.json({ items: [] }));
    const client = createApiClient("http://test/api/v1");

    const { data, error } = await client.GET("/products");

    expect(error).toBeUndefined();
    expect(data).toEqual({ items: [] });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const request = fetchMock.mock.calls[0][0];
    expect(request.method).toBe("GET");
    expect(request.url).toBe("http://test/api/v1/products");
    expect(request.credentials).toBe("same-origin");
    expect(inits).toHaveLength(1);
    expect(inits[0]?.credentials).toBe("same-origin");
  });

  it("exposes the problem code of an error response", async () => {
    const { fetchMock } = stubNetwork(
      new Response(
        JSON.stringify({
          type: "about:blank",
          title: "Not Found",
          status: 404,
          detail: "Barcode 4001234567890 ist unbekannt.",
          code: "unknown_barcode",
        }),
        {
          status: 404,
          headers: { "Content-Type": "application/problem+json" },
        },
      ),
    );
    const client = createApiClient("http://test/api/v1");

    const { data, error } = await client.POST("/movements", {
      body: { barcode: "4001234567890", kind: "consume" },
    });

    expect(data).toBeUndefined();
    expect(problemCode(error)).toBe("unknown_barcode");
    const request = fetchMock.mock.calls[0][0];
    expect(request.method).toBe("POST");
    expect(request.url).toBe("http://test/api/v1/movements");
  });
});

describe("problemCode", () => {
  it("returns the code of a problem object", () => {
    expect(
      problemCode({ title: "Conflict", status: 409, code: "barcode_in_use" }),
    ).toBe("barcode_in_use");
  });

  it.each([
    ["undefined", undefined],
    ["null", null],
    ["a string", "unknown_barcode"],
    ["an object without code", { title: "Not Found", status: 404 }],
    ["an object with a number as code", { status: 404, code: 404 }],
  ])("returns undefined for %s", (_name, value) => {
    expect(problemCode(value)).toBeUndefined();
  });
});
