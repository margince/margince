// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { useRecordZone } from "../app/recordzone";
import { StatCard } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { StatStrip } from "../design-system/statstrip";
import { type SectionState, SurfaceState } from "../design-system/surfacestate";
import { ProvenanceTag } from "../design-system/trust";
import {
  formatDate,
  formatMoney,
  formatNumber,
  formatSignedNumber,
} from "../format/format";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import {
  useWeeklyReview,
  useWeeklyReviewIndex,
  type WeeklyReview,
} from "./home.queries";

import "./home.weekly.css";

// Derived from the review's own deal shape rather than reached for separately:
// one import, and the outcome vocabulary cannot drift from the payload the
// panel actually renders.
type WeeklyReviewDealOutcome = WeeklyReview["deals"][number]["outcome"];

// The week just gone, on Home.
//
// NO NAV ENTRY, deliberately. The product's own argument against one is in
// nav.ts: Today is the single door to the work that waits on a person, and
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
  const [week, setWeek] = useState<string | undefined>(undefined);
  const review = useWeeklyReview(week);
  const index = useWeeklyReviewIndex();

  return (
    <section id="home-weekly" aria-label={t("home.panel.weekly")}>
      <Panel
        title={t("home.panel.weekly")}
        sub={
          review.data
            ? t("home.weekly.weekOf", {
                day: formatDate(
                  review.data.local_week_start,
                  locale,
                  recordZone,
                ),
              })
            : undefined
        }
        titleAction={
          index.data && index.data.length > 1 ? (
            <Select
              aria-label={t("home.weekly.pickWeek")}
              value={week ?? index.data[0]}
              onChange={(next) => setWeek(next)}
              options={index.data.map((start) => ({
                value: start,
                label: formatDate(start, locale, recordZone),
              }))}
            />
          ) : undefined
        }
      >
        <PanelBody>
          <WeeklyBody review={review.data ?? null} state={readState(review)} />
        </PanelBody>
      </Panel>
    </section>
  );
}

/**
 * The week's workings, under the strip.
 *
 * Five readings that answer "how did the week go" rather than "what did the week
 * produce" — how much of the queue was worked, how proposals were decided, how
 * many deals moved without closing. They were slots six to ten of a ten-slot
 * strip, where they made the row fold into two ranks and cost the outcomes their
 * one-comparison reading.
 *
 * A definition list, not more cards: these are looked up one at a time by
 * somebody who already read the strip, which is the opposite of the strip's
 * read-across claim.
 */
function WeeklyWorkings({
  counts,
}: Readonly<{ counts: WeeklyReview["counts"] }>) {
  const t = useT();
  const { locale } = useLocale();
  const n = (value: number) => formatNumber(value, locale);
  return (
    <dl className="home-weekly-workings">
      <Working
        label={t("home.weekly.tasksDelivered")}
        value={t("home.weekly.ofDue", {
          done: n(counts.tasks_done),
          due: n(counts.tasks_due),
        })}
      />
      <Working
        label={t("home.weekly.dealsMoved")}
        value={n(counts.deals_moved)}
      />
      <Working
        label={t("home.weekly.dealsLost")}
        value={n(counts.deals_lost)}
      />
      <Working
        label={t("home.weekly.decided")}
        value={t("home.weekly.acceptedRejected", {
          accepted: n(counts.proposals_accepted),
          rejected: n(counts.proposals_rejected),
        })}
      />
      <Working
        label={t("home.weekly.queueWorked")}
        value={t("home.weekly.actedDismissed", {
          acted: n(counts.brief_items_acted),
          dismissed: n(counts.brief_items_dismissed),
        })}
      />
    </dl>
  );
}

function Working({ label, value }: Readonly<{ label: string; value: string }>) {
  return (
    <div className="home-weekly-working">
      <dt className="t-caption">{label}</dt>
      <dd className="t-body">{value}</dd>
    </div>
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
  const t = useT();
  if (!review.narrated_at) {
    return (
      <p className="home-weekly-narrative home-weekly-narrative-absent t-caption">
        {t("home.weekly.noNarrative")}
      </p>
    );
  }
  if (!review.narrative) {
    return null;
  }
  return (
    <div className="home-weekly-narrative">
      <ProvenanceTag provenance={{ kind: "agent" }} />
      <p>{review.narrative}</p>
    </div>
  );
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
      return t("home.weekly.outcome.won");
    case "lost":
      return t("home.weekly.outcome.lost");
    default:
      return t("home.weekly.outcome.moved");
  }
}

/** What one read says about itself, in the state vocabulary every section
 *  draws from — the same three-way answer Home's other panels give. */
function readState(
  query: Readonly<{ isError: boolean; isPending: boolean }>,
): SectionState {
  if (query.isError) {
    return "failed";
  }
  return query.isPending ? "loading" : "ready";
}

