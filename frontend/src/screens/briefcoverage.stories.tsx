// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { readingsDay } from "./brief.fixtures";
import { BriefCoverage } from "./briefcoverage";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof BriefCoverage> = {
  title: "Shell/Brief coverage",
  component: BriefCoverage,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BriefCoverage>;

export const FailedTasks: Story = {
  args: {
    day: {
      ...readingsDay(),
      sources_unavailable: [
        { source: "task", reason: "failed", category: "tasks" },
      ],
    },
  },
};
export const TwoFailedSources: Story = {
  args: {
    day: {
      ...readingsDay(),
      sources_unavailable: [
        { source: "task", reason: "failed", category: "tasks" },
        {
          source: "customer_waiting",
          reason: "failed",
          category: "customer_waiting",
        },
      ],
    },
  },
};
