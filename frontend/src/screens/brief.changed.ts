// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { waitingRows } from "./brief.sentence";
import { worklistLaneHref } from "./worklist.header";
import type { Worklist } from "./worklist.queries";

// What has happened since the night looked.
//
// The morning is assembled overnight and read hours later, and the gap is where
// a rep gets caught out: a buyer replies at 07:15, the page still reflects 06:00,
// and the row that changed sits in the same position saying the same thing. The
// feed is correctly ordered either way — the ranking is live — but nothing on
// the page said "this is new since the brief".
//
// A COUNT AND A DOOR, NOT A CALLOUT. This was a titled notice between the
// greeting and the readings, naming three row titles and counting the rest,
// which put the same rows on the page twice — once in the notice and again in
// the feed directly below it, where they are drawn in full with their verbs. So
// the counting stays and the surface goes: the Today panel carries the count in
// its own head, where a reader meets it beside the rows it is about.
//
// THE SERVER DECIDES WHAT IS NEW, not this. Each row carries
// `changed_since_brief`, stamped against the run's own DATA CUTOFF rather than
// the later instant it finished writing — a run that read at 06:00 and wrote at
// 06:42 has a 42-minute window, and the wrong instant hides exactly the reply a
// rep opens the page to find. The browser cannot make that comparison: it does
// not know which of a row's several timestamps is the material one.
//
// ABSENT IS NOT FALSE. A row carries no flag at all when there was no run to
// compare against, and this answers undefined in that case rather than
// reporting a morning where nothing changed. "The night saw this" and "there
// was no night" are different facts, and a reader who cannot tell them apart
// will trust the wrong one.

/**
 * How many of the rows this page is answerable for have moved since the
 * overnight run, and where the reader can see exactly those rows.
 *
 * Undefined when none did. A surface that said "nothing has changed" every
 * morning would teach a reader to stop reading it, and on a day with no run it
 * would be saying something it cannot know.
 *
 * PURE, and that is what makes it shareable: the count belongs to the same
 * worklist answer the feed is drawn from, so the head and the rows under it
 * cannot disagree about how many moved.
 */
export function changedSinceBrief(
  day: Worklist | undefined,
): Readonly<{ count: number; href: string }> | undefined {
  // waitingRows, not the raw queue: it drops the approvals the Decisions deck
  // already answers, and it is the ONE spelling of "what this page is
  // answerable for". Counting `day.queue` here reported a decision the deck was
  // drawing as a card at the same moment. It also answers [] for a payload with
  // no queue, so it is the null guard too.
  const count = waitingRows(day).filter(
    (item) => item.changed_since_brief === true,
  ).length;
  if (count === 0) {
    return undefined;
  }
  // The SAME cut the count was taken over. A bare `#/worklist` named three rows
  // and opened a queue of forty with no way to tell which three, so the count
  // and its door shared nothing at all. The server applies the same freshness
  // test that stamped the flags read above, which is why this is one rule
  // rather than a browser-side narrowing that could disagree.
  return { count, href: worklistLaneHref("changed_since_brief") };
}
