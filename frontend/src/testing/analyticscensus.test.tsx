/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// How many reads each Analytics opening view makes, under the app's own query
// client. Here rather than beside the screen, because a feature test may not
// import the shell's client.

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { createQueryClient } from "../app/queryclient";
import {
  bySeat,
  isMovement,
  metric,
  openOverview as open,
  readings,
  salesEvaluation,
  startingMovement,
} from "../features/analytics/analytics.overview.testkit";
import { removeStored, STORAGE_KEYS } from "../lib/storage";
import { jsonResponse } from "./story-utils";

const originalFetch = globalThis.fetch;
beforeEach(() => removeStored(STORAGE_KEYS.analyticsOverviewView));
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

describe("the request census of each opening view", () => {
  const analyticsReads = (asked: { url: URL }[]) =>
    asked.filter(({ url }) => url.pathname.includes("/analytics/"));

  it("asks Sales review for one headline evaluation of this fiscal quarter", async () => {
    const asked = open(
      "#/analytics",
      bySeat("leader"),
      {},
      createQueryClient(),
    );
    await screen.findByText(
      "Comparison starts after a second compatible capture.",
    );
    const evaluations = asked.filter(({ url }) =>
      url.pathname.endsWith("/analytics/evaluate"),
    );
    const headline = evaluations.filter(({ url }) => !isMovement(url));
    expect(headline).toHaveLength(1);
    expect(headline[0]?.url.searchParams.get("period")).toBe("this_quarter");
    expect(headline[0]?.url.searchParams.getAll("metrics")).toContain(
      "forecast_landing",
    );
    // The movement read is the optional one, behind the checks.
    const critical = analyticsReads(asked).filter(
      ({ url }) => !isMovement(url),
    );
    expect(critical.map(({ url }) => url.pathname).sort()).toEqual([
      "/v1/analytics/context",
      "/v1/analytics/evaluate",
      "/v1/analytics/framework",
      "/v1/analytics/metrics",
    ]);
  });

  it("asks My prospecting for one evaluation of this week and no pipelines", async () => {
    const asked = open(
      "#/analytics",
      bySeat("sdr"),
      { seat: "sdr" },
      createQueryClient(),
    );
    await screen.findByRole("heading", { name: /^Monthly targets, / });
    const evaluations = asked.filter(({ url }) =>
      url.pathname.endsWith("/analytics/evaluate"),
    );
    expect(evaluations).toHaveLength(1);
    const sent = evaluations[0]?.url.searchParams;
    expect(sent?.get("period")).toBe("this_week");
    // The monthly strip is read in the same evaluation, over its own month.
    expect(sent?.get("target_basis")).toBe("month");
    expect(sent?.get("pipeline_id")).toBeNull();
    expect(asked.some(({ url }) => url.pathname.endsWith("/pipelines"))).toBe(
      false,
    );
    // The waiting rows are the action list's own, optional read.
    const critical = analyticsReads(asked).filter(
      ({ url }) => !url.pathname.endsWith("/handoffs"),
    );
    expect(critical.map(({ url }) => url.pathname).sort()).toEqual([
      "/v1/analytics/context",
      "/v1/analytics/evaluate",
      "/v1/analytics/framework",
      "/v1/analytics/metrics",
      "/v1/analytics/sdr-summary",
    ]);
  });

  it("keeps at most two optional reads in flight while the checks wait", async () => {
    const asked = open("#/analytics", bySeat("leader"), {
      checks: () => new Promise<Response>(() => {}),
    });
    await readings();
    // The checks never answer, so the movement read waits behind them.
    expect(asked.some(({ url }) => isMovement(url))).toBe(false);
    const optional = asked.filter(({ url }) =>
      ["/forecast", "/analytics/coverage"].some((path) =>
        url.pathname.includes(path),
      ),
    );
    expect(optional.length).toBeLessThanOrEqual(2);
  });

  it("aborts an obsolete period and never draws it under the new one", async () => {
    const user = userEvent.setup({ delay: null });
    let releaseQuarter: (value: Response) => void = () => {};
    const asked = open("#/analytics", (url) => {
      if (isMovement(url)) return startingMovement();
      if (url.searchParams.get("period") === "this_quarter")
        return new Promise<Response>((resolve) => {
          releaseQuarter = resolve;
        });
      return salesEvaluation({
        metrics: [metric("bookings_won", 12_300_000)],
      });
    });
    const period = await screen.findByRole("combobox", { name: "Date range" });
    await user.click(period);
    await user.click(await screen.findByRole("option", { name: "This month" }));
    expect(await screen.findByText("€123k")).toBeVisible();
    const quarter = asked.find(
      ({ url }) => url.searchParams.get("period") === "this_quarter",
    );
    expect(quarter?.signal?.aborted).toBe(true);
    releaseQuarter(jsonResponse(salesEvaluation()));
    await waitFor(() => expect(screen.queryByText("€420k")).toBeNull());
  });
});
