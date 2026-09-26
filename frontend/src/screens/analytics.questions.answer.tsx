// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { EntityKind } from "../app/entity";
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
import { ExplainDrawer } from "./analytics.explain.drawer";
import { type Namer, useValueNamer } from "./analytics.questions.names";
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
import { EntityRef } from "./entityref";

type Translate = ReturnType<typeof useT>;

/** Where a row's records are read from: the question itself, or a saved run. */
export type ExplainSource =
  | Readonly<{ kind: "query"; query: AnalyticsQuery }>
  | Readonly<{ kind: "run"; runId: string }>;

// The record a drill-down row IS, per population, so its id opens that record.
// Activities have no page of their own and keep their id.
const RECORD_KIND: Readonly<Record<string, EntityKind>> = {
  "pipeline-current": "deal",
  "deals-by-stage": "deal",
  forecast: "deal",
  "open-deals-per-company": "deal",
  "win-loss": "deal",
  "stage-age": "deal",
  "leads-by-status": "lead",
  "projects-by-phase": "project",
  "project-commitments": "project",
  "projects-gone-quiet": "project",
};

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
}>;

// One cell. The row's first cell also carries the way into its records, and a
// withheld row says so there, once, with nothing in the cells beside it.
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
  const { locale } = useLocale();
  const { query, namer, baseCurrency } = frame;
  if (isWithheldRow(row)) {
    return first ? t("analytics.q.withheldRow") : null;
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
 * group. A withheld group stays a row, saying so, rather than printing zeros
 * or vanishing; the row count is not itself a signal of anything.
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
  const namer = useValueNamer(query.group_by ?? [], answer.rows);
  const frame: CellFrame = { query, namer, source, baseCurrency };
  const position = new Map(answer.rows.map((row, index) => [row, index]));
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
          rows={answer.rows}
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
  const namer = useValueNamer(data?.columns ?? [], data?.rows ?? []);
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
  const recordKind = Object.hasOwn(RECORD_KIND, query.entity)
    ? RECORD_KIND[query.entity]
    : undefined;
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
            columns={data.columns.map((column) => ({
              key: column,
              header:
                column === "id"
                  ? t("analytics.q.record")
                  : analyticsFieldLabel(t, column),
              render: (row: AnswerRow) =>
                recordCell(column, row, {
                  query,
                  recordKind,
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

function recordCell(
  column: string,
  row: AnswerRow,
  frame: Readonly<{
    query: AnalyticsQuery;
    recordKind: EntityKind | undefined;
    namer: Namer;
    baseCurrency: string | null;
    locale: Locale;
  }>,
): ReactNode {
  const value = row[column];
  if (column === "id" && typeof value === "string" && frame.recordKind) {
    return <EntityRef kind={frame.recordKind} id={value} newTab />;
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
