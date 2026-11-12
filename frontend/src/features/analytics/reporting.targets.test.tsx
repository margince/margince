// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { render } from "./analytics.testkit";
import {
  answerGoalRosters,
  goalRoster,
  nextGoalRoster,
} from "./progress.fixtures";
import { reportingTargets } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("ReportingTargets", () => {
  it("reads a team member's published sales goals as one row a year that opens the plan", async () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/targets": () => jsonResponse({ data: reportingTargets }),
    });
    answerGoalRosters(goalRoster, nextGoalRoster);
    const stubbed = globalThis.fetch;
    const asked: string[] = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      asked.push(input instanceof Request ? input.url : String(input));
      return stubbed(input, init);
    });
    render(<ReportingTargets />);
    const [thisYear, nextYear] = await screen.findAllByText("Sam Okafor");
    const row = thisYear?.closest("tr");
    if (!(row instanceof HTMLElement)) throw new Error("no row for the goals");
    // With no month picked, the plan already published for next year is listed too.
    expect(
      within(nextYear?.closest("tr") ?? row).getByText(
        "Sales goals 2027 · 12 months",
      ),
    ).toBeTruthy();
    expect(within(row).getByText("€1,140,000")).toBeTruthy();
    expect(within(row).getByText("Sales goals 2026 · 12 months")).toBeTruthy();
    await userEvent
      .setup()
      .click(within(row).getByRole("button", { name: "Open sales goals" }));
    expect(window.location.hash).toContain(
      `seller=${goalRoster.data[1]?.owner_id}`,
    );
    // A drafted or empty plan is no target, and a plan's months are not listed.
    expect(screen.queryByText("Maya Lindqvist")).toBeNull();
    expect(asked.find((url) => url.includes("/analytics/targets"))).toContain(
      "plan_owned=false",
    );
  });

  it("says the goals could not be read instead of calling the list empty", async () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/targets": () => jsonResponse({ data: [] }),
      "GET /analytics/sales-goals": () =>
        jsonResponse({ code: "internal", message: "no" }, 500),
    });
    render(<ReportingTargets />);
    expect(await screen.findByRole("alert")).toBeTruthy();
    expect(screen.queryByText("Nothing here yet.")).toBeNull();
  });

  it("lists no published goals among the retired targets", async () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/targets": () => jsonResponse({ data: reportingTargets }),
    });
    answerGoalRosters(goalRoster, nextGoalRoster);
    const user = userEvent.setup();
    render(<ReportingTargets />);
    await screen.findAllByText("Sam Okafor");
    await user.click(screen.getByRole("button", { name: "Target retired" }));
    await waitFor(() => expect(screen.queryByText("Sam Okafor")).toBeNull());
  });
});
