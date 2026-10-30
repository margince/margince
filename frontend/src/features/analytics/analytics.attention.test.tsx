// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import { render } from "./analytics.testkit";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

const finding = (
  id: string,
  severity: "high" | "medium" | "low" = "high",
  amount: number | null = 500000,
) => ({
  id,
  type: "close_past",
  subject_kind: "deal",
  subject_id: `deal-${id}`,
  subject: { type: "deal", id: `deal-${id}`, label: `Deal ${id}` },
  severity,
  affected_minor: amount ?? undefined,
  currency: "EUR",
  owner_id: "u-owner",
  status: "open",
  first_seen_at: "2026-09-01T02:00:00Z",
  last_seen_at: "2026-09-14T02:00:00Z",
  inputs_changed: false,
});

const readings = (priced: number, eligible: number) => ({
  period_start: "2026-07-01",
  period_end: "2026-09-30",
  scope_kind: "workspace",
  won_minor: 40000,
  evidence_minor: 120000,
  best_case_minor: 180000,
  open_minor: 250000,
  weighted_minor: 130000,
  eligible_count: eligible,
  priced_count: priced,
  confirmed_date_count: 10,
  fx_missing_count: eligible - priced,
  as_of: "2026-09-14T09:00:00Z",
  timezone: REPORTING_FIXTURE_ZONE,
  base_currency: "EUR",
});

// Performance, for a manager who may read reports, the forecast and the
// nightly check's coverage.
function routes(overrides: RouteMap = {}): RouteMap {
  const base = reportingStoryRoutes();
  return {
    ...base,
    "GET /me": meRoute({
      report_definition: ["read"],
      reporting_framework: ["read"],
      deal: ["read"],
      forecast: ["read"],
      pipeline: ["read"],
      data_coverage: ["read"],
    }),
    "GET /forecast/assurance/exceptions": () =>
      jsonResponse({ data: [finding("a"), finding("b")] }),
    "GET /forecast": () => jsonResponse(readings(10, 12)),
    "GET /analytics/coverage": () =>
      jsonResponse({
        run_id: "r1",
        as_of: "2026-09-14T02:00:00Z",
        sources: [
          { source: "mail", state: "checked" },
          { source: "offers", state: "not_connected" },
        ],
      }),
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: "u-owner", display_name: "Riley Rep" }] }),
    ...overrides,
  };
}

it("lists what stands between the reader and numbers they can trust", async () => {
  window.location.hash = "#/analytics/performance";
  installFetchStub(routes());
  render(<AnalyticsScreen />);

  expect(
    await screen.findByRole("heading", { name: "Needs your attention" }),
  ).toBeTruthy();
  expect(await screen.findByText("Deal a")).toBeTruthy();
  expect(screen.getByText("Deal b")).toBeTruthy();
  expect(screen.getAllByText("Close date has passed")).toHaveLength(2);
  expect((await screen.findAllByText(/^Riley Rep · .+ at stake$/)).length).toBe(
    2,
  );
  expect(screen.queryByText("2 forecast checks to answer")).toBeNull();
  expect(
    await screen.findByText("10 of 12 open deals are priced"),
  ).toBeTruthy();
  expect(
    await screen.findByText("1 data source was not fully checked"),
  ).toBeTruthy();
});

it("opens the section where each item is answered", async () => {
  const user = userEvent.setup({ delay: null });
  window.location.hash = "#/analytics/performance";
  installFetchStub(routes());
  render(<AnalyticsScreen />);

  await user.click(
    (await screen.findAllByRole("button", { name: "Review" }))[0],
  );
  await waitFor(() =>
    expect(
      screen
        .getByRole("button", { name: "Forecast" })
        .getAttribute("aria-pressed"),
    ).toBe("true"),
  );
});

it("keeps the list when the performance evaluation fails", async () => {
  window.location.hash = "#/analytics/performance";
  installFetchStub(
    routes({
      "GET /analytics/evaluate": () =>
        jsonResponse({ status: 500, title: "Internal Server Error" }, 500),
    }),
  );
  render(<AnalyticsScreen />);

  expect(
    await screen.findByRole("heading", { name: "Needs your attention" }),
  ).toBeTruthy();
  expect(await screen.findByText("Deal a")).toBeTruthy();
});

