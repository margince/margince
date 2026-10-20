// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { unwrap } from "../../lib/problem";

// The sender's own queue behind "send later", read once and shared. The page
// and the top bar's outbox button ask the same question, so they share one
// cache entry and one request; this file stays free of any drawing so the bar
// can import it without pulling the page into the startup graph.

export type ScheduledSend = components["schemas"]["ScheduledSend"];

export type Status = ScheduledSend["status"];

/** The route this screen answers on, spelled where the queue lives. */
export const SCHEDULED_SCREEN = "scheduled" as const;

/**
 * The caller's own queue.
 *
 * No status filter on the request: the page reads in groups and one unfiltered
 * read fills all of them, where asking per group would be three requests for
 * one question.
 */
export function useScheduledSends() {
  const t = useT();
  return useQuery({
    queryKey: ["scheduled-sends"],
    queryFn: async () => {
      return unwrap(await api.GET("/scheduled-sends", {}), t);
    },
    staleTime: 60_000,
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
