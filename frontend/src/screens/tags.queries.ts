// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { throwProblem } from "./common";
import { RECORD_LIST_KEY } from "./recordlistkeys";

// The reads and writes behind the record page's tag panel.

export type RecordTag = components["schemas"]["RecordTag"];
export type Tag = components["schemas"]["Tag"];

/** The record types the tags panel serves. */
export type TaggableType = "contact" | "company" | "deal" | "lead";

/**
 * The panel, every list drawing this record's chips, and the tag page and its
 * counts all go stale together: each shows who carries the word.
 */
function invalidateTagged(
  queryClient: ReturnType<typeof useQueryClient>,
  entityType: TaggableType,
  entityID: string,
) {
  return Promise.all([
    queryClient.invalidateQueries({
      queryKey: ["record-tags", entityType, entityID],
    }),
    queryClient.invalidateQueries({
      queryKey: [RECORD_LIST_KEY[entityType]],
    }),
    queryClient.invalidateQueries({ queryKey: ["tag"] }),
    queryClient.invalidateQueries({ queryKey: ["tag-records"] }),
  ]);
}

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
export function useTagVocabulary(enabled = true) {
  return useQuery({
    queryKey: ["tags", "vocabulary"],
    enabled,
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
      void invalidateTagged(queryClient, entityType, entityID);
    },
  });
}

export type RemovalUndo = components["schemas"]["RemovalUndo"];

// On the hook, not on `mutate`: an Undo runs after its pill has unmounted, and
// React Query drops a `mutate` call's own callbacks once its observer is gone.
type Outcome<V> = Readonly<{
  onSuccess: (variables: V) => void;
  onError: (error: Error) => void;
}>;

/**
 * Take one tag off one record, leaving the tag itself alone. `onSuccess` gets
 * the handle that puts it back, or null when the record did not carry the tag.
 */
export function useRemoveTag(
  entityType: TaggableType,
  entityID: string,
  outcome: Outcome<TagRestore | null>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (tagID: string): Promise<TagRestore | null> => {
      const { data, error } = await api.DELETE("/tags/{id}/apply", {
        params: { path: { id: tagID } },
        body: { entity_type: entityType, entity_id: entityID },
      });
      if (error) {
        throwProblem(error);
      }
      return data ? { tagID, undo: data } : null;
    },
    onError: outcome.onError,
    onSuccess: async (restore) => {
      await invalidateTagged(queryClient, entityType, entityID);
      outcome.onSuccess(restore);
    },
  });
}

export type TagRestore = Readonly<{ tagID: string; undo: RemovalUndo }>;

/** Put back a tagging this reader removed, as it was assigned. */
export function useRestoreTag(
  entityType: TaggableType,
  entityID: string,
  outcome: Outcome<TagRestore>,
) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: TagRestore) => {
      const { error } = await api.POST("/tags/{id}/apply/restore", {
        params: { path: { id: input.tagID } },
        body: input.undo,
      });
      if (error) {
        throwProblem(error);
      }
    },
    onError: outcome.onError,
    onSuccess: async (_, input) => {
      await invalidateTagged(queryClient, entityType, entityID);
      outcome.onSuccess(input);
    },
  });
}
