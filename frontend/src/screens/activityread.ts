// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { useT } from "../i18n";
import { unwrap } from "./common";

/**
 * One activity, read whole, under the key a write to it invalidates. Nothing is
 * kept once no reader holds it, and nothing is trusted for longer than a render:
 * the readers sharing this key include ones whose access can be revoked while
 * they are open, and an old answer must not outlive that.
 *
 * So `data` is the current read's answer and never the cache's. A refused read
 * leaves its last answer cached, and an observer outliving one reader keeps
 * `gcTime: 0` from evicting it; a read switched off keeps it too.
 */
export function useActivity(activityId: string | undefined, enabled = true) {
  const t = useT();
  const reading = enabled && Boolean(activityId);
  const query = useQuery({
    queryKey: ["activity", activityId],
    staleTime: 0,
    gcTime: 0,
    enabled: reading,
    queryFn: async () => {
      return unwrap(
        await api.GET("/activities/{id}", {
          params: { path: { id: activityId ?? "" } },
        }),
        t,
      );
    },
  });
  return {
    data: reading && !query.isError ? query.data : undefined,
    error: query.error,
    isError: query.isError,
    isPending: query.isPending,
    refetch: query.refetch,
  };
}
