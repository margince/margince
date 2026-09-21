// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BetaBadge } from "./betabadge";

// One story, because the marker has one state: it says the same word at both
// rail widths and in both themes. It goes when `betabadge.tsx` does — see that
// file's header for the rest of the list.
const meta: Meta<typeof BetaBadge> = {
  title: "Shell/Beta badge",
  component: BetaBadge,
};
export default meta;
type Story = StoryObj<typeof BetaBadge>;

export const Marker: Story = {};
