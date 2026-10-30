// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { HierarchyRollupPanel } from "./companies.hierarchy";

type Rollup = components["schemas"]["CompanyHierarchyRollup"];

// The rollup panel in its two period states. In the quarter under way the step
// forward is refused. The other is an earlier quarter the reader stepped back to.
// The weighted deal value is the same in both, because it is always today's.

const underWay: Rollup = {
  root_id: "o-1",
  scope: "tree",
  weighted_pipeline: { amount_minor: 2_400_000, currency: "EUR" },
  pipeline_state_at: "2026-10-30T09:00:00Z",
  closed_won: { amount_minor: 1_800_000, currency: "EUR" },
  closed_won_period: {
    grain: "quarter",
    label: "2026-Q4",
    start_at: "2026-10-01T00:00:00Z",
    end_at: "2027-01-01T00:00:00Z",
    anchor: "closed_at",
    chosen: false,
    previous: "2026-Q3",
    next: null,
  },
  activity_count_30d: 14,
  activity_window: {
    start_at: "2026-09-30T09:00:00Z",
    end_at: "2026-10-30T09:00:00Z",
  },
  aggregated_account_count: 3,
  restricted_excluded: [{ id: "o-9", display_name: "Hidden Subsidiary GmbH" }],
  computed_at: "2026-10-30T09:00:00Z",
  current_periods: {
    period_year: "2026",
    period_quarter: "2026-Q4",
    period_month: "2026-10",
  },
};

const earlier: Rollup = {
  ...underWay,
  closed_won: { amount_minor: 5_200_000, currency: "EUR" },
  closed_won_period: {
    ...underWay.closed_won_period,
    label: "2026-Q3",
    start_at: "2026-07-01T00:00:00Z",
    end_at: "2026-10-01T00:00:00Z",
    chosen: true,
    previous: "2026-Q2",
    next: "2026-Q4",
  },
};

function story(rollup: Rollup): StoryObj {
  return {
    render: () => {
      installFetchStub({
        "GET /companies/o-1/hierarchy-rollup": () => jsonResponse(rollup),
      });
      return (
        <StoryProviders>
          <HierarchyRollupPanel companyId="o-1" />
        </StoryProviders>
      );
    },
  };
}

const meta: Meta = { title: "Records/Company 360/Hierarchy rollup" };
export default meta;

export const QuarterUnderWay = story(underWay);
export const EarlierQuarter = story(earlier);
