// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatMoney } from "../format/format";
import { AnalyticsScreen } from "./analytics";
import { render, reportsStub } from "./analytics.testkit";

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
