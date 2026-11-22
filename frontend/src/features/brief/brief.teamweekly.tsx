// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { TrendingUp } from "lucide-react";
import { StatCard } from "../../design-system/atoms";
import { DateInput, isISODate } from "../../design-system/dateinput";
import { Heading } from "../../design-system/heading";
import { Panel, PanelBody } from "../../design-system/panel";
import { Meter } from "../../design-system/readings";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState } from "../../design-system/surfacestate";
import { middayInstant, shiftCalendarDay } from "../../format/calendarday";
import {
  formatDate,
  formatDateTime,
  formatMoneyCompact,
  formatNumber,
} from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { openAnalyticsSection } from "../../lib/analytics.address";
import { useCompanyZone } from "../../lib/companyzone";
import { coverageOfWire, hasGap } from "../../lib/moneycoverage";
import { useUrlParams } from "../../lib/urlstate";
import { useViewerZone } from "../../lib/viewerzone";
import {
  anyUnrecorded,
  beforeRecordedHistory,
  figureReading,
  figureShown,
  type WeeklyFigure,
  weeklyNumericStatus,
} from "./brief.numeric";
import { ReportCard } from "./brief.reportcard";
import { AgendaSummary, TeamAgenda } from "./brief.teamweeklyagenda";
import { TeamCoverage, TeamQuality } from "./brief.teamweeklycoverage";
import { OutlookPanel } from "./brief.waterfall";
import {
  type TeamWeeklyReview,
  useTeamWeeklyReview,
} from "./teamweekly.queries";

import "./brief.teamweekly.css";

// A team's week, frozen. `/worklist/team` says what the team carries now; this
// says what last week was, so two weeks compare and neither moves.

export function TeamWeeklySection({
  teamId,
  week,
}: Readonly<{ teamId: string; week?: string }>) {
  const t = useT();
  const answer = useTeamWeeklyReview(teamId, week);
  const review = answer.data?.kind === "review" ? answer.data.review : null;
  if (!review)
    return (
      <section id="brief-team-weekly" data-testid="team-weekly">
        <WeekPicker week={week} />
        <SurfaceState
          state={
            answer.isPending ? "loading" : answer.isError ? "failed" : "empty"
          }
          loadingLabel={t("teamweekly.loading")}
          emptyLabel={t(
            answer.data?.kind === "absent" && answer.data.why === "forbidden"
              ? "teamweekly.forbidden"
              : "teamweekly.noSnapshot",
          )}
          detail={{ onRetry: () => void answer.refetch() }}
        >
          {null}
        </SurfaceState>
      </section>
    );
  return <TeamWeek review={review} week={week} />;
}

