// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import {
  formatDuration,
  formatHours,
  formatNumber,
  formatPercent,
} from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { type HandoffScope, upcomingThrough } from "./analytics.outcomes.sdr";
import { readingState } from "./analytics.overview.prospectinghero";
import { metricOf } from "./analytics.overview.readings";
import {
  type Decided,
  ReasonsDrawer,
  WaitingDrawer,
} from "./analytics.overview.team.drawers";
import { figureWithheld } from "./analytics.overview.team.members";
import { useSelectionHeld } from "./reporting.evidenceselection";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
} from "./reporting.model";

type Review = components["schemas"]["SdrTeamReview"];
type Handoffs = components["schemas"]["SdrTeamHandoffs"];
type Summary = components["schemas"]["SdrSummary"];
// What the panel reads of a query: its answer, or that it failed. Neither is
// a query still pending.
type Read<T> = Readonly<{ data?: T; isError: boolean }>;
type SummaryRead = Read<Summary>;

/** The team's booked meetings, read by the summary My prospecting reads. */
export function useTeamSummary(scope: HandoffScope, enabled: boolean) {
  return useQuery({
    queryKey: ["sdr-summary", scope.id ?? scope.kind ?? "default"],
    enabled,
    queryFn: async ({ signal }): Promise<Summary> =>
      unwrap(
        await api.GET("/analytics/sdr-summary", {
          params: {
            query: {
              ...(scope.kind ? { scope_kind: scope.kind } : {}),
              ...(scope.id ? { scope_id: scope.id } : {}),
            },
          },
          signal,
        }),
      ),
  });
}

/** One thing holding the team up, its population and the way to it. */
type Bottleneck = Readonly<{
  key: string;
  title: string;
  basis: string;
  action?: Readonly<{ label: string; open: () => void }>;
}>;

type Opener = Readonly<{
  evidence: (reference: ReportingEvidenceRef) => void;
  reasons: (outcome: Decided) => void;
  waiting: () => void;
}>;

/**
 * Where the team stands: its outcomes in the period, then what holds it up
 * now, each with the population behind it one press away. A rate is shown
 * only over a cohort big enough to compare, with its denominator named.
 */
