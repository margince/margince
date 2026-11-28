// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Panel } from "../../design-system/panel";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { monthTotals, SpendByTask, spendRows } from "./aiusage-spend";

afterEach(cleanup);

const line = (tier: string, cost?: number, unpriced = 0) => ({
  task: "triage",
  tier,
  calls: 2,
  tokens_in: 10,
  tokens_out: 1,
  cost_est_minor: cost,
  unpriced_calls: unpriced,
});

function show(lines: ReturnType<typeof line>[]) {
  render(
    <LocaleProvider initial="en">
      <Panel title="Spend">
        <SpendByTask rows={spendRows(lines)} showCost currency="EUR" />
      </Panel>
    </LocaleProvider>,
  );
}

it("marks a task's cost as partly priced when one of its tiers had no rate", () => {
  const [total] = spendRows([line("cheap_cloud", 120), line("premium")]);
  expect(total.cost).toBe(120);
  expect(total.partlyPriced).toBe(true);
  show([line("cheap_cloud", 120), line("premium")]);
  const row = screen.getByTestId("spend-triage");
  expect(
    within(row).getByText(englishCopy["aiusage.partlyPriced"]),
  ).toBeTruthy();
  expect(
    within(screen.getByTestId("spend-triage-premium")).getByText("—"),
  ).toBeTruthy();
});

it("marks a priced line that also carried calls with no rate", () => {
  show([line("premium", 300, 1)]);
  expect(
    within(screen.getByTestId("spend-triage-premium")).getByText(
      englishCopy["aiusage.partlyPriced"],
    ),
  ).toBeTruthy();
});

it("reads a task whose every call was priced as its plain total", () => {
  const [total] = spendRows([line("cheap_cloud", 120), line("premium", 80)]);
  expect(total.cost).toBe(200);
  expect(total.partlyPriced).toBe(false);
  show([line("cheap_cloud", 120), line("premium", 80)]);
  expect(screen.queryByText(englishCopy["aiusage.partlyPriced"])).toBeNull();
});

it("sums the month over every task and tier, and counts the calls with no rate", () => {
  expect(
    monthTotals([
      { ...line("cheap_cloud", 120, 1), cached_hits: 2 },
      { ...line("premium"), task: "enrich", calls: 3, unpriced_calls: 3 },
      line("premium", 80),
    ]),
  ).toEqual({
    calls: 7,
    cached: 2,
    tokensIn: 30,
    tokensOut: 3,
    cost: 200,
    costTooLarge: false,
    partlyPriced: true,
    unpricedCalls: 4,
  });
});

it("leaves the month without a cost when nothing was priced", () => {
  expect(monthTotals([line("premium")])).toEqual({
    calls: 2,
    cached: 0,
    tokensIn: 10,
    tokensOut: 1,
    cost: undefined,
    costTooLarge: false,
    partlyPriced: false,
    unpricedCalls: 0,
  });
});

// Each cost the server sends is bounded and their sum is not. Past 2^53 the
// line says the cost is too large instead of printing another number.
it("says a cost too large to add is too large, not unpriced", () => {
  const big = 2 ** 52;
  expect(
    monthTotals([line("cheap_cloud", big), line("premium", big)]).costTooLarge,
  ).toBe(true);
  show([line("cheap_cloud", big), line("premium", big)]);
  expect(
    within(screen.getByTestId("spend-triage")).getByText(
      englishCopy["format.tooLargeExact"],
    ),
  ).toBeTruthy();
});
