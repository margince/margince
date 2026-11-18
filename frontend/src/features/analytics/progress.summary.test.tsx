// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { render } from "./analytics.testkit";
import { sellerMarch, sellerMarchSummary } from "./progress.fixtures";
import type { ProgressAsk, SalesProgress } from "./progress.queries";
import { ProgressScenarios, ProgressSummary } from "./progress.summary";

afterEach(cleanup);

const ASK: ProgressAsk = {
  period: "month",
  year: 2026,
  month: 3,
  scope: { kind: "owner", id: sellerMarch.scope.id ?? "" },
};

function summaryStub(answer: () => Response): unknown[] {
  const asked: unknown[] = [];
  installFetchStub({
    "POST /analytics/progress/summary": (body: unknown) => {
      asked.push(body);
      return answer();
    },
  });
  return asked;
}

describe("ProgressSummary", () => {
  it("writes the summary from the page's figures and names who wrote it", async () => {
    const asked = summaryStub(() => jsonResponse(sellerMarchSummary));
    render(<ProgressSummary ask={ASK} progress={sellerMarch} />);
    expect(
      await screen.findByText(
        "You closed €60,000 of the €100,000 goal in March.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText("That leaves €40,000 still missing to the goal."),
    ).toBeTruthy();
    expect(screen.getByText("Written by Margince")).toBeTruthy();
    expect(asked[0]).toMatchObject({
      period: "month",
      year: 2026,
      month: 3,
      scope_kind: "owner",
      refresh: false,
    });
  });

  it("asks the model again when the reader refreshes", async () => {
    const asked = summaryStub(() => jsonResponse(sellerMarchSummary));
    const user = userEvent.setup();
    render(<ProgressSummary ask={ASK} progress={sellerMarch} />);
    await user.click(
      await screen.findByRole("button", { name: "Refresh summary" }),
    );
    await waitFor(() => expect(asked).toHaveLength(2));
    expect(asked[1]).toMatchObject({ refresh: true });
  });

  it("draws nothing when no model wrote a sentence", async () => {
    const asked = summaryStub(() =>
      jsonResponse({
        ...sellerMarchSummary,
        generated_by: "deterministic",
        sentences: [],
        paragraphs: undefined,
      }),
    );
    const { container } = render(
      <ProgressSummary ask={ASK} progress={sellerMarch} />,
    );
    await waitFor(() => expect(asked).toHaveLength(1));
    await waitFor(() => expect(container.textContent).toBe(""));
  });

  it("draws nothing against a server that has no summary route", async () => {
    const asked = summaryStub(
      () =>
        new Response(JSON.stringify({ code: "not_found" }), { status: 404 }),
    );
    const { container } = render(
      <ProgressSummary ask={ASK} progress={sellerMarch} />,
    );
    await waitFor(() => expect(asked).toHaveLength(1));
    await waitFor(() => expect(container.textContent).toBe(""));
  });

  it("offers a retry when the summary could not be read", async () => {
    summaryStub(
      () => new Response(JSON.stringify({ code: "internal" }), { status: 500 }),
    );
    render(<ProgressSummary ask={ASK} progress={sellerMarch} />);
    expect(await screen.findByRole("button", { name: "Retry" })).toBeTruthy();
  });
});

// March still running, against the €100,000 goal: €60,000 won, a Likely
// landing of €90,000 and €130,000 if every open deal closes. The amounts and
// their deltas are the server's; the block only words them.
const marchRunning: SalesProgress = {
  ...sellerMarch,
  state: "current",
  scenarios: {
    as_of: "2026-03-20T09:00:00Z",
    worst: { amount_minor: 6000000, delta_minor: -4000000 },
    expected: { amount_minor: 9000000, delta_minor: -1000000 },
    best: { amount_minor: 13000000, delta_minor: 3000000 },
    landing: {
      amount_minor: 9000000,
      measure: "likely",
      won_minor: 6000000,
      remaining_minor: 3000000,
    },
    eligible_count: 5,
    priced_count: 4,
    fx_missing_count: 0,
    scope_limited: false,
  },
};

describe("ProgressScenarios", () => {
  it("states each case against the goal and names the measure the landing used", () => {
    render(<ProgressScenarios progress={marchRunning} />);
    expect(screen.getByText("€60,000")).toBeTruthy();
    expect(screen.getByText("€40,000 below goal")).toBeTruthy();
    expect(screen.getByText("Won + Likely deals")).toBeTruthy();
    expect(screen.getByText("€10,000 below goal")).toBeTruthy();
    expect(screen.getByText("€30,000 above goal")).toBeTruthy();
    expect(
      screen.getByText(/Deals with no amount or no exchange rate are left out/),
    ).toBeTruthy();
  });

  it("opens the forecast's deals for the running month", () => {
    render(<ProgressScenarios progress={marchRunning} />);
    const links = screen.getAllByRole("link");
    expect(links).toHaveLength(3);
    expect(links[0]?.getAttribute("href")).toContain("period=this_month");
  });

  it("says a year has no scenarios", () => {
    render(
      <ProgressScenarios
        progress={{
          ...marchRunning,
          period: { kind: "year", year: 2026 },
          scenarios: undefined,
        }}
      />,
    );
    expect(
      screen.getByText(/The forecast has no year to read them from/),
    ).toBeTruthy();
  });

  it("draws nothing for a month the server gave no scenarios", () => {
    const { container } = render(
      <ProgressScenarios
        progress={{ ...marchRunning, scenarios: undefined }}
      />,
    );
    expect(container.textContent).toBe("");
  });
});
