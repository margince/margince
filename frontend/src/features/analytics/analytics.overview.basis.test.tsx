/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { formatDecimal } from "../../format/format";
import { removeStored, STORAGE_KEYS } from "../../lib/storage";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  overviewAddress,
  type ScenarioName,
} from "./analytics.overview.scenarios";
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

// What each Sales review headline counts. The landing has its basis beside
// it, and the modelled finish is its own reading. Coverage names both
// operands, and the period stands apart from its cutoff.

const originalFetch = globalThis.fetch;
beforeEach(() => removeStored(STORAGE_KEYS.analyticsOverviewView));
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

function openScenario(name: ScenarioName): ReportingEvaluation {
  const pair = OVERVIEW_SCENARIOS[name].views.sales;
  if (!isSetup(pair)) throw new Error(`${name} excludes Sales review`);
  openOverview(overviewAddress("sales"), pair.answer, pair.routes);
  return pair.answer(new URL("http://localhost/evaluate"));
}

function openEvaluation(evaluation: ReportingEvaluation) {
  openOverview(overviewAddress("sales"), (url) =>
    isMovement(url) ? startingMovement() : evaluation,
  );
}

// The card a reading's label heads, so a figure and its basis are read
// together and never from a neighbour.
async function card(label: string) {
  const strip = await readings();
  const section = strip.getByText(label).closest("section");
  if (!section) throw new Error(`${label} is not drawn as a card`);
  return within(section);
}

const money = (minor: number | undefined) =>
  reportingCompactAmount(minor, "EUR", "EUR", "en");

describe("the expected finish beside the modelled finish", () => {
  it("names committed business, the model and the manager's call apart", async () => {
    openScenario("modelCommitDivergence");
    const finish = await card("Expected finish");
    expect(finish.getByText("€0")).toBeVisible();
    expect(finish.getByText("Won + confirmed commit")).toBeVisible();
    expect(
      finish.getByText(`Manager’s call ${money(190_000_000)}, not added`),
    ).toBeVisible();
    const model = await card("Modelled quarter finish");
    expect(
      model.getByText(`${money(140_000_000)} to ${money(260_000_000)}`),
    ).toBeVisible();
    expect(
      model.getByText("Central 80% of modelled outcomes under current chances"),
    ).toBeVisible();
    expect(model.getByText("1 unpriced deal left out")).toBeVisible();
  });

  it("never draws the range inside the landing's card", async () => {
    openScenario("modelCommitDivergence");
    const finish = await card("Expected finish");
    expect(finish.queryByText(/ to €/)).toBeNull();
    expect(finish.queryByText(/80%|8 in 10/)).toBeNull();
  });

  it("keeps a true zero landing at zero with its basis", async () => {
    openScenario("trueZero");
    const finish = await card("Expected finish");
    expect(finish.getByText("€0")).toBeVisible();
    expect(finish.getByText("Won + confirmed commit")).toBeVisible();
    expect(finish.getByText("€0 won + €0 to come")).toBeVisible();
  });

  it("says when no range was drawn", async () => {
    const evaluation = salesEvaluation();
    openEvaluation({
      ...evaluation,
      metrics: evaluation.metrics.map(
        ({ range_low_minor: _low, range_high_minor: _high, ...each }) => each,
      ),
    });
    const model = await card("Modelled quarter finish");
    expect(model.getByText("No range")).toBeVisible();
    expect(
      model.getByText("No modelled range was drawn for this period."),
    ).toBeVisible();
  });

  it("says the basis is unknown on an answer stored before it was kept", async () => {
    openScenario("legacyLanding");
    const finish = await card("Expected finish");
    expect(finish.getByText("€760k")).toBeVisible();
    expect(finish.getByText("Basis not recorded")).toBeVisible();
    expect(finish.queryByText("Won + confirmed commit")).toBeNull();
  });
});

