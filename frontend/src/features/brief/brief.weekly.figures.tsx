// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Badge } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Panel } from "../../design-system/panel";
import { isoWeekBefore, isoWeekOf } from "../../format/calendarday";
import {
  formatDayRange,
  formatMoney,
  formatNumber,
  formatSignedMoney,
  formatSignedNumber,
} from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
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
 * `share` is a figure's two numbers when it is part of a whole; a share of
 * nothing reads as words.
 */
type WeekFigure = Readonly<{
  key: string;
  label: string;
  figure: WeeklyFigure;
  count: (counts: Counts) => number | undefined;
  shown: (counts: Counts) => string;
  share?: (counts: Counts) => Share;
}>;

type Share = Readonly<{ filled: number; total: number }>;

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
      share: (c) => ({ filled: c.commitments_kept, total: c.commitments_due }),
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
      share: (c) => ({
        filled: c.leads_answered_in_target,
        total: c.leads_routed,
      }),
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
      count: (c) => c.meetings_with_next_step,
      share: (c) => ({
        filled: c.meetings_with_next_step ?? 0,
        total: c.meetings_held ?? 0,
      }),
      shown: (c) =>
        c.meetings_held === 0
          ? t("brief.weekly.noMeetings")
          : t("brief.weekly.ofMeetings", {
              withStep: n(c.meetings_with_next_step ?? 0),
              held: n(c.meetings_held ?? 0),
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

/**
 * Only the figures the review carries: an older one has no tasks, and one read
 * by a seat that may not read activities has no meetings.
 */
function carried(review: WeeklyReview, t: Translator, locale: Locale) {
  return weekFigures(t, locale).filter(
    (figure) => figure.count(review.counts) !== undefined,
  );
}

/** The change against the week before, or nothing for a first week. */
function difference(
  figure: WeekFigure,
  review: WeeklyReview,
): number | undefined {
  const now = figure.count(review.counts);
  const prior = review.prior?.counts;
  const before = prior ? figure.count(prior) : undefined;
  if (now === undefined || before === undefined) return undefined;
  return now - before;
}

function change(
  figure: WeekFigure,
  review: WeeklyReview,
  locale: Locale,
): string | undefined {
  const delta = difference(figure, review);
  return delta === undefined ? undefined : formatSignedNumber(delta, locale);
}

/**
 * The four figures the week is judged by, as one plate read across. Only a
 * count is a figure: a figure with nothing to count shows a dash, and its
 * words move under it. Each counted figure says what it moved since last week.
 */
export function HeadlineFigures({
  review,
  figures,
}: Readonly<{ review: WeeklyReview; figures: WeeklyFigures }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const priorStart = review.prior?.local_week_start;
  return (
    <>
      <Panel>
        <div className="brief-report-tiles" data-testid="weekly-strip">
          {carried(review, t, locale)
            .filter((figure) => HEADLINE.includes(figure.key))
            .map((figure) => (
              <HeadlineTile
                key={figure.key}
                figure={figure}
                review={review}
                state={figures[figure.figure]}
              />
            ))}
        </div>
      </Panel>
      {priorStart && (
        <p className="t-caption">
          {t("brief.report.priorWeek", {
            week: formatNumber(isoWeekOf(priorStart), locale),
            range: formatDayRange(
              priorStart,
              startOfDayInZone(priorStart, zone, 6),
              locale,
              zone,
            ),
          })}
        </p>
      )}
    </>
  );
}

/**
 * One headline figure: its name, and the count with what it is out of or, for
 * deals won, their worth. Under it come the share as a bar and the change.
 */
function HeadlineTile({
  figure,
  review,
  state,
}: Readonly<{
  figure: WeekFigure;
  review: WeeklyReview;
  state: WeeklyFigures[WeeklyFigure];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const reading = figureReading(
    state,
    { value: figure.shown(review.counts) },
    { t, locale, zone },
  );
  const share = figure.share?.(review.counts);
  const count = figure.count(review.counts);
  const counted =
    (state.kind === "measured" || state.kind === "partial") &&
    share?.total !== 0 &&
    count !== undefined;
  const delta = counted ? difference(figure, review) : undefined;
  const pipeline = figure.key === "won" ? review.pipeline : undefined;
  // Counted, the reading's qualifier goes under the figure; not counted, its
  // words do, then the reason.
  const first = counted ? reading.detail : reading.value;
  const second = counted ? undefined : reading.detail;
  return (
    <div className="brief-report-tile">
      <span className="brief-report-tile-label">{figure.label}</span>
      <span className="brief-report-tile-figure">
        <span className="brief-report-tile-value t-num">
          {counted ? formatNumber(share?.filled ?? count, locale) : "—"}
        </span>
        {counted && share && (
          <span className="brief-report-tile-of t-num">
            {`/ ${formatNumber(share.total, locale)}`}
          </span>
        )}
        {counted && pipeline && (
          <span className="brief-report-tile-money t-num">
            {formatMoney(pipeline.won_minor, pipeline.currency, locale)}
          </span>
        )}
      </span>
      {/* The share as a bar under the words that state it, so a screen
          reader is not read it twice. */}
      {counted && share && (
        <span className="brief-report-tile-bar" aria-hidden="true">
          <span
            data-down={delta !== undefined && delta < 0}
            style={{
              inlineSize: `${Math.round((Math.min(share.filled, share.total) / share.total) * 100)}%`,
            }}
          />
        </span>
      )}
      {first && (
        <span className="brief-report-figure-line t-caption">{first}</span>
      )}
      {second && (
        <span className="brief-report-figure-line t-caption">{second}</span>
      )}
      {delta !== undefined && (
        <span className="brief-report-figure-delta">
          <Badge tone={deltaTone(delta)}>
            {t("brief.report.vsPrior", {
              delta: formatSignedNumber(delta, locale),
            })}
          </Badge>
        </span>
      )}
    </div>
  );
}

/** A week up reads as good news and a week down as a warning; level is level. */
function deltaTone(delta: number): "success" | "danger" | "default" {
  if (delta > 0) return "success";
  return delta < 0 ? "danger" : "default";
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
    prior:
      prior && figure.count(prior) !== undefined ? figure.shown(prior) : "—",
    change:
      figures[figure.figure].kind === "measured"
        ? (change(figure, review, locale) ?? "—")
        : "—",
  }));
  const money = wonValueRow(review, figures, t, locale);
  const rows = money ? [counted[0], money, ...counted.slice(1)] : counted;
  return (
    <DataTable
      bleed
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
          header: t("brief.report.col.week", {
            week: formatNumber(isoWeekOf(review.local_week_start), locale),
          }),
          align: "end",
          render: (row) => row.week,
        },
        {
          key: "prior",
          header: t("brief.report.col.prior", {
            week: formatNumber(isoWeekBefore(review.local_week_start), locale),
          }),
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
