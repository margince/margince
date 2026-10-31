/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { forgetSeat } from "../../lib/storage";
import { reportingFixtureFrames } from "../../testing/reporting.fixtures";
import { backlogFindings } from "./analytics.overview.scenariodata";
import {
  isSetup,
  OVERVIEW_SCENARIOS,
  OVERVIEW_VIEWS,
  type OverviewView,
  overviewAddress,
  type ScenarioName,
  scenarioStoryName,
} from "./analytics.overview.scenarios.fixtures";
import * as stories from "./analytics.overview.stories";
import { openOverview, readings } from "./analytics.overview.testkit";

const originalFetch = globalThis.fetch;
beforeEach(() => forgetSeat());
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  window.location.hash = "";
});

// Every scenario the Overview must keep, typed apart from the map: S, P and T
// name the views it applies to. The first twelve are the states every view
// keeps apart.
const CENSUS: Readonly<Record<string, string>> = {
  populated: "SPT",
  trueZero: "SPT",
  noTarget: "SP",
  wrongTargetScope: "S",
  notEnoughHistory: "SP",
  limitedImportHistory: "ST",
  partialValuation: "S",
  sourceUnavailable: "SPT",
  tooFewObservations: "SPT",
  individualPerformanceOff: "SPT",
  noActionableWork: "SP",
  largeBacklog: "SP",
  readOnlyReviewer: "S",
  modelCommitDivergence: "S",
  coverageNumerator: "S",
  cutoffVersusHorizon: "S",
  fiscalQuarter: "S",
  weekAcrossMonths: "PT",
  titleOnlyWork: "P",
  handoffWorkflow: "PT",
  qualificationWorkflow: "PT",
  weakNoHandoffTable: "T",
  targetReached: "S",
  landingBases: "S",
  legacyLanding: "S",
  noForecast: "S",
  twoPipelines: "S",
  unknownWorkflow: "P",
  worklistFailed: "P",
  readingsWithheld: "P",
};
const LETTER: Readonly<Record<OverviewView, string>> = {
  sales: "S",
  prospecting: "P",
  team: "T",
};

// The story names the catalog had before the scenario map, kept as aliases.
const ALIASES = [
  "SalesReview",
  "SalesReviewDark",
  "SalesReviewOnAPhone",
  "SalesReviewWithoutATarget",
  "SalesReviewWithAFailedFigure",
  "MyProspecting",
  "MyProspectingDark",
  "MyProspectingOnAPhone",
  "MyProspectingWithoutHandoffs",
  "MyProspectingWithNothingYet",
  "TeamProspecting",
  "TeamProspectingDark",
  "TeamProspectingOnAPhone",
  "TeamProspectingWithIndividualPerformanceOff",
  "TeamProspectingWithoutHandoffs",
];

const censusPairs = Object.entries(CENSUS).flatMap(([name, views]) =>
  OVERVIEW_VIEWS.filter((view) => views.includes(LETTER[view])).map((view) => ({
    name: name as ScenarioName,
    view,
  })),
);