// 100 won, 40 confirmed commit, 70 weighted open and a call of 190 for the
// whole quarter. Each measure lands on its own figure; none reads 290.
describe("one quarter landed three ways", () => {
  const cases = [
    {
      measure: "commit_evidence",
      amount: 14_000,
      basis: "Won + confirmed commit",
    },
    { measure: "weighted", amount: 17_000, basis: "Won + weighted open deals" },
    { measure: "manager_call", amount: 19_000, basis: "Manager’s call" },
  ] as const;

  function landedBy(
    measure: (typeof cases)[number]["measure"],
    amount: number,
    target: boolean,
  ): ReportingEvaluation {
    const call = measure === "manager_call";
    return salesEvaluation({
      metrics: [
        metric("bookings_won", 10_000),
        metric("open_pipeline", 25_000),
        metric("forecast_landing", amount, {
          forecast_basis: landingBasis(
            {
              won: 10_000,
              remaining: call ? 0 : amount - 10_000,
              measure,
              amount_minor: amount,
              ...(call ? { system_minor: 14_000 } : {}),
            },
            { call_minor: 19_000 },
          ),
          target_comparison: target
            ? {
                ...compared,
                basis: measure,
                target: 20_000,
                landing: amount,
                won: 10_000,
                gap: 20_000 - amount, // ds:ignore the target comparison money gap, not a CSS gap
                remaining: 10_000,
                open_pipeline: 11_000,
                coverage: 1.1,
              }
            : {
                status: "no_target",
                target_metric: "bookings_won",
                basis: measure,
              },
        }),
      ],
    });
  }

  it.each(
    cases.flatMap((each) => [
      { ...each, target: true },
      { ...each, target: false },
    ]),
  )(
    "$measure lands on its own figure (target: $target)",
    async ({ measure, amount, basis, target }) => {
      openEvaluation(landedBy(measure, amount, target));
      const finish = await card("Expected finish");
      expect(finish.getByText(money(amount))).toBeVisible();
      expect(finish.getByText(basis)).toBeVisible();
      expect(
        finish.getByText(
          measure === "manager_call"
            ? `Records alone project ${money(14_000)}`
            : `Manager’s call ${money(19_000)}, not added`,
        ),
      ).toBeVisible();
      expect(screen.queryByText(money(29_000))).toBeNull();
      if (!target) return;
      // Left to win stays target minus won; the shortfall follows the landing.
      const shortfall = await card("Projected shortfall");
      expect(shortfall.getByText(money(20_000 - amount))).toBeVisible();
      expect(
        shortfall.getByText(
          `Target ${money(20_000)} − expected finish ${money(amount)}`,
        ),
      ).toBeVisible();
      const coverage = await card("Coverage");
      expect(
        coverage.getByText(
          `${money(11_000)} eligible to close this quarter / ${money(10_000)} still to win`,
        ),
      ).toBeVisible();
    },
  );
});

describe("coverage against what is still to win", () => {
  it("states the ratio the server divided, with both its operands", async () => {
    const evaluation = openScenario("coverageNumerator");
    const comparison = evaluation.metrics.find(
      (each) => each.id === "forecast_landing",
    )?.target_comparison;
    const eligible = comparison?.open_pipeline ?? Number.NaN;
    const remaining = comparison?.remaining ?? Number.NaN;
    const coverage = await card("Coverage");
    expect(
      coverage.getByText(`${formatDecimal(eligible / remaining, "en", 1)}×`),
    ).toBeVisible();
    expect(
      coverage.getByText(
        `${money(eligible)} eligible to close this quarter / ${money(remaining)} still to win`,
      ),
    ).toBeVisible();
    const open = await card("All open pipeline");
    expect(open.getByText(money(520_000_000))).toBeVisible();
    expect((await readings()).queryByText("Open pipeline")).toBeNull();
  });

  it("reads a target already passed as reached and its surplus, never as an endless ratio", async () => {
    openScenario("targetReached");
    expect((await card("Coverage")).getByText("Target reached")).toBeVisible();
    const surplus = await card("Projected surplus");
    expect(surplus.getByText(money(16_000_000))).toBeVisible();
    expect(screen.queryByText(/Infinity|NaN/)).toBeNull();
  });

  it.each([
    { status: "fx_missing" },
    { status: "partial_visibility" },
    { status: "colleague_withheld" },
  ] as const)(
    "compares nothing when the comparison is $status",
    async ({ status }) => {
      openEvaluation(
        salesEvaluation({
          comparison: { status, target_metric: "bookings_won" },
        }),
      );
      const strip = await readings();
      expect(strip.getByText("Not compared")).toBeVisible();
      expect(strip.queryByText("Coverage")).toBeNull();
      expect(strip.queryByText(/Infinity|NaN|×/)).toBeNull();
    },
  );
});

describe("the period apart from its cutoff", () => {
  it("names the whole quarter and the cutoff as two facts", async () => {
    openScenario("cutoffVersusHorizon");
    const period = await screen.findByText("1 October – 31 December");
    const through = screen.getByText("Actuals through 31/10/2026, 17:00");
    expect(period.parentElement).toBe(through.parentElement);
    expect(period).not.toBe(through);
  });

  it("names only the cutoff on an answer stored before the period was kept", async () => {
    openScenario("legacyLanding");
    expect(
      await screen.findByText("Actuals through 31/10/2026, 17:00"),
    ).toBeVisible();
    expect(screen.queryByText("1 October – 31 December")).toBeNull();
  });
});
