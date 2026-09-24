import {
  mutationOptions,
  queryOptions,
  type QueryClient,
} from "@tanstack/react-query";
import { shrinkPhoto } from "../photo";
import type { ProductPatch } from "../productForm";
import {
  replaceProduct,
  sortByName,
  withBarcode,
  withoutBarcode,
  type Product,
} from "../products";
import type { MarkResult, MovementResult } from "../scan";
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

/**
 * The shopping list: all products with missing > 0 or marked, in the order
 * of the server (sorted by name). With the default staleTime of 0 it is
 * fetched again whenever a view using it mounts, so opening the shopping
 * view shows the current list.
 */
export const shoppingListQuery = queryOptions({
  queryKey: ["shopping-list"],
  queryFn: async () => {
    const { data, error, response } = await api.GET("/shopping-list");
    if (!response.ok || data === undefined) {
      throw error ?? new Error(`GET /shopping-list: status ${response.status}`);
    }
    return data.items;
  },
});

// Marks the cached shopping list as out of date after a change of stock,
// target or mark, so it is fetched again. Every mutation that changes
// stock, target or mark of a product (bookings, reversals, stock counts,
// changes, marks, merges and deletions) calls it.
function invalidateShoppingList(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: shoppingListQuery.queryKey });
}

/** A movement of one unit for a product, booked by product_id. */
export interface ProductMovement {
  productId: string;
  kind: "add" | "consume";
}

/**
 * Books a ProductMovement with POST /movements. A failed booking throws
 * the Problem Details of the response, so problemCode can read its code.
 * On success the product in the cached product list is replaced with
 * product from the response, and the movements of the product and the
 * shopping list are fetched again. On stock_already_zero the cached lists
 * are out of date (someone else took the last one), so the product list
 * and the shopping list are fetched again.
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
      invalidateShoppingList(queryClient);
    },
    onError: (error) => {
      if (problemCode(error) === "stock_already_zero") {
        void queryClient.invalidateQueries({ queryKey: productListQuery.queryKey });
        invalidateShoppingList(queryClient);
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
 * movements of the product and the shopping list are fetched again.
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
// the product, the list is fetched again. The movements of the product and
// the shopping list are fetched again.
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
  invalidateShoppingList(queryClient);
}

/**
 * Marks a scanned code for shopping with POST /shopping-list/items
 * (docs/plan.md, F15). Timeout and errors are those of
 * scanMovementMutation, and it also runs while the browser reports being
 * offline. On success product from the response replaces the cached
 * product and its entry in the product list; if the mark created the
 * product, the list is fetched again. The shopping list is fetched again.
 */
export function markScanMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async (barcode: string) => {
      const { data, error, response } = await api.POST("/shopping-list/items", {
        body: { barcode },
        signal: AbortSignal.timeout(scanTimeout),
      });
      if (!response.ok || data === undefined) {
        throw bookingError(error, response, "POST /shopping-list/items");
      }
      return data;
    },
    networkMode: "always",
    onSuccess: (result) => cacheMarkResult(queryClient, result),
  });
}

/**
 * Marks the product with the given id for shopping with POST
 * /shopping-list/items and product_id, for the product page (docs/plan.md,
 * F16). A failed mark throws the Problem Details of the response. The
 * cache is updated like after a scanned mark (markScanMutation).
 */
