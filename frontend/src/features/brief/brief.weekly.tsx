// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  ArrowRight,
  ChartColumn,
  Check,
  ChevronLeft,
  ChevronRight,
  type LucideIcon,
  Sparkles,
  Table2,
  TrendingUp,
} from "lucide-react";
import type { ReactNode } from "react";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, Card } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { isoWeekOf } from "../../format/calendarday";
import {
  formatDate,
  formatDayLong,
  formatDayRange,
  formatMoney,
  formatNumber,
} from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { openAnalyticsSection } from "../../lib/analytics.address";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { weeklyNumericStatus } from "./brief.numeric";
import {
  useWeeklyReview,
  useWeeklyReviewIndex,
  type WeeklyReview,
} from "./brief.queries";
import { ReportCard } from "./brief.reportcard";
import { OutlookPanel } from "./brief.waterfall";
import { FiguresTable, HeadlineFigures } from "./brief.weekly.figures";
import { LearningsPanel } from "./brief.weekly.learnings";
import { ScorecardPanel } from "./brief.weekly.scorecard";
import { WeeklyWorkings } from "./brief.weekly.workings";

import "./brief.weekly.css";

// The week just gone, on Brief, as a report read top to bottom.
//
// No nav entry: the week is a view of the same work Today opens. Past weeks
// open through the stepper beside the title.
//
// It renders what was written when the week closed, and recomputes nothing.
// A retrospective that changed when you reopened it would not be one.

export function WeeklySection({
  plan,
}: Readonly<{
  /** The week ahead's plan, a section of the report. */
  plan?: ReactNode;
}>) {
  const [params] = useUrlParams();
  const review = useWeeklyReview(params.get("week") ?? undefined);
  const data = review.data ?? null;
  return (
    <article id="brief-weekly" className="brief-report">
      {data && <WeeklyNarrative review={data} />}
      <WeeklyBody review={data} state={readState(review)} plan={plan} />
    </article>
  );
}

/**
 * Beside the report: the week in figures, the weeks to open, what the week
 * is up against, and when the next report comes.
 */
export function WeeklyRail({
  constraints,
}: Readonly<{ constraints?: ReactNode }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [params] = useUrlParams();
  const asked = params.get("week") ?? undefined;
  const review = useWeeklyReview(asked).data ?? null;
  const weeks = useWeeklyReviewIndex().data;
  // The next report follows the newest week, whichever week is shown. Until the
  // index loads, only the default view is known to show the newest.
  const newest =
    weeks?.[0] ?? (asked === undefined ? review?.local_week_start : undefined);
  if (review === null) return null;
  const numeric = weeklyNumericStatus(review.numeric_summary);
  return (
    <>
      <Panel className="brief-weekly-summary">
        <HeadlineFigures review={review} figures={numeric.figures} />
      </Panel>
      <WeeksPanel weeks={weeks ?? []} shown={review.local_week_start} />
      {constraints}
      {newest && (
        <p className="t-caption brief-report-closing">
          {t("brief.report.next", {
            date: formatDayLong(
              startOfDayInZone(newest, recordZone, 14),
              locale,
              recordZone,
            ),
          })}
        </p>
      )}
    </>
  );
}

// How many of the newest weeks the side column lists; the stepper reaches
// every older one.
const LISTED_WEEKS = 6;

