// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef } from "react";
import { type CrawlNode, drawCrawl, layOutCrawl } from "./crawl-graph";
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
    let frame = 0;
    let started = 0;
    let nodes: CrawlNode[] = [];

    const measure = () => {
      const box = el.getBoundingClientRect();
      if (box.width === 0 || box.height === 0) {
        return false;
      }
      const dpr = Math.min(2, globalThis.devicePixelRatio || 1);
      el.width = box.width * dpr;
      el.height = box.height * dpr;
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      nodes = layOutCrawl(latest.current.length, box.width, box.height);
      return true;
    };

    const paint = (now: number) => {
      if (started === 0) {
        started = now;
      }
      const palette = paletteOf(el);
      // No colours, no picture. The three come from this element's own
      // stylesheet, so an empty one means the design system moved out from
      // under it; painting a guessed colour would put a mark on screen that
      // belongs to no token and follows no theme. The words beside the canvas
      // carry the statement either way.
      if (palette !== null && measure()) {
        drawCrawl(ctx, {
          nodes,
          // Reduced motion draws the graph already complete: the end state, not
          // an empty box. The picture is worth having; the arrival is what the
          // preference asks us to drop.
          elapsed: reduced ? Number.POSITIVE_INFINITY : (now - started) / 1000,
          width: el.width / Math.min(2, globalThis.devicePixelRatio || 1),
          height: el.height / Math.min(2, globalThis.devicePixelRatio || 1),
          ...palette,
        });
      }
      if (reduced) {
        return;
      }
      frame = requestAnimationFrame(paint);
    };

    frame = requestAnimationFrame(paint);
    return () => cancelAnimationFrame(frame);
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
