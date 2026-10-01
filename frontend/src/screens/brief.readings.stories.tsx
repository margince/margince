// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { leadRow, readingsDay, wholeLeads } from "./brief.fixtures";
import { BriefReadingsStrip } from "./brief.readings";
import { StoryProviders } from "./story-utils";

const meta: Meta<typeof BriefReadingsStrip> = {
  title: "Shell/Home/Prospecting readings",
  component: BriefReadingsStrip,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BriefReadingsStrip>;

export const PlannedOutreach: Story = {
  args: {
    day: readingsDay(
      { prospecting: 1 },
      [leadRow("lead-1", "2026-08-31T15:00:00Z")],
      [wholeLeads(1)],
    ),
  },
};

export const TaskSourceUnavailable: Story = {
  args: {
    day: {
      ...readingsDay({ prospecting: 0 }, [], []),
      sources_unavailable: [
        { source: "task", category: "tasks", reason: "failed" },
      ],
    },
  },
};

export const PlannedOutreachDark: Story = {
  ...PlannedOutreach,
  globals: { theme: "dark" },
};

export const TaskSourceUnavailableDark: Story = {
  ...TaskSourceUnavailable,
  globals: { theme: "dark" },
};
