// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { DataTable } from "../../design-system/datatable";
import { middayInstant } from "../../format/calendarday";
import {
  formatDate,
  formatMoney,
  formatNumber,
  formatSignedMoney,
  formatSignedNumber,
} from "../../format/format";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import {
  figureReading,
  figureValue,
  type WeeklyFigure,
  type WeeklyFigures,
} from "./brief.numeric";
import type { WeeklyReview } from "./brief.queries";

type Counts = WeeklyReview["counts"];

/**
 * One of the week's figures, read off the frozen counts.
 *
 * `count` is what the change against the week before is taken on. `shown` is
 * how the figure reads, which for a share is "3 of 4" rather than the count.
 * `none` marks a share with nothing to share out, which reads as words.
 */
type WeekFigure = Readonly<{
  key: string;
  label: string;
  figure: WeeklyFigure;
  count: (counts: Counts) => number | undefined;
  shown: (counts: Counts) => string;
  none?: (counts: Counts) => boolean;
}>;

/**
 * Every figure the report states, in the order the figures table reads them.
 * The headline is a cut of this list, so the two cannot describe one count
 * two ways.
 */
function weekFigures(t: Translator, locale: Locale): readonly WeekFigure[] {
  const n = (value: number) => formatNumber(value, locale);
  return [
    {
      key: "won",
      label: t("brief.weekly.dealsWon"),
      figure: "won",
      count: (c) => c.deals_won,
      shown: (c) => n(c.deals_won),
    },
    {
      key: "lost",
      label: t("brief.week.lostLabel"),
      figure: "lost",
      count: (c) => c.deals_lost,
      shown: (c) => n(c.deals_lost),
    },
    {
      key: "commitments",
      label: t("brief.weekly.planCommitmentsKept"),
      figure: "commitments",
      count: (c) => c.commitments_kept,
      none: (c) => c.commitments_due === 0,
      shown: (c) =>
        c.commitments_due === 0
          ? t("brief.weekly.noCommitments")
          : t("brief.weekly.ofDue", {
              done: n(c.commitments_kept),
              due: n(c.commitments_due),
            }),
    },
    {
      key: "leads",
      label: t("brief.weekly.leadsAnswered"),
      figure: "leads",
      count: (c) => c.leads_answered_in_target,
      none: (c) => c.leads_routed === 0,
      shown: (c) =>
        c.leads_routed === 0
          ? t("brief.weekly.noLeads")
          : t("brief.weekly.ofRouted", {
              answered: n(c.leads_answered_in_target),
              routed: n(c.leads_routed),
            }),
    },
    {
      key: "meetings",
      label: t("brief.weekly.meetingsHeld"),
      figure: "meetings",
      count: (c) => c.meetings_held,
      none: (c) => c.meetings_held === 0,
      shown: (c) =>
        c.meetings_held === 0
          ? t("brief.weekly.noMeetings")
          : t("brief.weekly.ofMeetings", {
              withStep: n(c.meetings_with_next_step),
              held: n(c.meetings_held),
            }),
    },
    {
      key: "tasks",
      label: t("brief.weekly.tasksCompleted"),
      figure: "tasks",
      count: (c) => c.tasks_completed,
      shown: (c) => n(c.tasks_completed ?? 0),
    },
    {
      key: "carried",
      label: t("brief.weekly.carriedOver"),
      figure: "tasks",
      count: (c) => c.tasks_carried_over,
      shown: (c) => n(c.tasks_carried_over),
    },
  ];
}

const HEADLINE = ["won", "commitments", "leads", "meetings"];

/** Only the figures a review of this age carries: an older one has no tasks. */
function carried(review: WeeklyReview, t: Translator, locale: Locale) {
  return weekFigures(t, locale).filter(
    (figure) => figure.count(review.counts) !== undefined,
  );
}

/** The change against the week before, or nothing for a first week. */
function change(
  figure: WeekFigure,
  review: WeeklyReview,
  locale: Locale,
): string | undefined {
  const now = figure.count(review.counts);
  const prior = review.prior?.counts;
  const before = prior ? figure.count(prior) : undefined;
  if (now === undefined || before === undefined) return undefined;
  return formatSignedNumber(now - before, locale);
}

/**
 * The four figures the week is judged by, large, each with what it moved since
 * the week before. Only a count is drawn large: a figure with nothing to count
 * shows a dash, and its words move to the caption, where they fit.
 */
