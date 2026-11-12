// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { isTimelineRead, TASK_QUEUE_KEY } from "./activitykeys";
import { unwrap } from "./problem";
import { worklistKey } from "./workkeys";

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
