// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { routeHash } from "../app/router";
import { useUrlParams } from "../app/urlstate";
import { Badge, Disclosure, StatCard } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { StatStrip } from "../design-system/statstrip";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { ProvenanceTag } from "../design-system/trust";
import { middayInstant } from "../format/calendarday";
import {
  formatDate,
  formatDateTime,
  formatMoney,
  formatNumber,
  formatSignedMoney,
  formatSignedNumber,
} from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { openAnalyticsSection } from "./analytics.address";
import {
  useWeeklyReview,
  useWeeklyReviewIndex,
  type WeeklyReview,
} from "./brief.queries";
import { OutlookPanel } from "./brief.waterfall";
import { LearningsPanel } from "./brief.weekly.learnings";
import { ScorecardPanel } from "./brief.weekly.scorecard";
import { WeeklyWorkings } from "./brief.weekly.workings";

import "./brief.weekly.css";

// Derived from the review's own deal shape rather than reached for separately:
// one import, and the outcome vocabulary cannot drift from the payload the
// panel actually renders.
type WeeklyReviewDealOutcome = WeeklyReview["deals"][number]["outcome"];

// The week just gone, on Brief.
//
// NO NAV ENTRY, deliberately. The product's own argument against one is in
// nav.ts: Today is the single door to the work that waits on a contact, and
// three sidebar rows for one question read as three separate piles. A
// retrospective of the week is a view of that same work, so it lives here and
// past weeks open through the picker rather than through a second destination.
//
// It renders what was WRITTEN when the week closed. Nothing here recomputes:
// a retrospective that changed when you reopened it would not be one.

export function WeeklySection() {
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

  return (
    <section id="brief-weekly">
      <Panel
        title={t("brief.panel.weekly")}
        // The mark and the way out of the week, in that order.
        //
        // FROZEN is the claim that separates this panel from every other on
        // Brief: the numbers under it were written when the week closed and can
        // no longer move, so a rep who acts on Tuesday and re-reads on Thursday
        // is not looking at a stale figure — they are looking at a record. The
        // team weekly has said so since it shipped; the rep's, which is the one
        // a rep actually opens, did not.
        //
        // Drawn only over a review that exists, so an empty or failed read does
        // not certify a week nobody wrote.
        titleAction={
          <span className="brief-weekly-mark">
            {review.data && (
              <>
                <Badge>{t("brief.weekly.frozen")}</Badge>
                {/* Two badges, two different facts: the one beside it says how
                    settled the week is, this says part of what is under it was
                    written by a model. Drawn only when a narrative actually
                    arrived — the numbers under it are a deterministic pass, so
                    a standing mark would claim the whole panel. */}
                {narratedByModel(review.data) && (
                  <Badge tone="ai">{t("co.assistant.aiTag")}</Badge>
                )}
                {/* When it was written, which is what makes the badge a fact
                    rather than a decoration — a reader can tell a week closed
                    an hour ago from one closed on Monday. */}
                <span className="t-caption">
                  {t("brief.weekly.written", {
                    at: formatDateTime(
                      review.data.generated_at,
                      locale,
                      recordZone,
                    ),
                  })}
                </span>
              </>
            )}
            {index.data && index.data.length > 1 && (
              <Select
                aria-label={t("brief.weekly.pickWeek")}
                value={week ?? index.data[0]}
                onChange={(next) => setWeek(next)}
                options={index.data.map((start) => ({
                  value: start,
                  label: formatDate(
                    middayInstant(start, recordZone),
                    locale,
                    recordZone,
                  ),
                }))}
              />
            )}
          </span>
        }
      >
        {/* No body around the whole week: the panel's sections are its own
            direct children, so each one takes the pane's interval and the seam
            between two of them is the panel's rather than a margin a screen
            picked. */}
        <WeeklyBody review={review.data ?? null} state={readState(review)} />
      </Panel>
    </section>
  );
}

/**
 * The sentence about the week, above its numbers.
 *
 * THREE STATES, and the third is the one worth the code. A review with no
 * sentence can mean a pass ran and found the week unremarkable, or that no pass
 * ran at all — no model bound, an exhausted budget, a provider outage. Those
 * read identically as silence, so `narrated_at` separates them and the panel
 * says which.
 *
 * Saying nothing in the third case would be the dishonest option: the rep would
 * read a week with no remark and conclude there was nothing to remark on, when
 * in fact nobody looked.
 */
function WeeklyNarrative({ review }: Readonly<{ review: WeeklyReview }>) {
  if (!review.narrated_at) {
    return null;
  }
  if (!review.narrative) {
    return null;
  }
  // Indigo on the SENTENCE and not on the panel around it. The panel also
  // carries the week's outlook, its scorecard and its five frozen figures, and
  // every one of those is a deterministic pass — a tinted panel head would
  // claim a model wrote the numbers too.
  return (
    <Panel tone="ai" className="brief-weekly-narrative">
      <PanelBody className="brief-weekly-narrative-text">
        <ProvenanceTag provenance={{ kind: "agent" }} />
        <p>{review.narrative}</p>
      </PanelBody>
    </Panel>
  );
}

