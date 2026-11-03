// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "../lib/problem";

/** The catalog's products whose name matches q, for a picker. */
export async function searchProductCandidates(
  q: string,
): Promise<{ id: string; name: string }[]> {
  const data = unwrap(
    await api.GET("/products", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((p) => ({ id: p.id, name: p.name }));
}

/**
 * The name of the product a record names by id. A record keeps only the id,
 * and every surface that draws it reads the name the same way.
 */
export function useProductName(id: string | null | undefined) {
  return useQuery({
    queryKey: ["product", id],
    enabled: Boolean(id),
    queryFn: async () =>
      unwrap(
        await api.GET("/products/{id}", { params: { path: { id: id ?? "" } } }),
      ).name,
  });
}
