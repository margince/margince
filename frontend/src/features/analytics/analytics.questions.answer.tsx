// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EmptyState } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { QueryGate } from "../../design-system/querystates";
import { formatAnalyticsNumber, formatDateTime } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useCompanyZone } from "../../lib/companyzone";
import { ExcludedNote } from "./analytics.explain";
import { measureCell } from "./analytics.questions.cell";
import {
  isMoneyField,
  isNativeMoneyField,
  type MoneyFields,
  measureReading,
  moneyFieldsOf,
} from "./analytics.questions.money";
import { type Namer, useValueNamer } from "./analytics.questions.names";
import { RowExplain } from "./analytics.questions.rowexplain";
import {
  type AnalyticsAnswer,
  type AnalyticsQuery,
  type AnswerRow,
  type CustomLabels,
  columnLabel,
  isWithheldRow,
  measureOfColumn,
  QUESTION_LIMIT,
} from "./analytics.questions.vocab";
import { useAnalyticsSchema } from "./analytics.schema";

const NO_LABELS: CustomLabels = new Map();

/** Where a row's records are read from: the question itself, or a saved run. */
export type ExplainSource =
  | Readonly<{ kind: "query"; query: AnalyticsQuery }>
  | Readonly<{ kind: "run"; runId: string }>;

// Whether any shown cell reads the way asked: an amount summed across
// currencies, or a converted one with no base currency to state it in.
function anyReading(
  kind: "mixed" | "noBase",
  query: AnalyticsQuery,
  answer: AnalyticsAnswer,
  baseCurrency: string | null,
  money: MoneyFields,
): boolean {
  return answer.rows.some(
    (row) =>
      !isWithheldRow(row) &&
      answer.columns.some((column) => {
        const measure = measureOfColumn(query, column);
        return (
          measure !== undefined &&
          measureReading(measure, row, query, baseCurrency, money).kind === kind
        );
      }),
  );
}

type CellFrame = Readonly<{
  query: AnalyticsQuery;
  namer: Namer;
  source?: ExplainSource;
  baseCurrency: string | null;
  money: MoneyFields;
  withheldGroups: number;
  fieldLabels: CustomLabels;
}>;

// One cell. The row's first cell also carries the way into its records; the
// closing withheld row counts the groups there, with nothing beside it.
function AnswerCell({
  frame,
  row,
  column,
  first,
}: Readonly<{
  frame: CellFrame;
  row: AnswerRow;
  column: string;
  first: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const { query, namer, baseCurrency, money, withheldGroups } = frame;
  if (isWithheldRow(row)) {
    return first
      ? plural("analytics.q.withheldGroups", withheldGroups, {
          count: formatAnalyticsNumber(withheldGroups, locale),
        })
      : null;
  }
  const cell = (query.group_by ?? []).includes(column)
    ? namer(column, row[column])
    : measureCell(query, column, row, baseCurrency, money, locale, t);
  if (!first || frame.source === undefined) {
    return <>{cell}</>;
  }
  return (
    <RowExplain frame={frame} source={frame.source} row={row}>
      {cell}
    </RowExplain>
  );
}

/**
 * The answer as a table: the group keys, then each measure, one row per
 * group. Withheld groups are counted in ONE closing row rather than printed
 * as zeros or dropped: each is null in every column, so a row apiece repeats
 * one sentence, and how many there are is already on the wire.
 */
export function AnswerTable(props: AnswerProps) {
  const t = useT();
  const schema = useAnalyticsSchema();
  // The schema says which measures are amounts, so no cell is drawn before it
  // arrives. A raw minor-unit figure reads as a hundred times the money.
  return (
    <QueryGate query={schema} pendingLabel={t("common.loading")}>
      {(vocabulary) => (
        <MoneyAnswerTable
          {...props}
          money={moneyFieldsOf(vocabulary.entities, props.query.entity)}
        />
      )}
    </QueryGate>
  );
}

type AnswerProps = Readonly<{
  query: AnalyticsQuery;
  answer: AnalyticsAnswer;
  baseCurrency: string | null;
  // Absent for an answer kept from the past: its records are today's, so a
  // cell offers no way into them.
  source?: ExplainSource;
  /** The administrator's names for the installation's own fields. */
  fieldLabels?: CustomLabels;
}>;

function MoneyAnswerTable({
  query,
  answer,
  baseCurrency,
  money,
  source,
  fieldLabels = NO_LABELS,
}: AnswerProps & Readonly<{ money: MoneyFields }>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const zone = useCompanyZone();
  const answerCurrency = answer.base_currency ?? baseCurrency;
  const namer = useValueNamer(
    query.entity,
    query.group_by ?? [],
    answer.labels,
  );
  const shown = answer.rows.filter((row) => !isWithheldRow(row));
  const withheldGroups = answer.rows.length - shown.length;
  const closing: AnswerRow = { _withheld: true };
  const rows = withheldGroups > 0 ? [...shown, closing] : shown;
  const frame: CellFrame = {
    query,
    namer,
    source,
    baseCurrency: answerCurrency,
    money,
    withheldGroups,
    fieldLabels,
  };
  const position = new Map(rows.map((row, index) => [row, index]));
  const limit = query.limit ?? QUESTION_LIMIT;
  return (
    <>
      {query.measures.some(
        (measure) =>
          measure.field &&
          isMoneyField(measure.field, money) &&
          !isNativeMoneyField(measure.field, money),
      ) &&
        answer.base_currency &&
        answer.rate_date && (
          <p className="t-caption">
            {t("analytics.q.conversionFrame", {
              currency: answer.base_currency,
              date: formatDateTime(answer.rate_date, locale, zone),
            })}
          </p>
        )}
      <ExcludedNote count={answer.excluded_by_permission} />
      {answer.withheld && (
        <Callout
          tone="info"
          kind="standing"
          title={t("analytics.q.withheldTitle")}
        >
          <p>{t("analytics.q.withheldBody")}</p>
          {answer.floor !== undefined && (
            <p>
              {plural("analytics.q.withheldFloor", answer.floor, {
                count: formatAnalyticsNumber(answer.floor, locale),
              })}
            </p>
          )}
        </Callout>
      )}
      {anyReading("mixed", query, answer, answerCurrency, money) && (
        <Callout
          tone="info"
          kind="standing"
          title={t("analytics.q.mixedTitle")}
        >
          {t("analytics.q.mixedBody")}
        </Callout>
      )}
      {anyReading("noBase", query, answer, answerCurrency, money) && (
        <Callout
          tone="info"
          kind="standing"
          title={t("analytics.noBaseCurrencyWhy")}
        />
      )}
      {answer.rows.length === 0 ? (
        <EmptyState>{t("analytics.q.empty")}</EmptyState>
      ) : (
        <DataTable
          label={t("analytics.q.answerTitle")}
          columns={answer.columns.map((column, index) => ({
            key: column,
            header: columnLabel(t, query, column, fieldLabels),
            render: (row: AnswerRow) => (
              <AnswerCell
                frame={frame}
                row={row}
                column={column}
                first={index === 0}
              />
            ),
          }))}
          rows={rows}
          rowKey={(row) => `row:${position.get(row)}`}
        />
      )}
      {answer.rows.length >= limit && (
        <p className="t-caption">
          {plural("analytics.q.limited", limit, {
            count: formatAnalyticsNumber(limit, locale),
          })}
        </p>
      )}
    </>
  );
}
