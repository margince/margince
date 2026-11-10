// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./problem";

/**
 * One approval, under the key a write to it invalidates.
 *
 * The worklist, the record page, a transcript and the setup repair all show
 * one approval. Read under one key, two of them open together pay once, and a
 * write that invalidates it refreshes every one.
 */
export function approvalQueryKey(approvalId: string) {
  return ["approval", approvalId] as const;
}

// The read itself, for a surface that asks for several at once.
export function approvalQuery(approvalId: string) {
  return {
    queryKey: approvalQueryKey(approvalId),
    queryFn: async () =>
      unwrap(
        await api.GET("/approvals/{id}", {
          params: { path: { id: approvalId } },
        }),
      ),
  };
}

export function useApproval(approvalId: string, enabled = true) {
  return useQuery({
    ...approvalQuery(approvalId),
    enabled: enabled && Boolean(approvalId),
  });
}
