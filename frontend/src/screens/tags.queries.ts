// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";

// The reads and writes behind the record page's tag panel.

export type RecordTag = components["schemas"]["RecordTag"];
export type Tag = components["schemas"]["Tag"];

/** The record types the tags panel serves. */
export type TaggableType = "person" | "organization" | "deal";

/**
 * The tags on one record, and whether the vocabulary was withheld.
 *
 * `withheld` is NOT the same as an empty list, and the panel draws them
 * differently: a caller who may read the record but not the vocabulary is told
 * the words are hidden, because "no tags" is a claim about the record that
 * nobody established.
 */
export function useRecordTags(entityType: TaggableType, entityID: string) {
  return useQuery({
    queryKey: ["record-tags", entityType, entityID],
    queryFn: async () => {
      const { data, error } = await api.GET(
        "/records/{entity_type}/{entity_id}/tags",
        { params: { path: { entity_type: entityType, entity_id: entityID } } },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    staleTime: 30_000,
  });
}

/**
 * The workspace's live tags, for the add-tag picker.
 *
 * Live only: the picker offers what can be applied, and a retired word cannot.
 * It stays on a record that already carries it, which is the panel's business
 * rather than this list's.
 */
export function useTagVocabulary() {
  return useQuery({
    queryKey: ["tags", "vocabulary"],
    // The catalog is capped and has no cursor, so a workspace past the cap gets
    // a CUT list. Carrying `has_more` through is what lets the picker say the
    // list is short — without it a word beyond the cap is indistinguishable
    // from a word that does not exist, and the reader coins a near-duplicate.
    queryFn: async (): Promise<{ tags: Tag[]; truncated: boolean }> => {
      const { data, error } = await api.GET("/tags", {});
      if (error) {
        throwProblem(error);
      }
      return { tags: data.data, truncated: data.page.has_more };
    },
    staleTime: 5 * 60_000,
  });
}

/** Apply one existing tag to one record. */
export function useApplyTag(entityType: TaggableType, entityID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (tagID: string) => {
      const { error } = await api.POST("/tags/{id}/apply", {
        params: { path: { id: tagID } },
        body: { entity_type: entityType, entity_id: entityID },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      // The panel and any list showing this record's chips both go stale.
      void queryClient.invalidateQueries({
        queryKey: ["record-tags", entityType, entityID],
      });
    },
  });
}

/** Take one tag off one record, leaving the tag itself alone. */
export function useRemoveTag(entityType: TaggableType, entityID: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (tagID: string) => {
      const { error } = await api.DELETE("/tags/{id}/apply", {
        params: { path: { id: tagID } },
        body: { entity_type: entityType, entity_id: entityID },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["record-tags", entityType, entityID],
      });
    },
  });
}
