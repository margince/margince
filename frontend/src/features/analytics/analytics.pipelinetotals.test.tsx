// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatAnalyticsMoney } from "../../format/format";
import { en as englishCopy } from "../../i18n/en";
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

it("totals the whole pipeline above its stages and says which deals it leaves out", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      stageRows: [
        {
          stage_id: "pl-s1",
          raw_minor: 100_000,
          weighted_minor: 20_000,
          deal_count: 2,
          coverage: {
            aggregates: ["raw_minor"],
            partial: false,
            no_value_deals: 0,
            no_rate_deals: 0,
            missing_currencies: [],
          },
        },
        {
          stage_id: "pl-s9",
          raw_minor: 50_000,
          weighted_minor: 10_000,
          deal_count: 3,
          coverage: {
            aggregates: ["raw_minor"],
            partial: true,
            no_value_deals: 1,
            no_rate_deals: 1,
            missing_currencies: ["JPY"],
          },
        },
      ],
    }),
  );
  render(<AnalyticsScreen />);
  await openPipeline();

  // Both header figures carry the Partial mark, and so does the stage whose
  // sums leave deals out, in its raw and its weighted cell.
  expect(
    await screen.findByText(
      `${formatAnalyticsMoney(150_000, "EUR", "en")} Partial`,
    ),
  ).toBeTruthy();
  expect(
    screen.getByText(`${formatAnalyticsMoney(30_000, "EUR", "en")} Partial`),
  ).toBeTruthy();
  expect(screen.getAllByText("Partial")).toHaveLength(2);
  expect(screen.getByText("5")).toBeTruthy();
  // Under both money totals: the weighted one is short the same deals.
  expect(screen.getAllByText("1 deal has no exchange rate (JPY)")).toHaveLength(
    3,
  );
  expect(screen.getAllByText(/1 deal has no value/)).toHaveLength(3);
});

it("withholds the pipeline totals when a stage with deals states no coverage", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      stageRows: [
        {
          stage_id: "pl-s1",
          raw_minor: 100_000,
          weighted_minor: 20_000,
          deal_count: 2,
          coverage: {
            aggregates: ["raw_minor"],
            partial: true,
            no_value_deals: 0,
            no_rate_deals: 1,
            missing_currencies: [],
          },
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

  await screen.findByText("Qualify");
  expect(
    screen.queryByText((text) =>
      text.startsWith(formatAnalyticsMoney(150_000, "EUR", "en")),
    ),
  ).toBeNull();
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
    await screen.findAllByText(formatAnalyticsMoney(100_000, "EUR", "en")),
  ).toHaveLength(1);
  expect(
    screen.getAllByText(formatAnalyticsMoney(20_000, "EUR", "en")),
  ).toHaveLength(1);
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
    await screen.findByRole("combobox", {
      name: englishCopy["analytics.scopeLabel"],
    }),
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
  // Two cards ask pipeline-current: by stage and by close month. Both follow
  // the picker.
  const byMonth = bodies.find(
    (sent) =>
      sent.key === "pipeline-current" &&
      Array.isArray(sent.body.group_by) &&
      sent.body.group_by.includes("close_month"),
  );
  expect(byMonth?.body.scope).toEqual({ kind: "team", id: "t-north" });
  // A stage's count opens the deals of the picked team, not every deal.
  const hrefs = (await screen.findAllByRole("link")).map((link) =>
    link.getAttribute("href"),
  );
  expect(
    hrefs.some(
      (href) =>
        href?.includes("stage_id=") && href.includes("owner_team_id=t-north"),
    ),
  ).toBe(true);
  expect(byMonth?.body.aggregates).toContainEqual({
    fn: "sum",
    field: "amount_base_minor",
    as: "raw_minor",
  });

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
  expect(
    screen.queryByRole("combobox", {
      name: englishCopy["analytics.scopeLabel"],
    }),
  ).toBeNull();
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
    name: englishCopy["analytics.reportPipelineByCloseMonth"],
  });
  const months = within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => within(row).getAllByRole("cell")[0]?.textContent);
  expect(months).toEqual([
    "November 2026",
    "December 2026",
    englishCopy["analytics.noCloseDate"],
  ]);
});

it("says when a month's total leaves out deals, and why", async () => {
  vi.stubGlobal(
    "fetch",
    reportsStub({
      closeMonthRows: [
        {
          close_month: "2026-11",
          raw_minor: 70_000,
          weighted_minor: 14_000,
          deal_count: 2,
          coverage: {
            aggregates: ["raw_minor"],
            partial: true,
            no_value_deals: 0,
            no_rate_deals: 1,
            missing_currencies: ["SEK"],
          },
        },
      ],
    }),
  );
  render(<AnalyticsScreen />);
  await openPipeline();

  const table = await screen.findByRole("table", {
    name: englishCopy["analytics.reportPipelineByCloseMonth"],
  });
  expect(
    within(table).getByText("1 deal has no exchange rate (SEK)"),
  ).toBeTruthy();
  // The raw and the weighted figure of the month both carry the mark.
  expect(within(table).getAllByText("Partial")).toHaveLength(2);
});
