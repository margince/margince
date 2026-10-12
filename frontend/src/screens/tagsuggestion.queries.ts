// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { worklistKey } from "./worklist.queries";

export type TagSuggestion = components["schemas"]["TagSuggestion"];

const tagSuggestionKey = ["tag-suggestion"] as const;

/**
 * One open tag suggestion with the mail and notes it cites. The server answers
 * 404 for one that was decided or that the reader may not see. The card draws
 * both the same way, as nothing left to decide.
 */
export function useTagSuggestion(id: string) {
  return useQuery({
    queryKey: [...tagSuggestionKey, id],
    queryFn: async (): Promise<TagSuggestion | null> => {
      const { data, error, response } = await api.GET("/tag-suggestions/{id}", {
        params: { path: { id } },
      });
      if (error) {
        if (response.status === 404) {
          return null;
        }
        return throwProblem(error);
      }
      return data;
    },
  });
}

/** The Worklist row that asked goes once the reader is done with it. */
export function useTagSuggestionSettled() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: worklistKey });
    void queryClient.invalidateQueries({ queryKey: tagSuggestionKey });
  };
}

/**
 * Accept or dismiss. Accepting refreshes the record's tags and leaves the
 * Worklist row in place. The card then offers the tag to the company or its
 * contacts before the row goes.
 */
export function useDecideTagSuggestion() {
  const queryClient = useQueryClient();
  const settled = useTagSuggestionSettled();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      verb: "accept" | "dismiss";
    }): Promise<TagSuggestion> => {
      const { data, error } =
        input.verb === "accept"
          ? await api.POST("/tag-suggestions/{id}/accept", {
              params: { path: { id: input.id } },
            })
          : await api.POST("/tag-suggestions/{id}/dismiss", {
              params: { path: { id: input.id } },
            });
      if (error) {
        return throwProblem(error);
      }
      return data;
    },
    onSuccess: (suggestion, input) => {
      void queryClient.invalidateQueries({
        queryKey: ["record-tags", suggestion.entity_type, suggestion.entity_id],
      });
      if (input.verb === "dismiss") {
        settled();
      }
    },
  });
}

/** A contact's current company, for the offer that follows an acceptance. */
export function useContactEmployer(contactId: string | undefined) {
  return useQuery({
    queryKey: ["contact", contactId, "employer"],
    enabled: contactId !== undefined,
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}", {
        params: { path: { id: contactId ?? "" } },
      });
      if (error) {
        return throwProblem(error);
      }
      return data.employer ?? null;
    },
  });
}
