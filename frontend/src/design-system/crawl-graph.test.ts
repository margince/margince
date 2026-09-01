// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { crawlAges, crawlArrived, layOutCrawl } from "./crawl-graph";

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

describe("when each page arrived", () => {
  it("ages every page from its own stamp, not from one clock", () => {
    // The defect this replaced: one elapsed-since-mount said every page landed
    // in the first three seconds, so the graph finished, cooled to grey, and
    // then sat still for the rest of a read that was still running.
    const now = 10_000;
    expect(crawlAges([10_000, 9_000, 7_500], now)).toEqual([0, 1, 2.5]);
  });

  it("gives a page that has not come due yet a negative age", () => {
    // The first hand is dealt a beat apart, so a stamp can sit in the future.
    // Negative is what tells the painter this one has no entrance to draw yet.
    expect(crawlAges([12_000], 10_000)).toEqual([-2]);
  });

  it("draws an entrance for every page it has a stamp for", () => {
    expect(crawlArrived(crawlAges([1, 2, 3], 10))).toBe(3);
    expect(crawlArrived([])).toBe(0);
  });
});
