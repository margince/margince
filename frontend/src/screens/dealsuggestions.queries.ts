// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { worklistKey } from "./worklist.queries";

export type DealSuggestion = components["schemas"]["DealSuggestion"];
export type AcceptDealSuggestionBody =
  components["schemas"]["AcceptDealSuggestionRequest"];
export type DealSuggestionAcceptance =
  components["schemas"]["DealSuggestionAcceptance"];

/** Which suggestions a surface asks for: one company's, or one pipeline's. */
export type SuggestionFilter = { company_id?: string; pipeline_id?: string };

export const dealSuggestionsKey = ["deal-suggestions"] as const;

// The server's page ceiling. A board or a company page shows every open
// suggestion it is given; two hundred is well past what either holds.
const SUGGESTION_PAGE = 200;

/**
 * The open Deal Scout suggestions the reader may see. The server shows a
 * suggestion only to a reader who may read every piece of its evidence, so an
 * empty answer can mean "none" or "none of yours" — both are nothing to draw.
 */
export function useDealSuggestions(filter: SuggestionFilter, enabled = true) {
  return useQuery({
    queryKey: [...dealSuggestionsKey, filter],
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/deal-suggestions", {
        params: { query: { ...filter, limit: SUGGESTION_PAGE } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
}

// A decision changes the deals the board draws, the suggestions beside them and
// the Worklist row that asked, so all three are read again.
function useDecided() {
  const queryClient = useQueryClient();
  return () => {
    queryClient.invalidateQueries({ queryKey: ["deals"] });
    queryClient.invalidateQueries({ queryKey: dealSuggestionsKey });
    queryClient.invalidateQueries({ queryKey: worklistKey });
  };
}

/**
 * Opening the suggested deal, with the reader's corrections. The key is minted
 * by the dialog when it opens and travels as a variable, so a retry after a lost
 * answer replays the first acceptance instead of being refused as a second one.
 */
export function useAcceptDealSuggestion() {
  const decided = useDecided();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      idempotencyKey: string;
      body: AcceptDealSuggestionBody;
    }): Promise<DealSuggestionAcceptance> => {
      const { data, error } = await api.POST("/deal-suggestions/{id}/accept", {
        params: {
          path: { id: input.id },
          header: { "Idempotency-Key": input.idempotencyKey },
        },
        body: input.body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: decided,
  });
}

/** Saying the suggestion is not a deal, for the whole workspace. */
export function useDismissDealSuggestion() {
  const decided = useDecided();
  return useMutation({
    mutationFn: async (input: { id: string; idempotencyKey: string }) => {
      const { error } = await api.POST("/deal-suggestions/{id}/dismiss", {
        params: {
          path: { id: input.id },
          header: { "Idempotency-Key": input.idempotencyKey },
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: decided,
  });
}
