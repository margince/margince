// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { useT } from "../i18n";
import { throwProblem } from "./common";

/**
 * One activity, read whole, under the key a write to it invalidates. Nothing is
 * kept once no reader holds it, and nothing is trusted for longer than a render:
 * the readers sharing this key include ones whose access can be revoked while
 * they are open, and an old answer must not outlive that.
 */
export function useActivity(activityId: string | undefined, enabled = true) {
  const t = useT();
  return useQuery({
    queryKey: ["activity", activityId],
    staleTime: 0,
    gcTime: 0,
    enabled: enabled && Boolean(activityId),
    queryFn: async () => {
      const { data, error } = await api.GET("/activities/{id}", {
        params: { path: { id: activityId ?? "" } },
      });
      if (error) {
        throwProblem(error, t);
      }
      return data;
    },
  });
}
