// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight, ChevronLeft, ChevronRight } from "lucide-react";
import { type ReactNode, useId } from "react";
import { Badge, Button, Card } from "../../design-system/atoms";
import { Eyebrow } from "../../design-system/eyebrow";
import { Panel, PanelRow } from "../../design-system/panel";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { formatDate, formatDayLong, formatMoney } from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
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
  /** The week ahead's plan, the report's last section. */
  plan?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const [params] = useUrlParams();
  const asked = params.get("week") ?? undefined;
  const review = useWeeklyReview(asked);
  const data = review.data ?? null;
  // The next report follows the newest week, whichever week is shown. Until the
  // index loads, only the default view is known to show the newest.
  const newest =
    useWeeklyReviewIndex().data?.[0] ??
    (asked === undefined ? data?.local_week_start : undefined);
  return (
    <article id="brief-weekly" className="brief-report">
      {data && <WeeklyNarrative review={data} />}
      <WeeklyBody review={data} state={readState(review)} plan={plan} />
      {data && newest && (
        <footer className="brief-report-closing">
          <p className="t-caption">
            {t("brief.report.next", {
              date: formatDayLong(
                startOfDayInZone(newest, recordZone, 14),
                locale,
                recordZone,
              ),
            })}
          </p>
        </footer>
      )}
    </article>
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
      <span className="brief-report-keep">
        <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
      </span>
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

/** One section of the report: its label, what stands at the label's end, and
 *  its body. */
function ReportSection({
  title,
  aside,
  children,
}: Readonly<{ title: string; aside?: ReactNode; children: ReactNode }>) {
  const id = useId();
  return (
    <section className="brief-report-section" aria-labelledby={id}>
      <div className="brief-report-section-head">
        <Eyebrow as="h2" id={id} className="brief-section-label">
          {title}
        </Eyebrow>
        {aside}
      </div>
      {children}
    </section>
  );
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
      {reportSections(review, plan).map((section) => (
        <ReportSection
          key={section.key}
          title={t(section.title)}
          aside={section.aside}
        >
          {section.body}
        </ReportSection>
      ))}
    </>
  );
}

type Section = Readonly<{
  key: string;
  title: MessageKey;
  aside?: ReactNode;
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
    {
      key: "glance",
      title: "brief.report.chapter.glance",
      body: <HeadlineFigures review={review} figures={numeric.figures} />,
    },
    ...optional(closed.length > 0, {
      key: "closed",
      title: "brief.report.chapter.closed",
      aside: <ClosedTotal review={review} />,
      body: <ClosedDeals review={review} deals={closed} />,
    }),
    ...optional(moved.length > 0, {
      key: "movement",
      title: "brief.report.chapter.movement",
      body: <MovedDeals deals={moved} />,
    }),
    {
      key: "forecast",
      title: "brief.report.chapter.forecast",
      body: <Forecast review={review} />,
    },
    {
      key: "figures",
      title: "brief.report.chapter.figures",
      body: (
        <>
          <Panel>
            <FiguresTable review={review} figures={numeric.figures} />
          </Panel>
          <WeeklyWorkings counts={review.counts} figures={numeric.figures} />
          <ScorecardPanel
            scorecard={review.scorecard}
            weekStart={review.local_week_start}
          />
          <FiguresFootnote partial={numeric.partial} basis={numeric.basis} />
        </>
      ),
    },
    ...optional(learnings, {
      key: "observations",
      title: "brief.report.chapter.observations",
      body: <LearningsPanel learnings={learnings} />,
    }),
    ...optional(plan, {
      key: "commitments",
      title: "brief.report.chapter.commitments",
      body: plan,
    }),
  ];
}

type Deal = WeeklyReview["deals"][number];

/** The deal's name, linked. The label was frozen with the review, so a deal
 *  renamed since still reads as it did that week. */
function DealName({ deal }: Readonly<{ deal: Deal }>) {
  return (
    <a
      className="brief-weekly-deal-name link-button"
      href={routeHash({ screen: "deals", id: deal.deal_id })}
    >
      {deal.label}
    </a>
  );
}

/** What the week won and lost, at the end of the section's label. */
function ClosedTotal({ review }: Readonly<{ review: WeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const pipeline = review.pipeline;
  if (!pipeline) return null;
  return (
    <p className="brief-report-total t-num">
      {t("brief.report.closedTotal", {
        won: formatMoney(pipeline.won_minor, pipeline.currency, locale),
        lost: formatMoney(pipeline.lost_minor, pipeline.currency, locale),
      })}
    </p>
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
      )}
      <Panel>
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
                <Badge tone={deal.outcome === "won" ? "success" : "warning"}>
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
      </Panel>
    </>
  );
}

/** The deals that changed stage without closing, and the stage each reached. */
function MovedDeals({ deals }: Readonly<{ deals: readonly Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <Panel>
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
    </Panel>
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
