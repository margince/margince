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
  type AskedRequest,
  bySeat,
  isMovement,
  metric,
  openOverview as open,
  peakInFlight,
  readings,
  salesEvaluation,
  startingMovement,
  teamEvaluation,
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

// Reads the app shell or a shared lookup makes on any screen, by name. Every
// other request a view makes is either on its critical path or optional.
const SHELL: readonly string[] = ["/v1/me"];

type View = Readonly<{
  critical: (url: URL) => boolean;
  optional: (url: URL) => boolean;
}>;

const path = (url: URL) => url.pathname;

const SALES: View = {
  critical: (url) =>
    [
      "/v1/analytics/context",
      "/v1/analytics/framework",
      "/v1/analytics/metrics",
      "/v1/pipelines",
    ].includes(path(url)) ||
    (path(url) === "/v1/analytics/evaluate" && !isMovement(url)),
  optional: (url) =>
    path(url) === "/v1/forecast/assurance/exceptions" ||
    (path(url) === "/v1/analytics/evaluate" && isMovement(url)),
};

const PROSPECTING: View = {
  critical: (url) =>
    [
      "/v1/analytics/context",
      "/v1/analytics/framework",
      "/v1/analytics/metrics",
      "/v1/analytics/evaluate",
      "/v1/analytics/sdr-summary",
    ].includes(path(url)),
  optional: (url) =>
    ["/v1/worklist", "/v1/analytics/sdr-summary/handoffs"].includes(path(url)),
};

const TEAM: View = {
  critical: (url) =>
    [
      "/v1/analytics/context",
      "/v1/analytics/framework",
      "/v1/analytics/metrics",
      "/v1/analytics/evaluate",
      "/v1/analytics/sdr-team",
    ].includes(path(url)),
  optional: () => false,
};

// Every request the page made, sorted into the shell, the critical path and
// the optional reads. Anything else is named, so a new read fails the census.
function census(asked: readonly AskedRequest[], view: View) {
  const critical = asked.filter(({ url }) => view.critical(url));
  const unplaced = asked.filter(
    ({ url }) =>
      !SHELL.includes(path(url)) && !view.critical(url) && !view.optional(url),
  );
  return {
    critical: critical.map(({ url }) => path(url)).sort(),
    unplaced: unplaced.map(({ url }) => `${path(url)}${url.search}`),
  };
}

describe("the request census of each opening view", () => {
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
    const headline = asked.filter(
      ({ url }) => path(url) === "/v1/analytics/evaluate" && !isMovement(url),
    );
    expect(headline[0]?.url.searchParams.get("period")).toBe("this_quarter");
    expect(headline[0]?.url.searchParams.getAll("metrics")).toContain(
      "forecast_landing",
    );
    expect(census(asked, SALES)).toEqual({
      critical: [
        "/v1/analytics/context",
        "/v1/analytics/evaluate",
        "/v1/analytics/framework",
        "/v1/analytics/metrics",
        "/v1/pipelines",
      ],
      unplaced: [],
    });
  });

  it("asks My prospecting for one evaluation of this week and no pipelines", async () => {
    const asked = open(
      "#/analytics",
      bySeat("sdr"),
      { seat: "sdr" },
      createQueryClient(),
    );
    await screen.findByRole("heading", { name: /^Monthly targets, / });
    const sent = asked.find(({ url }) => path(url) === "/v1/analytics/evaluate")
      ?.url.searchParams;
    expect(sent?.get("period")).toBe("this_week");
    // The monthly strip is read in the same evaluation, over its own month.
    expect(sent?.get("target_basis")).toBe("month");
    expect(sent?.get("pipeline_id")).toBeNull();
    expect(census(asked, PROSPECTING)).toEqual({
      critical: [
        "/v1/analytics/context",
        "/v1/analytics/evaluate",
        "/v1/analytics/framework",
        "/v1/analytics/metrics",
        "/v1/analytics/sdr-summary",
      ],
      unplaced: [],
    });
  });

  it("asks Team prospecting for one evaluation and one team review, whatever the team's size", async () => {
    const asked = open(
      "#/analytics/overview?overview=team",
      teamEvaluation,
      { template: "sdr" },
      createQueryClient(),
    );
    await screen.findByRole("table", { name: "Members" });
    expect(census(asked, TEAM)).toEqual({
      critical: [
        "/v1/analytics/context",
        "/v1/analytics/evaluate",
        "/v1/analytics/framework",
        "/v1/analytics/metrics",
        "/v1/analytics/sdr-team",
      ],
      unplaced: [],
    });
  });

  it.each([
    ["Sales review", "leader", SALES],
    ["My prospecting", "sdr", PROSPECTING],
  ] as const)(
    "keeps at most two optional reads of %s in flight at once",
    async (_name, seat, view) => {
      // No optional read is ever answered, so each stays in flight.
      const asked = open(
        "#/analytics",
        bySeat(seat),
        { seat },
        createQueryClient(),
        view.optional,
      );
      await readings();
      await waitFor(() =>
        expect(
          asked.filter(({ url }) => view.optional(url)).length,
        ).toBeGreaterThan(0),
      );
      expect(census(asked, view).unplaced).toEqual([]);
      expect(peakInFlight(asked, view.optional)).toBeLessThanOrEqual(2);
    },
  );

  it("starts the capture comparison only after the checks have answered", async () => {
    const asked = open("#/analytics", bySeat("leader"), {
      checks: () => new Promise<Response>(() => {}),
    });
    await readings();
    expect(asked.some(({ url }) => isMovement(url))).toBe(false);
  });

  it("aborts the checks of a scope the reader has left", async () => {
    const user = userEvent.setup({ delay: null });
    const asked = open(
      "#/analytics",
      bySeat("leader"),
      {},
      undefined,
      (url) => path(url) === "/v1/forecast/assurance/exceptions",
    );
    const picker = await screen.findByRole("combobox", {
      name: "Record scope",
    });
    await user.click(picker);
    await user.click(await screen.findByRole("option", { name: "Riley Rep" }));
    await waitFor(() =>
      expect(
        asked.filter(
          ({ url }) => path(url) === "/v1/forecast/assurance/exceptions",
        ),
      ).toHaveLength(2),
    );
    const [team, mine] = asked.filter(
      ({ url }) => path(url) === "/v1/forecast/assurance/exceptions",
    );
    expect(team?.url.searchParams.get("scope_kind")).toBe("team");
    expect(team?.signal?.aborted).toBe(true);
    expect(mine?.url.searchParams.get("scope_kind")).toBe("owner");
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
