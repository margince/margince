// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The filter a page is drafting, as one reducer: the tree, the proposal still
// marked on it, the phrases the last answer could not use, and whether the
// folded description is open. One state, so a model's answer lands against
// the tree as it stands when the answer ARRIVES, with no ref kept in step.

import type { FetchStatus } from "@tanstack/react-query";
import { useReducer, useState } from "react";
import type { components } from "../api/schema";
import {
  acceptProposals,
  landProposal,
  type Proposal,
  proposedCount,
  replaceWithProposal,
} from "./filterproposal";
import { encode, type Group, type Node, rootGroup } from "./segmentpredicate";

type UnusedPhrase = components["schemas"]["FilterProposalUnsupported"];

export type FilterDraft = Readonly<{
  tree: Group;
  /**
   * The action that last wrote the tree. Focus follows the reader's own edits;
   * a tree they did not write moves it only when it has nowhere else to be.
   */
  by: FilterDraftAction["type"];
  /** The proposal on screen, null once none of its marks is left. */
  proposal: Proposal | null;
  unused: readonly UnusedPhrase[];
  /** Whether the description folded above the rows is open. */
  wordsOpen: boolean;
}>;

export type FilterDraftAction =
  /** The reader changed the tree. */
  | Readonly<{ type: "edit"; tree: Node }>
  /** A tree from outside the editor: an opened view, a list's filter. */
  | Readonly<{ type: "reset"; tree: Node }>
  /** A model's answer, decoded, and what it could not use. */
  | Readonly<{
      type: "answer";
      proposed: Group;
      unused: readonly UnusedPhrase[];
      text: string;
    }>
  /** An answer that expressed nothing still names what it could not use. */
  | Readonly<{ type: "unused"; unused: readonly UnusedPhrase[] }>
  /**
   * A new description is on its way: the last one's phrases are spent, and
   * the folded box opens so the wait shows even under the reader's first row.
   */
  | Readonly<{ type: "asking" }>
  | Readonly<{ type: "keepAll" }>
  | Readonly<{ type: "replaceMine" }>
  | Readonly<{ type: "undo" }>
  | Readonly<{ type: "setWordsOpen"; open: boolean }>
  | Readonly<{ type: "dismissUnused" }>;

// A proposal is on screen only while one of its rows still carries the mark:
// once the reader has made every one their own, there is nothing to keep or
// undo, whichever action took the last mark.
function draftReducer(
  state: FilterDraft,
  action: FilterDraftAction,
): FilterDraft {
  const written = applied(state, action);
  const next =
    written.tree === state.tree ? written : { ...written, by: action.type };
  return next.proposal !== null && proposedCount(next.tree) === 0
    ? { ...next, proposal: null }
    : next;
}

// Every stored tree goes through `rootGroup`: a stored single clause decodes
// as a bare leaf, and the builder and every edit need a group to add to.
function applied(state: FilterDraft, action: FilterDraftAction): FilterDraft {
  switch (action.type) {
    case "edit":
      return { ...state, tree: rootGroup(action.tree) };
    case "reset":
      return {
        tree: rootGroup(action.tree),
        by: "reset",
        proposal: null,
        unused: [],
        wordsOpen: false,
      };
    case "answer": {
      const landed = landProposal(
        state.tree,
        action.proposed,
        action.text,
        state.proposal,
      );
      return { ...state, ...landed, unused: action.unused, wordsOpen: false };
    }
    case "unused":
      return { ...state, unused: action.unused };
    case "asking":
      return { ...state, unused: [], wordsOpen: true };
    case "dismissUnused":
      return { ...state, unused: [] };
    case "keepAll":
      return { ...state, tree: acceptProposals(state.tree), proposal: null };
    case "replaceMine":
      return state.proposal === null
        ? state
        : {
            ...state,
            tree: replaceWithProposal(state.tree, state.proposal),
            proposal: { ...state.proposal, hadOwn: false },
          };
    case "undo":
      return state.proposal === null
        ? state
        : { ...state, tree: state.proposal.before, proposal: null };
    case "setWordsOpen":
      return { ...state, wordsOpen: action.open };
  }
}

/** A function rather than a tree, so a fresh tree's id is minted once per page. */
export function useFilterDraft(initial: () => Group) {
  return useReducer(
    draftReducer,
    initial,
    (start): FilterDraft => ({
      tree: start(),
      by: "reset",
      proposal: null,
      unused: [],
      wordsOpen: false,
    }),
  );
}

/**
 * A tree as the wire carries it, for asking whether a draft still says what it
 * opened on. Ids and proposal marks never reach the wire, so neither reads as
 * an edit, and a stored single clause reads as the group the editor wraps it
 * in.
 */
export function wireSignature(tree: Node): string {
  return JSON.stringify(encode(rootGroup(tree)));
}

/**
 * The first answer this opening's read settled on, kept while later reads come
 * and go: a refetch that answers differently, or fails, must not swap the page
 * out from under the draft. A cached copy the mount is reading again does not
 * count, since it may predate a write made elsewhere and its version is the
 * one a save would be held to.
 */
export function useFirstAnswer<T>(
  read: Readonly<{
    data: T | undefined;
    fetchStatus: FetchStatus;
    isError: boolean;
  }>,
): T | undefined {
  // A failed read keeps the copy it had, which is not what it answered.
  const settled = read.fetchStatus === "idle" && !read.isError;
  const answer = settled ? read.data : undefined;
  const [first, setFirst] = useState(answer);
  if (first === undefined && answer !== undefined) {
    setFirst(answer);
  }
  return first ?? answer;
}
