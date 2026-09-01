// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The crawl picture's geometry and paint, apart from the component that hosts
 * it.
 *
 * Split out because the layout is the part with decisions in it, and a decision
 * that can only be exercised through a real canvas context is a decision nobody
 * tests. `layOutCrawl` is arithmetic and is tested directly; `drawCrawl` is the
 * part that genuinely needs a context and holds no branching of its own.
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
const GROW_S = 0.34;
const POP_S = 0.3;
const HALO_S = 0.9;
const ROOT_X = 0.06;
const RING_ONE_X = 0.38;
const RING_TWO_X = 0.74;
const RING_ONE_SIZE = 4;
/** How many marks of evidence each page sends back, and how long they fly. */
const MOTES_PER_PAGE = 3;
const MOTE_LIFE_S = 1.1;

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
export function crawlAges(stamps: readonly number[], now: number): number[] {
  return stamps.map((at) => (now - at) / 1000);
}

/** How many pages have an entrance to draw: everything stamped. */
export function crawlArrived(ages: readonly number[]): number {
  return ages.length;
}

export function drawCrawl(
  ctx: CanvasRenderingContext2D,
  scene: Readonly<{
    nodes: readonly CrawlNode[];
    /** Seconds since each node arrived, in the same order as `nodes`. */
    ages: readonly number[];
    width: number;
    height: number;
    ink: string;
    faint: string;
    dim: string;
  }>,
): void {
  const { nodes, ages, width, height, ink, faint, dim } = scene;
  const arrived = Math.min(nodes.length, ages.length);
  ctx.clearRect(0, 0, width, height);
  ctx.lineWidth = 1;

  // Edges first, so a node always sits on top of the line that reached it.
  ctx.strokeStyle = faint;
  for (let i = 0; i < arrived; i++) {
    const node = nodes[i];
    // Not due yet: the first hand is dealt a beat apart, so a page's stamp can
    // sit in the future and its age be negative until its turn comes.
    if (node.parent < 0 || ages[i] < 0) {
      continue;
    }
    const from = nodes[node.parent];
    const grow = span(ages[i], GROW_S);
    ctx.beginPath();
    ctx.moveTo(from.x, from.y);
    ctx.quadraticCurveTo(
      (from.x + node.x) / 2,
      from.y + (node.y - from.y) * 0.15,
      from.x + (node.x - from.x) * grow,
      from.y + (node.y - from.y) * grow,
    );
    ctx.stroke();
  }

  for (let i = 0; i < arrived; i++) {
    const node = nodes[i];
    const age = ages[i];
    if (age < 0) {
      continue;
    }
    const halo = Math.max(0, 1 - age / HALO_S);
    if (halo > 0) {
      ctx.fillStyle = ink;
      ctx.globalAlpha = halo * 0.16;
      ctx.beginPath();
      ctx.arc(node.x, node.y, node.r + 14 * (1 - halo), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
    // A page cools from the agent's colour to a resting grey once it is read:
    // indigo marks what is happening NOW, and a graph where everything stays
    // indigo says the whole site is being read at once.
    ctx.fillStyle = i === 0 || age < 1.2 ? ink : dim;
    ctx.beginPath();
    ctx.arc(node.x, node.y, node.r * span(age, POP_S), 0, Math.PI * 2);
    ctx.fill();
  }

  drawEvidence(ctx, nodes, ages, arrived, width, height, ink);
  ctx.globalAlpha = 1;
}

/**
 * What each page gave up, travelling back to where the read is assembled.
 *
 * This is the claim the whole screen rests on, in motion: a page was read, and
 * something came off it that is going somewhere. It arcs rather than sliding,
 * because a straight line between two dots reads as a wire and this is meant to
 * read as something carried.
 *
 * DERIVED FROM THE CLOCK, not accumulated in a list. Each page emits its motes
 * at a known time and they live a known span, so the whole population is a pure
 * function of `elapsed`. Keeping a growing array here would make the picture
 * depend on how many frames had been drawn, and a tab left in the background
 * would come back with a different scene than one that stayed open.
 */
function drawEvidence(
  ctx: CanvasRenderingContext2D,
  nodes: readonly CrawlNode[],
  ages: readonly number[],
  arrived: number,
  width: number,
  height: number,
  ink: string,
): void {
  const homeX = width * ROOT_X;
  const homeY = height / 2;
  for (let i = 1; i < arrived; i++) {
    for (let k = 0; k < MOTES_PER_PAGE; k++) {
      const span = MOTE_LIFE_S + k * 0.16;
      const life = ages[i];
      if (life < 0 || life > span) {
        continue;
      }
      const node = nodes[i];
      const k2 = (life / span) ** 2;
      const offset = (k - 1) * 9;
      ctx.globalAlpha = (1 - life / span) * 0.9;
      ctx.fillStyle = ink;
      ctx.beginPath();
      ctx.arc(
        node.x + (homeX - node.x) * k2,
        node.y + (homeY + offset - node.y) * k2 - Math.sin(k2 * Math.PI) * 16,
        2.4,
        0,
        Math.PI * 2,
      );
      ctx.fill();
    }
  }
}

/** How far through its own entrance a node of this age is, 0..1. */
function span(age: number, over: number): number {
  return Math.max(0, Math.min(1, age / over));
}
