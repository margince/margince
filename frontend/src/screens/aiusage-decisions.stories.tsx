// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { DecisionSummary } from "./aiusage-decisions";
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

// In a pane, as the usage card stands it: the table bleeds to the pane's edge.
const meta: Meta<typeof DecisionSummary> = {
  title: "Settings/AI/AI usage/Decision model",
  component: DecisionSummary,
  render: (args) => (
    <StoryProviders>
      <Panel title="Estimated AI spend and usage">
        <DecisionSummary {...args} />
      </Panel>
    </StoryProviders>
  ),
  args: {
    decisions: DECISIONS,
    taskName: (task: string) => task,
    taskSummary: () => undefined,
  },
};
export default meta;
type Story = StoryObj<typeof DecisionSummary>;

// The fallback rate is the button that explains itself; a task that never fell
// back shows its rate as plain text.
export const Fallbacks: Story = {};
export const FallbacksDark: Story = { globals: { theme: "dark" } };

// Opened: the reasons one per line, largest first.
export const ReasonsOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Fallbacks by reason for 78%",
      }),
    );
  },
};
export const ReasonsOpenDark: Story = {
  ...ReasonsOpen,
  globals: { theme: "dark" },
};
export const NothingAsked: Story = { args: { decisions: [] } };
export const FallbacksPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
