import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

export function useAutomationPreview({
  id,
  version,
  params,
  windowDays = 30,
}: Readonly<{
  id: string;
  version?: number;
  params?: Record<string, unknown>;
  windowDays?: number;
}>) {
  return useQuery({
    queryKey: ["automation-preview", id, version, params, windowDays],
    queryFn: async (): Promise<components["schemas"]["AutomationPreview"]> =>
      unwrap(
        await api.POST("/automations/{id}/preview", {
          params: { path: { id } },
          body: { params, window_days: windowDays },
        }),
      ),
  });
}
