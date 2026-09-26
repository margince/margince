// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The words and rules an Analytics question is written in: what each
// population, field and aggregate is called on screen, which field an
// aggregate accepts, what shape a filter value takes, and how a question
// being edited becomes the query the engine reads.
//
// Pure functions over the schema's own names, so every rule here is tested
// without a render and the builder only draws what these decide.

import type { components } from "../api/schema";
import { isMessageKey, type useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { Reference } from "./filterreference";
import type { FilterOp } from "./segmentpredicate";

export type AnalyticsQuery = components["schemas"]["AnalyticsQuery"];
export type AnalyticsAnswer = components["schemas"]["AnalyticsAnswer"];
export type AnalyticsEntity = components["schemas"]["AnalyticsEntity"];
type AnalyticsMeasure = components["schemas"]["AnalyticsMeasure"];
type AnalyticsFilter = components["schemas"]["AnalyticsFilter"];
export type MeasureFn = AnalyticsMeasure["fn"];
export type QuestionOp = AnalyticsFilter["op"];
export type AnswerRow = AnalyticsAnswer["rows"][number];
type Translate = ReturnType<typeof useT>;

/**
 * A population's name on screen: the title of the report it is derived from.
 *
 * The report cards read the same map, so a population and the card drawing it
 * cannot come to have two names. A backend gate holds its keys against the
 * report catalog in both directions, which is why it stays a plain literal.
 */
export const ENTITY_LABEL_KEY = {
  "pipeline-current": "analytics.reportDeals",
  "deals-by-stage": "analytics.reportDealsByStage",
  forecast: "analytics.reportForecast",
  "open-deals-per-company": "analytics.reportOpenByCompany",
  "win-loss": "analytics.reportWinLoss",
  "stage-age": "analytics.reportStageAge",
  "leads-by-status": "analytics.reportLeadsByStatus",
  "activities-by-kind": "analytics.reportActivitiesByKind",
  "meeting-conversion": "analytics.reportMeetingConversion",
  "projects-by-phase": "analytics.reportProjectsByPhase",
  "project-commitments": "analytics.reportProjectCommitments",
  "projects-gone-quiet": "analytics.reportProjectsGoneQuiet",
} as const satisfies Record<string, MessageKey>;

type EntityName = keyof typeof ENTITY_LABEL_KEY;

function isEntityName(name: string): name is EntityName {
  return Object.hasOwn(ENTITY_LABEL_KEY, name);
}

/**
 * The money measures kept in each deal's OWN currency. Every other `_minor`
 * measure was converted into the base currency before it was summed.
 * Mirrors nativeMoney() in backend/internal/compose/reportnativemoney.go;
 * backend/gates/analyticsvocabularylabels_test.go holds the two equal.
 */
export const NATIVE_CURRENCY_MEASURES = [
  "amount_minor",
  "weighted_amount_minor",
] as const;

// How many groups a question asks for. Sent rather than defaulted, so "only
// the first N are shown" states the number the server actually applied.
export const QUESTION_LIMIT = 100;

export const MEASURE_FNS: readonly MeasureFn[] = [
  "count",
  "count_distinct",
  "sum",
  "avg",
  "min",
  "max",
  "median",
  "p75",
];

const FN_LABEL_KEY: Readonly<Record<MeasureFn, MessageKey>> = {
  count: "analytics.fn.count",
  count_distinct: "analytics.fn.count_distinct",
  sum: "analytics.fn.sum",
  avg: "analytics.fn.avg",
  min: "analytics.fn.min",
  max: "analytics.fn.max",
  median: "analytics.fn.median",
  p75: "analytics.fn.p75",
};

export const QUESTION_OPS: readonly QuestionOp[] = [
  "eq",
  "ne",
  "gt",
  "gte",
  "lt",
  "lte",
  "is_null",
  "is_not_null",
];

// The Filters builder's own words for the same comparisons, so "is at least"
// reads the same on both screens.
const OP_LABEL_KEY: Readonly<Record<QuestionOp, MessageKey>> = {
  eq: "filters.op.eq",
  ne: "filters.op.neq",
  gt: "filters.op.moreThan",
  gte: "filters.op.atLeast",
  lt: "filters.op.lessThan",
  lte: "filters.op.atMost",
  is_null: "filters.isEmpty",
  is_not_null: "filters.op.exists",
};

// The engine's two null tests carry no value at all; any other op carries one.
const VALUELESS_OPS: ReadonlySet<QuestionOp> = new Set([
  "is_null",
  "is_not_null",
]);

export function takesValue(op: QuestionOp): boolean {
  return !VALUELESS_OPS.has(op);
}

// The Filters builder's value control speaks its own operator names; only
// `ne` is spelled differently, and the null tests draw no control at all.
export function valueControlOp(op: QuestionOp): FilterOp | null {
  switch (op) {
    case "is_null":
    case "is_not_null":
      return null;
    case "ne":
      return "neq";
    default:
      return op;
  }
}

export function entityLabel(t: Translate, name: string): string {
  return isEntityName(name) ? t(ENTITY_LABEL_KEY[name]) : name;
}

/** A field's name on screen, or its wire name when the catalog has none. */
export function analyticsFieldLabel(t: Translate, name: string): string {
  const key = `analytics.field.${name}`;
  return isMessageKey(key) ? t(key) : name;
}

export function fnLabel(t: Translate, fn: MeasureFn): string {
  return t(FN_LABEL_KEY[fn]);
}

export function opLabel(t: Translate, op: QuestionOp): string {
  return t(OP_LABEL_KEY[op]);
}

// Summing a stage name means nothing, so these four take a measure; counting
// distinct values, or finding the lowest and highest, works on any field.
const NEEDS_MEASURE: ReadonlySet<MeasureFn> = new Set([
  "sum",
  "avg",
  "median",
  "p75",
]);

/** The fields an aggregate accepts on this population. `count` takes none. */
export function fieldsForFn(entity: AnalyticsEntity, fn: MeasureFn): string[] {
  if (fn === "count") {
    return [];
  }
  if (NEEDS_MEASURE.has(fn)) {
    return [...entity.measures];
  }
  return filterFields(entity);
}

/** The aggregates worth offering: `count`, and any other with a field to take. */
export function fnsFor(entity: AnalyticsEntity): MeasureFn[] {
  return MEASURE_FNS.filter(
    (fn) => fn === "count" || fieldsForFn(entity, fn).length > 0,
  );
}

/** Every field a filter may name: the dimensions, then the measures. */
export function filterFields(entity: AnalyticsEntity): string[] {
  return [...new Set([...entity.group_by, ...entity.measures])];
}

// The two dimensions whose column is not text, as the engine's schema declares
// them. A measure is always a number.
const NUMBER_DIMENSIONS: ReadonlySet<string> = new Set(["win_probability"]);
const BOOLEAN_DIMENSIONS: ReadonlySet<string> = new Set(["became_opportunity"]);

export type ValueShape = "number" | "boolean" | "text";

export function valueShape(entity: AnalyticsEntity, field: string): ValueShape {
  if (entity.measures.includes(field) || NUMBER_DIMENSIONS.has(field)) {
    return "number";
  }
  return BOOLEAN_DIMENSIONS.has(field) ? "boolean" : "text";
}

// The record type an id field points at, in the Filters vocabulary, so a value
// is picked from the same list and named by the same lookup there and here.
const FIELD_REFERENCE: Readonly<Record<string, Reference>> = {
  owner_id: "app_user",
  host_user_id: "app_user",
  stage_id: "stage",
  pipeline_id: "pipeline",
  project_id: "project",
  company_id: "company",
  partner_company_id: "company",
};

export function fieldReference(field: string): Reference | undefined {
  return Object.hasOwn(FIELD_REFERENCE, field)
    ? FIELD_REFERENCE[field]
    : undefined;
}

// --- The answer -------------------------------------------------------------

/** The column the engine names a measure by when the question gave it no alias. */
function measureAlias(m: AnalyticsMeasure): string {
  return m.as ?? (m.field ? `${m.fn}_${m.field}` : m.fn);
}

export function measureOfColumn(
  query: AnalyticsQuery,
  column: string,
): AnalyticsMeasure | undefined {
  return query.measures.find((m) => measureAlias(m) === column);
}

/** A measure's name on screen: the aggregate, and the field it reads. */
export function measureLabel(t: Translate, measure: AnalyticsMeasure): string {
  if (!measure.field) {
    return fnLabel(t, measure.fn);
  }
  return t("analytics.q.measureOf", {
    fn: fnLabel(t, measure.fn),
    field: analyticsFieldLabel(t, measure.field),
  });
}

export function columnLabel(
  t: Translate,
  query: AnalyticsQuery,
  column: string,
): string {
  const measure = measureOfColumn(query, column);
  return measure ? measureLabel(t, measure) : analyticsFieldLabel(t, column);
}

export function isMoneyField(field: string): boolean {
  return field.endsWith("_minor");
}

function isNativeMoney(field: string): boolean {
  return NATIVE_CURRENCY_MEASURES.some((native) => native === field);
}

// The one currency a question's filters pin every record to, if any.
function filteredCurrency(query: AnalyticsQuery): string | null {
  const pinned = (query.filters ?? []).find(
    (f) => f.field === "currency" && f.op === "eq",
  );
  return typeof pinned?.value === "string" && pinned.value !== ""
    ? pinned.value
    : null;
}

/**
 * How one measure cell reads: a plain number, an amount in one currency, or
 * an amount over several currencies that no single figure can state.
 */
export type MeasureReading =
  | Readonly<{ kind: "number" }>
  | Readonly<{ kind: "money"; currency: string }>
  | Readonly<{ kind: "mixed" }>;

export function measureReading(
  measure: AnalyticsMeasure,
  row: AnswerRow,
  query: AnalyticsQuery,
  baseCurrency: string | null,
): MeasureReading {
  const field = measure.field ?? "";
  if (measure.fn === "count_distinct" || !isMoneyField(field)) {
    return { kind: "number" };
  }
  const currency = moneyCurrency(field, row, query, baseCurrency);
  return currency ? { kind: "money", currency } : { kind: "mixed" };
}

/**
 * The currency a money field's value on this row is in, null when no single
 * one applies, and undefined for a field that is not money at all. A native
 * amount is in the row's own currency, or the one a filter pinned; a
 * converted one is in the base currency.
 */
export function moneyCurrency(
  field: string,
  row: AnswerRow,
  query: AnalyticsQuery,
  baseCurrency: string | null,
): string | null | undefined {
  if (!isMoneyField(field)) {
    return undefined;
  }
  if (!isNativeMoney(field)) {
    return baseCurrency;
  }
  return rowText(row, "currency") ?? filteredCurrency(query);
}

function rowText(row: AnswerRow, column: string): string | null {
  const value = row[column];
  return typeof value === "string" && value !== "" ? value : null;
}

export function isWithheldRow(row: AnswerRow): boolean {
  return row._withheld === true;
}

/**
 * One row's cell, named by its group key values in the question's own
 * grouping order. A null entry is the group whose value is unset, which the
 * engine resolves to exactly those records. An ungrouped answer has one cell
 * and names none.
 */
export function explainGroup(
  query: AnalyticsQuery,
  row: AnswerRow,
): unknown[] | undefined {
  const groupBy = query.group_by ?? [];
  if (groupBy.length === 0) {
    return undefined;
  }
  return groupBy.map((field) => row[field] ?? null);
}

// --- A refusal ----------------------------------------------------------------

export type Refusal = Readonly<{
  kind: string;
  suggest: string;
  message: string | null;
}>;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}

// "<kind>: <message> — <suggest>" is the detail's wire shape; the message is
// the middle, which the structured details do not repeat.
function refusalMessage(detail: unknown, kind: string, suggest: string) {
  if (typeof detail !== "string") {
    return null;
  }
  let message = detail.startsWith(`${kind}: `)
    ? detail.slice(kind.length + 2)
    : detail;
  if (suggest !== "" && message.endsWith(` — ${suggest}`)) {
    message = message.slice(0, -(suggest.length + 3));
  }
  return message.trim() === "" ? null : message;
}

/**
 * The engine's refusal, read off a problem body: why the question was not
 * answered and the smallest change that would have worked. Null for any
 * other failure, which reads as an ordinary error.
 */
export function refusalOf(problem: unknown): Refusal | null {
  if (!isRecord(problem) || !isRecord(problem.details)) {
    return null;
  }
  const { kind, suggest, message } = problem.details;
  if (typeof kind !== "string" || kind === "") {
    return null;
  }
  const suggestion = typeof suggest === "string" ? suggest : "";
  return {
    kind,
    suggest: suggestion,
    message:
      typeof message === "string" && message !== ""
        ? message
        : refusalMessage(problem.detail, kind, suggestion),
  };
}