/** Whether a model actually wrote a sentence about this week — which is neither
 *  "a pass ran" nor "there is a sentence field". Both the head's badge and the
 *  narrative's own tint answer to it, so they cannot come to disagree. */
function narratedByModel(review: WeeklyReview): boolean {
  return Boolean(review.narrated_at && review.narrative);
}

/** The outcome as a word. A lookup rather than a template key, because a
 *  template key built from wire data is an unchecked assertion: the contract's
 *  enum can grow and the interpolation would ask for a message nobody wrote. */
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
 *  draws from — the same three-way answer Brief's other panels give. */
function readState(
  query: Readonly<{ isError: boolean; isPending: boolean }>,
): SectionState {
  if (query.isError) {
    return "failed";
  }
  return query.isPending ? "loading" : "ready";
}

/**
 * What the week's wins were worth, and whether that beat the week before.
 *
 * THE PACE NUMBER, and it lives here rather than on the morning for a reason
 * the morning's own contract states: every figure in that strip describes the
 * same set — today's queue, before filtering — which is what keeps those
 * numbers stable as a rep works down the page. Money closed over a week is a
 * different population, and standing it beside four same-set figures would put
 * two measurements in one row with nothing saying they differ.
 *
 * Here the whole panel is already about one closed week, so a week-on-week
 * comparison is the question the surface exists to answer.
 *
 * BOTH WEEKS OR NEITHER for the delta. A prior week with no pipeline block is
 * one nobody could price, not one that earned nothing — so it yields no
 * comparison rather than a change measured against a zero that was never a
 * figure. The value still draws; only the comparison is withheld.
 *
 * The currencies must MATCH. Each week is stored in the currency it was
 * measured in, and an operator who changed the base currency mid-quarter leaves
 * two weeks whose numbers are not comparable. Subtracting across them would
 * print a change nobody can act on.
 */
