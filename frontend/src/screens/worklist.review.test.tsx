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
  it("keeps a judgement out of the queue and still shows it", async () => {
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
