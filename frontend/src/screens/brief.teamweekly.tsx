// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { Badge, StatCard } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { Meter } from "../design-system/readings";
import { Select } from "../design-system/select";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDate, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { EntityRef } from "./entityref";
import {
  type TeamWeeklyFocusKind,
  type TeamWeeklyReview,
  useTeams,
  useTeamWeeklyReview,
} from "./teamweekly.queries";

import "./brief.teamweekly.css";

// A team's week, frozen. `/worklist/team` says what the team is carrying now;
// this says what last week WAS, so two weeks compare and neither moves under
// the comparison.

/**
 * The bars the reader is told they were measured against.
 *
 * Declared constants rather than numbers buried in a condition, because the
 * headline states a verdict and a verdict has to name its bar — "answered in
 * time on 9 of 10 leads" is a reading, "first response is healthy" is a claim,
 * and the second one is only honest if the reader can see where the line was
 * drawn.
 */
const HEALTHY_RATE = 0.9;
const WEAK_RATE = 0.7;

/** Which focus rules are a coaching prompt and which are something to copy. */
const CELEBRATED: ReadonlySet<TeamWeeklyFocusKind> = new Set([
  "strong_week",
  "quiet_week",
]);

const FOCUS_LABEL: Readonly<Record<TeamWeeklyFocusKind, MessageKey>> = {
  help_requested: "teamweekly.focus.help_requested",
  leads_breached: "teamweekly.focus.leads_breached",
  commitments_missed: "teamweekly.focus.commitments_missed",
  meetings_without_next_step: "teamweekly.focus.meetings_without_next_step",
  strong_week: "teamweekly.focus.strong_week",
  quiet_week: "teamweekly.focus.quiet_week",
};

/** A rate, or null when nothing was due — zero of zero is not zero per cent. */
function rate(part: number, whole: number): number | null {
  return whole === 0 ? null : part / whole;
}

/** One reading, with what it was measured against carried beside it. */
type Reading = Readonly<{
  key: MessageKey;
  value: number;
  verdict: "healthy" | "weak" | "middling";
}>;

function readingOf(key: MessageKey, value: number | null): Reading | null {
  if (value === null) {
    return null;
  }
  const verdict =
    value >= HEALTHY_RATE
      ? "healthy"
      : value <= WEAK_RATE
        ? "weak"
        : "middling";
  return { key, value, verdict };
}

/**
 * The two clauses of the headline: the healthiest reading and the weakest.
 *
 * Both come from the stored snapshot, so the sentence cannot disagree with the
 * figures under it. When no reading is decided either way the caller says the
 * plainest true thing instead — a verdict the data does not support is worse
 * than no verdict.
 */
export function headlineReadings(
  review: TeamWeeklyReview,
): Readonly<{ best: Reading | null; worst: Reading | null }> {
  const counts = review.counts;
  const readings = [
    readingOf(
      "teamweekly.reading.firstResponse",
      rate(counts.leads_answered_in_target, counts.leads_routed),
    ),
    readingOf(
      "teamweekly.reading.nextStep",
      rate(counts.meetings_with_next_step, counts.meetings_held),
    ),
    readingOf(
      "teamweekly.reading.commitments",
      rate(counts.commitments_kept, counts.commitments_due),
    ),
  ].filter((reading): reading is Reading => reading !== null);

  return {
    best: pick(readings, "healthy", (a, b) => a.value > b.value),
    worst: pick(readings, "weak", (a, b) => a.value < b.value),
  };
}

/** The one reading of a verdict that `beats` every other of the same verdict. */
function pick(
  readings: readonly Reading[],
  verdict: Reading["verdict"],
  beats: (a: Reading, b: Reading) => boolean,
): Reading | null {
  return readings.reduce<Reading | null>(
    (best, reading) =>
      reading.verdict !== verdict
        ? best
        : best === null || beats(reading, best)
          ? reading
          : best,
    null,
  );
}

