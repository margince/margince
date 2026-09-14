import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The outcome review: what a closed deal recorded about how it went.
//
// A review hangs off a CLOSING, not off the deal. A deal can be closed,
// reopened and closed again, and each closing is its own outcome to review — so
// every read here carries the closing it belongs to, and the panel compares it
// against the deal's current one to tell a review of THIS outcome from a review
// of an earlier one.

export type OutcomeReview = components["schemas"]["OutcomeReview"];
export type ReviewTemplate = components["schemas"]["ActivityReviewTemplate"];
export type ReviewQuestion = components["schemas"]["ReviewQuestion"];

export const REVIEW_TEMPLATES_KEY = ["activity-review-templates"] as const;

export function outcomeReviewsKey(dealId: string) {
  return ["deals", dealId, "outcome-reviews"] as const;
}

export function useOutcomeReviews(dealId: string, enabled: boolean) {
  return useQuery({
    // `enabled` rather than a conditional call: an OPEN deal has no closing to
    // review, so asking would spend a request to be told nothing. The hook
    // still runs, which is what keeps the rules-of-hooks contract.
    enabled,
    queryKey: outcomeReviewsKey(dealId),
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/deals/{id}/outcome-reviews",
        { params: { path: { id: dealId } } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

export function useReviewTemplates() {
  return useQuery({
    queryKey: REVIEW_TEMPLATES_KEY,
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/activity-review-templates",
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

/**
 * The template to ask at this closing: the active one for the outcome the deal
 * actually recorded.
 *
 * A win review and a loss review ask different questions, and offering both is
 * asking somebody to file what the record already knows. Retired templates are
 * excluded — an old review keeps its own frozen questions, so nothing is lost
 * by not offering a retired template for a NEW one.
 */
export function templateForOutcome(
  templates: ReviewTemplate[] | undefined,
  outcome: "won" | "lost",
): ReviewTemplate | undefined {
  return templates?.find((tpl) => tpl.active && tpl.outcome === outcome);
}

export function useCreateOutcomeReview(dealId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (
      body: components["schemas"]["CreateOutcomeReviewRequest"],
    ) => {
      const { data, error, response } = await api.POST(
        "/deals/{id}/outcome-reviews",
        { params: { path: { id: dealId } }, body },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
    // The review is written as a note on the deal's timeline, so the timeline
    // and the deal itself are as stale as this list was.
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: outcomeReviewsKey(dealId) });
      void qc.invalidateQueries({ queryKey: ["activities"] });
    },
  });
}
