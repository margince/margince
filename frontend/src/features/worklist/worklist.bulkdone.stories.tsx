// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../../i18n/en";
import { StoryProviders } from "../../testing/story-utils";
import { useWorklistPicks, WorklistBulkBar } from "./worklist.bulkdone";
import { row } from "./worklist.fixtures";

// Selection mode over the loaded queue. Select opens it, and Select all ticks
// every row that takes a box. Cancel leaves it with focus back on Select.

const queue = [
  row({ id: "t1", title: "Send the revised offer", actions: ["complete"] }),
  row({
    id: "t2",
    title: "Call back about the rollout",
    actions: ["complete"],
  }),
];

function Bar() {
  const selection = useWorklistPicks(queue, "today");
  return <WorklistBulkBar selection={selection} />;
}

const meta: Meta<typeof WorklistBulkBar> = {
  title: "Records/Worklist/Selection",
  component: WorklistBulkBar,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof WorklistBulkBar>;

export const SelectAndCancel: Story = {
  render: () => (
    <StoryProviders>
      <Bar />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      canvas.getByRole("button", { name: en["worklist.bulk.select"] }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: "Select all 2 items shown" }),
    );
    await expect(
      canvas.getByRole("button", { name: "Complete 2 tasks" }),
    ).toBeVisible();
    await userEvent.click(
      canvas.getByRole("button", { name: en["worklist.bulk.cancel"] }),
    );
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: en["worklist.bulk.select"] }),
      ).toHaveFocus(),
    );
  },
};
