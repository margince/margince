// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Kbd } from "./atoms";

const meta: Meta<typeof Kbd> = {
  title: "Components/Text and data display/Kbd",
  component: Kbd,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof Kbd>;

// In the body face, inside a sentence: a key someone presses is not code.
export const KeyLegend: Story = {
  render: () => (
    <p className="t-caption">
      Press <Kbd>/</Kbd> to search, <Kbd>Ctrl</Kbd> <Kbd>K</Kbd> for the command
      bar, <Kbd>Esc</Kbd> to close.
    </p>
  ),
};
