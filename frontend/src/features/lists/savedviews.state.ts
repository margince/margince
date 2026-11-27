// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a saved view stores for a list, and where a view opens. Pure, so the
// library and a list's tab rail read one answer.

import type { ListQuery } from "../../components/listquery";
import {
  type CoreLibraryType,
  LIBRARY_TYPES,
  type ObjectTab,
  tabOfViewResource,
} from "../../lib/filtersaddress";
import { openableTree, type SavedView } from "../../lib/savedviews.queries";
import { type Node, rootGroup } from "../../lib/segmentpredicate";

/**
 * The key a view keeps its list state under. Not `filter`: the server reads
 * that key as a filter tree, and a list's filters are flat `param=value` pairs.
 */
const LIST_STATE_KEY = "list";

export type SavedListState = Pick<
  ListQuery,
  "q" | "sort" | "includeArchived" | "filters"
> & {
  /**
   * The page size the view claims, or absent when it claims none. Not
   * defaulted: a view that stored none leaves the reader's own page size alone.
   */
  perPage?: number;
};

/** The saved state of one view, or null when the row predates this shape. */
export function listStateOf(view: SavedView): SavedListState | null {
  // A hand-written row can lack `query`, and a rail that throws takes its list down.
  const stored = view.query as Record<string, unknown> | undefined;
  const raw = stored?.[LIST_STATE_KEY];
  if (!raw || typeof raw !== "object") {
    return null;
  }
  const state = raw as Partial<SavedListState>;
  // Each field is checked, since a stored row can hold any shape. A missing
  // dial reads as its neutral value; a missing page size stays absent.
  return {
    q: typeof state.q === "string" ? state.q : "",
    sort: typeof state.sort === "string" ? state.sort : "",
    includeArchived: state.includeArchived === true,
    filters:
      state.filters && typeof state.filters === "object"
        ? Object.fromEntries(
            Object.entries(state.filters).filter(
              ([, value]) => typeof value === "string",
            ),
          )
        : {},
    perPage: typeof state.perPage === "number" ? state.perPage : undefined,
  };
}

/** What a view saves, given the list the reader is looking at now. */
export function listStateFrom(query: ListQuery): Record<string, unknown> {
  return {
    [LIST_STATE_KEY]: {
      q: query.q,
      sort: query.sort,
      includeArchived: query.includeArchived,
      filters: query.filters,
      perPage: query.perPage,
    },
  };
}

/** Where a saved view opens: the builder, its list, or nowhere this build can. */
export type ViewOpening =
  | Readonly<{ kind: "builder"; tree: Node; tab: ObjectTab }>
  | Readonly<{ kind: "list"; state: SavedListState; type: CoreLibraryType }>
  | null;

/**
 * The builder wins whenever it can read the view's filter: its list would drop
 * that filter and show rows the view leaves out. Otherwise a view with list
 * state opens on its list. A stored single clause is wrapped in a group.
 */
export function viewOpening(view: SavedView): ViewOpening {
  const tree = openableTree(view);
  const tab = tabOfViewResource(view.resource);
  if (tree && tab) {
    return { kind: "builder", tree: rootGroup(tree), tab };
  }
  const type = LIBRARY_TYPES.find((known) => known === view.resource);
  const state = listStateOf(view);
  return type && state ? { kind: "list", state, type } : null;
}
