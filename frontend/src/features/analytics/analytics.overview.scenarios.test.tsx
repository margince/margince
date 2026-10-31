/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { removeStored, STORAGE_KEYS } from "../../lib/storage";
import { reportingFixtureFrames } from "../../testing/reporting.fixtures";
import { backlogFindings } from "./analytics.overview.scenariodata";
import {
  ANALYTICS_STATES,
  isSetup,
  OVERVIEW_SCENARIOS,
  OVERVIEW_VIEWS,
  overviewAddress,
  type ScenarioName,
  scenarioStoryName,
} from "./analytics.overview.scenarios";
import * as stories from "./analytics.overview.scenarios.stories";
import { openOverview, readings } from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => removeStored(STORAGE_KEYS.analyticsOverviewView));
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

const names = Object.keys(OVERVIEW_SCENARIOS) as ScenarioName[];
const pairs = names.flatMap((name) =>
  OVERVIEW_VIEWS.map((view) => ({
    name,
    view,
    pair: OVERVIEW_SCENARIOS[name].views[view],
  })),
);
const setups = pairs.flatMap(({ name, view, pair }) =>
  isSetup(pair) ? [{ name, view, setup: pair }] : [],
);

describe("the Overview scenario map", () => {
  it("names a scenario for every state a view must keep apart", () => {
    for (const state of ANALYTICS_STATES) expect(names).toContain(state);
  });

  it("gives every excluded pair its reason", () => {
    for (const { pair } of pairs) {
      if (!isSetup(pair)) expect(pair.excluded.length).toBeGreaterThan(10);
    }
  });

  it("has one story per applicable pair, and no story for an excluded one", () => {
    const expected = setups.map(({ name, view }) =>
      scenarioStoryName(name, view),
    );
    const exported = Object.keys(stories).filter((key) => key !== "default");
    expect(exported.sort()).toEqual(expected.sort());
  });

  it("puts at least 20 findings on at most 8 deals in the backlog", () => {
    const findings = backlogFindings(
      reportingFixtureFrames.quarterCutoff.clock,
    );
    const deals = new Set(findings.map((finding) => finding.subject_id));
    expect(findings.length).toBeGreaterThanOrEqual(20);
    expect(deals.size).toBeLessThanOrEqual(8);
    expect(new Set(findings.map((finding) => finding.id)).size).toBe(
      findings.length,
    );
  });

  it("keeps the coverage numerator apart from all open pipeline", () => {
    const pair = OVERVIEW_SCENARIOS.coverageNumerator.views.sales;
    if (!isSetup(pair)) throw new Error("coverageNumerator draws Sales review");
    const evaluation = pair.answer(new URL("http://localhost/evaluate"));
    const open = evaluation.metrics.find((each) => each.id === "open_pipeline");
    const comparison = evaluation.metrics.find(
      (each) => each.id === "forecast_landing",
    )?.target_comparison;
    const { open_pipeline: eligible = 0, remaining = 1 } = comparison ?? {};
    expect(comparison?.coverage).toBeCloseTo(eligible / remaining, 6);
    expect(open?.value).not.toBe(eligible);
  });

  it("frames a fiscal quarter off January, a week across months and a clock change", () => {
    const { fiscalQuarter, weekAcrossMonths, weekWithClockChange } =
      reportingFixtureFrames;
    expect(fiscalQuarter.fiscalYearStartMonth).not.toBe(1);
    const hours = (from: string, to: string) =>
      (Date.parse(to) - Date.parse(from)) / 3_600_000;
    // Berlin midnight is 22:00Z in summer and 23:00Z in winter.
    expect(
      hours(
        fiscalQuarter.targetInterval.start_at,
        fiscalQuarter.targetInterval.end_at,
      ) % 24,
    ).toBe(1);
    expect(
      hours(weekWithClockChange.interval.start_at, "2026-10-25T23:00:00Z"),
    ).toBe(7 * 24 + 1);
    expect(Date.parse(weekAcrossMonths.targetInterval.end_at)).toBeLessThan(
      Date.parse(weekAcrossMonths.interval.start_at) + 7 * 24 * 3_600_000,
    );
  });
});

describe("each applicable pair", () => {
  it.each(
    setups.map(
      ({ name, view, setup }) => [`${name} on ${view}`, view, setup] as const,
    ),
  )("%s opens on its readings", async (_label, view, setup) => {
    openOverview(overviewAddress(view), setup.answer, setup.routes);
    expect(await readings()).toBeDefined();
  });
});
