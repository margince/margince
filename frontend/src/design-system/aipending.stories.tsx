// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AiPending } from "./aipending";
import { Panel, PanelBody } from "./panel";

// The wait for a machine's answer, inside the panel it will fill. The tile
// breathes and a light passes over the lines; under reduced motion both rest
// and the shape alone says the answer is coming.
const meta: Meta<typeof AiPending> = {
  title: "Design System/AiPending",
  component: AiPending,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: 520 }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof AiPending>;

export const ReadingAnAccount: Story = {
  args: {
    label: "Margince is reading this account's exchanges and deals.",
    lines: 3,
  },
  render: (args) => (
    <Panel title="What needs you">
      <PanelBody>
        <AiPending {...args} />
      </PanelBody>
    </Panel>
  ),
};

export const OneLine: Story = {
  args: {
    label: "Weighing what it found.",
    lines: 1,
  },
};
