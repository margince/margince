/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { AnalyticsScreen } from "./analytics";
import { render } from "./analytics.testkit";
import { ReportingForecastGraphs } from "./reporting.forecast";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
  reportingStoryScope,
} from "./reporting.story-fixtures";
import { installFetchStub, jsonResponse } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

it.each([
  "#/analytics",
  "#/analytics/performance",
  "#/analytics/win-loss",
  "#/analytics/stage-age",
])(
  "%s opens reporting without a deployment availability flag",
  async (address) => {
    window.location.hash = address;
    installFetchStub(reportingStoryRoutes());
    render(<AnalyticsScreen />);
    expect(
      await screen.findByRole("heading", { name: "Sales won over time" }),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: "Saved reports" })).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Reporting settings" }),
    ).toBeTruthy();
    expect(screen.queryByText("Won and lost")).toBeNull();
    expect(screen.queryByText("Time in stage")).toBeNull();
  },
);

it("opens the saved report library with the same reporting implementation", async () => {
  const user = userEvent.setup({ delay: null });
  window.location.hash = "#/analytics";
  installFetchStub(reportingStoryRoutes());
  render(<AnalyticsScreen />);
  await user.click(
    await screen.findByRole("button", { name: "Saved reports" }),
  );
  expect(
    await screen.findByRole("button", { name: "Create from Performance" }),
  ).toBeTruthy();
});

it.each([
  "#/analytics",
  "#/analytics/performance",
  "#/analytics/reports",
  "#/analytics/targets",
  "#/analytics/definitions",
])("%s keeps a custom role on its authorized forecast", async (address) => {
  const forbidden = vi.fn(() =>
    jsonResponse({ status: 403, title: "Forbidden" }, 403),
  );
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: {
            deal: ["read"],
            forecast: ["read", "create"],
            pipeline: ["read"],
          },
          settingsAvailability: { reporting: false },
        }),
      ),
    "GET /analytics/metrics": forbidden,
    "GET /analytics/framework": forbidden,
    "GET /analytics/evaluate": forbidden,
    "GET /forecast": () =>
      jsonResponse({
        period_start: "2026-07-01",
        period_end: "2026-09-30",
        scope_kind: "workspace",
        won_minor: 40000,
        evidence_minor: 120000,
        best_case_minor: 180000,
        open_minor: 250000,
        weighted_minor: 130000,
        eligible_count: 12,
        priced_count: 12,
        confirmed_date_count: 10,
        fx_missing_count: 0,
        as_of: "2026-09-14T09:00:00Z",
        timezone: reportingStoryEvaluation.context.timezone,
        base_currency: "EUR",
      }),
    "GET /forecast/assurance": () => jsonResponse({}, 404),
    "GET /forecast/assurance/preview": () =>
      jsonResponse({
        started: false,
        eligible_deals: 12,
        findings: [],
        readiness: "ready",
        sources: [],
      }),
  });
  window.location.hash = address;
  render(<AnalyticsScreen />);
  expect(
    await screen.findByRole("button", { name: "Share view" }),
  ).toBeTruthy();
  expect(
    screen
      .getByRole("button", { name: "Forecast" })
      .getAttribute("aria-pressed"),
  ).toBe("true");
  expect(screen.queryByRole("button", { name: "Performance" })).toBeNull();
  expect(screen.queryByRole("button", { name: "Saved reports" })).toBeNull();
  expect(
    screen.queryByRole("button", { name: "Reporting settings" }),
  ).toBeNull();
  expect(forbidden).not.toHaveBeenCalled();
});

it("shares a live forecast before any capture exists", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: { forecast: ["read", "create"], report_definition: ["read"] },
        }),
      ),
  });
  render(<ReportingForecastGraphs scope={reportingStoryScope} />);
  await user.click(await screen.findByRole("button", { name: "Share view" }));
  expect(
    screen.getByRole("radio", { name: /Snapshot/ }).hasAttribute("disabled"),
  ).toBe(true);
});
