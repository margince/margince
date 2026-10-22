// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useT } from "../i18n";
import { unwrap } from "./problem";

export type LegalReviewSummary = components["schemas"]["LegalReviewSummary"];
export type LegalReview = components["schemas"]["LegalReview"];
export type LegalReviewQueueItem =
  components["schemas"]["LegalReviewQueueItem"];
export type LegalReviewOutcome = components["schemas"]["LegalReviewOutcome"];
export type QueueStatus = "open" | "decided";

/** The route the queue and a review answer on. */
export const LEGAL_REVIEWS_SCREEN = "legal-reviews" as const;

const KEY = "legal-reviews";
const SETTINGS_KEY = [KEY, "settings"] as const;

export function useLegalReviewSettings() {
  return useQuery({
    queryKey: SETTINGS_KEY,
    queryFn: async () => unwrap(await api.GET("/legal-reviews/settings")),
  });
}

/**
 * Whether the feature is switched on. Undefined while the answer is out, so
 * no surface offers a door that would answer 404, or hides one that is there.
 */
export function useLegalReviewAvailable(): boolean | undefined {
  return useLegalReviewSettings().data?.enabled;
}

export function useUpdateLegalReviewSettings() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (enabled: boolean) =>
      unwrap(await api.PUT("/legal-reviews/settings", { body: { enabled } })),
    onSuccess: (saved) => {
      queryClient.setQueryData(SETTINGS_KEY, saved);
      queryClient.invalidateQueries({ queryKey: [KEY] });
    },
  });
}

export function useDealLegalReviews(dealId: string, enabled: boolean) {
  return useQuery({
    queryKey: [KEY, "deal", dealId],
    enabled,
    queryFn: async () =>
      unwrap(
        await api.GET("/deals/{id}/legal-reviews", {
          params: { path: { id: dealId } },
        }),
      ).data,
  });
}

export function useLegalReviewQueue(status: QueueStatus, enabled: boolean) {
  return useInfiniteQuery({
    queryKey: [KEY, "queue", status],
    enabled,
    initialPageParam: undefined as string | undefined,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/legal-reviews", {
          params: { query: { status, cursor: pageParam } },
        }),
      ),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
}

export function useLegalReview(id: string) {
  return useQuery({
    queryKey: [KEY, "one", id],
    queryFn: async () =>
      unwrap(
        await api.GET("/legal-reviews/{id}", { params: { path: { id } } }),
      ),
  });
}

/** Every legal review read depends on any write: refresh them together. */
function useRefreshReviews() {
  const queryClient = useQueryClient();
  return () => queryClient.invalidateQueries({ queryKey: [KEY] });
}

export type RequestInput = Readonly<{
  dealId: string;
  note: string;
  previousReviewId?: string;
}>;

export function useRequestLegalReview() {
  const t = useT();
  const refresh = useRefreshReviews();
  return useMutation({
    mutationFn: async (input: RequestInput) =>
      unwrap(
        await api.POST("/deals/{id}/legal-reviews", {
          params: { path: { id: input.dealId } },
          body: {
            note: input.note.trim() === "" ? null : input.note,
            previous_review_id: input.previousReviewId ?? null,
          },
        }),
        t,
      ),
    onSuccess: refresh,
  });
}

export type DecideInput = Readonly<{
  id: string;
  outcome: LegalReviewOutcome;
  comments: string;
}>;

export function useDecideLegalReview() {
  const t = useT();
  const refresh = useRefreshReviews();
  return useMutation({
    mutationFn: async (input: DecideInput) =>
      unwrap(
        await api.POST("/legal-reviews/{id}/decide", {
          params: { path: { id: input.id } },
          body: {
            outcome: input.outcome,
            comments: input.comments.trim() === "" ? null : input.comments,
          },
        }),
        t,
      ),
    onSuccess: refresh,
  });
}

export function useWithdrawLegalReview() {
  const t = useT();
  const refresh = useRefreshReviews();
  return useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.POST("/legal-reviews/{id}/withdraw", {
          params: { path: { id } },
        }),
        t,
      ),
    onSuccess: refresh,
  });
}
