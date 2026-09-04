// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import {
  arrivalStamps,
  CRAWL_BEAT_S,
  crawlAges,
  crawlArrived,
  layOutCrawl,
} from "./crawl-graph";

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

describe("when each page enters the picture", () => {
  const BEAT_MS = CRAWL_BEAT_S * 1000;

  // A poll hands over everything the crawl walked since the last one. Stamped
  // with a single `now` they all pop together, which is a graph that jumps
  // between polls rather than one that walks.
  it("spreads a batch that arrives at once", () => {
    const stamps = arrivalStamps([], 4, 1000, false);
    expect(stamps).toEqual([
      1000,
      1000 + BEAT_MS,
      1000 + 2 * BEAT_MS,
      1000 + 3 * BEAT_MS,
    ]);
  });

  // The first frame runs before any page has been reported, and a picture that
  // treated that empty hand as "the entrance is spent" would stamp every real
  // page with one moment forever after.
  it("does not spend the entrance on a poll that reported nothing", () => {
    const empty = arrivalStamps([], 0, 1000, false);
    expect(empty).toEqual([]);
    expect(arrivalStamps(empty, 3, 2000, false)).toEqual([
      2000,
      2000 + BEAT_MS,
      2000 + 2 * BEAT_MS,
    ]);
  });

  // Pages already walked in keep the moment they walked in at: restamping them
  // would replay entrances the reader has already watched.
  it("leaves the pages it has already stamped alone", () => {
    expect(arrivalStamps([10, 20], 3, 5000, false)).toEqual([10, 20, 5000]);
  });

  // A page that arrives while the batch before it is still entering queues
  // behind it, so the walk stays in order rather than doubling back.
  it("queues a new page behind one still entering", () => {
    const first = arrivalStamps([], 2, 1000, false);
    const next = arrivalStamps(first, 3, 1100, false);
    expect(next[2]).toBe(first[1] + BEAT_MS);
  });

  // A read that comes back shorter (a retry, a different read) must not leave
  // stamps pointing at nodes that are gone.
  it("drops stamps for pages a shorter read no longer has", () => {
    expect(arrivalStamps([10, 20, 30], 2, 5000, false)).toEqual([10, 20]);
  });

  // Reduced motion asks for the end state, not an entrance.
  it("settles every page at once when motion is refused", () => {
    const stamps = arrivalStamps([], 3, 100_000, true);
    expect(new Set(stamps).size).toBe(1);
    for (const age of crawlAges(stamps, 100_000)) {
      expect(age).toBeGreaterThan(1);
    }
  });
});
