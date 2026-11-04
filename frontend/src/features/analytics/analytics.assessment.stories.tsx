// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AssessmentPanel, type SalesAssessment } from "./analytics.assessment";
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
type View = "sales" | "prospecting" | "team";
type Sentence = NonNullable<SalesAssessment["paragraphs"]>[number][number];
function sentence(text: string, ...cells: string[]): Sentence {
  return { text, kind: "observation", cells };
}
const examples: Record<View, Pick<SalesAssessment, "paragraphs" | "cells">> = {
  sales: {
    paragraphs: [
      [
        sentence(
          "Sales won stands at €420k against the quarter’s €800k target.",
          "won",
          "target",
        ),
        sentence(
          "The current forecast is €760k, combining €420k already won with €340k of confirmed open commitments.",
          "landing",
          "won",
          "remaining",
        ),
        sentence("That projection remains below target.", "landing", "target"),
      ],
      [
        sentence(
          "There are 18 open deals with €1.36m in pipeline due this quarter.",
          "pipeline",
          "deals",
        ),
        sentence(
          "The modelled quarter finish ranges from €610k to €980k, with the €800k target inside that range.",
          "low",
          "high",
          "target",
        ),
        sentence(
          "Open the pipeline records to review the opportunities and their close dates before the next forecast discussion.",
          "pipeline",
          "deals",
        ),
      ],
    ],
    cells: [
      { id: "won", label: "Sales won this quarter", value: "€420k" },
      { id: "target", label: "Quarter target", value: "€800k" },
      { id: "landing", label: "Won + confirmed commit", value: "€760k" },
      { id: "remaining", label: "Confirmed open commitments", value: "€340k" },
      { id: "pipeline", label: "Pipeline due this quarter", value: "€1.36m" },
      { id: "deals", label: "Open deals", value: "18" },
      { id: "low", label: "Modelled range, lower bound", value: "€610k" },
      { id: "high", label: "Modelled range, upper bound", value: "€980k" },
    ],
  },
  prospecting: {
    paragraphs: [
      [
        sentence(
          "Four meetings were confirmed as held and two handoffs were accepted this week.",
          "held",
          "accepted",
        ),
        sentence(
          "The monthly meeting total is 18 of the 24 target, and monthly accepted handoffs stand at 6 of 8.",
          "held_month",
          "held_target",
          "accepted_month",
          "accepted_target",
        ),
      ],
      [
        sentence(
          "Three leads are still waiting for their first reply.",
          "waiting",
        ),
        sentence(
          "Median first response time is 1.7 hours among leads with a recorded reply; it does not measure the wait for those three unanswered leads.",
          "response",
          "waiting",
        ),
        sentence(
          "Open the waiting-reply records to work through that queue.",
          "waiting",
        ),
      ],
    ],
    cells: [
      { id: "held", label: "Meetings held this week", value: "4" },
      { id: "accepted", label: "Handoffs accepted this week", value: "2" },
      { id: "held_month", label: "Meetings held this month", value: "18" },
      { id: "held_target", label: "Monthly meeting target", value: "24" },
      {
        id: "accepted_month",
        label: "Handoffs accepted this month",
        value: "6",
      },
      { id: "accepted_target", label: "Monthly handoff target", value: "8" },
      { id: "waiting", label: "Leads awaiting a first reply now", value: "3" },
      {
        id: "response",
        label: "Median first response time",
        value: "1.7 hours",
      },
    ],
  },
  team: {
    paragraphs: [
      [
        sentence(
          "Five leads were first qualified and seven meetings were confirmed as held this week.",
          "qualified",
          "held",
        ),
        sentence(
          "The five qualifications count first events in the week; a later reassignment keeps the original qualification credit.",
          "qualified",
        ),
      ],
      [
        sentence("Two leads are still awaiting their first reply.", "waiting"),
        sentence(
          "That queue describes work waiting now, while the five qualifications describe completed outcomes in the selected week.",
          "waiting",
          "qualified",
        ),
        sentence(
          "Open the two waiting records and confirm who will reply before the next team review.",
          "waiting",
        ),
      ],
    ],
    cells: [
      { id: "qualified", label: "First qualifications this week", value: "5" },
      { id: "held", label: "Meetings held this week", value: "7" },
      { id: "waiting", label: "Leads awaiting a first reply now", value: "2" },
    ],
  },
};
function outlook(view: View): Story {
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
      const example = examples[view];
      const assessment: SalesAssessment = {
        ...example,
        evaluation_key: evaluation.evaluation_key,
        generated_at: evaluation.context.evaluated_at,
        generated_by: "model",
        sentences: example.paragraphs?.flat().slice(0, 6) ?? [],
        decisions: 0,
        action_ids: [action.id],
      };
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
