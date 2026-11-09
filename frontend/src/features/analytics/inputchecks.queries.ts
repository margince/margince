import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { unwrap } from "../../lib/problem";
import { useUrlParams } from "../../lib/urlstate";
import { type AnalyticsScope, scopeQuery } from "./analytics.context";
import type { Dimensions } from "./reporting.dimensions";

export function useInputChecks(
  enabled = true,
  scope?: AnalyticsScope,
  dimensions?: Dimensions,
) {
  const [params] = useUrlParams();
  const query = {
    ...(scope ? scopeQuery(scope) : {}),
    product_id: dimensions?.product_id ?? params.get("product") ?? undefined,
    country: dimensions?.country ?? params.get("country") ?? undefined,
    industry: dimensions?.industry ?? params.get("industry") ?? undefined,
  };
  return useQuery({
    enabled,
    queryKey: ["input-checks", query],
    // The signal stops the read of a scope the reader has already left.
    queryFn: async ({ signal }) => {
      const data = unwrap(
        await api.GET("/forecast/assurance/exceptions", {
          params: { query },
          signal,
        }),
      );
      return data;
    },
  });
}
