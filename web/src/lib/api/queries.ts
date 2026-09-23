import {
  mutationOptions,
  queryOptions,
  type QueryClient,
} from "@tanstack/react-query";
import type { ProductPatch } from "../productForm";
import {
  replaceProduct,
  sortByName,
  withBarcode,
  withoutBarcode,
  type Product,
} from "../products";
import type { MovementResult } from "../scan";
import { api, problemCode } from "./client";

/**
 * All products, sorted by name in German order. With the default
 * staleTime of 0 the list is fetched again whenever a view using it
 * mounts, so opening the stock view shows the current stock.
 */
export const productListQuery = queryOptions({
  queryKey: ["products"],
  queryFn: async () => {
    const { data, error, response } = await api.GET("/products");
    if (!response.ok || data === undefined) {
      throw error ?? new Error(`GET /products: status ${response.status}`);
    }
    return sortByName(data.items);
  },
});

/** A movement of one unit for a product, booked by product_id. */
export interface ProductMovement {
  productId: string;
  kind: "add" | "consume";
}

/**
 * Books a ProductMovement with POST /movements. A failed booking throws
 * the Problem Details of the response, so problemCode can read its code.
 * On success the product in the cached product list is replaced with
 * product from the response, and the movements of the product are fetched
 * again. On stock_already_zero the cached list is out of date (someone
 * else took the last one), so it is fetched again.
 */
export function productMovementMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ productId, kind }: ProductMovement) => {
      const { data, error, response } = await api.POST("/movements", {
        body: { product_id: productId, kind },
      });
      if (!response.ok || data === undefined) {
        throw error ?? new Error(`POST /movements: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (result, { productId }) => {
      queryClient.setQueryData(productListQuery.queryKey, (products) =>
        replaceProduct(products, result.product),
      );
      void queryClient.invalidateQueries({
        queryKey: productMovementsQuery(productId).queryKey,
      });
    },
    onError: (error) => {
      if (problemCode(error) === "stock_already_zero") {
        void queryClient.invalidateQueries({ queryKey: productListQuery.queryKey });
      }
    },
  });
}

/** A scanned code, booked with the kind of the scan mode. */
export interface ScanMovement {
  barcode: string;
  kind: "add" | "consume";
}

// How long a scan booking may take before it counts as a network error.
const scanTimeout = 10_000;

/**
 * Books a ScanMovement with POST /movements and a new Idempotency-Key. A
 * failed booking throws the Problem Details of the response; a response
 * without them (for example a 502 of a reverse proxy) throws an Error
 * with the field status. Without a response it throws the TypeError of
 * fetch, or the TimeoutError after scanTimeout. The mutation also runs
 * while the browser reports being offline, so a scan fails at once
 * instead of waiting for the network. On success product from the
 * response replaces the cached product and its entry in the product list;
 * if the booking created the product, the list is fetched again. The
 * movements of the product are fetched again.
 */
export function scanMovementMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ barcode, kind }: ScanMovement) => {
      const { data, error, response } = await api.POST("/movements", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { barcode, kind },
        signal: AbortSignal.timeout(scanTimeout),
      });
      if (!response.ok || data === undefined) {
        throw bookingError(error, response, "POST /movements");
      }
      return data;
    },
    networkMode: "always",
    onSuccess: (result) => cacheBookingResult(queryClient, result),
  });
}

/**
 * Books one more unit of a product for the result card of the scan view
 * ([+1]): POST /movements with product_id, like scanMovementMutation with
 * a new Idempotency-Key, the same timeout, the same errors and the same
 * cache updates.
 */
export function repeatMovementMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ productId, kind }: ProductMovement) => {
      const { data, error, response } = await api.POST("/movements", {
        params: { header: { "Idempotency-Key": crypto.randomUUID() } },
        body: { product_id: productId, kind },
        signal: AbortSignal.timeout(scanTimeout),
      });
      if (!response.ok || data === undefined) {
        throw bookingError(error, response, "POST /movements");
      }
      return data;
    },
    networkMode: "always",
    onSuccess: (result) => cacheBookingResult(queryClient, result),
  });
}

/**
 * Undoes the booking with the given id for the result card of the scan
 * view: POST /movements/{id}/reversal with a new Idempotency-Key. Timeout,
 * errors and cache updates are those of scanMovementMutation.
 */
export function reversalMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async (id: string) => {
      const { data, error, response } = await api.POST("/movements/{id}/reversal", {
        params: {
          path: { id },
          header: { "Idempotency-Key": crypto.randomUUID() },
        },
        signal: AbortSignal.timeout(scanTimeout),
      });
      if (!response.ok || data === undefined) {
        throw bookingError(error, response, `POST /movements/${id}/reversal`);
      }
      return data;
    },
    networkMode: "always",
    onSuccess: (result) => cacheBookingResult(queryClient, result),
  });
}

// Returns the error for a failed booking: the Problem Details of the
// response, or an Error with the field status for a response without them.
function bookingError(error: unknown, response: Response, request: string): unknown {
  if (problemCode(error) !== undefined) {
    return error;
  }
  const message = `${request}: status ${response.status}`;
  return Object.assign(new Error(message), { status: response.status });
}

// Updates the cache after a booking: product from the result replaces the
// cached product and its entry in the product list; if the booking created
// the product, the list is fetched again. The movements of the product are
// fetched again.
function cacheBookingResult(
  queryClient: QueryClient,
  { product, product_created }: MovementResult,
) {
  if (product_created) {
    queryClient.setQueryData(productQuery(product.id).queryKey, product);
    void queryClient.invalidateQueries({
      queryKey: productListQuery.queryKey,
      exact: true,
    });
  } else {
    setCachedProduct(queryClient, product);
  }
  void queryClient.invalidateQueries({
    queryKey: productMovementsQuery(product.id).queryKey,
  });
}

/**
 * One product by id. A missing product (404 not_found) is not retried, so
 * the view shows it at once.
 */
export function productQuery(id: string) {
  return queryOptions({
    queryKey: ["products", id],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/products/{id}", {
        params: { path: { id } },
      });
      if (!response.ok || data === undefined) {
        throw error ?? new Error(`GET /products/${id}: status ${response.status}`);
      }
      return data;
    },
    retry: (failureCount, error) =>
      problemCode(error) !== "not_found" && failureCount < 3,
  });
}

/** A change of the product with id, as a JSON Merge Patch. */
export interface ProductUpdate {
  id: string;
  patch: ProductPatch;
}

/**
 * Changes a product with PATCH /products/{id}. A failed change throws the
 * Problem Details of the response. On success the product from the
 * response replaces the cached product and its entry in the product list.
 */
export function productUpdateMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ id, patch }: ProductUpdate) => {
      const { data, error, response } = await api.PATCH("/products/{id}", {
        params: { path: { id } },
        body: patch,
      });
      if (!response.ok || data === undefined) {
        throw error ?? new Error(`PATCH /products/${id}: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (product) => setCachedProduct(queryClient, product),
  });
}

