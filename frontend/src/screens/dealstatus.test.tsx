// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { DealStatusCardPanel } from "./dealstatus";

// What Deal360 owes a reader, in the order it owes it.
//
// The card is read two ways and only one of them is reading: a rep working
// this deal reads the prose, a rep scanning thirty before a forecast call
// looks for the one that needs them. The second read is what these tests are
// about, because it is the one the old card defeated — the call sat fourth,
// under three paragraphs, so the single word a scanner needs was the last
// thing they reached.
//
// So: the call is in the document before the prose, the prose is behind a
// fold, and a coverage finding states the NUMBER that tripped it rather than
// a label the reader has to trust.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const DEAL = "01a03000-0000-7000-8000-000000000001";

type Risk = {
  kind: string;
  summary: string;
  days_since_touch?: number;
};

function serve({
  standing = "blocked",
  risks = [] as Risk[],
  sectionsOmitted = [] as string[],
}) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input);
      if (url.includes("/status")) {
        return new Response(
          JSON.stringify({
            deal_id: DEAL,
            story: {
              sentences: [{ text: "They asked for slots.", evidence: [] }],
            },
            verdict: {
              standing,
              because: {
                sentences: [
                  { text: "Nobody sent the times.", evidence: [] },
                  { text: "That was three weeks ago.", evidence: [] },
                ],
              },
            },
            generated_at: "2026-08-24T00:00:00Z",
            generated_by: "model",
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      if (url.includes("/coverage")) {
        return new Response(
          JSON.stringify({
            deal_id: DEAL,
            stakeholders: [],
            risks,
            sections_omitted: sectionsOmitted,
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ data: [] }), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

function renderCard() {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <DealStatusCardPanel dealId={DEAL} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("Deal360 leads with the call", () => {
  it("puts the standing before the prose in the document", async () => {
    serve({ standing: "blocked" });
    renderCard();
    const standing = await screen.findByText("Blocked");
    const prose = await screen.findByText("They asked for slots.");
    // Document order, not styling: a scanner reads down the page, and a call
    // rendered after the paragraphs is a call they reach last however it is
    // painted. compareDocumentPosition answers the question the reader asks.
    expect(
      standing.compareDocumentPosition(prose) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("shows the first line of the reasoning beside the call", async () => {
    serve({ standing: "drifting" });
    renderCard();
    expect(await screen.findByText("Drifting")).toBeInTheDocument();
    // The head carries ONE line. The rest is in the fold, so a reader scanning
    // gets the call and its reason without the whole briefing.
    expect(await screen.findByText("Nobody sent the times.")).toBeVisible();
  });

  it("keeps the full briefing behind a closed fold", async () => {
    serve({ standing: "live" });
    renderCard();
    await screen.findByText("Live");
    const fold = document.querySelector("details.deal360-fold");
    expect(fold).not.toBeNull();
    // Closed, not absent: every word is still on the page and still cited.
    // A fold that shipped open would be the old card with a summary on it.
    expect((fold as HTMLDetailsElement).open).toBe(false);
    expect(screen.getByText("They asked for slots.")).toBeInTheDocument();
  });

  it("renders a standing this build does not know rather than dropping it", async () => {
    // A newer server sending a fifth word is still making a call, and a card
    // that showed nothing would report a deal with no verdict at all.
    serve({ standing: "stalled_pending_legal" });
    renderCard();
    expect(await screen.findByText("stalled_pending_legal")).toBeVisible();
  });
});

describe("Deal360's signal chips state their trigger", () => {
  it("shows the number that tripped the rule, not just its name", async () => {
    serve({
      risks: [
        {
          kind: "going_cold",
          summary: "No contact in a long time.",
          days_since_touch: 84,
        },
      ],
    });
    renderCard();
    expect(await screen.findByText("Going cold")).toBeVisible();
    // The figure is the point. "Going cold" is a label a reader has to trust;
    // "84 days" is the finding, and they can check it against the timeline.
    expect(await screen.findByText("84 days")).toBeVisible();
  });

  it("renders a rule with no figure as its label alone", async () => {
    serve({
      risks: [{ kind: "coverage_gap", summary: "Nobody is championing this." }],
    });
    renderCard();
    expect(await screen.findByText("No engaged champion")).toBeVisible();
    expect(screen.queryByText(/\d+ days/)).toBeNull();
  });

  it("draws no strip when the findings were withheld, even if rows arrive", async () => {
    // Withheld is not "nothing is wrong". A caller without the relationship
    // grant is served no findings, and a strip drawn from that would report a
    // clean bill of health from a check that never ran.
    //
    // The fixture sends a risk ALONGSIDE the withheld marker, and that is the
    // whole test. With an empty list the two readings — "withheld" and "no
    // findings" — produce the same empty strip, so the assertion held whether
    // or not the code checked anything; deleting the withheld check left it
    // green. A row that must NOT be rendered is what makes the check visible.
    serve({
      risks: [
        {
          kind: "going_cold",
          summary: "No contact in a long time.",
          days_since_touch: 84,
        },
      ],
      sectionsOmitted: ["risks"],
    });
    renderCard();
    await screen.findByText("Blocked");
    await waitFor(() => {
      expect(document.querySelector(".r360-signals")).toBeNull();
    });
    expect(screen.queryByText("Going cold")).toBeNull();
  });
});
