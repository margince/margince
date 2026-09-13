// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { useUrlParams } from "../app/urlstate";
import { Badge, Disclosure, StatCard } from "../design-system/atoms";
import { DateInput, isISODate } from "../design-system/dateinput";
import { Eyebrow } from "../design-system/eyebrow";
import { Panel, PanelBody } from "../design-system/panel";
import { Meter } from "../design-system/readings";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import { calendarDay, middayInstant } from "../format/calendarday";
import { formatDate, formatMoney, formatNumber } from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import { openAnalyticsSection } from "./analytics.address";
import { BriefTeamSelect } from "./brief.teamselect";
import { AgendaPanel, AgendaSummary } from "./brief.teamweeklyagenda";
import { OutlookPanel } from "./brief.waterfall";
import {
  type TeamWeeklyReview,
  useTeamWeeklyReview,
} from "./teamweekly.queries";

import "./brief.teamweekly.css";

// A team's week, frozen. `/worklist/team` says what the team is carrying now;
// this says what last week WAS, so two weeks compare and neither moves under
// the comparison.

export function TeamWeeklySection({
  teamId,
  week,
}: Readonly<{ teamId: string | undefined; week?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const answer = useTeamWeeklyReview(teamId, week);
  const review = answer.data?.kind === "review" ? answer.data.review : null;
  if (!review)
    return (
      <Panel title={t("teamweekly.title")}>
        <SurfaceState
          state={
            answer.isPending
              ? "loading"
              : answer.isError
                ? "unavailable"
                : "ready"
          }
          loadingLabel={t("teamweekly.loading")}
          emptyLabel={t("teamweekly.empty")}
          detail={{ onRetry: () => void answer.refetch() }}
        >
          <PanelBody>
            {t(
              answer.data?.kind === "absent" && answer.data.why === "forbidden"
                ? "teamweekly.forbidden"
                : "teamweekly.noSnapshot",
            )}
          </PanelBody>
        </SurfaceState>
      </Panel>
    );
  const measured = review.counts.reps_counted > 0;
  return (
    <section id="brief-team-weekly">
      <p>
        {t("teamweekly.weekOf", {
          team: review.team_name,
          day: formatDate(review.local_week_start, locale, recordZone),
        })}
      </p>
      <Coverage review={review} />
      {!measured && <Headline review={review} />}
      {measured && <AgendaPanel review={review} />}
      {measured && (
        <Disclosure summary={t("brief.week.supporting")}>
          <Panel
            title={t("teamweekly.title")}
            titleAction={<Badge quiet>{t("teamweekly.frozen")}</Badge>}
          >
            <PanelBody className="teamweekly-reading">
              <Headline review={review} />
              <AgendaSummary review={review} />
            </PanelBody>
            <PanelBody>
              <Scorecard review={review} />
            </PanelBody>
            <Movement review={review} />
          </Panel>
          <TeamOutlook review={review} />
        </Disclosure>
      )}
    </section>
  );
}

// The team's landing, drawn through the rep panel's own component.
//
// Its own horizon state, held here rather than lifted: the team page and the
// rep page are different surfaces a reader reads at different moments, and a
// shared dial would move one when they turned the other.
function TeamOutlook({
  review,
}: Readonly<{ review: NonNullable<TeamWeeklyReview> }>) {
  const { locale } = useLocale();
  const [horizon, setHorizon] = useState("quarter");
  return (
    <OutlookPanel
      outlook={review.outlook ?? []}
      locale={locale}
      horizon={horizon}
      onHorizon={setHorizon}
      onOpenForecast={() => openAnalyticsSection("forecast")}
    />
  );
}

/** Two clauses, each naming a reading against the bar it was measured by. */
function Headline({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const counts = review.counts;
  if (counts.reps_counted === 0 || (review.reps_unread ?? 0) > 0)
    return (
      <h3 className="teamweekly-headline">
        {t(
          counts.reps_counted === 0
            ? "teamweekly.headline.unmeasured"
            : "teamweekly.headline.partial",
        )}
      </h3>
    );
  const n = (value: number) => formatNumber(value, locale);
  if (counts.meetings_held === 0 && counts.commitments_due === 0)
    return (
      <h3 className="teamweekly-headline">{t("teamweekly.headline.plain")}</h3>
    );
  return (
    <h3 className="teamweekly-headline">
      {counts.meetings_held > 0 &&
        t("brief.team.meetingRate", {
          done: n(counts.meetings_with_next_step),
          total: n(counts.meetings_held),
        })}{" "}
      {counts.commitments_due > 0 &&
        t("brief.team.commitmentRate", {
          done: n(counts.commitments_kept),
          total: n(counts.commitments_due),
        })}
    </h3>
  );
}

/**
 * How much of the team the snapshot actually covers.
 *
 * `reps_unread` is drawn whenever it is non-zero, never behind a disclosure: a
 * snapshot silently covering four of six reps reads exactly like a team of
 * four, and every figure above is short by the same two contacts.
 */
function Coverage({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const unread = review.reps_unread ?? 0;
  if (unread === 0) {
    return null;
  }
  return (
    <p className="teamweekly-coverage">
      {t("teamweekly.repsUnread", {
        count: formatNumber(unread, locale),
        counted: formatNumber(review.counts.reps_counted, locale),
      })}
    </p>
  );
}

/**
 * What the team's wins were worth, or nothing.
 *
 * NOTHING, not a zero, when the snapshot carries no pipeline block. That block
 * is absent whenever any member's week could not be converted — the schema says
 * so — because summing only the reps who DID convert would be a confident
 * number quietly missing one. "€0 won" over a team that won deals is the
 * opposite of what happened.
 *
 * The currency is the SNAPSHOT's, never the installation's current setting:
 * base currency is operator-mutable, and re-reading it would re-label a closed
 * week with a currency its numbers were never in.
 */
function wonValue(
  review: TeamWeeklyReview,
  locale: Locale,
): string | undefined {
  const pipeline = review.pipeline;
  if (pipeline === undefined) {
    return undefined;
  }
  return formatMoney(pipeline.won_minor, pipeline.currency, locale);
}

function Scorecard({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const counts = review.counts;
  const n = (value: number) => formatNumber(value, locale);
  const ofTotal = (part: number, whole: number) =>
    t("teamweekly.ofTotal", { part: n(part), whole: n(whole) });
  const won = wonValue(review, locale);

  return (
    <StatStrip testId="teamweekly-strip">
      <StatCard
        density="compact"
        label={t("teamweekly.card.firstResponse")}
        value={ofTotal(counts.leads_answered_in_target, counts.leads_routed)}
        detail={t("teamweekly.card.firstResponseBasis", {
          breached: n(counts.leads_breached),
        })}
      />
      <StatCard
        density="compact"
        label={t("teamweekly.card.meetings")}
        value={ofTotal(counts.meetings_with_next_step, counts.meetings_held)}
        detail={t("teamweekly.card.meetingsBasis")}
      />
      <StatCard
        density="compact"
        label={t("teamweekly.card.commitments")}
        value={ofTotal(counts.commitments_kept, counts.commitments_due)}
        detail={t("teamweekly.card.commitmentsBasis")}
      />
      <StatCard
        density="compact"
        label={t("teamweekly.card.won")}
        value={n(counts.deals_won)}
        // What the wins were WORTH, beside how many were lost. The count alone
        // says a week of five small renewals and a week of one company-making
        // deal are the same week — and the money was computed, FX-converted and
        // stored when the snapshot was written, then read by nothing.
        //
        // It rides the won slot rather than taking a sixth: five is what a
        // strip can be read across as one comparison. The lost count stays,
        // because it is a different fact rather than a delta the money replaces.
        detail={
          won === undefined
            ? t("teamweekly.card.wonBasis", { lost: n(counts.deals_lost) })
            : t("teamweekly.card.wonBasisValue", {
                value: won,
                lost: n(counts.deals_lost),
              })
        }
      />
      <StatCard
        density="compact"
        label={t("teamweekly.card.reps")}
        value={n(counts.reps_counted)}
        detail={t("teamweekly.card.repsBasis")}
      />
    </StatStrip>
  );
}

/**
 * What the week did, as bars sharing one baseline.
 *
 * Length follows magnitude and the figure carries the reading — a bar whose
 * length alone said "good" or "bad" would be making a claim the snapshot does
 * not, since a team that lost four deals and won four drew two equal bars.
 *
 * ONE ROW PER DIMENSION, and the row carries the words: the name leading, the
 * bar taking the room between, the count trailing. `Meter` draws the bar and
 * nothing else — its `label` is an `aria-label` — so five bars under one
 * heading were five unlabelled tracks to everybody who could see them, which
 * on a quiet week is a single grey band with a heading over it.
 *
 * A WEEK IN WHICH NOTHING HAPPENED DRAWS NO BARS. With every count at zero
 * there is no baseline to draw against: the bars are empty tracks, and a row of
 * empty tracks reads as a reading that failed to load rather than as a week
 * that was quiet. The strip above already reports the zeros as figures.
 */
function Movement({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const counts = review.counts;
  const rows = [
    { key: "teamweekly.movement.won" as const, value: counts.deals_won },
    { key: "teamweekly.movement.lost" as const, value: counts.deals_lost },
    // ADVANCED sits with the two outcomes above it rather than with the
    // activity rows below, because it is the same kind of fact: what happened
    // to the team's deals. Without it a week that moved eleven deals and closed
    // none read as a week where nothing happened, which is the week most teams
    // have and the one a lead most needs to see.
    { key: "teamweekly.movement.moved" as const, value: counts.deals_moved },
    {
      key: "teamweekly.movement.meetings" as const,
      value: counts.meetings_held,
    },
    { key: "teamweekly.movement.leads" as const, value: counts.leads_routed },
  ];
  // One baseline for every bar. A per-row max would draw four full bars and say
  // nothing about which number is the big one.
  const max = Math.max(...rows.map((row) => row.value));
  if (max === 0) {
    return null;
  }

  return (
    <PanelBody className="teamweekly-movement">
      <Eyebrow as="h3">{t("teamweekly.movement.title")}</Eyebrow>
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
  );
}

/**
 * The team weekly with its team picker, for a reader whose scope reaches a team.
 *
 * Gated on the same `scope_options` the team board beside it reads, so the
 * control and the refusal cannot disagree — a picker offered to a rep who will
 * be refused every team is a control that exists to fail.
 */
export function TeamWeeklyPanel({ offered }: Readonly<{ offered: boolean }>) {
  const [params, setParams] = useUrlParams();
  const t = useT();
  const week = params.get("week") ?? "";
  const zone = useRecordZone();
  if (!offered) return null;
  return (
    <>
      <DateInput
        aria-label={t("brief.team.week")}
        value={isISODate(week) ? week : ""}
        onChange={(event) => {
          const next = new Map(params);
          if (isISODate(event.target.value)) {
            const day = new Date(middayInstant(event.target.value, zone));
            const weekday = new Date(
              `${event.target.value}T12:00:00Z`,
            ).getUTCDay();
            day.setUTCDate(day.getUTCDate() - ((weekday + 6) % 7));
            next.set("week", calendarDay(day, zone));
          } else next.delete("week");
          setParams(next);
        }}
      />
      <BriefTeamSelect>
        {(team) => (
          <TeamWeeklySection teamId={team} week={params.get("week")} />
        )}
      </BriefTeamSelect>
    </>
  );
}
