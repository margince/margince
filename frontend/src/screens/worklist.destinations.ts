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
