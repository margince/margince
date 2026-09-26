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
// Rendering is recursive because the tree is. A group draws its children and its
// own join toggle; a child is either a clause row or another group, and the same
// component handles both depths, so nesting is not a special case to maintain.

import { X } from "lucide-react";
import { Badge, Button, SegmentedControl } from "../design-system/atoms";
import { Select, type SelectOption } from "../design-system/select";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import "./filterbuilder.css";
import { fieldLabel, groupFields, type VocabularyField } from "./filterdata";
import { ValueControl } from "./filtervalue";
import {
  addToGroup,
  type FilterOp,
  type Group,
  isGroup,
  type Leaf,
  type LeafValue,
  type Node,
  newGroup,
  newLeaf,
  removeNode,
  replaceNode,
  toggleJoin,
} from "./segmentpredicate";

/**
 * Which message names each operator, for a field whose comparisons read as dates.
 *
 * Keyed by reading rather than by symbol: `gte` is "on or after" a date and "at
 * least" a quantity, and one label serving both would send a reader looking for a
 * calendar on a score.
 */
const OPERATOR_KEY: Record<FilterOp, MessageKey> = {
  eq: "filters.op.eq",
  neq: "filters.op.neq",
  gt: "filters.op.afterDate",
  gte: "filters.op.onOrAfterDate",
  lt: "filters.op.beforeDate",
  lte: "filters.op.onOrBeforeDate",
  in: "filters.op.in",
  contains: "filters.op.contains",
  exists: "filters.op.exists",
};

/** The four whose reading changes on a quantity. */
const NUMERIC_OPERATOR_KEY: Partial<Record<FilterOp, MessageKey>> = {
  gt: "filters.op.moreThan",
  gte: "filters.op.atLeast",
  lt: "filters.op.lessThan",
  lte: "filters.op.atMost",
};

function operatorKey(op: FilterOp, type: VocabularyField["type"]): MessageKey {
  if (type === "number" || type === "currency") {
    return NUMERIC_OPERATOR_KEY[op] ?? OPERATOR_KEY[op];
  }
  return OPERATOR_KEY[op];
}

/**
 * The value a clause starts with when its field or operator changes.
 *
 * It is deliberately the EMPTY one for its shape rather than a guess: an operand
 * this screen invented would be a filter the human did not write, and the count
 * would move before they had said anything. `isComplete` then holds the preview
 * back until they fill it in, which is the whole reason an empty value is safe to
 * put in the tree.
 *
 * `exists` is the exception, because its operand is a boolean and every boolean
 * is complete — so it starts at `true` ("has a value"), the reading its label
 * gives.
 */
function emptyValueFor(op: FilterOp): LeafValue {
  if (op === "exists") {
    return true;
  }
  return op === "in" ? [] : "";
}

export type FilterBuilderProps = Readonly<{
  tree: Node;
  onChange: (next: Node) => void;
  fields: readonly VocabularyField[];
}>;

