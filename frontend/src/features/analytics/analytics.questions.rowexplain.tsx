// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The drawer a row of an answer opens: the records behind that one cell, read
// under the reader's own authority.

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import { DataTable } from "../../design-system/datatable";
import { SurfaceState } from "../../design-system/surfacestate";
import {
  formatAnalyticsMoneyOrAbsent,
  formatAnalyticsNumber,
} from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import { derivationColumns } from "./analytics.explain";
import { ExplainDrawer } from "./analytics.explain.drawer";
import type { ExplainSource } from "./analytics.questions.answer";
import { QuestionFailure } from "./analytics.questions.failure";
import { type MoneyFields, moneyCurrency } from "./analytics.questions.money";
import {
  type Namer,
  shortId,
  useValueNamer,
} from "./analytics.questions.names";
import {
  type AnalyticsQuery,
  type AnswerRow,
  analyticsFieldLabel,
  type CustomLabels,
  explainGroup,
} from "./analytics.questions.vocab";

type Translate = ReturnType<typeof useT>;

/** What a row's drawer needs of the table it opens from. */
export type RowExplainFrame = Readonly<{
  query: AnalyticsQuery;
  namer: Namer;
  baseCurrency: string | null;
  money: MoneyFields;
  fieldLabels: CustomLabels;
}>;

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

export function RowExplain({
  frame,
  source,
  row,
  children,
}: Readonly<{
  frame: RowExplainFrame;
  source: ExplainSource;
  row: AnswerRow;
  children: ReactNode;
}>) {
  const t = useT();
  const { query, namer, baseCurrency, money, fieldLabels } = frame;
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
          money={money}
          fieldLabels={fieldLabels}
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
  money,
  fieldLabels,
}: Readonly<{
  source: ExplainSource;
  query: AnalyticsQuery;
  group: unknown[] | undefined;
  baseCurrency: string | null;
  money: MoneyFields;
  fieldLabels: CustomLabels;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const plural = usePlural();
  const explanation = useQuery({
    queryKey: ["analytics-explain", source, group ?? null],
    queryFn: () => readExplanation(source, group),
  });
  const data = explanation.data;
  const namer = useValueNamer(query.entity, data?.columns ?? [], data?.labels);
  if (explanation.isError) {
    return (
      <QuestionFailure
        error={explanation.error}
        onRetry={() => explanation.refetch()}
      />
    );
  }
  const state = explanationState(data);
  return (
    <>
      {data?.truncated && (
        <p className="t-caption">
          {plural("analytics.q.explainTruncated", data.rows.length, {
            count: formatAnalyticsNumber(data.rows.length, locale),
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
              header: recordHeader(t, column, fieldLabels),
              render: (row: AnswerRow) =>
                recordCell(column, row, {
                  query,
                  namer,
                  baseCurrency,
                  money,
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

function recordHeader(
  t: Translate,
  column: string,
  fieldLabels: CustomLabels,
): string {
  if (column === "label") return t("analytics.q.record");
  if (column === "id") return t("analytics.q.recordId");
  return analyticsFieldLabel(t, column, fieldLabels);
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
    money: MoneyFields;
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
  const currency = moneyCurrency(
    column,
    row,
    frame.query,
    frame.baseCurrency,
    frame.money,
  );
  if (currency !== undefined && typeof value === "number") {
    return formatAnalyticsMoneyOrAbsent(value, currency, frame.locale);
  }
  return frame.namer(column, value);
}
