import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { ANALYTICS_SCHEMA_KEY } from "../../lib/analyticskeys";
import { unwrap } from "../../lib/problem";

/** The analytics vocabulary this seat may ask in, shared by every reader of it. */
export function useAnalyticsSchema() {
  return useQuery({
    queryKey: ANALYTICS_SCHEMA_KEY,
    queryFn: async () => {
      return unwrap(await api.GET("/analytics/schema", {}));
    },
  });
}
