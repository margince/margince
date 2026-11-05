// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { feature } from "../../testing/ai-admin.testkit";
import { StoryProviders } from "../../testing/story-utils";
import { TaskDetails } from "./ai-task-details";

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

function Details({
  canTrace,
  keyless = false,
}: Readonly<{ canTrace: boolean; keyless?: boolean }>) {
  return (
    <StoryProviders>
      <TaskDetails
        row={deciding}
        health={undefined}
        providers={undefined}
        lanesNeedingAKey={
          keyless
            ? [
                {
                  lane: "decide",
                  provider: "jev",
                  model: "jev-1.13.0",
                  key_env: "TYPESAFE_API_KEY",
                },
              ]
            : undefined
        }
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

// The saved binding cannot run: the lane holds no key, so the model named
// above it is not the one answering.
export const LaneNeedsAKey: Story = { args: { keyless: true } };
export const LaneNeedsAKeyDark: Story = {
  ...LaneNeedsAKey,
  globals: { theme: "dark" },
};
