// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Worklist } from "./worklist.queries";

// What the readings strip is allowed to CLAIM, separated from how it draws.
//
// Both answers here are about honesty rather than layout: which figures are
// floors, and how many decisions genuinely hold somebody up. They live beside
// the strip rather than inside it because each is a question about the day's
// data that a test can ask directly, without rendering five cards to find out.

/** The lanes the strip's four worklist figures are summed from. */
export const MEETINGS = "meetings";
export const LEADS = "leads";
export const DECISIONS = "decisions";

// Which categories came back at a bound, as the server marked them.
//
// An ABSENT category is not a bounded one. The server seeds `counts` from the
// bounded sources BEFORE it walks the rows (reach.go), so a lane that stopped
// early always leaves an entry even when the scope filter took every row it
// found. A category with no entry at all therefore had no bound and no rows —
// an honest nothing — and marking it would put a `+` on the zeros.
//
// A lane that could not be read AT ALL is a different fact and is not in
// `counts`: it travels in `sources_unavailable`, which names a SOURCE, and only
// the server maps a source to its lane. The caller keeps that case strip-wide.
export function boundedCategories(day: Worklist): ReadonlySet<string> {
  const out = new Set<string>();
  for (const count of day.counts) {
    if (count.more_available) {
      out.add(count.category);
    }
  }
  return out;
}

// How many of the decisions genuinely hold somebody up, or null where the page
// cannot honestly say.
//
// The strip used to claim "somebody is blocked until you answer" under every
// pending decision, whatever kind. Most are not: the ranker puts a decision
// about a SEND at the blocking level and contact hygiene — capturing a
// counterparty, merging two records — a level below it, because a queue of
// identical contact questions must never read as a customer waiting
// (classifydecision.go). Claiming a person is held up by a duplicate pair is
// the sentence that teaches a reader to discount the line.
//
// Counted off the rows' own `work_blocked` consequence, which is the server's
// answer to the same question, so the strip cannot disagree with the row a
// reader opens.
//
// NULL WHERE THE PAGE IS NOT THE WHOLE LANE, and this is the care the figure
// needs. The count beside it is `readings.review`, taken over everything the
// read weighed; this one can only be taken over the rows the page carries. On a
// day whose decisions ran past page one those are different populations, and
// pairing them prints "30 · 25 holding up customer work" when the true number
// is higher — understating a block, silently, which is the one direction this
// line must not fail in. So it is claimed ONLY where the page carries every
// decision the read counted, the way the leads deadline and the meetings
// readiness are.
export function decisionsBlocking(day: Worklist): number | null {
  const entry = day.counts.find((count) => count.category === DECISIONS);
  if (entry === undefined) {
    // No decision was read at all: nothing to be blocked by, and nothing
    // missing either.
    return 0;
  }
  if (entry.shown !== entry.considered || entry.more_available) {
    return null;
  }
  return day.queue.filter(
    (item) =>
      item.category === DECISIONS && item.consequence === "work_blocked",
  ).length;
}
