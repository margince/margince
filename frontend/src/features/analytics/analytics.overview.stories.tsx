// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import {
  installOverviewServer,
  isSetup,
  OVERVIEW_SCENARIOS,
  type OverviewView,
  overviewAddress,
  type ScenarioName,
} from "./analytics.overview.scenarios";

// Analytics' opening page as each reader meets it, one story per scenario and
// view the scenario map applies to, named by scenarioStoryName. The address
// names the view, so each story opens the one it is about.
const meta: Meta = {
  title: "Records/Reports/Analytics/Overview",
  parameters: { layout: "padded" },
  beforeEach: () => {
    const previousHash = globalThis.location.hash;
    return () => {
      globalThis.location.hash = previousHash;
    };
  },
};
export default meta;
type Story = StoryObj;

function scenario(name: ScenarioName, view: OverviewView): Story {
  const pair = OVERVIEW_SCENARIOS[name].views[view];
  return {
    render: () => {
      if (!isSetup(pair)) throw new Error(`${name} excludes ${view}`);
      globalThis.location.hash = overviewAddress(view);
      installOverviewServer(pair);
      return (
        <StoryProviders>
          <AnalyticsScreen />
        </StoryProviders>
      );
    },
  };
}

const dark = { globals: { theme: "dark" } };
const phone = { globals: { viewport: { value: "phone" } } };

// The names this catalog had before the scenario map, each one a scenario.
export const SalesReview = scenario("populated", "sales");
export const SalesReviewDark = { ...scenario("populated", "sales"), ...dark };
export const SalesReviewOnAPhone = {
  ...scenario("populated", "sales"),
  ...phone,
};
export const SalesReviewWithoutATarget = scenario("noTarget", "sales");
export const SalesReviewWithAFailedFigure = scenario(
  "sourceUnavailable",
  "sales",
);
export const MyProspecting = scenario("populated", "prospecting");
export const MyProspectingDark = {
  ...scenario("populated", "prospecting"),
  ...dark,
};
export const MyProspectingOnAPhone = {
  ...scenario("populated", "prospecting"),
  ...phone,
};
export const MyProspectingWithoutHandoffs = scenario(
  "qualificationWorkflow",
  "prospecting",
);
export const MyProspectingWithNothingYet = scenario("trueZero", "prospecting");
export const TeamProspecting = scenario("populated", "team");
export const TeamProspectingDark = {
  ...scenario("populated", "team"),
  ...dark,
};
export const TeamProspectingOnAPhone = {
  ...scenario("populated", "team"),
  ...phone,
};
export const TeamProspectingWithIndividualPerformanceOff = scenario(
  "individualPerformanceOff",
  "team",
);
export const TeamProspectingWithoutHandoffs = scenario(
  "qualificationWorkflow",
  "team",
);

export const SalesReviewPopulated: Story = scenario("populated", "sales");
export const MyProspectingPopulated: Story = scenario(
  "populated",
  "prospecting",
);
export const TeamProspectingPopulated: Story = scenario("populated", "team");
export const SalesReviewTrueZero: Story = scenario("trueZero", "sales");
export const MyProspectingTrueZero: Story = scenario("trueZero", "prospecting");
export const TeamProspectingTrueZero: Story = scenario("trueZero", "team");
export const SalesReviewNoTarget: Story = scenario("noTarget", "sales");
export const MyProspectingNoTarget: Story = scenario("noTarget", "prospecting");
export const SalesReviewWrongTargetScope: Story = scenario(
  "wrongTargetScope",
  "sales",
);
export const SalesReviewNotEnoughHistory: Story = scenario(
  "notEnoughHistory",
  "sales",
);
export const SalesReviewLimitedImportHistory: Story = scenario(
  "limitedImportHistory",
  "sales",
);
export const TeamProspectingLimitedImportHistory: Story = scenario(
  "limitedImportHistory",
  "team",
);
export const SalesReviewPartialValuation: Story = scenario(
  "partialValuation",
  "sales",
);
export const SalesReviewSourceUnavailable: Story = scenario(
  "sourceUnavailable",
  "sales",
);
export const MyProspectingSourceUnavailable: Story = scenario(
  "sourceUnavailable",
  "prospecting",
);
export const TeamProspectingSourceUnavailable: Story = scenario(
  "sourceUnavailable",
  "team",
);
export const SalesReviewTooFewObservations: Story = scenario(
  "tooFewObservations",
  "sales",
);
export const MyProspectingTooFewObservations: Story = scenario(
  "tooFewObservations",
  "prospecting",
);
export const TeamProspectingTooFewObservations: Story = scenario(
  "tooFewObservations",
  "team",
);
export const SalesReviewIndividualPerformanceOff: Story = scenario(
  "individualPerformanceOff",
  "sales",
);
export const MyProspectingIndividualPerformanceOff: Story = scenario(
  "individualPerformanceOff",
  "prospecting",
);
export const TeamProspectingIndividualPerformanceOff: Story = scenario(
  "individualPerformanceOff",
  "team",
);
export const SalesReviewNoActionableWork: Story = scenario(
  "noActionableWork",
  "sales",
);
export const MyProspectingNoActionableWork: Story = scenario(
  "noActionableWork",
  "prospecting",
);
export const SalesReviewLargeBacklog: Story = scenario("largeBacklog", "sales");
export const MyProspectingLargeBacklog: Story = scenario(
  "largeBacklog",
  "prospecting",
);
export const SalesReviewModelCommitDivergence: Story = scenario(
  "modelCommitDivergence",
  "sales",
);
export const SalesReviewCoverageNumerator: Story = scenario(
  "coverageNumerator",
  "sales",
);
export const SalesReviewCutoffVersusHorizon: Story = scenario(
  "cutoffVersusHorizon",
  "sales",
);
export const SalesReviewFiscalQuarter: Story = scenario(
  "fiscalQuarter",
  "sales",
);
export const MyProspectingWeekAcrossMonths: Story = scenario(
  "weekAcrossMonths",
  "prospecting",
);
export const TeamProspectingWeekAcrossMonths: Story = scenario(
  "weekAcrossMonths",
  "team",
);
export const MyProspectingTitleOnlyWork: Story = scenario(
  "titleOnlyWork",
  "prospecting",
);
export const MyProspectingHandoffWorkflow: Story = scenario(
  "handoffWorkflow",
  "prospecting",
);
export const TeamProspectingHandoffWorkflow: Story = scenario(
  "handoffWorkflow",
  "team",
);
export const MyProspectingQualificationWorkflow: Story = scenario(
  "qualificationWorkflow",
  "prospecting",
);
export const TeamProspectingQualificationWorkflow: Story = scenario(
  "qualificationWorkflow",
  "team",
);
export const TeamProspectingWeakNoHandoffTable: Story = scenario(
  "weakNoHandoffTable",
  "team",
);
export const SalesReviewTargetReached: Story = scenario(
  "targetReached",
  "sales",
);
export const SalesReviewLandingBases: Story = scenario("landingBases", "sales");
export const SalesReviewLegacyLanding: Story = scenario(
  "legacyLanding",
  "sales",
);
export const SalesReviewNoForecast: Story = scenario("noForecast", "sales");
