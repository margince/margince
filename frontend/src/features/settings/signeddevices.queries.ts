// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

export type MySession = components["schemas"]["MySession"];

export const MY_SESSIONS_KEY = ["me", "sessions"] as const;

/** The sessions open under the reader's account, newest activity first. */
export function useMySessions() {
  return useQuery({
    queryKey: MY_SESSIONS_KEY,
    queryFn: async (): Promise<readonly MySession[]> =>
      unwrap(await api.GET("/me/sessions")).sessions,
  });
}

/**
 * Ends the sessions named, trying every one even when another fails. It answers
 * how many could not be ended, and throws only when none could. A session that
 * is already gone answers 204 too, so a stale list cannot fail.
 */
export function useEndSessions() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (sessionIds: readonly string[]): Promise<number> => {
      const results = await Promise.allSettled(
        sessionIds.map(async (sessionId) =>
          unwrap(
            await api.DELETE("/me/sessions/{sessionId}", {
              params: { path: { sessionId } },
            }),
          ),
        ),
      );
      const failures = results.filter((result) => result.status === "rejected");
      if (failures.length > 0 && failures.length === results.length)
        throw failures[0]?.reason;
      return failures.length;
    },
    onSettled: () => client.invalidateQueries({ queryKey: MY_SESSIONS_KEY }),
  });
}