describe("the Overview scenario map", () => {
  it("holds exactly the census scenarios, each on the views it names", () => {
    expect(Object.keys(OVERVIEW_SCENARIOS).sort()).toEqual(
      Object.keys(CENSUS).sort(),
    );
    for (const [name, views] of Object.entries(CENSUS)) {
      const scenario = OVERVIEW_SCENARIOS[name as ScenarioName];
      const applied = OVERVIEW_VIEWS.filter((view) =>
        isSetup(scenario.views[view]),
      )
        .map((view) => LETTER[view])
        .join("");
      expect(applied, name).toBe(views);
    }
  });

  it("gives every excluded pair its reason", () => {
    for (const scenario of Object.values(OVERVIEW_SCENARIOS)) {
      for (const pair of Object.values(scenario.views)) {
        if (!isSetup(pair)) expect(pair.excluded.length).toBeGreaterThan(10);
      }
    }
  });

  it("has one story per census pair, and otherwise only the old names", () => {
    const expected = censusPairs.map(({ name, view }) =>
      scenarioStoryName(name, view),
    );
    const exported = Object.keys(stories).filter((key) => key !== "default");
    expect(exported.sort()).toEqual([...expected, ...ALIASES].sort());
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

  it("reads every Sales review chart and movement on the scenario's own clock", () => {
    for (const { name, view } of censusPairs) {
      const pair = OVERVIEW_SCENARIOS[name].views[view];
      if (view !== "sales" || !isSetup(pair)) continue;
      const headline = pair.answer(new URL("http://localhost/evaluate"));
      const movement = pair.answer(
        new URL("http://localhost/evaluate?blocks=pipeline_movement"),
      );
      const { evaluated_at: clock, interval } = headline.context;
      expect(movement.context.evaluated_at, name).toBe(clock);
      for (const chart of [...headline.charts, ...movement.charts]) {
        if (chart.state_at) expect(chart.state_at, name).toBe(clock);
        if (chart.capture_status)
          expect(chart.capture_status.last_attempt_at, name).toBe(clock);
        if (chart.interval)
          expect(chart.interval.start_at, name).toBe(interval.start_at);
      }
    }
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

type Signature = () => Promise<unknown>;
const shows =
  (text: string | RegExp): Signature =>
  async () =>
    expect((await screen.findAllByText(text))[0]).toBeVisible();
const lacks =
  (text: string): Signature =>
  async () =>
    expect(screen.queryByText(text)).toBeNull();

// What each pair must put on the screen beyond its readings.
const SIGNATURES: Readonly<
  Record<string, Partial<Record<OverviewView, Signature>>>
> = {
  populated: {
    sales: shows("3.6×"),
    prospecting: shows("All 5 items in your Worklist"),
    team: shows("Wrong territory"),
  },
  trueZero: {
    sales: shows(/^0(\.0)?×$/),
    prospecting: async () => {
      await shows("No meetings recorded as held this week")();
      await shows("Nothing needs you right now")();
    },
    team: shows(/Of the 0 rounds/),
  },
  noTarget: {
    sales: shows("No sales target for this quarter"),
    prospecting: shows(/^No monthly target set\./),
  },
  wrongTargetScope: { sales: shows("Pick one team to compare with a target") },
  notEnoughHistory: {
    sales: shows("Comparison starts after a second compatible capture."),
    prospecting: async () => {
      await shows("—")();
      expect(screen.queryByText(/history/i)).toBeNull();
    },
  },
  limitedImportHistory: { sales: lacks("4 Oct"), team: shows("Members") },
  partialValuation: {
    sales: shows(/1 open deal in NOK has no exchange rate to EUR\./),
  },
  sourceUnavailable: {
    sales: async () => {
      await shows("Could not be calculated")();
      await shows("Connection maintenance")();
    },
    prospecting: shows(
      "Some of your work could not be read, so this list may be missing items.",
    ),
    team: shows("Retry"),
  },
  tooFewObservations: {
    sales: shows("Too few observations"),
    prospecting: async () => {
      await shows("Too few")();
      await shows("2 leads still waiting for a reply")();
    },
    team: shows("A small sample"),
  },
  individualPerformanceOff: {
    sales: lacks("Maya"),
    prospecting: shows("All 5 items in your Worklist"),
    team: shows(
      "Individual performance is off, so no member’s figures are shown.",
    ),
  },
  noActionableWork: {
    sales: shows("No forecast check is open for this scope."),
    prospecting: shows("Nothing needs you right now"),
  },
  largeBacklog: {
    sales: shows("All open checks: 24 findings · 7 deals"),
    prospecting: shows("All 210 items in your Worklist"),
  },
  readOnlyReviewer: {
    sales: async () => {
      const list = within(
        await screen.findByRole("region", { name: "Needs your attention" }),
      );
      expect(await list.findAllByRole("link", { name: "Open" })).toHaveLength(
        3,
      );
      expect(list.queryByRole("link", { name: "Answer" })).toBeNull();
    },
  },
  modelCommitDivergence: { sales: shows("Modelled quarter finish") },
  coverageNumerator: { sales: shows("38.8×") },
  cutoffVersusHorizon: { sales: shows("1 October – 31 December") },
  fiscalQuarter: { sales: shows("1 August – 31 October") },
  weekAcrossMonths: {
    prospecting: shows(/Monthly targets, .*Oct/),
    team: shows("Wrong territory"),
  },
  titleOnlyWork: {
    prospecting: shows("Call Nordhafen Logistik back about the fleet trial"),
  },
  handoffWorkflow: {
    prospecting: async () => {
      await shows(/Casey Closer can no longer act/)();
      await shows(/No budget this quarter/)();
    },
    team: shows("Wrong territory"),
  },
  qualificationWorkflow: {
    prospecting: shows("Meetings to prepare"),
    team: shows(/does not use SDR handoffs/),
  },
  weakNoHandoffTable: { team: shows("Members") },
  targetReached: { sales: shows("Target reached") },
  landingBases: { sales: shows("Won + confirmed commit") },
  legacyLanding: { sales: shows("Basis not recorded") },
  noForecast: { sales: shows("Unavailable") },
  unknownWorkflow: {
    prospecting: async () => {
      await shows("Could not read whether this company uses SDR handoffs.")();
      expect(screen.queryByText("Accepted handoffs")).toBeNull();
      expect(screen.queryByText(/does not use SDR handoffs/)).toBeNull();
    },
  },
  worklistFailed: {
    prospecting: async () => {
      await shows("This section did not load.")();
      expect(screen.queryByText("Nothing needs you right now")).toBeNull();
      expect(screen.queryByRole("link", { name: /^Open Call/ })).toBeNull();
    },
  },
  readingsWithheld: {
    prospecting: async () => {
      await shows("Hidden from you")();
      await shows("The count of unanswered leads is hidden from you")();
    },
  },
  twoPipelines: {
    sales: async () => {
      const user = userEvent.setup({ delay: null });
      await user.click(screen.getByRole("button", { name: "Filters" }));
      await user.click(screen.getByRole("combobox", { name: "Pipeline" }));
      const listbox = await screen.findByRole("listbox");
      expect(
        within(listbox).getByRole("option", { name: "Renewals" }),
      ).toBeVisible();
    },
  },
};

describe("each census pair", () => {
  it("has a signature", () => {
    for (const { name, view } of censusPairs)
      expect(SIGNATURES[name]?.[view], `${name} on ${view}`).toBeDefined();
  });

  it.each(
    censusPairs.map(
      ({ name, view }) => [`${name} on ${view}`, name, view] as const,
    ),
  )(
    "%s opens on its readings and shows what it is about",
    async (_label, name, view) => {
      const pair = OVERVIEW_SCENARIOS[name].views[view];
      if (!isSetup(pair)) throw new Error(`${name} excludes ${view}`);
      openOverview(overviewAddress(view), pair.answer, pair.routes);
      expect(await readings()).toBeDefined();
      await SIGNATURES[name]?.[view]?.();
    },
  );
});