/**
 * What the week's wins were worth, or nothing.
 *
 * NOTHING, not a zero, when the review carries no pipeline block. That block is
 * optional on the wire: a week assembled before the money columns existed, or
 * one where an FX rate was missing, has no honest figure — and "€0 won" is a
 * claim about a week nobody measured, which is the opposite of what happened.
 *
 * The currency comes from the review, never from the installation's current
 * setting. Base currency is operator-mutable, so re-reading it later would
 * re-label an old week with a currency its numbers were never in. The contract
 * stores it beside the figures for exactly this reason.
 */
function wonValue(review: WeeklyReview, locale: Locale): string | undefined {
  const pipeline = review.pipeline;
  if (pipeline === undefined) {
    return undefined;
  }
  return formatMoney(pipeline.won_minor, pipeline.currency, locale);
}

function WeeklyBody({
  review,
  state,
}: Readonly<{ review: WeeklyReview | null; state: SectionState }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();

  if (state !== "ready") {
    return (
      <SurfaceState
        state={state}
        emptyLabel={t("home.weekly.none")}
        loadingLabel={t("home.panel.weekly")}
      >
        {null}
      </SurfaceState>
    );
  }
  if (review === null) {
    // A rep whose first Monday has not come round yet. Saying so is the honest
    // answer; a page of zeroes would claim a week that was measured and empty.
    return (
      <p className="home-weekly-none t-caption">{t("home.weekly.none")}</p>
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
      <span className="home-weekly-delta t-caption">
        {t("home.weekly.sincePrior", {
          delta: formatSignedNumber(delta, locale),
        })}
      </span>
    );
  };
  return (
    <>
      <WeeklyNarrative review={review} />
      {/* FIVE slots, because a strip is read ACROSS as one comparison and ten
          is a table wearing a strip's clothes — at 1280 the row folded to two
          ranks of five and stopped being one reading at all (#3709).
          These five are the week's outcomes: what the rep planned and kept,
          what closed, how fast new business was answered, whether meetings led
          anywhere, and what did not get finished. The other five are workings
          — how the queue was worked, how proposals were decided — and they
          read as a list under the strip, where they are still available to
          anyone who wants them and no longer compete with the outcomes. */}
      <StatStrip testId="weekly-strip">
        <StatCard
          label={t("home.weekly.planCommitmentsKept")}
          value={t("home.weekly.ofDue", {
            done: formatNumber(c.commitments_kept, locale),
            due: formatNumber(c.commitments_due, locale),
          })}
          detail={since(c.commitments_kept, prior?.commitments_kept)}
        />
        <StatCard
          label={t("home.weekly.dealsWon")}
          value={formatNumber(c.deals_won, locale)}
          // What those wins were WORTH, at each deal's own close-time rate.
          //
          // The count alone says a week of five small renewals and a week of
          // one company-making deal are the same week. The money was computed,
          // FX-converted, stored with the currency it is in, and served — and
          // read by nothing until now.
          //
          // It rides the won slot rather than taking a sixth: five is what a
          // strip can be read across as one comparison, and a tenth slot folded
          // the row into two ranks at 1280 (#3709). The delta line gives way to
          // it, because "what it was worth" is the fact a reader wants first
          // and the strip has one detail line to give.
          detail={
            wonValue(review, locale) ?? since(c.deals_won, prior?.deals_won)
          }
        />
        <StatCard
          label={t("home.weekly.leadsAnswered")}
          value={t("home.weekly.ofRouted", {
            answered: formatNumber(c.leads_answered_in_target, locale),
            routed: formatNumber(c.leads_routed, locale),
          })}
          detail={since(
            c.leads_answered_in_target,
            prior?.leads_answered_in_target,
          )}
        />
        <StatCard
          label={t("home.weekly.meetingsHeld")}
          value={t("home.weekly.ofMeetings", {
            withStep: formatNumber(c.meetings_with_next_step, locale),
            held: formatNumber(c.meetings_held, locale),
          })}
          detail={since(c.meetings_held, prior?.meetings_held)}
        />
        <StatCard
          label={t("home.weekly.carriedOver")}
          value={formatNumber(c.tasks_carried_over, locale)}
          detail={since(c.tasks_carried_over, prior?.tasks_carried_over)}
        />
      </StatStrip>
      <WeeklyWorkings counts={c} />
      {review.deals.length > 0 && (
        <ul className="home-weekly-deals">
          {review.deals.map((deal) => (
            <li key={`${deal.deal_id}-${deal.occurred_at}`}>
              {/* The LABEL, not a lookup. It was frozen when the review was
                  written, so a deal renamed or deleted since still reads as it
                  did that week. */}
              <span className="home-weekly-deal-name">{deal.label}</span>
              <span className="home-weekly-deal-outcome t-caption">
                {outcomeWord(t, deal.outcome)}
                {deal.to_stage_label ? ` · ${deal.to_stage_label}` : ""}
              </span>
              <time
                className="home-weekly-deal-when t-caption"
                dateTime={deal.occurred_at}
              >
                {formatDate(deal.occurred_at, locale, recordZone)}
              </time>
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
