// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { WorklistView } from "../../lib/worklistviews";
import type { Worklist, WorklistItem } from "./worklist.queries";
import { rowIdentity } from "./worklist.rowidentity";

// The headings the page draws, and which rows sit under each.
//
// The server has always sent `bands` — every band in draw order, including one
// holding no rows — and no client file read it. Headings were inferred from the
// rows instead, which can say everything except the one thing the field exists
// for: a band with nothing under it draws nothing, so a reader whose Now band
// is empty is told the same as a reader whose page simply started at
// Prospecting.
//
// "Nothing needs you today" is an answer. Inferring headings from rows cannot
// give it, because the absence leaves no row to hang it on.
//
// Each band draws once, where its first row falls in the server's order. Later
// pages and the backlog tail can name a band again; those rows join its one
// heading. Declared bands supply empty headings; they never reorder the bands.

type Band = NonNullable<WorklistItem["band"]>;

/** One heading and the run of rows beneath it. */
export type BandSection = Readonly<{
  band: Band;
  items: readonly WorklistItem[];
}>;

/**
 * The page's sections, one per band, in the order the QUEUE first names each.
 *
 * A band the server declared but the loaded rows do not reach yields an empty
 * section in the server's own position, which is what lets the page say the
 * band is clear.
 */
export function bandSections(
  day: Worklist,
  queue: readonly WorklistItem[],
): BandSection[] {
  const runs: { band: Band; items: WorklistItem[] }[] = [];
  const held = new Map<Band, WorklistItem[]>();
  for (const item of queue) {
    if (!item.band) {
      continue;
    }
    const items = held.get(item.band);
    if (items) {
      items.push(item);
      continue;
    }
    const opened = [item];
    held.set(item.band, opened);
    runs.push({ band: item.band, items: opened });
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
 * above a drawn Prospecting instead of after everything. A band the rows
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
      let run = runs[at];
      while (run !== undefined && run.band !== band) {
        out.push(run);
        run = runs[++at];
      }
      while (run !== undefined && run.band === band) {
        out.push(run);
        run = runs[++at];
      }
      continue;
    }
    out.push({ band, items: [] });
  }
  return [...out, ...runs.slice(at)];
}

/**
 * The rows in the order the page draws them: band by band, then the unbanded.
 * Rank numbers count this order, so they ascend down the page.
 */
export function drawnOrder(
  day: Worklist,
  queue: readonly WorklistItem[],
): readonly WorklistItem[] {
  return [
    ...bandSections(day, queue).flatMap((section) => section.items),
    ...unbandedRows(queue),
  ];
}

/** Each row's rank. Today draws by band, so its numbers count the drawn order. */
export function positionsOf(
  day: Worklist,
  queue: readonly WorklistItem[],
  view: WorklistView,
): ReadonlyMap<string, number> {
  const drawn = view === "today" ? drawnOrder(day, queue) : queue;
  return new Map(drawn.map((item, at) => [rowIdentity(item), at] as const));
}

/**
 * Whether the page may honestly say a band is empty.
 *
 * Only when the whole day is loaded. The queue arrives band-sorted, so a band
 * missing from page one may hold rows on page three — telling a reader
 * "nothing needs you now" while a Load more button sits below the fold would be
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

/**
 * The groups that earn a heading, which is a narrower set than the wire's.
 *
 * `overdue` and `today` are deliberately absent: they draw under the band's own
 * heading. They have no copy key, and the type says so. `none` heads the
 * undated backlog, after the dated upcoming work.
 */
export type HeadedDueGroup = "tomorrow" | "this_week" | "later" | "none";

/** One run of rows inside a band that share a due group. */
export type DueRun = {
  /** Absent on rows carrying no deadline, which draw under no sub-heading. */
  group?: HeadedDueGroup;
  items: [WorklistItem, ...WorklistItem[]];
};

/** The copy key for a run's heading, so the caller never builds one by hand. */
export function dueRunHeading(group: HeadedDueGroup) {
  return `worklist.dueGroup.${group}` as const;
}

/**
 * A band's rows split into consecutive runs by the day their work is due.
 *
 * Consecutive, never regrouped: the server ranks the queue and the page draws
 * the order it is given, so this only ever names a boundary the ranking already
 * put there. Sorting here would let the page disagree with the counts above it.
 *
 * Overdue and today's work carry no sub-heading — the band's own heading
 * already says that is what it is — so only the runs a reader would otherwise
 * mistake for today get one.
 */
export function dueRuns(items: readonly WorklistItem[]): DueRun[] {
  const runs: DueRun[] = [];
  for (const item of items) {
    const group = headedDueGroup(item.due_group);
    const open = runs.at(-1);
    if (open && open.group === group) {
      open.items.push(item);
      continue;
    }
    runs.push({ group, items: [item] });
  }
  return runs;
}

/**
 * Which groups earn a heading of their own.
 *
 * A row with no group, an overdue one and today's all draw under the band
 * heading, so they share one unlabelled run rather than three.
 */
function headedDueGroup(
  group: WorklistItem["due_group"],
): HeadedDueGroup | undefined {
  if (!group || group === "overdue" || group === "today") {
    return undefined;
  }
  return group;
}
