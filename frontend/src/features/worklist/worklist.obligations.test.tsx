// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("one obligation is drawn once", () => {
  // The top row is the one thing to do next, and the page says so once. A focus
  // card could draw that row, and a Next-up list the three after it. A reader
  // counting their morning would then count the first four rows twice.
  //
  // The first row is selected on arrival instead, which puts it in the context
  // pane without drawing it again.
  it("draws the top row once, not once on a card and once in the queue", async () => {
    stub(
      day({
        queue: [
          row({
            id: "focus-me",
            title: "Northstar renewal",
            band: "now",
            primary_action: "act",
            source: "deal_at_risk",
            category: "deals_at_risk",
            level: 3,
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-0000000000cc",
              label: "Northstar",
            },
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(await screen.findAllByText(/Northstar renewal/)).toHaveLength(1);
  });

  // Review work is judgement the queue collects, not a rep's next action. It
  // still belongs on the page, but it must not claim the top of it.
  it("keeps review work on the page without promoting it", async () => {
    stub(
      day({
        queue: [
          row({
            id: "dupe",
            title: "Two Acme records",
            band: "review",
            primary_action: "merge",
            source: "dedupe_candidate",
            category: "decisions",
            level: 6,
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 1, total: 1 },
      }),
    );
    renderWorklist();

    expect(await screen.findAllByText(/Two Acme records/)).toHaveLength(1);
  });

  // A row the server named no verb for is still real work and still drawn. It
  // simply has nothing to press, which the row says by drawing no control.
  it("draws a row the server named no verb for", async () => {
    stub(
      day({
        queue: [
          row({ id: "verbless", title: "Something happened", band: "now" }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    expect(await screen.findAllByText(/Something happened/)).toHaveLength(1);
  });
});

// One obligation, drawn once.
//
// The first item could be drawn as a focus card, as the head of a "next up"
// strip and as a queue row. Three drawings of one thing are not emphasis: a
// reader counts their morning wrong, and a rep meets the same customer twice.
//
// Counted over every `.worklist-row-title` on the page, not within its list
// items. The duplication this guards against renders outside the queue's <ol>,
// as a card above it drawing the top row again. A walk over <li> elements would
// filter out the one shape of the defect the test is named for.
//
// Titles rather than identities, and the fixture is built so that is sound:
// three rows, three distinct titles. Two rows may share a title in production
// (two tasks called "Follow up" are two obligations). The fixture keeps them
// distinct for the count to mean what it says, and `seen.size` at 3 holds that.
describe("every obligation is drawn once", () => {
  it("AC-WORKLIST-SDR-02: renders no row twice, including the one at the top", async () => {
    stub(
      day({
        queue: [
          row({
            id: "a",
            source: "customer_waiting",
            title: "Kirsten replied",
          }),
          row({ id: "b", source: "task", title: "Send the quote" }),
          row({ id: "c", source: "meeting", title: "Quarterly review" }),
        ],
        summary: { urgent: 1, due: 1, lower_priority: 1, total: 3 },
      }),
    );

    const { container } = renderWorklist();
    await screen.findByText("Kirsten replied");

    // Over the whole page, not over its list items. The focus card this test
    // keeps away draws its copy outside any <li>. Walking list items would
    // filter the duplicate out and pass while the screen shows it.
    const drawn = [...container.querySelectorAll(".worklist-row-title")].map(
      (title) => title.textContent ?? "",
    );
    const seen = new Map<string, number>();
    for (const title of drawn.filter(Boolean)) {
      seen.set(title, (seen.get(title) ?? 0) + 1);
    }

    const twice = [...seen.entries()].filter(([, times]) => times > 1);
    expect(twice).toEqual([]);
    expect(seen.size).toBe(3);
  });
});
