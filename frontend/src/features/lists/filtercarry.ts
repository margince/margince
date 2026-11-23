// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a clause's value becomes when its field or operator changes.

import type { VocabularyField } from "../../lib/filterdata";
import {
  type FilterOp,
  isRelativeDate,
  type LeafValue,
} from "../../lib/segmentpredicate";

type FieldType = VocabularyField["type"];

/**
 * The value a clause starts with when its field changes, or when its operator
 * changes and the value it held cannot carry over.
 *
 * Empty, not a guess: an invented operand would move the count before the
 * reader said anything, and `isComplete` holds an empty one back. A boolean
 * starts at `true`, the arm its two-way control shows pressed.
 */
export function emptyValueFor(
  op: FilterOp,
  type: FieldType | undefined,
): LeafValue {
  if (op === "in") {
    return [];
  }
  return op === "exists" || type === "boolean" ? true : "";
}

/**
 * The value a clause keeps when only its operator changes, where the new
 * operator's control can show it. Out of a list only a sole item carries, since
 * picking one of several is the reader's choice. On a number or money field
 * only a number enters or leaves a list, as the engine compares only numbers.
 */
export function carriedValue(
  from: FilterOp,
  to: FilterOp,
  value: LeafValue,
  type: FieldType | undefined,
): LeafValue {
  if (from === to) {
    return value;
  }
  const empty = emptyValueFor(to, type);
  const shaped = (op: FilterOp) => op === "exists" || op === "within_radius";
  if (type === "boolean" || shaped(from) || shaped(to)) {
    return empty;
  }
  const numeric = type === "number" || type === "currency";
  if (to === "in") {
    return listOf(value, numeric) ?? empty;
  }
  const one = from === "in" ? soleItem(value, numeric) : value;
  return fits(to, one) ? one : empty;
}

function listOf(value: LeafValue, numeric: boolean): LeafValue | undefined {
  if (typeof value === "number") {
    return [value];
  }
  return typeof value === "string" && value !== "" && !numeric
    ? [value]
    : undefined;
}

function soleItem(value: LeafValue, numeric: boolean): LeafValue | undefined {
  if (!Array.isArray(value) || value.length !== 1) {
    return undefined;
  }
  const [item] = value;
  if (typeof item === "number") {
    return item;
  }
  return typeof item === "string" && !numeric ? item : undefined;
}

function fits(op: FilterOp, one: LeafValue | undefined): one is LeafValue {
  if (op === "contains") {
    return typeof one === "string";
  }
  return (
    typeof one === "string" || typeof one === "number" || isRelativeDate(one)
  );
}
