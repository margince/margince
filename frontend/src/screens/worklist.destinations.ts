import type { components } from "../api/schema";

type WorklistItem = components["schemas"]["WorklistItem"];

// Which SCREEN each row belongs on, as the server decided it.
//
// The queue mixes three jobs. A buyer waiting on a reply is work a seller
// executes; a duplicate pair is a judgement somebody makes before work can
// continue; a stopped mailbox is a source an administrator restores. Drawn in
// one list they compete for the same attention, and a rep scanning for their
// next call steps over the product's own housekeeping to find it.
//
// THE SERVER DECIDES AND THIS READS. `destination` arrives on every row, and
// the counts above the queue are computed from that same value — so a client
// that re-derived the split from `source` or `category` would eventually
// disagree with the figures it is drawn beside. That is the whole reason the
// field exists rather than a rule living here.

// A row the server did not classify is SELLER WORK, and the direction matters.
//
// An older server sends no `destination` at all. Treating that silence as
// review would empty a rep's day the moment they talked to one — every row
// swept onto a screen they were not looking at — while treating it as today
// leaves the queue exactly as it was before this split existed. One is a
// regression nobody would attribute to a version skew; the other is the
// behaviour that shipped for a year.
const DEFAULT_DESTINATION = "today";

export function destinationOf(item: WorklistItem): string {
  return item.destination ?? DEFAULT_DESTINATION;
}

/** The rows a seller executes. */
export function sellerWork(
  queue: readonly WorklistItem[],
): readonly WorklistItem[] {
  return queue.filter((item) => destinationOf(item) === "today");
}

/**
 * The rows waiting on a judgement, a restored source, or a receipt.
 *
 * The three are counted together and drawn together, because the question the
 * reader has here is "what is not mine to execute" rather than which of the
 * three kinds it is. Splitting them further is a screen of its own.
 */
export function reviewWork(
  queue: readonly WorklistItem[],
): readonly WorklistItem[] {
  return queue.filter((item) => destinationOf(item) !== "today");
}

/**
 * What the Review panel can honestly say about how much it is showing.
 *
 * The panel draws the review rows LOADED SO FAR, and it has no cursor of its
 * own: review rows arrive as a side effect of paging the day. So a reader with
 * an approval past the page cut sees a panel that looks complete, and nothing
 * on the screen says otherwise — which is the whole reason this figure exists.
 *
 * `buckets.review` is the day's own total, counted over every candidate the
 * read weighed rather than over the page. Drawn bare beside the panel it would
 * CLAIM ROWS THE PANEL DOES NOT HOLD, so it is only ever shown as the
 * denominator of what is loaded.
 *
 * Null once the two agree: a panel holding everything the day has needs no
 * fraction, and "3 of 3" is noise on a complete list. This mirrors the
 * completeness line above the queue, deliberately — one page should not have
 * two different ways of admitting it is showing part of something.
 */
export function reviewShortfall(
  loaded: number,
  total: number | undefined,
): { loaded: number; total: number } | null {
  // An older server sends no partition. Saying nothing is right: a figure
  // invented here would be a claim about a day this client cannot count.
  if (total === undefined) {
    return null;
  }
  // A total BELOW what is loaded is not a shortfall to report. The two are
  // counted over different populations — the day's candidates against the rows
  // this walk has served — and a walk that outran its own total would draw
  // "5 of 3", which reads as a bug in the product rather than in the number.
  if (loaded >= total) {
    return null;
  }
  return { loaded, total };
}