// Stores product from a response as the cached product and as its entry in
// the cached product list.
function setCachedProduct(queryClient: QueryClient, product: Product) {
  queryClient.setQueryData(productQuery(product.id).queryKey, product);
  queryClient.setQueryData(productListQuery.queryKey, (products) =>
    replaceProduct(products, product),
  );
}

/**
 * Deletes the product with the given id with DELETE /products/{id}. A
 * failed deletion throws the Problem Details of the response. On success
 * the product is removed from the cached product list.
 */
export function productDeleteMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/products/{id}", {
        params: { path: { id } },
      });
      if (!response.ok) {
        throw error ?? new Error(`DELETE /products/${id}: status ${response.status}`);
      }
      return id;
    },
    onSuccess: (id) => {
      queryClient.setQueryData(productListQuery.queryKey, (products) =>
        products?.filter((product) => product.id !== id),
      );
    },
  });
}

/** A barcode code of the product with productId. */
export interface ProductBarcode {
  productId: string;
  code: string;
}

// Applies change to the cached product with id and to its entry in the
// cached product list. Queries without data stay as they are.
function changeCachedProduct(
  queryClient: QueryClient,
  id: string,
  change: (product: Product) => Product,
) {
  queryClient.setQueryData(
    productQuery(id).queryKey,
    (product) => product && change(product),
  );
  queryClient.setQueryData(productListQuery.queryKey, (products) =>
    products?.map((product) => (product.id === id ? change(product) : product)),
  );
}

