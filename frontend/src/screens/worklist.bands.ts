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

type Band = NonNullable<WorklistItem["band"]>;

/** One heading and the loaded rows beneath it. */
export type BandSection = Readonly<{
  band: Band;
  items: readonly WorklistItem[];
}>;

// The order to fall back on when the server sent no bands.
//
// `bands` is optional in the contract, so a client talking to an older server
// gets none. Rather than drawing an unheaded list, the page falls back to the
// bands its rows name, in the order those rows arrive — which is the behaviour
// that shipped before this file and is correct as far as it goes.
function bandsFromRows(queue: readonly WorklistItem[]): Band[] {
  const seen: Band[] = [];
  for (const item of queue) {
    if (item.band && !seen.includes(item.band)) {
      seen.push(item.band);
    }
  }
  return seen;
}

/**
 * The page's sections, in the server's draw order.
 *
 * A band with no loaded rows keeps its place and reports an empty list, which
 * is what lets the page say so. A row whose band this build does not know still
 * draws — under the band it names — because dropping a row to keep the headings
 * tidy would hide work.
 */
export function bandSections(
  day: Worklist,
  queue: readonly WorklistItem[],
): BandSection[] {
  const order = day.bands?.map((band) => band.band) ?? bandsFromRows(queue);
  const under = new Map<Band, WorklistItem[]>(
    order.map((band) => [band, [] as WorklistItem[]]),
  );
  for (const item of queue) {
    if (!item.band) {
      continue;
    }
    const rows = under.get(item.band);
    if (rows) {
      rows.push(item);
      continue;
    }
    // A band the server did not list. It goes last, after every declared one,
    // for the reason bandRank puts an unknown band last on the server: a
    // heading this build cannot place must not push known work down the page.
    under.set(item.band, [item]);
  }
  return [...under].map(([band, items]) => ({ band, items }));
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
