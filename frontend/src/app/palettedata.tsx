// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Disclosure, EmptyState } from "../design-system/atoms";
import { DataTable } from "../design-system/datatable";
import { Eyebrow } from "../design-system/eyebrow";
import { AnswerTable } from "../features/analytics/analytics.questions.answer";
import {
  reportingAmount,
  reportingPeriodLabel,
} from "../features/analytics/reporting.model";
import { useMetricNames } from "../features/analytics/reporting.queries";
import { metricView, VIEW_SHAPE } from "../features/analytics/reporting.views";
import { formatNumber } from "../format/format";
import { dayInZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { recordRoute } from "../lib/entity";
import { navigate } from "../lib/router";
import type { DataAnswer } from "./palettedata.queries";

export function PaletteDataAnswer({
  answer,
  onLeave,
}: Readonly<{
  answer: DataAnswer;
  onLeave: () => void;
}>) {
  const t = useT();
  const evaluation = answer.evaluation;
  const records = answer.records;
  const openAnalytics = () => {
    onLeave();
    navigate(
      {
        screen: "analytics",
        id: evaluation?.metrics[0]
          ? VIEW_SHAPE[metricView(evaluation.metrics[0].id)].section
          : "questions",
      },
      evaluation ? analyticsDials(evaluation) : undefined,
    );
  };
  return (
    <section className="palette-answer" aria-label={t("palette.answerAria")}>
      <Eyebrow>
        {t("palette.dataSource", {
          source: records ? t("palette.crmRecords") : t("nav.analytics"),
        })}
      </Eyebrow>
      {evaluation ? (
        <SearchMetric evaluation={evaluation} count={answer.count ?? false} />
      ) : null}
      {records ? <SearchRecords records={records} onLeave={onLeave} /> : null}
      {answer.analytics && answer.analytics_query ? (
        <AnswerTable
          answer={answer.analytics}
          query={answer.analytics_query}
          baseCurrency={answer.analytics.base_currency ?? null}
          source={{ kind: "query", query: answer.analytics_query }}
        />
      ) : null}
      {answer.source ? (
        <Disclosure summary={t("palette.dataPlan")}>
          <pre className="code-block">{answer.source}</pre>
        </Disclosure>
      ) : null}
      {evaluation ? (
        <Button onClick={openAnalytics}>{t("palette.openAnalytics")}</Button>
      ) : null}
    </section>
  );
}

type Evaluation = NonNullable<DataAnswer["evaluation"]>;
type Records = NonNullable<DataAnswer["records"]>;

function SearchMetric({
  evaluation,
  count,
}: Readonly<{ evaluation: Evaluation; count: boolean }>) {
  const t = useT();
  const names = useMetricNames();
  const { locale } = useLocale();
  const metric = evaluation.metrics[0];
  if (!metric) return null;
  return (
    <>
      <p>{names(metric.id)}</p>
      <p>
        {evaluation.context.scope.label ?? evaluation.context.scope.kind} ·{" "}
        {reportingPeriodLabel(
          evaluation.context.interval,
          evaluation.context.timezone,
          locale,
        )}
      </p>
      <p className="t-num">
        {count && metric.coverage.eligible_count != null
          ? reportingAmount(
              metric.coverage.eligible_count,
              "count",
              evaluation.context.currency,
              locale,
            )
          : metric.coverage.withheld ||
              !["ok", "partial"].includes(metric.coverage.status)
            ? t(`reporting.status.${metric.coverage.status}`)
            : reportingAmount(
                count ? metric.coverage.eligible_count : metric.value,
                count ? "count" : metric.unit,
                evaluation.context.currency,
                locale,
              )}
      </p>
      {metric.coverage.withheld ? <p>{t("reporting.status.partial")}</p> : null}
      {metric.coverage.reason ? <p>{metric.coverage.reason}</p> : null}
    </>
  );
}

function SearchRecords({
  records,
  onLeave,
}: Readonly<{ records: Records; onLeave: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <p>{records.executed_plan}</p>
      <p>
        {t("palette.recordsCoverage", {
          coverage:
            records.coverage === "complete_exact"
              ? t("palette.coverageExact")
              : t("palette.coveragePartial"),
          limit: formatNumber(records.limit, locale),
        })}
      </p>
      {records.notes.map((note) => (
        <p key={`${note.code}:${note.path}`}>{note.detail}</p>
      ))}
      {records.rows.length ? (
        <DataTable
          label={t("reporting.recordCount")}
          rows={records.rows}
          rowKey={(row) => row.record.id}
          columns={[
            {
              key: "record",
              header: t("palette.recordName"),
              render: ({ record }) => {
                const route = recordRoute(record.record_type, record.id);
                const label =
                  record.fields?.full_name ??
                  record.fields?.name ??
                  record.fields?.subject;
                const title = typeof label === "string" ? label : record.id;
                return route ? (
                  <Button
                    variant="link"
                    onClick={() => {
                      onLeave();
                      navigate(route);
                    }}
                  >
                    {title}
                  </Button>
                ) : (
                  title
                );
              },
            },
          ]}
        />
      ) : (
        <EmptyState>{t("palette.noDataRecords")}</EmptyState>
      )}
    </>
  );
}

function analyticsDials(evaluation: Evaluation): ReadonlyMap<string, string> {
  const { context, selection } = evaluation;
  const scope = context.scope.id
    ? `${context.scope.kind}:${context.scope.id}`
    : context.scope.kind;
  const dials = new Map([
    ["scope", scope],
    ["period", selection.period],
    ["pipeline", selection.pipeline_id ?? "all"],
    ["close", selection.close_window],
    ["target", selection.target_basis],
  ]);
  if (selection.interval) {
    dials.set(
      "from",
      dayInZone(Date.parse(selection.interval.start_at), context.timezone),
    );
    dials.set(
      "through",
      dayInZone(Date.parse(selection.interval.end_at) - 1, context.timezone),
    );
  }
  for (const [key, value] of [
    ["product", selection.product_id],
    ["initiative", selection.initiative_id],
    ["country", selection.country],
    ["industry", selection.industry],
    ["basis", selection.value_basis],
  ]) {
    if (key && value) dials.set(key, value);
  }
  return dials;
}
