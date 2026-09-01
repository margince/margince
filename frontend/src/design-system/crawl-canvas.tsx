// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import {
  CRAWL_BEAT_S,
  type CrawlNode,
  crawlAges,
  drawCrawl,
  layOutCrawl,
} from "./crawl-graph";
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
 * arcing back towards the root is the same claim in motion: what the crawl
 * found is going somewhere, and nothing has been written to a record yet.
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
 */
export function CrawlCanvas({
  pages,
  label,
}: Readonly<{
  pages: readonly CrawlPage[];
  /** What this picture is, for a reader who cannot see it. */
  label: string;
}>) {
  const canvas = useRef<HTMLCanvasElement>(null);
  const reduced = usePrefersReducedMotion();
  // The frame loop reads the latest pages without being torn down and rebuilt
  // every poll: re-running the effect on each arrival would restart the
  // animation clock and pop every node in again from nothing.
  const latest = useRef<readonly CrawlPage[]>(pages);
  latest.current = pages;

  useEffect(() => {
    const el = canvas.current;
    const ctx = el?.getContext("2d");
    if (!el || !ctx) {
      return;
    }
    return runCrawl(el, ctx, latest, reduced);
  }, [reduced]);

  return (
    <canvas
      className="crawl-canvas"
      ref={canvas}
      role="img"
      aria-label={label}
    />
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
): () => void {
  let frame = 0;
  let nodes: CrawlNode[] = [];
  // When each page was first seen. The read runs for minutes and pages land as
  // the crawl reaches them, so this is the only honest clock: a page's entrance
  // belongs to the moment IT arrived, not to the moment the canvas mounted.
  const stamps: number[] = [];

  let dealtFirstHand = false;

  /**
   * Give every page not yet stamped the moment it arrived.
   *
   * The FIRST hand is dealt a beat apart, so a read already in progress when
   * this mounts still gets an entrance instead of the whole graph appearing at
   * once. Everything after it is stamped NOW, because it really did just
   * arrive: staggering those too would put a page's entrance further into the
   * future the longer the read ran.
   */
  const stamp = (now: number, reduced: boolean) => {
    const wanted = pages.current.length;
    // A poll that returns fewer pages (a retry, a different read) must not
    // leave stamps pointing at nodes that are gone.
    if (stamps.length > wanted) {
      stamps.length = wanted;
    }
    while (stamps.length < wanted) {
      if (reduced) {
        // Far enough back to be settled: the end state, not an entrance nobody
        // asked for.
        stamps.push(now - SETTLED_MS);
      } else if (dealtFirstHand) {
        stamps.push(now);
      } else {
        stamps.push(now + stamps.length * CRAWL_BEAT_S * 1000);
      }
    }
    dealtFirstHand = true;
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
    const dpr = Math.min(2, globalThis.devicePixelRatio || 1);
    el.width = box.width * dpr;
    el.height = box.height * dpr;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    nodes = layOutCrawl(pages.current.length, box.width, box.height);
    return { width: box.width, height: box.height };
  };

  const paint = (now: number) => {
    stamp(now, reduced);
    const scene = sceneOf(el, measure);
    if (scene !== null) {
      drawCrawl(ctx, { nodes, ages: crawlAges(stamps, now), ...scene });
    }
    if (reduced) {
      return;
    }
    frame = requestAnimationFrame(paint);
  };

  frame = requestAnimationFrame(paint);
  return () => cancelAnimationFrame(frame);
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

/** Far enough into the past that every entrance has finished playing. */
const SETTLED_MS = 10_000;
