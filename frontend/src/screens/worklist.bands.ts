// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Worklist, WorklistItem } from "./worklist.queries";

// The headings the page draws, and which rows sit under each.
//
// The server has always sent `bands` — every band in draw order, including one
// holding no rows — and no client file read it. Headings were inferred from the
// rows instead, which can say everything except the one thing the field exists
// for: a band with nothing under it draws nothing, so a reader whose Now band
// is empty is told the same as a reader whose page simply started at Build
// pipeline.
//
// "Nothing needs you today" is an answer. Inferring headings from rows cannot
// give it, because the absence leaves no row to hang it on.
//
// THE QUEUE'S OWN ORDER IS NEVER CHANGED HERE, and that is the whole shape of
// this file. The server sends the rows sorted with each band contiguous, and
// says so in the contract; the ranks the page prints are positions in that
// order. Grouping the rows BY band instead — walking `bands` and collecting
// each one's rows — reorders them whenever the two disagree, which a paginated
// walk makes possible, and the page then prints rank 2 above rank 1. So the
// sections are runs of CONSECUTIVE rows, and `bands` is read only for which
// headings exist and which of them are empty.

type Band = NonNullable<WorklistItem["band"]>;

/** One heading and the run of rows beneath it. */
export type BandSection = Readonly<{
  band: Band;
  items: readonly WorklistItem[];
}>;

/**
 * The page's sections, in the QUEUE's order.
 *
 * Each is a run of consecutive rows naming one band. A band the server declared
 * but the loaded rows do not reach yields an empty run in the server's own
 * position, which is what lets the page say the band is clear.
 */
export function bandSections(
  day: Worklist,
  queue: readonly WorklistItem[],
): BandSection[] {
  const runs: { band: Band; items: WorklistItem[] }[] = [];
  for (const item of queue) {
    if (!item.band) {
      continue;
    }
    const open = runs.at(-1);
    if (open?.band === item.band) {
      open.items.push(item);
      continue;
    }
    runs.push({ band: item.band, items: [item] });
  }
  // The server's bands, minus the ones whose work is no longer drawn here.
  //
  // `review` names judgements, and those are drawn in their own panel below the
  // day. Left in this list it yields an empty run inside the day — a heading
  // standing over nothing, saying the reader has nothing to review, while the
  // review panel underneath holds exactly that work. A band the day does not
  // hold is not an empty band; it is a band that belongs somewhere else.
  const declared = (day.bands ?? [])
    .map((band) => band.band)
    .filter((band) => band !== "review");
  return withDeclaredEmpties(runs, declared);
}

/**
 * Slot each declared band the loaded rows never reached into its own place.
 *
 * Walked against the DECLARED order rather than appended, so an empty Now sits
 * above a drawn Build pipeline instead of after everything. A band the rows
 * name and the server did not declare keeps its place in the run order: it is
 * real work, and a heading this build cannot place must not reorder the rest.
 */
function withDeclaredEmpties(
  runs: readonly { band: Band; items: WorklistItem[] }[],
  declared: readonly Band[],
): BandSection[] {
  const drawn = new Set(runs.map((run) => run.band));
  const out: BandSection[] = [];
  let at = 0;
  for (const band of declared) {
    if (drawn.has(band)) {
      // Every run up to and including this band's first, in the queue's order.
      while (at < runs.length && runs[at].band !== band) {
        out.push(runs[at++]);
      }
      while (at < runs.length && runs[at].band === band) {
        out.push(runs[at++]);
      }
      continue;
    }
    out.push({ band, items: [] });
  }
  return [...out, ...runs.slice(at)];
}

/**
 * Whether the page may honestly say a band is empty.
 *
 * Only when the whole day is loaded. The queue arrives band-sorted, so a band
 * missing from page one may hold rows on page three — telling a reader
 * "nothing needs you now" while a Show more button sits below the fold would be
 * the page claiming to have looked at work it has not fetched.
 */
export function canReportEmptyBands(hasMore: boolean): boolean {
  return !hasMore;
}

/**
 * Rows carrying no band at all, which are drawn under no heading.
 *
 * An older server may send a queue with no bands on its rows. Those rows are
 * real work and are drawn after the banded sections rather than dropped.
 */
export function unbandedRows(
  queue: readonly WorklistItem[],
): readonly WorklistItem[] {
  return queue.filter((item) => !item.band);
}
