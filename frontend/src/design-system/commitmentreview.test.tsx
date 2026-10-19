/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";

afterEach(cleanup);

import { CommitmentReview } from "./commitmentreview";

const decision = {
  locator: "pricing",
  summary: "Send revised pricing",
  source_quote: "I will send revised pricing.",
  outcome: "later_context_not_read",
  current_work: false,
  rule_version: "1",
  evidence_ids: [],
  evaluated_at: "2026-10-18T10:00:00Z",
};
describe("commitment review", () => {
  it("explains incomplete evidence without claiming fulfillment", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider initial="en">
        <CommitmentReview zone={viewerZone()} decisions={[decision]} />
      </LocaleProvider>,
    );
    const summary = screen
      .getAllByText("Commitment review")[0]
      ?.closest("summary");
    if (!summary) throw new Error("Review disclosure is missing");
    await user.click(summary);
    expect(screen.getByText(decision.source_quote)).not.toBeNull();
    expect(
      screen.getByText(
        "Later messages were not fully read. The commitment may already be fulfilled.",
      ),
    ).not.toBeNull();
  });
  it("does not relist admitted work as withheld", () => {
    const { container } = render(
      <LocaleProvider initial="en">
        <CommitmentReview
          zone={viewerZone()}
          decisions={[{ ...decision, current_work: true }]}
        />
      </LocaleProvider>,
    );
    expect(container.textContent).toBe("");
  });
});