it("names the first five findings in the server's order and counts the rest", async () => {
  window.location.hash = "#/analytics/performance";
  installFetchStub(
    routes({
      "GET /forecast/assurance/exceptions": () =>
        jsonResponse({
          data: ["one", "two", "three", "four", "five", "six", "seven"].map(
            (id) => finding(id),
          ),
        }),
    }),
  );
  render(<AnalyticsScreen />);

  expect(await screen.findByText("7 forecast checks to answer")).toBeTruthy();
  expect(screen.getAllByText(/^Deal /).map((node) => node.textContent)).toEqual(
    ["Deal one", "Deal two", "Deal three", "Deal four", "Deal five"],
  );
});

it("asks for the findings of the record scope Performance shows", async () => {
  const asked: string[] = [];
  window.location.hash = "#/analytics/performance?scope=team:t-1";
  installFetchStub(
    routes({
      "GET /analytics/context": () =>
        jsonResponse({
          default_scope: { kind: "workspace", label: "Whole company" },
          allowed_scopes: [
            { kind: "workspace", label: "Whole company" },
            { kind: "team", id: "t-1", label: "North" },
          ],
          capabilities: {
            view_manager_forecast: true,
            submit_manager_forecast: true,
          },
          as_of: "2026-09-04T00:00:00Z",
          timezone: REPORTING_FIXTURE_ZONE,
          base_currency: "EUR",
        }),
    }),
  );
  const stub = globalThis.fetch;
  globalThis.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.includes("/forecast/assurance/exceptions")) {
      asked.push(url);
    }
    return stub(input, init);
  }) as typeof fetch;
  render(<AnalyticsScreen />);

  await screen.findByText("Deal a");
  expect(
    asked.some(
      (url) => /scope_kind=team/.test(url) && /scope_id=t-1/.test(url),
    ),
  ).toBe(true);
});

it("leaves out an owner whose name has not arrived instead of calling the deal unowned", async () => {
  window.location.hash = "#/analytics/performance";
  installFetchStub(
    routes({
      "GET /users/names": () =>
        jsonResponse({ status: 500, title: "Internal Server Error" }, 500),
    }),
  );
  render(<AnalyticsScreen />);

  expect(await screen.findByText("Deal a")).toBeTruthy();
  expect(screen.queryByText(/No owner/)).toBeNull();
  expect(screen.getAllByText(/^.+ at stake$/).length).toBe(2);
});

it("draws no panel when nothing needs the reader", async () => {
  window.location.hash = "#/analytics/performance";
  installFetchStub(
    routes({
      "GET /forecast/assurance/exceptions": () => jsonResponse({ data: [] }),
      "GET /forecast": () => jsonResponse(readings(12, 12)),
      "GET /analytics/coverage": () =>
        jsonResponse({
          run_id: "r1",
          as_of: "2026-09-14T02:00:00Z",
          sources: [{ source: "mail", state: "checked" }],
        }),
    }),
  );
  render(<AnalyticsScreen />);

  await screen.findByRole("heading", { name: "Sales won over time" });
  expect(
    screen.queryByRole("heading", { name: "Needs your attention" }),
  ).toBeNull();
});

it("asks nothing of the forecast for a reader without the forecast grant", async () => {
  const forecast = vi.fn(() => jsonResponse(readings(10, 12)));
  const exceptions = vi.fn(() => jsonResponse({ data: [finding("a")] }));
  window.location.hash = "#/analytics/performance";
  installFetchStub(
    routes({
      "GET /me": meRoute({
        report_definition: ["read"],
        reporting_framework: ["read"],
        deal: ["read"],
        pipeline: ["read"],
      }),
      "GET /forecast": forecast,
      "GET /forecast/assurance/exceptions": exceptions,
    }),
  );
  render(<AnalyticsScreen />);

  await screen.findByRole("heading", { name: "Sales won over time" });
  expect(forecast).not.toHaveBeenCalled();
  expect(exceptions).not.toHaveBeenCalled();
});

it("says when a named finding is out of date", async () => {
  window.location.hash = "#/analytics/performance";
  installFetchStub(
    routes({
      "GET /forecast/assurance/exceptions": () =>
        jsonResponse({
          data: [{ ...finding("a"), inputs_changed: true }, finding("b")],
        }),
    }),
  );
  render(<AnalyticsScreen />);

  expect(
    await screen.findByText(
      /^Riley Rep · .+ at stake · out of date — refreshes on the next run$/,
    ),
  ).toBeTruthy();
  expect(screen.getAllByText(/out of date/)).toHaveLength(1);
});
