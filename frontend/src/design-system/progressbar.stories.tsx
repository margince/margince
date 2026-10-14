// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ProgressBar } from "./progressbar";

const meta: Meta<typeof ProgressBar> = {
  title: "Components/Text and data display/Progress bar",
  component: ProgressBar,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProgressBar>;

/** A task part-way through, as a poll between two steps leaves it. */
export const Running: Story = {
  args: { fraction: 0.42, label: "Import progress", valueText: "42%" },
};

/** A task that has not reported a count yet. */
export const Empty: Story = {
  args: { fraction: 0, label: "Import progress", valueText: "0%" },
};

export const Complete: Story = {
  args: { fraction: 1, label: "Import progress", valueText: "100%" },
};
