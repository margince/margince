// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../i18n/en";
import type { BriefSentence } from "./brief.sentence";
import type { WeeklyReview } from "./home.queries";

// The weekly Brief's opening sentence, composed from the counts the week was
// frozen with.
//
// Same construction rule as the morning's `briefSentence`, and for the same
// reason: it is built from the very figures the strip below draws, so the
// sentence cannot say something those figures contradict, and no pass has to
// have run before the page can speak. It carries no agent tag — indigo means
// Margince decided, and a client-composed sentence marked that way would claim
// an authorship that is not true.
//
// What it says is fixed by two questions a closed week answers: what moved, and
// what did not finish. The result leads because a rep opening a closed week
// wants the outcome before the debt; the carry follows because it is the part
// that is still theirs on Monday.

/**
 * The week's largest movement, as the key naming it and the figure to fill.
 *
 * Ordered by what a week is judged on rather than by magnitude: a won deal
 * outranks a leads figure however many leads there were, because the numbers
 * are not comparable and picking the bigger integer would let twelve routed
 * leads outrank the deal that paid for the quarter.
 */
function resultOf(
  review: WeeklyReview,
): { key: MessageKey; values: Record<string, string> } | null {
  const c = review.counts;
  if (c.deals_won > 0) {
    // THE COUNT, NOT THE MONEY. What the wins were worth is already the Won
    // card's detail line, which gave up its week-on-week delta to carry it
    // (#3898) — and a figure printed twice on one page is two places a reader
    // has to reconcile. The sentence says what the week did; the strip says
    // what it was worth.
    return { key: "brief.week.won", values: { count: String(c.deals_won) } };
  }
  if (c.deals_moved > 0) {
    return {
      key: "brief.week.moved",
      values: { count: String(c.deals_moved) },
    };
  }
  if (c.meetings_held > 0) {
    return {
      key: "brief.week.met",
      values: { count: String(c.meetings_held) },
    };
  }
  return null;
}

/**
 * What the week left behind, or null when it left nothing.
 *
 * A commitment that was due and not kept is the first carry, because the rep
 * said they would do it. Postponed tasks come second: they are older than the
 * week and were already postponed at least once.
 */
function carryOf(
  review: WeeklyReview,
): { key: MessageKey; values: Record<string, string> } | null {
  const c = review.counts;
  const missed = c.commitments_due - c.commitments_kept;
  if (missed > 0) {
    return {
      key: "brief.week.carryPromises",
      values: { count: String(missed) },
    };
  }
  if (c.tasks_carried_over > 0) {
    return {
      key: "brief.week.carryTasks",
      values: { count: String(c.tasks_carried_over) },
    };
  }
  return null;
}

/**
 * The closed week in one sentence, or null when there is no week to describe.
 *
 * Null on an unread week, deliberately — the same distinction the panel below
 * keeps between a week that was quiet and a read that never landed. A page that
 * said "a quiet week" over a failed read would tell a rep their week was calm
 * on no evidence at all.
 */
export function weekSentence(
  review: WeeklyReview | null | undefined,
  t: (key: MessageKey, values?: Record<string, string>) => string,
): BriefSentence | null {
  if (!review) {
    return null;
  }
  const result = resultOf(review);
  const carry = carryOf(review);
  // A week with no result of its own still has a true thing to say. It says the
  // week was quiet — never a manufactured outcome, and never silence, which a
  // reader would take for a page that failed to load.
  const said =
    result === null
      ? { key: "brief.week.quiet" as MessageKey, values: {} }
      : result;
  if (carry === null) {
    return said;
  }
  // ONE SENTENCE, TWO CLAUSES, and the join is a whole key rather than a
  // template that glues two rendered strings together. A translator gets the
  // sentence with both holes in it, which is what lets German put the clauses
  // where German puts them — the same reason the morning stores `.one` and
  // `.oneWithCost` separately instead of appending the cost.
  return {
    key: "brief.week.andCarry",
    values: {
      result: t(said.key, said.values),
      carry: t(carry.key, carry.values),
    },
  };
}
