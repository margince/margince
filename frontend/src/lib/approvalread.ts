// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { unwrap } from "./problem";

/**
 * One approval, under the key a write to it invalidates.
 *
 * Four surfaces show the same approval: the worklist, the record page, a
 * transcript and the setup repair. They used to ask under keys of their own.
 * So two open together paid twice, and invalidating one left the other stale.
 */
export function approvalQueryKey(approvalId: string) {
  return ["approval", approvalId] as const;
}

export function useApproval(approvalId: string, enabled = true) {
  return useQuery({
    queryKey: approvalQueryKey(approvalId),
    enabled: enabled && Boolean(approvalId),
    queryFn: async () => {
      return unwrap(
        await api.GET("/approvals/{id}", {
          params: { path: { id: approvalId } },
        }),
      );
    },
  });
}
