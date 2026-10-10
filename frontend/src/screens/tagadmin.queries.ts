// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch, requireVersion } from "../api/version";
import { unwrap } from "./common";
import type { MutationOutcome } from "./undoableremoval";

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

/** The whole vocabulary, archived words included, so the filter sentence can
 *  tell a retired tag in a clause from a live one. */
export function useTagCatalog(enabled = true) {
  return useQuery({
    queryKey: ["tags", "catalog"],
    enabled,
    queryFn: async () => {
      return unwrap(
        await api.GET("/tags", {
          params: { query: { include_archived: true } },
        }),
      );
    },
  });
}

/** The catalog with each word's record count, for the admin card alone:
 *  counting reads every tagging, which no list page should pay for. */
export function useCountedTagCatalog(enabled: boolean) {
  return useQuery({
    queryKey: ["tags", "catalog", "carried_by"],
    enabled,
    queryFn: async () => {
      return unwrap(
        await api.GET("/tags", {
          params: { query: { include_archived: true, with_carried_by: true } },
        }),
      );
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
const TAGGED_LISTS = ["contacts", "companies", "deals"] as const;

export function useCreateTag() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (body: { name: string; color?: TagColor }) => {
      return unwrap(await api.POST("/tags", { body }));
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
      suggestible?: boolean;
    }) => {
      const { id, version, ...body } = input;
      // Inside the mutation, not at the call site: a throw here becomes this
      // mutation's `error` and lands on the dialog's own error line, while the
      // same throw in a click handler escapes into React and takes the page
      // down. The refusal itself is not optional — an unpinned PATCH is
      // last-write-wins, landing on top of an edit it never saw and reporting
      // success to both editors.
      return unwrap(
        await api.PATCH("/tags/{id}", {
          params: { path: { id }, ...ifMatch(requireVersion(version)) },
          body,
        }),
      );
    },
    onSuccess: invalidate,
  });
}

/**
 * Retire a word: it stops being offered, and stays on what already carries it.
 * `onSuccess` gets the id that restores it.
 */
export function useArchiveTag(outcome: MutationOutcome<string | null>) {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.DELETE("/tags/{id}", {
          params: { path: { id } },
        }),
      );
      return id;
    },
    onError: outcome.onError,
    onSuccess: (id) => {
      invalidate();
      outcome.onSuccess(id);
    },
  });
}

export function useRestoreTag(outcome: MutationOutcome<string>) {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (id: string) => {
      unwrap(
        await api.POST("/tags/{id}/restore", {
          params: { path: { id } },
        }),
      );
    },
    onError: outcome.onError,
    onSuccess: (_, id) => {
      invalidate();
      outcome.onSuccess(id);
    },
  });
}

/** Fold one word into another. The source is retired; the target survives. */
export function useMergeTags() {
  const invalidate = useVocabularyInvalidation();
  return useMutation({
    mutationFn: async (input: { id: string; intoTagID: string }) => {
      return unwrap(
        await api.POST("/tags/{id}/merge", {
          params: { path: { id: input.id } },
          body: { into_tag_id: input.intoTagID },
        }),
      );
    },
    onSuccess: invalidate,
  });
}
