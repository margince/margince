// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { MessageKey } from "../i18n/en";
import { en } from "../i18n/en";
import type { WeeklyReview } from "./brief.queries";
import { weekSentence } from "./brief.weeksentence";

// The catalog's own words, filled the way the app fills them. A second copy of
// the templates here would let this suite pass over a sentence the product
// never prints.
function t(key: MessageKey, values?: Record<string, string>): string {
  // Widened to a plain string: the catalog's values are a union of every
  // literal it holds, and a reduce that substitutes into one cannot stay inside
  // that union.
  const template: string = en[key];
  return Object.entries(values ?? {}).reduce(
    (text, [name, value]) => text.replaceAll(`{${name}}`, value),
    template,
  );
}

function say(review: WeeklyReview | null | undefined): string | null {
  const sentence = weekSentence(review, t);
  return sentence === null ? null : t(sentence.key, sentence.values);
}

const ZERO_COUNTS = {
  tasks_due: 0,
  tasks_done: 0,
  tasks_carried_over: 0,
  deals_moved: 0,
  deals_won: 0,
  deals_lost: 0,
  proposals_accepted: 0,
  proposals_rejected: 0,
  brief_items_acted: 0,
  brief_items_dismissed: 0,
  commitments_due: 0,
  commitments_kept: 0,
  leads_routed: 0,
  leads_answered_in_target: 0,
  leads_breached: 0,
  meetings_held: 0,
  meetings_with_next_step: 0,
};

function week(
  counts: Partial<typeof ZERO_COUNTS>,
  pipeline?: NonNullable<WeeklyReview["pipeline"]>,
): WeeklyReview {
  return {
    id: "22222222-2222-2222-2222-222222222222",
    local_week_start: "2026-06-29",
    generated_at: "2026-07-06T06:00:00Z",
    as_of: "2026-07-06T06:00:00Z",
    deals: [],
    counts: { ...ZERO_COUNTS, ...counts },
    ...(pipeline === undefined ? {} : { pipeline }),
  };
}

describe("weekSentence — what the closed week says about itself", () => {
  // The distinction the whole composer exists for. A read that never landed is
  // not a quiet week, and saying so would send a rep away believing their week
  // was calm on no evidence at all.
  it("says nothing at all about a week it has not read", () => {
    expect(say(undefined)).toBeNull();
  });

  // 404 from the weekly read: no week has been written yet. Same silence, same
  // reason — there is no week to describe.
  it("says nothing about a week nobody has written", () => {
    expect(say(null)).toBeNull();
  });

  it("leads with what the week closed", () => {
    expect(say(week({ deals_won: 2 }))).toContain("Deals won: 2");
  });

  // What the wins were WORTH belongs to the Won card's detail line, which gave
  // up its week-on-week delta to carry it (#3898). A week whose money the
  // review DOES record must still not print it here, or one figure is on the
  // page twice and a reader has two places to reconcile.
  it("never prices the wins, even when the week recorded the money", () => {
    const priced = say(
      week(
        { deals_won: 2 },
        {
          created_minor: 0,
          won_minor: 4200000,
          lost_minor: 0,
          currency: "EUR",
        },
      ),
    );
    expect(priced).toBe(say(week({ deals_won: 2 })));
    expect(priced).not.toContain("42");
  });

  // Ranked by what a week is judged on, not by the bigger integer: twelve
  // routed leads must not outrank the deal that paid for the quarter.
  it("puts a single win ahead of a busier count of anything else", () => {
    const said = say(week({ deals_won: 1, deals_moved: 9, meetings_held: 12 }));
    expect(said).toContain("Deals won: 1");
    expect(said).not.toContain("9");
  });

  it("falls back to what moved, then to what was held", () => {
    expect(say(week({ deals_moved: 3, meetings_held: 12 }))).toContain(
      "Deals moved forward: 3",
    );
    expect(say(week({ meetings_held: 12 }))).toContain("Meetings held: 12");
  });

  // A promise the rep made and did not keep is the first debt, because they
  // said they would do it.
  it("names the promises that did not survive the week", () => {
    const said = say(
      week({ deals_won: 1, commitments_due: 4, commitments_kept: 1 }),
    );
    expect(said).toContain("Commitments carried over: 3");
  });

  // A commitment DROPPED is in neither figure — the schema says deciding on
  // Wednesday that a thing is not worth doing is not failing to do it. Kept
  // equal to due therefore leaves no debt, whatever was dropped.
  it("charges nothing to a week that kept every promise it made", () => {
    const said = say(
      week({ deals_won: 1, commitments_due: 4, commitments_kept: 4 }),
    );
    expect(said).not.toContain("Commitments");
  });

  it("falls back to postponed tasks when no promise was missed", () => {
    expect(say(week({ deals_won: 1, tasks_carried_over: 2 }))).toContain(
      "Tasks carried over: 2",
    );
  });

  // A week with no result of its own still has a true thing to say, and silence
  // would read as a page that failed to load.
  it("calls a week with nothing in it quiet, and still names its debt", () => {
    expect(say(week({}))).toBe(en["brief.week.quiet"]);
    expect(say(week({ tasks_carried_over: 2 }))).toContain("No completed work");
    expect(say(week({ tasks_carried_over: 2 }))).toContain(
      "Tasks carried over: 2",
    );
  });
});

it("recognizes completed undated work in the weekly headline", () => {
  const review = week({});
  review.counts.tasks_completed = 1;
  expect(say(review)).toBe(en["brief.week.workRecorded"]);
});

it("recognizes obligations delivered early even without an in-week completion", () => {
  const review = week({ tasks_due: 3, tasks_done: 3 });
  review.counts.tasks_completed = 0;
  expect(say(review)).toBe(en["brief.week.workRecorded"]);
});
