import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EmptyState, StatCard } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { type Locale, type Translator, useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import {
  type ReportingEvaluation,
  type ReportingMetricID,
  type ReportingSelection,
  reportingCompactAmount,
  reportingPeriodLabel,
  reportingQuery,
} from "./reporting.model";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";

type TargetBasis = ReportingSelection["target_basis"];
type Metric = ReportingEvaluation["metrics"][number];
type Readings = Readonly<Record<TargetBasis, ReportingEvaluation>>;

// A target covers a month or a fiscal quarter. The server matches each on its
// own window, so the figures are read once per basis, the longer one first.
const BASES: readonly TargetBasis[] = ["fiscal_quarter", "month"];

// The seat's figures against its own targets. The evaluation Performance reads
// matches them, never this file. The catalog names the figures that take one,
// so a metric that becomes targetable shows up here unasked.
export function MyTargets({
  self,
  locale,
}: Readonly<{ self: string; locale: Locale }>) {
  const t = useT();
  const readsTargets = useCan("sales_target", "read");
  const readsReports = useCan("report_definition", "read");
  const catalog = useReportingCatalog(readsTargets && readsReports);
  // Without the target grant every figure would read "No target set".
  if (!readsTargets || !readsReports) return null;
  return (
    <Panel title={t("analytics.myTargets")}>
      <PanelBody>
        <QueryGate query={catalog} pendingLabel={t("analytics.myTargets")}>
          {(catalog) => {
            const targeted = catalog.metrics
              .filter((metric) => metric.supports_target)
              .map((metric) => metric.id);
            return targeted.length ? (
              <TargetReadings self={self} metrics={targeted} locale={locale} />
            ) : (
              <EmptyState>{t("analytics.noTargetSet")}</EmptyState>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function TargetReadings({
  self,
  metrics,
  locale,
}: Readonly<{
  self: string;
  metrics: readonly ReportingMetricID[];
  locale: Locale;
}>) {
  const t = useT();
  const metricName = useMetricNames();
  const query = useQuery({
    queryKey: ["reporting-evaluation", "my-targets", self, metrics],
    queryFn: async (): Promise<Readings> => {
      const [fiscal_quarter, month] = await Promise.all([
        evaluate(self, "fiscal_quarter", metrics),
        evaluate(self, "month", metrics),
      ]);
      return { fiscal_quarter, month };
    },
  });
  return (
    <QueryGate query={query} pendingLabel={t("analytics.myTargets")}>
      {(readings) => (
        <StatStrip>
          {metrics.flatMap((id) =>
            targetCards(id, metricName(id), readings, t, locale).map((card) => (
              <StatCard
                key={card.key}
                narrow="row"
                label={card.label}
                value={card.value}
                detail={card.detail}
                meter={card.meter}
              />
            )),
          )}
        </StatStrip>
      )}
    </QueryGate>
  );
}

async function evaluate(
  self: string,
  basis: TargetBasis,
  metrics: readonly ReportingMetricID[],
): Promise<ReportingEvaluation> {
  const selection: ReportingSelection = {
    scope: { kind: "owner", id: self },
    period: "this_month",
    target_basis: basis,
    close_window: "all_open",
    metrics: [...metrics],
    blocks: [],
  };
  return unwrap(
    await api.GET("/analytics/evaluate", {
      params: { query: reportingQuery(selection) },
    }),
  );
}

type Card = Readonly<{
  key: string;
  label: string;
  value: string;
  detail?: string;
  meter?: { filled: number; total: number };
}>;

// One card per target the seat holds for this figure. With none, one card says
// so and still shows the month so far.
function targetCards(
  id: ReportingMetricID,
  label: string,
  readings: Readings,
  t: Translator,
  locale: Locale,
): Card[] {
  const cards = BASES.flatMap((basis): Card[] => {
    const metric = readings[basis].metrics.find((each) => each.id === id);
    if (metric?.target == null) return [];
    const amount = (value: number) =>
      reportingCompactAmount(
        value,
        metric.unit,
        readings[basis].context.currency,
        locale,
      );
    const actual = metric.target_actual;
    const { context } = readings[basis];
    // The dates the target covers, named as Performance names them.
    const period = context.target_interval
      ? reportingPeriodLabel(context.target_interval, context.timezone, locale)
      : null;
    const share =
      metric.attainment == null
        ? null
        : t("reporting.attainment", {
            percent: formatNumber(Math.round(metric.attainment), locale),
          });
    return [
      {
        key: `${id}-${basis}`,
        label,
        value:
          actual == null
            ? t(`reporting.status.${metric.coverage.status}`)
            : t("analytics.targetOf", {
                actual: amount(actual),
                target: amount(metric.target),
              }),
        detail: [period, share].filter((part) => part != null).join(" · "),
        meter:
          actual == null ? undefined : { filled: actual, total: metric.target },
      },
    ];
  });
  if (cards.length) return cards;
  const month = readings.month.metrics.find((each) => each.id === id);
  return month ? [untargeted(month, label, readings, t, locale)] : [];
}

function untargeted(
  metric: Metric,
  label: string,
  readings: Readings,
  t: Translator,
  locale: Locale,
): Card {
  // A figure the reader may not see in full gets no target applied. "No target
  // set" over it would claim something nobody knows.
  if (metric.coverage.withheld) {
    return {
      key: metric.id,
      label,
      value: t("analytics.readingUnavailable"),
      detail: t("reporting.withheld"),
    };
  }
  return {
    key: metric.id,
    label,
    value: t("analytics.noTargetSet"),
    // No figure means the metric could not be read, and the status says why.
    detail:
      metric.value == null
        ? t(`reporting.status.${metric.coverage.status}`)
        : t("analytics.noTargetSetActual", {
            actual: reportingCompactAmount(
              metric.value,
              metric.unit,
              readings.month.context.currency,
              locale,
            ),
          }),
  };
}
