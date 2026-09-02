// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { throwProblem } from "./common";

export type Tag = components["schemas"]["Tag"];
/** The palette, from the contract rather than restated beside it. */
export type TagColor = NonNullable<Tag["color"]>;
/**
 * What an UPDATE may set a colour to.
 *
 * Wider than the read's by one word: clearing is spelled `"none"` rather than
 * null, because an absent field and a null field decode to the same thing in
 * the generated request type — a contract promising the two differ would be
 * promising what no server can honour.
 */
export type TagColorEdit = NonNullable<
  components["schemas"]["UpdateTagRequest"]["color"]
>;
export type TagDetail = components["schemas"]["TagDetail"];

/**
 * The whole vocabulary, ARCHIVED INCLUDED, for the admin card.
 *
 * The picker asks for live words only, because a retired one cannot be
 * applied. This card is where a retired word is restored, so a list that hid
 * them would leave the verb with nothing to act on — and an admin looking for
 * a word they archived last month would conclude it was deleted.
 */
export function useTagCatalog(enabled = true) {
  return useQuery({
    queryKey: ["tags", "catalog"],
    // Not asked at all without the grant: the settings entry opens on any of
    // five data-model reads, so this card is mounted for seats that hold none
    // of the tag ones, and the request could only answer 403.
    enabled,
    queryFn: async () => {
      const { data, error } = await api.GET("/tags", {
        params: { query: { include_archived: true } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * One tag with how much of the workspace carries it.
 *
 * Asked per tag rather than folded into the catalog: the counts are row-scoped
 * per record type, which is three joins the list read does not do for words
 * nobody has opened.
 */
export function useTagDetail(tagID: string | undefined) {
  return useQuery({
    queryKey: ["tag", tagID],
    enabled: Boolean(tagID),
    queryFn: async () => {
      const { data, error } = await api.GET("/tags/{id}", {
        params: { path: { id: tagID as string } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/**
 * Every write on this card refreshes the same three reads.
 *
 * The catalog is what the card draws. The vocabulary is what the pickers and
 * the filter dials offer, and a word coined here has to reach them or an admin
 * adds a tag and then cannot find it. The record reads carry the words
 * themselves, so a rename that did not reach them would leave the old spelling
 * on every open record page.
 */
function useVocabularyInvalidation() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: ["tags"] });
    void queryClient.invalidateQueries({ queryKey: ["tag"] });
    void queryClient.invalidateQueries({ queryKey: ["record-tags"] });
    // The three record LISTS carry their rows' tags inline, keyed under their
    // own prefixes rather than under any tag key. Without these a rename or a
    // merge leaves the old spelling on every row a reader has already loaded,
    // for as long as that page stays fresh — and this client disables refetch
    // on focus, so coming back to the tab does not repair it.
    for (const list of TAGGED_LISTS) {
      void queryClient.invalidateQueries({ queryKey: [list] });
    }
  };
}

/** The list reads whose rows carry tags inline. */
const TAGGED_LISTS = ["people", "organizations", "deals"] as const;

export function useCreateTag() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (body: { name: string; color?: TagColor }) => {
      const { data, error } = await api.POST("/tags", { body });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

/**
 * A rename or a recolour, pinned to the version the card read.
 *
 * If-Match, not last-write-wins: two admins tidying the vocabulary in the same
 * minute would otherwise silently overwrite one another, and a tag is a shared
 * word — the loser's spelling is what every record then carries.
 */
export function useUpdateTag() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (input: {
      id: string;
      /** The row's own, straight off the read. Undefined is a REFUSAL rather
       *  than a default — see the mutationFn. */
      version: number | undefined;
      name?: string;
      color?: TagColorEdit;
      description?: string;
    }) => {
      const { id, version, ...body } = input;
      // Inside the mutation, not at the call site: a throw here becomes this
      // mutation's `error` and lands on the dialog's own error line, while the
      // same throw in a click handler escapes into React and takes the page
      // down. The refusal itself is not optional — an unpinned PATCH is
      // last-write-wins, landing on top of an edit it never saw and reporting
      // success to both editors.
      const { data, error } = await api.PATCH("/tags/{id}", {
        params: { path: { id }, ...ifMatch(requireVersion(version)) },
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}

/** Retire a word: it stops being offered, and stays on what already carries it. */
export function useArchiveTag() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/tags/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
}

export function useRestoreTag() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.POST("/tags/{id}/restore", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
}

/** Fold one word into another. The source is retired; the target survives. */
export function useMergeTags() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (input: { id: string; intoTagID: string }) => {
      const { data, error } = await api.POST("/tags/{id}/merge", {
        params: { path: { id: input.id } },
        body: { into_tag_id: input.intoTagID },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
}
