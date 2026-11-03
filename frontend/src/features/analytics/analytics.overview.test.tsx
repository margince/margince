/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { forgetSeat, STORAGE_KEYS, writeStored } from "../../lib/storage";
import { reportingStoryEvaluation } from "../../testing/reporting.fixtures";
import { jsonResponse } from "../../testing/story-utils";
import {
  ADMIN_GRANTS,
  bySeat,
  compared,
  isMovement,
  metric,
  openOverview as open,
  readings,
  salesEvaluation,
  startingMovement,
} from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

describe("where Analytics opens", () => {
  it("opens Overview first, as the first tab", async () => {
    open("#/analytics", bySeat("leader"));
    await screen.findByRole("button", { name: "Overview" });
    const row = screen.getByRole("group", { name: "Analytics sections" });
    const tabs = within(row).getAllByRole("button");
    expect(tabs[0]).toHaveTextContent("Overview");
    expect(tabs[0]).toHaveAttribute("aria-pressed", "true");
    expect(await screen.findByText("Expected finish")).toBeVisible();
  });

  it.each([
    ["#/analytics/performance?view=sdr", "SDR outcomes"],
    ["#/analytics?view=leads", "Leads"],
    ["#/analytics/performance", "Performance"],
    ["#/analytics/deals-by-stage", "Pipeline"],
  ])("keeps the old address %s on its own tab", async (address, tab) => {
    open(address, bySeat("leader"));
    await waitFor(() =>
      expect(
        within(
          screen.getByRole("group", { name: "Analytics sections" }),
        ).getByRole("button", { name: tab }),
      ).toHaveAttribute("aria-pressed", "true"),
    );
  });

  it("recommends My prospecting to a reader measuring their own work in an SDR team", async () => {
    open("#/analytics", bySeat("sdr"), { seat: "sdr" });
    expect(
      await screen.findByRole("button", { name: "My prospecting" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("recommends Sales review to the same team's manager", async () => {
    open("#/analytics", bySeat("leader"), { template: "sdr" });
    expect(
      await screen.findByRole("button", { name: "Sales review" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("opens the view a shared link names over the one the reader chose last", async () => {
    writeStored(STORAGE_KEYS.analyticsOverviewView, "sales");
    open("#/analytics/overview?overview=prospecting", bySeat("sdr"), {
      seat: "sdr",
    });
    expect(
      await screen.findByRole("button", { name: "My prospecting" }),
    ).toHaveAttribute("aria-pressed", "true");
  });

  it("remembers an explicit choice and puts it in the address", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics?period=this_month", bySeat("leader"));
    await user.click(
      await screen.findByRole("button", { name: "My prospecting" }),
    );
    expect(window.location.hash).toContain("overview=prospecting");
    // The sales period does not follow into the other view.
    expect(window.location.hash).not.toContain("period=");
    cleanup();
    open("#/analytics", bySeat("leader"));
    expect(
      await screen.findByRole("button", { name: "My prospecting" }),
    ).toHaveAttribute("aria-pressed", "true");
  });
});

describe("Sales review", () => {
  it("leads with sales won and states the quarter's position in readings and words", async () => {
    open("#/analytics", bySeat("leader"));
    const strip = await readings();
    const hero = within(screen.getByRole("region", { name: "Sales won" }));
    expect(hero.getByText("€420k")).toBeVisible();
    expect(strip.queryByText("€420k")).toBeNull();
    expect(strip.getByText("€760k")).toBeVisible();
    // The shortfall stands beside target progress, not as a reading.
    expect(
      hero.getByText(
        "Projected shortfall €40k, from an expected finish of €760k",
      ),
    ).toBeVisible();
    expect(strip.getByText("3.6×")).toBeVisible();
    expect(
      strip.getByText(
        "€1.3m eligible to close this quarter / €380k still to win",
      ),
    ).toBeVisible();
    expect(
      screen.getByText(
        /Expected finish for .+: €760k, €40k short of the €800k target\./,
      ),
    ).toBeVisible();
  });

  it("asks the preset's trend, stage and team charts apart from the figures, and draws them", async () => {
    const asked = open("#/analytics", (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({ charts: reportingStoryEvaluation.charts }),
    );
    expect(
      await screen.findByRole("heading", { name: "Open pipeline by stage" }),
    ).toBeVisible();
    expect(
      screen.getByRole("heading", { name: "Sales won by team member" }),
    ).toBeVisible();
    const reads = asked.filter(
      ({ url }) =>
        url.pathname.endsWith("/analytics/evaluate") && !isMovement(url),
    );
    // The figures are one read with no chart; the charts another, with only
    // the metrics they belong to.
    const charts = reads.find(({ url }) => url.searchParams.has("blocks"));
    expect(charts?.url.searchParams.getAll("blocks")).toEqual([
      "bookings_trend",
      "stage_distribution",
      "owner_attainment",
    ]);
    expect(charts?.url.searchParams.getAll("metrics")).toEqual([
      "bookings_won",
      "open_pipeline",
    ]);
    expect(
      reads.filter(({ url }) => !url.searchParams.has("blocks")),
    ).toHaveLength(1);
  });

  it("shows the win rate and no gap without a target, and tells a reader who sets targets", async () => {
    open(
      "#/analytics",
      (url) =>
        isMovement(url)
          ? startingMovement()
          : salesEvaluation({
              comparison: {
                status: "no_target",
                target_metric: "bookings_won",
              },
            }),
      { grants: ADMIN_GRANTS },
    );
    const strip = await readings();
    expect(strip.getByText("Closed win rate")).toBeVisible();
    expect(strip.queryByText("Gap to target")).toBeNull();
    expect(screen.getByRole("link", { name: "Set targets" })).toBeVisible();
    expect(screen.getByText(/No sales target is set for/)).toBeVisible();
  });

  it("tells a reader who may not set targets who can", async () => {
    open("#/analytics", (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({
            comparison: { status: "no_target", target_metric: "bookings_won" },
          }),
    );
    expect(
      await screen.findByText(/A colleague who manages targets/),
    ).toBeVisible();
    expect(screen.queryByRole("button", { name: "Set targets" })).toBeNull();
  });

  it("offers one click to the quarter on another period", async () => {
    const user = userEvent.setup({ delay: null });
    open("#/analytics?period=this_month", (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({ metrics: [metric("bookings_won", 1_000_000)] }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Show this quarter" }),
    );
    expect(window.location.hash).toContain("period=this_quarter");
  });

  it("says a figure failed instead of concluding from it", async () => {
    open("#/analytics", (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({
            metrics: [
              metric("bookings_won", null, {
                coverage: { status: "failed", withheld: false },
              }),
              metric("open_pipeline", 0),
              metric("forecast_landing", 76_000_000, {
                target_comparison: {
                  status: "no_target",
                  target_metric: "bookings_won",
                },
              }),
            ],
          }),
    );
    await readings();
    expect(screen.getByText("Could not be calculated")).toBeVisible();
    expect(
      screen.getByText(/Some figures could not be read just now/),
    ).toBeVisible();
    expect(screen.queryByText(/Won in/)).toBeNull();
  });

  it("does not call a failed checks read a clear list", async () => {
    open("#/analytics", bySeat("leader"), {
      checks: () => jsonResponse({ title: "Unavailable", status: 503 }, 503),
    });
    await readings();
    expect(await screen.findByText(/Some data did not load/)).toBeVisible();
    expect(
      screen.queryByText("No forecast check is open for this scope."),
    ).toBeNull();
  });

  it("says a clear list only once the checks answered with none", async () => {
    open("#/analytics", bySeat("leader"));
    expect(
      await screen.findByText("No forecast check is open for this scope."),
    ).toBeVisible();
  });

  it("opens the deal behind a check, from an address that keeps the review", async () => {
    open(
      "#/analytics?overview=sales&period=this_quarter&pipeline=all",
      bySeat("leader"),
      {
        checks: () =>
          jsonResponse({
            data: [
              {
                id: "x1",
                type: "close_past",
                subject_kind: "deal",
                subject_id: "deal-x1",
                subject: {
                  type: "deal",
                  id: "deal-x1",
                  label: "Northstar rollout",
                },
                severity: "high",
                affected_minor: 500000,
                currency: "EUR",
                status: "open",
                first_seen_at: "2026-09-01T02:00:00Z",
                last_seen_at: "2026-09-14T02:00:00Z",
                inputs_changed: false,
              },
            ],
          }),
      },
    );
    const deal = await screen.findByRole("link", { name: "Northstar rollout" });
    expect(deal.getAttribute("href")).toBe("/#/deals/deal-x1");
    expect(deal.closest("p")?.textContent).toBe("Review Northstar rollout");
    // The row opens its own finding in the review, never the deal's company.
    const door = screen.getByRole("link", { name: "Answer" });
    expect(door.getAttribute("href")).toMatch(
      /^\/#\/analytics\/forecast\?.*finding=x1/,
    );
    // Back returns to this address, and the address holds the whole review.
    expect(window.location.hash).toContain("overview=sales");
    expect(window.location.hash).toContain("pipeline=all");
  });

  it("says in one line when movement history starts, with no empty panel", async () => {
    open("#/analytics", bySeat("leader"));
    const line = await screen.findByText(
      /^History starts with the next capture, .+\. Movement shows once a second one follows\.$/,
    );
    expect(line).toBeVisible();
    expect(
      screen.queryByRole("region", { name: "Since the last capture" }),
    ).toBeNull();
  });

  it("names a part of a figure the reader may not see and a history the import lacks", async () => {
    open("#/analytics", (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({
            metrics: [
              metric("bookings_won", 42_000_000, {
                coverage: { status: "partial", withheld: true },
              }),
              metric("open_pipeline", 136_000_000, {
                coverage: {
                  status: "partial",
                  withheld: false,
                  reason: "Imported deals have no stage history before March.",
                },
              }),
            ],
          }),
    );
    await readings();
    expect(
      screen.getByText("Part of this figure is hidden from you"),
    ).toBeVisible();
    // The withheld figure is never stated as the whole in words.
    expect(screen.queryByText(/Won in/)).toBeNull();
    expect(
      screen.getByText(/Imported deals have no stage history before March\./),
    ).toBeInTheDocument();
  });

  it("shows the refusal and no figures when individual performance is off", async () => {
    open("#/analytics", () =>
      jsonResponse(
        {
          title: "Individual performance is off",
          status: 403,
          code: "individual_performance_off",
        },
        403,
      ),
    );
    expect(
      await screen.findByText(/switched off individual performance/),
    ).toBeVisible();
    expect(
      screen.queryByRole("region", { name: "Opening readings" }),
    ).toBeNull();
  });
});

describe("Sales review gap", () => {
  it("shows no gap when the comparison carries none", async () => {
    const { gap: _gap, ...withoutGap } = compared;
    open("#/analytics", (url) =>
      isMovement(url)
        ? startingMovement()
        : salesEvaluation({ comparison: withoutGap }),
    );
    const strip = await readings();
    const hero = within(screen.getByRole("region", { name: "Sales won" }));
    expect(
      hero.getByText("The comparison came back without a gap."),
    ).toBeVisible();
    expect(hero.queryByText(/Projected/)).toBeNull();
    expect(strip.queryByText("€0")).toBeNull();
  });
});
