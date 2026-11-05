// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen } from "storybook/test";
import type { components } from "../../api/schema";
import { Modal } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { en } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { TaskOutcome } from "./ai-task-outcome";

const meta: Meta = {
  title: "Settings/AI/AI models/Task outcome",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const FLOW: components["schemas"]["AiTaskFlow"] = {
  task: "capture_confidentiality_verdict",
  window: "7d",
  total: 10,
  unanswered: 1,
  steps: [
    {
      decision: true,
      tier: "decide",
      provider: "jev_compatible",
      model: "typesafe/jev-1.13",
      attempts: 10,
      answered: 8,
      p50_ms: 500,
      gave_up: { timeout: 2 },
      switched: 0,
    },
    {
      decision: false,
      tier: "local_small",
      provider: "openai_compatible",
      model: "openai/gpt-oss-120b",
      attempts: 2,
      answered: 1,
      p50_ms: 1100,
      gave_up: { provider_error: 1 },
      switched: 0,
    },
  ],
};

export const InTheSheet: Story = {
  render: () => (
    <StoryProviders>
      <Modal open onClose={() => {}} labelledBy="outcome-title" intent="drawer">
        <Heading size="large" id="outcome-title" className="modal-title">
          {en["aiFigures.recentCalls"]}
        </Heading>
        <TaskOutcome flow={FLOW} />
      </Modal>
    </StoryProviders>
  ),
  play: async () => {
    await screen.findByRole("dialog");
  },
};

export const InTheSheetDark: Story = {
  ...InTheSheet,
  globals: { theme: "dark" },
};

/** The provider answered the tier step with another model than the binding. */
export const AProviderSwitchedTheModel: Story = {
  render: () => (
    <StoryProviders>
      <TaskOutcome
        flow={{
          ...FLOW,
          steps: FLOW.steps.map((step) =>
            step.decision
              ? step
              : {
                  ...step,
                  switched: 2,
                  switched_to: "openai/gpt-oss-120b-0925",
                },
          ),
        }}
      />
    </StoryProviders>
  ),
  play: async () => {
    await screen.findByText(/openai\/gpt-oss-120b-0925/);
  },
};
