// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatMoney } from "../../format/format";
import { AnalyticsScreen } from "./analytics";
import { ownLensContext, render, reportsStub } from "./analytics.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

async function openPipeline() {
  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "Pipeline" }));
}

it("totals the whole pipeline above its stages, priced shortfall included", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      stageRows: [
        {
          stage_id: "pl-s1",
          raw_minor: 100_000,
          weighted_minor: 20_000,
          deal_count: 2,
          priced_deals: 2,
        },
        {
          stage_id: "pl-s9",
          raw_minor: 50_000,
          weighted_minor: 10_000,
          deal_count: 3,
          priced_deals: 2,
        },
      ],
    }),
  );
  render(<AnalyticsScreen />);
  await openPipeline();

  expect(
    await screen.findByText(formatMoney(150_000, "EUR", "en")),
  ).toBeTruthy();
  expect(screen.getByText(formatMoney(30_000, "EUR", "en"))).toBeTruthy();
  expect(screen.getByText("5")).toBeTruthy();
  // Under both money totals: the weighted one is short the same deals.
  expect(
    screen.getAllByText((text) => text.includes("4 of 5 priced")),
  ).toHaveLength(2);
});

it("states no pipeline-wide shortfall when a stage left its priced count out", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      stageRows: [
        {
          stage_id: "pl-s1",
          raw_minor: 100_000,
          weighted_minor: 20_000,
          deal_count: 2,
          priced_deals: 1,
        },
        {
          stage_id: "pl-s9",
          raw_minor: 50_000,
          weighted_minor: 10_000,
          deal_count: 3,
        },
      ],
    }),
  );
  render(<AnalyticsScreen />);
  await openPipeline();

  expect(
    await screen.findByText(formatMoney(150_000, "EUR", "en")),
  ).toBeTruthy();
  expect(screen.queryByText((text) => text.includes("of 5 priced"))).toBeNull();
});

it("draws no pipeline total when an unpriced stage's shortfall cannot be stated", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      stageRows: [
        {
          stage_id: "pl-s1",
          raw_minor: 100_000,
          weighted_minor: 20_000,
          deal_count: 2,
        },
        {
          stage_id: "pl-s9",
          raw_minor: null,
          weighted_minor: null,
          deal_count: 3,
        },
      ],
    }),
  );
  render(<AnalyticsScreen />);
  await openPipeline();

  // The priced stage's own row, and no total that quietly equals it.
  expect(
    await screen.findAllByText(formatMoney(100_000, "EUR", "en")),
  ).toHaveLength(1);
  expect(screen.getAllByText(formatMoney(20_000, "EUR", "en"))).toHaveLength(1);
});

it("asks every pipeline card for the picked record scope, and leaves delivery company-wide", async () => {
  const bodies: { key: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    "fetch",
    reportsStub({
      onRun: (key, body) => bodies.push({ key, body }),
      context: {
        ...ownLensContext,
        default_scope: { kind: "workspace", label: "Whole company" },
        allowed_scopes: [
          { kind: "workspace", label: "Whole company" },
          { kind: "team", id: "t-north", label: "North" },
        ],
      },
    }),
  );
  window.location.hash = "#/analytics/pipeline?scope=team:t-north";
  render(<AnalyticsScreen />);

  expect(
    await screen.findByRole("combobox", { name: "Record scope" }),
  ).toBeTruthy();
  const pipelineReports = [
    "pipeline-current",
    "forecast",
    "open-deals-per-company",
  ];
  await vi.waitFor(() =>
    expect(
      pipelineReports.map(
        (report) => bodies.find((sent) => sent.key === report)?.body.scope,
      ),
    ).toEqual(pipelineReports.map(() => ({ kind: "team", id: "t-north" }))),
  );

  await userEvent
    .setup()
    .click(await screen.findByRole("button", { name: "Delivery" }));
  await vi.waitFor(() =>
    expect(bodies.some((sent) => sent.key === "projects-by-phase")).toBe(true),
  );
  expect(
    bodies
      .filter((sent) => sent.key.startsWith("project"))
      .map((sent) => sent.body.scope),
  ).toEqual([undefined, undefined, undefined]);
});

it("keeps the pipeline cards for a deal reader whose context read is refused", async () => {
  const bodies: { key: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    "fetch",
    reportsStub({
      onRun: (key, body) => bodies.push({ key, body }),
      contextStatus: 403,
    }),
  );
  window.location.hash = "#/analytics/pipeline";
  render(<AnalyticsScreen />);

  await vi.waitFor(() =>
    expect(bodies.some((sent) => sent.key === "pipeline-current")).toBe(true),
  );
  expect(
    bodies.find((sent) => sent.key === "pipeline-current")?.body.scope,
  ).toBeUndefined();
  expect(screen.queryByRole("combobox", { name: "Record scope" })).toBeNull();
});

it("shows a failed context read instead of totals for a scope the link did not ask for", async () => {
  const bodies: { key: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    "fetch",
    reportsStub({
      onRun: (key, body) => bodies.push({ key, body }),
      contextStatus: 500,
    }),
  );
  window.location.hash = "#/analytics/pipeline?scope=team:t-north";
  render(<AnalyticsScreen />);

  expect(await screen.findByRole("alert", {}, { timeout: 5000 })).toBeTruthy();
  expect(bodies.some((sent) => sent.key === "pipeline-current")).toBe(false);
});

it("lists open pipeline by close month in calendar order, undated deals last", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      closeMonthRows: [
        {
          close_month: null,
          raw_minor: 9_000,
          weighted_minor: 900,
          deal_count: 1,
        },
        {
          close_month: "2026-12",
          raw_minor: 50_000,
          weighted_minor: 10_000,
          deal_count: 1,
        },
        {
          close_month: "2026-11",
          raw_minor: 70_000,
          weighted_minor: 14_000,
          deal_count: 2,
        },
      ],
    }),
  );
  render(<AnalyticsScreen />);
  await openPipeline();

  const table = await screen.findByRole("table", {
    name: "Open pipeline by close month",
  });
  const months = within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell")[0]?.textContent);
  expect(months).toEqual(["November 2026", "December 2026", "No close date"]);
});
