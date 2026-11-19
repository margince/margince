// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { TaskName } from "./ai-task-name";

// A task's name wherever a screen names an AI task. Click it to read what the
// task does and the key it runs under; a bare key is plain text.
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
    task: "capture_confidentiality_verdict",
    mode: "In the background",
    summary:
      "Decides whether an email thread in a restricted mailbox is ordinary enough to open to the team.",
  },
};

export const WithoutSummary: Story = {
  args: { name: "Unknown task", task: "nightly_batch", summary: undefined },
};

export const WithSummaryDark: Story = {
  globals: { theme: "dark" },
  args: WithSummary.args,
};
