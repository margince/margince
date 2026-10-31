/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { forgetSeat } from "../../lib/storage";
import { reportingFixtureFrames } from "../../testing/reporting.fixtures";
import { backlogList } from "./analytics.overview.scenariodata";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  overviewAddress,
  type ScenarioName,
} from "./analytics.overview.scenarios.fixtures";
import {
  isMovement,
  landingBasis,
  metric,
  openOverview,
  readings,
  salesEvaluation,
  startingMovement,
} from "./analytics.overview.testkit";

// Sales review's first screen and its action list: the controls, the lead
// figure with its target, three readings, the summary and the top findings.

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

function openScenario(name: ScenarioName, address = overviewAddress("sales")) {
  const pair = OVERVIEW_SCENARIOS[name].views.sales;
  if (!isSetup(pair)) throw new Error(`${name} excludes Sales review`);
  return openOverview(address, pair.answer, pair.routes);
}

async function actionList() {
  return within(
    await screen.findByRole("region", { name: "Needs your attention" }),
  );
}

const hero = () => within(screen.getByRole("region", { name: "Sales won" }));

describe("the controls", () => {
  it("keeps the pipeline and close window behind Filters, with what they hold always shown", async () => {
    openScenario("populated");
    await readings();
    expect(screen.getByRole("combobox", { name: "Date range" })).toBeVisible();
    expect(screen.queryByRole("combobox", { name: "Pipeline" })).toBeNull();
    expect(screen.getByText("Pipeline: Sales")).toBeVisible();
    expect(screen.getByText(/^Closing: /)).toBeVisible();
    const user = userEvent.setup({ delay: null });
    const filters = screen.getByRole("button", { name: "Filters" });
    expect(filters).toHaveAttribute("aria-expanded", "false");
    await user.click(filters);
    expect(filters).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("combobox", { name: "Pipeline" })).toBeVisible();
    expect(
      screen.getByRole("combobox", { name: "Expected close window" }),
    ).toBeVisible();
  });

  it("puts Save view, the templates and the export behind More", async () => {
    openScenario("populated");
    await readings();
    expect(screen.queryByRole("link", { name: "Report templates" })).toBeNull();
    const user = userEvent.setup({ delay: null });
    await user.click(screen.getByRole("button", { name: "More" }));
    expect(
      await screen.findByRole("link", { name: "Report templates" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Export CSV" })).toBeVisible();
  });
});

describe("the lead and its readings", () => {
  it("shows exactly three readings beside the lead", async () => {
    openScenario("populated");
    const strip = await readings();
    const cards = strip
      .getAllByRole("generic")
      .filter((node) => node.matches("section.stat-card"));
    expect(cards).toHaveLength(3);
    for (const label of ["Expected finish", "Coverage", "Closed win rate"])
      expect(strip.getByText(label)).toBeVisible();
  });

  it("keeps a measured zero's explanation and actions with no blank chart", async () => {
    openScenario("trueZero");
    await readings();
    expect(hero().getByText("€0")).toBeVisible();
    expect(hero().queryByRole("figure")).toBeNull();
    expect(
      await screen.findByRole("region", { name: "Needs your attention" }),
    ).toBeVisible();
  });

  it("explains a target of another scope once, in the lead, with the way to compare", async () => {
    openScenario("wrongTargetScope");
    await readings();
    expect(
      hero().getByText("Pick one team to compare with a target"),
    ).toBeVisible();
    expect(hero().queryByText(/still to win/)).toBeNull();
    expect(
      screen.getAllByText("Pick one team to compare with a target"),
    ).toHaveLength(1);
  });
});

describe("the action list", () => {
  it("names the top three findings, then the groups counted over the whole list", async () => {
    const asked = openScenario("largeBacklog");
    const list = await actionList();
    expect(await list.findAllByText(/^Review\b/)).toHaveLength(3);
    const { groups, totals } = backlogList(
      reportingFixtureFrames.quarterCutoff.clock,
    );
    expect(
      list.getByText(
        `All open checks: ${totals.findings} findings · ${totals.deals} deals`,
      ),
    ).toBeVisible();
    expect(list.getAllByRole("link", { name: "Show" })).toHaveLength(
      groups.length,
    );
    expect(list.getByText("3 findings · 3 deals")).toBeVisible();
    // The list was read once, whole: the groups are not counted from a page.
    const reads = asked.filter(({ url }) =>
      url.pathname.endsWith("/forecast/assurance/exceptions"),
    );
    expect(reads).toHaveLength(1);
  });

  it("opens each finding and group on the review, with the list's own scope", async () => {
    openScenario("largeBacklog");
    const list = await actionList();
    const [first] = await list.findAllByRole("link", { name: "Answer" });
    const dials = (link: HTMLElement | undefined) =>
      new URLSearchParams(link?.getAttribute("href")?.split("?")[1]);
    expect(first?.getAttribute("href")).toMatch(/^\/#\/analytics\/forecast\?/);
    expect(dials(first).get("finding")).toBe(
      "41000000-0000-4000-8000-000000000000",
    );
    expect(dials(first).get("scope")).toBeTruthy();
    // The list reads every pipeline, so its doors carry none.
    expect(dials(first).has("pipeline")).toBe(false);
    const [group] = list.getAllByRole("link", { name: "Show" });
    expect(dials(group).get("check")).toBe("no_economic_buyer");
    expect(dials(group).get("scope")).toBe(dials(first).get("scope"));
    expect(list.getByRole("link", { name: "View all checks" })).toBeVisible();
  });

  it("says who owns a finding, or that nobody does, and never invents a date", async () => {
    openScenario("largeBacklog");
    const list = await actionList();
    await list.findAllByText(/^Review\b/);
    for (const facts of list.getAllByText(/ · (Unassigned|Found) /))
      expect(facts.textContent).not.toMatch(/Due/);
    expect(list.getAllByText(/Found 12 Oct/).length).toBeGreaterThan(0);
  });

  it("offers a read-only reviewer the finding to read, not an answer", async () => {
    openScenario("readOnlyReviewer");
    const list = await actionList();
    expect(await list.findAllByRole("link", { name: "Open" })).toHaveLength(3);
    expect(list.queryByRole("link", { name: "Answer" })).toBeNull();
  });

  it("keeps a failed source on its own maintenance line, apart from the findings", async () => {
    openScenario("sourceUnavailable");
    const list = await actionList();
    expect(await list.findByText("Connection maintenance")).toBeVisible();
    expect(
      list.getByText("1 data source could not be read in the last check."),
    ).toBeVisible();
    expect(list.queryByText(/^Review\b/)).toBeNull();
  });
});

describe("the summary", () => {
  it("links the count of checks to the action list", async () => {
    openScenario("largeBacklog");
    const summary = within(
      await screen.findByRole("region", { name: "Summary" }),
    );
    const sentence = await summary.findByRole("button", {
      name: "24 open checks on 7 deals need an answer.",
    });
    const user = userEvent.setup({ delay: null });
    await user.click(sentence);
    await waitFor(() =>
      expect(document.activeElement?.id).toBe("analytics-interventions"),
    );
  });

  it("gives no on-target verdict while the checks could not be read", async () => {
    openScenario("sourceUnavailable");
    const summary = within(
      await screen.findByRole("region", { name: "Summary" }),
    );
    expect(
      await summary.findByText(
        "The open checks could not be read, so nothing is said here about what needs attention.",
      ),
    ).toBeVisible();
    expect(summary.queryByText(/above the|on target|reached the/)).toBeNull();
  });
});

describe("a reader who may not read forecast checks", () => {
  it("says the checks are not theirs to see, and the summary says nothing about them", async () => {
    const pair = OVERVIEW_SCENARIOS.populated.views.sales;
    if (!isSetup(pair)) throw new Error("populated draws Sales review");
    openOverview(overviewAddress("sales"), pair.answer, {
      ...pair.routes,
      grants: {
        report_definition: ["read"],
        reporting_framework: ["read"],
        sales_target: ["read"],
        deal: ["read"],
        lead: ["read"],
        pipeline: ["read"],
      },
    });
    const list = await actionList();
    expect(
      await list.findByText("Forecast checks are not available to you."),
    ).toBeVisible();
    expect(
      list.queryByText("No forecast check is open for this scope."),
    ).toBeNull();
    const summary = within(screen.getByRole("region", { name: "Summary" }));
    expect(summary.queryByText(/open checks?/)).toBeNull();
  });
});

describe("the composition of the expected finish", () => {
  it("counts only the priced deals the totals hold, and names those left out", async () => {
    openOverview(overviewAddress("sales"), (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({
            metrics: [
              metric("bookings_won", 42_000_000),
              metric("forecast_landing", 76_000_000, {
                forecast_basis: landingBasis(
                  { won: 42_000_000, remaining: 34_000_000 },
                  {
                    eligible_count: 18,
                    priced_count: 17,
                    fx_missing_count: 2,
                    included_count: 15,
                  },
                ),
              }),
            ],
          }),
    );
    const summary = within(
      await screen.findByRole("region", { name: "Summary" }),
    );
    expect(
      await summary.findByText(
        "Counts 15 of the 18 open deals the forecast considered.",
      ),
    ).toBeVisible();
    expect(
      summary.getByText(
        "Left out: 1 deal with no amount, 2 deals with no exchange rate.",
      ),
    ).toBeVisible();
  });
});

describe("individual performance off", () => {
  it("keeps the owner on each action row while the owner chart names nobody", async () => {
    const pair = OVERVIEW_SCENARIOS.individualPerformanceOff.views.sales;
    if (!isSetup(pair)) throw new Error("the scenario draws Sales review");
    openOverview(overviewAddress("sales"), pair.answer, {
      ...pair.routes,
      checks: () =>
        new Response(
          JSON.stringify(
            backlogList(reportingFixtureFrames.quarterCutoff.clock),
          ),
          { headers: { "Content-Type": "application/json" } },
        ),
    });
    const routed = globalThis.fetch;
    globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input instanceof Request ? input.url : input);
      if (!url.includes("/users/names")) return routed(input, init);
      const data = new URL(url, "http://localhost").searchParams
        .getAll("id")
        .map((id) => ({ id, display_name: `Owner ${id.slice(-2)}` }));
      return Promise.resolve(
        new Response(JSON.stringify({ data }), {
          headers: { "Content-Type": "application/json" },
        }),
      );
    }) as typeof fetch;
    const list = await actionList();
    // Responsibility for a finding is not a performance comparison.
    expect(await list.findAllByText(/ · Owner a1 · Found/)).toHaveLength(3);
    const chart = within(
      screen.getByRole("region", { name: "Sales won by team member" }),
    );
    expect(chart.queryByText(/Owner a/)).toBeNull();
  });

  it("shows no team member's name in the owner chart or anywhere on the page", async () => {
    openScenario("individualPerformanceOff");
    await readings();
    expect(
      await screen.findByRole("heading", { name: "Sales won by team member" }),
    ).toBeVisible();
    expect(screen.queryByText(/Maya|Jordan|Priya/)).toBeNull();
  });
});
