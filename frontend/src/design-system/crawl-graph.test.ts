// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { CRAWL_BEAT_S, crawlArrived, layOutCrawl } from "./crawl-graph";

describe("the crawl picture's geometry", () => {
  it("draws nothing for a read that has reached no pages", () => {
    expect(layOutCrawl(0, 800, 200)).toEqual([]);
  });

  it("puts the root at the left, where the evidence flies back to", () => {
    const [root] = layOutCrawl(1, 800, 200);
    expect(root.parent).toBe(-1);
    expect(root.x).toBeLessThan(800 * 0.1);
    expect(root.y).toBe(100);
  });

  it("hangs the first ring off the root and the second off the first", () => {
    const nodes = layOutCrawl(12, 800, 200);
    expect(nodes[1].parent).toBe(0);
    expect(nodes[4].parent).toBe(0);
    // Past the inner ring, a page hangs off one of the inner ones rather than
    // off the root, which is what makes the picture fan out instead of forming
    // a single sheaf.
    for (const node of nodes.slice(5)) {
      expect(node.parent).toBeGreaterThan(0);
      expect(node.parent).toBeLessThanOrEqual(4);
    }
  });

  it("stays inside the box it was given", () => {
    const width = 640;
    const height = 180;
    for (const node of layOutCrawl(24, width, height)) {
      expect(node.x).toBeGreaterThan(0);
      expect(node.x).toBeLessThan(width);
      expect(node.y).toBeGreaterThan(0);
      expect(node.y).toBeLessThan(height);
    }
  });

  it("lays the same read out the same way twice", () => {
    // The scatter is a function of the index rather than a random, so a repaint
    // is a repaint. Re-rolling it per frame would make the graph shiver.
    expect(layOutCrawl(16, 800, 200)).toEqual(layOutCrawl(16, 800, 200));
  });
});

describe("how much of the crawl has arrived", () => {
  it("shows the first page immediately, then one per beat", () => {
    expect(crawlArrived(0, 10)).toBe(1);
    expect(crawlArrived(CRAWL_BEAT_S * 1.5, 10)).toBe(2);
    expect(crawlArrived(CRAWL_BEAT_S * 4.2, 10)).toBe(5);
  });

  it("never draws a page the server has not sent", () => {
    // The one way this could lie: running ahead of the read and showing
    // progress that has not happened.
    expect(crawlArrived(CRAWL_BEAT_S * 99, 3)).toBe(3);
    expect(crawlArrived(CRAWL_BEAT_S * 99, 0)).toBe(0);
  });

  it("is already complete under reduced motion", () => {
    // The end state, not an empty box: the picture is worth having, the
    // arrival is what the preference asks us to drop.
    expect(crawlArrived(Number.POSITIVE_INFINITY, 7)).toBe(7);
  });
});
