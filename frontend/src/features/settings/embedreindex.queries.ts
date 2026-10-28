// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { available } from "../../lib/settingscapability";
import { holds } from "../../lib/settingscatalog";

// The embedding-reindex reads: the settings card's status (with the backlog
// count) and the app-shell banner's identities (without it).

export type ReindexStatus = components["schemas"]["EmbedReindexStatus"];
export type ReindexIdentities = components["schemas"]["EmbedReindexIdentities"];

// The settings card's status read. A successful confirm's setQueryData (below)
// seeds it and the banner's identities from the one write.
export const embedReindexStatusQueryKey = ["embed-reindex-status"];
// The app-shell banner's read: the two identities alone, with no backlog scan.
export const embedReindexIdentitiesQueryKey = ["embed-reindex-identities"];

// Whether this installation has an embed lane bound. The status read answers
// 501 without one, so both surfaces ask this before issuing it.
export function useEmbedReindexAvailable(): boolean {
  return holds(available("embedding_reindex"), useMe().data);
}

export function useEmbedReindexStatus(enabled: boolean, staleTime?: number) {
  return useQuery({
    queryKey: embedReindexStatusQueryKey,
    enabled,
    staleTime,
    queryFn: async (): Promise<ReindexStatus> => {
      const data = unwrap(await api.GET("/embeddings/reindex/status"));
      if (!data) {
        throw new Error("malformed reindex status response");
      }
      return data;
    },
  });
}

export function useEmbedReindexIdentities(
  enabled: boolean,
  staleTime?: number,
) {
  return useQuery({
    queryKey: embedReindexIdentitiesQueryKey,
    enabled,
    staleTime,
    queryFn: async (): Promise<ReindexIdentities> => {
      const data = unwrap(await api.GET("/embeddings/reindex/identities"));
      if (!data) {
        throw new Error("malformed reindex identities response");
      }
      return data;
    },
  });
}
