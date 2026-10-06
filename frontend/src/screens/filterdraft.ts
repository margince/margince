// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The filter a page is drafting, as one reducer: the tree and the phrases the
// last plain-words answer could not use. One state, so a change to the tree
// and the phrases it came with land together or not at all.

import { useReducer } from "react";
import type { components } from "../api/schema";
import { type Group, type Node, rootGroup } from "./segmentpredicate";

type UnusedPhrase = components["schemas"]["FilterProposalUnsupported"];

export type FilterDraft = Readonly<{
  tree: Group;
  unused: readonly UnusedPhrase[];
}>;

export type FilterDraftAction =
  /** The reader changed the tree. */
  | Readonly<{ type: "edit"; tree: Node }>
  /** A tree from outside the editor: an opened view, a list's filter. */
  | Readonly<{ type: "reset"; tree: Node }>
  /**
   * A plain-words answer the reader took, with what it could not use. A null
   * tree keeps the reader's own: an answer that expressed nothing still has
   * phrases to name back.
   */
  | Readonly<{
      type: "apply";
      tree: Node | null;
      unused: readonly UnusedPhrase[];
    }>
  | Readonly<{ type: "dismissUnused" }>;

// Every stored tree goes through `rootGroup`: a stored single clause decodes
// as a bare leaf, and the builder and every edit need a group to add to.
function draftReducer(
  state: FilterDraft,
  action: FilterDraftAction,
): FilterDraft {
  switch (action.type) {
    case "edit":
      return { ...state, tree: rootGroup(action.tree) };
    case "reset":
      return { tree: rootGroup(action.tree), unused: [] };
    case "apply":
      return {
        tree: action.tree === null ? state.tree : rootGroup(action.tree),
        unused: action.unused,
      };
    case "dismissUnused":
      return { ...state, unused: [] };
  }
}

/** A function rather than a tree, so a fresh tree's id is minted once per page. */
export function useFilterDraft(initial: () => Group) {
  return useReducer(draftReducer, initial, (start) => ({
    tree: start(),
    unused: [],
  }));
}