export function TeamWeeklySection({
  teamId,
  week,
}: Readonly<{ teamId: string | undefined; week?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const answer = useTeamWeeklyReview(teamId, week);

  const state = answer.isPending
    ? "loading"
    : answer.isError
      ? "unavailable"
      : "ready";
  const review = answer.data?.kind === "review" ? answer.data.review : null;

  return (
    <section id="brief-team-weekly" aria-label={t("teamweekly.title")}>
      <Panel
        title={t("teamweekly.title")}
        sub={
          review
            ? t("teamweekly.weekOf", {
                team: review.team_name,
                day: formatDate(review.local_week_start, locale, recordZone),
              })
            : undefined
        }
        titleAction={
          review ? <Badge quiet>{t("teamweekly.frozen")}</Badge> : undefined
        }
      >
        <SurfaceState
          state={state}
          emptyLabel={t("teamweekly.empty")}
          loadingLabel={t("teamweekly.loading")}
          detail={{ onRetry: () => void answer.refetch() }}
        >
          {answer.data?.kind === "absent" && (
            <PanelBody>
              <p>
                {answer.data.why === "forbidden"
                  ? t("teamweekly.forbidden")
                  : t("teamweekly.noSnapshot")}
              </p>
            </PanelBody>
          )}
          {review && (
            <>
              <PanelBody>
                <Headline review={review} />
                <Coverage review={review} />
              </PanelBody>
              <Scorecard review={review} />
              <Movement review={review} />
            </>
          )}
        </SurfaceState>
      </Panel>
      {review && <CoachPanel review={review} />}
    </section>
  );
}

/** Two clauses, each naming a reading against the bar it was measured by. */
function Headline({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const { best, worst } = headlineReadings(review);

  if (!best && !worst) {
    return (
      <h3 className="teamweekly-headline">{t("teamweekly.headline.plain")}</h3>
    );
  }
  return (
    <h3 className="teamweekly-headline">
      {best &&
        t("teamweekly.headline.healthy", {
          reading: t(best.key),
          pct: formatNumber(Math.round(best.value * 100), locale),
          bar: formatNumber(Math.round(HEALTHY_RATE * 100), locale),
        })}{" "}
      {worst &&
        t("teamweekly.headline.weak", {
          reading: t(worst.key),
          pct: formatNumber(Math.round(worst.value * 100), locale),
          bar: formatNumber(Math.round(WEAK_RATE * 100), locale),
        })}
    </h3>
  );
}

/**
 * How much of the team the snapshot actually covers.
 *
 * `reps_unread` is drawn whenever it is non-zero, never behind a disclosure: a
 * snapshot silently covering four of six reps reads exactly like a team of
 * four, and every figure above is short by the same two people.
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

function Scorecard({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const { locale } = useLocale();
  const counts = review.counts;
  const n = (value: number) => formatNumber(value, locale);
  const ofTotal = (part: number, whole: number) =>
    t("teamweekly.ofTotal", { part: n(part), whole: n(whole) });

  return (
    <StatStrip testId="teamweekly-strip">
      <StatCard
        label={t("teamweekly.card.firstResponse")}
        value={ofTotal(counts.leads_answered_in_target, counts.leads_routed)}
        detail={t("teamweekly.card.firstResponseBasis", {
          breached: n(counts.leads_breached),
        })}
      />
      <StatCard
        label={t("teamweekly.card.meetings")}
        value={ofTotal(counts.meetings_with_next_step, counts.meetings_held)}
        detail={t("teamweekly.card.meetingsBasis")}
      />
      <StatCard
        label={t("teamweekly.card.commitments")}
        value={ofTotal(counts.commitments_kept, counts.commitments_due)}
        detail={t("teamweekly.card.commitmentsBasis")}
      />
      <StatCard
        label={t("teamweekly.card.won")}
        value={n(counts.deals_won)}
        detail={t("teamweekly.card.wonBasis", { lost: n(counts.deals_lost) })}
      />
      <StatCard
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
 */
function Movement({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  const counts = review.counts;
  const rows = [
    { key: "teamweekly.movement.won" as const, value: counts.deals_won },
    { key: "teamweekly.movement.lost" as const, value: counts.deals_lost },
    {
      key: "teamweekly.movement.meetings" as const,
      value: counts.meetings_held,
    },
    { key: "teamweekly.movement.leads" as const, value: counts.leads_routed },
  ];
  // One baseline for every bar. A per-row max would draw four full bars and say
  // nothing about which number is the big one.
  const max = Math.max(...rows.map((row) => row.value), 1);

  return (
    <PanelBody>
      <h3 className="teamweekly-subhead">{t("teamweekly.movement.title")}</h3>
      {rows.map((row) => (
        <Meter
          key={row.key}
          label={t(row.key)}
          value={row.value}
          max={max}
          dense
          flat
        />
      ))}
    </PanelBody>
  );
}

/**
 * One row per rep, including the rep whose week went well.
 *
 * A page promising one focus per rep and delivering rows only for the troubled
 * ones reads as a team where only those people exist — which is why the server
 * has positive focus kinds at all.
 */
function CoachPanel({ review }: Readonly<{ review: TeamWeeklyReview }>) {
  const t = useT();
  return (
    <Panel title={t("teamweekly.coach.title")} sub={t("teamweekly.coach.sub")}>
      {review.reps.length === 0 && (
        <PanelBody>
          <p>{t("teamweekly.coach.empty")}</p>
        </PanelBody>
      )}
      {review.reps.map((rep) => (
        <PanelRow key={rep.user_id}>
          <div className="teamweekly-rep">
            <span className="teamweekly-rep-name">
              <EntityRef kind="user" id={rep.user_id} name={rep.display_name} />
            </span>
            <span className="teamweekly-rep-focus">
              <Badge
                quiet
                tone={CELEBRATED.has(rep.focus_kind) ? "success" : "warn"}
              >
                {t(FOCUS_LABEL[rep.focus_kind])}
              </Badge>
              {/* Composed by the server from the stored figures, never
                  model-written, so it cannot say what the snapshot does not
                  hold. */}
              <span>{rep.focus_label}</span>
            </span>
          </div>
        </PanelRow>
      ))}
    </Panel>
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
  const t = useT();
  const teams = useTeams();
  const [teamId, setTeamId] = useState("");
  if (!offered) {
    return null;
  }
  const options = (teams.data ?? []).map((team) => ({
    value: team.id,
    label: team.name,
  }));
  // One team is not a choice. Reading it straight skips a control whose only
  // option is the one already showing.
  const chosen = teamId || (options.length === 1 ? options[0].value : "");
  return (
    <>
      {options.length > 1 && (
        <Select
          options={options}
          value={chosen}
          onChange={setTeamId}
          placeholder={t("teamweekly.pickTeam")}
          aria-label={t("teamweekly.pickTeam")}
        />
      )}
      {chosen !== "" && <TeamWeeklySection teamId={chosen} />}
    </>
  );
}
