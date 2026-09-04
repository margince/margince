// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The crawl picture's geometry, apart from the component that hosts it and the
 * canvas calls that draw it.
 *
 * Split out because the layout is the part with decisions in it, and a decision
 * that can only be exercised through a real canvas context is a decision nobody
 * tests. Everything here is arithmetic and is tested directly; the painting it
 * feeds lives in `crawl-paint.ts`, which cannot be reached without a real
 * context and holds no branching of its own.
 */

/** One page's place in the picture, and which page it hangs off. */
export type CrawlNode = Readonly<{
  x: number;
  y: number;
  r: number;
  /** Index of the page that links to this one; -1 for the root. */
  parent: number;
}>;

/** How long each page takes to arrive, and how long its halo lingers. */
export const CRAWL_BEAT_S = 0.4;
const ROOT_X = 0.06;
const RING_ONE_X = 0.38;
const RING_TWO_X = 0.74;
const RING_ONE_SIZE = 4;
/** How many marks of evidence each page sends back, and how long they fly. */

/**
 * Where each page sits.
 *
 * TWO RINGS, LEFT TO RIGHT, because that is the shape of the read rather than
 * of a site: the root is what was given, the first ring is what the root linked
 * to, the second is what those linked to. A radial tree would be prettier and
 * would say something false, since the crawl does not know the site's real
 * depth and never claims to.
 *
 * The scatter is deterministic (a sine of the index, not a random) so the same
 * read draws the same picture on every frame and on every reload. A random
 * offset re-rolled per frame is a graph that shivers.
 */
export function layOutCrawl(
  count: number,
  width: number,
  height: number,
): CrawlNode[] {
  const nodes: CrawlNode[] = [];
  for (let i = 0; i < count; i++) {
    if (i === 0) {
      nodes.push({ x: width * ROOT_X, y: height / 2, r: 7, parent: -1 });
      continue;
    }
    const inner = i <= RING_ONE_SIZE;
    const within = inner ? i - 1 : i - 1 - RING_ONE_SIZE;
    const inRing = inner
      ? RING_ONE_SIZE
      : Math.max(1, count - 1 - RING_ONE_SIZE);
    const spread = (within + 0.5) / inRing;
    nodes.push({
      x:
        width * (inner ? RING_ONE_X : RING_TWO_X) +
        Math.sin(i * 2.7) * width * 0.04,
      y: height * (0.12 + spread * 0.76) + Math.cos(i * 1.9) * height * 0.03,
      r: 4.5,
      // An outer page hangs off one of the inner ones, spread evenly so the
      // second ring fans out of the first instead of all hanging off one node.
      parent: inner ? 0 : 1 + (within % RING_ONE_SIZE),
    });
  }
  return nodes;
}

/**
 * When each page arrived, in seconds before now.
 *
 * ONE AGE PER PAGE, not one clock for the picture. The read runs for minutes and
 * pages land whenever the crawl reaches them, so a single elapsed-since-mount
 * says every page arrived in the first three seconds: the graph finishes its
 * entrance, cools to grey, and then sits still for the rest of the read while
 * pages are still arriving. Worse, each new one appears already cold, which is
 * the opposite of the thing worth showing.
 *
 * `stamps` holds the moment each page was first seen. The pages already there on
 * the first frame are dealt in one beat apart, so a resumed read still gets an
 * entrance rather than appearing all at once.
 */
/** Far enough into the past that every entrance has finished playing. */
const SETTLED_MS = 10_000;

/**
 * When each page should enter, given the ones already stamped.
 *
 * A POLL DELIVERS A BATCH, not a page. The crawl walked those pages one at a
 * time over the seconds between two polls, so stamping a batch with one `now`
 * makes the whole lot pop together and the picture jumps between polls instead
 * of walking. Each new page enters a beat after the one before it and never
 * before the present, so a batch spreads and a lone arrival is immediate.
 *
 * This is an entrance's timing, never a claim about progress: a page is
 * stamped only once the server has actually sent it.
 *
 * A read that returns FEWER pages than are stamped (a retry, a different read)
 * drops the extra stamps rather than leaving them pointing at nodes that are
 * gone.
 */
export function arrivalStamps(
  stamps: readonly number[],
  wanted: number,
  now: number,
  reduced: boolean,
): number[] {
  const out = stamps.slice(0, wanted);
  while (out.length < wanted) {
    if (reduced) {
      // Settled, not an entrance nobody asked for.
      out.push(now - SETTLED_MS);
      continue;
    }
    const last = out[out.length - 1];
    out.push(
      last === undefined ? now : Math.max(now, last + CRAWL_BEAT_S * 1000),
    );
  }
  return out;
}

export function crawlAges(stamps: readonly number[], now: number): number[] {
  return stamps.map((at) => (now - at) / 1000);
}

/** How many pages have an entrance to draw: everything stamped. */
export function crawlArrived(ages: readonly number[]): number {
  return ages.length;
}

/** A point in the room's own coordinates, which every surface shares. */
export type Point = Readonly<{ x: number; y: number }>;

/**
 * The graph's nodes, moved from the picture's own box into the room's space.
 *
 * The two canvases do not share an origin: the picture sits in the board and the
 * motes cross the whole room. `offset` is the picture's own position in the
 * shared space, so a node's place is the same fact expressed twice rather than
 * two facts that can disagree.
 */
export function motePath(nodes: readonly CrawlNode[], offset: Point): Point[] {
  return nodes.map((node) => ({ x: node.x + offset.x, y: node.y + offset.y }));
}