export function TeamStanding({
  review,
  readings,
  summary,
  scope,
  zone,
  onEvidence,
}: Readonly<{
  review: Review;
  readings: Read<ReportingEvaluation>;
  summary: SummaryRead;
  scope: HandoffScope;
  zone: string;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  // The population a bottleneck opens, held for this review's question only,
  // as the evidence of a reading is.
  const opened = useSelectionHeld<Decided | "waiting">(
    JSON.stringify([scope, review.interval]),
  );
  const evaluation = readings.data;
  const rows = useBottlenecks(review, readings, {
    evidence: onEvidence,
    reasons: opened.open,
    waiting: () => opened.open("waiting"),
  });
  // Nothing waits only when every input was read. A hidden, failed or pending
  // input is a row of its own, and a handoff team needs its handoffs read.
  const nothingWaits =
    rows.length === 0 &&
    (review.workflow !== "handoff" || review.handoffs !== undefined);
  return (
    <Panel title={t("analytics.team.standing")}>
      <PanelBody>
        <TeamOutcomes review={review} evaluation={evaluation} />
        <UpcomingLine summary={summary} />
      </PanelBody>
      {rows.map((row) => (
        <PanelRow key={row.key} className="analytics-attention-row">
          <div className="analytics-attention-text">
            <p className="analytics-attention-title">{row.title}</p>
            <p className="t-caption">{row.basis}</p>
          </div>
          {row.action && (
            <Button onClick={row.action.open}>{row.action.label}</Button>
          )}
        </PanelRow>
      ))}
      {nothingWaits && (
        <PanelBody>
          <p>{t("analytics.team.nothingWaits")}</p>
        </PanelBody>
      )}
      {opened.held && opened.held !== "waiting" && review.handoffs && (
        <ReasonsDrawer
          outcome={opened.held}
          review={review}
          handoffs={review.handoffs}
          zone={zone}
          onClose={opened.close}
        />
      )}
      {opened.held === "waiting" && (
        <WaitingDrawer scope={scope} review={review} onClose={opened.close} />
      )}
    </Panel>
  );
}

// The period's outcome as one cohort: of what started in the period, how much
// ended which way so far. Rates need a cohort at least the review's floor.
function TeamOutcomes({
  review,
  evaluation,
}: Readonly<{ review: Review; evaluation?: ReportingEvaluation }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const count = (value: number) => formatNumber(value, locale);
  const floor = review.small_sample_below;
  if (review.workflow === "handoff") {
    const cohort = review.handoffs?.cohort;
    if (!cohort) return null;
    return (
      <p>
        {plural("analytics.team.cohort", cohort.rounds, {
          count: count(cohort.rounds),
          accepted: count(cohort.accepted),
          returned: count(cohort.recycled),
          rejected: count(cohort.rejected),
          waiting: count(cohort.waiting),
        })}{" "}
        {cohort.rounds >= floor
          ? t("analytics.team.acceptedRate", {
              rate: formatPercent(cohort.accepted / cohort.rounds, locale),
              rounds: count(cohort.rounds),
            })
          : cohort.rounds > 0 &&
            t("analytics.team.tooFewForRate", { below: count(floor) })}{" "}
        {cohort.median_decision_hours != null &&
          t("analytics.team.medianDecision", {
            hours: formatHours(cohort.median_decision_hours, locale),
          })}
      </p>
    );
  }
  const qualified = evaluation && metricOf(evaluation, "leads_promoted");
  const arrived = qualified?.denominator;
  if (qualified?.value == null || arrived == null) return null;
  return (
    <p>
      {plural("analytics.team.qualifiedOf", arrived, {
        qualified: count(qualified.value),
        count: count(arrived),
      })}{" "}
      {arrived >= floor
        ? t("analytics.team.qualifiedRate", {
            rate: formatPercent(qualified.value / arrived, locale),
          })
        : arrived > 0 &&
          t("analytics.team.tooFewForRate", { below: count(floor) })}
    </p>
  );
}

// Meetings booked ahead, in the window and words My prospecting uses.
function UpcomingLine({ summary }: Readonly<{ summary: SummaryRead }>) {
  const t = useT();
  const { locale } = useLocale();
  const label = t("analytics.prospect.upcoming");
  if (summary.isError)
    return (
      <p className="t-caption">
        {label}: {t("analytics.prospect.failed")}
      </p>
    );
  const data = summary.data;
  if (!data) return null;
  if (data.upcoming_meetings == null || !data.upcoming_window)
    return (
      <p className="t-caption">
        {label}: {t("analytics.prospect.withheld")}
      </p>
    );
  return (
    <p className="t-caption">
      {label}: {formatNumber(data.upcoming_meetings, locale)} ·{" "}
      {upcomingThrough(data.upcoming_window, t, locale)}
    </p>
  );
}

function useBottlenecks(
  review: Review,
  readings: Read<ReportingEvaluation>,
  open: Opener,
): Bottleneck[] {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const rows: Bottleneck[] = [];
  if (review.workflow === "handoff" && figureWithheld(review, "handoffs")) {
    rows.push({
      key: "handoffs-hidden",
      title: t("analytics.team.handoffsHidden"),
      basis: t("analytics.team.hiddenBasis"),
    });
  }
  if (review.handoffs) {
    rows.push(
      ...handoffBottlenecks(review, review.handoffs, open, t, plural, locale),
    );
  }
  const unanswered = unansweredBottleneck(
    review,
    readings,
    open,
    t,
    plural,
    locale,
  );
  if (unanswered) rows.push(unanswered);
  return rows;
}

// The handoffs waiting for a decision now, with the oldest wait and how many
// wait on a recipient who can no longer act.
function waitingBottleneck(
  review: Review,
  handoffs: Handoffs,
  open: Opener,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): Bottleneck | undefined {
  const { waiting } = handoffs;
  if (waiting.waiting === 0) return undefined;
  const count = (value: number) => formatNumber(value, locale);
  const facts = [t("analytics.team.waitingBasis")];
  if (waiting.oldest_waiting_since)
    facts.push(
      t("analytics.sdrLongestWait", {
        age: formatDuration(
          Date.parse(review.as_of) - Date.parse(waiting.oldest_waiting_since),
          locale,
        ),
      }),
    );
  if (waiting.blocked > 0)
    facts.push(
      plural("analytics.team.blocked", waiting.blocked, {
        count: count(waiting.blocked),
      }),
    );
  return {
    key: "waiting",
    title: plural("analytics.team.waitingRow", waiting.waiting, {
      count: count(waiting.waiting),
    }),
    basis: facts.join(" · "),
    action: { label: t("analytics.team.showHandoffs"), open: open.waiting },
  };
}

function handoffBottlenecks(
  review: Review,
  handoffs: Handoffs,
  open: Opener,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): Bottleneck[] {
  const { outcomes, reasons } = handoffs;
  const count = (value: number) => formatNumber(value, locale);
  const waiting = waitingBottleneck(review, handoffs, open, t, plural, locale);
  const rows: Bottleneck[] = waiting ? [waiting] : [];
  for (const outcome of ["recycled", "rejected"] as const) {
    const total =
      outcome === "recycled" ? outcomes.recycled : outcomes.rejected;
    if (total === 0) continue;
    const top = reasons.find((reason) => reason.outcome === outcome);
    const facts = [t("analytics.team.decidedBasis")];
    if (top)
      facts.push(
        t("analytics.team.mostOften", {
          reason: top.given ? top.label : t("analytics.team.noReason"),
        }),
      );
    rows.push({
      key: outcome,
      title: plural(
        outcome === "recycled"
          ? "analytics.team.returnedRow"
          : "analytics.team.rejectedRow",
        total,
        { count: count(total) },
      ),
      basis: facts.join(" · "),
      action: {
        label: t("analytics.team.showReasons"),
        open: () => open.reasons(outcome),
      },
    });
  }
  return rows;
}

// Leads that still owe a first reply. Hidden, failed and pending are each a
// row of their own, so only a measured zero leaves the row out.
function unansweredBottleneck(
  review: Review,
  readings: Read<ReportingEvaluation>,
  open: Opener,
  t: ReturnType<typeof useT>,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): Bottleneck | undefined {
  const hidden = {
    key: "unanswered",
    title: t("analytics.team.unansweredHidden"),
    basis: t("analytics.team.hiddenBasis"),
  };
  const failed = {
    key: "unanswered",
    title: t("analytics.team.unansweredFailed"),
    basis: t("analytics.prospect.failedDetail"),
  };
  if (figureWithheld(review, "leads") || figureWithheld(review, "unanswered"))
    return hidden;
  if (readings.isError) return failed;
  if (!readings.data)
    return {
      key: "unanswered",
      title: t("analytics.team.unansweredPending"),
      basis: t("analytics.readingLoading"),
    };
  const metric = metricOf(readings.data, "leads_awaiting_response");
  if (metric?.coverage.withheld) return hidden;
  const state = readingState(metric);
  if (state === "failed" || state === "other" || !metric) return failed;
  const value = metric.value ?? 0;
  if (value === 0) return undefined;
  return {
    key: "unanswered",
    title: plural("analytics.team.unansweredRow", value, {
      count: formatNumber(value, locale),
    }),
    basis: t("analytics.team.unansweredBasis"),
    action: {
      label: t("analytics.team.showLeads"),
      open: () => open.evidence(metric.evidence),
    },
  };
}
