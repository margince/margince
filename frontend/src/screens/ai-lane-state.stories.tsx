// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { TaskState } from "./ai-lane-state";
import { StoryProviders } from "./story-utils";

// How the lane a task starts on is answering: green, red, or grey for a lane
// that took no calls in the window. Absent for a reader with no health read.

const health = {
  window_hours: 1,
  rungs: [
    {
      tier: "cheap_cloud",
      healthy: true,
      calls: 12,
      failures: 0,
      median_latency_ms: 800,
    },
    {
      tier: "decide",
      healthy: false,
      calls: 6,
      failures: 6,
      median_latency_ms: 430,
      last_sentinel: "decision_error",
    },
  ],
};

function States() {
  return (
    <StoryProviders>
      <div style={{ display: "flex", gap: 8 }}>
        <TaskState health={health} tier="cheap_cloud" decisionFirst={false} />
        <TaskState health={health} tier="decide" decisionFirst />
        <TaskState health={health} tier="premium" decisionFirst={false} />
        <TaskState health={undefined} tier="premium" decisionFirst={false} />
      </div>
    </StoryProviders>
  );
}

const meta: Meta<typeof States> = {
  title: "Settings/AI/Models and routing/Task state",
  component: States,
};
export default meta;
type Story = StoryObj<typeof States>;

export const Default: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
