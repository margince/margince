// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// One condition of the filter tree: field, operator, value, remove. The
// builder (filterbuilder.tsx) lays these out; this file knows how one is drawn
// and how an edit to it is written back.

import { X } from "lucide-react";
import { useLayoutEffect, useRef } from "react";
import { Badge, Button } from "../design-system/atoms";
import { Select, type SelectOption } from "../design-system/select";
import { forReader } from "../format/collate";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { fieldLabel, groupFields, type VocabularyField } from "./filterdata";
import { ownLeaf } from "./filterproposal";
import { operatorKey } from "./filtersentence";
import { ValueControl } from "./filtervalue";
import {
  type FilterOp,
  isGroup,
  type Leaf,
  type LeafValue,
  type Node,
  newLeaf,
  removeNode,
  replaceNode,
} from "./segmentpredicate";

/**
 * The value a clause starts with when its field or operator changes.
 *
 * It is deliberately the EMPTY one for its shape rather than a guess: an operand
 * this screen invented would be a filter the human did not write, and the count
 * would move before they had said anything. `isComplete` then holds the preview
 * back until they fill it in, which is the whole reason an empty value is safe to
 * put in the tree.
 *
 * A boolean operand, on `exists` or a yes/no field, is the exception: its
 * two-way control always shows one arm pressed, and every boolean is complete.
 * So it starts at `true`, the arm shown, and the row never shows a choice the
 * tree does not hold.
 */
function emptyValueFor(
  op: FilterOp,
  type: VocabularyField["type"] | undefined,
): LeafValue {
  if (op === "in") {
    return [];
  }
  return op === "exists" || type === "boolean" ? true : "";
}

/**
 * One picker group as options, in the order a reader scans them: by the word
 * they see, not the wire name — `created_at` and `last_activity_at` would
 * otherwise sit far from each other under "Created" and "Last activity".
 */
function labelledInOrder(
  fields: readonly VocabularyField[],
  t: (key: MessageKey) => string,
  locale: Locale,
): SelectOption[] {
  return fields
    .map((f) => ({ value: f.name, label: fieldLabel(f, t) }))
    .sort((a, b) => forReader(a.label, b.label, locale));
}

/** A new clause starts on the vocabulary's first field, a reader picks from there. */
export function firstClause(fields: readonly VocabularyField[]): Node {
  const first = fields[0];
  if (!first) {
    // A resource with no filterable field cannot happen through this screen —
    // the vocabulary read 404s rather than answering an empty set — but the tree
    // must still be a valid node if it ever did.
    return newLeaf("", "eq", "");
  }
  const op = (first.operators[0] ?? "eq") as FilterOp;
  return newLeaf(first.name, op, emptyValueFor(op, first.type));
}

// Deliberately not the group's props: a clause has no children, so depth would
// be a prop it accepts and ignores.
type ClauseRowProps = Readonly<{
  leafID: string;
  field: string;
  op: FilterOp;
  value: LeafValue;
  /** A model proposed this clause and the reader has not changed it yet. */
  proposed?: boolean;
  /** The reader's last edit brought this clause: focus starts at its field. */
  arrived?: boolean;
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
/**
 * A row takes focus on its field, its first combobox: the first choice the
 * reader makes about a condition, and the one everything after it reads from.
 * Given rows rather than one, the first row takes it.
 */
export function focusRow(row: ParentNode | null | undefined) {
  row?.querySelector<HTMLElement>('[role="combobox"]')?.focus();
}

export function ClauseRow({
  leafID,
  field,
  op,
  value,
  proposed,
  arrived = false,
  fields,
  tree,
  onChange,
}: ClauseRowProps) {
  const t = useT();
  const { locale } = useLocale();
  const row = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => {
    if (arrived) {
      focusRow(row.current);
    }
  }, [arrived]);
  const chosen = fields.find((f) => f.name === field);
  const { core, custom } = groupFields(fields);
  const fieldOptions: SelectOption[] = [
    ...labelledInOrder(core, t, locale),
    ...labelledInOrder(custom, t, locale),
  ];
  const operatorOptions: SelectOption[] = (chosen?.operators ?? []).map(
    (candidate) => ({
      value: candidate,
      label: t(operatorKey(candidate as FilterOp, chosen?.type ?? "text")),
    }),
  );

  // Every edit below rewrites the leaf IN PLACE, keeping its id: React keys the
  // row on it, and a fresh leaf per keystroke would remount the row and take
  // the caret with it.
  //
  // And every edit goes through here, so a proposed clause becomes the
  // reader's own on whichever part they change: one place drops the mark.
  const edit = (change: (found: Leaf) => Leaf) =>
    onChange(
      replaceNode(tree, leafID, (found) =>
        isGroup(found) ? found : ownLeaf(change(found)),
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
      op: keptOp,
      value: emptyValueFor(keptOp, next?.type),
    }));
  };

  return (
    <div
      ref={row}
      className="filter-clause"
      data-proposed={proposed ? "" : undefined}
    >
      <Select
        options={fieldOptions}
        value={field}
        onChange={retype}
        aria-label={t("filters.field")}
        placeholder={t("filters.choosePlaceholder")}
      />
      {chosen?.custom && <Badge>{t("filters.customBadge")}</Badge>}
      <Select
        options={operatorOptions}
        value={op}
        onChange={(nextOp) =>
          edit((found) => ({
            ...found,
            op: nextOp as FilterOp,
            value: emptyValueFor(nextOp as FilterOp, chosen?.type),
          }))
        }
        aria-label={t("filters.operator")}
      />
      <ValueControl
        type={chosen?.type ?? "text"}
        references={chosen?.references}
        options={chosen?.options}
        currency={chosen?.currency}
        op={op}
        value={value}
        onChange={(nextValue) =>
          edit((found) => ({ ...found, value: nextValue }))
        }
      />
      {proposed && <Badge tone="ai">{t("filters.proposed")}</Badge>}
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
