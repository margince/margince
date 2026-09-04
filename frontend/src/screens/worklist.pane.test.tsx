// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  day,
  jsonResponse,
  renderWorklist,
  row,
  stub,
  type Worklist,
} from "./worklist.testkit";

// What opens beside the queue, and — just as much — what does not.
//
// The rank is the way in, and it is a control only where pressing it opens
// something. A rank that toggles a pressed state over no pane teaches the
// reader that the page lies about what is pressable, so these cases assert
// both halves: a button where a pane exists, a plain number where none does.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("what the selected row is about", () => {
  it("draws no pane until the reader picks a row", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "A task",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-0000000000aa",
              label: "Kirsten Vogel",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The full-width list a reader had before selection existed. An empty
    // column standing ready reads as a pane that failed to load.
    await screen.findByText("A task · Kirsten Vogel");
    expect(screen.queryByText("They last wrote")).toBeNull();
  });

  it("has the first row in hand on arrival, and closes it on a press", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "A task",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-0000000000aa",
              label: "Kirsten Vogel",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // OPEN ALREADY. The first row is the day's answer to what matters most, so
    // it is the row in hand when the page arrives — which is what lets the
    // queue be the focus without a card above it repeating the row.
    //
    // The pane names the record and answers the question the row cannot: how
    // long the silence has run, in both directions.
    await screen.findByText("They last wrote");
    expect(screen.getByText("We last wrote")).toBeTruthy();

    // And it is still a control: pressing the rank puts the row down.
    await userEvent.click(screen.getByRole("button", { name: /^Show what/ }));
    await waitFor(() => {
      expect(screen.queryByText("They last wrote")).toBeNull();
    });

    // And picks it up again, so the press is a toggle rather than a one-way
    // dismissal the reader cannot undo.
    await userEvent.click(screen.getByRole("button", { name: /^Show what/ }));
    await screen.findByText("They last wrote");
  });

  // A rank that opens nothing is not a control.
  //
  // This case asserted the ABSENCE of a pane after a click, which passed
  // against a button that toggled a pressed state and opened nothing — the
  // dead control it was meant to describe. The claim worth making is about the
  // rank: a deal row has no pane, so its rank is a plain number with no button
  // to press.
  it("draws the rank as a plain number for a row with no pane", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "Northstar renewal",
            source: "deal_at_risk",
            category: "deals_at_risk",
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-0000000000bb",
              label: "Northstar",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The row drew — this is not a test that the page failed to render.
    await screen.findByText("Northstar renewal");
    expect(screen.queryByRole("button", { name: /^Show what/ })).toBeNull();
  });

  // The other half of the same rule. Without this case, "no rank button" would
  // also be satisfied by a page that never drew one at all.
  it("draws the rank as a button where a pane opens, and it toggles", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "Re: pricing",
            source: "customer_waiting",
            category: "customer_waiting",
            subject: {
              type: "person",
              id: "01a05500-0000-7000-8000-0000000000cc",
              label: "Alice Müller",
            },
          }),
        ],
        summary: { urgent: 1, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    renderWorklist();

    // The aside is there on arrival, because the first row is in hand.
    await waitFor(() => {
      expect(screen.getByRole("complementary")).toBeTruthy();
    });

    // And the rank is a real control, not a pressed state over a pane that was
    // going to be there anyway: pressing it takes the aside away.
    await userEvent.click(
      await screen.findByRole("button", { name: /^Show what/ }),
    );
    await waitFor(() => {
      expect(screen.queryByRole("complementary")).toBeNull();
    });

    await userEvent.click(screen.getByRole("button", { name: /^Show what/ }));
    await waitFor(() => {
      expect(screen.getByRole("complementary")).toBeTruthy();
    });
  });

  it("closes the pane when the selected row leaves the queue", async () => {
    const withRow = day({
      queue: [
        row({
          id: "one",
          title: "A task",
          subject: {
            type: "person",
            id: "01a05500-0000-7000-8000-0000000000aa",
            label: "Kirsten Vogel",
          },
        }),
      ],
      summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
    });
    // The same day with the row gone — a disposition, a filter, a refetch that
    // found it answered. The pane must not go on describing a record whose row
    // is no longer on the page.
    let current: Worklist = withRow;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const url = String(input instanceof Request ? input.url : input);
        if (url.includes("/worklist")) {
          return jsonResponse(current);
        }
        return jsonResponse({ data: [] });
      }),
    );
    renderWorklist();

    // In hand on arrival, so there is nothing to press to open it.
    await screen.findByText("They last wrote");

    current = day({
      queue: [],
      summary: { urgent: 0, due: 0, lower_priority: 0, total: 0 },
    });
    // Re-render through the filter, which refetches: the row is gone, and the
    // pane goes with it rather than outliving the row it describes.
    await userEvent.click(screen.getByRole("button", { name: /Decisions/ }));
    await waitFor(() => {
      expect(screen.queryByText("They last wrote")).toBeNull();
    });
  });
  it("draws no aside landmark for a row that has no pane", async () => {
    stub(
      day({
        queue: [
          row({
            id: "one",
            title: "Northstar renewal",
            source: "deal_at_risk",
            category: "deals_at_risk",
            subject: {
              type: "deal",
              id: "01a05500-0000-7000-8000-0000000000bb",
              label: "Northstar",
            },
          }),
        ],
        summary: { urgent: 0, due: 0, lower_priority: 0, total: 1 },
      }),
    );
    const { container } = renderWorklist();

    await screen.findByText("Northstar renewal");
    // A deal row has no pane. An empty <aside> would still be announced as a
    // landmark and still take its third of the grid, which reads as a pane
    // that failed rather than as one that was never meant to be there.
    //
    // Reached by rendering rather than by clicking the rank: the rank on such
    // a row is no longer a button, because a control that opens nothing
    // teaches the reader that the page lies about what is pressable.
    await waitFor(() => {
      expect(container.querySelectorAll("aside")).toHaveLength(0);
    });
  });
});

// What `#/worklist/<segment>` opens on.
//
// The address is the other half of the team board's row: a board that writes a
// hash nothing reads is a row that goes nowhere. These assert the REQUEST, not
// the rendering, because whose day the page is showing is a question about
// which day it asked the server for.
