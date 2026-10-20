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
 */
type WeekFigure = Readonly<{
  key: string;
  label: string;
  figure: WeeklyFigure;
  count: (counts: Counts) => number | undefined;
  shown: (counts: Counts) => string;
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

// A closed week is comparable only to a priced prior week in the same currency.
export function wonPace(
  review: WeeklyReview,
  locale: Locale,
  t: Translator,
  zone: string,
): string | undefined {
  const pipeline = review.pipeline;
  if (pipeline === undefined) {
    return undefined;
  }
  const value = formatMoney(pipeline.won_minor, pipeline.currency, locale);
  const prior = review.prior;
  const before = prior?.pipeline;
  if (!prior || before === undefined || before.currency !== pipeline.currency) {
    return value;
  }
  return t("brief.weekly.wonVsPrior", {
    value,
    week: formatDate(middayInstant(prior.local_week_start, zone), locale, zone),
    delta: formatSignedMoney(
      pipeline.won_minor - before.won_minor,
      pipeline.currency,
      locale,
    ),
  });
}

/**
 * The four figures the week is judged by, large, each with what it moved since
 * the week before. A figure its source did not measure says so instead.
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
  return (
    <dl className="brief-report-figures" data-testid="weekly-strip">
      {carried(review, t, locale)
        .filter((figure) => HEADLINE.includes(figure.key))
        .map((figure) => {
          const detail =
            figure.key === "won"
              ? (wonPace(review, locale, t, zone) ??
                since(change(figure, review, locale)))
              : since(change(figure, review, locale));
          const reading = figureReading(
            figures[figure.figure],
            { value: figure.shown(review.counts), detail },
            { t, locale, zone },
          );
          return (
            <div className="brief-report-figure" key={figure.key}>
              <dt>{figure.label}</dt>
              <dd className="brief-report-figure-value t-num">
                {reading.value}
              </dd>
              {reading.detail && (
                <dd className="t-caption">{reading.detail}</dd>
              )}
            </div>
          );
        })}
    </dl>
  );
}

/** Every figure of the week beside the week before it, as one table. */
export function FiguresTable({
  review,
  figures,
}: Readonly<{ review: WeeklyReview; figures: WeeklyFigures }>) {
  const t = useT();
  const { locale } = useLocale();
  const prior = review.prior?.counts;
  return (
    <DataTable
      label={t("brief.report.chapter.figures")}
      rowKey={(row: WeekFigure) => row.key}
      rows={[...carried(review, t, locale)]}
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
          render: (row) =>
            figureValue(figures[row.figure], row.shown(review.counts), t),
        },
        {
          key: "prior",
          header: t("brief.report.col.prior"),
          align: "end",
          render: (row) => (prior ? row.shown(prior) : "—"),
        },
        {
          key: "change",
          header: t("brief.report.col.change"),
          align: "end",
          render: (row) =>
            figures[row.figure].kind === "measured"
              ? (change(row, review, locale) ?? "—")
              : "—",
        },
      ]}
    />
  );
}