export function HeadlineFigures({
  review,
  figures,
}: Readonly<{ review: WeeklyReview; figures: WeeklyFigures }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const priorWeek = review.prior?.local_week_start;
  const since = (delta: string | undefined) =>
    delta && priorWeek
      ? t("brief.weekly.sincePrior", {
          delta,
          week: formatDate(middayInstant(priorWeek, zone), locale, zone),
        })
      : undefined;
  const pipeline = review.pipeline;
  const wonValue = pipeline
    ? formatMoney(pipeline.won_minor, pipeline.currency, locale)
    : undefined;
  return (
    <dl className="brief-report-figures" data-testid="weekly-strip">
      {carried(review, t, locale)
        .filter((figure) => HEADLINE.includes(figure.key))
        .map((figure) => {
          const state = figures[figure.figure];
          const reading = figureReading(
            state,
            {
              value: figure.shown(review.counts),
              detail: since(change(figure, review, locale)),
            },
            { t, locale, zone },
          );
          const counted =
            (state.kind === "measured" || state.kind === "partial") &&
            !figure.none?.(review.counts);
          const lines = counted
            ? [figure.key === "won" ? wonValue : undefined, reading.detail]
            : [reading.value, reading.detail];
          return (
            <div className="brief-report-figure" key={figure.key}>
              <dt>{figure.label}</dt>
              <dd className="brief-report-figure-value t-num">
                {counted ? reading.value : "—"}
              </dd>
              {lines.filter(Boolean).map((line) => (
                <dd className="t-caption" key={line}>
                  {line}
                </dd>
              ))}
            </div>
          );
        })}
    </dl>
  );
}

type TableRow = Readonly<{
  key: string;
  label: string;
  week: string;
  prior: string;
  change: string;
}>;

/**
 * What the wins were worth, beside the week before. Only a prior week priced
 * in the same currency is compared; any other comparison would be invented.
 */
function wonValueRow(
  review: WeeklyReview,
  figures: WeeklyFigures,
  t: Translator,
  locale: Locale,
): TableRow | undefined {
  const pipeline = review.pipeline;
  if (pipeline === undefined) return undefined;
  const before = review.prior?.pipeline;
  const comparable =
    before !== undefined && before.currency === pipeline.currency;
  const measured = figures.won.kind === "measured";
  return {
    key: "wonValue",
    label: t("brief.weekly.outlook.won"),
    week: figureValue(
      figures.won,
      formatMoney(pipeline.won_minor, pipeline.currency, locale),
      t,
    ),
    prior: comparable
      ? formatMoney(before.won_minor, before.currency, locale)
      : "—",
    change:
      comparable && measured
        ? formatSignedMoney(
            pipeline.won_minor - before.won_minor,
            pipeline.currency,
            locale,
          )
        : "—",
  };
}

/** Every figure of the week beside the week before it, as one table. */
export function FiguresTable({
  review,
  figures,
}: Readonly<{ review: WeeklyReview; figures: WeeklyFigures }>) {
  const t = useT();
  const { locale } = useLocale();
  const prior = review.prior?.counts;
  const counted: TableRow[] = carried(review, t, locale).map((figure) => ({
    key: figure.key,
    label: figure.label,
    week: figureValue(figures[figure.figure], figure.shown(review.counts), t),
    prior: prior ? figure.shown(prior) : "—",
    change:
      figures[figure.figure].kind === "measured"
        ? (change(figure, review, locale) ?? "—")
        : "—",
  }));
  const money = wonValueRow(review, figures, t, locale);
  const rows = money ? [counted[0], money, ...counted.slice(1)] : counted;
  return (
    <DataTable
      label={t("brief.report.chapter.figures")}
      rowKey={(row: TableRow) => row.key}
      rows={rows.filter((row): row is TableRow => row !== undefined)}
      columns={[
        {
          key: "figure",
          header: t("brief.report.col.figure"),
          grow: true,
          render: (row) => row.label,
        },
        {
          key: "week",
          header: t("brief.report.col.week"),
          align: "end",
          render: (row) => row.week,
        },
        {
          key: "prior",
          header: t("brief.report.col.prior"),
          align: "end",
          render: (row) => row.prior,
        },
        {
          key: "change",
          header: t("brief.report.col.change"),
          align: "end",
          render: (row) => row.change,
        },
      ]}
    />
  );
}
