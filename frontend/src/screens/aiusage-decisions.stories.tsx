// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DecisionSummaryRow } from "./aiusage-decisions";
import { StoryProviders } from "./story-utils";

const DECISIONS: components["schemas"]["AiDecisionSummary"][] = [
  {
    task: "site_triage",
    asked: 9,
    decided: 2,
    fallbacks: {
      decision_below_floor: 4,
      decision_error: 2,
      decision_uncertified: 1,
    },
  },
  { task: "capture_classify", asked: 3, decided: 3, fallbacks: {} },
];

const meta: Meta<typeof DecisionSummaryRow> = {
  title: "Settings/AI/AI usage/Decision model",
  component: DecisionSummaryRow,
  render: (args) => (
    <StoryProviders>
      <DecisionSummaryRow {...args} />
    </StoryProviders>
  ),
  args: { decisions: DECISIONS, taskName: (task: string) => task },
};
export default meta;
type Story = StoryObj<typeof DecisionSummaryRow>;

// One reason per line, largest first, and a dash where a task never fell back.
export const Fallbacks: Story = {};
export const FallbacksDark: Story = { globals: { theme: "dark" } };
export const NothingAsked: Story = { args: { decisions: [] } };
