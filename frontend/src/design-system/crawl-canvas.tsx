// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import {
  arrivalStamps,
  type CrawlNode,
  crawlAges,
  layOutCrawl,
  motePath,
} from "./crawl-graph";
import { drawCrawl, drawMotes } from "./crawl-paint";
import { usePrefersReducedMotion } from "./motion";
import "./crawl-canvas.css";

/** One page the read has reached, as this surface needs it. */
export type CrawlPage = Readonly<{
  /** The path, shown verbatim. Never the whole URL: the host is the same on every row. */
  path: string;
  /** What that page gave up, in the reader's words. Absent while it is still being read. */
  note?: string;
}>;

/**
 * The site, drawn as it is read.
 *
 * WHY A PICTURE AT ALL. This is the one screen in the product where somebody
 * waits a full two minutes with nothing to do, and a progress bar spends that
 * time saying only "not yet". The read has a shape worth showing instead: pages
 * hang off the pages that linked to them, so the picture is the site's own
 * structure appearing, and a reader can see their own site in it.
 *
 * WHY IT IS THE AGENT'S COLOUR. Indigo throughout, because every mark here is
 * something a machine did (see the README's provenance rule). The evidence
 * flying into the Core is the same claim in motion: what the crawl found is
 * going somewhere, and that somewhere is the thing doing the reading.
 *
 * IT DRAWS ONLY WHAT THE SERVER SENT. Nodes arrive as pages do; there is no
 * invented total and nothing is drawn ahead of the read. A canvas that filled
 * itself in while the crawl stalled would be the interface lying about progress,
 * which is the one thing this screen cannot afford.
 *
 * IT IS NOT THE STATEMENT. One label names the whole picture, and the caller
 * puts the same walk in words beside it: this replaced a strip that named every
 * page one by one, and a canvas carrying a summary alone would have quietly
 * taken that away.
 *
 * TWO LAYERS, because the evidence leaves the picture. The graph is a box on
 * the board; the Core is somewhere else in the room entirely, and a canvas
 * clips at its own edges, so motes drawn in the graph's layer could only ever
 * stop at its border. The second layer is the whole viewport, which is the one
 * space both ends of the journey can be measured in.
 */
export function CrawlCanvas({
  pages,
  label,
  flowToId,
}: Readonly<{
  pages: readonly CrawlPage[];
  /** What this picture is, for a reader who cannot see it. */
  label: string;
  /**
   * The element the evidence flies into, by id: the Core.
   *
   * Absent where the surface has no Core to deliver to, and then the motes are
   * not drawn at all rather than aimed at a guess. A screen that showed evidence
   * streaming towards nothing in particular would be animation for its own sake.
   */
  flowToId?: string;
}>) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const motes = useRef<HTMLCanvasElement>(null);
  const reduced = usePrefersReducedMotion();
  // The frame loop reads the latest pages without being torn down and rebuilt
  // every poll: re-running the effect on each arrival would restart the
  // animation clock and pop every node in again from nothing.
  const latest = useRef<readonly CrawlPage[]>(pages);
  latest.current = pages;
  // Under reduced motion `runCrawl` paints one frame and stops, so a page
  // arriving afterwards would never be drawn — and pages arrive by poll, so
  // the still picture a reader was left with was usually the empty one. That
  // mode therefore re-runs the effect per arrival to repaint.
  //
  // The animated path must NOT take this dependency: re-running its effect
  // restarts the clock and pops every node in again from nothing, which is
  // exactly what `latest` being a ref exists to prevent.
  const stillPaints = reduced ? pages.length : 0;

  // biome-ignore lint/correctness/useExhaustiveDependencies: trigger-only dep — `stillPaints` is not read in the body; re-running the effect IS the repaint, and it holds at 0 while the loop is animating so the clock is never restarted.
  useEffect(() => {
    const el = canvas.current;
    const ctx = el?.getContext("2d");
    if (!el || !ctx) {
      return;
    }
    return runCrawl(el, ctx, latest, reduced, motes, flowToId);
  }, [reduced, flowToId, stillPaints]);

  return (
    <>
      <canvas
        className="crawl-canvas"
        ref={canvas}
        role="img"
        aria-label={label}
      />
      {/* Decoration on top of the room, and nothing else: the picture it
          belongs to is already the labelled one, so a second `role="img"` here
          would announce the same thing twice. */}
      {flowToId === undefined ? null : (
        // `tabIndex` -1 alongside `aria-hidden`: a canvas counts as focusable,
        // and hiding a focusable element from the accessibility tree while
        // leaving it in the tab order strands a keyboard on something a screen
        // reader cannot name.
        <canvas
          className="crawl-motes"
          ref={motes}
          aria-hidden="true"
          tabIndex={-1}
        />
      )}
    </>
  );
}

/**
 * The frame loop, outside the component that mounts it.
 *
 * Not a style preference: as an arrow inside an effect inside a component this
 * sat three levels deep, and everything it does counts double at that depth.
 * At module scope it is one function that takes what it needs and hands back
 * the way to stop it.
 */
