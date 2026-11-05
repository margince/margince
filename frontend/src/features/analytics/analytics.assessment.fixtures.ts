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
          "won",
          "target",
        ),
        sentence(
          "The current forecast is 760000 EUR, combining 420000 EUR already won with 340000 EUR of confirmed open commitments.",
          "landing",
          "won",
          "remaining",
        ),
        sentence("That projection remains below target.", "landing", "target"),
      ],
      [
        sentence(
          "There are 18 open deals with 1360000 EUR in pipeline due this quarter.",
          "pipeline",
          "deals",
        ),
        sentence(
          "The modelled quarter finish ranges from 610000 EUR to 980000 EUR, with the 800000 EUR target inside that range.",
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
      { id: "won", label: "Sales won this quarter", value: "420000 EUR" },
      { id: "target", label: "Quarter target", value: "800000 EUR" },
      { id: "landing", label: "Won + Likely deals", value: "760000 EUR" },
      {
        id: "remaining",
        label: "Confirmed open commitments",
        value: "340000 EUR",
      },
      {
        id: "pipeline",
        label: "Pipeline due this quarter",
        value: "1360000 EUR",
      },
      { id: "deals", label: "Open deals", value: "18" },
      { id: "low", label: "Modelled range, lower bound", value: "610000 EUR" },
      { id: "high", label: "Modelled range, upper bound", value: "980000 EUR" },
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
