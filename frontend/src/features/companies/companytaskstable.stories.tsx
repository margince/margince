// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { company360, emptySection } from "../../testing/company.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { CompanyTaskTable } from "./companytaskstable";

const meta: Meta<typeof CompanyTaskTable> = {
  title: "Records/Company 360/Task table",
  component: CompanyTaskTable,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CompanyTaskTable>;

// One late task, one due within the week and one with no date. They are read
// against the 360's own day, so the badges never move with the clock.
const tasks = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  next_steps: {
    ...emptySection,
    data: [
      {
        activity_id: "a-1",
        subject: "Send Dana the renewal paperwork",
        due_at: "2026-07-25T12:00:00Z",
        overdue: true,
      },
      {
        activity_id: "a-2",
        subject: "Prepare the walkthrough brief",
        due_at: "2026-08-19T12:00:00Z",
        overdue: false,
      },
      {
        activity_id: "a-3",
        subject: "Find a second contact at Brandt",
        overdue: false,
      },
    ],
  },
};

export const LateDueAndUndated: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <CompanyTaskTable view={tasks} />
      </div>
    </StoryProviders>
  ),
};

export const LateDueAndUndatedDark: Story = {
  ...LateDueAndUndated,
  globals: { theme: "dark" },
};

export const NoOpenTasks: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <CompanyTaskTable view={company360} />
      </div>
    </StoryProviders>
  ),
};
