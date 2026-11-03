// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { SummaryFields } from "./approvalsummary";

const words = {
  field: (name: string) =>
    (({ name: "Name", stage_id: "Stage" }) as Record<string, string>)[name] ??
    name,
  more: (count: number) => `${count} more not shown`,
};

const meta: Meta<typeof SummaryFields> = {
  title: "Components/Text and data display/Summary fields",
  component: SummaryFields,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "26rem" }}>
        <Story />
      </div>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof SummaryFields>;

// A value with ", " in it stays one row: the joined summary could not show that.
export const ProposedValues: Story = {
  args: {
    words,
    parts: {
      sentence: "Update record deal",
      fields: [
        { field: "name", value: "Acme, Inc. renewal" },
        { field: "stage_id", value: "Negotiation" },
      ],
      more_fields: 2,
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Acme, Inc. renewal")).toBeVisible();
    await expect(canvas.getByText("2 more not shown")).toBeVisible();
  },
};
