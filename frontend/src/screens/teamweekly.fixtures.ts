// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { TeamWeeklyRep, TeamWeeklyReview } from "./teamweekly.queries";

// The team week's patterns, funnel, quality, coverage and live lists, spread
// into brief.fixtures' teamWeek. Fixed values, so a story reads the same on
// every day it is opened.

export const tobiasPatterns: TeamWeeklyRep["patterns"] = [
  {
    kind: "help_requested",
    label: "Asked for help on the Hansa renewal",
  },
  {
    kind: "leads_breached",
    label: "1 lead went past the response target",
  },
  {
    kind: "no_next_step_coverage",
    label: "1 of 4 open deals have a next step",
  },
];

export const tobiasFunnel: NonNullable<TeamWeeklyRep["funnel"]> = [
  {
    pipeline_id: "p-sales",
    pipeline_name: "Sales",
    stage_id: "s-meeting",
    stage_name: "Meeting",
    next_stage_name: "Proposal",
    rep_entered: 10,
    rep_converted: 2,
    team_entered: 20,
    team_converted: 9,
    deviation_bp: -2500,
  },
];

export const teamWeekFigures: Pick<
  TeamWeeklyReview,
  | "summary"
  | "quality"
  | "coverage"
  | "replan_recommended"
  | "exceptions"
  | "stale_deals"
> = {
  summary:
    "3 of 3 members counted; 4 deals won and 1 lost. Asked for help: Tobias Kern. Month: coverage at risk against the manager’s call (64%); a replan is recommended.",
  quality: {
    proposals_accepted: 6,
    proposals_rejected: 2,
    auto_change_reversals: 1,
  },
  coverage: [
    {
      period_kind: "month",
      basis: "manager_call",
      reference_landing_minor: 100_000_00,
      remaining_to_support_minor: 60_000_00,
      needed_open_minor: 200_000_00,
      current_open_minor: 128_000_00,
      coverage_bp: 6400,
      base_currency: "EUR",
      replan_recommended: true,
    },
    {
      period_kind: "quarter",
      absent: "insufficient_basis",
      base_currency: "EUR",
      replan_recommended: false,
    },
  ],
  replan_recommended: true,
  exceptions: [],
  stale_deals: [
    {
      deal_id: "d-hansa",
      deal_name: "Hansa renewal",
      owner_id: "u-tobias",
      last_activity_at: "2026-05-01T09:00:00Z",
      days_quiet: 66,
    },
  ],
};