/**
 * Assigns a barcode to a product with POST /products/{id}/barcodes. The
 * code is sent as given (normalize it with normalizeGtin first), without
 * units. A failed assignment throws the Problem Details of the response.
 * On success the barcode from the response is added to the cached product
 * and to its entry in the product list.
 */
export function barcodeAddMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ productId, code }: ProductBarcode) => {
      const { data, error, response } = await api.POST(
        "/products/{id}/barcodes",
        { params: { path: { id: productId } }, body: { code } },
      );
      if (!response.ok || data === undefined) {
        const path = `/products/${productId}/barcodes`;
        throw error ?? new Error(`POST ${path}: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (barcode, { productId }) => {
      changeCachedProduct(queryClient, productId, (product) =>
        withBarcode(product, barcode),
      );
    },
  });
}

/**
 * Removes a barcode from a product with DELETE
 * /products/{id}/barcodes/{code}. A failed removal throws the Problem
 * Details of the response. On success the barcode is removed from the
 * cached product and from its entry in the product list. On not_found the
 * cached product is out of date, so it is fetched again.
 */
export function barcodeRemoveMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ productId, code }: ProductBarcode) => {
      const { error, response } = await api.DELETE(
        "/products/{id}/barcodes/{code}",
        { params: { path: { id: productId, code } } },
      );
      if (!response.ok) {
        const path = `/products/${productId}/barcodes/${code}`;
        throw error ?? new Error(`DELETE ${path}: status ${response.status}`);
      }
    },
    onSuccess: (_data, { productId, code }) => {
      changeCachedProduct(queryClient, productId, (product) =>
        withoutBarcode(product, code),
      );
    },
    onError: (error, { productId }) => {
      if (problemCode(error) === "not_found") {
        void queryClient.invalidateQueries({
          queryKey: productQuery(productId).queryKey,
        });
      }
    },
  });
}

// The number of movements the product page shows.
const historyLimit = 10;

/**
 * The latest movements of the product with productId, newest first, at
 * most historyLimit.
 */
export function productMovementsQuery(productId: string) {
  return queryOptions({
    queryKey: ["movements", productId],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/movements", {
        params: { query: { product_id: productId, limit: historyLimit } },
      });
      if (!response.ok || data === undefined) {
        throw error ?? new Error(`GET /movements: status ${response.status}`);
      }
      return data.items;
    },
  });
}

/** The new stock of the product with productId, from a stock count. */
export interface ProductInventory {
  productId: string;
  stock: number;
}

/**
 * Sets the stock of a product with POST /movements and kind inventory. A
 * failed booking throws the Problem Details of the response. On success
 * product from the response replaces the cached product and its entry in
 * the product list, and the movements of the product are fetched again.
 */
export function inventoryMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ productId, stock }: ProductInventory) => {
      const { data, error, response } = await api.POST("/movements", {
        body: { product_id: productId, kind: "inventory", stock },
      });
      if (!response.ok || data === undefined) {
        throw error ?? new Error(`POST /movements: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (result, { productId }) => {
      setCachedProduct(queryClient, result.product);
      void queryClient.invalidateQueries({
        queryKey: productMovementsQuery(productId).queryKey,
      });
    },
  });
}

/** The merge of the product with sourceId into the one with targetId. */
export interface ProductMerge {
  sourceId: string;
  targetId: string;
}

/**
 * Merges a product into another with POST /products/{id}/merge; the
 * server deletes the source then. A failed merge throws the Problem
 * Details of the response. On success the source is removed from the
 * cached product list, the target from the response replaces the cached
 * target and its entry in the list, and all cached movement lists are
 * fetched again, because the movements of the source moved to the target.
 */
export function productMergeMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ sourceId, targetId }: ProductMerge) => {
      const { data, error, response } = await api.POST(
        "/products/{id}/merge",
        {
          params: { path: { id: sourceId } },
          body: { target_product_id: targetId },
        },
      );
      if (!response.ok || data === undefined) {
        const path = `/products/${sourceId}/merge`;
        throw error ?? new Error(`POST ${path}: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (target, { sourceId }) => {
      queryClient.setQueryData(productListQuery.queryKey, (products) =>
        products?.filter((product) => product.id !== sourceId),
      );
      setCachedProduct(queryClient, target);
      void queryClient.invalidateQueries({ queryKey: ["movements"] });
    },
  });
}
