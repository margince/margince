// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StagePips } from "./stagepips";

const meta: Meta<typeof StagePips> = {
  title: "Components/Text and data display/Stage pips",
  component: StagePips,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof StagePips>;

export const Moving: Story = {
  args: { at: 3, of: 5, label: "Proposal" },
};

// Stalled in its stage: the filled bars turn amber.
export const Stalled: Story = {
  args: { at: 2, of: 5, stalled: true, label: "Qualification" },
};
