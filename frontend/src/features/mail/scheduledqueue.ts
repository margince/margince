// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

// The sender's own queue behind "send later", read once and shared.
// The page and the top bar's outbox button share one cache entry and one request.
// This file draws nothing, so the bar can import it without loading the page.

export type ScheduledSend = components["schemas"]["ScheduledSend"];

export type Status = ScheduledSend["status"];

/** The route this screen answers on, spelled where the queue lives. */
export const SCHEDULED_SCREEN = "scheduled" as const;

/**
 * The caller's own queue.
 *
 * No status filter on the request. One unfiltered read fills every group the
 * page shows, where asking per group would be three requests for one question.
 */
export function useScheduledSends() {
  const t = useT();
  return useQuery({
    queryKey: ["scheduled-sends"],
    queryFn: async () => {
      return unwrap(await api.GET("/scheduled-sends", {}), t);
    },
    staleTime: 60_000,
    // A message can be held while the rep is on another page.
    refetchInterval: 60_000,
  });
}

/**
 * One message as it stands now, files resolved, for an edit to start from.
 *
 * Dropped from the cache once nobody reads it, so each opening asks again.
 */
export function useScheduledSend(id: string | undefined) {
  const t = useT();
  return useQuery({
    queryKey: ["scheduled-sends", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/scheduled-sends/{id}", {
          params: { path: { id: id ?? "" } },
        }),
        t,
      );
    },
    enabled: id !== undefined,
    gcTime: 0,
  });
}

/**
 * What is still going to happen to a message. `waiting` is the queue proper and
 * `held` is the only status that waits on the sender; every other status has
 * already settled.
 */
export function queueCounts(sends: readonly ScheduledSend[]): {
  waiting: number;
  held: number;
} {
  return {
    waiting: sends.filter((send) => send.status === "scheduled").length,
    held: sends.filter((send) => send.status === "held").length,
  };
}
