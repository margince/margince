// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  Activity,
  Check,
  ChevronLeft,
  ChevronRight,
  type LucideIcon,
  Sparkles,
  Table2,
  TrendingUp,
} from "lucide-react";
import type { ReactNode } from "react";
import { Badge, Button, Card, Disclosure } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { openAnalyticsSection } from "../../lib/analytics.address";
import { useUrlParams } from "../../lib/urlstate";
import { weeklyNumericStatus } from "./brief.numeric";
import {
  useWeeklyReview,
  useWeeklyReviewIndex,
  type WeeklyReview,
} from "./brief.queries";
import { ReportCard } from "./brief.reportcard";
import { OutlookPanel } from "./brief.waterfall";
import { ActivityTotal, WeekActivity } from "./brief.weekly.activity";
import {
  ClosedDeals,
  ClosedTotal,
  MovedCount,
  MovedDeals,
} from "./brief.weekly.deals";
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
  /** The plan for the week now running, drawn after the report. */
  plan?: ReactNode;
}>) {
  const t = useT();
  const [params] = useUrlParams();
  const review = useWeeklyReview(params.get("week") ?? undefined);
  const newest = useWeeklyReviewIndex().data?.[0];
  const data = review.data ?? null;
  // The report is what was written when its week closed. The plan is live and
  // belongs to the week now running, so it stands outside the report. Over an
  // earlier week it waits behind its own opener.
  const pastWeek = params.has("week") && params.get("week") !== newest;
  return (
    <>
      <article id="brief-weekly" className="brief-report">
        {data && <WeeklyNarrative review={data} />}
        <WeeklyBody review={data} state={readState(review)} />
      </article>
      {plan &&
        (pastWeek ? (
          <Disclosure summary={t("plan.titleThisWeek")}>{plan}</Disclosure>
        ) : (
          plan
        ))}
    </>
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
}: Readonly<{
  review: WeeklyReview | null;
  state: SectionState;
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
  const numeric = weeklyNumericStatus(
    review.numeric_summary,
    review.leads_withheld,
  );
  return (
    <>
      {/* The week's outcomes once, at the head of the report. */}
      <Panel className="brief-report-summary">
        <HeadlineFigures review={review} figures={numeric.figures} />
      </Panel>
      {reportSections(review).map((section) =>
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
function reportSections(review: WeeklyReview): readonly Section[] {
  const numeric = weeklyNumericStatus(
    review.numeric_summary,
    review.leads_withheld,
  );
  const closed = review.deals.filter((deal) => deal.outcome !== "moved");
  const moved = review.deals.filter((deal) => deal.outcome === "moved");
  const learnings = review.learnings;
  // Absent on an older review and for a reader who may not read activities.
  const days = review.daily_activity;
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
      // The lines are capped for reading; the frozen count is complete.
      aside: <MovedCount count={review.counts.deals_moved} />,
      body: <MovedDeals deals={moved} />,
    }),
    ...(days
      ? [
          {
            key: "activity",
            icon: Activity,
            title: "brief.weekly.activity.title",
            aside: <ActivityTotal days={days} />,
            body: <WeekActivity days={days} />,
          } satisfies Section,
        ]
      : []),
    ...optional(learnings?.items.length, {
      key: "observations",
      icon: Sparkles,
      title: "brief.report.chapter.observations",
      aside: <AiTag />,
      body: <LearningsPanel learnings={learnings} />,
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
      body: <FiguresBreakdown review={review} figures={numeric.figures} />,
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

/**
 * The scorecard, every figure beside the week before, and the workings, in one
 * breakdown the reader opens. The headline above already says how the week
 * went.
 */
function FiguresBreakdown({
  review,
  figures,
}: Readonly<{
  review: WeeklyReview;
  figures: ReturnType<typeof weeklyNumericStatus>["figures"];
}>) {
  const t = useT();
  return (
    <PanelBody>
      <Disclosure summary={t("brief.report.breakdown")}>
        {scorecardRows(review) && (
          <ScorecardPanel
            scorecard={review.scorecard}
            weekStart={review.local_week_start}
            meetingsWithheld={review.meetings_withheld}
          />
        )}
        <FiguresTable review={review} figures={figures} />
        <WeeklyWorkings counts={review.counts} figures={figures} />
      </Disclosure>
    </PanelBody>
  );
}

/**
 * What the figures rest on, behind its own opener. A partial read stays in
 * view: it changes how far the figures can be trusted.
 */
function FiguresFootnote({
  partial,
  basis,
}: Readonly<{ partial: boolean; basis: MessageKey }>) {
  const t = useT();
  return (
    <>
      {partial && (
        <p className="t-caption brief-report-footnote">
          {t("brief.weekly.numericPartial")}
        </p>
      )}
      <Disclosure summary={t("brief.report.howCounted")}>
        <p className="t-caption brief-report-footnote">
          {t("brief.weekly.basis")} {t(basis)}
        </p>
      </Disclosure>
    </>
  );
}
