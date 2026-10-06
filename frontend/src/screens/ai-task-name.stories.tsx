// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskName } from "./ai-task-name";
import { StoryProviders } from "./story-utils";

// A task's name in the AI tasks and usage tables. Click it to read what the
// task does; a task with nothing to say is plain text.
const meta: Meta<typeof TaskName> = {
  title: "Settings/AI/AI models/Task name",
  component: TaskName,
  render: (args) => (
    <StoryProviders>
      <TaskName {...args} />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof TaskName>;

export const WithSummary: Story = {
  args: {
    name: "Thread confidentiality check",
    summary:
      "Decides whether an email thread in a restricted mailbox is ordinary enough to open to the team.",
  },
};

export const WithoutSummary: Story = {
  args: { name: "embeddings", summary: undefined },
};

export const WithSummaryDark: Story = {
  globals: { theme: "dark" },
  args: WithSummary.args,
};
