// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { day, renderWorklist, row, stub } from "./worklist.testkit";

// The standing line on a deal row.
//
// The row it exists for said "draft a reply" over a deal and nothing about the
// deal. Whether that reply is chasing a live buyer or reopening something cold
// changes what the rep writes, and the row made them open the deal page to find
// out.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function dealRow(verdict: unknown) {
  return row({
    id: "deal-row",
    source: "deal_at_risk",
    category: "deals_at_risk",
    level: 3,
    consequence: "deal_drifts",
    title: "Fleet retrofit",
    because: [{ kind: "quiet_days", value: { kind: "days", days: 41 } }],
    subject: { type: "deal", id: "11111111-1111-7111-8111-111111111111" },
    ...(verdict ? { verdict } : {}),
  } as Parameters<typeof row>[0]);
}

describe("the standing beside the step", () => {
  it("draws the standing, the sentence and who is speaking", async () => {
    stub(
      day({
        queue: [
          dealRow({
            standing: "blocked",
            line: "Legal has not returned the DPA.",
            source: "deal_status",
            as_of: "2026-08-31T06:42:00Z",
          }),
        ],
      }),
    );
    renderWorklist();

    expect(await screen.findByText("Blocked")).toBeTruthy();
    expect(screen.getByText("Legal has not returned the DPA.")).toBeTruthy();
    expect(screen.getByText("Margince believes")).toBeTruthy();
  });

  // The night's finding is prose about the deal and carries no standing word.
  // Drawing an invented one would put a judgement on the row that nobody made.
  it("draws a brief finding as a sentence with no standing badge", async () => {
    stub(
      day({
        queue: [
          dealRow({
            line: "The buyer asked for pricing and nobody answered.",
            source: "brief_finding",
          }),
        ],
      }),
    );
    renderWorklist();

    expect(
      await screen.findByText(
        "The buyer asked for pricing and nobody answered.",
      ),
    ).toBeTruthy();
    for (const word of ["Live", "Drifting", "Blocked", "Cold"]) {
      expect(screen.queryByText(word)).toBeNull();
    }
  });

  // THE case the source field exists for. A row with no verdict still explains
  // itself, through the typed reason the client phrases — and that explanation
  // must not arrive wearing a label that says Margince read the deal.
  it("labels no reading as Margince's when the server sent no verdict", async () => {
    stub(day({ queue: [dealRow(null)] }));
    renderWorklist();

    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(screen.queryByText("Margince believes")).toBeNull();
    // The deterministic explanation is still there.
    expect(screen.getByText(/41/)).toBeTruthy();
  });

  it("says when the reading was taken, so a stale one can be told", async () => {
    stub(
      day({
        queue: [
          dealRow({
            standing: "cold",
            line: "Quiet since June.",
            source: "deal_status",
            as_of: "2026-08-24T06:42:00Z",
          }),
        ],
      }),
    );
    renderWorklist();

    expect(await screen.findByText(/Read /)).toBeTruthy();
  });

  it("says nothing about when a reading with no instant was taken", async () => {
    stub(
      day({
        queue: [
          dealRow({ line: "Quiet since June.", source: "brief_finding" }),
        ],
      }),
    );
    renderWorklist();

    expect(await screen.findByText("Quiet since June.")).toBeTruthy();
    expect(screen.queryByText(/Read /)).toBeNull();
  });
});