/** The builder's root: one group, drawn with the recursive renderer below. */
export function FilterBuilder({ tree, onChange, fields }: FilterBuilderProps) {
  if (!isGroup(tree)) {
    // The root is always a group — a bare leaf has nowhere to put the join
    // toggle, and every operation in segmentpredicate assumes a group root.
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
 * One group: its join toggle, its children, and the two ways to grow it.
 *
 * Depth is carried only to stop offering "add a group" past the engine's nesting
 * bound — a builder that let a human nest a fifth level would be building a tree
 * the server refuses with `filter_too_deep`, which is a refusal the UI can see
 * coming.
 */
function GroupNode({
  group,
  fields,
  depth,
  tree,
  onChange,
}: NodeProps & Readonly<{ group: Group }>) {
  const t = useT();
  const canNest = depth < MAX_GROUP_DEPTH;
  return (
    <div className="filter-group" data-depth={depth}>
      <div className="filter-group-head">
        <SegmentedControl
          options={["and", "or"] as const}
          value={group.join}
          onChange={() => onChange(toggleJoin(tree, group.id))}
          labels={{ and: t("filters.joinAll"), or: t("filters.joinAny") }}
          label={t("filters.joinLabel")}
        />
        {depth > 1 && (
          <Button
            variant="ghost"
            onClick={() => onChange(removeNode(tree, group.id))}
          >
            {t("filters.removeGroup")}
          </Button>
        )}
      </div>

      <div className="filter-group-children">
        {group.children.map((child) =>
          isGroup(child) ? (
            <GroupNode
              key={child.id}
              group={child}
              fields={fields}
              depth={depth + 1}
              tree={tree}
              onChange={onChange}
            />
          ) : (
            <ClauseRow
              key={child.id}
              leafID={child.id}
              field={child.field}
              op={child.op}
              value={child.value}
              fields={fields}
              tree={tree}
              onChange={onChange}
            />
          ),
        )}
        {group.children.length === 0 && (
          <p className="filter-group-empty t-sub">{t("filters.emptyGroup")}</p>
        )}
      </div>

      <div className="filter-group-actions">
        <Button
          variant="ghost"
          onClick={() =>
            onChange(addToGroup(tree, group.id, firstClause(fields)))
          }
        >
          {t("filters.addClause")}
        </Button>
        {canNest && (
          <Button
            variant="ghost"
            onClick={() =>
              onChange(
                addToGroup(
                  tree,
                  group.id,
                  newGroup(group.join === "and" ? "or" : "and"),
                ),
              )
            }
          >
            {t("filters.addGroup")}
          </Button>
        )}
      </div>
    </div>
  );
}

/**
 * The engine's own nesting bound. Named here rather than imported because the
 * tree module does not enforce it — the server does, and this is the UI's copy of
 * a number whose authority is `storekit.PredicateMaxDepth`. It exists to stop
 * OFFERING what would be refused; the refusal itself remains the server's.
 */
const MAX_GROUP_DEPTH = 4;

/** A new clause starts on the first field a picker would offer. */
function firstClause(fields: readonly VocabularyField[]): Node {
  const first = fields[0];
  if (!first) {
    // A resource with no filterable field cannot happen through this screen —
    // the vocabulary read 404s rather than answering an empty set — but the tree
    // must still be a valid node if it ever did.
    return newLeaf("", "eq", "");
  }
  const op = (first.operators[0] ?? "eq") as FilterOp;
  return newLeaf(first.name, op, emptyValueFor(op));
}

// Deliberately NOT NodeProps: a clause has no children, so depth would be a
// prop it accepts and ignores — and a prop nothing reads is one the next contact
// has to check before trusting.
type ClauseRowProps = Readonly<{
  leafID: string;
  field: string;
  op: FilterOp;
  value: LeafValue;
  fields: readonly VocabularyField[];
  tree: Node;
  onChange: (next: Node) => void;
}>;

/**
 * One clause: field, operator, value, remove.
 *
 * The three selects are chained by the vocabulary rather than independent. Change
 * the field and the operator list changes with its type; change either and the
 * value control changes shape. Keeping them chained is what stops a reader
 * assembling `created_at contains "x"` and learning from a 422 that dates have no
 * substring.
 */
function ClauseRow({
  leafID,
  field,
  op,
  value,
  fields,
  tree,
  onChange,
}: ClauseRowProps) {
  const t = useT();
  const chosen = fields.find((f) => f.name === field);
  const { core, custom } = groupFields(fields);
  const fieldOptions: SelectOption[] = [
    ...core.map((f) => ({ value: f.name, label: fieldLabel(f, t) })),
    ...custom.map((f) => ({ value: f.name, label: fieldLabel(f, t) })),
  ];
  const operatorOptions: SelectOption[] = (chosen?.operators ?? []).map(
    (candidate) => ({
      value: candidate,
      label: t(operatorKey(candidate as FilterOp, chosen?.type ?? "text")),
    }),
  );

  // Every edit below rewrites the leaf IN PLACE, keeping its id.
  //
  // That is not cosmetic. React keys a clause row on its node's id, so minting a
  // fresh leaf per keystroke remounts the row and the caret goes with it — a
  // human typing "gold" would get "g" and lose focus. Spreading the node that
  // was found keeps the row alive through its own value changing.
  const edit = (change: (found: Leaf) => Leaf) =>
    onChange(
      replaceNode(tree, leafID, (found) =>
        isGroup(found) ? found : change(found),
      ) ?? tree,
    );

  const retype = (nextField: string) => {
    const next = fields.find((f) => f.name === nextField);
    // The operator may not survive the move — a date has no `contains` — so it
    // falls back to the new field's first admitted one rather than staying and
    // being refused.
    const keptOp = next?.operators.includes(op)
      ? op
      : ((next?.operators[0] ?? "eq") as FilterOp);
    edit((found) => ({
      ...found,
      field: nextField,
      op: keptOp as FilterOp,
      value: emptyValueFor(keptOp as FilterOp),
    }));
  };

  return (
    <div className="filter-clause">
      <Select
        options={fieldOptions}
        value={field}
        onChange={retype}
        aria-label={t("filters.field")}
        placeholder={t("filters.choosePlaceholder")}
      />
      {chosen?.custom && (
        <Badge tone="accent">{t("filters.customBadge")}</Badge>
      )}
      <Select
        options={operatorOptions}
        value={op}
        onChange={(nextOp) =>
          edit((found) => ({
            ...found,
            op: nextOp as FilterOp,
            value: emptyValueFor(nextOp as FilterOp),
          }))
        }
        aria-label={t("filters.operator")}
      />
      <ValueControl
        type={chosen?.type ?? "text"}
        references={chosen?.references}
        options={chosen?.options}
        op={op}
        value={value}
        onChange={(nextValue) =>
          edit((found) => ({ ...found, value: nextValue }))
        }
      />
      <Button
        variant="ghost"
        iconOnly
        aria-label={t("filters.removeClause", {
          field: fieldLabel(
            chosen ?? {
              name: field,
              type: "text",
              operators: [],
              custom: false,
            },
            t,
          ),
        })}
        onClick={() => onChange(removeNode(tree, leafID))}
      >
        <X aria-hidden="true" />
      </Button>
    </div>
  );
}
