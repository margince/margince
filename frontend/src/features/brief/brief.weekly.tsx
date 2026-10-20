// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ChevronLeft, ChevronRight } from "lucide-react";
import { type ReactNode, useId } from "react";
import { Badge, Button } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import {
  type SectionState,
  SurfaceState,
} from "../../design-system/surfacestate";
import { ProvenanceTag } from "../../design-system/trust";
import {
  formatDate,
  formatDateTime,
  formatDayLong,
  formatMoney,
  formatNumber,
} from "../../format/format";
import { startOfDayInZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { openAnalyticsSection } from "../analytics/analytics.address";
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

// Derived from the review's own deal shape rather than reached for separately:
// one import, and the outcome vocabulary cannot drift from the payload the
// panel actually renders.
type WeeklyReviewDealOutcome = WeeklyReview["deals"][number]["outcome"];

// The week just gone, on Brief, as a report read top to bottom.
//
// No nav entry: the week is a view of the same work Today opens. Past weeks
// open through the stepper.
//
// It renders what was written when the week closed, and recomputes nothing.
// A retrospective that changed when you reopened it would not be one.

export function WeeklySection({
  plan,
}: Readonly<{
  /** The week ahead's plan, the report's last chapter. */
  plan?: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // undefined = the most recent. A chosen week is a different read, keyed
  // separately, so moving between weeks does not overwrite the cache of either.
  const [params, setParams] = useUrlParams();
  const week = params.get("week");
  const setWeek = (next: string) =>
    setParams(new Map([...params, ["week", next]]));
  const review = useWeeklyReview(week);
  const index = useWeeklyReviewIndex();
  const weeks = index.data ?? [];
  const shown = week ?? weeks[0];
  return (
    <article id="brief-weekly" className="brief-report">
      {review.data && (
        <div className="brief-report-byline">
          {/* FROZEN: the figures were written when the week closed and can no
              longer move. Drawn only over a review that exists, so an empty
              read does not certify a week nobody wrote. */}
          <Badge>{t("brief.weekly.frozen")}</Badge>
          {/* Only where a model wrote the week's sentence: the figures are a
              deterministic pass, so a standing mark would claim them too. */}
          {narratedByModel(review.data) && (
            <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
          )}
          <span className="t-caption">
            {t("brief.weekly.written", {
              at: formatDateTime(review.data.generated_at, locale, recordZone),
            })}
          </span>
          {shown !== undefined && weeks.length > 1 && (
            <WeekStepper weeks={weeks} shown={shown} onWeek={setWeek} />
          )}
        </div>
      )}
      <WeeklyBody
        review={review.data ?? null}
        state={readState(review)}
        plan={plan}
      />
      {review.data && (
        <footer className="brief-report-closing">
          <p className="t-caption">
            {t("brief.report.next", {
              date: formatDayLong(
                startOfDayInZone(review.data.local_week_start, recordZone, 14),
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
 * The way to the week before or after this one. The index lists the weeks with
 * a report, newest first, so a step never lands on a week nobody wrote.
 */
function WeekStepper({
  weeks,
  shown,
  onWeek,
}: Readonly<{
  weeks: readonly string[];
  shown: string;
  onWeek: (week: string) => void;
}>) {
  const t = useT();
  const at = weeks.indexOf(shown);
  const earlier = at >= 0 ? weeks[at + 1] : undefined;
  const later = at > 0 ? weeks[at - 1] : undefined;
  return (
    <nav
      className="brief-report-stepper"
      aria-label={t("brief.weekly.pickWeek")}
    >
      <Button
        variant="ghost"
        disabled={earlier === undefined}
        onClick={() => earlier && onWeek(earlier)}
      >
        <ChevronLeft aria-hidden="true" />
        {t("brief.report.earlier")}
      </Button>
      <Button
        variant="ghost"
        disabled={later === undefined}
        onClick={() => later && onWeek(later)}
      >
        {t("brief.report.later")}
        <ChevronRight aria-hidden="true" />
      </Button>
    </nav>
  );
}

/**
 * The sentence about the week, at the head of its first chapter.
 *
 * A review with no sentence can mean a pass ran and found the week
 * unremarkable, or that no pass ran at all; `narrated_at` separates the two.
 */
function WeeklyNarrative({ review }: Readonly<{ review: WeeklyReview }>) {
  if (!review.narrated_at || !review.narrative) {
    return null;
  }
  return (
    <div className="brief-report-narrative">
      <ProvenanceTag provenance={{ kind: "agent" }} />
      <p>{review.narrative}</p>
    </div>
  );
}

/** Whether a model actually wrote a sentence about this week. The byline's
 *  badge and the narrative both answer to it, so they cannot disagree. */
function narratedByModel(review: WeeklyReview): boolean {
  return Boolean(review.narrated_at && review.narrative);
}

/** The outcome as a word. A lookup rather than a template key, because the
 *  contract's enum can grow and a built key would ask for a missing message. */
function outcomeWord(
  t: (key: MessageKey) => string,
  outcome: WeeklyReviewDealOutcome,
): string {
  switch (outcome) {
    case "won":
      return t("brief.weekly.outcome.won");
    case "lost":
      return t("brief.weekly.outcome.lost");
    default:
      return t("brief.weekly.outcome.moved");
  }
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

/** One numbered chapter of the report. */
function Chapter({
  number,
  title,
  children,
}: Readonly<{ number: number; title: string; children: ReactNode }>) {
  const { locale } = useLocale();
  const id = useId();
  return (
    <section className="brief-report-chapter" aria-labelledby={id}>
      <Heading
        size="medium"
        as="h2"
        id={id}
        className="brief-report-chapter-title"
      >
        <span className="brief-report-chapter-number t-num">
          {formatNumber(number, locale)}
        </span>
        {title}
      </Heading>
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
  const chapters = reportChapters(review, plan);
  return (
    <>
      {chapters.map((chapter, at) => (
        <Chapter key={chapter.key} number={at + 1} title={t(chapter.title)}>
          {chapter.body}
        </Chapter>
      ))}
    </>
  );
}

type ReportChapter = Readonly<{
  key: string;
  title: MessageKey;
  body: ReactNode;
}>;

/**
 * The chapters this week has something to say in, in reading order: what
 * happened, the full record, what it taught, then the week ahead. A chapter
 * with nothing in it is left out, and the numbers close up over the gap.
 */
function reportChapters(
  review: WeeklyReview,
  plan: ReactNode | undefined,
): readonly ReportChapter[] {
  const numeric = weeklyNumericStatus(review.numeric_summary);
  const closed = review.deals.filter((deal) => deal.outcome !== "moved");
  const moved = review.deals.filter((deal) => deal.outcome === "moved");
  const learnings = review.learnings;
  return [
    {
      key: "glance",
      title: "brief.report.chapter.glance",
      body: (
        <>
          <WeeklyNarrative review={review} />
          <HeadlineFigures review={review} figures={numeric.figures} />
        </>
      ),
    },
    ...(closed.length > 0
      ? [
          {
            key: "closed",
            title: "brief.report.chapter.closed" as const,
            body: <ClosedDeals review={review} deals={closed} />,
          },
        ]
      : []),
    {
      key: "movement",
      title: "brief.report.chapter.movement",
      body: <Movement review={review} deals={moved} />,
    },
    {
      key: "figures",
      title: "brief.report.chapter.figures",
      body: (
        <>
          <FiguresTable review={review} figures={numeric.figures} />
          <WeeklyWorkings counts={review.counts} figures={numeric.figures} />
          <ScorecardPanel scorecard={review.scorecard} />
          <FiguresFootnote partial={numeric.partial} basis={numeric.basis} />
        </>
      ),
    },
    ...(learnings
      ? [
          {
            key: "observations",
            title: "brief.report.chapter.observations" as const,
            body: <LearningsPanel learnings={learnings} />,
          },
        ]
      : []),
    ...(plan
      ? [
          {
            key: "commitments",
            title: "brief.report.chapter.commitments" as const,
            body: plan,
          },
        ]
      : []),
  ];
}

type Deal = WeeklyReview["deals"][number];

/**
 * One deal line. The LABEL, not a lookup: it was frozen when the review was
 * written, so a deal renamed since still reads as it did that week.
 */
function DealLine({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <li>
      <a
        className="brief-weekly-deal-name link-button"
        href={routeHash({ screen: "deals", id: deal.deal_id })}
      >
        {deal.label}
      </a>
      <span className="t-caption">
        {outcomeWord(t, deal.outcome)}
        {deal.to_stage_label ? ` · ${deal.to_stage_label}` : ""}
      </span>
      <time
        className="brief-weekly-deal-when t-caption"
        dateTime={deal.occurred_at}
      >
        {formatDate(deal.occurred_at, locale, recordZone)}
      </time>
    </li>
  );
}

/** The deals the week won and lost, and what they were worth. */
function ClosedDeals({
  review,
  deals,
}: Readonly<{ review: WeeklyReview; deals: readonly Deal[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const pipeline = review.pipeline;
  return (
    <>
      <ul className="brief-weekly-deals">
        {deals.map((deal) => (
          <DealLine key={`${deal.deal_id}-${deal.occurred_at}`} deal={deal} />
        ))}
      </ul>
      {pipeline && (
        <p className="brief-report-total t-num">
          {t("brief.report.closedTotal", {
            won: formatMoney(pipeline.won_minor, pipeline.currency, locale),
            lost: formatMoney(pipeline.lost_minor, pipeline.currency, locale),
          })}
        </p>
      )}
    </>
  );
}

/** The deals that changed stage without closing, and where the week was
 *  landing against the forecast. */
function Movement({
  review,
  deals,
}: Readonly<{ review: WeeklyReview; deals: readonly Deal[] }>) {
  const { locale } = useLocale();
  return (
    <>
      {deals.length > 0 && (
        <ul className="brief-weekly-deals">
          {deals.map((deal) => (
            <DealLine key={`${deal.deal_id}-${deal.occurred_at}`} deal={deal} />
          ))}
        </ul>
      )}
      <OutlookPanel
        outlook={review.outlook ?? []}
        locale={locale}
        onOpenForecast={() => openAnalyticsSection("forecast")}
      />
    </>
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
