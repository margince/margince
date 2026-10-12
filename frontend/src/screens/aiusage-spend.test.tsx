// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { Panel } from "../design-system/panel";
import { LocaleProvider } from "../i18n";
import { SpendByTask, spendRows } from "./aiusage-spend";

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
  expect(within(row).getByText("Partly priced")).toBeTruthy();
  expect(
    within(screen.getByTestId("spend-triage-premium")).getByText("—"),
  ).toBeTruthy();
});

it("marks a priced line that also carried calls with no rate", () => {
  show([line("premium", 300, 1)]);
  expect(
    within(screen.getByTestId("spend-triage-premium")).getByText(
      "Partly priced",
    ),
  ).toBeTruthy();
});

it("reads a task whose every call was priced as its plain total", () => {
  const [total] = spendRows([line("cheap_cloud", 120), line("premium", 80)]);
  expect(total.cost).toBe(200);
  expect(total.partlyPriced).toBe(false);
  show([line("cheap_cloud", 120), line("premium", 80)]);
  expect(screen.queryByText("Partly priced")).toBeNull();
});