function wonPace(
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

function WeeklyBody({
  review,
  state,
}: Readonly<{ review: WeeklyReview | null; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  // Which horizon the outlook strip is showing. Per-reader and not persisted:
  // it is a way of looking at one review, not a setting about the account.
  const [horizon, setHorizon] = useState<string>("quarter");

  if (state !== "ready") {
    return (
      <SurfaceState
        state={state}
        emptyLabel={t("brief.weekly.none")}
        loadingLabel={t("brief.panel.weekly")}
      >
        {null}
      </SurfaceState>
    );
  }
  if (review === null) {
    // A rep whose first Monday has not come round yet. Saying so is the honest
    // answer; a page of zeroes would claim a week that was measured and empty.
    // Drawn through the same arm the branch above uses for the same sentence —
    // hand-rolled here, it was the one line on the card set as a caption while
    // every other absence on the page read as prose.
    return (
      <SurfaceState
        state="empty"
        emptyLabel={t("brief.weekly.none")}
        loadingLabel={t("brief.panel.weekly")}
      >
        {null}
      </SurfaceState>
    );
  }

  const c = review.counts;
  const prior = review.prior?.counts;
  // The delta line, or nothing. A reading with no earlier week to measure
  // against gets no line at all rather than "+0": a rep's first week did not
  // stay level, it had nothing to stay level against.
  const since = (now: number, before: number | undefined) => {
    if (before === undefined) {
      return undefined;
    }
    const delta = now - before;
    return (
      <span className="t-caption">
        {t("brief.weekly.sincePrior", {
          delta: formatSignedNumber(delta, locale),
          week: formatDate(
            middayInstant(
              review.prior?.local_week_start ?? review.local_week_start,
              recordZone,
            ),
            locale,
            recordZone,
          ),
        })}
      </span>
    );
  };
  return (
    <>
      {/* The week SAID, in one block: the sentence about it and where it was
          landing. Two readings of the same week, so they share a body and the
          body's own stack sets the interval — before this each paid a browser
          margin and an empty state's padding on top of it, which put two
          sentences 40px apart with nothing between them. */}
      <PanelBody className="brief-weekly-outcomes">
        <p className="t-sub">{t("brief.weekly.basis")}</p>
        {/* On a phone the strip is a list, not ten boxes stacked. */}
        <StatStrip testId="weekly-strip">
          {c.tasks_completed !== undefined && (
            <StatCard
              narrow="row"
              label={t("brief.weekly.tasksCompleted")}
              value={formatNumber(c.tasks_completed, locale)}
              detail={since(c.tasks_completed, prior?.tasks_completed)}
            />
          )}
          <StatCard
            narrow="row"
            label={t("brief.week.lostLabel")}
            value={formatNumber(c.deals_lost, locale)}
            detail={since(c.deals_lost, prior?.deals_lost)}
          />
          <StatCard
            narrow="row"
            label={t("brief.week.movedLabel")}
            value={formatNumber(c.deals_moved, locale)}
            detail={since(c.deals_moved, prior?.deals_moved)}
          />
          <StatCard
            narrow="row"
            label={t("brief.weekly.planCommitmentsKept")}
            value={
              c.commitments_due === 0
                ? t("brief.weekly.noCommitments")
                : t("brief.weekly.ofDue", {
                    done: formatNumber(c.commitments_kept, locale),
                    due: formatNumber(c.commitments_due, locale),
                  })
            }
            detail={since(c.commitments_kept, prior?.commitments_kept)}
          />
          <StatCard
            narrow="row"
            label={t("brief.weekly.dealsWon")}
            value={formatNumber(c.deals_won, locale)}
            // Value uses the frozen close-time exchange rates.
            detail={
              wonPace(review, locale, t, recordZone) ??
              since(c.deals_won, prior?.deals_won)
            }
          />
          <StatCard
            narrow="row"
            label={t("brief.weekly.leadsAnswered")}
            value={
              c.leads_routed === 0
                ? t("brief.weekly.noLeads")
                : t("brief.weekly.ofRouted", {
                    answered: formatNumber(c.leads_answered_in_target, locale),
                    routed: formatNumber(c.leads_routed, locale),
                  })
            }
            detail={since(
              c.leads_answered_in_target,
              prior?.leads_answered_in_target,
            )}
          />
          <StatCard
            narrow="row"
            label={t("brief.weekly.meetingsHeld")}
            value={
              c.meetings_held === 0
                ? t("brief.weekly.noMeetings")
                : t("brief.weekly.ofMeetings", {
                    withStep: formatNumber(c.meetings_with_next_step, locale),
                    held: formatNumber(c.meetings_held, locale),
                  })
            }
            detail={since(c.meetings_held, prior?.meetings_held)}
          />
          <StatCard
            narrow="row"
            label={t("brief.weekly.carriedOver")}
            value={formatNumber(c.tasks_carried_over, locale)}
            detail={since(c.tasks_carried_over, prior?.tasks_carried_over)}
          />
        </StatStrip>
        <Disclosure summary={t("brief.readings.summary")}>
          <WeeklyWorkings counts={c} />
        </Disclosure>
      </PanelBody>
      {review.deals.length > 0 && (
        <PanelBody>
          <ul className="brief-weekly-deals">
            {review.deals.map((deal) => (
              <li key={`${deal.deal_id}-${deal.occurred_at}`}>
                {/* The LABEL, not a lookup. It was frozen when the review was
                    written, so a deal renamed or deleted since still reads as
                    it did that week — which is why this is a plain anchor and
                    not `EntityRef`: resolving the name would undo the freeze.
                    The ADDRESS is safe to build from the frozen id either way;
                    a deal that has since gone answers 404, which is the honest
                    outcome for a week that is over. */}
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
            ))}
          </ul>
        </PanelBody>
      )}
      <Disclosure summary={t("brief.week.supporting")}>
        <PanelBody className="brief-weekly-week">
          <WeeklyNarrative review={review} />
          {/* Where the week was landing, before what the rep did about it: a
            retrospective is read outcome-first, and the counts below answer
            "what did I do" against the figure this answers "about what". */}
          <OutlookPanel
            outlook={review.outlook ?? []}
            locale={locale}
            horizon={horizon}
            onHorizon={setHorizon}
            onOpenForecast={() => openAnalyticsSection("forecast")}
          />
        </PanelBody>
        {/* How well the week went, after where it was landing and before the
          outcome strip's tallies. Absent blocks draw nothing at all — the
          panel never substitutes zeros for work the rep did not have. */}
        <ScorecardPanel scorecard={review.scorecard} />
        {/* What the week TAUGHT, after how well it went. Last because it is the
          only part of the retrospective that is a claim rather than a count,
          and a reader should meet the numbers before the lessons drawn from
          them. */}
        <LearningsPanel learnings={review.learnings} />
        {/* FIVE slots, because a strip is read ACROSS as one comparison and ten
          is a table wearing a strip's clothes — at 1280 the row folded to two
          ranks of five and stopped being one reading at all (#3709).
          These five are the week's outcomes: what the rep planned and kept,
          what closed, how fast new business was answered, whether meetings led
          anywhere, and what did not get finished. The other five are workings
          — how the queue was worked, how proposals were decided — and they
          read as a list under the strip, where they are still available to
          anyone who wants them and no longer compete with the outcomes. */}
      </Disclosure>
    </>
  );
}