/** The newest weeks with a report, the one shown marked; a press opens one. */
function WeeksPanel({
  weeks,
  shown,
}: Readonly<{ weeks: readonly string[]; shown: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [params, setParams] = useUrlParams();
  if (weeks.length < 2) return null;
  return (
    <Panel flatHead title={t("brief.report.weeks")} titleLevel={2}>
      <ul className="brief-weekly-weeks">
        {weeks.slice(0, LISTED_WEEKS).map((week) => (
          <li key={week}>
            <button
              type="button"
              className="brief-weekly-week"
              aria-current={week === shown ? "true" : undefined}
              onClick={() => setParams(new Map([...params, ["week", week]]))}
            >
              <span>
                {t("brief.report.col.week", {
                  week: formatNumber(isoWeekOf(week), locale),
                })}
              </span>
              <span className="t-caption">
                {formatDayRange(
                  week,
                  startOfDayInZone(week, zone, 6),
                  locale,
                  zone,
                )}
              </span>
            </button>
          </li>
        ))}
      </ul>
    </Panel>
  );
}

/**
 * The way to the week before or after the one shown. The index lists the weeks
 * with a report, newest first, so a step never lands on a week nobody wrote.
 */
export function WeekStepper() {
  const t = useT();
  const [params, setParams] = useUrlParams();
  const weeks = useWeeklyReviewIndex().data ?? [];
  const shown = params.get("week") ?? weeks[0];
  if (shown === undefined || weeks.length < 2) return null;
  const at = weeks.indexOf(shown);
  const earlier = at >= 0 ? weeks[at + 1] : undefined;
  const later = at > 0 ? weeks[at - 1] : undefined;
  const open = (week: string) =>
    setParams(new Map([...params, ["week", week]]));
  return (
    <nav
      className="brief-report-stepper"
      aria-label={t("brief.weekly.pickWeek")}
    >
      <Button
        variant="ghost"
        iconOnly
        aria-label={t("brief.report.earlier")}
        disabled={earlier === undefined}
        onClick={() => earlier && open(earlier)}
      >
        <ChevronLeft aria-hidden="true" />
      </Button>
      <Button
        variant="ghost"
        iconOnly
        aria-label={t("brief.report.later")}
        disabled={later === undefined}
        onClick={() => later && open(later)}
      >
        <ChevronRight aria-hidden="true" />
      </Button>
    </nav>
  );
}

/**
 * What Margince wrote about the week, as the report's opening paragraph.
 *
 * A review with no sentence can mean a pass ran and found the week
 * unremarkable, or that no pass ran at all; `narrated_at` separates the two.
 * Without one, the head's counted sentence opens the report instead.
 */
function WeeklyNarrative({ review }: Readonly<{ review: WeeklyReview }>) {
  const t = useT();
  if (!review.narrated_at || !review.narrative) {
    return null;
  }
  return (
    <Card as="div" className="brief-report-narrative">
      <Sparkles role="img" aria-label={t("co.assistant.aiTag")} />
      <p>{review.narrative}</p>
    </Card>
  );
}

/** What one read says about itself, in the state vocabulary every section
 *  draws from. */
function readState(
  query: Readonly<{ isError: boolean; isPending: boolean }>,
): SectionState {
  if (query.isError) {
    return "failed";
  }
  return query.isPending ? "loading" : "ready";
}

function WeeklyBody({
  review,
  state,
  plan,
}: Readonly<{
  review: WeeklyReview | null;
  state: SectionState;
  plan?: ReactNode;
}>) {
  const t = useT();
  if (state !== "ready" || review === null) {
    // A rep whose first Monday has not come round yet says so; a page of
    // zeroes would claim a week that was measured and empty.
    return (
      <SurfaceState
        state={state === "ready" ? "empty" : state}
        emptyLabel={t("brief.weekly.none")}
        loadingLabel={t("brief.panel.weekly")}
      >
        {null}
      </SurfaceState>
    );
  }
  return (
    <>
      {reportSections(review, plan).map((section) =>
        section.icon ? (
          <ReportCard
            key={section.key}
            icon={section.icon}
            title={t(section.title)}
            aside={section.aside}
            footer={section.footer}
          >
            {section.body}
          </ReportCard>
        ) : (
          <section key={section.key}>{section.body}</section>
        ),
      )}
    </>
  );
}

type Section = Readonly<{
  key: string;
  title: MessageKey;
  // A section that brings its own card, as the plan does, has none.
  icon?: LucideIcon;
  aside?: ReactNode;
  footer?: ReactNode;
  body: ReactNode;
}>;

/**
 * The sections this week has something to say in, in reading order: what
 * happened, the full record, what it taught, then the week ahead. A section
 * with nothing in it is left out.
 */
function reportSections(
  review: WeeklyReview,
  plan: ReactNode | undefined,
): readonly Section[] {
  const numeric = weeklyNumericStatus(review.numeric_summary);
  const closed = review.deals.filter((deal) => deal.outcome !== "moved");
  const moved = review.deals.filter((deal) => deal.outcome === "moved");
  const learnings = review.learnings;
  const optional = (when: unknown, section: Section) => (when ? [section] : []);
  return [
    ...optional(closed.length > 0, {
      key: "closed",
      icon: Check,
      title: "brief.report.chapter.closed",
      aside: <ClosedTotal review={review} />,
      body: <ClosedDeals review={review} deals={closed} />,
    }),
    ...optional(moved.length > 0, {
      key: "movement",
      icon: TrendingUp,
      title: "brief.report.chapter.movement",
      aside: <MovedCount count={moved.length} />,
      body: <MovedDeals deals={moved} />,
    }),
    ...optional(plan, {
      key: "commitments",
      title: "brief.report.chapter.commitments",
      body: plan,
    }),
    ...optional(learnings, {
      key: "observations",
      icon: Sparkles,
      title: "brief.report.chapter.observations",
      aside: <AiTag />,
      body: <LearningsPanel learnings={learnings} />,
    }),
    ...optional(scorecardRows(review), {
      key: "scorecard",
      icon: ChartColumn,
      title: "brief.weekly.scorecard.title",
      body: (
        <ScorecardPanel
          scorecard={review.scorecard}
          weekStart={review.local_week_start}
          meetingsWithheld={review.meetings_withheld}
        />
      ),
    }),
    {
      key: "forecast",
      icon: TrendingUp,
      title: "brief.report.chapter.forecast",
      body: (
        <PanelBody>
          <Forecast review={review} />
        </PanelBody>
      ),
    },
    {
      key: "figures",
      icon: Table2,
      title: "brief.report.chapter.figures",
      footer: (
        <FiguresFootnote partial={numeric.partial} basis={numeric.basis} />
      ),
      body: (
        <>
          <FiguresTable review={review} figures={numeric.figures} />
          <WeeklyWorkings counts={review.counts} figures={numeric.figures} />
        </>
      ),
    },
  ];
}

/** Margince wrote this section, at the end of its label. */
function AiTag() {
  const t = useT();
  return <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>;
}

/** Whether the week carries a scorecard with a block to show. */
function scorecardRows(review: WeeklyReview): boolean {
  const card = review.scorecard;
  return Boolean(card && (card.lead || card.deal));
}

/** How many deals moved stage, at the end of the section's label. */
function MovedCount({ count }: Readonly<{ count: number }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <Badge dot>
      {plural("brief.report.movedCount", count, {
        count: formatNumber(count, locale),
      })}
    </Badge>
  );
}

