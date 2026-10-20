/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { viewerZone } from "../format/timezone";
import { type Locale, LocaleProvider } from "../i18n";

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

it.each([
  {
    locale: "en",
    outcome: "semantic_completed",
    text: "The source describes work already done. No new task was created.",
  },
  {
    locale: "de",
    outcome: "semantic_completed",
    text: "Die Quelle beschreibt bereits erledigte Arbeit. Es wurde keine neue Aufgabe erstellt.",
  },
  {
    locale: "vi",
    outcome: "semantic_completed",
    text: "Nguồn mô tả công việc đã làm xong. Không tạo việc mới.",
  },
  {
    locale: "en",
    outcome: "semantic_conditional",
    text: "The commitment depends on a condition that has not been met.",
  },
  {
    locale: "de",
    outcome: "semantic_conditional",
    text: "Die Zusage hängt von einer noch nicht erfüllten Bedingung ab.",
  },
  {
    locale: "vi",
    outcome: "semantic_conditional",
    text: "Cam kết phụ thuộc vào một điều kiện chưa được đáp ứng.",
  },
  {
    locale: "en",
    outcome: "semantic_optional",
    text: "This is an offer, not an agreed commitment.",
  },
  {
    locale: "en",
    outcome: "semantic_request",
    text: "This asks for work that nobody has agreed to do.",
  },
  {
    locale: "en",
    outcome: "semantic_unclear",
    text: "The source does not establish an open commitment.",
  },
] satisfies { locale: Locale; outcome: string; text: string }[])(
  "explains $outcome in $locale",
  async ({ locale, outcome, text }) => {
    const user = userEvent.setup();
    const { container } = render(
      <LocaleProvider initial={locale}>
        <CommitmentReview
          zone={viewerZone()}
          decisions={[{ ...decision, outcome }]}
        />
      </LocaleProvider>,
    );
    const summary = container.querySelector("summary");
    if (!summary) throw new Error("Review disclosure is missing");
    await user.click(summary);
    expect(screen.getByText(text)).not.toBeNull();
    expect(screen.getByText(decision.source_quote)).not.toBeNull();
    expect(screen.queryByRole("button", { name: /create task/i })).toBeNull();
  },
);

it("keeps the condition and deadline words visible without inventing a date", async () => {
  const user = userEvent.setup();
  const { container } = render(
    <LocaleProvider initial="en">
      <CommitmentReview
        zone={viewerZone()}
        decisions={[
          {
            ...decision,
            outcome: "semantic_conditional",
            meaning: {
              state: "conditional",
              condition: "if the buyer agrees",
              deadline_precision: "range",
              deadline_quote: "next week",
            },
          },
        ]}
      />
    </LocaleProvider>,
  );
  const summary = container.querySelector("summary");
  if (!summary) throw new Error("Review disclosure is missing");
  await user.click(summary);
  expect(screen.getByText("Condition: if the buyer agrees")).not.toBeNull();
  expect(screen.getByText("Stated deadline: next week")).not.toBeNull();
});
