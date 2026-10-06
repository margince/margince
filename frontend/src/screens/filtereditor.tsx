// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The body of a filter page's editing panel: two ways in while there is no
// condition, then the rows, with the plain-words box folded above them.
//
// The plain-words box keeps ONE position in both states. A description being
// read when the reader starts building by hand is a request still in flight,
// and its answer arrives through the component that asked; moving the box to
// another place in the tree would remount it and drop that answer.

import { Plus } from "lucide-react";
import { type Dispatch, useLayoutEffect, useRef, useState } from "react";
import { Button, Card } from "../design-system/atoms";
import { Heading } from "../design-system/heading";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { FilterBuilder } from "./filterbuilder";
import { firstClause } from "./filterclause";
import type { FilterResource, FilterVocabulary } from "./filterdata";
import type { FilterDraft, FilterDraftAction } from "./filterdraft";
import { PlainWordsFilter, UnusedPhrases } from "./filterpropose";
import { addToGroup, isGroup, type Node } from "./segmentpredicate";
import "./filters.css";

type VocabularyRead = Readonly<{
  data?: FilterVocabulary;
  isPending: boolean;
  isError: boolean;
}>;

export function FilterEditor({
  resource,
  draft,
  dispatch,
  vocabulary,
}: Readonly<{
  resource: FilterResource;
  draft: FilterDraft;
  dispatch: Dispatch<FilterDraftAction>;
  vocabulary: VocabularyRead;
}>) {
  const t = useT();
  const fields = vocabulary.data?.fields ?? [];
  const empty = draft.tree.children.length === 0;
  // Where focus goes once an edit has removed the control that made it: the
  // calm start's button, a proposal's card and the last row's remove all
  // unmount with the press, and focus left there falls to <body>.
  const [arrived, setArrived] = useState<string | null>(null);
  const [restarted, setRestarted] = useState(false);
  const follow = (next: Node) => {
    setArrived(firstNewLeaf(draft.tree, next));
    setRestarted(!empty && isGroup(next) && next.children.length === 0);
  };
  const edit = (tree: Node) => {
    dispatch({ type: "edit", tree });
    follow(tree);
  };
  return (
    <SurfaceState
      state={vocabularyState(vocabulary)}
      emptyLabel={t("filters.noFields")}
      loadingLabel={t("filters.loadingVocabulary")}
      // A condition row plus its verbs is what lands here.
      loadingLines={4}
    >
      <div
        className={empty ? "filters-editor filters-start" : "filters-editor"}
      >
        {/* Keyed by object so a sentence typed for contacts is not offered
            to deals, whose vocabulary it was never read against. */}
        <PlainWordsFilter
          key={resource}
          resource={resource}
          tree={draft.tree}
          layout={empty ? "start" : "folded"}
          onApply={(tree, unused) => {
            dispatch({ type: "apply", tree, unused });
            if (tree !== null) {
              follow(tree);
            }
          }}
        />
        {empty ? (
          <BuildByHand
            refocus={restarted}
            onAdd={() =>
              edit(addToGroup(draft.tree, draft.tree.id, firstClause(fields)))
            }
          />
        ) : (
          <FilterBuilder
            tree={draft.tree}
            onChange={edit}
            fields={fields}
            arrived={arrived}
          />
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
 * The first row a change brought onto the page, in reading order: the one the
 * reader just added, or the first a proposal wrote. Every edit keeps the ids
 * of the rows it leaves alone, so a row that is new here is new on screen.
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
 * start coming back because the reader removed the last row.
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
