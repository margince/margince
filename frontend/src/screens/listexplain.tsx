// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How a Live List's filter reads against one record: each clause judged by the
// same SQL that decides who is a member, with the record's own value beside it
// — or "hidden" where the reader may not see that value. The value is read the
// same way in a member table's filter columns, so the two never disagree.

import { Check, CircleHelp, X } from "lucide-react";
import {
  calendarDaysBetween,
  formatMoney,
  formatNumber,
} from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import { operatorKey } from "./filterbuilder";
import {
  type FilterVocabulary,
  fieldLabel,
  useFilterVocabulary,
  type VocabularyField,
} from "./filterdata";
import type {
  ListClauseVerdict,
  ListExplanation,
  ListRecordType,
} from "./lists.queries";
import { useMemberNames } from "./membernames";
import { type FilterOp, isRelativeDate } from "./segmentpredicate";

/** Whether a Live List selects the record, and each clause's verdict. */
export function LiveExplanation({
  entityType,
  why,
}: Readonly<{ entityType: ListRecordType; why: ListExplanation }>) {
  const t = useT();
  return (
    <div className="lists-why">
      <p className="lists-note">
        {why.member
          ? t("lists.why.liveMember")
          : why.eligible === false
            ? t("lists.why.notEligible")
            : t("lists.why.liveNotMember")}
      </p>
      {why.clauses && <Verdict node={why.clauses} entityType={entityType} />}
    </div>
  );
}

function VerdictMark({ result }: Readonly<{ result: boolean | null }>) {
  const t = useT();
  if (result === true) {
    return <Check aria-label={t("lists.why.met")} className="lists-why-met" />;
  }
  if (result === false) {
    // ds:ignore a verdict glyph drawn in the ink, not a failure line
    return <X aria-label={t("lists.why.unmet")} className="lists-why-unmet" />;
  }
  return (
    <CircleHelp
      aria-label={t("lists.why.unknown")}
      className="lists-why-unknown"
    />
  );
}

function Verdict({
  node,
  entityType,
}: Readonly<{ node: ListClauseVerdict; entityType: ListRecordType }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const vocabulary = useFilterVocabulary(entityType);
  const field = vocabularyField(vocabulary.data, node.field);
  const valueText = useFieldValueText(entityType, colleaguesIn(field, node));
  if (node.join) {
    return (
      <div className="lists-why-group">
        <p className="t-caption">
          <VerdictMark result={node.result} />
          {node.join === "and" ? t("lists.why.all") : t("lists.why.any")}
        </p>
        <ul className="lists-why-clauses">
          {(node.children ?? []).map((child, index) => (
            // Clauses have no identity of their own; their place is it.
            // biome-ignore lint/suspicious/noArrayIndexKey: a clause is its position in the tree
            <li key={index}>
              <Verdict node={child} entityType={entityType} />
            </li>
          ))}
        </ul>
      </div>
    );
  }
  const label = field ? fieldLabel(field, t) : (node.field ?? "");
  const op = t(
    operatorKey((node.op ?? "eq") as FilterOp, field?.type ?? "text"),
  );
  const operand =
    (field && moneyOf(field, node.operand, locale)) ??
    operandText(node.operand, t, plural, locale);
  return (
    <p className="lists-why-clause">
      <VerdictMark result={node.result} />
      <span>
        {label} {op} <strong>{operand}</strong>
      </span>
      <span className="t-caption">
        {node.hidden
          ? t("lists.why.hidden")
          : t("lists.why.value", {
              value:
                node.value == null
                  ? t("lists.why.empty")
                  : (node.value_label ??
                    valueText(node.field ?? "", node.value)),
            })}
      </span>
    </p>
  );
}

/**
 * How a filter field's value on a record reads, as the server states it in a
 * why or a member's `values`: money in the field's currency, a day as how long
 * ago it was, a yes/no as words, a user by name, anything else as given.
 * `userIds` are the colleagues the caller will ask about, named by id up front
 * because the returned function cannot call a hook per value.
 */
export function useFieldValueText(
  entityType: ListRecordType,
  userIds: readonly string[],
) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const vocabulary = useFilterVocabulary(entityType);
  const users = useMemberNames(userIds);
  return (name: string, raw: string): string => {
    const field = vocabularyField(vocabulary.data, name);
    switch (field?.type) {
      case "date":
        return dayText(raw, plural, t, locale) ?? raw;
      case "boolean":
        return raw === "true" ? t("filters.yes") : t("filters.no");
      case "currency":
        return moneyOf(field, raw, locale) ?? raw;
      default:
        return field?.references === "app_user" ? (users.get(raw) ?? raw) : raw;
    }
  };
}

// The colleague a clause's value names, when its field references one.
function colleaguesIn(
  field: VocabularyField | undefined,
  node: ListClauseVerdict,
): string[] {
  return field?.references === "app_user" && node.value ? [node.value] : [];
}

/**
 * The vocabulary's entry for a field. An answer without `fields` finds none
 * rather than taking the page down: the field then reads by its name.
 */
export function vocabularyField(
  vocabulary: FilterVocabulary | undefined,
  name: string | undefined,
): VocabularyField | undefined {
  return (vocabulary?.fields ?? []).find((f) => f.name === name);
}

function moneyOf(
  field: VocabularyField,
  raw: unknown,
  locale: Locale,
): string | undefined {
  return field.type === "currency" && field.currency
    ? moneyText(raw, field.currency, locale)
    : undefined;
}

/** A calendar day ("2026-08-10") as how far it lies from today. */
function dayText(
  raw: string,
  plural: ReturnType<typeof usePlural>,
  t: ReturnType<typeof useT>,
  locale: Locale,
): string | undefined {
  const day = new Date(`${raw.slice(0, 10)}T00:00:00Z`);
  if (Number.isNaN(day.getTime())) {
    return undefined;
  }
  const days = calendarDaysBetween(day, new Date());
  if (days === 0) {
    return t("lists.why.today");
  }
  const key = days > 0 ? "lists.why.daysAgo" : "lists.why.inDays";
  return plural(key, Math.abs(days), {
    count: formatNumber(Math.abs(days), locale),
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

/** A clause's operand as the filter states it. */
function operandText(
  operand: unknown,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): string {
  if (operand === true) {
    return t("filters.yes");
  }
  if (operand === false) {
    return t("filters.no");
  }
  if (Array.isArray(operand)) {
    return operand.join(", ");
  }
  if (isRelativeDate(operand)) {
    const days = operand.days_ago;
    return plural("lists.why.daysAgo", days, {
      count: formatNumber(days, locale),
    });
  }
  return String(operand ?? "");
}
