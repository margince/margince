// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  TrajectoryChart,
  type TrajectoryChartProps,
} from "../../design-system/trajectorychart";
import {
  formatDayMonth,
  formatMoneyCompact,
  monthName,
} from "../../format/format";
import { DAY_MS, dayInZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { metricOf } from "./analytics.overview.readings";
import { lastInstant } from "./analytics.period";
import {
  type ReportingChart,
  type ReportingEvaluation,
  type ReportingEvidenceRef,
  reportingCompactAmount,
} from "./reporting.model";

type Window = Readonly<{ start_at: string; end_at: string }>;

/**
 * Sales won through the period, drawn against where it can still finish.
 * Every value is the server's: the daily trend, the target, the projection and
 * the modelled range. The page places them on the period's own time axis.
 */
export function QuarterTrajectory({
  evaluation,
  charts,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  charts: ReportingEvaluation | undefined;
  // The drawer reads the point off the chart read it came from.
  onEvidence: (
    reference: ReportingEvidenceRef,
    from?: ReportingEvaluation,
  ) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const trend = charts?.charts.find((chart) => chart.kind === "bookings_trend");
  if (!charts || trend?.coverage.status !== "ok") return null;
  const { interval, horizon, evaluated_at, timezone, currency } =
    evaluation.context;
  const whole = horizon ?? interval;
  const running = interval.end_at === evaluated_at && horizon != null;
  const points = weeklyToToday(trend, whole);
  const now = points.at(-1);
  if (!now) return null;
  const short = (value: number) => formatMoneyCompact(value, currency, locale);
  const amount = (value: number | null | undefined) =>
    reportingCompactAmount(value, trend.unit, currency, locale);
  const nowDate = formatDayMonth(
    running ? interval.end_at : lastInstant(interval),
    locale,
    timezone,
  );
  return (
    <TrajectoryChart
      label={t("analytics.trajectory.label")}
      dataLabel={t("reporting.data")}
      valueLabel={t("reporting.actual")}
      points={points.map((point) => ({
        key: point.key,
        label: point.label,
        value: point.value,
        at: point.at,
        amount: amount(point.value),
      }))}
      nowLabel={t(
        running ? "analytics.trajectory.today" : "analytics.trajectory.through",
        { date: nowDate },
      )}
      ticks={monthTicks(whole, timezone, locale)}
      axisLabel={short}
      {...outlookMarks(evaluation, running, short, t)}
      onSelect={(key) => {
        const reference = trend.points.find(
          (point) => point.key === key,
        )?.evidence;
        if (reference) onEvidence(reference, charts);
      }}
    />
  );
}

// The target, and on a running period where it can still finish: the
// committed projection, the modelled range and any shortfall to the target.
function outlookMarks(
  evaluation: ReportingEvaluation,
  running: boolean,
  short: (value: number) => string,
  t: Translator,
): Pick<TrajectoryChartProps, "target" | "projection" | "range" | "shortfall"> {
  const forecast = metricOf(evaluation, "forecast_landing");
  const visible = forecast?.coverage.withheld ? undefined : forecast;
  const target = visible?.target_comparison?.target;
  const gap = visible?.target_comparison?.gap;
  const low = visible?.range_low_minor;
  const high = visible?.range_high_minor;
  const marks: Pick<TrajectoryChartProps, "target"> =
    target != null
      ? {
          target: {
            value: target,
            label: t("analytics.trajectory.target", { amount: short(target) }),
          },
        }
      : {};
  if (!running || visible?.value == null) return marks;
  return {
    ...marks,
    projection: {
      value: visible.value,
      label: t("analytics.trajectory.projection", {
        amount: short(visible.value),
      }),
    },
    range:
      low != null && high != null
        ? { low, high, label: t("analytics.trajectory.range") }
        : undefined,
    shortfall:
      gap != null && gap > 0
        ? t("analytics.hero.short", { amount: short(gap) })
        : undefined,
  };
}

type Placed = Readonly<{
  key: string;
  label: string;
  value: number;
  at: number;
}>;

// The server reads the total through the end of each day. One reading a week,
// counted back from the last, keeps today on the line. A reader then opens a
// point every seven days rather than ninety close together.
export function weeklyToToday(
  trend: ReportingChart,
  whole: Window,
): readonly Placed[] {
  const start = Date.parse(whole.start_at);
  const span = Date.parse(whole.end_at) - start;
  if (!(span > 0)) return [];
  const read = trend.points.flatMap((point) => {
    const through = point.evidence?.through ?? point.at;
    if (point.value == null || !through) return [];
    return [{ ...point, value: point.value, through: Date.parse(through) }];
  });
  const kept: Placed[] = [];
  let next = Number.POSITIVE_INFINITY;
  for (const point of [...read].reverse()) {
    if (point.through > next) continue;
    kept.unshift({
      key: point.key,
      label: point.label,
      value: point.value,
      at: Math.min(1, Math.max(0, (point.through - start) / span)),
    });
    next = point.through - WEEK + DAY_MS / 2;
  }
  return kept;
}

// The first day of each month the period spans, read in the company's zone.
export function monthTicks(
  whole: Window,
  zone: string,
  locale: Locale,
): readonly Readonly<{ at: number; label: string }>[] {
  const start = Date.parse(whole.start_at);
  const span = Date.parse(whole.end_at) - start;
  if (!(span > 0)) return [];
  const ticks: { at: number; label: string }[] = [];
  // Read at each local midday, so a clock change never moves a day across.
  for (let offset = 0; offset + DAY_MS / 2 < span; offset += DAY_MS) {
    const day = dayInZone(start + offset + DAY_MS / 2, zone);
    if (offset === 0 || day.endsWith("-01"))
      ticks.push({
        at: offset / span,
        label: monthName(Number(day.slice(5, 7)), locale),
      });
  }
  return ticks;
}

const WEEK = 7 * DAY_MS;
