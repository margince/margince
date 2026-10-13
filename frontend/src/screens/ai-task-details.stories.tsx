// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { feature } from "./ai-admin.testkit";
import { TaskDetails } from "./ai-task-details";
import { StoryProviders } from "./story-utils";

// What a task row opens onto, here for a task that asks a decision model first:
// the model's vendor is named, never its adapter key.
const deciding: components["schemas"]["AiFeatureRoute"] = {
  ...feature,
  decision_first: true,
  decision_candidate: {
    tier: "decide",
    provider: "jev_compatible",
    model: "typesafe/jev-1.13",
    processing: "cloud_provider",
  },
};

function Details({ canTrace }: Readonly<{ canTrace: boolean }>) {
  return (
    <StoryProviders>
      <TaskDetails
        row={deciding}
        health={undefined}
        providers={undefined}
        canTrace={canTrace}
        custom={false}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Details> = {
  title: "Settings/AI/AI models/Task details",
  component: Details,
  args: { canTrace: true },
};
export default meta;
type Story = StoryObj<typeof Details>;

export const DecisionFirst: Story = {};
export const DecisionFirstDark: Story = { globals: { theme: "dark" } };
