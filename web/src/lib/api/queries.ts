import {
  mutationOptions,
  queryOptions,
  type QueryClient,
} from "@tanstack/react-query";
import { replaceProduct, sortByName } from "../products";
import { api } from "./client";

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
 * product from the response.
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
  });
}
