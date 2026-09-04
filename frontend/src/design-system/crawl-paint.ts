// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The crawl picture's canvas calls, apart from the arithmetic that decides
 * what to draw.
 *
 * Every line past this module's first statement needs a real
 * `CanvasRenderingContext2D`, and no lane in this repository has one — jsdom
 * answers `getContext("2d")` with null and the browser lane drives the app
 * rather than these two functions. Faking a context would assert against the
 * mock: a hand-rolled `arc` proves the mock records calls, not that the page
 * appeared. So the decisions were kept out of here, in `crawl-graph.ts`, where
 * they are covered for real — this file holds the strokes and no branch that
 * anything but a stroke depends on.
 */

import type { CrawlNode, Point } from "./crawl-graph";

const GROW_S = 0.34;
const POP_S = 0.3;
const HALO_S = 0.9;
const MOTES_PER_PAGE = 3;
const MOTE_LIFE_S = 1.1;

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

  ctx.globalAlpha = 1;
}

/**
 * What each page gave up, travelling back into the Core.
 *
 * This is the claim the whole screen rests on, in motion: a page was read,
 * something came off it, and that something goes into the thing doing the
 * reading. It arcs rather than sliding, because a straight line between two
 * dots reads as a wire and this is meant to read as something carried.
 *
 * IT ENDS IN THE ORB, not at the graph's own root. The root is a page like any
 * other; the Core is what the evidence is FOR, and a mote that stopped at the
 * left edge of the picture was a delivery to nobody. The orb lives outside this
 * canvas — several elements away, in the room rather than in the picture — so
 * both ends arrive here already measured, in one shared space (see
 * `MoteScene.nodes`). That is also why the two cannot drift apart when the Core
 * changes size: the caller measures the real element every frame.
 *
 * DERIVED FROM THE CLOCK, not accumulated in a list. Each page emits its motes
 * at a known time and they live a known span, so the whole population is a pure
 * function of the ages. Keeping a growing array here would make the picture
 * depend on how many frames had been drawn, and a tab left in the background
 * would come back with a different scene than one that stayed open.
 */
export function drawMotes(
  ctx: CanvasRenderingContext2D,
  scene: Readonly<{
    /** Where each page sits, in the shared space. Index 0 is the root. */
    nodes: readonly Point[];
    /** Seconds since each page arrived, in the same order as `nodes`. */
    ages: readonly number[];
    /** The Core's centre, in the same space. */
    home: Point;
    width: number;
    height: number;
    ink: string;
  }>,
): void {
  const { nodes, ages, home, width, height, ink } = scene;
  const arrived = Math.min(nodes.length, ages.length);
  ctx.clearRect(0, 0, width, height);
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
        node.x + (home.x - node.x) * k2,
        node.y + (home.y + offset - node.y) * k2 - Math.sin(k2 * Math.PI) * 16,
        2.4,
        0,
        Math.PI * 2,
      );
      ctx.fill();
    }
  }
  ctx.globalAlpha = 1;
}

/** How far through its own entrance a node of this age is, 0..1. */
function span(age: number, over: number): number {
  return Math.max(0, Math.min(1, age / over));
}
