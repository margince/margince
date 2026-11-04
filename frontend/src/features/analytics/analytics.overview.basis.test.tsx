// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { forgetSeat } from "../../lib/storage";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  overviewAddress,
  type ScenarioName,
} from "./analytics.overview.scenarios.fixtures";
import {
  compared,
  isMovement,
  landingBasis,
  metric,
  openOverview,
  readings,
  salesEvaluation,
  startingMovement,
} from "./analytics.overview.testkit";
import {
  type ReportingEvaluation,
  reportingCompactAmount,
} from "./reporting.model";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});
function openScenario(name: ScenarioName) {
  const pair = OVERVIEW_SCENARIOS[name].views.sales;
  if (!isSetup(pair)) throw new Error("missing sales scenario");
  openOverview(overviewAddress("sales"), pair.answer, pair.routes);
}
function openEvaluation(evaluation: ReportingEvaluation) {
  openOverview(overviewAddress("sales"), (url) =>
    isMovement(url) ? startingMovement() : evaluation,
  );
}
async function card(label: string) {
  const strip = await readings();
  const section = strip.getByText(label).closest("section");
  if (!section) throw new Error("missing metric card");
  return within(section);
}
const money = (value: number) =>
  reportingCompactAmount(value, "EUR", "EUR", "en");

it("keeps committed business and modelled outcomes in different labelled readings", async () => {
  openScenario("modelCommitDivergence");
  const finish = await card("Won + confirmed commit");
  expect(finish.getByText("0 EUR")).toBeVisible();
  expect(finish.queryByText(/1,400,000 EUR/)).toBeNull();
  const model = await card("Modelled quarter finish");
  expect(
    model.getByText(`${money(140000000)} – ${money(260000000)}`),
  ).toBeVisible();
  expect(screen.queryByRole("region", { name: "Summary" })).toBeNull();
});

it("keeps a measured zero distinct from a missing model range", async () => {
  const evaluation = salesEvaluation();
  openEvaluation({
    ...evaluation,
    metrics: evaluation.metrics.map((m) =>
      m.id === "forecast_landing"
        ? {
            ...m,
            value: 0,
            range_low_minor: undefined,
            range_high_minor: undefined,
          }
        : m,
    ),
  });
  expect(
    (await card("Won + confirmed commit")).getByText("0 EUR"),
  ).toBeVisible();
  expect((await card("Modelled quarter finish")).getByText("—")).toBeVisible();
  expect(
    screen.queryByText("No modelled range was drawn for this period."),
  ).toBeNull();
});

it("does not invent a forecast basis for an older response", async () => {
  openScenario("legacyLanding");
  const finish = await card("Landing");
  expect(finish.getByText("760,000 EUR")).toBeVisible();
  expect(finish.queryByText("Won + confirmed commit")).toBeNull();
});

it.each([
  {
    measure: "commit_evidence",
    amount: 14000,
    label: "Won + confirmed commit",
  },
  { measure: "weighted", amount: 17000, label: "Won + weighted open deals" },
  { measure: "manager_call", amount: 19000, label: "Manager’s call" },
] as const)(
  "labels $measure without adding the manager call to won sales",
  async ({ measure, amount, label }) => {
    openEvaluation(
      salesEvaluation({
        metrics: [
          metric("bookings_won", 10000),
          metric("forecast_landing", amount, {
            forecast_basis: landingBasis(
              {
                won: 10000,
                remaining: amount - 10000,
                amount_minor: amount,
                measure,
                system_minor: 17000,
              },
              {
                call_minor: 19000,
                call_at: "2026-10-30T09:00:00Z",
                open_pipeline_minor: 11000,
              },
            ),
            target_comparison: {
              ...compared,
              basis: measure,
              target: 20000,
              won: 10000,
              landing: amount,
              gap: 20000 - amount, // ds:ignore money shortfall in minor units, not layout spacing
              remaining: 10000,
              open_pipeline: 11000,
            },
          }),
        ],
      }),
    );
    expect((await card(label)).getByText(money(amount))).toBeVisible();
    expect(screen.queryByText(money(29000))).toBeNull();
    expect(
      (await card("Pipeline due this quarter")).getByText(money(11000)),
    ).toBeVisible();
    expect(screen.getByText(`${money(10000)} still to win`)).toBeVisible();
    if (measure === "manager_call") {
      const call = await card(label);
      expect(call.getByText("Called 30 Oct 2026")).toBeVisible();
      expect(
        call.getByText(`Projected shortfall ${money(20000 - amount)}`),
      ).toBeVisible();
      expect(call.getByRole("button", { name: "Evidence" })).toBeVisible();
    }
  },
);

it("keeps current-quarter pipeline separate from explicitly labelled all-open stock", async () => {
  openScenario("coverageNumerator");
  expect(
    (await card("Pipeline due this quarter")).queryByText(money(520000000)),
  ).toBeNull();
  expect(
    screen.getByText(`All open pipeline: ${money(520000000)}`),
  ).toBeVisible();
});

it.each(["fx_missing", "partial_visibility", "colleague_withheld"] as const)(
  "does not invent a target gap for %s",
  async (status) => {
    openEvaluation(
      salesEvaluation({
        comparison: { status, target_metric: "bookings_won" },
      }),
    );
    const strip = await readings();
    expect(
      strip.queryByText(/still to win|Projected shortfall|Infinity|NaN/),
    ).toBeNull();
  },
);

it("names the whole quarter separately from the actuals cutoff", async () => {
  openScenario("cutoffVersusHorizon");
  const period = await screen.findByText("1 October – 31 December");
  expect(period.parentElement).toBe(
    screen.getByText("Actuals through 31/10/2026, 17:00").parentElement,
  );
});

it("withholds period-pipeline values and population details from a masked forecast", async () => {
  openEvaluation(
    salesEvaluation({
      metrics: [
        metric("forecast_landing", null, {
          coverage: { status: "unavailable", withheld: true },
          range_low_minor: 1000000,
          range_high_minor: 2000000,
          target_comparison: compared,
          forecast_basis: landingBasis(
            { won: 10000, remaining: 5000 },
            { open_pipeline_minor: 1234500, eligible_count: 18 },
          ),
        }),
      ],
    }),
  );
  const pipeline = await card("Pipeline due this quarter");
  expect(pipeline.getByText("—")).toBeVisible();
  expect(pipeline.queryByText(money(1234500))).toBeNull();
  expect(pipeline.queryByText("18 deals")).toBeNull();
  expect(pipeline.queryByRole("link")).toBeNull();
  expect((await card("Modelled quarter finish")).getByText("—")).toBeVisible();
  expect(screen.queryByText(/Projected shortfall/)).toBeNull();
});
