// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// Seller work and judgements are two jobs, drawn apart.
//
// A duplicate pair, a stopped mailbox and an approval somebody owes are not the
// next call to make. Drawn in the queue they competed with it — a rep scanning
// for their next customer stepped over the product's own housekeeping to find
// one — and the split is the server's `destination`, not a rule this client
// re-derives from `source`.

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the day is drawn apart from what it is not", () => {
  it("AC-WORKLIST-REVIEW-01: keeps a judgement out of the queue and still shows it", async () => {
    stub(
      day({
        queue: [
          row({
            id: "waiting-1",
            source: "customer_waiting",
            title: "Kirsten replied",
            destination: "today",
          }),
          row({
            id: "pair-1",
            source: "dedupe_candidate",
            title: "Two records for one company",
            destination: "review",
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );

    renderWorklist();

    // By its HEADING and the panel around it: Panel renders a bare <section>
    // with no accessible name, so it is not a landmark to query by role.
    const today = panelNamed(await screen.findByText(en["worklist.queue"]));
    // The buyer is in the day, and the pair is NOT — which is the half a test
    // that only asserted presence would miss: both rows render either way.
    expect(within(today).getByText("Kirsten replied")).toBeTruthy();
    expect(within(today).queryByText("Two records for one company")).toBeNull();

    // And it is drawn, not swallowed. Work nobody can see is the reason it
    // goes undone, so the split has to move the row rather than drop it.
    const review = panelNamed(await screen.findByText(en["worklist.review"]));
    expect(
      within(review).getByText("Two records for one company"),
    ).toBeTruthy();
  });

  it("draws no review panel on a day that holds none", async () => {
    stub(
      day({
        queue: [
          row({
            id: "waiting-1",
            source: "customer_waiting",
            title: "Kirsten replied",
            destination: "today",
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );

    renderWorklist();

    await screen.findByText("Kirsten replied");
    // An empty panel headed "To review" reads as a screen that failed to draw
    // its contents. Nothing to review is nothing to say.
    expect(screen.queryByText(en["worklist.review"])).toBeNull();
  });

  it("keeps a row an older server did not classify in the day", async () => {
    stub(
      day({
        queue: [
          row({
            id: "waiting-1",
            source: "customer_waiting",
            title: "Kirsten replied",
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );

    renderWorklist();

    // Version skew, and the direction is the point: reading silence as review
    // would empty a rep's day the moment they talked to an older server, while
    // reading it as seller work leaves the queue as it was before this split.
    // By its HEADING and the panel around it: Panel renders a bare <section>
    // with no accessible name, so it is not a landmark to query by role.
    const today = panelNamed(await screen.findByText(en["worklist.queue"]));
    await waitFor(() => {
      expect(within(today).getByText("Kirsten replied")).toBeTruthy();
    });
  });
});

// panelNamed is the panel a heading belongs to.
//
// Panel draws a bare <section> with no accessible name, so `getByRole("region")`
// finds nothing — the heading is the only handle, and its panel is what the
// assertion needs to look inside.
function panelNamed(heading: HTMLElement): HTMLElement {
  const panel = heading.closest("section");
  if (!panel) {
    throw new Error(`no panel around the heading "${heading.textContent}"`);
  }
  return panel as HTMLElement;
}

describe("the day's headings describe the day", () => {
  it("draws no empty heading for work that moved to review", async () => {
    stub(
      day({
        queue: [
          row({
            id: "waiting-1",
            source: "customer_waiting",
            title: "Kirsten replied",
            destination: "today",
            band: "now",
          }),
          row({
            id: "pair-1",
            source: "dedupe_candidate",
            title: "Two records for one company",
            destination: "review",
            band: "review",
          }),
        ],
        // The SERVER's band list counts every row it ranked, review included.
        bands: [
          { band: "now", shown: 1 },
          { band: "review", shown: 1 },
        ],
        summary: { urgent: 1, due: 0, lower_priority: 1, total: 2 },
      }),
    );

    renderWorklist();

    const today = panelNamed(await screen.findByText(en["worklist.queue"]));
    // The review band's rows are drawn BELOW now, so a heading for it inside
    // the day would stand over nothing and say the work is clear — while the
    // same work sits in the panel underneath. A band the day no longer holds
    // is not an empty band; it is a band that belongs somewhere else.
    expect(
      within(today).queryByText(en["worklist.bandClear.review"]),
    ).toBeNull();
  });
});

describe("each panel numbers its own rows", () => {
  it("starts both lists at one", async () => {
    stub(
      day({
        queue: [
          row({
            id: "w1",
            source: "customer_waiting",
            title: "Kirsten replied",
            destination: "today",
          }),
          row({
            id: "p1",
            source: "dedupe_candidate",
            title: "Two records for one company",
            destination: "review",
          }),
          row({
            id: "w2",
            source: "customer_waiting",
            title: "Anders is waiting",
            destination: "today",
          }),
        ],
        summary: { urgent: 2, due: 0, lower_priority: 1, total: 3 },
      }),
    );

    renderWorklist();

    const today = panelNamed(await screen.findByText(en["worklist.queue"]));
    const review = panelNamed(await screen.findByText(en["worklist.review"]));
    // Numbered within the list each row is drawn in. Ranking across the whole
    // day is arithmetically honest and reads as a fault: it puts 1 and 3 in one
    // panel and 2 in the other, and a list starting at 2 tells a reader nothing
    // they can act on.
    expect(within(today).getByText("1")).toBeTruthy();
    expect(within(today).getByText("2")).toBeTruthy();
    expect(within(today).queryByText("3")).toBeNull();
    expect(within(review).getByText("1")).toBeTruthy();
  });
});

// What the panel says about the work it is NOT holding.
//
// The panel has no cursor of its own: review rows arrive as a side effect of
// paging the day. Before this, an approval past the page cut simply did not
// render and nothing on the screen said it existed — a rep saw a clean panel
// and had no way to learn otherwise.
describe("the review panel admits what it is not showing", () => {
  it("says how many the day holds when the panel has fewer", async () => {
    stub(
      day({
        queue: [
          row({
            id: "t1",
            source: "task",
            destination: "today",
            title: "A task",
          }),
          row({
            id: "a1",
            source: "dedupe_candidate",
            destination: "review",
            title: "Two records for one company",
          }),
        ],
        summary: {
          urgent: 0,
          due: 0,
          lower_priority: 2,
          total: 2,
          buckets: { urgent: 0, due_today: 0, planned: 1, review: 4 },
        },
      }),
    );

    renderWorklist();
    await screen.findByText("Two records for one company");

    expect(
      screen.getByText(
        en["worklist.review.partial"]
          .replace("{loaded}", "1")
          .replace("{total}", "4"),
      ),
    ).toBeTruthy();
  });

  it("says nothing when the panel holds the day's whole review", async () => {
    stub(
      day({
        queue: [
          row({
            id: "a1",
            source: "dedupe_candidate",
            destination: "review",
            title: "Two records for one company",
          }),
        ],
        summary: {
          urgent: 0,
          due: 0,
          lower_priority: 1,
          total: 1,
          buckets: { urgent: 0, due_today: 0, planned: 0, review: 1 },
        },
      }),
    );

    renderWorklist();
    await screen.findByText("Two records for one company");

    // "1 of 1" is noise on a complete list, and the completeness line above
    // the queue falls silent on the same condition.
    expect(screen.queryByText(/of 1/)).toBeNull();
  });
});
