// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import type { DealSuggestion } from "./dealsuggestions.queries";
import { day, panelNamed, renderWorklist, row, stub } from "./worklist.testkit";

// A Deal Scout suggestion on the Worklist: a row To review, and under it the
// suggestion with the evidence a rep reads before opening a deal.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const suggestion: DealSuggestion = {
  id: "sg-1",
  kind: "open_deal",
  state: "open",
  company_id: "co-1",
  company_name: "Acme GmbH",
  pipeline_id: "pl",
  stage_id: "s1",
  name_hint: "proposal_sent",
  amount_minor: null,
  currency: null,
  confidence: 0.8,
  created_at: "2026-08-30T09:00:00Z",
  evidence: [
    {
      kind: "attachment",
      attachment_id: "at-1",
      occurred_at: "2026-08-29T15:00:00Z",
      title: "Angebot_2026.pdf",
    },
  ],
};

function suggestionRow(id = "sg-1") {
  return row({
    id,
    source: "deal_suggestion",
    category: "decisions",
    level: 6,
    consequence: "data_drifts",
    destination: "review",
    actions: ["decide", "dismiss", "open"],
    subject: { type: "company", id: "co-1", label: "Acme GmbH" },
  });
}

describe("a suggested deal on the Worklist", () => {
  it("stands To review with its evidence under the row", async () => {
    stub(day({ queue: [suggestionRow()] }), {
      data: [suggestion],
      page: { has_more: false },
    });
    renderWorklist();

    const review = panelNamed(await screen.findByText(en["worklist.review"]));
    expect(
      await within(review).findByText("Sent: Angebot_2026.pdf"),
    ).toBeTruthy();
    expect(within(review).getByTestId("deal-suggestion")).toBeTruthy();
  });

  it("says it was decided when the suggestion is no longer there", async () => {
    stub(day({ queue: [suggestionRow("sg-gone")] }), {
      data: [suggestion],
      page: { has_more: false },
    });
    renderWorklist();

    expect(await screen.findByText(en["dealSuggestion.decided"])).toBeTruthy();
    expect(screen.queryByTestId("deal-suggestion")).toBeNull();
  });

  it("says the suggestion could not be read when the read fails", async () => {
    stub(day({ queue: [suggestionRow()] }));
    vi.stubGlobal("fetch", failingSuggestions(vi.mocked(globalThis.fetch)));
    renderWorklist();

    expect(
      await screen.findByText(en["dealSuggestion.unavailable"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["dealSuggestion.decided"])).toBeNull();
  });
});

// The queue's own stub, with the suggestions read answering a server error.
function failingSuggestions(queue: typeof fetch): typeof fetch {
  return async (input, init) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.split("?")[0].endsWith("/deal-suggestions")) {
      return new Response(JSON.stringify({ code: "internal" }), {
        status: 500,
        headers: { "Content-Type": "application/problem+json" },
      });
    }
    return queue(input, init);
  };
}