type Deal = WeeklyReview["deals"][number];

/** The deal's name, linked. The label was frozen with the review, so a deal
 *  renamed since still reads as it did that week. */
function DealName({ deal }: Readonly<{ deal: Deal }>) {
  return (
    <AppLink
      className="brief-weekly-deal-name link-button"
      href={routeHash({ screen: "deals", id: deal.deal_id })}
    >
      {deal.label}
    </AppLink>
  );
}

/** What the week won and lost, at the end of the section's label. */
function ClosedTotal({ review }: Readonly<{ review: WeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const pipeline = review.pipeline;
  if (!pipeline) return null;
  return (
    <Badge tone="success" dot>
      {t("brief.report.closedTotal", {
        won: formatMoney(pipeline.won_minor, pipeline.currency, locale),
        lost: formatMoney(pipeline.lost_minor, pipeline.currency, locale),
      })}
    </Badge>
  );
}

/** The deals the week won and lost: the split of their worth, then a line
 *  each with its outcome and what it closed at. */
function ClosedDeals({
  review,
  deals,
}: Readonly<{ review: WeeklyReview; deals: readonly Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const pipeline = review.pipeline;
  const total = pipeline ? pipeline.won_minor + pipeline.lost_minor : 0;
  return (
    <>
      {/* The share won against lost, which the total beside the label states
          in words, so a screen reader is not read it twice. */}
      {pipeline && total > 0 && (
        <PanelBody>
          <div className="brief-report-split" aria-hidden="true">
            <span
              className="brief-report-split-won"
              style={{ flexGrow: pipeline.won_minor }}
            />
            <span
              className="brief-report-split-lost"
              style={{ flexGrow: pipeline.lost_minor }}
            />
          </div>
        </PanelBody>
      )}
      {deals.map((deal) => (
        <PanelRow key={`${deal.deal_id}-${deal.occurred_at}`}>
          <div className="brief-report-deal">
            <span className="brief-report-deal-what">
              <DealName deal={deal} />
              <time className="t-caption" dateTime={deal.occurred_at}>
                {formatDate(deal.occurred_at, locale, recordZone)}
              </time>
            </span>
            <span className="brief-report-keep">
              <Badge tone={deal.outcome === "won" ? "success" : "warning"} dot>
                {deal.outcome === "won"
                  ? t("brief.weekly.outcome.won")
                  : t("brief.weekly.outcome.lost")}
              </Badge>
            </span>
            <span
              className="brief-report-deal-amount t-num"
              data-outcome={deal.outcome}
            >
              {deal.amount_minor_at_close != null && deal.currency_at_close
                ? formatMoney(
                    deal.amount_minor_at_close,
                    deal.currency_at_close,
                    locale,
                  )
                : "—"}
            </span>
          </div>
        </PanelRow>
      ))}
    </>
  );
}

/** The deals that changed stage without closing, and the stage each reached. */
function MovedDeals({ deals }: Readonly<{ deals: readonly Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <>
      {deals.map((deal) => (
        <PanelRow key={`${deal.deal_id}-${deal.occurred_at}`}>
          <div className="brief-report-deal">
            <span className="brief-report-deal-what">
              <DealName deal={deal} />
              {deal.to_stage_label && (
                <span className="brief-report-stage">
                  {t("brief.weekly.outcome.moved")}
                  <ArrowRight aria-hidden="true" size={14} />
                  <Badge>{deal.to_stage_label}</Badge>
                </span>
              )}
            </span>
            <time className="t-caption" dateTime={deal.occurred_at}>
              {formatDate(deal.occurred_at, locale, recordZone)}
            </time>
          </div>
        </PanelRow>
      ))}
    </>
  );
}

/** Where the week was landing, frozen with the review. */
function Forecast({ review }: Readonly<{ review: WeeklyReview }>) {
  const { locale } = useLocale();
  return (
    <OutlookPanel
      outlook={review.outlook ?? []}
      locale={locale}
      onOpenForecast={() => openAnalyticsSection("forecast")}
    />
  );
}

/** What the figures rest on, once, under them. */
function FiguresFootnote({
  partial,
  basis,
}: Readonly<{ partial: boolean; basis: MessageKey }>) {
  const t = useT();
  return (
    <p className="t-caption brief-report-footnote">
      {t("brief.weekly.basis")} {t(basis)}
      {partial && <> {t("brief.weekly.numericPartial")}</>}
    </p>
  );
}
