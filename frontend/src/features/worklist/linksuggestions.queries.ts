// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { throwProblem } from "../../lib/problem";

export type LinkSuggestion = components["schemas"]["LinkSuggestion"];

const linkSuggestionsKey = ["link-suggestions"] as const;

/** The pending suggestions an extension made, oldest first. */
export function useLinkSuggestions() {
  return useQuery({
    queryKey: linkSuggestionsKey,
    queryFn: async (): Promise<LinkSuggestion[]> => {
      const { data, error } = await api.GET("/link-suggestions");
      if (error) {
        return throwProblem(error);
      }
      return data.data;
    },
  });
}

/**
 * Accept files the message under the suggested record, so the message's own
 * views refresh with the queue. Reject changes nothing but the queue.
 */
export function useDecideLinkSuggestion() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      verb: "accept" | "reject";
    }): Promise<LinkSuggestion> => {
      const params = { params: { path: { id: input.id } } };
      const { data, error } =
        input.verb === "accept"
          ? await api.POST("/link-suggestions/{id}/accept", params)
          : await api.POST("/link-suggestions/{id}/reject", params);
      if (error) {
        return throwProblem(error);
      }
      return data;
    },
    onSuccess: (_suggestion, input) => {
      void queryClient.invalidateQueries({ queryKey: linkSuggestionsKey });
      if (input.verb === "accept") {
        void queryClient.invalidateQueries({ queryKey: ["activities"] });
      }
    },
  });
}
