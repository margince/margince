// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The question while it is being edited, and the two ways across the wire: a
// draft becomes the query the engine reads, and a saved query becomes a draft
// again. Rows carry ids so the builder keeps each row's focus while it edits.

import { toMajorUnits, toMinorUnits } from "../format/minorunits";
import type { MessageKey } from "../i18n/en";
import {
  type AnalyticsEntity,
  type AnalyticsQuery,
  fieldsForFn,
  filterAmountCurrency,
  filterFields,
  isNativeMoneyField,
  type MeasureFn,
  QUESTION_LIMIT,
  type QuestionOp,
  takesValue,
} from "./analytics.questions.vocab";
import type { LeafValue } from "./segmentpredicate";

export type MeasureDraft = Readonly<{
  id: number;
  fn: MeasureFn;
  field: string;
}>;
export type FilterDraft = Readonly<{
  id: number;
  field: string;
  op: QuestionOp;
  value: LeafValue;
}>;
export type QuestionDraft = Readonly<{
  entity: string;
  groupBy: readonly string[];
  measures: readonly MeasureDraft[];
  filters: readonly FilterDraft[];
}>;

export function newDraft(entity: string): QuestionDraft {
  return {
    entity,
    groupBy: [],
    measures: [{ id: 1, fn: "count", field: "" }],
    filters: [],
  };
}

/** An id for a new row, unique within this draft, so React keeps each row's focus. */
export function nextRowId(draft: QuestionDraft): number {
  const ids = [...draft.measures, ...draft.filters].map((row) => row.id);
  return Math.max(0, ...ids) + 1;
}

/**
 * The draft moved onto another population: whatever still exists there is
 * kept, whatever does not is dropped rather than sent to be refused.
 */
export function retarget(
  draft: QuestionDraft,
  entity: AnalyticsEntity,
): QuestionDraft {
  const measures = draft.measures.filter(
    (measure) =>
      measure.fn === "count" ||
      measure.field === "" ||
      fieldsForFn(entity, measure.fn).includes(measure.field),
  );
  const fields = filterFields(entity);
  return {
    entity: entity.name,
    groupBy: inOptionOrder(draft.groupBy, entity),
    measures: measures.length > 0 ? measures : newDraft(entity.name).measures,
    filters: draft.filters.filter((filter) => fields.includes(filter.field)),
  };
}

/**
 * A grouping in the report's own field order, which is the order the picker
 * lists it in. The question groups in that order too, so the picker's face,
 * the request and the answer's columns all read the same way.
 */
export function inOptionOrder(
  groupBy: readonly string[],
  entity: AnalyticsEntity,
): string[] {
  return entity.group_by.filter((field) => groupBy.includes(field));
}

function valueMissing(value: LeafValue): boolean {
  return value === "" || (Array.isArray(value) && value.length === 0);
}

/**
 * Why this draft cannot be asked yet, or null when it can. A reason rather
 * than a silent disabled button, and never a field filled in for the reader.
 */
export function draftProblem(
  draft: QuestionDraft,
  baseCurrency: string | null,
): MessageKey | null {
  if (draft.entity === "") {
    return "analytics.q.needEntity";
  }
  if (draft.measures.some((m) => m.fn !== "count" && m.field === "")) {
    return "analytics.q.needField";
  }
  if (draft.filters.some((f) => f.field === "")) {
    return "analytics.q.needFilterField";
  }
  if (draft.filters.some((f) => takesValue(f.op) && valueMissing(f.value))) {
    return "analytics.q.needValue";
  }
  return amountProblem(draft, baseCurrency);
}

// A money filter compares an amount the reader typed in major units, so it
// needs a currency to scale by and a figure that currency can hold.
function amountProblem(
  draft: QuestionDraft,
  baseCurrency: string | null,
): MessageKey | null {
  for (const filter of draft.filters) {
    const currency = filterAmountCurrency(
      filter.field,
      draft.filters,
      baseCurrency,
    );
    if (currency === undefined || !takesValue(filter.op)) {
      continue;
    }
    if (currency === null) {
      return isNativeMoneyField(filter.field)
        ? "analytics.q.needCurrencyFilter"
        : "analytics.noBaseCurrencyWhy";
    }
    if (Number.isNaN(minorAmount(filter.value, currency))) {
      return "analytics.q.amountInvalid";
    }
  }
  return null;
}

// The typed amount in the currency's minor units, or NaN when it is not a
// number that currency can hold exactly.
function minorAmount(value: LeafValue, currency: string): number {
  return typeof value === "number" ? toMinorUnits(value, currency) : Number.NaN;
}

type ScopeWire = Readonly<{
  scope_kind?: "workspace" | "team" | "owner";
  scope_id?: string;
}>;

/** The draft as the engine reads it, over the population the page measures. */
export function toQuery(
  draft: QuestionDraft,
  scope: ScopeWire,
  baseCurrency: string | null,
): AnalyticsQuery {
  const query: AnalyticsQuery = {
    entity: draft.entity,
    ...scope,
    measures: draft.measures.map((m) =>
      m.fn === "count" ? { fn: "count" } : { fn: m.fn, field: m.field },
    ),
    limit: QUESTION_LIMIT,
  };
  if (draft.groupBy.length > 0) {
    query.group_by = [...draft.groupBy];
  }
  if (draft.filters.length > 0) {
    query.filters = draft.filters.map((f) =>
      takesValue(f.op)
        ? { field: f.field, op: f.op, value: wireValue(f, draft, baseCurrency) }
        : { field: f.field, op: f.op },
    );
  }
  return query;
}

// A money amount leaves in minor units; every other value as typed.
function wireValue(
  filter: FilterDraft,
  draft: QuestionDraft,
  baseCurrency: string | null,
): LeafValue {
  const currency = filterAmountCurrency(
    filter.field,
    draft.filters,
    baseCurrency,
  );
  if (!currency) {
    return filter.value;
  }
  const minor = minorAmount(filter.value, currency);
  return Number.isNaN(minor) ? filter.value : minor;
}

// A saved filter value arrives as JSON; the value control holds one of these.
function leafValue(value: unknown): LeafValue {
  if (
    typeof value === "string" ||
    typeof value === "number" ||
    typeof value === "boolean"
  ) {
    return value;
  }
  return value == null ? "" : JSON.stringify(value);
}

/**
 * A saved question back in the builder, so it can be changed and asked
 * again. A money amount returns to the major units the reader types.
 */
export function draftFromQuery(
  query: AnalyticsQuery,
  baseCurrency: string | null,
): QuestionDraft {
  const filters = query.filters ?? [];
  let id = 0;
  return {
    entity: query.entity,
    groupBy: query.group_by ?? [],
    measures: query.measures.map((m) => ({
      id: ++id,
      fn: m.fn,
      field: m.field ?? "",
    })),
    filters: filters.map((f) => {
      const currency = filterAmountCurrency(f.field, filters, baseCurrency);
      return {
        id: ++id,
        field: f.field,
        op: f.op,
        value:
          currency && typeof f.value === "number"
            ? toMajorUnits(f.value, currency)
            : leafValue(f.value),
      };
    }),
  };
}
