/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { emptyRollup, jsonResponse } from "../../testing/company.fixtures";
import { renderWithQueryClient } from "../../testing/queryrender";
import { HierarchyRollupPanel } from "./companies.hierarchy";

type Rollup = components["schemas"]["CompanyHierarchyRollup"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The server answers each period with its own won total and window. The
// pipeline is the same figure under every period, as the server sends it.
function answerFor(params: URLSearchParams): Rollup {
  const base = emptyRollup as Rollup;
  const pipeline = { amount_minor: 480_000, currency: "EUR" };
  if (params.has("period_year")) {
    return {
      ...base,
      weighted_pipeline: pipeline,
      closed_won: { amount_minor: 9_000_000, currency: "EUR" },
      closed_won_period: {
        ...base.closed_won_period,
        grain: "year",
        label: "2026",
        chosen: true,
        previous: "2025",
      },
    };
  }
  if (params.get("period_quarter") === "2026-Q1") {
    return {
      ...base,
      weighted_pipeline: pipeline,
      closed_won: { amount_minor: 300_000, currency: "EUR" },
      closed_won_period: {
        ...base.closed_won_period,
        label: "2026-Q1",
        chosen: true,
        previous: "2025-Q4",
        next: "2026-Q2",
      },
    };
  }
  return {
    ...base,
    weighted_pipeline: pipeline,
    closed_won: { amount_minor: 120_000, currency: "EUR" },
  };
}

function stubRollup(): URLSearchParams[] {
  const asked: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (url.pathname.endsWith("/hierarchy-rollup")) {
        asked.push(url.searchParams);
        return jsonResponse(answerFor(url.searchParams));
      }
      return jsonResponse({});
    }),
  );
  return asked;
}

describe("HierarchyRollupPanel periods", () => {
  it("reads the quarter under way by default and says the pipeline is today's", async () => {
    const asked = stubRollup();
    renderWithQueryClient(<HierarchyRollupPanel companyId="o-1" />);

    await waitFor(() => expect(screen.getByText("€1,200.00")).toBeTruthy());
    expect(screen.getByText("Won this quarter")).toBeTruthy();
    expect(screen.getByText("Weighted deal value today")).toBeTruthy();
    expect(
      screen.getByText(
        "Won deals count in 2026-Q2 by the day they were won. The weighted deal value is the open deals today.",
      ),
    ).toBeTruthy();
    expect([...asked[0].keys()]).toEqual(["scope"]);
  });

  it("asks for the year under way by the server's own label and moves only the won figure", async () => {
    const asked = stubRollup();
    const user = userEvent.setup();
    renderWithQueryClient(<HierarchyRollupPanel companyId="o-1" />);
    await waitFor(() => expect(screen.getByText("€1,200.00")).toBeTruthy());

    await user.click(
      screen.getByRole("combobox", { name: "Period for won deals" }),
    );
    await user.click(screen.getByRole("option", { name: "Year" }));

    await waitFor(() => expect(screen.getByText("Won in 2026")).toBeTruthy());
    expect(screen.getByText("€90,000.00")).toBeTruthy();
    expect(screen.getByText("€4,800.00")).toBeTruthy();
    expect(asked.at(-1)?.get("period_year")).toBe("2026");
  });

  it("steps back to the period before and refuses a step past the one under way", async () => {
    const asked = stubRollup();
    const user = userEvent.setup();
    renderWithQueryClient(<HierarchyRollupPanel companyId="o-1" />);
    await waitFor(() => expect(screen.getByText("€1,200.00")).toBeTruthy());
    const forward = screen.getByRole("button", {
      name: "The next period has not begun",
    });
    expect((forward as HTMLButtonElement).disabled).toBe(true);

    await user.click(screen.getByRole("button", { name: "Show 2026-Q1" }));

    await waitFor(() =>
      expect(screen.getByText("Won in 2026-Q1")).toBeTruthy(),
    );
    expect(asked.at(-1)?.get("period_quarter")).toBe("2026-Q1");
    expect(screen.getByRole("button", { name: "Show 2026-Q2" })).toBeTruthy();
  });
});
