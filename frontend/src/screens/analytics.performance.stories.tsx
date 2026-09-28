// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Stage } from "./analytics.cells";
import { StageAgeTable, WinLossTable } from "./analytics.performance";
import { StoryProviders } from "./story-utils";

// The performance section's two tables on their own. Each duration column draws
// the 75th percentile as a bar with the median solid inside it; a row the
// engine withheld below its sample floor says so in words and draws no bar,
// because an empty bar would read as a fast stage.
const meta: Meta = { title: "Records/Reports/Performance tables" };
export default meta;

type Story = StoryObj;

const STAGES: Stage[] = [
  {
    id: "st-qualify",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "st-proposal",
    pipeline_id: "pl",
    name: "Proposal sent",
    position: 2,
    semantic: "open",
    win_probability: 60,
  },
  {
    id: "st-negotiation",
    pipeline_id: "pl",
    name: "Negotiation",
    position: 3,
    semantic: "open",
    win_probability: 80,
  },
];

export const WonAndLost: Story = {
  render: () => (
    <StoryProviders>
      <WinLossTable
        locale="en"
        baseCurrency="EUR"
        rows={[
          {
            status: "won",
            deal_count: 38,
            raw_minor: 612_000_00,
            median_days: 41,
            p75_days: 68,
          },
          {
            status: "lost",
            deal_count: 22,
            raw_minor: 298_000_00,
            median_days: 55,
            p75_days: 90,
          },
        ]}
      />
    </StoryProviders>
  ),
};

// A young installation: the population is closed deals and there are none,
// which is an answer in words rather than a table of headers.
export const NothingClosedYet: Story = {
  render: () => (
    <StoryProviders>
      <WinLossTable locale="en" baseCurrency="EUR" rows={[]} />
    </StoryProviders>
  ),
};

export const TimeInStage: Story = {
  render: () => (
    <StoryProviders>
      <StageAgeTable
        locale="en"
        stages={STAGES}
        rows={[
          {
            stage_id: "st-qualify",
            deal_count: 14,
            median_days: 12,
            p75_days: 30,
          },
          {
            stage_id: "st-proposal",
            deal_count: 7,
            median_days: 21,
            p75_days: 45,
          },
          {
            stage_id: "st-negotiation",
            deal_count: 4,
            median_days: null,
            p75_days: null,
          },
        ]}
      />
    </StoryProviders>
  ),
};

export const TimeInStageDark: Story = {
  ...TimeInStage,
  globals: { theme: "dark" },
};
