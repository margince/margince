// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { DataTable } from "../design-system/datatable";
import { ErrorLine } from "../design-system/errorline";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../format/format";
import { type Locale, useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { derivationColumns } from "./analytics.explain";
import { ExplainDrawer } from "./analytics.explain.drawer";
import {
  type Namer,
  shortId,
  useValueNamer,
} from "./analytics.questions.names";
import {
  type AnalyticsAnswer,
  type AnalyticsQuery,
  type AnswerRow,
  analyticsFieldLabel,
  columnLabel,
  explainGroup,
  isWithheldRow,
  measureOfColumn,
  measureReading,
  moneyCurrency,
  QUESTION_LIMIT,
  refusalOf,
} from "./analytics.questions.vocab";
import { ProblemError, throwProblem } from "./common";

type Translate = ReturnType<typeof useT>;

/** Where a row's records are read from: the question itself, or a saved run. */
export type ExplainSource =
  | Readonly<{ kind: "query"; query: AnalyticsQuery }>
  | Readonly<{ kind: "run"; runId: string }>;

function measureCell(
  query: AnalyticsQuery,
  column: string,
  row: AnswerRow,
  baseCurrency: string | null,
  locale: Locale,
  t: Translate,
): ReactNode {
  const measure = measureOfColumn(query, column);
  const value = row[column];
  if (!measure || typeof value !== "number") {
    return value == null ? MONEY_ABSENT : String(value);
  }
  const reading = measureReading(measure, row, query, baseCurrency);
  if (reading.kind === "mixed") {
    return t("analytics.q.mixedCurrencies");
  }
  return (
    <span className="t-num">
      {reading.kind === "money"
        ? formatMoneyOrAbsent(value, reading.currency, locale)
        : formatNumber(value, locale)}
    </span>
  );
}

// Whether any shown cell sums one currency's amounts with another's.
function hasMixedCurrency(
  query: AnalyticsQuery,
  answer: AnalyticsAnswer,
  baseCurrency: string | null,
): boolean {
  return answer.rows.some(
    (row) =>
      !isWithheldRow(row) &&
      answer.columns.some((column) => {
        const measure = measureOfColumn(query, column);
        return (
          measure !== undefined &&
          measureReading(measure, row, query, baseCurrency).kind === "mixed"
        );
      }),
  );
}

type CellFrame = Readonly<{
  query: AnalyticsQuery;
  namer: Namer;
  source: ExplainSource;
  baseCurrency: string | null;
  withheldGroups: number;
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
  const { query, namer, baseCurrency, withheldGroups } = frame;
  if (isWithheldRow(row)) {
    return first
      ? plural("analytics.q.withheldGroups", withheldGroups, {
          count: formatNumber(withheldGroups, locale),
        })
      : null;
  }
  const cell = (query.group_by ?? []).includes(column)
    ? namer(column, row[column])
    : measureCell(query, column, row, baseCurrency, locale, t);
  if (!first) {
    return <>{cell}</>;
  }
  return (
    <RowExplain frame={frame} row={row}>
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
export function AnswerTable({
  query,
  answer,
  baseCurrency,
  source,
}: Readonly<{
  query: AnalyticsQuery;
  answer: AnalyticsAnswer;
  baseCurrency: string | null;
  source: ExplainSource;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const namer = useValueNamer(query.entity, query.group_by ?? [], answer.rows);
  const shown = answer.rows.filter((row) => !isWithheldRow(row));
  const withheldGroups = answer.rows.length - shown.length;
  const closing: AnswerRow = { _withheld: true };
  const rows = withheldGroups > 0 ? [...shown, closing] : shown;
  const frame: CellFrame = {
    query,
    namer,
    source,
    baseCurrency,
    withheldGroups,
  };
  const position = new Map(rows.map((row, index) => [row, index]));
  const limit = query.limit ?? QUESTION_LIMIT;
  return (
    <>
      {answer.withheld && (
        <Callout
          tone="info"
          kind="standing"
          title={t("analytics.q.withheldTitle")}
        >
          {t("analytics.q.withheldBody")}
        </Callout>
      )}
      {hasMixedCurrency(query, answer, baseCurrency) && (
        <Callout
          tone="info"
          kind="standing"
          title={t("analytics.q.mixedTitle")}
        >
          {t("analytics.q.mixedBody")}
        </Callout>
      )}
      {answer.rows.length === 0 ? (
        <EmptyState>{t("analytics.q.empty")}</EmptyState>
      ) : (
        <DataTable
          label={t("analytics.q.answerTitle")}
          columns={answer.columns.map((column, index) => ({
            key: column,
            header: columnLabel(t, query, column),
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
            count: formatNumber(limit, locale),
          })}
        </p>
      )}
    </>
  );
}

function rowFigure(
  query: AnalyticsQuery,
  row: AnswerRow,
  namer: Namer,
  t: Translate,
) {
  const groupBy = query.group_by ?? [];
  if (groupBy.length === 0) {
    return t("analytics.q.allRecords");
  }
  return groupBy.map((field) => namer(field, row[field])).join(", ");
}

function RowExplain({
  frame,
  row,
  children,
}: Readonly<{ frame: CellFrame; row: AnswerRow; children: ReactNode }>) {
  const t = useT();
  const { query, namer, source, baseCurrency } = frame;
  const group = explainGroup(query, row);
  return (
    <ExplainDrawer
      figure={rowFigure(query, row, namer, t)}
      body={(opens) => (
        <ExplainRecords
          key={opens}
          source={source}
          query={query}
          group={group}
          baseCurrency={baseCurrency}
        />
      )}
    >
      {children}
    </ExplainDrawer>
  );
}

async function readExplanation(
  source: ExplainSource,
  group: unknown[] | undefined,
) {
  const body = group === undefined ? {} : { group };
  const { data, error } =
    source.kind === "run"
      ? await api.POST("/analytics/runs/{run_id}/cells/explain", {
          params: { path: { run_id: source.runId } },
          body,
        })
      : await api.POST("/analytics/explain", {
          body: { query: source.query, ...body },
        });
  if (error) {
    throwProblem(error);
  }
  return data;
}

/**
 * The records behind one cell, read under the reader's own authority. A
 * withheld cell has nothing to open, and a capped list says it is capped.
 */
function ExplainRecords({
  source,
  query,
  group,
  baseCurrency,
}: Readonly<{
  source: ExplainSource;
  query: AnalyticsQuery;
  group: unknown[] | undefined;
  baseCurrency: string | null;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const explanation = useQuery({
    queryKey: ["analytics-explain", source, group ?? null],
    queryFn: () => readExplanation(source, group),
  });
  const data = explanation.data;
  const namer = useValueNamer(
    query.entity,
    data?.columns ?? [],
    data?.rows ?? [],
  );
  if (explanation.isError) {
    return (
      <ErrorLine
        error={explanation.error}
        actions={
          <Button onClick={() => explanation.refetch()}>
            {t("common.retry")}
          </Button>
        }
      />
    );
  }
  const state = explanationState(data);
  return (
    <>
      {data?.truncated && (
        <p className="t-caption">
          {plural("analytics.q.explainTruncated", data.rows.length, {
            count: formatNumber(data.rows.length, locale),
          })}
        </p>
      )}
      <SurfaceState
        state={state}
        emptyLabel={t("analytics.q.explainEmpty")}
        loadingLabel={t("analytics.q.explainLoading")}
        detail={{ withheldReason: t("analytics.q.explainWithheld") }}
      >
        {data && (
          <DataTable
            label={t("explain.sources")}
            // The report drawer's column rule: the record's name leads, and
            // its id goes once every row has a name.
            columns={derivationColumns(data).map((column) => ({
              key: column,
              header: recordHeader(t, column),
              render: (row: AnswerRow) =>
                recordCell(column, row, {
                  query,
                  namer,
                  baseCurrency,
                  locale,
                }),
            }))}
            rows={data.rows}
            rowKey={(row) => String(row.id ?? data.rows.indexOf(row))}
          />
        )}
      </SurfaceState>
    </>
  );
}

function explanationState(
  data: { withheld: boolean; rows: unknown[] } | undefined,
): "loading" | "withheld" | "empty" | "ready" {
  if (!data) return "loading";
  if (data.withheld) return "withheld";
  return data.rows.length === 0 ? "empty" : "ready";
}

function recordHeader(t: Translate, column: string): string {
  if (column === "label") return t("analytics.q.record");
  if (column === "id") return t("analytics.q.recordId");
  return analyticsFieldLabel(t, column);
}

// A record is named by the server, under the reader's grants, so the drawer
// reads nothing per row. One it may not name keeps a short id.
function recordCell(
  column: string,
  row: AnswerRow,
  frame: Readonly<{
    query: AnalyticsQuery;
    namer: Namer;
    baseCurrency: string | null;
    locale: Locale;
  }>,
): ReactNode {
  const value = row[column];
  if (column === "label") {
    return typeof value === "string" ? value : "";
  }
  if (column === "id" && typeof value === "string") {
    return <span title={value}>{shortId(value)}</span>;
  }
  const currency = moneyCurrency(column, row, frame.query, frame.baseCurrency);
  if (currency !== undefined && typeof value === "number") {
    return formatMoneyOrAbsent(value, currency, frame.locale);
  }
  return frame.namer(column, value);
}

// What a refusal's kind means, as the lead-in under the engine's suggestion.
const REFUSAL_KEY: Readonly<Record<string, MessageKey>> = {
  invalid: "analytics.q.refusal.invalid",
  unsupported: "analytics.q.refusal.unsupported",
  privacy: "analytics.q.refusal.privacy",
};

/**
 * Why a question was not answered. A refusal is guidance, not a fault: it
 * leads with the smallest change that would have worked. Anything else is an
 * ordinary failure, in the reader's words.
 */
export function QuestionFailure({ error }: Readonly<{ error: unknown }>) {
  const t = useT();
  const refusal =
    error instanceof ProblemError ? refusalOf(error.problem) : null;
  if (!refusal) {
    return <ErrorLine error={error} />;
  }
  const lead = Object.hasOwn(REFUSAL_KEY, refusal.kind)
    ? t(REFUSAL_KEY[refusal.kind])
    : t("analytics.q.refusal.other");
  // The suggestion leads; the kind's sentence and the engine's own account of
  // what was wrong follow it. With no suggestion, the kind's sentence leads.
  const body = [refusal.suggest ? lead : "", refusal.message ?? ""]
    .filter((part) => part !== "")
    .join(" ");
  return (
    <Callout tone="info" kind="outcome" title={refusal.suggest || lead}>
      {body === "" ? undefined : body}
    </Callout>
  );
}
