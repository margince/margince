// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The predicate builder: the visual half of the filter tree
// (AC-filters-and-views-3/4).
//
// Every edit goes through segmentpredicate's immutable operations and every
// choice offered comes from the server's vocabulary. That pairing is the whole
// design: the tree module knows what a filter IS, the vocabulary knows what a
// filter MAY SAY, and this file knows only how to draw the two. It invents no
// field, no operator, and no value type of its own — which is why a clause it
// builds cannot be one the engine refuses as unknown.
//
// It grows only as the reader builds. A group's join is drawn as the word
// between its conditions, so it appears once there are two of them; a nested
// group is offered from two conditions on, behind More, and arrives with a
// condition already in it.

import { Fragment, useId } from "react";
import { Button, OverflowMenu } from "../design-system/atoms";
import { useT } from "../i18n";
import "./filterbuilder.css";
import { ClauseRow, firstClause } from "./filterclause";
import type { VocabularyField } from "./filterdata";
import {
  addToGroup,
  type Group,
  isGroup,
  type Node,
  newGroup,
  removeNode,
  toggleJoin,
} from "./segmentpredicate";

export type FilterBuilderProps = Readonly<{
  tree: Node;
  onChange: (next: Node) => void;
  fields: readonly VocabularyField[];
  /** The clause the reader's last edit brought, which takes focus. */
  arrived?: string | null;
}>;

/** The builder's root: one group, drawn with the recursive renderer below. */
export function FilterBuilder({
  tree,
  onChange,
  fields,
  arrived = null,
}: FilterBuilderProps) {
  if (!isGroup(tree)) {
    // The root is always a group — a bare leaf has no join to draw, and every
    // operation in segmentpredicate assumes a group root.
    return null;
  }
  return (
    <div className="filter-builder">
      <GroupNode
        group={tree}
        fields={fields}
        depth={1}
        onChange={onChange}
        tree={tree}
        arrived={arrived}
      />
    </div>
  );
}

type NodeProps = Readonly<{
  fields: readonly VocabularyField[];
  depth: number;
  /** The whole tree, because every edit is expressed against its root. */
  tree: Node;
  onChange: (next: Node) => void;
}>;

/**
 * One group: its conditions with the join between them, and the ways to grow
 * it. The root draws no box; a nested group is an inset box, because a join
 * word between two rows needs a visible scope to say which rows it joins.
 */
function GroupNode({
  group,
  fields,
  depth,
  tree,
  onChange,
  arrived,
}: NodeProps & Readonly<{ group: Group; arrived: string | null }>) {
  const t = useT();
  const titleID = useId();
  const nested = depth > 1;
  const rows = (
    <>
      <div className="filter-group-children">
        {group.children.map((child, index) => (
          <Fragment key={child.id}>
            {index > 0 && (
              <Connector group={group} tree={tree} onChange={onChange} />
            )}
            {isGroup(child) ? (
              <GroupNode
                group={child}
                fields={fields}
                depth={depth + 1}
                tree={tree}
                onChange={onChange}
                arrived={arrived}
              />
            ) : (
              <ClauseRow
                leafID={child.id}
                field={child.field}
                op={child.op}
                value={child.value}
                proposed={child.proposed}
                arrived={child.id === arrived}
                fields={fields}
                tree={tree}
                onChange={onChange}
              />
            )}
          </Fragment>
        ))}
        {/* Said rather than left blank: an empty group compiles to a shape
            the engine refuses, so the count stops until it holds a row. */}
        {nested && group.children.length === 0 && (
          <p className="filter-group-empty t-sub">{t("filters.emptyGroup")}</p>
        )}
      </div>
      <GroupFoot
        group={group}
        fields={fields}
        depth={depth}
        tree={tree}
        onChange={onChange}
      />
    </>
  );
  if (!nested) {
    return <div className="filter-group">{rows}</div>;
  }
  // A fieldset named by its head, so a screen reader hears where the group's
  // conditions start and end, and whose "Remove group" each control is.
  return (
    <fieldset
      className="filter-group filter-group-box"
      aria-labelledby={titleID}
    >
      <div className="filter-group-head">
        <span id={titleID} className="filter-group-title">
          {t(group.join === "and" ? "filters.group.all" : "filters.group.any")}
        </span>
        <Button
          variant="ghost"
          onClick={() => onChange(removeNode(tree, group.id))}
        >
          {t("filters.removeGroup")}
        </Button>
      </div>
      {rows}
    </fieldset>
  );
}

/**
 * The word between two conditions, which is also the switch: a group joins
 * its children one way, so every connector in it flips together.
 */
function Connector({
  group,
  tree,
  onChange,
}: Readonly<{ group: Group; tree: Node; onChange: (next: Node) => void }>) {
  const t = useT();
  const all = group.join === "and";
  const word = t(all ? "filters.join.and" : "filters.join.or");
  return (
    <div className="filter-connector">
      <Button
        variant="ghost"
        // The name opens with the word on the button, so a reader who says
        // "click or" reaches it (WCAG 2.5.3), and goes on to say what it does.
        aria-label={t(
          all ? "filters.connector.matchAll" : "filters.connector.matchAny",
          { word },
        )}
        onClick={() => onChange(toggleJoin(tree, group.id))}
      >
        {word}
      </Button>
    </div>
  );
}

/**
 * How a group grows: another condition, and from two conditions on a nested
 * group of the opposite join. The nested group arrives holding a condition,
 * because an empty one stops the count until it is filled.
 *
 * Depth stops the offer at the engine's nesting bound: a fifth level would be
 * a tree the server refuses with `filter_too_deep`.
 */
function GroupFoot({
  group,
  fields,
  depth,
  tree,
  onChange,
}: NodeProps & Readonly<{ group: Group }>) {
  const t = useT();
  const nested = depth > 1;
  const canNest = group.children.length >= 2 && depth < MAX_GROUP_DEPTH;
  return (
    <div className="filter-group-foot">
      <Button
        variant="link"
        onClick={() =>
          onChange(addToGroup(tree, group.id, firstClause(fields)))
        }
      >
        {t(nested ? "filters.addToGroup" : "filters.addClause")}
      </Button>
      {canNest && (
        <OverflowMenu
          label={t(nested ? "filters.groupMore" : "filters.rowsMore")}
        >
          <Button
            onClick={() =>
              onChange(
                addToGroup(
                  tree,
                  group.id,
                  newGroup(group.join === "and" ? "or" : "and", [
                    firstClause(fields),
                  ]),
                ),
              )
            }
          >
            {t("filters.addGroup")}
          </Button>
        </OverflowMenu>
      )}
    </div>
  );
}

/**
 * The engine's nesting bound. The tree module does not enforce it — the server
 * does, and this is the UI's copy of a number whose authority is
 * `storekit.PredicateMaxDepth`. It stops OFFERING what would be refused; the
 * refusal itself stays the server's.
 */
const MAX_GROUP_DEPTH = 4;
