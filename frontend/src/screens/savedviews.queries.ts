// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The one module for /views: the reads, the writes and the filter blob a view
// stores. A list's tab rail, the Filters and views library and an opened view
// all read through here, so one write refreshes every one of them.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import { throwProblem } from "./common";
import type { ViewResource } from "./filtersaddress";
import { decode, encode, type Node } from "./segmentpredicate";

export type SavedView = components["schemas"]["SavedView"];

/** Every cache entry a saved-view write can make stale starts with this key. */
export const SAVED_VIEWS_KEY = "views";

/** The cache key every read of a resource's saved views shares. */
export function savedViewsKey(
  resource: ViewResource,
): readonly [typeof SAVED_VIEWS_KEY, ViewResource] {
  return [SAVED_VIEWS_KEY, resource];
}

/**
 * The caller's saved views for one resource, ordered by name. `fresh` reads
 * them again on mount even when the cache holds a recent answer, for a caller
 * that must not decide from a list that predates a view it was just sent to.
 */
export function useSavedViews(resource: ViewResource, fresh = false) {
  return useQuery({
    queryKey: savedViewsKey(resource),
    queryFn: async (): Promise<SavedView[]> => {
      const { data, error } = await api.GET("/views", {
        params: { query: { resource } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
    staleTime: 60_000,
    refetchOnMount: fresh ? "always" : true,
  });
}

/**
 * Every saved view the reader owns, of every resource, in one read. The
 * library merges them with the lists; `truncated` is the server saying it
 * stopped at its cap, which drops the counts that would otherwise lie.
 */
export function useAllSavedViews() {
  return useQuery({
    queryKey: [SAVED_VIEWS_KEY, "all"],
    queryFn: async () => {
      const { data, error } = await api.GET("/views");
      if (error) {
        throwProblem(error);
      }
      return { views: data.data, truncated: data.page.has_more };
    },
  });
}

/**
 * The key the segment builder's tree lives under. The server validates it as
 * a filter TREE and compiles it against the resource's engine: the same
 * predicate `/filters/preview` and `/exports` take. One filter dialect,
 * whichever surface reads it. A list's own dials live under a key of their own
 * (savedviews.tsx), so this one stays free for the tree.
 */
const FILTER_KEY = "filter";

/**
 * The filter tree a view restores, or null when it holds none this editor can
 * read. `query` is an open JSON object, so a row can carry a shape this build
 * has never seen; `decode` checks it and answers null rather than guessing.
 */
export function filterTreeOf(view: SavedView): Node | null {
  const stored = view.query as Record<string, unknown> | undefined;
  return decode(stored?.[FILTER_KEY]);
}

/** What a view saves, given the filter the reader has just built. */
export function filterStateFrom(tree: Node): Record<string, unknown> {
  return { [FILTER_KEY]: encode(tree) };
}

/**
 * Save a named view, rename one, and remove one that has served its purpose.
 *
 * All three invalidate the `views` prefix rather than one resource: every
 * saved-view read sits under it (a list's rail, the library, an opened view),
 * and a write seen by one of them and not the others is a row that lies.
 */
export function useSaveView() {
  const client = useQueryClient();
  const invalidate = () =>
    client.invalidateQueries({ queryKey: [SAVED_VIEWS_KEY] });

  // The blob is the caller's, not this hook's: a list saves its dials under one
  // key and the segment builder saves a tree under another, and both go through
  // ONE write so there is one place that stamps the resource. A second mutation
  // per shape is how the two would drift.
  const create = useMutation({
    mutationFn: async (
      input: Readonly<{
        resource: ViewResource;
        name: string;
        query: Record<string, unknown>;
      }>,
    ) => {
      const { data, error } = await api.POST("/views", {
        body: {
          resource: input.resource,
          name: input.name,
          query: input.query,
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });

  // The version the row was read at rides as If-Match, so a rename made in
  // another tab since this list was read is refused rather than overwritten.
  const rename = useMutation({
    mutationFn: async (
      input: Readonly<{ id: string; name: string; version: number }>,
    ) => {
      const { data, error } = await api.PATCH("/views/{id}", {
        params: { path: { id: input.id }, ...ifMatch(input.version) },
        body: { name: input.name },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: async (id: string) => {
      const { error } = await api.DELETE("/views/{id}", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });

  return { create, rename, remove };
}
