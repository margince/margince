// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a filter reads as words: the operator a picker offers, one clause, and a
// whole tree as one sentence. The builder's operator picker, a Live List's
// verdicts and the list page's Filter line all read through this module, so a
// reader meets one wording for one filter wherever it appears.
// filtersentence.census.test.ts fails any other module spelling an operator.

import { formatDateAbbrev, formatMoney, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import {
  type Locale,
  type PluralBase,
  type PluralTranslator,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../i18n";
import type { MessageKey } from "../i18n/en";
import { fieldLabel, type VocabularyField } from "./filterdata";
import {
  type FilterOp,
  isGroup,
  isRelativeDate,
  type Leaf,
  type Node,
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

export function operatorKey(
  op: FilterOp,
  type: VocabularyField["type"],
): MessageKey {
  if (type === "number" || type === "currency") {
    return NUMERIC_OPERATOR_KEY[op] ?? OPERATOR_KEY[op];
  }
  return OPERATOR_KEY[op];
}

type ReferenceKind = NonNullable<VocabularyField["references"]>;

const REFERENCE_NOUN: Record<ReferenceKind, PluralBase> = {
  stage: "filters.sentence.ref.stage",
  pipeline: "filters.sentence.ref.pipeline",
  app_user: "filters.sentence.ref.app_user",
  team: "filters.sentence.ref.team",
  company: "filters.sentence.ref.company",
  tag: "filters.sentence.ref.tag",
  project: "filters.sentence.ref.project",
};

const CALENDAR_DAY = /^\d{4}-\d{2}-\d{2}$/;
const WHOLE_OR_DECIMAL = /^-?\d+(\.\d+)?$/;

/** What a sentence is said with: the reader's catalog, plural rule and zone. */
export type SentenceWords = Readonly<{
  t: Translator;
  plural: PluralTranslator;
  locale: Locale;
  zone: string;
}>;

export function useSentenceWords(): SentenceWords {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return { t, plural, locale, zone: viewerZone() };
}

/** One clause as a filter states it: a stored leaf, or a verdict's clause. */
export type Clause = Readonly<{
  field: string;
  op: FilterOp;
  operand: unknown;
}>;

/**
 * A clause in words. No operand where the operator says it all, and neither
 * for a retired field, whose type no longer says how to read either.
 */
export type ClauseWords = Readonly<{
  field: string;
  op?: string;
  operand?: string;
}>;

/**
 * One clause in the reader's words.
 *
 * `fields` is the record type's vocabulary, undefined until it has answered: a
 * field it lacks is then a retired one, while a field read before it answers
 * keeps its own name rather than being called retired too early. A value not
 * given yet reads as a gap to fill, not as a finished clause.
 */
export function clauseWords(
  clause: Clause,
  fields: readonly VocabularyField[] | undefined,
  words: SentenceWords,
): ClauseWords {
  const { t } = words;
  const known = fields?.find((candidate) => candidate.name === clause.field);
  if (fields !== undefined && known === undefined) {
    return { field: t("filters.sentence.retiredField") };
  }
  const field = known ? fieldLabel(known, t) : clause.field;
  const { op, operand } = clause;
  if (op === "exists") {
    return {
      field,
      op: t(operand === false ? "filters.isEmpty" : "filters.hasValue"),
    };
  }
  const relative = relativeDateWords(op, operand, words);
  if (relative) {
    return { field, ...relative };
  }
  const text = operandText(operand, known, words);
  return {
    field,
    op: t(clauseOperatorKey(clause, known)),
    operand: text === "" ? t("filters.sentence.pendingValue") : text,
  };
}

/**
 * The operator as this clause says it. One value of `in` reads "is", except
 * where ids are counted: "Stage is 1 stage" says the count is the stage. A
 * counted `in` has words of its own, since German „eines von“ would have to
 * agree with a noun only the count names.
 */
function clauseOperatorKey(
  { op, operand }: Clause,
  field: VocabularyField | undefined,
): MessageKey {
  if (op === "in" && field?.references) {
    return "filters.sentence.inCounted";
  }
  if (op === "in" && Array.isArray(operand) && operand.length === 1) {
    return "filters.op.eq";
  }
  return operatorKey(op, field?.type ?? "text");
}

/**
 * A count back from today, read as how long ago rather than as a comparison:
 * `lt 45 days ago` is a day further back than that, "more than 45 days ago",
 * and `gt` is a day since then, "within the last 45 days", one phrase so that
 * one day reads "within the last day". Its own words, not the operator's:
 * German says „liegt mehr als 45 Tage zurück“, not „größer als“.
 */
function relativeDateWords(
  op: FilterOp,
  operand: unknown,
  { t, plural, locale }: SentenceWords,
): Omit<ClauseWords, "field"> | undefined {
  if (!isRelativeDate(operand)) {
    return undefined;
  }
  const days = operand.days_ago;
  const count = { count: formatNumber(days, locale) };
  if (op === "lt" || op === "lte") {
    return {
      op: t(
        op === "lt"
          ? "filters.sentence.moreThanAgo"
          : "filters.sentence.atLeastAgo",
      ),
      operand: plural("filters.sentence.daysAgo", days, count),
    };
  }
  if (op === "gt" || op === "gte") {
    return { op: plural("filters.sentence.withinLast", days, count) };
  }
  return undefined;
}

/** A clause's operand as the filter states it, or "" where it states none yet. */
function operandText(
  operand: unknown,
  field: VocabularyField | undefined,
  words: SentenceWords,
): string {
  if (Array.isArray(operand) && operand.length === 0) {
    return "";
  }
  const typed = field && typedOperandText(operand, field, words);
  if (typed !== undefined) {
    return typed;
  }
  if (Array.isArray(operand)) {
    return operand.map((one) => scalarText(one, field, words)).join(", ");
  }
  if (isRelativeDate(operand)) {
    return words.plural("lists.why.daysAgo", operand.days_ago, {
      count: formatNumber(operand.days_ago, words.locale),
    });
  }
  return scalarText(operand, field, words);
}

/** One value as the reader reads its kind: a quantity grouped, a day by name. */
function scalarText(
  operand: unknown,
  field: VocabularyField | undefined,
  { t, locale, zone }: SentenceWords,
): string {
  if (typeof operand === "boolean") {
    return t(operand ? "filters.yes" : "filters.no");
  }
  if (typeof operand === "number") {
    return formatNumber(operand, locale);
  }
  if (typeof operand !== "string") {
    return String(operand ?? "");
  }
  if (field?.type === "number" && WHOLE_OR_DECIMAL.test(operand)) {
    return formatNumber(Number(operand), locale);
  }
  const day =
    CALENDAR_DAY.test(operand) &&
    (field === undefined || field.type === "date");
  return day ? formatDateAbbrev(operand, locale, zone) : operand;
}

/**
 * An operand only its field's type can read: money in the field's currency,
 * and ids counted in their record's noun, since an id names nothing to a reader.
 * Money whose currency this reader may not learn states none, since its minor
 * units would read as the amount: "125,000" for €1,250.
 */
function typedOperandText(
  operand: unknown,
  field: VocabularyField,
  { plural, locale }: SentenceWords,
): string | undefined {
  if (field.type === "currency") {
    return field.currency ? moneyText(operand, field.currency, locale) : "";
  }
  // An id not chosen yet is no id to count, so it reads as a gap to fill.
  if (!field.references || operand === "") {
    return undefined;
  }
  const ids = Array.isArray(operand) ? operand.length : 1;
  return plural(REFERENCE_NOUN[field.references], ids, {
    count: formatNumber(ids, locale),
  });
}

/**
 * A money operand or value — minor units of the field's currency — as the
 * reader reads money, or undefined for anything that is not an amount.
 */
export function moneyText(
  raw: unknown,
  currency: string,
  locale: Locale,
): string | undefined {
  const one = (v: unknown) =>
    typeof v === "number" || (typeof v === "string" && /^-?\d+$/.test(v))
      ? formatMoney(Number(v), currency, locale)
      : undefined;
  if (Array.isArray(raw)) {
    const each = raw.map(one);
    return each.every((v) => v !== undefined) ? each.join(", ") : undefined;
  }
  return one(raw);
}

/**
 * A whole filter as one sentence: clauses joined the way their group joins
 * them, a nested group in brackets. Until the vocabulary answers, the tree is
 * only counted, because a sentence of wire names would be read as the filter.
 */
export function filterSentence(
  tree: Node,
  fields: readonly VocabularyField[] | undefined,
  words: SentenceWords,
): string {
  if (isGroup(tree) && tree.children.length === 0) {
    return "";
  }
  if (fields === undefined) {
    const count = leafCount(tree);
    return words.plural("filters.sentence.conditions", count, {
      count: formatNumber(count, words.locale),
    });
  }
  return nodeSentence(tree, fields, words);
}

function nodeSentence(
  node: Node,
  fields: readonly VocabularyField[],
  words: SentenceWords,
): string {
  const { t } = words;
  if (!isGroup(node)) {
    return clauseSentence(node, fields, words);
  }
  const join = ` ${t(node.join === "and" ? "filters.join.and" : "filters.join.or")} `;
  return node.children
    .map((child) => {
      if (!isGroup(child)) {
        return clauseSentence(child, fields, words);
      }
      return child.children.length === 0
        ? t("filters.sentence.emptyGroup")
        : t("filters.sentence.group", {
            clauses: nodeSentence(child, fields, words),
          });
    })
    .join(join);
}

function clauseSentence(
  leaf: Leaf,
  fields: readonly VocabularyField[],
  words: SentenceWords,
): string {
  const said = clauseWords(
    { field: leaf.field, op: leaf.op, operand: leaf.value },
    fields,
    words,
  );
  if (said.op === undefined) {
    return said.field;
  }
  return said.operand === undefined
    ? words.t("filters.sentence.clauseBare", { field: said.field, op: said.op })
    : words.t("filters.sentence.clause", {
        field: said.field,
        op: said.op,
        value: said.operand,
      });
}

function leafCount(node: Node): number {
  return isGroup(node)
    ? node.children.reduce((sum, child) => sum + leafCount(child), 0)
    : 1;
}