function TeamWeek({
  review,
  week,
}: Readonly<{ review: TeamWeeklyReview; week?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const companyZone = useCompanyZone();
  const readerZone = useViewerZone();
  const measured = review.counts.reps_counted > 0;
  const numeric = weeklyNumericStatus(
    review.numeric_summary,
    review.leads_withheld,
  );
  return (
    <section id="brief-team-weekly" data-testid="team-weekly">
      <WeekPicker week={week ?? review.local_week_start} />
      <Panel className="brief-report-summary">
        <PanelBody className="teamweekly-reading">
          <p className="t-sub">
            {t("teamweekly.weekOf", {
              team: review.team_name,
              day: formatDate(
                middayInstant(review.local_week_start, companyZone),
                locale,
                companyZone,
              ),
            })}
            {" · "}
            {t("brief.report.frozenAt", {
              at: formatDateTime(review.generated_at, locale, readerZone),
            })}
          </p>
          <Headline review={review} />
          {review.summary && (
            <p className="teamweekly-summary" data-testid="teamweekly-summary">
              {review.summary}
            </p>
          )}
          <RepsUnread review={review} />
          {measured && <AgendaSummary review={review} />}
          {numeric.partial && (
            <p className="t-sub">{t("brief.weekly.numericPartial")}</p>
          )}
        </PanelBody>
        {measured && (
          <PanelBody>
            <Scorecard review={review} />
          </PanelBody>
        )}
      </Panel>
      {measured && (
        <>
          <TeamAgenda review={review} />
          <Movement review={review} />
          <ReportCard
            icon={TrendingUp}
            title={t("brief.report.chapter.forecast")}
          >
            <PanelBody>
              <OutlookPanel
                outlook={review.outlook ?? []}
                locale={locale}
                onOpenForecast={() => openAnalyticsSection("forecast")}
              />
            </PanelBody>
          </ReportCard>
          <TeamCoverage review={review} />
          <TeamQuality review={review} />
        </>
      )}
      <p className="t-caption">{t(numeric.basis)}</p>
    </section>
  );
}

/** Two clauses, each naming a reading against the bar it was measured by. */
function Headline({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const counts = review.counts;
  const numeric = weeklyNumericStatus(
    review.numeric_summary,
    review.leads_withheld,
  );
  const say = (text: string) => (
    <Heading size="medium" className="teamweekly-headline">
      {text}
    </Heading>
  );
  if (
    counts.reps_counted === 0 ||
    (review.reps_unread ?? 0) > 0 ||
    numeric.partial
  )
    return say(
      t(
        counts.reps_counted === 0
          ? "teamweekly.headline.unmeasured"
          : "teamweekly.headline.partial",
      ),
    );
  // The clauses quote counts side by side, so one unrecorded source would put
  // a zero that measured nothing beside figures that did.
  if (anyUnrecorded(numeric.figures))
    return say(
      t(
        beforeRecordedHistory(numeric.figures)
          ? "brief.week.beforeHistory"
          : "teamweekly.headline.notRecorded",
      ),
    );
  const n = (value: number) => formatNumber(value, locale);
  if (
    counts.deals_won ||
    counts.deals_lost ||
    counts.deals_moved ||
    counts.leads_routed
  )
    return say(
      t("teamweekly.outcomes", {
        won: n(counts.deals_won),
        lost: n(counts.deals_lost),
        moved: n(counts.deals_moved),
        leads: n(counts.leads_routed),
      }),
    );
  if (counts.meetings_held === 0 && counts.commitments_due === 0)
    return say(t("teamweekly.headline.plain"));
  const clauses = [
    counts.meetings_held > 0 &&
      t("teamweekly.meetingRate", {
        done: n(counts.meetings_with_next_step),
        total: n(counts.meetings_held),
      }),
    counts.commitments_due > 0 &&
      t("teamweekly.commitmentRate", {
        done: n(counts.commitments_kept),
        total: n(counts.commitments_due),
      }),
  ];
  return say(clauses.filter(Boolean).join(" "));
}

/**
 * How much of the team the snapshot covers. Drawn whenever it is not zero: a
 * snapshot covering four of six members reads like a team of four.
 */
function RepsUnread({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  const unread = review.reps_unread ?? 0;
  if (unread === 0) return null;
  return (
    <p className="teamweekly-coverage">
      {plural("teamweekly.repsUnread", unread, {
        count: formatNumber(unread, locale),
        counted: formatNumber(review.counts.reps_counted, locale),
      })}
    </p>
  );
}

/**
 * What the team's wins were worth, or nothing. The pipeline block is absent
 * when any member's week could not be converted. A sum over the rest would be
 * short by one member. The currency is the snapshot's.
 */
function wonValue(
  review: TeamWeeklyReview,
  locale: Locale,
  partial: string,
): string | undefined {
  const pipeline = review.pipeline;
  if (pipeline === undefined) return undefined;
  const figure = formatMoneyCompact(
    pipeline.won_minor,
    pipeline.currency,
    locale,
  );
  return hasGap(coverageOfWire(review.pipeline_coverage))
    ? `${figure} ${partial}`
    : figure;
}

function Scorecard({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useCompanyZone();
  const counts = review.counts;
  const { figures } = weeklyNumericStatus(
    review.numeric_summary,
    review.leads_withheld,
  );
  const n = (value: number) => formatNumber(value, locale);
  const won = wonValue(review, locale, t("coverage.partial"));
  const place = { t, locale, zone };
  const reading = (
    figure: WeeklyFigure,
    shown: { value: string; detail?: string },
  ) => figureReading(figures[figure], shown, place);
  // "0 of 0" is a rate nobody could have scored, so an empty population says
  // so and nothing else.
  const share = (
    part: number,
    whole: number,
    none: MessageKey,
    basis: string,
  ) =>
    whole === 0
      ? { value: t(none), detail: undefined }
      : {
          value: t("teamweekly.ofTotal", { part: n(part), whole: n(whole) }),
          detail: basis,
        };
  return (
    <StatStrip testId="teamweekly-strip">
      <StatCard
        narrow="row"
        label={t("teamweekly.card.firstResponse")}
        {...reading(
          "leads",
          share(
            counts.leads_answered_in_target,
            counts.leads_routed,
            "teamweekly.card.noLeads",
            t("teamweekly.card.firstResponseBasis", {
              breached: n(counts.leads_breached),
            }),
          ),
        )}
      />
      <StatCard
        narrow="row"
        label={t("teamweekly.card.meetings")}
        {...reading(
          "meetings",
          share(
            counts.meetings_with_next_step,
            counts.meetings_held,
            "teamweekly.card.noMeetings",
            t("teamweekly.card.meetingsBasis"),
          ),
        )}
      />
      <StatCard
        narrow="row"
        label={t("teamweekly.card.commitments")}
        {...reading(
          "commitments",
          share(
            counts.commitments_kept,
            counts.commitments_due,
            "teamweekly.card.noCommitments",
            t("teamweekly.card.commitmentsBasis"),
          ),
        )}
      />
      <StatCard
        narrow="row"
        label={t("teamweekly.card.won")}
        {...reading("won", {
          value: n(counts.deals_won),
          detail:
            won === undefined
              ? t("teamweekly.card.wonBasis", { lost: n(counts.deals_lost) })
              : t("teamweekly.card.wonBasisValue", {
                  value: won,
                  lost: n(counts.deals_lost),
                }),
        })}
      />
      <StatCard
        narrow="row"
        label={t("teamweekly.card.reps")}
        value={n(counts.reps_counted)}
        detail={t("teamweekly.card.repsBasis")}
      />
    </StatStrip>
  );
}

/**
 * What the week did, as bars on one baseline, so the largest count draws the
 * longest bar. A week in which nothing happened draws no bars: a row of empty
 * tracks reads as a reading that failed to load.
 */
function Movement({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const counts = review.counts;
  const numeric = weeklyNumericStatus(
    review.numeric_summary,
    review.leads_withheld,
  );
  const movements: [WeeklyFigure, MessageKey, number][] = [
    ["won", "teamweekly.movement.won", counts.deals_won],
    ["lost", "teamweekly.movement.lost", counts.deals_lost],
    ["moved", "teamweekly.movement.moved", counts.deals_moved],
    ["meetings", "teamweekly.movement.meetings", counts.meetings_held],
    ["leads", "teamweekly.movement.leads", counts.leads_routed],
  ];
  const rows = movements
    .filter(([figure]) => figureShown(numeric.figures[figure]))
    .map(([, key, value]) => ({ key, value }));
  const max = Math.max(0, ...rows.map((row) => row.value));
  if (max === 0) return null;
  return (
    <ReportCard icon={TrendingUp} title={t("teamweekly.movement.title")}>
      <PanelBody className="teamweekly-movement">
        {rows.map((row) => (
          <div className="teamweekly-movement-row" key={row.key}>
            <span className="teamweekly-movement-name">{t(row.key)}</span>
            <Meter label={t(row.key)} value={row.value} max={max} dense flat />
            <span className="teamweekly-movement-count">
              {formatNumber(row.value, locale)}
            </span>
          </div>
        ))}
      </PanelBody>
    </ReportCard>
  );
}

/** Any day picks the Monday of its week, which is what the snapshot is keyed on. */
function WeekPicker({ week }: Readonly<{ week?: string }>) {
  const [params, setParams] = useUrlParams();
  const t = useT();
  return (
    <DateInput
      className="teamweekly-pick"
      aria-label={t("teamweekly.week")}
      value={week && isISODate(week) ? week : ""}
      onChange={(event) => {
        const next = new Map(params);
        const day = event.target.value;
        if (isISODate(day)) {
          const weekday = new Date(`${day}T12:00:00Z`).getUTCDay();
          next.set("week", shiftCalendarDay(day, -((weekday + 6) % 7)));
        } else next.delete("week");
        setParams(next);
      }}
    />
  );
}