export function markProductMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async (productId: string) => {
      const { data, error, response } = await api.POST("/shopping-list/items", {
        body: { product_id: productId },
      });
      if (!response.ok || data === undefined) {
        throw error ?? new Error(`POST /shopping-list/items: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (result) => cacheMarkResult(queryClient, result),
  });
}

// Updates the cache after a mark: product from the result replaces the
// cached product and its entry in the product list; if the mark created
// the product, the list is fetched again. The shopping list is fetched
// again.
function cacheMarkResult(queryClient: QueryClient, { product, product_created }: MarkResult) {
  if (product_created) {
    queryClient.setQueryData(productQuery(product.id).queryKey, product);
    void queryClient.invalidateQueries({
      queryKey: productListQuery.queryKey,
      exact: true,
    });
  } else {
    setCachedProduct(queryClient, product);
  }
  invalidateShoppingList(queryClient);
}

/**
 * Removes the mark of the product with the given id for the result card
 * of the scan view, the shopping view and the product page: DELETE
 * /shopping-list/items/{product_id}. Timeout and errors are those of
 * scanMovementMutation. The response holds no product, so on success the
 * product, the product list and the shopping list are fetched again.
 */
export function unmarkMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async (productId: string) => {
      const { error, response } = await api.DELETE("/shopping-list/items/{product_id}", {
        params: { path: { product_id: productId } },
        signal: AbortSignal.timeout(scanTimeout),
      });
      if (!response.ok) {
        throw bookingError(error, response, `DELETE /shopping-list/items/${productId}`);
      }
    },
    networkMode: "always",
    onSuccess: (_data, productId) => {
      void queryClient.invalidateQueries({ queryKey: productQuery(productId).queryKey });
      void queryClient.invalidateQueries({
        queryKey: productListQuery.queryKey,
        exact: true,
      });
      invalidateShoppingList(queryClient);
    },
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
 * response replaces the cached product and its entry in the product list,
 * and the shopping list is fetched again (target, name or brand may have
 * changed).
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
    onSuccess: (product) => {
      setCachedProduct(queryClient, product);
      invalidateShoppingList(queryClient);
    },
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
 * the product is removed from the cached product list, and the shopping
 * list is fetched again.
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
      invalidateShoppingList(queryClient);
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

/** A photo for the product with productId, as picked by the user. */
export interface ProductPhoto {
  productId: string;
  photo: Blob;
}

// How long the upload of a photo may take before it counts as failed.
const uploadTimeout = 30_000;

/**
 * Replaces the image of a product with a photo: shrinks it with
 * shrinkPhoto and uploads the JPEG with PUT /products/{id}/image, with a
 * timeout of 30 s. Throws PhotoDecodeError if the browser cannot decode
 * the photo, the Problem Details of a failed upload, or the TimeoutError.
 * On success the product from the response replaces the cached product
 * and its entry in the product list.
 */
export function productImageUploadMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async ({ productId, photo }: ProductPhoto) => {
      const jpeg = await shrinkPhoto(photo);
      const { data, error, response } = await api.PUT("/products/{id}/image", {
        params: { path: { id: productId } },
        body: jpeg,
        // The image is sent as it is; the default serializer makes JSON.
        bodySerializer: (body) => body,
        headers: { "Content-Type": "image/jpeg" },
        signal: AbortSignal.timeout(uploadTimeout),
      });
      if (!response.ok || data === undefined) {
        const path = `/products/${productId}/image`;
        throw error ?? new Error(`PUT ${path}: status ${response.status}`);
      }
      return data;
    },
    onSuccess: (product) => setCachedProduct(queryClient, product),
  });
}

/**
 * Removes the image of the product with the given id with DELETE
 * /products/{id}/image. A failed removal throws the Problem Details of the
 * response. The response holds no product, so on success the cached
 * product and its entry in the product list lose their image at once and
 * are fetched again.
 */
export function productImageDeleteMutation(queryClient: QueryClient) {
  return mutationOptions({
    mutationFn: async (id: string) => {
      const { error, response } = await api.DELETE("/products/{id}/image", {
        params: { path: { id } },
      });
      if (!response.ok) {
        throw error ?? new Error(`DELETE /products/${id}/image: status ${response.status}`);
      }
      return id;
    },
    onSuccess: (id) => {
      changeCachedProduct(queryClient, id, (product) => ({ ...product, has_image: false }));
      void queryClient.invalidateQueries({ queryKey: productQuery(id).queryKey });
      void queryClient.invalidateQueries({
        queryKey: productListQuery.queryKey,
        exact: true,
      });
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
 * the product list, and the movements of the product and the shopping list
 * are fetched again.
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
      invalidateShoppingList(queryClient);
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
 * The shopping list is fetched again as well.
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
      invalidateShoppingList(queryClient);
    },
  });
}
