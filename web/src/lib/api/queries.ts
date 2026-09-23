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
 * product from the response. On stock_already_zero the cached list is out
 * of date (someone else took the last one), so it is fetched again.
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
    onSuccess: (result) => {
      queryClient.setQueryData(productListQuery.queryKey, (products) =>
        replaceProduct(products, result.product),
      );
    },
    onError: (error) => {
      if (problemCode(error) === "stock_already_zero") {
        void queryClient.invalidateQueries({ queryKey: productListQuery.queryKey });
      }
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
    onSuccess: (product) => {
      queryClient.setQueryData(productQuery(product.id).queryKey, product);
      queryClient.setQueryData(productListQuery.queryKey, (products) =>
        replaceProduct(products, product),
      );
    },
  });
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
