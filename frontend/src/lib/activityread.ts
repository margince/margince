// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  queryOptions,
  skipToken,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useEffect } from "react";
import { api } from "../api/client";
import { useT } from "../i18n";
import { ProblemError, problemCode, unwrap } from "./problem";

// Cards remember a refused read without keeping the activity's protected body
// in the cache after its drawer closes.
export function activityUnavailableQuery(activityId: string | undefined) {
  return queryOptions<boolean>({
    queryKey: ["activity-unavailable", activityId],
    queryFn: skipToken,
    enabled: false,
    gcTime: Number.POSITIVE_INFINITY,
  });
}

export function useActivityUnavailable(
  activityId: string | undefined,
): boolean {
  return useQuery(activityUnavailableQuery(activityId)).data === true;
}

export function useReadableActivityList(ids: readonly string[]) {
  const client = useQueryClient();
  useEffect(() => {
    markActivitiesReadable(client, ids);
  }, [client, ids]);
}

export function markActivitiesReadable(
  client: QueryClient,
  ids: readonly string[],
) {
  for (const id of ids) {
    client.setQueryData(activityUnavailableQuery(id).queryKey, false);
  }
}

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
  const client = useQueryClient();
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
  useEffect(() => {
    if (!reading || query.isPending || query.isFetching) return;
    if (query.isError) {
      const code =
        query.error instanceof ProblemError
          ? problemCode(query.error.problem)
          : undefined;
      if (code !== "not_found" && code !== "permission_denied") return;
    }
    client.setQueryData(
      activityUnavailableQuery(activityId).queryKey,
      query.isError || query.data?.content_state === "withheld",
    );
  }, [
    activityId,
    client,
    reading,
    query.isPending,
    query.isFetching,
    query.isError,
    query.error,
    query.data,
  ]);
  return {
    data: reading && !query.isError ? query.data : undefined,
    error: query.error,
    isError: query.isError,
    isPending: query.isPending,
    refetch: query.refetch,
  };
}
