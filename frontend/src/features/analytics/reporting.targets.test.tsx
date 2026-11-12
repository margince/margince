// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { render } from "./analytics.testkit";
import { goalRoster } from "./progress.fixtures";
import { reportingTargets } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";
import { ReportingTargets } from "./reporting.targets";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("ReportingTargets", () => {
  it("reads a team member's published sales goals as one row that opens the plan", async () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/targets": () => jsonResponse({ data: reportingTargets }),
      "GET /analytics/sales-goals": () => jsonResponse(goalRoster),
    });
    const stubbed = globalThis.fetch;
    const asked: string[] = [];
    vi.stubGlobal("fetch", (input: RequestInfo | URL, init?: RequestInit) => {
      asked.push(input instanceof Request ? input.url : String(input));
      return stubbed(input, init);
    });
    render(<ReportingTargets />);
    const row = (await screen.findByText("Sam Okafor")).closest("tr");
    if (!(row instanceof HTMLElement)) throw new Error("no row for the goals");
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
});