function runCrawl(
  el: HTMLCanvasElement,
  ctx: CanvasRenderingContext2D,
  pages: { current: readonly CrawlPage[] },
  reduced: boolean,
  motes: { current: HTMLCanvasElement | null },
  flowToId?: string,
): () => void {
  let frame = 0;
  let nodes: CrawlNode[] = [];
  // When each page was first seen. The read runs for minutes and pages land as
  // the crawl reaches them, so this is the only honest clock: a page's entrance
  // belongs to the moment IT arrived, not to the moment the canvas mounted.
  let stamps: number[] = [];

  const stamp = (now: number, reduced: boolean) => {
    stamps = arrivalStamps(stamps, pages.current.length, now, reduced);
  };

  // The box in CSS pixels, or null when the canvas has no size to draw in (a
  // hidden tab, a parent still laying out). Sizing the backing store is part of
  // measuring: the two have to agree every frame, or the picture stretches the
  // first time the column changes width.
  const measure = (): { width: number; height: number } | null => {
    const box = el.getBoundingClientRect();
    if (box.width === 0 || box.height === 0) {
      return null;
    }
    fitBackingStore(el, ctx, box);
    nodes = layOutCrawl(pages.current.length, box.width, box.height);
    return { width: box.width, height: box.height };
  };

  const paint = (now: number) => {
    stamp(now, reduced);
    const scene = sceneOf(el, measure);
    const ages = crawlAges(stamps, now);
    if (scene !== null) {
      drawCrawl(ctx, { nodes, ages, ...scene });
      if (flowToId !== undefined) {
        paintMotes(motes.current, nodes, ages, el, flowToId, scene.ink);
      }
    }
    if (reduced) {
      return;
    }
    frame = requestAnimationFrame(paint);
  };

  frame = requestAnimationFrame(paint);
  return () => cancelAnimationFrame(frame);
}

/**
 * The evidence crossing the room, drawn in the viewport's own coordinates.
 *
 * VIEWPORT SPACE, not some parent's, because the two things this connects live
 * in different corners of the layout: the graph is a box inside the board, the
 * Core is positioned against the stage. `getBoundingClientRect` gives both in
 * one frame of reference and the layer is fixed to that same frame, so a scroll
 * or a resize needs no correction at all.
 */
function paintMotes(
  layer: HTMLCanvasElement | null,
  nodes: readonly CrawlNode[],
  ages: readonly number[],
  graph: HTMLCanvasElement,
  flowToId: string,
  ink: string,
): void {
  const ctx = layer?.getContext("2d");
  const core = document.getElementById(flowToId);
  if (!layer || !ctx || core === null) {
    return;
  }
  const box = layer.getBoundingClientRect();
  if (box.width === 0 || box.height === 0) {
    return;
  }
  fitBackingStore(layer, ctx, box);
  const from = graph.getBoundingClientRect();
  const to = core.getBoundingClientRect();
  drawMotes(ctx, {
    nodes: motePath(nodes, { x: from.left - box.left, y: from.top - box.top }),
    ages,
    home: {
      x: to.left + to.width / 2 - box.left,
      y: to.top + to.height / 2 - box.top,
    },
    width: box.width,
    height: box.height,
    ink,
  });
}

/**
 * Match a canvas's pixel buffer to the box it occupies.
 *
 * Every frame, and deliberately: the backing store and the CSS box have to
 * agree or the drawing stretches the first time either changes, and the cheapest
 * way to guarantee that is to never let them be set apart.
 */
function fitBackingStore(
  el: HTMLCanvasElement,
  ctx: CanvasRenderingContext2D,
  box: DOMRect,
): void {
  const dpr = Math.min(2, globalThis.devicePixelRatio || 1);
  const width = Math.round(box.width * dpr);
  const height = Math.round(box.height * dpr);
  // Assigning a canvas's size reallocates its buffer even when the value is
  // unchanged, and the motes layer is the size of the window: left unguarded,
  // that is a full-screen allocation on every frame.
  if (el.width !== width || el.height !== height) {
    el.width = width;
    el.height = height;
  }
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
}

/** Everything one frame needs, or null when this frame cannot honestly draw. */
function sceneOf(
  el: HTMLCanvasElement,
  measure: () => { width: number; height: number } | null,
): Readonly<{
  width: number;
  height: number;
  ink: string;
  faint: string;
  dim: string;
}> | null {
  const palette = paletteOf(el);
  if (palette === null) {
    return null;
  }
  const box = measure();
  return box === null ? null : { ...box, ...palette };
}

/**
 * The three colours the picture is drawn in, read off the canvas itself.
 *
 * A 2D context cannot resolve `var()`, so the values have to be read rather
 * than written. They are read EVERY FRAME, which is also what makes the graph
 * follow a theme change without remounting.
 *
 * Null when any of them is missing, which is the design system having moved out
 * from under this component. Nothing is drawn then, because the alternative is
 * a hard-coded colour belonging to no token and following no theme.
 */
function paletteOf(
  el: Element,
): Readonly<{ ink: string; faint: string; dim: string }> | null {
  const style = getComputedStyle(el);
  const ink = style.getPropertyValue("--crawl-ink").trim();
  const faint = style.getPropertyValue("--crawl-edge").trim();
  const dim = style.getPropertyValue("--crawl-rest").trim();
  if (ink === "" || faint === "" || dim === "") {
    return null;
  }
  return { ink, faint, dim };
}
