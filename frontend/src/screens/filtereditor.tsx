// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The body of a filter page's editing panel: two ways in while there is no
// condition, then the rows, with the plain-words box folded above them and a
// proposal's bar above the rows it marked.

import { Plus } from "lucide-react";
import { type Dispatch, useLayoutEffect, useRef, useState } from "react";
import { Button, Card } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { FilterBuilder } from "./filterbuilder";
import { firstClause, focusRow } from "./filterclause";
import type { FilterVocabulary } from "./filterdata";
import type { FilterDraft, FilterDraftAction } from "./filterdraft";
import {
  type PlainWords,
  PlainWordsFilter,
  ProposalBar,
  UnusedPhrases,
} from "./filterpropose";
import { addToGroup, isGroup, type Node } from "./segmentpredicate";
import "./filters.css";

/** The vocabulary read, which the editor and what is under it wait on. */
export type VocabularyRead = Readonly<{
  data?: FilterVocabulary;
  isPending: boolean;
  isError: boolean;
}>;

export function FilterEditor({
  draft,
  dispatch,
  vocabulary,
  words,
  records,
}: Readonly<{
  draft: FilterDraft;
  dispatch: Dispatch<FilterDraftAction>;
  vocabulary: VocabularyRead;
  words: PlainWords;
  /** The record type in the reader's words: "contacts". */
  records: string;
}>) {
  const t = useT();
  const fields = vocabulary.data?.fields ?? [];
  const empty = draft.tree.children.length === 0;
  const focus = useArrivalFocus(draft);
  return (
    <SurfaceState
      state={vocabularyState(vocabulary)}
      emptyLabel={t("filters.noFields")}
      loadingLabel={t("filters.loadingVocabulary")}
      // A condition row plus its verbs is what lands here.
      loadingLines={4}
    >
      <div
        ref={focus.editor}
        className={empty ? "filters-editor filters-start" : "filters-editor"}
      >
        <PlainWordsFilter
          words={words}
          records={records}
          layout={empty ? "start" : "folded"}
          open={draft.wordsOpen}
          onOpen={(open) => dispatch({ type: "setWordsOpen", open })}
        />
        {empty ? (
          <BuildByHand
            refocus={focus.restarted}
            onAdd={() =>
              dispatch({
                type: "edit",
                tree: addToGroup(
                  draft.tree,
                  draft.tree.id,
                  firstClause(fields),
                ),
              })
            }
          />
        ) : (
          <>
            <ProposalBar
              tree={draft.tree}
              proposal={draft.proposal}
              dispatch={dispatch}
            />
            <FilterBuilder
              tree={draft.tree}
              onChange={(tree) => dispatch({ type: "edit", tree })}
              fields={fields}
              arrived={focus.arrived}
            />
          </>
        )}
        {/* Outside both states: an answer that expressed nothing leaves the
            page on its calm start, and what it could not use is still said. */}
        <div className="filters-unused">
          <UnusedPhrases
            unused={draft.unused}
            fields={fields}
            onDismiss={() => dispatch({ type: "dismissUnused" })}
          />
        </div>
      </div>
    </SurfaceState>
  );
}

/**
 * Where focus goes after the tree changes. The reader's own new row takes it,
 * and the calm start takes it back when the last row goes.
 *
 * A tree the reader did not write in the rows (an answer, or a press on the
 * proposal's bar) moves focus only when it has nowhere else to be: on <body>
 * because the control pressed went with the press, or in the box that asked,
 * which folds or leaves as the answer lands. A reader typing in a row or
 * naming a save keeps their place, and a tree from outside moves nothing.
 */
function useArrivalFocus(draft: FilterDraft) {
  const editor = useRef<HTMLDivElement>(null);
  const [seen, setSeen] = useState(draft.tree);
  const [arrived, setArrived] = useState<string | null>(null);
  const [restarted, setRestarted] = useState(false);
  const { tree, by } = draft;
  if (tree !== seen) {
    setSeen(tree);
    setArrived(by === "edit" ? firstNewLeaf(seen, tree) : null);
    setRestarted(
      by !== "reset" && seen.children.length > 0 && tree.children.length === 0,
    );
  }
  // An empty tree is the calm start's, which takes focus itself. The rows an
  // answer marks are all new, because it replaces the last one's untouched rows.
  useLayoutEffect(() => {
    if (
      by === "edit" ||
      by === "reset" ||
      tree.children.length === 0 ||
      !focusIsFree()
    ) {
      return;
    }
    const rows = editor.current;
    focusRow(
      rows?.querySelector("[data-proposed]") ??
        rows?.querySelector(".filter-clause"),
    );
  }, [tree, by]);
  return { editor, arrived, restarted };
}

/**
 * Focus for a press whose control leaves with it, such as Discard: once the
 * rows it redrew are on screen, the first row takes it, rather than the
 * reader's place falling to the top of the document.
 */
export function useRowsFocus() {
  const rows = useRef<HTMLDivElement>(null);
  const asked = useRef(false);
  useLayoutEffect(() => {
    if (asked.current) {
      asked.current = false;
      focusRow(rows.current);
    }
  });
  return {
    rows,
    focusRows: () => {
      asked.current = true;
    },
  };
}

/** Focus on nothing, or still in the box that asked for the answer. */
function focusIsFree(): boolean {
  const active = document.activeElement;
  return (
    active === null ||
    active === document.body ||
    active.closest(".filters-propose") !== null
  );
}

/**
 * The first row the reader's edit brought onto the page, in reading order.
 * Every edit keeps the ids of the rows it leaves alone, so a row that is new
 * here is new on screen.
 */
function firstNewLeaf(before: Node, after: Node): string | null {
  const known = new Set(leafIDs(before));
  return leafIDs(after).find((id) => !known.has(id)) ?? null;
}

function leafIDs(node: Node): string[] {
  return isGroup(node) ? node.children.flatMap(leafIDs) : [node.id];
}

/**
 * The second way in: the first condition, by hand. `refocus` is the calm
 * start coming back because the last row went, removed or undone.
 */
function BuildByHand({
  onAdd,
  refocus,
}: Readonly<{ onAdd: () => void; refocus: boolean }>) {
  const t = useT();
  const add = useRef<HTMLButtonElement>(null);
  useLayoutEffect(() => {
    if (refocus) {
      add.current?.focus();
    }
  }, [refocus]);
  return (
    <>
      <p className="filters-start-or t-caption">{t("filters.startOr")}</p>
      <Card as="div" inset>
        <div className="filters-start-card">
          <Heading size="medium">{t("filters.start.buildTitle")}</Heading>
          <p className="t-sub">{t("filters.start.buildBody")}</p>
          <Button ref={add} variant="primary" onClick={onAdd}>
            <Plus aria-hidden />
            {t("filters.addClause")}
          </Button>
        </div>
      </Card>
    </>
  );
}

/**
 * The vocabulary read's outcomes as a surface state. An error is `unavailable`
 * rather than `empty`: an empty field list would say this record type has
 * nothing to filter on, and the endpoint 404s rather than answering one.
 */
export function vocabularyState(vocabulary: VocabularyRead): SectionState {
  if (vocabulary.isPending) {
    return "loading";
  }
  return vocabulary.isError ? "unavailable" : "ready";
}
