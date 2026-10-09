// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery, useMutation } from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../api/client";
import { unwrap } from "./common";
import { NOTICE_STATES, UNRESOLVED_NOTICE_STATES } from "./noticecases.logic";
import type { ExcuseState } from "./noticeexcuse";

// Both facets name their states: the server reads an absent filter as every
// unresolved one, which would make "All" return the "Still owed" rows.
export const NOTICE_FACETS = ["owed", "all"] as const;
export type NoticeFacet = (typeof NOTICE_FACETS)[number];

// The facet is a server-side filter, so the count on screen is the queue's own.
export function useNoticeQueue(facet: NoticeFacet, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: ["notice-cases", facet],
    enabled,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/privacy/notice-cases", {
          params: {
            query: {
              limit: 50,
              cursor: pageParam ?? undefined,
              state:
                facet === "owed"
                  ? [...UNRESOLVED_NOTICE_STATES]
                  : [...NOTICE_STATES],
            },
          },
        }),
      );
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
}

export function useAssignDuty(onDone: () => void) {
  return useMutation({
    mutationFn: async (vars: { id: string; owner: string }) => {
      return unwrap(
        await api.POST("/privacy/notice-cases/{id}/assign", {
          params: { path: { id: vars.id } },
          body: { owner_user_id: vars.owner },
        }),
      );
    },
    onSuccess: onDone,
  });
}

export function useExcuseDuty(onDone: () => void) {
  return useMutation({
    mutationFn: async (vars: {
      id: string;
      state: ExcuseState;
      note: string;
    }) => {
      return unwrap(
        await api.POST("/privacy/notice-cases/{id}/excuse", {
          params: { path: { id: vars.id } },
          body: { state: vars.state, resolution_note: vars.note },
        }),
      );
    },
    onSuccess: onDone,
  });
}
