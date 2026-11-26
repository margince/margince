// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { isTimelineRead, TASK_QUEUE_KEY } from "../../lib/activitykeys";
import { unwrap } from "../../lib/problem";
import { recordWriteKeys } from "../../lib/recordwritekeys";
import { worklistKey } from "../../lib/workkeys";

type DealDecline = components["schemas"]["ActivityDealDecline"];

/** What a decline names: the message, and the deal it is not about. No deal
 *  answers "none of them" to the message's open deal question. */
export type DealDeclineInput = { activityId: string; dealId?: string };

/**
 * The colleague's "not about this deal". The server takes the link off and
 * keeps every automatic pass from filing the message there again. Every
 * timeline, the task lists and the message itself are read again, since the
 * message leaves a deal and a question may close.
 */
export function useDealDecline(
  onDeclined?: (answer: DealDecline) => Promise<unknown>,
  mutationKey?: readonly unknown[],
) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey,
    mutationFn: async (input: DealDeclineInput) =>
      unwrap(
        await api.POST("/activities/{id}/deal-decline", {
          params: { path: { id: input.activityId } },
          body: input.dealId ? { deal_id: input.dealId } : {},
        }),
        t,
      ),
    onSuccess: async (answer, input) => {
      await Promise.all([
        queryClient.invalidateQueries({
          predicate: (query) => isTimelineRead(query.queryKey),
        }),
        queryClient.invalidateQueries({ queryKey: TASK_QUEUE_KEY }),
        queryClient.invalidateQueries({ queryKey: worklistKey }),
        queryClient.invalidateQueries({
          queryKey: ["activity", input.activityId],
        }),
      ]);
      if (answer) await onDeclined?.(answer);
    },
  });
}

type DealRelevance = components["schemas"]["ActivityDealRelevance"];

/** A colleague's mark on one activity's link to one deal. */
export type DealRelevanceInput = {
  activityId: string;
  dealId: string;
  relevance: "relevant" | "unrelated";
};

/**
 * Says whether an activity is about a deal it is linked to. The link stays and
 * carries the mark, so the deal's timeline still shows the row; the same call
 * with the other mark is the undo. Every timeline and the activity are read
 * again, since the deal's activity date moves with the mark.
 */
export function useDealRelevance(
  onMarked?: (answer: DealRelevance, input: DealRelevanceInput) => void,
) {
  const t = useT();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: DealRelevanceInput) =>
      unwrap(
        await api.PUT("/activities/{id}/deal-relevance", {
          params: { path: { id: input.activityId } },
          body: { deal_id: input.dealId, deal_relevance: input.relevance },
        }),
        t,
      ),
    onSuccess: async (answer, input) => {
      await Promise.all([
        queryClient.invalidateQueries({
          predicate: (query) => isTimelineRead(query.queryKey),
        }),
        queryClient.invalidateQueries({
          queryKey: ["activity", input.activityId],
        }),
        ...recordWriteKeys("deal", input.dealId).map((queryKey) =>
          queryClient.invalidateQueries({ queryKey }),
        ),
      ]);
      if (answer) onMarked?.(answer, input);
    },
  });
}
