// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AssessmentPanel } from "./analytics.assessment";
import { type OutlookView, outlookFor } from "./analytics.assessment.fixtures";
import {
  frames,
  prospectingOn,
  quarterSales,
  teamOn,
} from "./analytics.overview.scenariodata";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Outlook",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
function outlook(view: OutlookView): Story {
  return {
    render: () => {
      const base =
        view === "sales"
          ? quarterSales()
          : view === "prospecting"
            ? prospectingOn(frames.weekAcrossMonths)
            : teamOn(frames.weekWithClockChange);
      const metric =
        view === "sales" ? "open_pipeline" : "leads_awaiting_response";
      const action: NonNullable<typeof base.overview_actions>[number] = {
        id: `metric:${metric}`,
        label:
          view === "sales"
            ? "Open pipeline: review records"
            : "Leads awaiting a first reply: review records",
        count: view === "sales" ? 18 : view === "prospecting" ? 3 : 2,
        evidence: { metric, context_id: "state" },
      };
      const evaluation = { ...base, overview_actions: [action] };
      const assessment = outlookFor(view, evaluation, [action.id]);
      installFetchStub({
        ...reportingStoryRoutes(),
        "GET /analytics/metrics": () =>
          jsonResponse({
            metrics: [],
            presets: [],
            assessment_views: ["sales", "prospecting", "team"],
          }),
        "POST /analytics/assessment": () => jsonResponse(assessment),
      });
      return (
        <StoryProviders>
          <AssessmentPanel
            view={view}
            evaluation={evaluation}
            selection={evaluation.selection}
            onEvidence={() => {}}
          />
        </StoryProviders>
      );
    },
  };
}
export const QuarterOutlook = outlook("sales");
export const MyProspecting = outlook("prospecting");
export const TeamProspecting = outlook("team");
export const QuarterOutlookDark = {
  ...outlook("sales"),
  globals: { theme: "dark" },
};
export const QuarterOutlookOnPhone = {
  ...outlook("sales"),
  tags: ["uat-phone"],
  globals: { viewport: { value: "phone" } },
};
