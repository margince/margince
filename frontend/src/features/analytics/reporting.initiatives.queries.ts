import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

export type SalesInitiative = components["schemas"]["SalesInitiative"];

export const INITIATIVES_KEY = "sales-initiatives";

/** The initiatives a reader may open, drafts and live ones, newest first. */
export function useSalesInitiatives(enabled = true) {
  return useQuery({
    queryKey: [INITIATIVES_KEY],
    queryFn: async () => unwrap(await api.GET("/analytics/initiatives")).data,
    enabled,
    staleTime: 30_000,
  });
}
