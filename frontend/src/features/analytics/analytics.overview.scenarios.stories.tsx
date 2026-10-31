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

// Every scenario on every view it applies to, named by scenarioStoryName. A
// pair the scenario map excludes has no story, and its reason is in that map.
const meta: Meta = {
  title: "Records/Reports/Analytics/Overview scenarios",
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
export const SalesReviewPerPersonOff: Story = scenario("perPersonOff", "sales");
export const MyProspectingPerPersonOff: Story = scenario(
  "perPersonOff",
  "prospecting",
);
export const TeamProspectingPerPersonOff: Story = scenario(
  "perPersonOff",
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
export const SalesReviewNoForecast: Story = scenario("noForecast", "sales");
