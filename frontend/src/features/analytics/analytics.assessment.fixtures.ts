// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { SalesAssessment } from "./analytics.assessment";
import type { ReportingEvaluation } from "./reporting.model";

export type OutlookView = "sales" | "prospecting" | "team";
type Sentence = NonNullable<SalesAssessment["paragraphs"]>[number][number];
function sentence(text: string, ...cells: string[]): Sentence {
  return { text, kind: "observation", cells };
}
const examples: Record<
  OutlookView,
  Pick<SalesAssessment, "paragraphs" | "cells">
> = {
  sales: {
    paragraphs: [
      [
        sentence(
          "Sales won stands at 420000 EUR against the quarter’s 800000 EUR target.",
          "metric:bookings_won",
          "forecast:target",
        ),
        sentence(
          "The current forecast is 760000 EUR, combining 420000 EUR already won with 340000 EUR of confirmed open commitments.",
          "metric:forecast_landing",
          "metric:bookings_won",
          "forecast:remaining",
        ),
        sentence(
          "That projection remains below target.",
          "metric:forecast_landing",
          "forecast:target",
        ),
      ],
      [
        sentence(
          "There is 1360000 EUR in pipeline due this quarter.",
          "metric:open_pipeline",
        ),
        sentence(
          "The modelled quarter finish ranges from 610000 EUR to 980000 EUR, with the 800000 EUR target inside that range.",
          "forecast:range_low",
          "forecast:range_high",
          "forecast:target",
        ),
        sentence(
          "Open the pipeline records to review the opportunities and their close dates before the next forecast discussion.",
          "metric:open_pipeline",
        ),
      ],
    ],
    cells: [
      {
        id: "metric:bookings_won",
        label: "Sales won this quarter",
        value: "420000 EUR",
      },
      { id: "forecast:target", label: "Quarter target", value: "800000 EUR" },
      {
        id: "metric:forecast_landing",
        label: "Won + Likely deals",
        value: "760000 EUR",
      },
      {
        id: "forecast:remaining",
        label: "Confirmed open commitments",
        value: "340000 EUR",
      },
      {
        id: "metric:open_pipeline",
        label: "Pipeline due this quarter",
        value: "1360000 EUR",
      },
      {
        id: "forecast:range_low",
        label: "Modelled range, lower bound",
        value: "610000 EUR",
      },
      {
        id: "forecast:range_high",
        label: "Modelled range, upper bound",
        value: "980000 EUR",
      },
    ],
  },
  prospecting: {
    paragraphs: [
      [
        sentence(
          "Four meetings were confirmed as held and two handoffs were accepted this week.",
          "metric:meetings_held",
          "metric:accepted_opportunities",
        ),
      ],
      [
        sentence(
          "Three leads are still waiting for their first reply.",
          "metric:leads_awaiting_response_now",
        ),
        sentence(
          "Median first response time is 1.7 hours among leads with a recorded reply; it does not measure the wait for those three unanswered leads.",
          "metric:lead_first_response",
          "metric:leads_awaiting_response_now",
        ),
        sentence(
          "Open the waiting-reply records to work through that queue.",
          "metric:leads_awaiting_response_now",
        ),
      ],
    ],
    cells: [
      {
        id: "metric:meetings_held",
        label: "Meetings held this week",
        value: "4",
      },
      {
        id: "metric:accepted_opportunities",
        label: "Handoffs accepted this week",
        value: "2",
      },
      {
        id: "metric:leads_awaiting_response_now",
        label: "Leads awaiting a first reply now",
        value: "3",
      },
      {
        id: "metric:lead_first_response",
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
          "metric:leads_qualified",
          "metric:meetings_held",
        ),
        sentence(
          "The five qualifications count first events in the week; a later reassignment keeps the original qualification credit.",
          "metric:leads_qualified",
        ),
      ],
      [
        sentence(
          "Two leads are still awaiting their first reply.",
          "metric:leads_awaiting_response_now",
        ),
        sentence(
          "That queue describes work waiting now, while the five qualifications describe completed outcomes in the selected week.",
          "metric:leads_awaiting_response_now",
          "metric:leads_qualified",
        ),
        sentence(
          "Open the two waiting records and confirm who will reply before the next team review.",
          "metric:leads_awaiting_response_now",
        ),
      ],
    ],
    cells: [
      {
        id: "metric:leads_qualified",
        label: "First qualifications this week",
        value: "5",
      },
      {
        id: "metric:meetings_held",
        label: "Meetings held this week",
        value: "7",
      },
      {
        id: "metric:leads_awaiting_response_now",
        label: "Leads awaiting a first reply now",
        value: "2",
      },
    ],
  },
};

/** The written outlook for one view, as the server answers it for `evaluation`. */
export function outlookFor(
  view: OutlookView,
  evaluation: ReportingEvaluation,
  actionIds: readonly string[] = [],
): SalesAssessment {
  const example = examples[view];
  return {
    ...example,
    evaluation_key: evaluation.evaluation_key,
    generated_at: evaluation.context.evaluated_at,
    generated_by: "model",
    sentences: example.paragraphs?.flat().slice(0, 6) ?? [],
    decisions: 0,
    action_ids: [...actionIds],
  };
}
